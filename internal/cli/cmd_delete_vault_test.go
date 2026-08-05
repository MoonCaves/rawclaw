package cli

import (
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/index"
)

// indexSession writes a session and indexes it, so rawclaw's transcript vault
// holds its own copy of the transcript — the state an explicit delete has to
// clean up, and the one a rebuild would otherwise read the session back out of.
func indexSession(t *testing.T, root, project, id string, nLines int) string {
	t.Helper()
	path := writeSession(t, root, project, id, nLines)
	if _, _, _, err := index.EnsureIndexed(filepath.Dir(path), false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	return path
}

// TestDeleteCmd_EvictsVaultedTranscript: a real delete of a LIVE session has to
// take rawclaw's own copy with it. The tombstone alone would stop a rebuild
// from restoring the session, but the raw bytes would survive on disk — which
// contradicts the receipt the command prints ("Removed rawclaw's copy").
func TestDeleteCmd_EvictsVaultedTranscript(t *testing.T) {
	root := newCfgRoot(t)
	const id = "cafe0021-0000-0000-0000-000000000021"
	indexSession(t, root, "ledger", id, 2)

	if !durable.Has(id) {
		t.Fatalf("indexing should have vaulted %s; nothing to delete", id)
	}

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "--files", id)
	if err != nil {
		t.Fatalf("delete: %v\nout: %s", err, out)
	}
	if durable.Has(id) {
		p, _ := durable.PathFor(id)
		t.Errorf("delete left rawclaw's own transcript copy at %s", p)
	}
}

// TestDeleteCmd_EvictsVaultedRetainedTranscript: the same guarantee for a
// RETAINED session. Its source file is already gone, so the vault copy is the
// only remaining transcript — which makes leaving it behind worse here, not
// better.
func TestDeleteCmd_EvictsVaultedRetainedTranscript(t *testing.T) {
	root := newCfgRoot(t)
	const id = "cafe0022-0000-0000-0000-000000000022"
	retainSession(t, root, "billing", id, 2)

	if !durable.Has(id) {
		t.Fatalf("retention should have kept %s vaulted; nothing to delete", id)
	}

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", id)
	if err != nil {
		t.Fatalf("delete retained: %v\nout: %s", err, out)
	}
	if durable.Has(id) {
		p, _ := durable.PathFor(id)
		t.Errorf("delete left the retained session's only transcript at %s", p)
	}
}
