package archive

import (
	"os"
	"path/filepath"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// autosyncWindow is the minimum gap between background sync spawns: within it
// an ordinary invocation costs one stat, no child process. Matches the pull
// throttle — the child's throttled pull then usually runs, keeping the two
// cadences aligned.
const autosyncWindow = 5 * time.Minute

// autosyncTokenPath is <state-dir>/archive/last-autosync — the spawn-throttle
// token. Its MTIME is the record (the body stays empty), shared across
// processes; sibling stamp to the pull throttle's last-pull.
func autosyncTokenPath() string {
	return filepath.Join(store.CacheDir(), "archive", "last-autosync")
}

// AcquireAutosyncToken reports whether a background sync may spawn now, and
// atomically claims the slot when it may. The token is claimed at SPAWN time
// (not child completion), so a burst of invocations spawns one child even
// while that child is still running. Claiming = removing the stale token and
// re-creating it O_EXCL: of N racing processes exactly the creators win, and
// the rare double-winner (remove/create interleaving) costs one extra sync —
// the token rate-limits, it does not guard correctness (the flock does).
func AcquireAutosyncToken(now time.Time) bool {
	p := autosyncTokenPath()
	st, err := os.Stat(p)
	if err == nil {
		if now.Sub(st.ModTime()) < autosyncWindow {
			return false
		}
		_ = os.Remove(p)
	} else if !os.IsNotExist(err) {
		return false // unreadable state dir: stay quiet rather than spawn-storm
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false // lost the race (or unwritable): another process owns the slot
	}
	_ = f.Close()
	return true
}

// AutosyncLogPath is <state-dir>/archive/autosync.log — where the detached
// sync child's output lands (its receipt trail). The spawner redirects the
// child's stdout+stderr here; nothing is ever written to the invoking
// terminal.
func AutosyncLogPath() string {
	return filepath.Join(store.CacheDir(), "archive", "autosync.log")
}
