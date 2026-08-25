package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// openTestDB opens a fresh writable db with the schema ensured, returning the
// connection directly so a test can drive it without going through EnsureIndexed.
func openTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbp := filepath.Join(dir, "test.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("store.ConnectRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return con, dbp
}

// TestEnsureSchemaMigratesPreV4DB guards the v3→v4 migration path that a green
// unit suite shipped broken: EnsureSchema used to run the full Schema (which
// adds idx_msg_session_uuid on messages.uuid) BEFORE checking the version, so a
// real pre-v4 cache died with "no such column: uuid" instead of rebuilding.
func TestEnsureSchemaMigratesPreV4DB(t *testing.T) {
	dbp := filepath.Join(t.TempDir(), "old.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("store.ConnectRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })
	// A pre-v4 cache: messages WITHOUT the uuid column, stamped at an old version.
	if _, err := con.Exec(`
		CREATE TABLE messages (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
			role TEXT, content TEXT, ts REAL, ts_iso TEXT);
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
		INSERT INTO meta(key,value) VALUES('schema_version','3');`); err != nil {
		t.Fatalf("seed pre-v4 schema: %v", err)
	}
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema must migrate a pre-v4 db, got: %v", err)
	}
	if _, err := con.Exec("SELECT uuid FROM messages LIMIT 1"); err != nil {
		t.Errorf("messages.uuid missing after migration: %v", err)
	}
	var v string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&v); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if v == "3" {
		t.Errorf("schema_version still '3' after migration — rebuild did not stamp the new version")
	}
}

// TestMigrateDurabilityColumns_FailureReturned proves that a failure during the
// provenance backfill UPDATE is returned to the caller rather than silently swallowed.
func TestMigrateDurabilityColumns_FailureReturned(t *testing.T) {
	dbp := filepath.Join(t.TempDir(), "durability_fail.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("store.ConnectRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })

	// Pre-durability schema: sessions without durability columns
	if _, err := con.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			started_at REAL,
			last_ts REAL,
			message_count INTEGER,
			is_subagent INTEGER,
			parent_id TEXT
		);
		CREATE TABLE file_index (
			path TEXT PRIMARY KEY,
			mtime REAL,
			size INTEGER,
			fp TEXT,
			session_id TEXT
		);
		INSERT INTO sessions (id, started_at, last_ts, message_count, is_subagent)
		VALUES ('sess-fail', 1.0, 2.0, 1, 0);
		INSERT INTO file_index (path, mtime, size, fp, session_id)
		VALUES ('/repo/sess-fail.jsonl', 100.0, 50, 'fp1', 'sess-fail');
	`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}

	// Install a trigger that causes UPDATE on sessions to fail
	if _, err := con.Exec(`
		CREATE TRIGGER fail_provenance_update
		BEFORE UPDATE OF origin_machine ON sessions
		BEGIN
			SELECT RAISE(ABORT, 'injected backfill failure');
		END;
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	err = migrateDurabilityColumns(con, "claude")
	if err == nil {
		t.Fatal("migrateDurabilityColumns succeeded, want non-nil error when UPDATE fails")
	}
	if !strings.Contains(err.Error(), "backfill provenance") || !strings.Contains(err.Error(), "injected backfill failure") {
		t.Errorf("error = %q, want it to contain 'backfill provenance' and 'injected backfill failure'", err.Error())
	}

	// Verify the session row's origin_machine is still NULL (backfill did not complete)
	var origin sql.NullString
	if err := con.QueryRow("SELECT origin_machine FROM sessions WHERE id='sess-fail'").Scan(&origin); err != nil {
		t.Fatalf("query origin_machine: %v", err)
	}
	if origin.Valid {
		t.Errorf("origin_machine = %q, want NULL after aborted backfill", origin.String)
	}
}

