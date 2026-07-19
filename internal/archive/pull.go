package archive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// pullThrottleWindow is the minimum gap between throttled pulls. Five minutes
// matches the sync-on-invoke cadence: within the window a throttled pull is a
// single stat of the stamp file, no git, no network.
const pullThrottleWindow = 5 * time.Minute

// Pull refreshes the clone from the remote (re-cloning it if it is missing —
// deleting a corrupt clone and pulling is the documented recovery). With
// throttle=true it no-ops unless the last successful pull is older than
// pullThrottleWindow, judged by the stamp file's mtime in the state dir; the
// explicit CLI verb passes false and always pulls. Returns pulled=true when a
// network refresh actually ran ("already up to date" still counts — the clone
// was verified current), false on a throttled skip or an empty remote.
func (a *Archive) Pull(ctx context.Context, throttle bool) (bool, error) {
	if throttle && !pullDue(time.Now()) {
		return false, nil
	}
	if err := a.ensureClone(ctx); err != nil {
		return false, err
	}
	branch, err := a.currentBranch(ctx)
	if err != nil {
		return false, err
	}
	out, err := a.run(ctx, a.clone, "pull", "--rebase", "origin", branch)
	if err != nil {
		if isMissingRemoteRef(out) {
			// Empty remote: its default branch is born on the first push. Nothing
			// to pull is a fresh state, so the stamp is still written.
			stampPull()
			return false, nil
		}
		_, _ = a.run(ctx, a.clone, "rebase", "--abort") // never leave a wedged clone
		return false, fmt.Errorf("pull archive: %w", err)
	}
	stampPull()
	return true, nil
}

// isMissingRemoteRef classifies a pull failure against a remote whose branch
// does not exist yet (an empty archive repo before the first push).
func isMissingRemoteRef(out string) bool {
	return strings.Contains(out, "couldn't find remote ref")
}

// pullStampPath is <state-dir>/archive/last-pull — the throttle stamp. Its
// MTIME is the record (the file body stays empty), so freshness is one stat.
func pullStampPath() string {
	return filepath.Join(store.CacheDir(), "archive", "last-pull")
}

// pullDue reports whether a throttled pull should run: no stamp yet, or the
// stamp is at least pullThrottleWindow old.
func pullDue(now time.Time) bool {
	st, err := os.Stat(pullStampPath())
	if err != nil {
		return true
	}
	return now.Sub(st.ModTime()) >= pullThrottleWindow
}

// stampPull records a successful pull by (re)writing the stamp file, updating
// its mtime to now. Best-effort: a failed stamp only means the next throttled
// pull runs again.
func stampPull() {
	p := pullStampPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(p, nil, 0o644)
}
