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
