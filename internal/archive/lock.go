package archive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
)

// ErrBusy reports that another rawclaw process on this machine holds the
// archive sync lock. Callers treat it as a clean "someone else is already
// syncing" no-op, not a failure: the holder's run (or the next sync) covers
// the same trees.
var ErrBusy = errors.New("another rawclaw archive sync is already running on this machine")

// lockRetryDelay is the poll interval while waiting for the sync lock.
// flock acquisition costs a syscall per attempt, so the tick is coarse
// enough to limit churn while still reacting quickly on release.
const lockRetryDelay = 100 * time.Millisecond

// lockWaitMax bounds how long an acquirer waits for a busy lock before giving
// up with ErrBusy. Long enough to ride out a routine push by a sibling
// process; short enough that a manual verb behind a slow holder answers with
// the clean busy message instead of appearing hung. A var only so tests can
// shrink the wait.
var lockWaitMax = 10 * time.Second

// syncLockPath is <state-dir>/archive/clone.lock — the single-writer lock for
// every clone mutation (push and pull). It lives BESIDE the clone, never
// inside it: the clone is documented as freely deletable, and a lock file
// inside it would let "delete the clone dir" split the guarantee (the old
// holder keeps the deleted inode while a new pusher locks the fresh file —
// two writers on one clone).
func syncLockPath() string {
	return filepath.Join(store.CacheDir(), "archive", "clone.lock")
}

// acquireSyncLock takes the machine-wide single-writer lock serializing clone
// mutations across processes (concurrent pushes, push-vs-pull). It polls a
// try-lock until acquired, the wait cap elapses, or ctx is done — both give
// ErrBusy, so a second local syncer no-ops or waits cleanly, never interleaves.
// The returned release both unlocks and closes the lock fd.
// tryAcquireSyncLock takes the sync lock WITHOUT waiting: one try-lock, no
// poll. Read-side callers (scope enumeration deciding whether the clone is
// quiescent enough to ingest from) use it so a search never blocks behind a
// running sync — they degrade to serving existing state instead. ok=false on
// any failure (held elsewhere, fs error): the caller treats "can't prove
// quiescent" exactly like "busy".
func tryAcquireSyncLock() (release func(), ok bool) {
	p := syncLockPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, false
	}
	fl := flock.New(p)
	locked, err := fl.TryLock()
	if err != nil || !locked {
		return nil, false
	}
	return func() { _ = fl.Unlock() }, true
}

func acquireSyncLock(ctx context.Context) (release func(), err error) {
	p := syncLockPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, fmt.Errorf("create archive state dir: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, lockWaitMax)
	defer cancel()
	fl := flock.New(p)
	locked, err := fl.TryLockContext(waitCtx, lockRetryDelay)
	if err != nil || !locked {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr // the CALLER's ctx died — report that, not a fake busy
		}
		if waitCtx.Err() != nil {
			return nil, ErrBusy // wait cap elapsed with the lock still held
		}
		return nil, fmt.Errorf("acquire archive sync lock %s: %w", p, err)
	}
	return func() { _ = fl.Unlock() }, nil
}
