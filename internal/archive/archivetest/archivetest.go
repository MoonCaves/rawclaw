// Package archivetest stands up throwaway transcript-archive fixtures: an
// isolated HOME with one local Claude project, a configured archive against a
// local bare remote, and one FOREIGN machine dir pushed by a simulated second
// machine and pulled into the local clone. Tests across scopes/agentproto/cli
// drive the real enumeration + search paths against it.
package archivetest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/archive"
)

// Stable fixture identities, exported so tests can assert on them.
const (
	LocalName   = "machine-a" // this machine's archive dir name
	ForeignName = "machine-b" // the foreign machine's dir name
	ForeignID   = "beefbeefbeefbeefbeefbeefbeefbeef"

	// LocalBeacon / ForeignBeacon are distinctive search tokens planted in the
	// local and foreign transcripts.
	LocalBeacon   = "localbeacon"
	ForeignBeacon = "foreignbeacon"

	// ForeignSession/ForeignUUID identify the foreign transcript's one message:
	// ref = ForeignSession[:8] + ":" + ForeignUUID[:8].
	ForeignSession = "bbbb2222deadbeef"
	ForeignUUID    = "cccc3333-dddd-eeee-ffff-000011112222"
	ForeignCWD     = "/remote/proj"
)

// Fixture is the stood-up archive world.
type Fixture struct {
	Home    string
	Bare    string
	Archive *archive.Archive
}

// Setup builds the fixture. foreignDate pins the foreign machine's commit
// date (RFC3339; "" = now) — an old date makes its dir read as stale.
func Setup(t *testing.T, foreignDate string) *Fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("system git not available")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("RAWCLAW_ARCHIVE", "")
	// A CONFIGURED archive would let any test that drives the real command
	// tree spawn a detached background sync of the test binary — kill the
	// trigger fixture-wide; autosync's own tests opt back in explicitly.
	t.Setenv("RAWCLAW_ARCHIVE_AUTOSYNC", "off")

	// Local Claude project with a searchable beacon.
	writeFile(t, filepath.Join(home, ".claude", "projects", "-local-proj", "localsess.jsonl"),
		`{"type":"user","uuid":"aaaa1111-bbbb-cccc-dddd-eeeeeeeeeeee","timestamp":"2026-06-01T10:00:00Z","cwd":"/local/proj","message":{"role":"user","content":"question about `+LocalBeacon+` here"}}`+"\n")

	// Archive: init against a bare remote, push our dir.
	bare := filepath.Join(t.TempDir(), "remote.git")
	git(t, "", nil, "init", "--bare", "--initial-branch=main", bare)
	a, err := archive.Init(context.Background(), bare, LocalName)
	if err != nil {
		t.Fatalf("archive.Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("archive.PushLocal: %v", err)
	}

	// The foreign machine pushes its dir from its own clone.
	cloneB := filepath.Join(t.TempDir(), "clone-b")
	git(t, "", nil, "clone", bare, cloneB)
	bdir := filepath.Join(cloneB, ForeignName)
	writeFile(t, filepath.Join(bdir, ".rawclaw-machine.json"),
		`{"machine_id":"`+ForeignID+`","name":"`+ForeignName+`","hostname":"b-host","updated_at":"2026-01-01T00:00:00Z"}`+"\n")
	writeFile(t, filepath.Join(bdir, "claude", "-remote-proj", ForeignSession+".jsonl"),
		`{"type":"user","uuid":"`+ForeignUUID+`","timestamp":"2026-06-02T10:00:00Z","cwd":"`+ForeignCWD+`","message":{"role":"user","content":"asking about `+ForeignBeacon+` today"}}`+"\n")
	git(t, cloneB, nil, "add", "-A")
	var env []string
	if foreignDate != "" {
		env = []string{"GIT_AUTHOR_DATE=" + foreignDate, "GIT_COMMITTER_DATE=" + foreignDate}
	}
	git(t, cloneB, env, "commit", "-m", ForeignName+": sync transcripts")
	git(t, cloneB, nil, "push", "origin", "HEAD")

	// Pull the foreign dir into the local clone.
	if _, err := a.Pull(context.Background(), false); err != nil {
		t.Fatalf("archive.Pull: %v", err)
	}
	return &Fixture{Home: home, Bare: bare, Archive: a}
}

// writeFile writes content at path, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// git runs git in dir (extraEnv appended) with a pinned identity, failing the
// test on error.
func git(t *testing.T, dir string, extraEnv []string, args ...string) {
	t.Helper()
	full := append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.invalid",
		"-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.Command("git", full...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
