package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestEnsureIndexedContainers proves the source-agnostic ingestion path: two
// containers (a root + a subagent) index into a db, get the right is_subagent /
// parent_id tagging, and default FTS search (is_subagent=0) hides the subagent.
func TestEnsureIndexedContainers(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "codex.db")

	rootFile := filepath.Join(dir, "rollout-root.jsonl")
	subFile := filepath.Join(dir, "rollout-sub.jsonl")
	if err := os.WriteFile(rootFile, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subFile, []byte("y\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cs := []source.Container{
		{ID: "root1", Path: rootFile, CWD: "/repo", IsSubagent: false},
		{ID: "sub1", Path: subFile, CWD: "/repo", IsSubagent: true, ParentID: "root1"},
	}
	msgs := func(c source.Container) ([]model.Message, error) {
		switch c.ID {
		case "root1":
			return []model.Message{{Role: "user", Text: "deploy pipeline question", TS: 1, TSISO: "2026-07-15T00:00:00Z", UUID: "a"}}, nil
		case "sub1":
			return []model.Message{{Role: "assistant", Text: "deploy pipeline answer", TS: 2, TSISO: "2026-07-15T00:00:01Z", UUID: "b"}}, nil
		}
		return nil, nil
	}

	n, status, err := EnsureIndexedContainers(dbp, true, cs, msgs, "codex", "")
	if err != nil {
		t.Fatalf("EnsureIndexedContainers: %v", err)
	}
	if status != IndexFresh {
		t.Errorf("status = %v, want IndexFresh", status)
	}
	if n != 2 {
		t.Fatalf("sessions = %d, want 2", n)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	var isSub int
	if err := con.QueryRow("SELECT is_subagent FROM sessions WHERE id='sub1'").Scan(&isSub); err != nil {
		t.Fatal(err)
	}
	if isSub != 1 {
		t.Errorf("sub1 is_subagent = %d, want 1", isSub)
	}
	var parent sql.NullString
	if err := con.QueryRow("SELECT parent_id FROM sessions WHERE id='sub1'").Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if parent.String != "root1" {
		t.Errorf("sub1 parent_id = %q, want root1", parent.String)
	}

	// Default search (is_subagent=0) matches only the root's message.
	var cnt int
	if err := con.QueryRow(
		`SELECT COUNT(*) FROM messages_fts f
		 JOIN messages m ON m.id = f.rowid
		 JOIN sessions s ON s.id = m.session_id
		 WHERE messages_fts MATCH 'deploy' AND s.is_subagent = 0`,
	).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Errorf("default-search 'deploy' hits = %d, want 1 (root only, subagent hidden)", cnt)
	}

	// Idempotent re-run: unchanged files reindex to the same 2 sessions.
	n2, _, err := EnsureIndexedContainers(dbp, false, cs, msgs, "codex", "")
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if n2 != 2 {
		t.Errorf("re-run sessions = %d, want 2", n2)
	}
}

// TestReindexContainer_RollbackOnFailure verifies that a write failure during
// reindexing rolls back the transaction, preserving the existing complete session
// without leaving a half-indexed "franken-session".
func TestReindexContainer_RollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "atomic.db")
	f := filepath.Join(dir, "sess.jsonl")
	if err := os.WriteFile(f, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "sess-atomic", Path: f, CWD: "/atomic"}
	initialMsgs := []model.Message{
		{Role: "user", Text: "original message 1", TS: 1, TSISO: "2026-08-15T00:00:00Z", UUID: "u1"},
		{Role: "assistant", Text: "original message 2", TS: 2, TSISO: "2026-08-15T00:00:01Z", UUID: "u2"},
	}

	_, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, func(_ source.Container) ([]model.Message, error) {
		return initialMsgs, nil
	}, "test", "")
	if err != nil {
		t.Fatalf("initial index failed: %v", err)
	}

	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	// Install a trigger that fails on role='fail' to simulate mid-write failure
	if _, err := con.Exec("CREATE TRIGGER test_abort BEFORE INSERT ON messages WHEN new.role = 'fail' BEGIN SELECT RAISE(ABORT, 'injected write failure'); END;"); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	brokenMsgs := []model.Message{
		{Role: "user", Text: "replacement message 1", TS: 3, TSISO: "2026-08-15T00:00:02Z", UUID: "u-ok"},
		{Role: "fail", Text: "replacement message 2", TS: 4, TSISO: "2026-08-15T00:00:03Z", UUID: "u-bad"}, // triggers RAISE(ABORT)
	}

	err = reindexContainer(con, c, brokenMsgs, "test", "", f, 100.0, 10, "testfp")
	if err == nil {
		t.Fatal("reindexContainer should have returned error on injected failure")
	}

	// Verify the original session and its messages are 100% intact
	var count int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='sess-atomic'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("after rollback message count = %d, want 2 (original messages preserved)", count)
	}

	var firstMsg string
	if err := con.QueryRow("SELECT content FROM messages WHERE session_id='sess-atomic' AND uuid='u1'").Scan(&firstMsg); err != nil {
		t.Fatalf("query original message: %v", err)
	}
	if firstMsg != "original message 1" {
		t.Errorf("content = %q, want %q", firstMsg, "original message 1")
	}
}

