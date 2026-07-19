package archive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initBareRepo creates a local bare git repo (the test stand-in for any remote)
// and returns its path.
func initBareRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	dir := filepath.Join(t.TempDir(), "remote.git")
	gitT(t, "", "init", "--bare", "--initial-branch=main", dir)
	return dir
}

// requireGit skips the test when no system git is available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("system git not available")
	}
}

// gitT runs git in dir, failing the test on error. Identity is pinned so
// commits work under the isolated (gitconfig-less) HOME.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.invalid",
		"-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.Command("git", full...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// checkoutRemote clones the bare repo into a scratch dir so a test can assert
// on what actually reached the remote.
func checkoutRemote(t *testing.T, bare string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "verify")
	gitT(t, "", "clone", bare, dst)
	return dst
}

// TestInit_TracerBootstrap: init against an empty local bare repo → clone
// created, config written, machine registered (manifest with the stable
// machine id landed in the remote).
func TestInit_TracerBootstrap(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Clone exists and is a git repo.
	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git")); err != nil {
		t.Errorf("clone missing: %v", err)
	}

	// Config round-trips.
	got, err := Load()
	if err != nil || got == nil {
		t.Fatalf("Load after init = (%v, %v), want configured", got, err)
	}
	if got.Remote() != bare || got.Name() != "machine-a" {
		t.Errorf("Load config = (%q, %q), want (%q, machine-a)", got.Remote(), got.Name(), bare)
	}

	// The manifest reached the REMOTE (not just the clone).
	verify := checkoutRemote(t, bare)
	m, err := readManifest(filepath.Join(verify, "machine-a"))
	if err != nil {
		t.Fatalf("manifest not in remote: %v", err)
	}
	if m.MachineID != a.machineID {
		t.Errorf("manifest machine_id = %q, want %q", m.MachineID, a.machineID)
	}
	if m.Name != "machine-a" {
		t.Errorf("manifest name = %q, want machine-a", m.Name)
	}
	if m.UpdatedAt == "" || m.Hostname == "" {
		t.Errorf("manifest missing hostname/updated_at: %+v", m)
	}
}

// TestInit_RefusesClaimedName: a machine dir already claimed by a DIFFERENT
// machine_id refuses init — the belt-and-suspenders against two machines
// colliding on one human-readable name.
func TestInit_RefusesClaimedName(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)

	// Another machine claims "machine-a" in the remote.
	other := filepath.Join(t.TempDir(), "other-clone")
	gitT(t, "", "clone", bare, other)
	if err := writeManifest(filepath.Join(other, "machine-a"), manifest{
		MachineID: "feedfacefeedfacefeedfacefeedface",
		Name:      "machine-a",
		Hostname:  "other-host",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	gitT(t, other, "add", "-A")
	gitT(t, other, "commit", "-m", "register machine-a")
	gitT(t, other, "push", "origin", "HEAD")

	_, err := Init(context.Background(), bare, "machine-a")
	if err == nil {
		t.Fatal("Init succeeded, want refusal for claimed name")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Errorf("refusal error should point at --name, got: %v", err)
	}

	// Refusal must not leave config behind.
	if a, _ := Load(); a != nil {
		t.Error("config written despite refusal")
	}
}

// TestInit_SameMachineReclaims: re-init by the SAME machine (same machine_id,
// e.g. after state loss) is not a conflict.
func TestInit_SameMachineReclaims(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)

	if _, err := Init(context.Background(), bare, "machine-a"); err != nil {
		t.Fatalf("first init: %v", err)
	}
	// Simulate config loss (machine-id survives in the state dir).
	if err := os.Remove(configPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(context.Background(), bare, "machine-a"); err != nil {
		t.Fatalf("re-init after config loss: %v", err)
	}
}

// TestInit_AlreadyConfigured: a second init with config present refuses with a
// pointer at the config file.
func TestInit_AlreadyConfigured(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)

	if _, err := Init(context.Background(), bare, "machine-a"); err != nil {
		t.Fatalf("first init: %v", err)
	}
	_, err := Init(context.Background(), bare, "machine-b")
	if err == nil {
		t.Fatal("second init succeeded, want already-initialized error")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Errorf("error = %v, want already-initialized", err)
	}
}

// TestInit_RejectsBadName: names that would escape the machine dir, hide it,
// or act as git pathspec magic (globs, magic-prefix colons) are rejected up
// front — the name flows into `git add -- <name>` as a pathspec.
func TestInit_RejectsBadName(t *testing.T) {
	newTestHome(t)

	tests := []struct {
		name string
		bad  string
	}{
		{"path separator", "a/b"},
		{"backslash", `a\b`},
		{"parent traversal", ".."},
		{"hidden dir", ".hidden"},
		{"glob star", "machine*"},
		{"glob question mark", "machine?"},
		{"glob bracket", "ma[ch]ine"},
		{"pathspec magic colon", ":top"},
		{"space", "my machine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Init(context.Background(), "/nonexistent.git", tt.bad); err == nil {
				t.Errorf("Init(name=%q) succeeded, want rejection", tt.bad)
			}
		})
	}
}

// TestSanitizeMachineName locks the hostname → dir-name mapping.
func TestSanitizeMachineName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"mac hostname drops domain and case", "Alices-MacBook-Pro.local", "alices-macbook-pro"},
		{"plain name passes through", "box7", "box7"},
		{"specials fold to dashes", "Weird Host!Name", "weird-host-name"},
		{"nothing survives", "...", "machine"},
		{"empty hostname", "", "machine"},
		{"fqdn keeps first label", "host.sub.example.com", "host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeMachineName(tt.in); got != tt.want {
				t.Errorf("sanitizeMachineName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
