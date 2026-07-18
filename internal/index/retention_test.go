package index

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestRetentionPurgeSurvives is Test-plan #1: a session whose backing file is
// removed from the walk (no tombstone) is RETAINED — still counted, its messages
// and FTS content intact (so read/search keep working), and flagged missing_since.
func TestRetentionPurgeSurvives(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f, `{"type":"user","uuid":"uuid-aaa-1111","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"durableretentionbeacon token"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	// Remove the backing transcript — the source tool "purged" it.
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2 (after purge): %v", err)
	}

	// Session row survives.
	var n int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='s'").Scan(&n)
	if n != 1 {
		t.Fatalf("session 's' pruned after its file was purged (count=%d), want 1 — durable retention broken", n)
	}
	// Content survives in the messages table (read resolves from here, not the JSONL).
	var content string
	if err := con.QueryRow("SELECT content FROM messages WHERE session_id='s'").Scan(&content); err != nil {
		t.Fatalf("messages gone after purge: %v", err)
	}
	// FTS still matches — search keeps returning it.
	var hits int
	con.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'durableretentionbeacon'").Scan(&hits)
	if hits != 1 {
		t.Errorf("FTS lost the retained session (%d hits), want 1", hits)
	}
	// And it is flagged missing so it doesn't read as current.
	var missing sql.NullFloat64
	con.QueryRow("SELECT missing_since FROM sessions WHERE id='s'").Scan(&missing)
	if !missing.Valid || missing.Float64 <= 0 {
		t.Errorf("retained session not flagged missing_since, got %+v", missing)
	}

	// A later reindex, still with the file gone, is idempotent (stays retained,
	// missing_since not churned to a new value).
	before := missing.Float64
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 3: %v", err)
	}
	var after sql.NullFloat64
	con.QueryRow("SELECT missing_since FROM sessions WHERE id='s'").Scan(&after)
	if !after.Valid || after.Float64 != before {
		t.Errorf("missing_since changed on a repeat pass: %v -> %v (want stable)", before, after)
	}
}

// TestRetentionTombstonePrunes is Test-plan #2: an explicit user delete (file
// gone + tombstone) really prunes the row, and a subsequent reindex does not
// resurrect it.
func TestRetentionTombstonePrunes(t *testing.T) {
	// Isolate HOME so lifecycle.LoadTombstones("") reads our seeded sidecar.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := t.TempDir()
	f := filepath.Join(proj, "dead.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"soon to be deleted"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	var n int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='dead'").Scan(&n)
	if n != 1 {
		t.Fatalf("session 'dead' not indexed (count=%d)", n)
	}

	// Simulate `rawclaw delete`: remove the file AND write the tombstone sidecar.
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	tombDir := filepath.Join(tmpHome, ".cache", "session-search")
	if err := os.MkdirAll(tombDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tombDir, ".deleted"), []byte("dead\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex after delete: %v", err)
	}
	// Row, messages, and watermark all gone.
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='dead'").Scan(&n)
	if n != 0 {
		t.Errorf("tombstoned session should be pruned, still present (count=%d)", n)
	}
	con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='dead'").Scan(&n)
	if n != 0 {
		t.Errorf("tombstoned session left %d messages, want 0", n)
	}
	con.QueryRow("SELECT COUNT(*) FROM file_index WHERE session_id='dead'").Scan(&n)
	if n != 0 {
		t.Errorf("tombstoned session left a file_index row, want 0")
	}

	// Reindex must NOT resurrect it (tombstone persists).
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex resurrection pass: %v", err)
	}
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='dead'").Scan(&n)
	if n != 0 {
		t.Errorf("tombstoned session resurrected on reindex (count=%d), want 0", n)
	}
}

// TestRetentionForeignOriginSurvives is Test-plan #3: a row stamped with another
// machine's origin_machine is never a prune candidate for this machine's scan —
// it survives an absent-file scan and is NOT even flagged missing (it is out of
// this scan's scope, not "missing" here).
func TestRetentionForeignOriginSurvives(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "f.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"another machines session"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	// Re-stamp the row as sourced by a different machine (as a shared/central store
	// would hold a peer's session).
	if _, err := con.Exec("UPDATE sessions SET origin_machine='peer-machine-xyz' WHERE id='f'"); err != nil {
		t.Fatal(err)
	}

	// Its file vanishes from THIS machine's walk.
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}

	var n int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='f'").Scan(&n)
	if n != 1 {
		t.Fatalf("foreign-origin session pruned by this machine's scan (count=%d), want 1", n)
	}
	// Not flagged missing: it is not part of this machine's live tree at all.
	var missing sql.NullFloat64
	con.QueryRow("SELECT missing_since FROM sessions WHERE id='f'").Scan(&missing)
	if missing.Valid {
		t.Errorf("foreign-origin row wrongly flagged missing_since=%v, want NULL (out of scope)", missing.Float64)
	}
}

// TestRetentionUnchangedFastPath is Test-plan #4: a genuinely-unchanged file is
// skipped, not reindexed. A reindex DELETEs+re-INSERTs the session's messages,
// reassigning AUTOINCREMENT rowids; an unchanged file must keep the SAME rowid.
func TestRetentionUnchangedFastPath(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "c.jsonl")
	writeJSONL(t, f, `{"type":"user","uuid":"uuid-keep-0001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"unchanged content"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	var id1 int
	if err := con.QueryRow("SELECT id FROM messages WHERE session_id='c'").Scan(&id1); err != nil {
		t.Fatalf("read rowid: %v", err)
	}

	// Second pass, file untouched → the mtime/size/fingerprint fast-path must skip.
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}
	var id2 int
	if err := con.QueryRow("SELECT id FROM messages WHERE session_id='c'").Scan(&id2); err != nil {
		t.Fatalf("read rowid after pass 2: %v", err)
	}
	if id1 != id2 {
		t.Errorf("unchanged file was reindexed (rowid %d -> %d) — fast-path regressed", id1, id2)
	}
}