// TestEnsureIndexedContainers_SQLiteWALTrigger verifies that writes committed to a -wal file
// trigger an incremental re-index even when the main .db file mtime and size are unchanged.
func TestEnsureIndexedContainers_SQLiteWALTrigger(t *testing.T) {
	td := t.TempDir()
	mainDB := filepath.Join(td, "sessions.db")
	walFile := mainDB + "-wal"
	cacheDB := filepath.Join(td, "cache.db")

	if err := os.WriteFile(mainDB, []byte("main db data"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{
		ID:   "sess-wal-1",
		Path: mainDB + "#sess-wal-1",
		CWD:  "/workspace/wal",
	}

	msgCallCount := 0
	msgsFn := func(got source.Container) ([]model.Message, error) {
		msgCallCount++
		return []model.Message{
			{Role: "user", Text: fmt.Sprintf("message count = %d", msgCallCount), TS: float64(msgCallCount), TSISO: "2026-08-22T00:00:00Z", UUID: "u1"},
		}, nil
	}

	// 1. Initial index
	_, _, err := EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
	if err != nil {
		t.Fatalf("initial index failed: %v", err)
	}
	if msgCallCount != 1 {
		t.Fatalf("expected msgsFn called once, got %d", msgCallCount)
	}

	// 2. Second index pass with NO changes -> must skip (unchanged)
	_, _, err = EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
	if err != nil {
		t.Fatalf("second index failed: %v", err)
	}
	if msgCallCount != 1 {
		t.Fatalf("expected no reindex on unchanged file, got call count %d", msgCallCount)
	}

	// 3. Write to WAL file while keeping mainDB untouched
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(walFile, []byte("new transaction committed to wal"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Third index pass -> MUST trigger reindex due to WAL change
	_, _, err = EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
	if err != nil {
		t.Fatalf("third index failed: %v", err)
	}
	if msgCallCount != 2 {
		t.Fatalf("expected reindex triggered by WAL update, got call count %d", msgCallCount)
	}
}

// TestEnsureIndexedContainers_StaleWatermarkDroppedOnRename proves that when a container's
// backing file path changes for the same session ID, the old file_index watermark is
// purged so retention does not treat the old path as a purged session and flag/prune it.
func TestEnsureIndexedContainers_StaleWatermarkDroppedOnRename(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "antigravity.db")

	f1 := filepath.Join(dir, "transcript.jsonl")
	f2 := filepath.Join(dir, "transcript_full.jsonl")
	if err := os.WriteFile(f1, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c1 := source.Container{ID: "sess-rename", Path: f1, CWD: "/repo"}
	msgs1 := func(source.Container) ([]model.Message, error) {
		return []model.Message{{Role: "user", Text: "rotate secret keys", TS: 1, TSISO: "2026-08-15T00:00:00Z", UUID: "u1"}}, nil
	}

	// Pass 1: indexed under f1 (transcript.jsonl)
	n, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c1}, msgs1, "antigravity", "")
	if err != nil {
		t.Fatalf("initial EnsureIndexedContainers: %v", err)
	}
	if n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}

	// Now f1 is removed, and f2 (transcript_full.jsonl) is created
	if err := os.Remove(f1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("x\ny\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c2 := source.Container{ID: "sess-rename", Path: f2, CWD: "/repo"}
	msgs2 := func(source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "rotate secret keys", TS: 1, TSISO: "2026-08-15T00:00:00Z", UUID: "u1"},
			{Role: "user", Text: "validate secret keys", TS: 2, TSISO: "2026-08-15T00:00:01Z", UUID: "u2"},
		}, nil
	}

	// Pass 2: indexed under f2 (transcript_full.jsonl).
	//
	// reindex MUST be false here. ensureIndexedContainers os.Remove()s the whole db
	// when reindex is true, which wipes the f1 watermark before this pass can run —
	// the stale row would never exist, and this test would pass with or without the
	// fix it exists to pin. Carrying the db forward is the entire scenario.
	n2, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c2}, msgs2, "antigravity", "")
	if err != nil {
		t.Fatalf("second EnsureIndexedContainers: %v", err)
	}
	if n2 != 1 {
		t.Fatalf("sessions after rename = %d, want 1", n2)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	// file_index must have exactly 1 row pointing to f2 (the stale row for f1 is gone)
	var fiCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM file_index WHERE session_id='sess-rename'").Scan(&fiCount); err != nil {
		t.Fatal(err)
	}
	if fiCount != 1 {
		t.Errorf("file_index row count for sess-rename = %d, want 1", fiCount)
	}

	var fiPath string
	if err := con.QueryRow("SELECT path FROM file_index WHERE session_id='sess-rename'").Scan(&fiPath); err != nil {
		t.Fatal(err)
	}
	if fiPath != realpath(f2) {
		t.Errorf("file_index path = %q, want %q", fiPath, realpath(f2))
	}

	// missing_since must NOT be stamped
	var missing sql.NullFloat64
	if err := con.QueryRow("SELECT missing_since FROM sessions WHERE id='sess-rename'").Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing.Valid {
		t.Errorf("missing_since = %v, want NULL (session is live, not missing)", missing.Float64)
	}
}

