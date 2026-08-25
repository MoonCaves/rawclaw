package semantic

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MoonCaves/rawclaw/internal/adapters"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
)

// DefaultTopupMaxNew is the maximum number of new messages embedded during one
// background top-up pass. Bounding per-invoke work prevents an un-embedded or
// newly imported corpus from stampeding the embedder backend or locking the db.
const DefaultTopupMaxNew = 50

// topupWindow is the minimum gap between background top-up spawns for a single
// database store. Within it an ordinary invocation costs one stat check.
const topupWindow = 1 * time.Minute

// topupLockPath returns the flock file path for serializing vector top-ups on a store:
// <state-dir>/vector-topup/<encoded-dbp>.lock
func topupLockPath(dbp string) string {
	enc := filepath.Base(filepath.Clean(dbp))
	return filepath.Join(store.CacheDir(), "vector-topup", enc+".lock")
}

// topupTokenPath returns <state-dir>/vector-topup/<encoded-dbp>.token — the spawn throttle token.
func topupTokenPath(dbp string) string {
	enc := filepath.Base(filepath.Clean(dbp))
	return filepath.Join(store.CacheDir(), "vector-topup", enc+".token")
}

// VectorTopupLogPath is <state-dir>/vector-topup.log — where the detached
// vector top-up child's output lands.
func VectorTopupLogPath() string {
	return filepath.Join(store.CacheDir(), "vector-topup.log")
}

// AcquireTopupToken reports whether a background vector top-up may spawn now for
// dbp, and atomically claims the slot when it may. Mirrors archive.AcquireAutosyncToken.
func AcquireTopupToken(dbp string, now time.Time) bool {
	p := topupTokenPath(dbp)
	st, err := os.Stat(p)
	if err == nil {
		if age := now.Sub(st.ModTime()); age > -topupWindow && age < topupWindow {
			return false
		}
		_ = os.Remove(p)
	} else if !os.IsNotExist(err) {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// TryAcquireTopupLock takes the machine-wide single-writer lock for vector
// top-up on dbp WITHOUT waiting. ok=false if another top-up is already running on
// dbp or on any lock error. Uses flock.New, mirroring archive's tryAcquireSyncLock.
func TryAcquireTopupLock(dbp string) (release func(), ok bool) {
	p := topupLockPath(dbp)
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

var noVector atomic.Bool

// SetNoVector records whether --no-vector is active for the current process/invocation.
func SetNoVector(v bool) {
	noVector.Store(v)
}

// IsNoVector reports whether --no-vector is active.
func IsNoVector() bool {
	return noVector.Load()
}

var (
	spawnMu          sync.RWMutex
	spawnVectorTopup func(dbp string)
)

// SetSpawnVectorTopup sets the spawner hook (wired by package cli to spawn a detached child).
func SetSpawnVectorTopup(fn func(string)) {
	spawnMu.Lock()
	spawnVectorTopup = fn
	spawnMu.Unlock()
}

// MaybeVectorTopup quietly keeps a scope's vectors current after an ordinary
// indexing pass. Detached self-invocation so search latency is unaffected.
// Gates:
// 1. IsNoVector() returns true -> return (respect --no-vector).
// 2. adapters.GetEmbedder() returns nil -> return (unconfigured, zero spawns).
// 3. AcquireTopupToken(dbp, now) -> rate-limit spawns per store.
func MaybeVectorTopup(dbp string) {
	if IsNoVector() {
		return
	}
	emb := adapters.GetEmbedder()
	if emb == nil {
		return
	}
	if !AcquireTopupToken(dbp, time.Now()) {
		return
	}
	spawnMu.RLock()
	fn := spawnVectorTopup
	spawnMu.RUnlock()
	if fn != nil {
		fn(dbp)
	}
}

// init installs MaybeVectorTopup as the index package's post-index hook, so a
// vector top-up follows every indexing pass without index importing this
// package. Registration is one-way: index never learns what the embedder is.
func init() {
	index.SetVectorTopupHook(MaybeVectorTopup)
}
