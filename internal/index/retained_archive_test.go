package index

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// newRetainedDB creates an index db at cacheDir/name holding one retained
// (missing_since set) top-level session with the given id.
func newRetainedDB(t *testing.T, cacheDir, name, sessionID string) {
	t.Helper()
	dbp := filepath.Join(cacheDir, name)
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("store.ConnectRW: %v", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := con.Exec(
		`INSERT INTO sessions (id, source_path, message_count, is_subagent, missing_since)
		   VALUES (?, ?, 1, 0, ?)`,
		sessionID, "/gone/proj/"+sessionID+".jsonl", float64(time.Now().Unix())); err != nil {
		t.Fatalf("insert retained row: %v", err)
	}
}

// TestRetainedMatches_SkipsArchiveReplicaDBs: the retained scan must never
// surface rows from "archive-" namespaced dbs — those are FOREIGN machines'
// read-only replicas, and a row there entering the delete/tombstone path
// would be a cross-machine delete (out of scope v1, read-only contract).
func TestRetainedMatches_SkipsArchiveReplicaDBs(t *testing.T) {
	cacheDir := t.TempDir()
	newRetainedDB(t, cacheDir, "local-proj.db", "sess-local")
	newRetainedDB(t, cacheDir, "archive-machine-b-claude-remote-proj-deadbeef.db", "sess-foreign")

	got, err := RetainedMatches(cacheDir, "", time.Time{}, 0, "")
	if err != nil {
		t.Fatalf("RetainedMatches: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("RetainedMatches = %d rows, want 1 (archive db skipped): %+v", len(got), got)
	}
	if got[0].SessionID != "sess-local" {
		t.Errorf("SessionID = %q, want sess-local", got[0].SessionID)
	}
}
