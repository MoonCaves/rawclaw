package archive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPull_FetchesForeignCommits: a foreign machine pushes → Pull lands its dir
// in our clone and writes the throttle stamp.
func TestPull_FetchesForeignCommits(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	if _, err := os.Stat(filepath.Join(a.ClonePath(), "machine-b", manifestName)); err != nil {
		t.Errorf("foreign machine dir missing after pull: %v", err)
	}
	if _, err := os.Stat(pullStampPath()); err != nil {
		t.Errorf("throttle stamp missing after successful pull: %v", err)
	}
}

// TestPull_ThrottleSkipsWithinWindow: a fresh stamp makes a throttled pull a
// pure no-op — zero git invocations, no network.
func TestPull_ThrottleSkipsWithinWindow(t *testing.T) {
	newTestHome(t)
	stampPull() // just pulled

	calls := 0
	a := &Archive{
		cfg:   Config{Remote: "example.invalid/archive.git", Name: "machine-a"},
		clone: t.TempDir(),
		run: func(ctx context.Context, dir string, args ...string) (string, error) {
			calls++
			return "", nil
		},
	}

	pulled, err := a.Pull(context.Background(), true)
	if err != nil {
		t.Fatalf("throttled Pull: %v", err)
	}
	if pulled {
		t.Error("pulled = true, want throttled skip")
	}
	if calls != 0 {
		t.Errorf("git invoked %d times under throttle, want 0", calls)
	}
}

// TestPull_ThrottleRunsWhenStampOld: an aged stamp lets the throttled pull
// through — and the stamp is refreshed afterward.
func TestPull_ThrottleRunsWhenStampOld(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)
	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("PushLocal: %v", err)
	}

	stampPull()
	old := time.Now().Add(-2 * pullThrottleWindow)
	if err := os.Chtimes(pullStampPath(), old, old); err != nil {
		t.Fatal(err)
	}

	pulled, err := a.Pull(context.Background(), true)
	if err != nil {
		t.Fatalf("throttled Pull past window: %v", err)
	}
	if !pulled {
		t.Error("pulled = false, want a real pull once the stamp aged out")
	}
	st, err := os.Stat(pullStampPath())
	if err != nil {
		t.Fatalf("stamp gone after pull: %v", err)
	}
	if time.Since(st.ModTime()) > time.Minute {
		t.Error("stamp mtime not refreshed by the pull")
	}
}

// TestPull_ExplicitBypassesThrottle: throttle=false pulls even with a fresh
// stamp — the explicit verb always refreshes.
func TestPull_ExplicitBypassesThrottle(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)
	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("PushLocal: %v", err)
	}
	stampPull() // just pulled — must not matter

	pulled, err := a.Pull(context.Background(), false)
	if err != nil {
		t.Fatalf("explicit Pull: %v", err)
	}
	if !pulled {
		t.Error("explicit Pull skipped, want it to bypass the throttle")
	}
}

// TestPull_RecreatesDeletedClone: deleting the clone dir and pulling re-clones
// — corrupt-clone recovery ("every local artifact is a rebuildable
// cache").
func TestPull_RecreatesDeletedClone(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	if err := os.RemoveAll(a.ClonePath()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Pull(context.Background(), false); err != nil {
		t.Fatalf("Pull after clone deletion: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.ClonePath(), "machine-b", manifestName)); err != nil {
		t.Errorf("re-cloned archive missing the foreign dir: %v", err)
	}
}

// TestPull_EmptyRemoteSucceeds: pulling an archive whose remote is still empty
// (its default branch is born on the first push, which hasn't happened
// anywhere yet) succeeds — the remote was consulted and verified empty-fresh,
// so pulled is true and the stamp is written. Config written directly — Init
// would push the registration and un-empty the remote.
func TestPull_EmptyRemoteSucceeds(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)
	if err := writeConfig(Config{Remote: bare, Name: "machine-a"}); err != nil {
		t.Fatal(err)
	}
	a, err := Load()
	if err != nil || a == nil {
		t.Fatalf("Load: (%v, %v)", a, err)
	}

	pulled, err := a.Pull(context.Background(), false)
	if err != nil {
		t.Fatalf("Pull against empty remote: %v", err)
	}
	if !pulled {
		t.Error("pulled = false against an empty remote, want verified-fresh true")
	}
	if _, err := os.Stat(pullStampPath()); err != nil {
		t.Errorf("throttle stamp missing after empty-remote pull: %v", err)
	}
}

// TestPull_NetworkFailureSurfaces: a pull that fails for a non-empty-remote
// reason returns the error (the caller decides how to degrade) and aborts any
// half-applied rebase.
func TestPull_NetworkFailureSurfaces(t *testing.T) {
	newTestHome(t)

	var calls []string
	clone := t.TempDir()
	if err := os.MkdirAll(filepath.Join(clone, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &Archive{
		cfg:   Config{Remote: "example.invalid/archive.git", Name: "machine-a"},
		clone: clone,
		run: func(ctx context.Context, dir string, args ...string) (string, error) {
			calls = append(calls, strings.Join(args, " "))
			switch args[0] {
			case "remote":
				return "example.invalid/archive.git\n", nil
			case "symbolic-ref":
				return "main\n", nil
			case "pull":
				return "fatal: unable to access remote", errors.New("exit status 128")
			default:
				return "", nil
			}
		},
	}

	if _, err := a.Pull(context.Background(), false); err == nil {
		t.Fatal("Pull succeeded, want surfaced failure")
	}
	aborted := false
	for _, c := range calls {
		if c == "rebase --abort" {
			aborted = true
		}
	}
	if !aborted {
		t.Error("failed pull did not abort the rebase — clone could wedge")
	}
}
