package archive

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// writeCloneSentinel marks a (fixture) clone dir as fully created, the state
// a real successful `git clone` + sentinel write leaves behind.
func writeCloneSentinel(t *testing.T, clone string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(clone, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, ".git", cloneSentinel), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEnsureClone_RebuildsTornClone: a clone interrupted mid-`git clone`
// (`.git` and the origin remote exist — git sets both up early — but the
// sentinel was never written) is detected and rebuilt; the next push works
// against the fresh clone.
func TestEnsureClone_RebuildsTornClone(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Simulate the kill-mid-clone state: replace the good clone with one that
	// has a .git dir and the right origin but never finished (no sentinel, no
	// objects, no checkout).
	if err := os.RemoveAll(a.ClonePath()); err != nil {
		t.Fatal(err)
	}
	gitT(t, "", "init", a.ClonePath())
	gitT(t, a.ClonePath(), "remote", "add", "origin", bare)

	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push over torn clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git", cloneSentinel)); err != nil {
		t.Errorf("rebuilt clone missing sentinel: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.ClonePath(), "machine-a", ".rawclaw-machine.json")); err != nil {
		t.Errorf("rebuilt clone missing our machine dir: %v", err)
	}

	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-1111.jsonl")); err != nil {
		t.Errorf("remote lost content across the rebuild: %v", err)
	}
}

// TestEnsureClone_RebuildsUnrecoverableRebaseWedge: a kill during the rebase
// itself can leave marker state `rebase --abort` cannot undo (detached HEAD,
// torn internals). Recovery must not stop at the failed abort — the clone is
// rebuilt and the push lands.
func TestEnsureClone_RebuildsUnrecoverableRebaseWedge(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Wedge the clone the way a kill mid-rebase does: marker dir present,
	// HEAD detached — but with internals too torn for `rebase --abort`.
	if err := os.MkdirAll(filepath.Join(a.ClonePath(), ".git", "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, a.ClonePath(), "checkout", "--detach")

	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push over unrecoverable wedge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git", "rebase-merge")); !os.IsNotExist(err) {
		t.Error("rebase marker survived recovery")
	}
	out := gitT(t, a.ClonePath(), "symbolic-ref", "--short", "HEAD")
	if strings.TrimSpace(out) == "" {
		t.Error("HEAD still detached after recovery")
	}
}

// TestEnsureClone_AbortsRealWedgedRebase: a REAL mid-rebase wedge (conflict
// state left by an interrupted `pull --rebase`) is aborted by the next verb;
// the clone comes back to a branch and the marker is gone.
func TestEnsureClone_AbortsRealWedgedRebase(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Build real rebase-conflict state inside the clone: a local commit and a
	// remote commit both touch the same path with different content, then
	// `pull --rebase` stops mid-rebase — the exact state a kill leaves.
	conflict := filepath.Join(a.ClonePath(), "machine-a", "claude", "-tmp-proj", "sess-1111.jsonl")
	if err := os.WriteFile(conflict, []byte(`{"local":"side"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, a.ClonePath(), "add", "-A")
	gitT(t, a.ClonePath(), "commit", "-m", "local side")

	other := filepath.Join(t.TempDir(), "other")
	gitT(t, "", "clone", bare, other)
	if err := os.WriteFile(filepath.Join(other, "machine-a", "claude", "-tmp-proj", "sess-1111.jsonl"),
		[]byte(`{"remote":"side"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, other, "add", "-A")
	gitT(t, other, "commit", "-m", "remote side")
	gitT(t, other, "push", "origin", "HEAD")

	// The interrupted rebase: git stops with conflict state on disk.
	cmd := exec.Command("git", "pull", "--rebase", "origin", "main")
	cmd.Dir = a.ClonePath()
	_ = cmd.Run() // expected to fail mid-rebase
	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git", "rebase-merge")); err != nil {
		t.Skip("could not construct a wedged rebase on this git version")
	}

	// The next pull must not wedge on the leftover state. (It may still
	// ERROR: the synthetic conflicting commit re-conflicts on the fresh
	// rebase — a clear failure is fine; a wedged clone is not. Real machine
	// dirs are disjoint, so production rebases are content-conflict-free.)
	_, _ = a.Pull(context.Background(), false)
	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git", "rebase-merge")); !os.IsNotExist(err) {
		t.Error("real rebase wedge not cleared")
	}
	out := gitT(t, a.ClonePath(), "symbolic-ref", "--short", "HEAD")
	if strings.TrimSpace(out) == "" {
		t.Error("HEAD detached after recovery")
	}
}

// TestRecoverStaleOps_AgedIndexLock: an index.lock older than the grace
// period (git itself died holding it) is removed so the clone unwedges; a
// FRESH lock is honored — a concurrent healthy push may own it.
func TestRecoverStaleOps_AgedIndexLock(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	lock := filepath.Join(a.ClonePath(), ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * lockGracePeriod)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push over aged index.lock: %v", err)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Error("aged index.lock survived recovery")
	}

	// Fresh lock: recovery leaves it; the push fails loudly instead of
	// sabotaging a possible concurrent writer.
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PushLocal(context.Background()); err == nil {
		// No new content — the push may legitimately no-op without staging.
		// Force a staging attempt next time by adding a transcript.
		writeTranscript(t, filepath.Join(home, ".claude", "projects"),
			"-tmp-proj/sess-fresh.jsonl", "{}\n")
		if _, err := a.PushLocal(context.Background()); err == nil {
			t.Fatal("push succeeded through a fresh index.lock, want loud failure")
		}
	}
	if _, err := os.Stat(lock); err != nil {
		t.Error("fresh index.lock removed — concurrent writer sabotaged")
	}
}