// TestMigrateDurabilityColumns_Idempotent verifies that running migrateDurabilityColumns
// adds missing columns and backfills provenance, and running it a second time (or on
// an already-migrated db) is a clean no-op returning nil.
func TestMigrateDurabilityColumns_Idempotent(t *testing.T) {
	dbp := filepath.Join(t.TempDir(), "durability_idempotent.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("store.ConnectRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })

	// Pre-durability schema with an unbackfilled row
	if _, err := con.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			started_at REAL,
			last_ts REAL,
			message_count INTEGER,
			is_subagent INTEGER,
			parent_id TEXT
		);
		CREATE TABLE file_index (
			path TEXT PRIMARY KEY,
			mtime REAL,
			size INTEGER,
			fp TEXT,
			session_id TEXT
		);
		INSERT INTO sessions (id, started_at, last_ts, message_count, is_subagent)
		VALUES ('sess-idem', 1.0, 2.0, 1, 0);
		INSERT INTO file_index (path, mtime, size, fp, session_id)
		VALUES ('/repo/sess-idem.jsonl', 100.0, 50, 'fp1', 'sess-idem');
	`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}

	// First pass: adds columns and backfills
	if err := migrateDurabilityColumns(con, "claude"); err != nil {
		t.Fatalf("first migrateDurabilityColumns: %v", err)
	}

	var origin, tool, path string
	var missing sql.NullFloat64
	if err := con.QueryRow("SELECT origin_machine, source_tool, source_path, missing_since FROM sessions WHERE id='sess-idem'").Scan(&origin, &tool, &path, &missing); err != nil {
		t.Fatalf("scan backfilled row: %v", err)
	}
	if origin != provenance.MachineID() {
		t.Errorf("origin_machine = %q, want %q", origin, provenance.MachineID())
	}
	if tool != "claude" {
		t.Errorf("source_tool = %q, want claude", tool)
	}
	if path != "/repo/sess-idem.jsonl" {
		t.Errorf("source_path = %q, want /repo/sess-idem.jsonl", path)
	}
	if missing.Valid {
		t.Errorf("missing_since = %v, want NULL", missing.Float64)
	}

	// Second pass: idempotent clean no-op
	if err := migrateDurabilityColumns(con, "claude"); err != nil {
		t.Fatalf("second migrateDurabilityColumns (idempotent): %v", err)
	}

	var origin2, tool2, path2 string
	var missing2 sql.NullFloat64
	if err := con.QueryRow("SELECT origin_machine, source_tool, source_path, missing_since FROM sessions WHERE id='sess-idem'").Scan(&origin2, &tool2, &path2, &missing2); err != nil {
		t.Fatalf("scan row after second run: %v", err)
	}
	if origin2 != origin || tool2 != tool || path2 != path || missing2.Valid != missing.Valid {
		t.Errorf("row values mutated on second run: got (%q, %q, %q, %v), want (%q, %q, %q, %v)",
			origin2, tool2, path2, missing2, origin, tool, path, missing)
	}

	// Also verify on an already fully ensured schema (e.g. EnsureSchema)
	conFresh, _ := openTestDB(t)
	if err := migrateDurabilityColumns(conFresh, "codex"); err != nil {
		t.Fatalf("migrateDurabilityColumns on fresh schema: %v", err)
	}
}

func TestFTS5OK(t *testing.T) {
	if !FTS5OK() {
		t.Fatal("FTS5OK() = false; modernc.org/sqlite must support FTS5")
	}
}

func TestDBPath(t *testing.T) {
	got := DBPath("/Users/x/.claude/projects/-foo-bar")
	want := filepath.Join(store.CacheDir(), "-foo-bar.db")
	if got != want {
		t.Errorf("DBPath = %q, want %q", got, want)
	}
}

func TestEnsureSchemaStampsVersion(t *testing.T) {
	con, _ := openTestDB(t)
	var v string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&v); err != nil {
		t.Fatalf("schema_version not stamped: %v", err)
	}
	if v != "4" {
		t.Errorf("schema_version = %q, want 4", v)
	}
	// FTS table must exist.
	if _, err := con.Exec("SELECT 1 FROM messages_fts LIMIT 1"); err != nil {
		t.Errorf("messages_fts missing after EnsureSchema: %v", err)
	}
	// Re-running EnsureSchema must be idempotent (no rebuild, no error).
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("second EnsureSchema: %v", err)
	}
}

func TestEnsureSchemaRebuildsOnVersionMismatch(t *testing.T) {
	con, _ := openTestDB(t)
	// Insert a message, then force a version mismatch and re-ensure -> rebuild wipes it.
	if _, err := con.Exec("INSERT INTO messages(session_id,role,content,ts,ts_iso) VALUES('s','user','hi',1,'2026-01-01')"); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("UPDATE meta SET value='1' WHERE key='schema_version'"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema rebuild: %v", err)
	}
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("rebuild should have wiped messages, got %d rows", n)
	}
	var v string
	con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&v)
	if v != "4" {
		t.Errorf("version not re-stamped to 4, got %q", v)
	}
}

// writeJSONL writes a transcript file from raw line strings.
func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReindexFileBasic(t *testing.T) {
	con, _ := openTestDB(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "sess1.jsonl")
	writeJSONL(t, f,
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"first question about deploy"}}`,
		`{"type":"assistant","timestamp":"2026-06-01T10:01:00Z","message":{"role":"assistant","content":[{"type":"text","text":"the answer is here"}]}}`,
		``, // blank line skipped
		`{"type":"summary","summary":"a short recap"}`,
		`{not valid json`, // malformed, skipped
	)

	if !ReindexFile(con, f, dir) {
		t.Fatal("ReindexFile returned false on a valid file")
	}

	var nmsg int
	con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='sess1'").Scan(&nmsg)
	if nmsg != 3 {
		t.Errorf("got %d messages, want 3 (user+assistant+summary)", nmsg)
	}

	// Session watermark row exists with the right count.
	var mc int
	var started, last float64
	if err := con.QueryRow("SELECT message_count,started_at,last_ts FROM sessions WHERE id='sess1'").Scan(&mc, &started, &last); err != nil {
		t.Fatalf("session row missing: %v", err)
	}
	if mc != 3 {
		t.Errorf("message_count = %d, want 3", mc)
	}
	if started == 0 || last == 0 || last < started {
		t.Errorf("bad watermark started=%v last=%v", started, last)
	}

	// FTS is populated via triggers — a keyword query finds the message.
	var hits int
	con.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'deploy'").Scan(&hits)
	if hits != 1 {
		t.Errorf("FTS MATCH 'deploy' = %d hits, want 1", hits)
	}
}

