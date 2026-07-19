package cli

import (
	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/live"
	"github.com/spf13/cobra"
)

// newLiveCmd wires `rawclaw live <machine> [session-prefix]`: the direct-SSH
// live peek. It dials the machine (name → ssh destination via the archive
// config, default the name itself) and invokes the remote rawclaw's serving
// half — list recent sessions, or stream one in-progress transcript. One hop,
// seconds-fresh; the archive is never touched.
//
// The hidden --serve flag IS that serving half: what the client invokes on the
// far end (`rawclaw live --serve [prefix]`), reading this machine's transcript
// files directly.
func newLiveCmd() *cobra.Command {
	var (
		serve        bool
		limit        int
		tail         int
		includeTools bool
		jsonOut      bool
	)
	cmd := &cobra.Command{
		Use:   "live <machine> [session-prefix]",
		Short: "Peek at what a machine's agent is doing right now (direct SSH)",
		Long: "Peek at another machine's live sessions over SSH — seconds-fresh, one hop, no archive " +
			"round-trip.\n\n" +
			"  rawclaw live <machine>            list its recent sessions, newest first\n" +
			"  rawclaw live <machine> <prefix>   render that session's current transcript\n\n" +
			"<machine> resolves to an ssh destination via the archive config's optional " +
			"\"ssh\" map (e.g. {\"box-a\": \"user@10.0.0.5\"}); an unmapped name is used as the " +
			"destination itself, so an ~/.ssh/config Host alias just works. The far end needs " +
			"sshd and a rawclaw on its non-interactive PATH.",
		// Serve mode renders THIS machine (0-1 args); client mode dials one (1-2).
		Args: func(cmd *cobra.Command, args []string) error {
			if serve {
				return cobra.MaximumNArgs(1)(cmd, args)
			}
			return cobra.RangeArgs(1, 2)(cmd, args)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			// A flag on the wrong mode is a mistake worth a loud answer, not a
			// silent ignore: --tail shapes a session render, --limit a list.
			listing := (serve && len(args) == 0) || (!serve && len(args) == 1)
			if listing && cmd.Flags().Changed("tail") {
				return ExitError{Code: 2, Msg: "--tail applies to a session peek (pass a session prefix)"}
			}
			if listing && cmd.Flags().Changed("include-tools") {
				return ExitError{Code: 2, Msg: "--include-tools applies to a session peek (pass a session prefix)"}
			}
			if !listing && cmd.Flags().Changed("limit") {
				return ExitError{Code: 2, Msg: "--limit applies to the session list (drop the session prefix)"}
			}
			if serve {
				if len(args) == 0 {
					return live.ServeList(out, limit)
				}
				return live.ServeSession(out, args[0], tail, includeTools, jsonOut)
			}
			machine := args[0]
			dest, err := archive.SSHDestination(machine)
			if err != nil {
				return err
			}
			c := live.NewClient(machine, dest)
			if len(args) == 1 {
				return c.List(cmd.Context(), out, limit, jsonOut)
			}
			return c.Session(cmd.Context(), out, args[1], tail, includeTools, jsonOut)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&serve, "serve", false, "serve this machine's sessions (the half a live peek invokes remotely)")
	_ = f.MarkHidden("serve")
	f.IntVar(&limit, "limit", 0, "max sessions to list (default 10)")
	f.IntVar(&tail, "tail", 0, "trailing messages to render for a session (default 40)")
	f.BoolVar(&includeTools, "include-tools", false, "include tool calls in the session peek")
	f.BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}