// killSteps are the git steps the kill-injection suite dies at — between them
// they bracket every phase of the push sequence after the copy: staging,
// committing, pushing. (Mid-copy kills are covered by the temp-dir design and
// TestPushLocal_ClearsStaleCopyTemps; mid-clone and mid-rebase kills by the
// EnsureClone tests above.)
var killSteps = []string{"add", "commit", "push"}

// TestPushKillHelper is the subprocess half of the kill-injection suite: it
// runs PushLocal against the parent's HOME and dies with os.Exit — no defers,
// no cleanup, true kill semantics — the moment the injected git step is
// reached. Skipped unless spawned by TestPushLocal_KillInjection.
func TestPushKillHelper(t *testing.T) {
	step := os.Getenv("RAWCLAW_TEST_KILL_STEP")
	if step == "" {
		t.Skip("helper: only runs as a subprocess")
	}
	a, err := Load()
	if err != nil || a == nil {
		t.Fatalf("helper Load: (%v, %v)", a, err)
	}
	real := a.run
	a.run = func(ctx context.Context, dir string, args ...string) (string, error) {
		// Match the step anywhere in args: the commit invocation carries
		// identity flags before the subcommand ("-c user.name=... commit").
		for _, arg := range args {
			if arg == step {
				os.Exit(9) // the kill: nothing after this line runs
			}
		}
		return real(ctx, dir, args...)
	}
	rep, perr := a.PushLocal(context.Background())
	t.Logf("helper: step %q never hit; report=%+v err=%v", step, rep, perr)
	os.Exit(0) // reached only if the step never came up
}

