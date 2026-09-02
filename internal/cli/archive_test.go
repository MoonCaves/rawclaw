package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArchiveExportBundle_Unconfigured verifies that exporting a bundle on an
// unconfigured machine is a clean no-op pointing to archive init.
func TestArchiveExportBundle_Unconfigured(t *testing.T) {
	newArchiveHome(t)
	bundlePath := filepath.Join(t.TempDir(), "archive.bundle")

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "export-bundle", bundlePath)
	if err != nil {
		t.Fatalf("export-bundle unconfigured returned error: %v", err)
	}
	if !strings.Contains(out, "Archive not configured") {
		t.Errorf("export-bundle output = %q, want unconfigured notice", out)
	}
}

// TestArchiveBundle_EndToEnd verifies exporting a bundle on machine-a and
// seeding machine-b from the exported bundle.
func TestArchiveBundle_EndToEnd(t *testing.T) {
	homeA := newArchiveHome(t)
	bare := newBareRemote(t)

	// Init and populate machine-a
	rootA := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, rootA, "", "archive", "init", bare, "--name", "machine-a")
	if err != nil {
		t.Fatalf("machine-a archive init: %v\n%s", err, out)
	}

	p := filepath.Join(homeA, ".claude", "projects", "-proj", "sess.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{\"type\":\"user\",\"text\":\"hello\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootA = NewRootCmd(BuildInfo{})
	out, err = runCmd(t, rootA, "", "archive", "push")
	if err != nil {
		t.Fatalf("machine-a archive push: %v\n%s", err, out)
	}

	// Export bundle from machine-a
	bundleDir := t.TempDir()
	bundlePath := filepath.Join(bundleDir, "archive.bundle")
	rootA = NewRootCmd(BuildInfo{})
	out, err = runCmd(t, rootA, "", "archive", "export-bundle", bundlePath)
	if err != nil {
		t.Fatalf("archive export-bundle: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Archive bundle exported to") {
		t.Errorf("export-bundle output missing confirmation: %s", out)
	}
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("exported bundle file does not exist: %v", err)
	}

	// Switch to fresh environment for machine-b
	homeB := newArchiveHome(t)
	rootB := NewRootCmd(BuildInfo{})
	out, err = runCmd(t, rootB, "", "archive", "init", "--from-bundle", bundlePath, bare, "--name", "machine-b")
	if err != nil {
		t.Fatalf("machine-b archive init --from-bundle: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Archive initialized from bundle") {
		t.Errorf("machine-b init output missing bundle banner: %s", out)
	}
	if !strings.Contains(out, "machine-b") {
		t.Errorf("machine-b init output missing machine name: %s", out)
	}
	if !strings.Contains(out, bundlePath) {
		t.Errorf("machine-b init output missing bundle path: %s", out)
	}

	// Push from machine-b to verify git remote is configured and healthy
	pB := filepath.Join(homeB, ".claude", "projects", "-proj", "sess-b.jsonl")
	if err := os.MkdirAll(filepath.Dir(pB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pB, []byte("{\"type\":\"user\",\"text\":\"from machine b\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootB = NewRootCmd(BuildInfo{})
	out, err = runCmd(t, rootB, "", "archive", "push")
	if err != nil {
		t.Fatalf("machine-b archive push: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Pushed 1 file(s)") {
		t.Errorf("machine-b push output = %q, want pushed-1 report", out)
	}
}

// TestArchiveInit_MissingBundle verifies that specifying a non-existent bundle
// fails cleanly with a descriptive error.
func TestArchiveInit_MissingBundle(t *testing.T) {
	newArchiveHome(t)
	missingPath := filepath.Join(t.TempDir(), "nonexistent.bundle")

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "init", "--from-bundle", missingPath, "git@github.com:user/repo.git")
	if err == nil {
		t.Fatalf("expected error for missing bundle, got success: %s", out)
	}
	if !strings.Contains(err.Error(), "bundle file") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want bundle file not found message", err.Error())
	}
}

// TestArchiveHelpText_BundleSeeding verifies help menus include guidance on
// the 15-second cold-start seeding mechanism.
func TestArchiveHelpText_BundleSeeding(t *testing.T) {
	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "init", "--help")
	if err != nil {
		t.Fatalf("archive init --help: %v", err)
	}
	if !strings.Contains(out, "--from-bundle") {
		t.Errorf("init --help missing --from-bundle flag doc:\n%s", out)
	}
	if !strings.Contains(out, "15-second") {
		t.Errorf("init --help missing 15-second cold-start description:\n%s", out)
	}

	root = NewRootCmd(BuildInfo{})
	out, err = runCmd(t, root, "", "archive", "export-bundle", "--help")
	if err != nil {
		t.Fatalf("archive export-bundle --help: %v", err)
	}
	if !strings.Contains(out, "15-second") {
		t.Errorf("export-bundle --help missing 15-second cold-start description:\n%s", out)
	}
}
