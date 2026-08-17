package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

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

	ok := reindexContainer(con, c, brokenMsgs, "test", "", f, 100.0, 10)
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
