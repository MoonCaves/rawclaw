package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// defaultTimeout bounds a single rawclaw invocation. The consumer is an agent:
// the tool MUST self-terminate rather than wedge a parent that has no external
// `timeout(1)` around it. 30s is generous for a keyword search over the local
// transcript corpus; long jobs (e.g. --reindex-vectors over every project) can
// raise it or set --timeout 0 to disable the watchdog.
const defaultTimeout = 30 * time.Second

// upgradeWatchdog is the watchdog floor for `rawclaw upgrade` when the user gave no
// explicit --timeout / RAWCLAW_TIMEOUT. The self-update makes up to four serial
// network legs (releases API, the redirect fallback, the asset, checksums.txt), each
// bounded by netTimeout (60s); the 30s default watchdog would otherwise kill a
// legitimate download. This floor sits above the worst-case sum of those legs so the
// download can finish, while the per-leg netTimeouts still guarantee the run never
// hangs unbounded.
const upgradeWatchdog = 5 * time.Minute

// resolveTimeout picks the effective deadline: an explicit --timeout flag wins,
// else RAWCLAW_TIMEOUT (a Go duration like "45s" or "2m"), else defaultTimeout.
// A non-positive value disables the watchdog (returns 0). A malformed env var is
// ignored (falls through to the default) rather than aborting the run.
func resolveTimeout(flagSet bool, flagVal time.Duration, env string) time.Duration {
	if flagSet {
		return flagVal
	}
	if env != "" {
		if d, err := time.ParseDuration(env); err == nil {
			return d
		}
	}
	return defaultTimeout
}

// childKillGrace is the pause between the deadline cancelling the run context
// (which SIGKILLs any child started via exec.CommandContext — the ssh a live
// peek dials) and the watchdog's own exit(124): long enough for the kill to
// land, far too short to dent the never-hang guarantee.
const childKillGrace = 200 * time.Millisecond

// startWatchdog arms the hard self-bounding guarantee. It returns the run's
// context — cancelled the instant the deadline fires, so a child process
// started under it dies with the run instead of surviving as an orphan — and a
// `stop` that the caller MUST defer: stop() cancels the deadline and waits for
// the watchdog goroutine to exit, so a successful (or any returned) run leaks
// nothing.
//
// On deadline the watchdog writes a clear message to stderr, waits
// childKillGrace for context cancellation to reap children, and calls exit(124)
// — the conventional `timeout(1)` code — which terminates the process even if
// the main goroutine is parked inside a CGO-free SQLite call that context
// cancellation alone cannot unblock. That hard exit is the whole point: an agent
// can rely on rawclaw never hanging past its deadline.
//
// A non-positive timeout disables the watchdog: the context never expires and
// stop() is then a no-op.
func startWatchdog(timeout time.Duration, stderr io.Writer, exit func(int)) (ctx context.Context, stop func()) {
	if timeout <= 0 {
		return context.Background(), func() {}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	done := make(chan struct{})

	go func() {
		defer close(done)
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(stderr, "rawclaw: timed out after %s (raise --timeout or set --timeout 0 to disable)\n", timeout)
			time.Sleep(childKillGrace)
			exit(124)
		}
	}()

	return ctx, func() {
		cancel()
		<-done
	}
}

// osExit is the production exit hook (overridable in tests).
var osExit = os.Exit