func TestReindexFileAtomicReplace(t *testing.T) {
	con, _ := openTestDB(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "s.jsonl")

	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"version one"}}`)
	if !ReindexFile(con, f, dir) {
		t.Fatal("first reindex failed")
	}
	writeJSONL(t, f,
		`{"type":"user","timestamp":"2026-06-01T11:00:00Z","message":{"role":"user","content":"version two alpha"}}`,
		`{"type":"user","timestamp":"2026-06-01T11:01:00Z","message":{"role":"user","content":"version two beta"}}`,
	)
	if !ReindexFile(con, f, dir) {
		t.Fatal("second reindex failed")
	}

	var n int
	con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s'").Scan(&n)
	if n != 2 {
		t.Errorf("after replace got %d messages, want 2 (old row must be gone)", n)
	}
	var hits int
	con.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'one'").Scan(&hits)
	if hits != 0 {
		t.Errorf("stale FTS row 'one' should be gone, got %d", hits)
	}
}

func TestReindexFile_RollbackOnFailure(t *testing.T) {
	con, _ := openTestDB(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "atomic_file.jsonl")

	writeJSONL(t, f,
		`{"type":"user","uuid":"u-orig-1","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"original version line 1"}}`,
		`{"type":"user","uuid":"u-orig-2","timestamp":"2026-06-01T10:01:00Z","message":{"role":"user","content":"original version line 2"}}`,
	)
	if !ReindexFile(con, f, dir) {
		t.Fatal("initial reindex failed")
	}

	// Install trigger to inject failure during message insertion
	if _, err := con.Exec("CREATE TRIGGER abort_reindex BEFORE INSERT ON messages WHEN new.role = 'fail' BEGIN SELECT RAISE(ABORT, 'injected reindex failure'); END;"); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	// Write updated file containing a trigger-matching line
	writeJSONL(t, f,
		`{"type":"user","uuid":"u-new-1","timestamp":"2026-06-01T11:00:00Z","message":{"role":"user","content":"new version line 1"}}`,
		`{"type":"user","uuid":"u-new-2","timestamp":"2026-06-01T11:01:00Z","message":{"role":"fail","content":"new version line 2"}}`,
	)

	if ReindexFile(con, f, dir) {
		t.Fatal("ReindexFile should return false on transaction abort")
	}

	// Verify the original session and its messages are 100% intact
	var count int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='atomic_file'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("after rollback message count = %d, want 2 (original messages preserved)", count)
	}

	var firstMsg string
	if err := con.QueryRow("SELECT content FROM messages WHERE session_id='atomic_file' AND uuid='u-orig-1'").Scan(&firstMsg); err != nil {
		t.Fatalf("query original message: %v", err)
	}
	if firstMsg != "original version line 1" {
		t.Errorf("content = %q, want %q", firstMsg, "original version line 1")
	}
}

func TestReindexFileMissingReturnsFalse(t *testing.T) {
	con, _ := openTestDB(t)
	dir := t.TempDir()
	if ReindexFile(con, filepath.Join(dir, "ghost.jsonl"), dir) {
		t.Error("ReindexFile on a missing file should return false")
	}
}

func TestReindexFileSubagentParent(t *testing.T) {
	con, _ := openTestDB(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "parent", "subagents", "child.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"sub thread"}}`)
	if !ReindexFile(con, f, dir) {
		t.Fatal("reindex subagent failed")
	}
	var isSub int
	var parent sql.NullString
	if err := con.QueryRow("SELECT is_subagent,parent_id FROM sessions WHERE id='parent/child'").Scan(&isSub, &parent); err != nil {
		t.Fatalf("subagent session row missing: %v", err)
	}
	if isSub != 1 {
		t.Errorf("is_subagent = %d, want 1", isSub)
	}
	if !parent.Valid || parent.String != "parent" {
		t.Errorf("parent_id = %v, want 'parent'", parent)
	}
}

func TestUpdateIndexIncrementalAndPrune(t *testing.T) {
	// Lay out a project transcript dir; UpdateIndex must index it, skip an
	// unchanged file on the second pass, and — under durable retention (D1) — a
	// file that merely vanishes from the walk (no tombstone) is RETAINED with
	// missing_since stamped, NOT pruned. Only an explicit tombstone deletes.
	proj := t.TempDir()
	a := filepath.Join(proj, "a.jsonl")
	b := filepath.Join(proj, "b.jsonl")
	writeJSONL(t, a, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"alpha content"}}`)
	writeJSONL(t, b, `{"type":"user","timestamp":"2026-06-01T11:00:00Z","message":{"role":"user","content":"beta content"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	var ns int
	con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&ns)
	if ns != 2 {
		t.Fatalf("after pass 1 got %d sessions, want 2", ns)
	}
	var nfi int
	con.QueryRow("SELECT COUNT(*) FROM file_index").Scan(&nfi)
	if nfi != 2 {
		t.Errorf("file_index rows = %d, want 2", nfi)
	}

	// Pass 2 with no changes: still 2 sessions (idempotent).
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}
	con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&ns)
	if ns != 2 {
		t.Errorf("after idempotent pass got %d sessions, want 2", ns)
	}

	// Remove b's backing file (no tombstone), reindex: durable retention keeps the
	// session — both rows survive, and b is flagged missing_since (D1).
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 3: %v", err)
	}
	con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&ns)
	if ns != 2 {
		t.Errorf("after a merely-missing file got %d sessions, want 2 (retained, not pruned)", ns)
	}
	var exists int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='b'").Scan(&exists)
	if exists != 1 {
		t.Errorf("session 'b' must be retained when its file merely vanishes (no tombstone)")
	}
	var missing sql.NullFloat64
	if err := con.QueryRow("SELECT missing_since FROM sessions WHERE id='b'").Scan(&missing); err != nil {
		t.Fatalf("read b.missing_since: %v", err)
	}
	if !missing.Valid || missing.Float64 <= 0 {
		t.Errorf("session 'b' missing_since = %+v, want a positive timestamp", missing)
	}
	// a is still present on disk → its missing_since stays NULL.
	var aMissing sql.NullFloat64
	con.QueryRow("SELECT missing_since FROM sessions WHERE id='a'").Scan(&aMissing)
	if aMissing.Valid {
		t.Errorf("session 'a' (present) should not be flagged missing, got %v", aMissing.Float64)
	}
}