// TestPushLocal_KillInjection: for every kill point in the push sequence, a
// subprocess dies mid-push (os.Exit — no cleanup), then a normal in-process
// push must fully recover: it succeeds, the session content reaches the
// remote, and the remote holds no partial state beyond git's own atomicity
// (a commit either absent or complete).
func TestPushLocal_KillInjection(t *testing.T) {
	for _, step := range killSteps {
		t.Run("kill-at-"+step, func(t *testing.T) {
			home := newTestHome(t)
			bare := initBareRepo(t)
			seedTranscripts(t, home)

			// The parent process caches its machine id process-wide (minted
			// under an earlier test's HOME); persist it into THIS home's
			// state file so the helper subprocess loads the same identity
			// instead of minting a fresh one.
			if err := os.WriteFile(filepath.Join(store.CacheDir(), "machine-id"),
				[]byte(provenance.MachineID()+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := Init(context.Background(), bare, "machine-a"); err != nil {
				t.Fatalf("Init: %v", err)
			}

			cmd := exec.Command(os.Args[0], "-test.run", "^TestPushKillHelper$", "-test.v")
			cmd.Env = append(os.Environ(),
				"RAWCLAW_TEST_KILL_STEP="+step,
				"HOME="+home,
				"CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"),
				"CODEX_HOME="+filepath.Join(home, ".codex"),
			)
			out, err := cmd.CombinedOutput()
			ee, ok := err.(*exec.ExitError)
			if !ok || ee.ExitCode() != 9 {
				t.Fatalf("helper at step %q: err=%v (want exit 9)\n%s", step, err, out)
			}

			// Recovery: a plain push on the surviving state.
			a, err := Load()
			if err != nil || a == nil {
				t.Fatalf("Load: (%v, %v)", a, err)
			}
			rep, err := a.PushLocal(context.Background())
			if err != nil {
				t.Fatalf("recovery push after kill at %q: %v", step, err)
			}
			if !rep.Pushed {
				t.Errorf("recovery push after kill at %q landed nothing", step)
			}

			// The remote holds the full content and a consistent history.
			verify := checkoutRemote(t, bare)
			for _, want := range []string{
				"machine-a/claude/-tmp-proj/sess-1111.jsonl",
				"machine-a/codex/2026/07/rollout-abc.jsonl",
				"machine-a/.rawclaw-machine.json",
			} {
				if _, err := os.Stat(filepath.Join(verify, want)); err != nil {
					t.Errorf("remote missing %s after recovery: %v", want, err)
				}
			}
			gitT(t, bare, "fsck", "--strict")

			// And the clone is healthy: a second push is a clean no-op.
			rep2, err := a.PushLocal(context.Background())
			if err != nil {
				t.Fatalf("steady-state push after recovery: %v", err)
			}
			if rep2.Committed || rep2.Pushed {
				t.Errorf("steady-state push not a no-op: %+v", rep2)
			}
		})
	}
}

// TestEnsureClone_TransientFailureDoesNotDestroy: a COMPLETED clone (sentinel
// present) whose `remote get-url` probe fails for an environmental reason must
// surface a hard error — never be wiped. A rebuild discards any
// committed-but-unpushed sync; destruction needs positive torn-state
// evidence, not absence of proof.
func TestEnsureClone_TransientFailureDoesNotDestroy(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// A stranded local commit: content committed in the clone, never pushed.
	writeTranscript(t, a.ClonePath(), "machine-a/claude/-tmp-proj/sess-stranded.jsonl", "{}\n")
	gitT(t, a.ClonePath(), "add", "-A")
	gitT(t, a.ClonePath(), "commit", "-m", "machine-a: sync transcripts")
	stranded := strings.TrimSpace(gitT(t, a.ClonePath(), "rev-parse", "HEAD"))

	// Environmental failure: the remote probe errors (unreadable config,
	// exec failure, dying ctx — all look the same to the caller).
	real := a.run
	a.run = func(ctx context.Context, dir string, args ...string) (string, error) {
		if args[0] == "remote" {
			return "", errors.New("simulated transient failure")
		}
		return real(ctx, dir, args...)
	}
	if _, err := a.PushLocal(context.Background()); err == nil {
		t.Fatal("push with failing remote probe succeeded, want hard error")
	}

	// The clone survived, stranded commit intact.
	if got := strings.TrimSpace(gitT(t, a.ClonePath(), "rev-parse", "HEAD")); got != stranded {
		t.Errorf("clone HEAD changed under a transient failure: %s -> %s", stranded, got)
	}

	// Recovery once the environment heals: the stranded commit goes out.
	a.run = real
	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("recovery push: %v", err)
	}
	if !rep.Pushed {
		t.Error("stranded commit not pushed after recovery")
	}
	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-stranded.jsonl")); err != nil {
		t.Errorf("stranded content lost: %v", err)
	}
}

// TestPushLocal_OrphanGitInterleave: the residual the single-writer flock
// cannot close — a watchdog hard-exit releases the lock as the process dies,
// while its last git child (already ctx-cancelled/SIGTERMed) may briefly
// outlive it and keep touching the clone. A second pusher entering during
// that window must never corrupt anything: a FRESH index.lock is honored
// (fail loudly, no sabotage), the marker-abort race converges, and once the
// orphan is gone the next push fully succeeds.
func TestPushLocal_OrphanGitInterleave(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// The dead holder's leavings: mid-rebase marker + a FRESH index.lock,
	// with a live "orphan git" that finishes its (cancelled) work shortly
	// after — clearing its own state, as a SIGTERMed git does.
	marker := filepath.Join(a.ClonePath(), ".git", "rebase-merge")
	lock := filepath.Join(a.ClonePath(), ".git", "index.lock")
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	orphanDone := make(chan struct{})
	go func() {
		defer close(orphanDone)
		time.Sleep(300 * time.Millisecond)
		_ = os.RemoveAll(marker)
		_ = os.Remove(lock)
	}()

	// New content arrives; the second pusher enters while the orphan lives.
	writeTranscript(t, filepath.Join(home, ".claude", "projects"),
		"-tmp-proj/sess-during-orphan.jsonl", "{}\n")
	_, perr := a.PushLocal(context.Background())
	// Either outcome is legitimate: a loud failure on the honored fresh lock,
	// or success if the orphan finished first. Corruption is not.
	t.Logf("push during orphan window: err=%v", perr)

	<-orphanDone
	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("push after orphan exit: %v", err)
	}
	if perr == nil && (rep.Committed || rep.Pushed) {
		// If the first push already landed everything, this one must no-op.
		t.Errorf("post-orphan push not a no-op after a successful interleaved push: %+v", rep)
	}

	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-during-orphan.jsonl")); err != nil {
		t.Errorf("content lost across the orphan interleave: %v", err)
	}
	gitT(t, bare, "fsck", "--strict")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("rebase marker survived convergence")
	}
}
