package archive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

// shortLockWait shrinks the busy-wait cap for the duration of a test.
func shortLockWait(t *testing.T, d time.Duration) {
	t.Helper()
	old := lockWaitMax
	lockWaitMax = d
	t.Cleanup(func() { lockWaitMax = old })
}

// holdSyncLock takes the sync lock out-of-band (a stand-in for a sibling
// process; flock contends per file descriptor, so a second flock handle in
// the same process conflicts exactly like another process would) and returns
// its release.
func holdSyncLock(t *testing.T) func() {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(syncLockPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	fl := flock.New(syncLockPath())
	locked, err := fl.TryLock()
	if err != nil || !locked {
		t.Fatalf("pre-hold sync lock: locked=%v err=%v", locked, err)
	}
	return func() { _ = fl.Unlock() }
}

// TestPushLocal_BusyLockIsCleanNoOp: a second local pusher behind a held lock
// comes back with ErrBusy once the wait cap elapses — no clone mutation, no
// hang, no interleaved staging.
func TestPushLocal_BusyLockIsCleanNoOp(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)
	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	shortLockWait(t, 250*time.Millisecond)
	release := holdSyncLock(t)
	defer release()

	before := remoteCommitCount(t, bare)
	rep, err := a.PushLocal(context.Background())
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("PushLocal under held lock = (%+v, %v), want ErrBusy", rep, err)
	}
	if rep.Copied != 0 || rep.Committed || rep.Pushed {
		t.Errorf("busy push touched the clone: %+v", rep)
	}
	if after := remoteCommitCount(t, bare); after != before {
		t.Errorf("busy push moved the remote: %d → %d", before, after)
	}
}

// TestPushLocal_WaitsForRelease: a pusher arriving while the lock is held
// WAITS (within the cap) and completes once the holder releases — the
// "waits cleanly" half of the contract.
func TestPushLocal_WaitsForRelease(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)
	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	release := holdSyncLock(t)
	go func() {
		time.Sleep(300 * time.Millisecond)
		release()
	}()

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("PushLocal after release: %v", err)
	}
	if !rep.Pushed {
		t.Errorf("report = %+v, want a completed push", rep)
	}
}

// TestPushLocal_CanceledCallerReportsCancel: when the CALLER's ctx dies while
// waiting for the lock, the error is the ctx's — not a misleading ErrBusy.
func TestPushLocal_CanceledCallerReportsCancel(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)
	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	release := holdSyncLock(t)
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = a.PushLocal(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("PushLocal with dead ctx = %v, want DeadlineExceeded", err)
	}
}

// TestConcurrentPushes_Serialize: two overlapping local pushes both succeed —
// one waits for the other — and the remote ends consistent (both files
// present, no staging interleave, no tmp-dir deletion race). Run under -race.
func TestConcurrentPushes_Serialize(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)
	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	// A second Archive value = a second flock handle: same-process stand-in
	// for the sibling pusher (flock contention is per file descriptor).
	b, err := Load()
	if err != nil || b == nil {
		t.Fatalf("Load sibling: (%v, %v)", b, err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, ar := range []*Archive{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = ar.PushLocal(context.Background())
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil && !errors.Is(err, ErrBusy) {
			t.Fatalf("concurrent push %d: %v", i, err)
		}
	}

	verify := checkoutRemote(t, bare)
	for _, want := range []string{
		"machine-a/claude/-tmp-proj/sess-1111.jsonl",
		"machine-a/codex/2026/07/rollout-abc.jsonl",
	} {
		if _, err := os.Stat(verify + "/" + want); err != nil {
			t.Errorf("remote missing %s after concurrent pushes: %v", want, err)
		}
	}
}

// TestPull_BusyLockIsCleanNoOp: an explicit pull behind a held lock reports
// ErrBusy instead of racing the holder's clone mutation.
func TestPull_BusyLockIsCleanNoOp(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)
	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	shortLockWait(t, 250*time.Millisecond)
	release := holdSyncLock(t)
	defer release()

	pulled, err := a.Pull(context.Background(), false)
	if !errors.Is(err, ErrBusy) || pulled {
		t.Fatalf("Pull under held lock = (%v, %v), want (false, ErrBusy)", pulled, err)
	}
}

// TestPull_ThrottleRecheckedAfterWait: a throttled pull that waited out a
// holder re-checks the stamp before touching git — if the holder just pulled,
// the wait ends in a throttled skip, not a redundant network round-trip.
func TestPull_ThrottleRecheckedAfterWait(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)
	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	// No stamp yet: the throttled pull is due, so it proceeds to the lock.
	if err := os.Remove(pullStampPath()); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	release := holdSyncLock(t)
	done := make(chan struct{})
	var gitCalls int
	a.run = func(ctx context.Context, dir string, args ...string) (string, error) {
		gitCalls++
		return "", nil
	}
	go func() {
		defer close(done)
		// While the pull waits on the lock, the "holder" refreshes the clone
		// (fresh stamp), then releases.
		time.Sleep(200 * time.Millisecond)
		stampPull()
		release()
	}()

	pulled, err := a.Pull(context.Background(), true)
	<-done
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if pulled || gitCalls != 0 {
		t.Errorf("post-wait throttled pull = (pulled=%v, gitCalls=%d), want a stamped skip with zero git", pulled, gitCalls)
	}
}
