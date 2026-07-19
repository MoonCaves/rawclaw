package archive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// strandClone plants a commit that exists ONLY in the clone (not in the live
// transcript tree, not on the remote) — the exact artifact a rebuild destroys —
// and returns its SHA.
func strandClone(t *testing.T, a *Archive, rel string) string {
	t.Helper()
	writeTranscript(t, a.ClonePath(), rel, "{}\n")
	gitT(t, a.ClonePath(), "add", "-A")
	gitT(t, a.ClonePath(), "commit", "-m", "machine-a: sync transcripts")
	return strings.TrimSpace(gitT(t, a.ClonePath(), "rev-parse", "HEAD"))
}

// TestEnsureClone_LegacyCloneStrandedCommitPreserved: a structurally complete
// clone WITHOUT the sentinel (created before the sentinel existed) that holds
// an unpushed commit must never be wiped — sentinel absence is not positive
// torn-state evidence, and the rebuild would destroy the commit. The clone is
// adopted (sentinel stamped) and the stranded commit reaches the remote.
func TestEnsureClone_LegacyCloneStrandedCommitPreserved(t *testing.T) {
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

	// Content that exists only in the clone: committed, never pushed.
	strandClone(t, a, "machine-a/claude/-tmp-proj/sess-legacy.jsonl")
	// The pre-sentinel era: the marker was never written.
	if err := os.Remove(filepath.Join(a.ClonePath(), ".git", cloneSentinel)); err != nil {
		t.Fatal(err)
	}

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("push over legacy clone: %v", err)
	}
	if !rep.Pushed {
		t.Error("stranded commit not pushed from the adopted legacy clone")
	}
	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git", cloneSentinel)); err != nil {
		t.Errorf("adopted legacy clone not stamped with the sentinel: %v", err)
	}
	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-legacy.jsonl")); err != nil {
		t.Errorf("legacy clone's stranded commit lost: %v", err)
	}
}

// TestEnsureClone_LegacyCloneAdoptedNotWiped: a clean, fully pushed clone
// without the sentinel is adopted in place (stamped), never wiped-and-recloned
// — a witness file under .git survives the push.
func TestEnsureClone_LegacyCloneAdoptedNotWiped(t *testing.T) {
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

	if err := os.Remove(filepath.Join(a.ClonePath(), ".git", cloneSentinel)); err != nil {
		t.Fatal(err)
	}
	witness := filepath.Join(a.ClonePath(), ".git", "rawclaw-test-witness")
	if err := os.WriteFile(witness, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push over clean legacy clone: %v", err)
	}
	if _, err := os.Stat(witness); err != nil {
		t.Errorf("legacy clone was wiped instead of adopted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git", cloneSentinel)); err != nil {
		t.Errorf("adopted legacy clone not stamped: %v", err)
	}
}

// TestEnsureClone_WedgeWithStrandedCommitRefusesRebuild: even on positive
// wedge evidence (marker + detached HEAD that recovery cannot reattach), a
// clone holding commits absent from the remote is NEVER wiped — rebuild
// requires proof that nothing would be destroyed. The push fails loudly and
// the commit survives.
func TestEnsureClone_WedgeWithStrandedCommitRefusesRebuild(t *testing.T) {
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

	stranded := strandClone(t, a, "machine-a/claude/-tmp-proj/sess-wedged.jsonl")
	// The unrecoverable wedge: marker state `rebase --abort` cannot undo.
	if err := os.MkdirAll(filepath.Join(a.ClonePath(), ".git", "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, a.ClonePath(), "checkout", "--detach")

	if _, err := a.PushLocal(context.Background()); err == nil {
		t.Fatal("push over a wedge holding unpushed commits succeeded, want loud refusal")
	}
	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git")); err != nil {
		t.Fatalf("clone wiped despite stranded commits: %v", err)
	}
	out := gitT(t, a.ClonePath(), "rev-list", "--all")
	if !strings.Contains(out, stranded) {
		t.Errorf("stranded commit %s no longer reachable in the clone", stranded)
	}
}

// TestEnsureClone_CancelMidRecoveryDoesNotDestroy: the watchdog (or any
// caller) cancelling its ctx mid-recovery must not convert a recoverable
// wedge into a rebuild — the failed HEAD probe under a dead ctx is not
// positive corruption evidence, and the RemoveAll would destroy an unpushed
// commit. The run fails loudly; the clone and its commit survive.
func TestEnsureClone_CancelMidRecoveryDoesNotDestroy(t *testing.T) {
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

	stranded := strandClone(t, a, "machine-a/claude/-tmp-proj/sess-cancel.jsonl")
	// A wedged rebase as a kill leaves it: marker present, HEAD detached.
	if err := os.MkdirAll(filepath.Join(a.ClonePath(), ".git", "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, a.ClonePath(), "checkout", "--detach")

	// The watchdog fires the moment recovery reaches its `rebase --abort`:
	// the CALLER's ctx dies, and every git run on a dead ctx fails — exactly
	// what exec.CommandContext does after cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	real := a.run
	a.run = func(c context.Context, dir string, args ...string) (string, error) {
		for _, arg := range args {
			if arg == "rebase" {
				cancel()
			}
		}
		if cerr := c.Err(); cerr != nil {
			return "", cerr
		}
		return real(c, dir, args...)
	}

	if _, err := a.PushLocal(ctx); err == nil {
		t.Fatal("push with ctx dying mid-recovery succeeded, want loud failure")
	}
	a.run = real

	if _, err := os.Stat(filepath.Join(a.ClonePath(), ".git")); err != nil {
		t.Fatalf("clone destroyed by a cancelled-ctx recovery: %v", err)
	}
	out := gitT(t, a.ClonePath(), "rev-list", "--all")
	if !strings.Contains(out, stranded) {
		t.Errorf("stranded commit %s lost under a cancelled-ctx recovery", stranded)
	}
}
