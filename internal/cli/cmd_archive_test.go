package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newArchiveHome isolates HOME + both transcript roots for archive CLI tests.
func newArchiveHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("RAWCLAW_ARCHIVE", "")
	return home
}

// newBareRemote creates a local bare repo for the archive to push to.
func newBareRemote(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("system git not available")
	}
	dir := filepath.Join(t.TempDir(), "remote.git")
	out, err := exec.Command("git", "init", "--bare", "--initial-branch=main", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	return dir
}

// TestArchiveInitCmd: the tracer's front door — init prints the privacy
// warning and writes the config; push then reports the upload.
func TestArchiveInitCmd(t *testing.T) {
	home := newArchiveHome(t)
	bare := newBareRemote(t)

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "init", bare, "--name", "machine-a")
	if err != nil {
		t.Fatalf("archive init: %v\n%s", err, out)
	}
	if !strings.Contains(out, "PRIVATE") {
		t.Errorf("init output missing privacy warning:\n%s", out)
	}
	if !strings.Contains(out, "machine-a") {
		t.Errorf("init output missing machine dir:\n%s", out)
	}

	// A transcript, then push through the CLI.
	p := filepath.Join(home, ".claude", "projects", "-proj", "sess.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd(BuildInfo{}) // fresh tree per execute (cobra state)
	out, err = runCmd(t, root, "", "archive", "push")
	if err != nil {
		t.Fatalf("archive push: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Pushed 1 file(s)") {
		t.Errorf("push output = %q, want pushed-1 report", out)
	}
}

// TestArchivePushCmd_UnconfiguredNoOp: `archive push` without init is a clean
// no-op (exit 0) with a pointer at init — the feature-off contract.
func TestArchivePushCmd_UnconfiguredNoOp(t *testing.T) {
	newArchiveHome(t)

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "push")
	if err != nil {
		t.Fatalf("unconfigured push errored: %v", err)
	}
	if !strings.Contains(out, "archive init") {
		t.Errorf("no-op output should point at init, got %q", out)
	}
}

// TestArchiveSessionVerbStillWorks: the pre-existing `archive <session>`
// (move-to-archive-dir) verb must keep working with subcommands nested under
// the same verb.
func TestArchiveSessionVerbStillWorks(t *testing.T) {
	root := newCfgRoot(t)
	src := writeSession(t, root, "-proj", "cafebabe-dead-beef-cafe-babe12345678", 3)
	archiveDir := filepath.Join(filepath.Dir(root), "archive")

	cmd := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, cmd, "", "archive", src, "--archive-dir", archiveDir)
	if err != nil {
		t.Fatalf("archive <session>: %v\n%s", err, out)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("session file not moved out of the active tree")
	}
}