// TestRefSurvivesReindex is the #1 regression: a uuid-anchored ref must resolve
// to the SAME message after the transcript is appended-to and reindexed, even
// though the AUTOINCREMENT rowid is reassigned. Before C1 the external ref was
// the rowid, which churned; now it is the source uuid, which is stable.
func TestRefSurvivesReindex(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f,
		`{"type":"user","uuid":"uuid-aaa-1111","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"the anchored question"}}`,
		`{"type":"assistant","uuid":"uuid-bbb-2222","timestamp":"2026-06-01T10:01:00Z","message":{"role":"assistant","content":[{"type":"text","text":"the answer"}]}}`,
	)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}

	// Capture the (uuid -> id, content) for the anchor message.
	uuidToContent := func(uuid string) (id int, content string, found bool) {
		row := con.QueryRow("SELECT id, content FROM messages WHERE session_id='s' AND uuid=?", uuid)
		if err := row.Scan(&id, &content); err != nil {
			return 0, "", false
		}
		return id, content, true
	}
	id1, content1, ok := uuidToContent("uuid-aaa-1111")
	if !ok {
		t.Fatal("anchor uuid not resolvable after pass 1")
	}

	// Append a NEW turn and bump mtime so the watermark check fires and the
	// session is reindexed (DELETE + re-INSERT → rowids reassigned).
	writeJSONL(t, f,
		`{"type":"user","uuid":"uuid-aaa-1111","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"the anchored question"}}`,
		`{"type":"assistant","uuid":"uuid-bbb-2222","timestamp":"2026-06-01T10:01:00Z","message":{"role":"assistant","content":[{"type":"text","text":"the answer"}]}}`,
		`{"type":"user","uuid":"uuid-ccc-3333","timestamp":"2026-06-01T10:02:00Z","message":{"role":"user","content":"a follow-up that shifts ids"}}`,
	)
	if err := os.Chtimes(f, mustTime(), mustTime()); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}

	id2, content2, ok := uuidToContent("uuid-aaa-1111")
	if !ok {
		t.Fatal("anchor uuid not resolvable after reindex — ref churn regressed")
	}
	// The ref (uuid) resolves to the SAME message content across reindex.
	if content2 != content1 {
		t.Errorf("uuid resolved to different content after reindex: %q vs %q", content2, content1)
	}
	if content2 != "the anchored question" {
		t.Errorf("uuid resolved to wrong message: %q", content2)
	}
	// The internal rowid is allowed to change; that is the whole point of
	// anchoring on uuid instead.
	t.Logf("rowid before=%d after=%d (churn is expected; uuid is the stable handle)", id1, id2)
}

