package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/spf13/cobra"
)

// newArchiveInitCmd wires `rawclaw archive init <remote-url>`: clone (or start
// on an empty remote), register this machine under a human-readable dir name,
// push the registration, persist the config — and print the privacy warning.
func newArchiveInitCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "init <remote-url>",
		Short: "Set up the transcript archive against a git remote",
		Long: "Set up the transcript archive: clone <remote-url> (an empty repo works — it is " +
			"born on the first push), register this machine under a top-level dir, and write " +
			"the config that turns the archive feature on. The remote must be a PRIVATE " +
			"repository: transcripts contain whatever was pasted into sessions.\n\n" +
			"A shorthand is expanded to a full SSH remote: `user/repo` → " +
			"git@github.com:user/repo.git, a bare `user` → git@github.com:user/" + defaultArchiveRepo +
			".git, `host/user[/repo]` likewise. A full URL (git@…, https://…, ssh://…) is used as-is.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			remote := guessArchiveRemote(args[0])
			if remote != args[0] {
				fmt.Fprintf(out, "Resolved %q → %s\n", args[0], remote)
			}
			a, err := archive.Init(cmd.Context(), remote, name)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Archive initialized.\n  remote:      %s\n  machine dir: %s\n  local clone: %s\n\n",
				a.Remote(), a.Name(), a.ClonePath())
			fmt.Fprintln(out, archive.PrivacyWarning)
			fmt.Fprintln(out, "\nNext: `rawclaw archive push` uploads this machine's transcripts.")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "",
		"machine dir name in the archive (default: sanitized hostname)")
	return cmd
}

// newArchivePullCmd wires `rawclaw archive pull`: refresh the clone so other
// machines' pushed sessions become searchable here. The explicit verb always
// pulls; --throttle honors the stamp-file window (the variant background
// callers use, so a burst of invocations costs one network round-trip).
// Unconfigured is a clean no-op, not an error.
func newArchivePullCmd() *cobra.Command {
	var throttle bool
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull other machines' transcripts from the archive",
		Long: "Refresh the local archive clone from the remote, so sessions pushed by your " +
			"other machines become searchable here. A plain `rawclaw \"query\"` then covers " +
			"them automatically. A deleted or corrupt clone is re-cloned — unless it still " +
			"holds unpushed commits, which are never destroyed (the error names the recovery).",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := archive.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a == nil {
				fmt.Fprintln(out, "Archive not configured; run `rawclaw archive init <remote-url>` first. Nothing to do.")
				return nil
			}
			pulled, err := a.Pull(cmd.Context(), throttle)
			if errors.Is(err, archive.ErrBusy) {
				// A sibling process is mid-sync on this clone. A clean no-op,
				// not a failure — pull again once it finishes.
				fmt.Fprintln(out, "Archive pull skipped: another rawclaw sync is running on this machine; try again shortly.")
				return nil
			}
			if err != nil {
				return fmt.Errorf("archive pull: %w", err)
			}
			if pulled {
				fmt.Fprintf(out, "Archive refreshed from %s.\n", a.Remote())
			} else {
				fmt.Fprintln(out, "Archive pull skipped (pulled recently; --throttle honors the sync window).")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&throttle, "throttle", false,
		"skip the pull when one ran recently (for background/scripted callers; an explicit pull never needs this)")
	return cmd
}

// newArchivePushCmd wires `rawclaw archive push`: copy this machine's
// transcript trees into the clone, commit, and push (rebase-retry on
// concurrent pushers). Unconfigured is a clean no-op, not an error.
func newArchivePushCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "push",
		Short:         "Push this machine's transcripts to the archive",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := archive.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a == nil {
				fmt.Fprintln(out, "Archive not configured; run `rawclaw archive init <remote-url>` first. Nothing to do.")
				return nil
			}
			a.SetTagExporter(localTagExporter()) // ride this machine's tags along
			rep, err := a.PushLocal(cmd.Context())
			if errors.Is(err, archive.ErrBusy) {
				// Waited the lock's grace window and a sibling process still
				// holds the clone (timer, background sync, another shell).
				// Transcripts it hasn't seen are caught by the next sync —
				// a clean no-op, not a failure.
				fmt.Fprintln(out, "Archive push skipped: another rawclaw sync is running on this machine; re-run to push anything it misses.")
				return nil
			}
			if err != nil {
				return fmt.Errorf("archive push: %w", err)
			}
			switch {
			case !rep.Committed && rep.Pushed:
				// The stranded-commit recovery path: a previous run committed
				// and then died before its push landed. Say so — this run DID
				// change the remote.
				fmt.Fprintf(out, "Pushed a previously stranded sync to %s.\n", a.Remote())
			case !rep.Committed:
				fmt.Fprintln(out, "Archive up to date; nothing to push.")
			case rep.Retries > 0:
				fmt.Fprintf(out, "Pushed %d file(s)%s to %s (rebased over %d concurrent push(es)).\n",
					rep.Copied, removalNote(rep.Removed), a.Remote(), rep.Retries)
			default:
				fmt.Fprintf(out, "Pushed %d file(s)%s to %s.\n",
					rep.Copied, removalNote(rep.Removed), a.Remote())
			}
			return nil
		},
	}
}

