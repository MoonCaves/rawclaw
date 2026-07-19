package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/spf13/cobra"
)

// newArchiveEnableTimerCmd wires `rawclaw archive enable-timer [--eject]`: an
// hourly background `archive push` via the platform's user-level scheduler —
// a launchd agent on macOS, a systemd user timer on Linux. Never wired
// silently (this explicit verb is the only way in), and --eject removes
// exactly what was added: the registration plus rawclaw's own files, nothing
// else.
func newArchiveEnableTimerCmd() *cobra.Command {
	var eject bool
	cmd := &cobra.Command{
		Use:   "enable-timer",
		Short: "Install an hourly background `archive push` (launchd / systemd user timer)",
		Long: "Install an hourly `rawclaw archive push` under your user account, so this machine's " +
			"transcripts keep flowing to the archive even when you aren't running searches: a " +
			"launchd agent on macOS (~/Library/LaunchAgents), a systemd user timer on Linux " +
			"(~/.config/systemd/user). Re-running replaces rawclaw's own registration in place.\n\n" +
			"--eject removes exactly what enable-timer installed — the scheduler registration and " +
			"rawclaw's own files — and touches nothing else. Ejecting with nothing installed is a " +
			"clean no-op.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("resolve home dir: %w", err)
			}
			if eject {
				return ejectTimer(cmd.Context(), out, runtime.GOOS, home)
			}
			// An hourly push with no archive behind it would tick forever as a
			// no-op — refuse with the pointer instead of half-installing.
			// (--eject above deliberately skips this gate: symmetric removal
			// must work even after the archive config is gone.)
			a, err := archive.Load()
			if err != nil {
				return err
			}
			if a == nil {
				fmt.Fprintln(out, "Archive not configured; run `rawclaw archive init <remote-url>` first, then enable the timer.")
				return nil
			}
			return installTimer(cmd.Context(), out, runtime.GOOS, home)
		},
	}
	cmd.Flags().BoolVar(&eject, "eject", false,
		"remove exactly what enable-timer installed (registration + rawclaw's own files) instead of installing")
	return cmd
}