func TestUpdateIndexReindexesChangedFile(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "c.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"original wording"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}

	// Rewrite with different content AND bump mtime so the watermark check fires.
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"revised wording entirely"}}`)
	future := os.Chtimes(f, mustTime(), mustTime())
	if future != nil {
		t.Fatal(future)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	var hits int
	con.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'revised'").Scan(&hits)
	if hits != 1 {
		t.Errorf("reindex should pick up 'revised', got %d hits", hits)
	}
	con.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'original'").Scan(&hits)
	if hits != 0 {
		t.Errorf("stale 'original' should be gone, got %d hits", hits)
	}
}

func TestEnsureIndexedEndToEnd(t *testing.T) {
	// EnsureIndexed must create the db under DBPath and report the session count.
	// Use a temp HOME so DBPath lands in an isolated cache dir.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "x.jsonl"),
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hello indexed world"}}`)

	dbp, n, _, err := EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	if n != 1 {
		t.Errorf("n sessions = %d, want 1", n)
	}
	if _, statErr := os.Stat(dbp); statErr != nil {
		t.Errorf("db not created at %s: %v", dbp, statErr)
	}

	// CountSessions on the same db agrees.
	if got := store.CountSessions(dbp); got != 1 {
		t.Errorf("CountSessions = %d, want 1", got)
	}

	// CorpusStats reflects the single user message.
	cs, err := store.GetCorpusStats(dbp)
	if err != nil {
		t.Fatalf("GetCorpusStats: %v", err)
	}
	if cs.Sessions != 1 || cs.Messages != 1 || cs.User != 1 {
		t.Errorf("CorpusStats = %+v, want 1 session / 1 message / 1 user", cs)
	}
	if cs.First != "2026-06-01" || cs.Last != "2026-06-01" {
		t.Errorf("CorpusStats span = (%q,%q), want 2026-06-01", cs.First, cs.Last)
	}
}

func TestEnsureIndexedReindexWipes(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "y.jsonl"),
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"keep me"}}`)

	dbp, _, _, err := EnsureIndexed(proj, false)
	if err != nil {
		t.Fatal(err)
	}
	// reindex=true removes the db first, then rebuilds — still 1 session.
	_, n, _, err := EnsureIndexed(proj, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("after reindex got %d sessions, want 1", n)
	}
	_ = dbp
}

func TestCountSessionsMissingDB(t *testing.T) {
	if got := store.CountSessions(filepath.Join(t.TempDir(), "nope.db")); got != -1 {
		t.Errorf("CountSessions on missing db = %d, want -1 (unknown sentinel)", got)
	}
}

func TestConnectROIsReadOnly(t *testing.T) {
	con, dbp := openTestDB(t)
	con.Close()

	ro, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("ConnectRO: %v", err)
	}
	defer ro.Close()
	// A read works.
	var n int
	if err := ro.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
		t.Fatalf("read via RO conn: %v", err)
	}
	// A write must fail (read-only).
	if _, err := ro.Exec("INSERT INTO meta(key,value) VALUES('x','y')"); err == nil {
		t.Error("write via ConnectRO should fail (read-only)")
	}
}

func TestGetCorpusStatsMissingDB(t *testing.T) {
	// A db file that exists but has no schema returns zero-value stats and a nil
	// error rather than failing.
	dir := t.TempDir()
	dbp := filepath.Join(dir, "blank.db")
	if err := os.WriteFile(dbp, []byte("not a db"), 0o644); err != nil {
		t.Fatal(err)
	}
	cs, err := store.GetCorpusStats(dbp)
	if err != nil {
		t.Fatalf("GetCorpusStats should not error on a bad db, got %v", err)
	}
	if cs != (store.CorpusStats{}) {
		t.Errorf("expected zero CorpusStats, got %+v", cs)
	}
}

// TestUpdateIndexSkipsTombstoned is the lifecycle-integration guard: a session
// id recorded in the tombstone sidecar (~/.cache/session-search/.deleted) must
// NOT be re-indexed on a reindex pass, even though its transcript .jsonl is
// present on disk. This is what stops a user-deleted session from being
// resurrected by the next index run.
func TestUpdateIndexSkipsTombstoned(t *testing.T) {
	// Isolate HOME so lifecycle.LoadTombstones("") reads our seeded sidecar at
	// $HOME/.cache/session-search/.deleted and nothing from the real machine.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := t.TempDir()
	keep := filepath.Join(proj, "keep.jsonl")
	dead := filepath.Join(proj, "dead.jsonl")
	writeJSONL(t, keep, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"keep this session"}}`)
	writeJSONL(t, dead, `{"type":"user","timestamp":"2026-06-01T11:00:00Z","message":{"role":"user","content":"this session was deleted"}}`)

	// Tombstone the "dead" session id (the .jsonl stem == the top-level session
	// id) in the sidecar the indexer consults.
	tombDir := filepath.Join(tmpHome, ".cache", "session-search")
	if err := os.MkdirAll(tombDir, 0o755); err != nil {
		t.Fatalf("mkdir tombstone dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tombDir, ".deleted"), []byte("dead\n"), 0o644); err != nil {
		t.Fatalf("write tombstone: %v", err)
	}

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	// The tombstoned session must be absent; the live one must be present.
	var deadCount int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='dead'").Scan(&deadCount)
	if deadCount != 0 {
		t.Errorf("tombstoned session 'dead' was indexed (count=%d), want 0 — a deleted session was resurrected", deadCount)
	}
	var keepCount int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='keep'").Scan(&keepCount)
	if keepCount != 1 {
		t.Errorf("live session 'keep' = %d, want 1 (tombstone must not over-skip)", keepCount)
	}

	// No messages from the dead transcript should have leaked into the index/FTS.
	var deadMsgs int
	con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='dead'").Scan(&deadMsgs)
	if deadMsgs != 0 {
		t.Errorf("tombstoned session left %d messages, want 0", deadMsgs)
	}
	var hits int
	con.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'deleted'").Scan(&hits)
	if hits != 0 {
		t.Errorf("FTS still matches deleted-session content (%d hits), want 0", hits)
	}
}