// TestEnsureIndexedContainers_WriteLockContention proves that when a concurrent
// connection holds a write lock on the index db, EnsureIndexedContainers fails
// to reindex the container, logs the failure, and returns IndexStale rather than
// falsely claiming the result is IndexFresh.
func TestEnsureIndexedContainers_WriteLockContention(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "contention.db")
	f := filepath.Join(dir, "sess.jsonl")
	if err := os.WriteFile(f, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "sess-lock", Path: f, CWD: "/repo"}
	msgs := func(_ source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "initial message", TS: 1, TSISO: "2026-08-25T00:00:00Z", UUID: "u1"},
		}, nil
	}

	// 1. Initial index to set up schema and initial row
	n, status, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgs, "codex", "")
	if err != nil {
		t.Fatalf("initial indexing failed: %v", err)
	}
	if status != IndexFresh {
		t.Fatalf("initial status = %v, want IndexFresh", status)
	}
	if n != 1 {
		t.Fatalf("initial sessions = %d, want 1", n)
	}

	// 2. Open a second connection and hold an exclusive write lock
	con2, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}
	defer con2.Close()

	tx2, err := con2.Begin()
	if err != nil {
		t.Fatalf("begin con2 tx: %v", err)
	}
	defer tx2.Rollback()

	if _, err := tx2.Exec("INSERT INTO meta(key, value) VALUES('lock_probe', 'held')"); err != nil {
		t.Fatalf("hold write lock: %v", err)
	}

	// 3. Update container file so reindex is triggered
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(f, []byte("x-updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Run EnsureIndexedContainers: the write lock held by con2 causes reindexContainer
	// to fail with SQLITE_BUSY / database locked. EnsureIndexedContainers must report
	// status as not fresh (IndexStale).
	_, status2, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgs, "codex", "")
	if err != nil {
		t.Fatalf("EnsureIndexedContainers returned unexpected fatal error: %v", err)
	}
	if status2 == IndexFresh {
		t.Errorf("status = %v, want not fresh (IndexStale) when write lock is held", status2)
	}
	if status2 != IndexStale {
		t.Errorf("status = %v, want IndexStale", status2)
	}
}
