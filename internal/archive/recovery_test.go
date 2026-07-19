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

// TestEnsureClone_CheckoutKilledCloneNotAdopted: `git clone` writes refs and
// HEAD BEFORE the working-tree checkout, so a clone killed mid-checkout has a
// resolvable branch HEAD while its index and worktree never materialized.
// Adopting that shape would be catastrophic: the next push's `add -A --
// <machine>` builds a fresh index holding ONLY this machine's dir and the
// commit drops every other machine's dir from the archive tip. The shape must
// fail adoption (dirty/absent index ≠ completed checkout) and be rebuilt —
// after which a push must leave the remote with every machine's content
// intact.
func TestEnsureClone_CheckoutKilledCloneNotAdopted(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	// Shape the mid-checkout kill: sentinel never written, refs + HEAD are in
	// place, but the index and the checked-out tree are not.
	gitDir := filepath.Join(a.ClonePath(), ".git")
	if err := os.Remove(filepath.Join(gitDir, cloneSentinel)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(gitDir, "index")); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"machine-a", "machine-b"} {
		if err := os.RemoveAll(filepath.Join(a.ClonePath(), d)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push over checkout-killed clone: %v", err)
	}

	verify := checkoutRemote(t, a.cfg.Remote)
	for _, want := range []string{
		"machine-b/claude/-remote-proj/sess-bbbb.jsonl", // the foreign dir must survive
		"machine-a/.rawclaw-machine.json",
	} {
		if _, err := os.Stat(filepath.Join(verify, want)); err != nil {
			t.Errorf("remote lost %s after recovery from a checkout-killed clone: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(gitDir, cloneSentinel)); err != nil {
		t.Errorf("rebuilt clone missing sentinel: %v", err)
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

// TestInit_RefusesToWipeStrandedClone: re-init after config loss must not
// silently destroy a leftover clone's unpushed commits — same doctrine as
// the rebuild guard. Deleting the config alone (the partial state-loss case)
// and re-initing errors with the recovery path; the clone survives intact.
func TestInit_RefusesToWipeStrandedClone(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push: %v", err)
	}
	stranded := strandClone(t, a, "machine-a/claude/-tmp-proj/sess-reinit.jsonl")

	// The state-loss shape: config gone, clone (with its commit) intact.
	if err := os.Remove(configPath()); err != nil {
		t.Fatal(err)
	}

	if _, err := Init(context.Background(), bare, "machine-a"); err == nil {
		t.Fatal("re-init over a clone with unpushed commits succeeded, want refusal")
	}
	out := gitT(t, a.ClonePath(), "rev-list", "--all")
	if !strings.Contains(out, stranded) {
		t.Errorf("stranded commit %s destroyed by refused re-init", stranded)
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
	defer func() { a.run = real }()
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
