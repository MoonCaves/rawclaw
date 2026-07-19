package index

import (
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// originOf reads the origin_machine stamped on one session row.
func originOf(t *testing.T, dbp, sid string) string {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", dbp, err)
	}
	defer con.Close()
	var origin string
	if err := con.QueryRow("SELECT origin_machine FROM sessions WHERE id=?", sid).Scan(&origin); err != nil {
		t.Fatalf("read origin_machine for %s: %v", sid, err)
	}
	return origin
}

// TestEnsureIndexedTree_StampsGivenOrigin: a tree ingested with an explicit
// origin stamps THAT id as origin_machine — the seam a replicated tree (another
// machine's transcripts) uses so its rows carry the tree owner's identity, not
// the indexing machine's.
func TestEnsureIndexedTree_StampsGivenOrigin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "sess-foreign.jsonl"),
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"remote question"}}`,
	)
	dbp := filepath.Join(t.TempDir(), "foreign.db")

	const foreign = "feedfacefeedfacefeedfacefeedface"
	n, status, err := EnsureIndexedTree(dbp, dir, false, foreign)
	if err != nil {
		t.Fatalf("EnsureIndexedTree: %v", err)
	}
	if n != 1 || status != IndexFresh {
		t.Errorf("EnsureIndexedTree = (%d, %v), want (1, fresh)", n, status)
	}
	if got := originOf(t, dbp, "sess-foreign"); got != foreign {
		t.Errorf("origin_machine = %q, want the given origin %q", got, foreign)
	}
}

// TestEnsureIndexedTree_EmptyOriginIsLocal: origin "" keeps the existing
// behavior — rows stamped with THIS machine's id (EnsureIndexed's path).
func TestEnsureIndexedTree_EmptyOriginIsLocal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "sess-local.jsonl"),
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"local question"}}`,
	)
	dbp := filepath.Join(t.TempDir(), "local.db")

	if _, _, err := EnsureIndexedTree(dbp, dir, false, ""); err != nil {
		t.Fatalf("EnsureIndexedTree: %v", err)
	}
	if got := originOf(t, dbp, "sess-local"); got != provenance.MachineID() {
		t.Errorf("origin_machine = %q, want local machine id %q", got, provenance.MachineID())
	}
}

// TestEnsureIndexedContainers_StampsGivenOrigin: the container ingest path
// honors the same origin seam.
func TestEnsureIndexedContainers_StampsGivenOrigin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	p := filepath.Join(dir, "rollout-x.jsonl")
	writeJSONL(t, p, `{"type":"session_meta"}`)
	cs := []source.Container{{ID: "rollout-x", Path: p}}
	msgs := func(source.Container) ([]model.Message, error) {
		return []model.Message{{Role: "user", Text: "hi", UUID: "u1"}}, nil
	}
	dbp := filepath.Join(t.TempDir(), "codex-foreign.db")

	const foreign = "beefbeefbeefbeefbeefbeefbeefbeef"
	if _, _, err := EnsureIndexedContainers(dbp, false, cs, msgs, "codex", foreign); err != nil {
		t.Fatalf("EnsureIndexedContainers: %v", err)
	}
	if got := originOf(t, dbp, "rollout-x"); got != foreign {
		t.Errorf("origin_machine = %q, want the given origin %q", got, foreign)
	}
}
