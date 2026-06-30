package cli

import (
	"strings"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/spf13/cobra"
)

// newTopicsCmd wires the top-level `rawclaw topics "<query>"` verb: the on-demand
// topic finder. Topics are deliberately OUT of the default search ranking — this
// is the separate tool an agent reaches for only when a normal search is
// ambiguous. It searches ONLY the topic layer across scope (all projects by
// default, like search; --this-project / --dir to narrow), resolving each hit to
// a read-ref pointing at where the topic begins. Thin wrapper over
// agentproto.Topics — flag parsing only, no business logic.
func newTopicsCmd() *cobra.Command {
	var (
		limit       int
		thisProject bool
		dir         string
		includePath string
		jsonOut     bool
	)
	cmd := &cobra.Command{
		Use:   "topics <query>",
		Short: "Find tagged topics when a normal search is ambiguous (on-demand)",
		Long: "Search ONLY the topic layer — the concept labels a tagging subagent attached to past sessions — " +
			"and print, per hit, `<topic> · <project> · read ref=<sess8>:<uuid8>` pointing at where that topic " +
			"begins. Topics are NOT in the default search ranking; reach for this when a normal `rawclaw \"query\"` " +
			"is ambiguous. Searches every project by default; --this-project (with --dir) to narrow.",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.Join(args, " ")
			return agentproto.TopicsAndRender(cmd.OutOrStdout(), q, verbScope(thisProject, dir), limit, includePath, jsonOut)
		},
	}
	f := cmd.Flags()
	f.IntVar(&limit, "limit", 8, "max topic hits per project")
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	f.StringVar(&includePath, "include-path", "", "only search projects whose working dir matches this regex")
	f.BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}
