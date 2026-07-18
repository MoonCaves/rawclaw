package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if got := CountSessions(dbp); got != 1 {
		t.Errorf("CountSessions = %d, want 1", got)
	}

	// CorpusStats reflects the single user message.
	cs, err := GetCorpusStats(dbp)
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
	if got := CountSessions(filepath.Join(t.TempDir(), "nope.db")); got != -1 {
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
	cs, err := GetCorpusStats(dbp)
	if err != nil {
		t.Fatalf("GetCorpusStats should not error on a bad db, got %v", err)
	}
	if cs != (CorpusStats{}) {
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
