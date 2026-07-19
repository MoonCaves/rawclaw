package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/archive/archivetest"
)

// TestArchiveAutosyncCmd_PushesAndReceipts: the sync child pushes new local
// transcript bytes to the remote and receipts both halves — a completed push
// and the throttled pull skip (the fixture's own pull just stamped).
func TestArchiveAutosyncCmd_PushesAndReceipts(t *testing.T) {
	archivetest.Setup(t, "")

	// New local bytes since the fixture's push.
	sess := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects", "-local-proj", "localsess.jsonl")
	f, err := os.OpenFile(sess, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"user","uuid":"aaaa1111-bbbb-cccc-dddd-eeeeeeeeeef0","timestamp":"2026-06-01T10:01:00Z","cwd":"/local/proj","message":{"role":"user","content":"more"}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "autosync")
	if err != nil {
		t.Fatalf("archive autosync: %v\n%s", err, out)
	}
	if !strings.Contains(out, "push: 1 file(s) pushed") {
		t.Errorf("receipt missing the push line, got:\n%s", out)
	}
	if !strings.Contains(out, "pull: skipped (throttled)") {
		t.Errorf("receipt missing the throttled-pull line, got:\n%s", out)
	}
}

// TestArchiveAutosyncCmd_UpToDateReceipt: nothing new → an honest "up to
// date" receipt, no error.
func TestArchiveAutosyncCmd_UpToDateReceipt(t *testing.T) {
	archivetest.Setup(t, "")

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "autosync")
	if err != nil {
		t.Fatalf("archive autosync: %v\n%s", err, out)
	}
	if !strings.Contains(out, "push: up to date") {
		t.Errorf("receipt missing up-to-date line, got:\n%s", out)
	}
}

// TestArchiveAutosyncCmd_UnconfiguredNoOp: the config can vanish between
// spawn and child start — the child receipts the no-op and exits 0.
func TestArchiveAutosyncCmd_UnconfiguredNoOp(t *testing.T) {
	newArchiveHome(t)

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "autosync")
	if err != nil {
		t.Fatalf("unconfigured autosync errored: %v", err)
	}
	if !strings.Contains(out, "archive not configured") {
		t.Errorf("receipt = %q, want the not-configured no-op line", out)
	}
}

// TestArchiveAutosyncCmd_Hidden: autosync is an implementation seam, not user
// surface — it must not appear in `archive` help.
func TestArchiveAutosyncCmd_Hidden(t *testing.T) {
	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "--help")
	if err != nil {
		t.Fatalf("archive --help: %v", err)
	}
	if strings.Contains(out, "autosync") {
		t.Errorf("hidden verb leaked into help:\n%s", out)
	}
}
