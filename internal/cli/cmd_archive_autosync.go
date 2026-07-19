package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/spf13/cobra"
)

// newArchiveAutosyncCmd wires the hidden `rawclaw archive autosync` verb: the
// body of the detached background sync child that ordinary searches spawn.
// Hidden — it is an implementation seam, not user surface; its stdout is the
// receipt log the spawner redirects it into. One process does push + throttled
// pull so a single setsid child (and a single log) covers the whole sync.
func newArchiveAutosyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "autosync",
		Short:         "Push + throttled pull (the detached background sync child)",
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAutosyncChild(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

// runAutosyncChild is the sync child's whole life: push this machine's
// transcripts, then a throttled pull — each step receipted as one timestamped
// log line. A busy sync lock is a clean skip (the holder covers the same
// work), an unconfigured archive a clean no-op (the config could vanish
// between spawn and start). Real failures are receipted AND returned, so the
// child's exit code stays honest.
func runAutosyncChild(ctx context.Context, w io.Writer) error {
	// Bound every git subprocess comfortably below this child's own watchdog:
	// the watchdog's hard exit releases the sync flock as the process dies,
	// but only a dying ctx actually kills a hung git — without this margin an
	// orphaned git could outlive the lock and overlap the next sync's writer.
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, autosyncChildTimeout-30*time.Second)
	defer cancel()

	a, err := archive.Load()
	if err != nil {
		autosyncLogLine(w, "autosync: %v", err)
		return err
	}
	if a == nil {
		autosyncLogLine(w, "autosync: archive not configured; nothing to do")
		return nil
	}

	var failed error
	rep, err := a.PushLocal(ctx)
	switch {
	case errors.Is(err, archive.ErrBusy):
		autosyncLogLine(w, "push: skipped (%v)", err)
	case err != nil:
		autosyncLogLine(w, "push: %v", err)
		failed = err
	case rep.Pushed:
		autosyncLogLine(w, "push: %d file(s) pushed (retries %d)", rep.Copied, rep.Retries)
	default:
		autosyncLogLine(w, "push: up to date")
	}

	pulled, err := a.Pull(ctx, true)
	switch {
	case errors.Is(err, archive.ErrBusy):
		autosyncLogLine(w, "pull: skipped (%v)", err)
	case err != nil:
		autosyncLogLine(w, "pull: %v", err)
		if failed == nil {
			failed = err
		}
	case pulled:
		autosyncLogLine(w, "pull: refreshed")
	default:
		autosyncLogLine(w, "pull: skipped (throttled)")
	}
	return failed
}

// autosyncLogLine writes one timestamped receipt line to the sync log.
func autosyncLogLine(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, time.Now().UTC().Format(time.RFC3339)+" "+format+"\n", args...)
}
