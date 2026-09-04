package cli

import (
	"fmt"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/spf13/cobra"
)

func parseWithFlags(withFlags []string, includeTools, includeSubagents bool) (tools, thinking, subagents bool, err error) {
	tools = includeTools
	subagents = includeSubagents
	for _, w := range withFlags {
		for part := range strings.SplitSeq(w, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			switch part {
			case "tools":
				tools = true
			case "thinking":
				thinking = true
			case "subagents":
				subagents = true
			default:
				return false, false, false, ExitError{Code: 2, Msg: fmt.Sprintf("invalid --with choice %q (valid: tools, thinking, subagents)", part)}
			}
		}
	}
	return tools, thinking, subagents, nil
}

// newReadCmd wires the top-level `rawclaw read <ref>` verb: a bounded excerpt
// around a search ref (<session8>:<uuid8>). Thin wrapper over agentproto.Read.
func newReadCmd() *cobra.Command {
	var (
		focus        string
		budget       int
		moreLevel    int
		around       int
		includeTools bool
		withFlags    []string
		thisProject  bool
		dir          string
		jsonOut      bool
	)
	cmd := &cobra.Command{
		Use:   "read <session8:uuid8>",
		Short: "Read a bounded excerpt around a search ref (--more to widen)",
		Long: "Read a bounded excerpt around a search ref taken from `rawclaw \"query\"` output.\n\n" +
			"The ref is <session8>:<uuid8> — copy it from a search hit. The excerpt is whole by default; " +
			"--budget N caps it, --more widens the window, --around N shifts it — all on the same ref.\n\n" +
			"The excerpt is indexed session history, not a guarantee of the live transcript's current tail; verify current state before acting. Human output adds a freshness note when applicable.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --budget omitted = no cap (nil); bare --budget = the default ceiling
			// (NoOptDefVal); --budget N = N. Detect "omitted" via Changed.
			var b *int
			if cmd.Flags().Changed("budget") {
				v := budget
				b = &v
			}
			tools, thinking, subagents, err := parseWithFlags(withFlags, includeTools, false)
			if err != nil {
				return err
			}
			window := 0
			if moreLevel > 0 {
				window = agentproto.MoreWindow(moreLevel)
			}
			scope, more := verbScope(cmd.Context(), thisProject, dir, cmd.Flags().Changed("dir"))
			ropts := agentproto.ReadOpts{
				Focus:            focus,
				Budget:           b,
				IncludeTools:     tools,
				IncludeThinking:  thinking,
				IncludeSubagents: subagents,
				Window:           window,
				Around:           around,
			}
			var (
				isStale bool
				sfNote  string
			)
			if con, _, err := index.OpenConsolidated(); err == nil {
				defer con.Close()
				sessionPrefix := agentproto.NormalizeSessionArg(args[0])
				if idx := strings.Index(sessionPrefix, ":"); idx >= 0 {
					sessionPrefix = sessionPrefix[:idx]
				}
				if sf, fErr := index.CheckSessionFreshness(con, sessionPrefix); fErr == nil {
					isStale, sfNote = sessionStaleNote(sf)
				}
			}
			if jsonOut {
				ropts.ScopeFallback = more
				res, err := agentproto.Read(args[0], scope, ropts)
				if err != nil {
					return err
				}
				return EmitJSON(cmd.OutOrStdout(), struct {
					*agentproto.ReadResult
					Stale     bool   `json:"stale,omitempty"`
					StaleNote string `json:"stale_note,omitempty"`
				}{
					ReadResult: res,
					Stale:      isStale,
					StaleNote:  sfNote,
				})
			}
			if err := agentproto.ReadAndRender(cmd.OutOrStdout(), args[0], scope, more, ropts, false); err != nil {
				return err
			}
			if sfNote != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "note: %s\n", sfNote)
			}
			maybeAutosync() // after the excerpt is printed; never before
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&focus, "focus", "", "highlight the first match of this term in the window")
	f.IntVar(&budget, "budget", agentproto.DefaultReadBudget, "cap the excerpt to N chars (omit for no cap)")
	f.IntVar(&moreLevel, "more", 0, "widen the window: --more 1 (1 level) or --more N (N levels)")
	f.IntVar(&around, "around", 0, "re-center the window N messages from the anchor")
	f.BoolVar(&includeTools, "include-tools", false, "include tool calls in the excerpt")
	f.StringSliceVar(&withFlags, "with", nil, "include extra content in the excerpt: tools, thinking, subagents (comma-separated or repeated)")
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	f.BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}

// newOutlineCmd wires the top-level `rawclaw outline <session8>` verb: a session's
// goal→resolution arc. Thin wrapper over agentproto.Outline.
func newOutlineCmd() *cobra.Command {
	var (
		includeTools     bool
		includeSubagents bool
		withFlags        []string
		thisProject      bool
		dir              string
		jsonOut          bool
	)
	cmd := &cobra.Command{
		Use:   "outline <session8>",
		Short: "Show a session's goal→resolution arc",
		Long: "Outline a session's arc — its opening goal and closing resolution — to decide where to read next. " +
			"Takes the 8-char session id from a search hit.\n\n" +
			"The arc is read from the indexed session record. It may lag a transcript that is still being written; verify current state before acting. A human-rendered result may add a staleness note when the index is behind the source.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, thinking, subagents, err := parseWithFlags(withFlags, includeTools, includeSubagents)
			if err != nil {
				return err
			}
			scope, more := verbScope(cmd.Context(), thisProject, dir, cmd.Flags().Changed("dir"))
			oopts := agentproto.OutlineOpts{
				IncludeTools:     tools,
				IncludeThinking:  thinking,
				IncludeSubagents: subagents,
			}
			var (
				isStale bool
				sfNote  string
			)
			if con, _, err := index.OpenConsolidated(); err == nil {
				defer con.Close()
				sessionPrefix := agentproto.NormalizeSessionArg(args[0])
				if sf, fErr := index.CheckSessionFreshness(con, sessionPrefix); fErr == nil {
					isStale, sfNote = sessionStaleNote(sf)
				}
			}
			if jsonOut {
				oopts.ScopeFallback = more
				res, err := agentproto.OutlineWith(args[0], scope, oopts)
				if err != nil {
					return err
				}
				return EmitJSON(cmd.OutOrStdout(), struct {
					*agentproto.OutlineResult
					Stale     bool   `json:"stale,omitempty"`
					StaleNote string `json:"stale_note,omitempty"`
				}{
					OutlineResult: res,
					Stale:         isStale,
					StaleNote:     sfNote,
				})
			}
			if err := agentproto.OutlineAndRender(cmd.OutOrStdout(), args[0],
				scope, more, oopts, false); err != nil {
				return err
			}
			if sfNote != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\nnote: %s\n", sfNote)
			}
			maybeAutosync() // after the arc is printed; never before
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&includeTools, "include-tools", false, "include tool calls in the arc")
	f.BoolVar(&includeSubagents, "include-subagents", false, "include subagent threads in the arc")
	f.StringSliceVar(&withFlags, "with", nil, "include extra sections in outline: tools, thinking, subagents (comma-separated or repeated)")
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	f.BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}

// runRoot dispatches the output shape in priority order:
// list → reindex-vectors → resume → stats → browse → (search).