// mustTime returns a fixed time in the future for Chtimes mtime bumping.
func mustTime() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }

// TestUpdateIndexMsgIDDeterministic pins the determinism of msg_id assignment.
// msg_id is the SQLite AUTOINCREMENT rowid, assigned in the order files are
// inserted; that order is set by the file walk. The walk
// (paths.ContainedJSONL -> sort.Strings) is lexicographically SORTED, so the same
// corpus always produces the same msg_ids — reproducible across runs and machines.
//
// The id is a session-internal handle for read, not a
// stable external contract: queries match on WHICH sessions/messages they hit,
// not on exact anchor/ref id numbers. The sorted walk makes those internal ids
// deterministic, and this test pins that invariant.
func TestUpdateIndexMsgIDDeterministic(t *testing.T) {
	build := func() map[string][2]any {
		proj := t.TempDir()
		// Names chosen so insertion order matters: 'z' before 'a' lexically? no —
		// sorted walk inserts a.jsonl then z.jsonl regardless of creation order.
		writeJSONL(t, filepath.Join(proj, "z.jsonl"),
			`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"zulu one"}}`)
		writeJSONL(t, filepath.Join(proj, "a.jsonl"),
			`{"type":"user","timestamp":"2026-06-01T11:00:00Z","message":{"role":"user","content":"alpha one"}}`)
		con, _ := openTestDB(t)
		if err := UpdateIndex(con, proj); err != nil {
			t.Fatalf("UpdateIndex: %v", err)
		}
		rows, err := con.Query("SELECT content, id FROM messages")
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()
		got := map[string][2]any{}
		for rows.Next() {
			var content string
			var id int
			if err := rows.Scan(&content, &id); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got[content] = [2]any{id, true}
		}
		return got
	}

	first := build()
	second := build()

	if len(first) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(first))
	}
	// Sorted walk => a.jsonl inserted first => alpha gets the lower id.
	alpha := first["alpha one"][0].(int)
	zulu := first["zulu one"][0].(int)
	if alpha >= zulu {
		t.Errorf("sorted walk should insert a.jsonl before z.jsonl: alpha id %d should be < zulu id %d", alpha, zulu)
	}
	// Two independent builds of the same corpus must yield identical msg_ids.
	for content, v := range first {
		if second[content][0].(int) != v[0].(int) {
			t.Errorf("msg_id for %q not reproducible: %d vs %d", content, v[0], second[content][0])
		}
	}
}

// TestEnsureSchemaAddsTrigramIndexInPlace pins the migration decision: a db
// already at the current schema version gains the substring index WITHOUT a
// rebuild. Rebuilding to get it would drop the messages table and force a
// re-walk of the live transcript tree, which re-prunes every session retained
// after its source was purged — so the test asserts the existing rows are still
// there afterwards, and that the backfill covered them.
func TestEnsureSchemaAddsTrigramIndexInPlace(t *testing.T) {
	dbp := filepath.Join(t.TempDir(), "current.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("store.ConnectRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })

	// A db at the CURRENT version that predates the substring index: build the
	// full current shape, then drop the trigram objects to reproduce it.
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema (seed): %v", err)
	}
	if _, err := con.Exec(`INSERT INTO sessions(id,message_count,is_subagent) VALUES('ledger',1,0);
		INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid)
		VALUES('ledger','user','the reconciliation finished cleanly',100,'2026-01-01T10:00:00Z','uuid-l1');`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}
	if _, err := con.Exec(`DROP TRIGGER messages_tri_ai; DROP TRIGGER messages_tri_ad;
		DROP TRIGGER messages_tri_au; DROP TABLE messages_fts_trigram;
		DELETE FROM meta WHERE key='trigram_backfill_done';`); err != nil {
		t.Fatalf("strip trigram objects: %v", err)
	}

	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema must add the substring index in place, got: %v", err)
	}

	// No rebuild: the pre-existing row survived and the version never moved.
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("messages = %d rows after migration, want 1 — the db was rebuilt, not migrated", n)
	}
	var v string
	con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&v)
	if v != strconv.Itoa(store.SchemaVersion) {
		t.Errorf("schema_version = %q, want %d", v, store.SchemaVersion)
	}

	// The backfill covered the row that was already there: "iliat" sits inside
	// "reconciliation", so only the substring index can find it.
	var hits int
	if err := con.QueryRow(
		`SELECT COUNT(*) FROM messages_fts_trigram WHERE messages_fts_trigram MATCH '"iliat"'`).Scan(&hits); err != nil {
		t.Fatalf("query substring index: %v", err)
	}
	if hits != 1 {
		t.Errorf("substring index = %d hits for a pre-existing message, want 1 (backfill did not run)", hits)
	}

	// A row inserted after the migration arrives through the re-created triggers.
	if _, err := con.Exec(
		`INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid)
		 VALUES('ledger','user','the settlement finished cleanly',200,'2026-01-02T10:00:00Z','uuid-l2')`); err != nil {
		t.Fatal(err)
	}
	if err := con.QueryRow(
		`SELECT COUNT(*) FROM messages_fts_trigram WHERE messages_fts_trigram MATCH '"tleme"'`).Scan(&hits); err != nil {
		t.Fatalf("query substring index: %v", err)
	}
	if hits != 1 {
		t.Errorf("substring index = %d hits for a post-migration message, want 1 (triggers not re-created)", hits)
	}
}

