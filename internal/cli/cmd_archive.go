package cli

import (
	"fmt"

	"github.com/MoonCaves/rawclaw/internal/archive"
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
			"repository: transcripts contain whatever was pasted into sessions.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := archive.Init(cmd.Context(), args[0], name)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
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
			rep, err := a.PushLocal(cmd.Context())
			if err != nil {
				return fmt.Errorf("archive push: %w", err)
			}
			switch {
			case !rep.Committed:
				fmt.Fprintln(out, "Archive up to date; nothing to push.")
			case rep.Retries > 0:
				fmt.Fprintf(out, "Pushed %d file(s) to %s (rebased over %d concurrent push(es)).\n",
					rep.Copied, a.Remote(), rep.Retries)
			default:
				fmt.Fprintf(out, "Pushed %d file(s) to %s.\n", rep.Copied, a.Remote())
			}
			return nil
		},
	}
}
