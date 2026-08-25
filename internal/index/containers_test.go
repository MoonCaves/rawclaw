package index

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
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

	ok := reindexContainer(con, c, brokenMsgs, "test", "", f, 100.0, 10, "testfp")
	if ok {
		t.Fatal("reindexContainer should have returned false on injected failure")
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

// TestEnsureIndexedContainers_SQLiteWALTrigger verifies that writes committed to a real SQLite WAL
// trigger incremental re-indexing, verifies the WAL magic header and nonzero size, covers
// checkpointing away the WAL mid-run, and verifies correct behavior when a -shm file exists alongside.
func TestEnsureIndexedContainers_SQLiteWALTrigger(t *testing.T) {
	td := t.TempDir()
	mainDB := filepath.Join(td, "sessions.db")
	walFile := mainDB + "-wal"
	shmFile := mainDB + "-shm"
	cacheDB := filepath.Join(td, "cache.db")

	// Open a genuine SQLite database and set journal_mode=WAL.
	db, err := sql.Open("sqlite", "file:"+mainDB+"?_pragma=busy_timeout(10000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatalf("set journal_mode=WAL: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE sessions (id TEXT PRIMARY KEY, content TEXT);"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO sessions (id, content) VALUES ('sess-wal-1', 'initial transcript data');"); err != nil {
		t.Fatalf("insert initial session: %v", err)
	}

	// Verify the -wal file is real: exists, nonzero size, and valid WAL magic header.
	assertRealWAL(t, walFile)

	// Verify -shm file exists alongside in WAL mode.
	if _, err := os.Stat(shmFile); err != nil {
		t.Fatalf("expected -shm file to exist alongside WAL: %v", err)
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

	// 1. Initial index pass
	_, _, err = EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
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

	// 3. Perform a read query to ensure reading/touching -shm does not trigger false reindex.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		t.Fatalf("read query: %v", err)
	}
	_, _, err = EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
	if err != nil {
		t.Fatalf("read query pass failed: %v", err)
	}
	if msgCallCount != 1 {
		t.Fatalf("expected read query / -shm interaction to not trigger reindex, got call count %d", msgCallCount)
	}

	// 4. Commit a new transaction into the real SQLite DB in WAL mode.
	time.Sleep(10 * time.Millisecond)
	if _, err := db.Exec("INSERT INTO sessions (id, content) VALUES ('sess-wal-2', 'second transaction committed to real wal');"); err != nil {
		t.Fatalf("insert second session: %v", err)
	}

	// Verify WAL file is still real with valid magic and nonzero size.
	assertRealWAL(t, walFile)

	// 5. Third index pass -> MUST trigger reindex due to real WAL update
	_, _, err = EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
	if err != nil {
		t.Fatalf("third index failed: %v", err)
	}
	if msgCallCount != 2 {
		t.Fatalf("expected reindex triggered by WAL update, got call count %d", msgCallCount)
	}

	// 6. Fourth index pass with NO changes -> must skip
	_, _, err = EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
	if err != nil {
		t.Fatalf("fourth index failed: %v", err)
	}
	if msgCallCount != 2 {
		t.Fatalf("expected no reindex on unchanged WAL, got call count %d", msgCallCount)
	}

	// 7. Checkpoint the WAL into the main database with TRUNCATE.
	// This flushes all WAL pages into mainDB and truncates -wal to 0 bytes.
	time.Sleep(10 * time.Millisecond)
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		t.Fatalf("wal_checkpoint(TRUNCATE) failed: %v", err)
	}

	// Verify -wal file is now truncated (size 0) or removed.
	if st, err := os.Stat(walFile); err == nil && st.Size() > 0 {
		t.Fatalf("expected -wal file to be truncated to 0 bytes after TRUNCATE checkpoint, got size %d", st.Size())
	}

	// 8. Fifth index pass -> MUST reindex because the backing file state transitioned
	// from (mainDB + wal) to (mainDB checkpointed, wal truncated), updating the watermark.
	_, _, err = EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
	if err != nil {
		t.Fatalf("fifth index failed: %v", err)
	}
	if msgCallCount != 3 {
		t.Fatalf("expected reindex triggered by checkpointed state transition, got call count %d", msgCallCount)
	}

	// 9. Sixth index pass with NO changes after checkpoint -> must skip
	_, _, err = EnsureIndexedContainers(cacheDB, false, []source.Container{c}, msgsFn, "goose", "")
	if err != nil {
		t.Fatalf("sixth index failed: %v", err)
	}
	if msgCallCount != 3 {
		t.Fatalf("expected no reindex on unchanged checkpointed state, got call count %d", msgCallCount)
	}
}

func assertRealWAL(t *testing.T, walPath string) {
	t.Helper()
	st, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("expected WAL file %s to exist: %v", walPath, err)
	}
	if st.Size() < 32 {
		t.Fatalf("expected WAL file %s size >= 32 bytes (WAL header), got %d", walPath, st.Size())
	}
	f, err := os.Open(walPath)
	if err != nil {
		t.Fatalf("open WAL file %s: %v", walPath, err)
	}
	defer f.Close()

	header := make([]byte, 4)
	if _, err := io.ReadFull(f, header); err != nil {
		t.Fatalf("read WAL header magic from %s: %v", walPath, err)
	}
	magic := binary.BigEndian.Uint32(header)
	if magic != 0x377f0682 && magic != 0x377f0683 {
		t.Fatalf("expected valid WAL magic header 0x377f0682 or 0x377f0683, got 0x%08x", magic)
	}
}