// TestTrigramBackfillRunsOnce pins the marker: once the substring index is
// filled, a later EnsureSchema must not empty and re-fill it. The re-fill is
// only cheap on a fresh db — on a real corpus it is the whole index rebuilt on
// every invocation.
func TestTrigramBackfillRunsOnce(t *testing.T) {
	con, _ := openTestDB(t)
	if _, err := con.Exec(
		`INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid)
		 VALUES('ledger','user','the reconciliation finished cleanly',100,'2026-01-01T10:00:00Z','uuid-l1')`); err != nil {
		t.Fatal(err)
	}
	var done string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='trigram_backfill_done'").Scan(&done); err != nil || done != "1" {
		t.Fatalf("trigram_backfill_done = %q (%v), want \"1\" — a fresh db must be stamped as filled", done, err)
	}

	// Empty the index behind the marker's back. A second EnsureSchema honors the
	// marker and leaves it empty; if it re-filled, the marker is being ignored.
	if _, err := con.Exec(store.TrigramResetSQL); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("second EnsureSchema: %v", err)
	}
	var hits int
	con.QueryRow(`SELECT COUNT(*) FROM messages_fts_trigram WHERE messages_fts_trigram MATCH '"iliat"'`).Scan(&hits)
	if hits != 0 {
		t.Errorf("substring index re-filled despite the done marker (%d hits) — every run would rebuild it", hits)
	}
}

