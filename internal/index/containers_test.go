package index

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	err = reindexContainer(con, reindexContainerParams{
		container:   c,
		messages:    brokenMsgs,
		sourceID:    "test",
		path:        f,
		mtime:       100.0,
		size:        10,
		fingerprint: "testfp",
	})
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

	// only_copy_since must NOT be stamped
	var onlyCopy sql.NullFloat64
	if err := con.QueryRow("SELECT only_copy_since FROM sessions WHERE id='sess-rename'").Scan(&onlyCopy); err != nil {
		t.Fatal(err)
	}
	if onlyCopy.Valid {
		t.Errorf("only_copy_since = %v, want NULL (session is live, not only copy)", onlyCopy.Float64)
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

type testLogRecorder struct {
	records []slog.Record
	mu      sync.Mutex
}

func (r *testLogRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *testLogRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec)
	return nil
}
func (r *testLogRecorder) WithAttrs(_ []slog.Attr) slog.Handler { return r }
func (r *testLogRecorder) WithGroup(_ string) slog.Handler      { return r }

func (r *testLogRecorder) Warns() []slog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []slog.Record
	for _, rec := range r.records {
		if rec.Level >= slog.LevelWarn {
			out = append(out, rec)
		}
	}
	return out
}

// TestEnsureIndexedContainers_ReindexFailure_WrapsContextWithoutLogging proves that
// when an internal reindex statement fails (non-busy error), EnsureIndexedContainers
// propagates the error with session context and does NOT log locally, following Go's
// single-handling rule.
func TestEnsureIndexedContainers_ReindexFailure_WrapsContextWithoutLogging(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "fail.db")
	f := filepath.Join(dir, "sess.jsonl")
	if err := os.WriteFile(f, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "sess-err-ctx", Path: f, CWD: "/repo"}
	msgs := func(_ source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "initial message", TS: 1, TSISO: "2026-08-25T00:00:00Z", UUID: "u1"},
		}, nil
	}

	// 1. Initial index
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgs, "codex", ""); err != nil {
		t.Fatalf("initial indexing failed: %v", err)
	}

	// 2. Install a trigger on messages to force a non-busy failure during reindex
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := con.Exec("CREATE TRIGGER fail_messages BEFORE INSERT ON messages BEGIN SELECT RAISE(FAIL, 'forced write failure'); END;"); err != nil {
		con.Close()
		t.Fatalf("create trigger: %v", err)
	}
	con.Close()

	// 3. Touch backing file to force reindexContainer to run
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(f, []byte("x-mod\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Capture slog records
	recorder := &testLogRecorder{}
	origLogger := slog.Default()
	slog.SetDefault(slog.New(recorder))
	defer slog.SetDefault(origLogger)

	// 5. Run EnsureIndexedContainers: must fail with non-busy error containing session ID
	_, _, err = EnsureIndexedContainers(dbp, false, []source.Container{c}, msgs, "codex", "")
	if err == nil {
		t.Fatal("EnsureIndexedContainers succeeded, want error when messages table is missing")
	}
	if !strings.Contains(err.Error(), "sess-err-ctx") {
		t.Errorf("error %q does not contain session ID %q", err.Error(), "sess-err-ctx")
	}

	// 6. Ensure no slog warnings were emitted inside reindexContainer/EnsureIndexedContainers
	if got := len(recorder.Warns()); got != 0 {
		t.Errorf("got %d slog.Warn calls, want 0 (single handling rule: errors are returned, not logged in index package)", got)
	}
}

// TestEnsureFreshContainer_PreservesRefreshDBOnPublishFailure proves that a
// failed consolidated publish does not discard the successfully refreshed
// per-container database needed for retry/recovery.
func TestEnsureFreshContainer_PreservesRefreshDBOnPublishFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	path := filepath.Join(home, "session.jsonl")
	if err := os.WriteFile(path, []byte("message\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := source.Container{ID: "sess-seed", Path: path, CWD: "/work"}
	msgs := func(source.Container) ([]model.Message, error) {
		return []model.Message{{Role: "user", Text: "retry me", TS: 1, TSISO: "2026-08-25T00:00:00Z", UUID: "u1"}}, nil
	}
	seedDB := RefreshDBPath("claude", c.ID, c.Path)
	if _, err := EnsureFreshContainer(seedDB, c, msgs, "claude"); err != nil {
		t.Fatalf("seed EnsureFreshContainer: %v", err)
	}

	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("CREATE TRIGGER fail_publish BEFORE INSERT ON messages BEGIN SELECT RAISE(ABORT, 'injected publish failure'); END;"); err != nil {
		con.Close()
		t.Fatal(err)
	}
	if err := con.Close(); err != nil {
		t.Fatal(err)
	}

	failurePath := filepath.Join(home, "failed-session.jsonl")
	if err := os.WriteFile(failurePath, []byte("message\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c = source.Container{ID: "sess-publish-failure", Path: failurePath, CWD: "/work"}
	dbp := RefreshDBPath("claude", c.ID, c.Path)
	if _, err := EnsureFreshContainer(dbp, c, msgs, "claude"); err == nil {
		t.Fatal("EnsureFreshContainer succeeded, want consolidated publish failure")
	}

	refresh, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open refresh db after publish failure: %v", err)
	}
	defer refresh.Close()
	var count int
	if err := refresh.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("refresh db message count = %d, want 1 after failed publish", count)
	}
}

func TestRefreshDBPath_PrunesStaleCacheButRetainsFreshAndReused(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	refreshDir := filepath.Join(store.CacheDir(), "refresh")
	if err := os.MkdirAll(refreshDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(refreshDir, "stale.db")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale+"-wal", []byte("wal"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-(refreshCacheStaleAfter + time.Hour))
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stale+"-wal", old, old); err != nil {
		t.Fatal(err)
	}

	fresh := RefreshDBPath("claude", "fresh", "/fresh.jsonl")
	if err := os.WriteFile(fresh, []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}
	RefreshDBPath("claude", "fresh", "/fresh.jsonl")
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh refresh db was pruned: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale refresh db still exists, err=%v", err)
	}
	if _, err := os.Stat(stale + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("stale refresh WAL still exists, err=%v", err)
	}

	reused := RefreshDBPath("claude", "reused", "/reused.jsonl")
	if err := os.WriteFile(reused, []byte("reused"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(reused, old, old); err != nil {
		t.Fatal(err)
	}
	if got := RefreshDBPath("claude", "reused", "/reused.jsonl"); got != reused {
		t.Fatalf("reused refresh db path = %q, want %q", got, reused)
	}
	if _, err := os.Stat(reused); err != nil {
		t.Fatalf("reused refresh db was pruned: %v", err)
	}
}

func TestPrepareFreshContainer_ProvesFreshnessWithoutConsolidatedSync(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)

	f := filepath.Join(cfg, "sess1.jsonl")
	if err := os.WriteFile(f, []byte("msg1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := source.Container{ID: "sess-prepare", Path: f, CWD: "/work"}
	msgs := func(_ source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "prepare msg", TS: 1, TSISO: "2026-08-25T00:00:00Z", UUID: "u1"},
		}, nil
	}
	dbp := RefreshDBPath("claude", c.ID, c.Path)
	stalePath := filepath.Join(store.CacheDir(), "refresh", "unrelated-stale.db")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stalePath, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	n, err := PrepareFreshContainer(dbp, c, msgs, "claude")
	if err != nil {
		t.Fatalf("PrepareFreshContainer: %v", err)
	}
	if n != 1 {
		t.Errorf("PrepareFreshContainer n = %d, want 1", n)
	}
	if _, err := os.Stat(stalePath); err != nil {
		t.Fatalf("PrepareFreshContainer pruned unrelated stale refresh db: %v", err)
	}

	// Refresh DB exists and has the message
	conRefresh, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("ConnectRO refresh db: %v", err)
	}
	defer conRefresh.Close()
	var count int
	if err := conRefresh.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("refresh db message count = %d, want 1 (err: %v)", count, err)
	}

	// Consolidated store must NOT exist or have this session yet
	if _, statErr := os.Stat(ConsolidatedPath()); statErr == nil {
		conConsolidated, err := store.ConnectRO(ConsolidatedPath())
		if err == nil {
			defer conConsolidated.Close()
			var consCount int
			if err := conConsolidated.QueryRow("SELECT COUNT(*) FROM sessions WHERE id=?", c.ID).Scan(&consCount); err == nil && consCount != 0 {
				t.Errorf("session already in consolidated store after PrepareFreshContainer: %d", consCount)
			}
		}
	}
}

