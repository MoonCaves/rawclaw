package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArchiveStatusCmd: after init + push, `archive status` reports the
// remote, the clone, this machine's dir with a fresh last-sync, a recorded
// last push, and a never-pulled pull stamp.
func TestArchiveStatusCmd(t *testing.T) {
	home := newArchiveHome(t)
	bare := newBareRemote(t)

	p := filepath.Join(home, ".claude", "projects", "-proj", "sess.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd(BuildInfo{})
	if out, err := runCmd(t, root, "", "archive", "init", bare, "--name", "machine-a"); err != nil {
		t.Fatalf("archive init: %v\n%s", err, out)
	}
	root = NewRootCmd(BuildInfo{})
	if out, err := runCmd(t, root, "", "archive", "push"); err != nil {
		t.Fatalf("archive push: %v\n%s", err, out)
	}

	root = NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "status")
	if err != nil {
		t.Fatalf("archive status: %v\n%s", err, out)
	}
	for _, want := range []string{bare, "machine-a", "this machine", "last push:"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "last pull:   never") {
		t.Errorf("status should report a never-run pull:\n%s", out)
	}
	if strings.Contains(out, "last push:   never") {
		t.Errorf("status reports last push as never right after a push:\n%s", out)
	}
	if strings.Contains(out, "STALE") {
		t.Errorf("fresh own dir flagged STALE:\n%s", out)
	}
}

// TestArchiveStatusCmd_Unconfigured: status without init is a clean no-op
// pointing at init — same contract as push/pull.
func TestArchiveStatusCmd_Unconfigured(t *testing.T) {
	newArchiveHome(t)

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "status")
	if err != nil {
		t.Fatalf("unconfigured status errored: %v", err)
	}
	if !strings.Contains(out, "archive init") {
		t.Errorf("no-op output should point at init, got %q", out)
	}
}

// TestArchivePushCmd_ReportsRemovals: a push that propagates a local delete
// says so — the user sees the archive-side removal, not a silent shrink.
func TestArchivePushCmd_ReportsRemovals(t *testing.T) {
	home := newArchiveHome(t)
	bare := newBareRemote(t)

	p := filepath.Join(home, ".claude", "projects", "-proj", "sess-gone.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd(BuildInfo{})
	if out, err := runCmd(t, root, "", "archive", "init", bare, "--name", "machine-a"); err != nil {
		t.Fatalf("archive init: %v\n%s", err, out)
	}
	root = NewRootCmd(BuildInfo{})
	if out, err := runCmd(t, root, "", "archive", "push"); err != nil {
		t.Fatalf("first push: %v\n%s", err, out)
	}

	// v0.3.0 delete semantics: file removed, id tombstoned.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	tomb := filepath.Join(home, ".cache", "session-search", ".deleted")
	if err := os.MkdirAll(filepath.Dir(tomb), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tomb, []byte("sess-gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "push")
	if err != nil {
		t.Fatalf("push after delete: %v\n%s", err, out)
	}
	if !strings.Contains(out, "removed 1 deleted session(s)") {
		t.Errorf("push output = %q, want removal report", out)
	}
}