// TestMigrationInPlaceAddsColumnsNoRebuild is Test-plan #5: upgrading a
// pre-existing, populated db (current schema_version, but WITHOUT the durability
// columns) adds the columns and backfills them via in-place ALTER TABLE — NOT a
// full rebuild. Proof it wasn't rebuilt: the existing rows are preserved (a
// rebuild drops the sessions table to zero rows).
func TestMigrationInPlaceAddsColumnsNoRebuild(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the machine-id file

	dbp := filepath.Join(t.TempDir(), "old.db")
	con, err := openRW(dbp)
	if err != nil {
		t.Fatalf("openRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })

	// Seed a pre-durability db: sessions WITHOUT the four new columns, messages in
	// the current (uuid-bearing) shape, file_index watermarks, and the CURRENT
	// schema_version so EnsureSchema takes the in-place path, not a rebuild.
	if _, err := con.Exec(`
		CREATE TABLE sessions (
		    id TEXT PRIMARY KEY, started_at REAL, last_ts REAL,
		    message_count INTEGER DEFAULT 0, is_subagent INTEGER DEFAULT 0, parent_id TEXT);
		CREATE TABLE messages (
		    id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
		    role TEXT, content TEXT, ts REAL, ts_iso TEXT, uuid TEXT);
		CREATE TABLE file_index (path TEXT PRIMARY KEY, mtime REAL, size INTEGER, fp TEXT, session_id TEXT);
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
		INSERT INTO meta(key,value) VALUES('schema_version','4');
		INSERT INTO sessions(id,message_count) VALUES('s1',1),('s2',1),('s3',1);
		INSERT INTO file_index(path,mtime,size,fp,session_id) VALUES
		    ('/live/s1.jsonl',1,1,'fp1','s1'),
		    ('/live/s2.jsonl',1,1,'fp2','s2'),
		    ('/live/s3.jsonl',1,1,'fp3','s3');`); err != nil {
		t.Fatalf("seed pre-durability db: %v", err)
	}

	if err := EnsureSchema(con, "codex"); err != nil {
		t.Fatalf("EnsureSchema (in-place migration): %v", err)
	}

	// No rebuild: the three pre-existing rows are still there.
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if n != 3 {
		t.Fatalf("sessions row count = %d, want 3 preserved — a rebuild wiped the rows", n)
	}
	// schema_version untouched (the whole point: no version bump).
	var v string
	con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&v)
	if v != "4" {
		t.Errorf("schema_version = %q, want unchanged 4 (no bump)", v)
	}
	// Columns exist AND are backfilled.
	rows, err := con.Query("SELECT id, origin_machine, source_tool, source_path FROM sessions ORDER BY id")
	if err != nil {
		t.Fatalf("select new columns: %v", err)
	}
	defer rows.Close()
	wantPath := map[string]string{"s1": "/live/s1.jsonl", "s2": "/live/s2.jsonl", "s3": "/live/s3.jsonl"}
	seen := 0
	for rows.Next() {
		var id string
		var origin, tool, path sql.NullString
		if err := rows.Scan(&id, &origin, &tool, &path); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if origin.String != MachineID() {
			t.Errorf("%s origin_machine = %q, want this machine %q", id, origin.String, MachineID())
		}
		if tool.String != "codex" {
			t.Errorf("%s source_tool = %q, want codex (the scope's source)", id, tool.String)
		}
		if path.String != wantPath[id] {
			t.Errorf("%s source_path = %q, want %q (from file_index)", id, path.String, wantPath[id])
		}
		seen++
	}
	if seen != 3 {
		t.Errorf("scanned %d migrated rows, want 3", seen)
	}
}

// TestMigrationInterruptedBackfillResumes is Test-plan #6 (F3): simulates a
// process killed AFTER the ALTER TABLE ADD COLUMNs committed but BEFORE the
// backfill UPDATE ran — the columns exist, but every row's provenance is still
// NULL. A naive migration guard (add columns → track "did I just add
// anything?" → backfill only if so) sees the columns already present on rerun
// and skips the backfill forever, leaving the rows blank until a full reindex.
// EnsureSchema must instead recheck row state, not its own "did I just ALTER"
// flag, and complete the backfill on this very next call.
func TestMigrationInterruptedBackfillResumes(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the machine-id file

	dbp := filepath.Join(t.TempDir(), "interrupted.db")
	con, err := openRW(dbp)
	if err != nil {
		t.Fatalf("openRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })

	// Seed a db that already has the durability columns (the ALTER TABLE step
	// completed) but with every row still NULL (the backfill step never ran) —
	// exactly the state a kill between those two steps leaves on disk.
	if _, err := con.Exec(`
		CREATE TABLE sessions (
		    id TEXT PRIMARY KEY, started_at REAL, last_ts REAL,
		    message_count INTEGER DEFAULT 0, is_subagent INTEGER DEFAULT 0, parent_id TEXT,
		    origin_machine TEXT, source_tool TEXT, source_path TEXT, missing_since REAL);
		CREATE TABLE messages (
		    id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
		    role TEXT, content TEXT, ts REAL, ts_iso TEXT, uuid TEXT);
		CREATE TABLE file_index (path TEXT PRIMARY KEY, mtime REAL, size INTEGER, fp TEXT, session_id TEXT);
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
		INSERT INTO meta(key,value) VALUES('schema_version','4');
		INSERT INTO sessions(id,message_count) VALUES('s1',1),('s2',1),('s3',1);
		INSERT INTO file_index(path,mtime,size,fp,session_id) VALUES
		    ('/live/s1.jsonl',1,1,'fp1','s1'),
		    ('/live/s2.jsonl',1,1,'fp2','s2'),
		    ('/live/s3.jsonl',1,1,'fp3','s3');`); err != nil {
		t.Fatalf("seed post-ALTER pre-backfill db: %v", err)
	}

	if err := EnsureSchema(con, "codex"); err != nil {
		t.Fatalf("EnsureSchema (resume interrupted backfill): %v", err)
	}

	// Row count preserved — no rebuild.
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if n != 3 {
		t.Fatalf("sessions row count = %d, want 3 preserved — a rebuild wiped the rows", n)
	}

	// No row is left with blank provenance.
	rows, err := con.Query("SELECT id, origin_machine, source_tool, source_path FROM sessions ORDER BY id")
	if err != nil {
		t.Fatalf("select new columns: %v", err)
	}
	defer rows.Close()
	wantPath := map[string]string{"s1": "/live/s1.jsonl", "s2": "/live/s2.jsonl", "s3": "/live/s3.jsonl"}
	seen := 0
	for rows.Next() {
		var id string
		var origin, tool, path sql.NullString
		if err := rows.Scan(&id, &origin, &tool, &path); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if !origin.Valid || origin.String != MachineID() {
			t.Errorf("%s origin_machine = %+v, want this machine %q (resumed backfill left it blank)", id, origin, MachineID())
		}
		if !tool.Valid || tool.String != "codex" {
			t.Errorf("%s source_tool = %+v, want codex", id, tool)
		}
		if !path.Valid || path.String != wantPath[id] {
			t.Errorf("%s source_path = %+v, want %q", id, path, wantPath[id])
		}
		seen++
	}
	if seen != 3 {
		t.Errorf("scanned %d rows, want 3", seen)
	}
}

// TestRetentionMirrorModePrunes: RAWCLAW_RETENTION=mirror restores the
// pre-retention prune semantics (v0.2.0 parity) — an absent own-source file
// prunes the session at the next index pass: row, messages, FTS content, and
// watermark all gone. The user setting exists because retention is the USER's
// choice (setting, default keep); mirror is the opt-out.
func TestRetentionMirrorModePrunes(t *testing.T) {
	t.Setenv("RAWCLAW_RETENTION", "mirror")

	proj := t.TempDir()
	f := filepath.Join(proj, "m.jsonl")
	writeJSONL(t, f, `{"type":"user","uuid":"uuid-mmm-0001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"mirrormodebeacon token"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2 (after purge): %v", err)
	}

	var n int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='m'").Scan(&n)
	if n != 0 {
		t.Errorf("mirror mode: session survived an absent file (count=%d), want 0 (v0.2.0 parity)", n)
	}
	con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='m'").Scan(&n)
	if n != 0 {
		t.Errorf("mirror mode left %d messages, want 0", n)
	}
	con.QueryRow("SELECT COUNT(*) FROM file_index WHERE session_id='m'").Scan(&n)
	if n != 0 {
		t.Errorf("mirror mode left a file_index row, want 0")
	}
	con.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'mirrormodebeacon'").Scan(&n)
	if n != 0 {
		t.Errorf("mirror mode left FTS content (%d hits), want 0", n)
	}
}

// TestRetentionSettingUnrecognizedKeeps: any value other than "mirror"
// (including garbage) resolves to keep — the default is durable retention, and
// a typo must never silently turn deletion on.
func TestRetentionSettingUnrecognizedKeeps(t *testing.T) {
	t.Setenv("RAWCLAW_RETENTION", "bogus-value")

	proj := t.TempDir()
	f := filepath.Join(proj, "k.jsonl")
	writeJSONL(t, f, `{"type":"user","uuid":"uuid-kkk-0001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"keepdefaultbeacon token"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}

	var n int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='k'").Scan(&n)
	if n != 1 {
		t.Fatalf("unrecognized setting value pruned the session (count=%d), want 1 (keep default)", n)
	}
	var missing sql.NullFloat64
	con.QueryRow("SELECT missing_since FROM sessions WHERE id='k'").Scan(&missing)
	if !missing.Valid {
		t.Errorf("retained session not flagged missing_since under unrecognized setting value")
	}
}

// TestRetentionMirrorForeignOriginSurvives: mirror mode governs THIS machine's
// own sources only — a foreign-origin row is still out of the scan's scope and
// must never be pruned by it, whatever the setting says.
func TestRetentionMirrorForeignOriginSurvives(t *testing.T) {
	t.Setenv("RAWCLAW_RETENTION", "mirror")

	proj := t.TempDir()
	f := filepath.Join(proj, "fm.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"peer session under mirror"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if _, err := con.Exec("UPDATE sessions SET origin_machine='peer-machine-xyz' WHERE id='fm'"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}

	var n int
	con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='fm'").Scan(&n)
	if n != 1 {
		t.Fatalf("mirror mode pruned a foreign-origin row (count=%d), want 1 (out of scope)", n)
	}
}

// dbDigest hashes the raw db file bytes — the "did discovery write?" probe.
// Any write (stamp, prune, schema touch, WAL checkpoint into the main file)
// changes the digest; a pure read leaves it identical.
func dbDigest(t *testing.T, dbp string) string {
	t.Helper()
	b, err := os.ReadFile(dbp)
	if err != nil {
		t.Fatalf("read db file: %v", err)
	}
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}

// TestOrphanReconcileIsReadMostly: discovery of an orphaned
// db is read-MOSTLY — the first pass after orphaning may write (it stamps
// missing_since), but every subsequent pass with no pending work is a pure
// read: the db file's bytes stay identical. New work (a fresh tombstone) gets
// exactly one write pass, then it goes quiet again.
func TestOrphanReconcileIsReadMostly(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := t.TempDir()
	f := filepath.Join(proj, "o.jsonl")
	writeJSONL(t, f, `{"type":"user","uuid":"uuid-ooo-0001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"orphanreadonlybeacon token"}}`)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}
	con.Close()
	// The whole source project vanishes — the 30-day-purge case (D8).
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}

	// Pass 1: reconcile stamps missing_since — a write is expected and allowed.
	n, err := EnsureOrphanReconciled(dbp)
	if err != nil {
		t.Fatalf("EnsureOrphanReconciled pass 1: %v", err)
	}
	if n != 1 {
		t.Fatalf("pass 1 surviving count = %d, want 1", n)
	}

	// Pass 2+3: nothing changed — pure reads, file bytes identical.
	before := dbDigest(t, dbp)
	for pass := 2; pass <= 3; pass++ {
		n, err = EnsureOrphanReconciled(dbp)
		if err != nil {
			t.Fatalf("EnsureOrphanReconciled pass %d: %v", pass, err)
		}
		if n != 1 {
			t.Fatalf("pass %d surviving count = %d, want 1", pass, n)
		}
		if got := dbDigest(t, dbp); got != before {
			t.Fatalf("pass %d WROTE to the orphan db (digest changed) — discovery must be a pure read when no work is pending", pass)
		}
	}

	// A new tombstone is new work: the next pass prunes (one write)…
	tombDir := filepath.Join(tmpHome, ".cache", "session-search")
	if err := os.MkdirAll(tombDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tombDir, ".deleted"), []byte("o\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err = EnsureOrphanReconciled(dbp)
	if err != nil {
		t.Fatalf("EnsureOrphanReconciled after tombstone: %v", err)
	}
	if n != 0 {
		t.Fatalf("tombstoned orphan session still counted (n=%d), want 0", n)
	}
	// …then discovery goes quiet again.
	after := dbDigest(t, dbp)
	if _, err = EnsureOrphanReconciled(dbp); err != nil {
		t.Fatalf("EnsureOrphanReconciled quiet pass: %v", err)
	}
	if got := dbDigest(t, dbp); got != after {
		t.Fatalf("post-tombstone quiet pass wrote to the orphan db, want pure read")
	}
}

// TestRetentionMirrorSparesOrphanArchive (live-verified data-loss
// footgun): a search run with RAWCLAW_RETENTION=mirror must NOT prune an
// orphaned archive's already-retained rows. Mirror governs live-scope scans;
// retained history is removed by an explicit tombstone alone (D5). The orphan
// db must survive a mirror-mode discovery pass byte-identical.
func TestRetentionMirrorSparesOrphanArchive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	f := filepath.Join(proj, "arch.jsonl")
	writeJSONL(t, f, `{"type":"user","uuid":"uuid-arc-0001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"mirrorsparebeacon token"}}`)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}
	con.Close()
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	// Retain (keep mode), then reconcile to quiescence.
	if n, err := EnsureOrphanReconciled(dbp); err != nil || n != 1 {
		t.Fatalf("initial retain: n=%d err=%v, want 1,nil", n, err)
	}
	before := dbDigest(t, dbp)

	// A discovery pass under mirror mode must leave the archive untouched.
	t.Setenv("RAWCLAW_RETENTION", "mirror")
	n, err := EnsureOrphanReconciled(dbp)
	if err != nil {
		t.Fatalf("EnsureOrphanReconciled under mirror: %v", err)
	}
	if n != 1 {
		t.Fatalf("mirror-mode discovery pruned the retained archive (n=%d), want 1 — retained rows die only by tombstone", n)
	}
	if got := dbDigest(t, dbp); got != before {
		t.Fatalf("mirror-mode discovery WROTE to the orphan archive, want byte-identical")
	}
}