// TestEnsureIndexedContainers_StampsFreshnessWatermark pins the release-
// blocking gap found by the final review: the freshness fix (#3) stamped
// MetaLastIngestTime/MetaLastIngestCatalogMTime in ensureIndexedTree (the
// Claude-only path) but not here — so Codex, Antigravity, and Goose
// per-project dbs, which all index through EnsureIndexedContainers, never
// got the watermark and stayed permanently "freshness unknown" instead of
// ever reporting fresh.
func TestEnsureIndexedContainers_StampsFreshnessWatermark(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	catDir := filepath.Join(home, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatalf("create catalog dir: %v", err)
	}

	dir := t.TempDir()
	dbp := filepath.Join(dir, "codex.db")

	f := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(f, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cs := []source.Container{{ID: "sess1", Path: f, CWD: "/repo"}}
	msgs := func(source.Container) ([]model.Message, error) {
		return []model.Message{{Role: "user", Text: "hi", TS: 1, TSISO: "2026-07-15T00:00:00Z", UUID: "u"}}, nil
	}

	if _, status, err := EnsureIndexedContainers(dbp, false, cs, msgs, "codex", ""); err != nil || status != IndexFresh {
		t.Fatalf("EnsureIndexedContainers: status=%v err=%v", status, err)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer con.Close()
	freshness, err := CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if !freshness.Fresh {
		t.Fatalf("freshness = %+v, want Fresh — the container path never stamped the watermark", freshness)
	}
}