// TestTrigramBackfillResumesFromWatermark pins the property that makes the
// backfill survivable: a run killed part-way leaves a watermark, and the next
// run continues from it rather than starting over. On a large corpus the whole
// fill takes longer than the CLI watchdog allows one run to live, so a pass
// that restarted would be killed at the same point every time and never finish.
func TestTrigramBackfillResumesFromWatermark(t *testing.T) {
	con, _ := openTestDB(t)
	if _, err := con.Exec(
		`INSERT INTO sessions(id,message_count,is_subagent) VALUES('ledger',3,0);
		 INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES
		   ('ledger','user','the reconciliation finished cleanly',100,'2026-01-01T10:00:00Z','uuid-1'),
		   ('ledger','user','the settlement finished cleanly',200,'2026-01-02T10:00:00Z','uuid-2'),
		   ('ledger','user','the adjustment finished cleanly',300,'2026-01-03T10:00:00Z','uuid-3');`); err != nil {
		t.Fatal(err)
	}

	// Reproduce what a killed backfill leaves behind: the index holds exactly
	// the rows the watermark claims, the done marker is absent. The entry for
	// row 1 is given content the message does NOT have, which is how the test
	// can tell resuming from starting over — a pass that re-read row 1 would
	// overwrite the sentinel, a pass that resumed leaves it alone.
	if _, err := con.Exec(
		`DELETE FROM meta WHERE key='trigram_backfill_done';
		 DELETE FROM messages_fts_trigram;
		 INSERT INTO messages_fts_trigram(rowid, content) VALUES (1, 'sentinelmarker');
		 INSERT OR REPLACE INTO meta(key,value) VALUES('trigram_backfill_at','1');`); err != nil {
		t.Fatal(err)
	}

	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("resumed EnsureSchema: %v", err)
	}

	// Every message is indexed exactly once: the resumed pass covered 2 and 3,
	// and did not duplicate 1.
	var total int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages_fts_trigram").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("substring index holds %d entries, want 3", total)
	}
	// The sentinel survived, so the pass started above the watermark instead of
	// re-doing work already committed.
	var kept int
	if err := con.QueryRow(
		`SELECT COUNT(*) FROM messages_fts_trigram WHERE messages_fts_trigram MATCH '"tinelma"'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("the entry below the watermark was re-read — the pass restarted instead of resuming")
	}
	for _, probe := range []string{`"tleme"`, `"justme"`} {
		var n int
		if err := con.QueryRow(
			"SELECT COUNT(*) FROM messages_fts_trigram WHERE messages_fts_trigram MATCH ?", probe).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("probe %s = %d hits, want 1 (the resumed batch missed it)", probe, n)
		}
	}
	// Finished: the marker is set and the resume point is gone.
	var done string
	con.QueryRow("SELECT value FROM meta WHERE key='trigram_backfill_done'").Scan(&done)
	if done != "1" {
		t.Errorf("done marker = %q, want \"1\"", done)
	}
	var at string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='trigram_backfill_at'").Scan(&at); err == nil {
		t.Errorf("resume point %q left behind after the fill finished", at)
	}
}

// TestTrigramBackfillToleratesRowsAlreadyIndexed pins the one collision the
// batched fill can meet in the wild: another process writing a message during
// the backfill indexes it through the insert trigger, so a later batch copies a
// row that is already there. That must be a no-op, not a failed migration.
func TestTrigramBackfillToleratesRowsAlreadyIndexed(t *testing.T) {
	con, _ := openTestDB(t)
	if _, err := con.Exec(
		`INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid)
		 VALUES('ledger','user','the reconciliation finished cleanly',100,'2026-01-01T10:00:00Z','uuid-1')`); err != nil {
		t.Fatal(err)
	}
	// The row is already indexed (the trigger did it) but the watermark says
	// the fill has not reached it yet — exactly the concurrent-writer state.
	if _, err := con.Exec(
		`DELETE FROM meta WHERE key='trigram_backfill_done';
		 INSERT OR REPLACE INTO meta(key,value) VALUES('trigram_backfill_at','0');`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema over an already-indexed row: %v", err)
	}
	var n int
	con.QueryRow("SELECT COUNT(*) FROM messages_fts_trigram").Scan(&n)
	if n != 1 {
		t.Errorf("substring index holds %d entries, want 1", n)
	}
}

// TestTrigramBatchIsAtomic pins that a batch's rows and its resume point land
// together or not at all. If the rows could commit without the watermark, a
// kill between the two would leave the index holding work no watermark claims —
// and the next run, seeing no watermark, would throw that work away.
//
// The failure is injected at the stamp: with meta renamed out from under it the
// watermark write fails, and the rows written moments earlier must roll back
// with it.
func TestTrigramBatchIsAtomic(t *testing.T) {
	con, _ := openTestDB(t)
	if _, err := con.Exec(
		`INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES
		   ('ledger','user','the reconciliation finished cleanly',100,'2026-01-01T10:00:00Z','uuid-1'),
		   ('ledger','user','the settlement finished cleanly',200,'2026-01-02T10:00:00Z','uuid-2');`); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("ALTER TABLE meta RENAME TO meta_hidden"); err != nil {
		t.Fatal(err)
	}

	if err := migrateTrigramIndex(con); err == nil {
		t.Fatal("migrateTrigramIndex must fail when it cannot record its resume point")
	}

	if _, err := con.Exec("ALTER TABLE meta_hidden RENAME TO meta"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages_fts_trigram").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("substring index holds %d rows the watermark cannot account for, want 0 — the batch did not roll back", n)
	}
}

// TestTrigramSurvivesRebuildByAnOlderBinary pins the cross-version hazard the
// done marker creates. A binary that predates the substring index rebuilds a db
// by dropping messages — taking the trigram triggers with it — but leaves the
// trigram table and the done marker behind, because its drop list never named
// them. The next run by a current binary must not believe that marker: it is
// vouching for an index full of rows whose messages are gone, and re-created
// triggers would collide with those rows on a reused id.
func TestTrigramSurvivesRebuildByAnOlderBinary(t *testing.T) {
	con, _ := openTestDB(t)
	if _, err := con.Exec(
		`INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES
		   ('ledger','user','the reconciliation finished cleanly',100,'2026-01-01T10:00:00Z','uuid-1');`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	// What an older binary's Rebuild leaves behind: messages and its triggers
	// gone, the trigram table and its marker untouched.
	if _, err := con.Exec(`
		DROP TRIGGER messages_tri_ai;
		DROP TRIGGER messages_tri_ad;
		DROP TRIGGER messages_tri_au;
		DELETE FROM messages;`); err != nil {
		t.Fatal(err)
	}
	if n := trigramRowCount(t, con); n != 1 {
		t.Fatalf("setup: orphaned index holds %d rows, want the 1 stale row", n)
	}

	// A current binary takes over and re-indexes, reusing id 1.
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema after an older binary's rebuild: %v", err)
	}
	if _, err := con.Exec(
		`INSERT INTO messages(id,session_id,role,content,ts,ts_iso,uuid) VALUES
		   (1,'ledger','user','the settlement finished cleanly',200,'2026-01-02T10:00:00Z','uuid-2');`); err != nil {
		t.Fatalf("re-indexing a reused id must not collide with a stale index entry: %v", err)
	}

	if n := trigramProbeCount(t, con, `"nciliat"`); n != 0 {
		t.Errorf("stale entry for the dropped message is still indexed (%d rows), want 0", n)
	}
	if n := trigramProbeCount(t, con, `"tlement"`); n != 1 {
		t.Errorf("re-indexed message = %d rows in the substring index, want 1", n)
	}
}

// trigramRowCount counts every entry in the substring index, stale ones
// included — a plain COUNT(*) rather than a probe.
func trigramRowCount(t *testing.T, con *sql.DB) int {
	t.Helper()
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages_fts_trigram").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// trigramProbeCount counts the entries the substring index holds for a probe,
// reading the index directly so an entry left behind for an already-deleted
// message is visible — a join back to messages would hide exactly that.
func trigramProbeCount(t *testing.T, con *sql.DB, probe string) int {
	t.Helper()
	var n int
	if err := con.QueryRow(
		"SELECT COUNT(*) FROM messages_fts_trigram WHERE messages_fts_trigram MATCH ?", probe).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