// removalNote renders the delete-propagation half of a push report — empty
// when nothing was removed, so the common no-delete output stays unchanged.
func removalNote(removed int) string {
	if removed == 0 {
		return ""
	}
	return fmt.Sprintf(", removed %d deleted session(s)", removed)
}

// newArchiveStatusCmd wires `rawclaw archive status`: an offline report of
// where the archive lives, when this machine last pushed/pulled (with an
// overdue warning on aged own-sync stamps), and when each machine's dir last
// received new content. Unconfigured is a clean no-op, not an error.
func newArchiveStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "status",
		Short:         "Report archive state: remote, clone, last push/pull, last new content per machine",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := archive.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a == nil {
				fmt.Fprintln(out, "Archive not configured; run `rawclaw archive init <remote-url>` first. Nothing to do.")
				return nil
			}
			st, err := a.Status(cmd.Context())
			if err != nil {
				return fmt.Errorf("archive status: %w", err)
			}
			printArchiveStatus(out, st)
			return nil
		},
	}
}

// printArchiveStatus renders one StatusReport. Wording is deliberate: a
// machine dir's git history only says when NEW CONTENT last arrived — an
// idle-but-healthy machine and a dead one look identical from here — so
// per-machine lines report "last new content" with no staleness verdict. The
// overdue warning is reserved for this machine's OWN sync stamps, the one
// freshness fact known first-hand.
func printArchiveStatus(w io.Writer, st archive.StatusReport) {
	fmt.Fprintf(w, "Archive status\n  remote:      %s\n  local clone: %s", st.Remote, st.Clone)
	if !st.CloneOK {
		fmt.Fprint(w, " (missing; run `rawclaw archive pull`)")
	}
	fmt.Fprintf(w, "\n  last push:   %s%s\n  last pull:   %s%s\n",
		stampLabel(st.LastPush), overdueNote(st.PushOverdue),
		stampLabel(st.LastPull), overdueNote(st.PullOverdue))
	if len(st.Machines) == 0 {
		return
	}
	fmt.Fprintln(w, "Machines:")
	for _, m := range st.Machines {
		name := m.Name
		if m.Own {
			name += " (this machine)"
		}
		fmt.Fprintf(w, "  %-32s last new content %s\n", name, stampLabel(m.LastCommit))
	}
	// Surface cross-machine tag conflicts where a human already looks.
	// Deterministic winner is kept; every side's tag file is retained in the archive.
	if n := len(st.TagConflicts); n > 0 {
		fmt.Fprintf(w, "Tag conflicts: %d session(s) with disagreeing cross-machine tags (deterministic winner kept; all tag files retained):\n", n)
		for _, sid := range st.TagConflicts {
			fmt.Fprintf(w, "  %s\n", sid)
		}
	}
}

// overdueNote renders the own-sync overdue warning — empty when the stamp is
// fresh (or was never written; "never" is its own honest state).
func overdueNote(overdue bool) string {
	if !overdue {
		return ""
	}
	return "  (overdue: no successful sync in over a day — is the timer/autosync running?)"
}

// stampLabel renders a recorded time for status output; the zero time reads
// "never" (no stamp / no history yet). `archive status` is a human surface, so
// the timefmt seam renders local time WITH the zone abbreviation — an
// unmarked local stamp reads as ambiguous next to the marked-UTC agent
// surfaces.
func stampLabel(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return timefmt.Local(t)
}
