// Package cli is the thin composition root: the cobra command tree, flag
// wiring, the flat-output printers, and the JSON emitters. The engine lives in
// the sibling packages (parse, paths, index, query, retrieve, view, render,
// semantic, adapters, agentproto).
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/adapters"
	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/query"
	"github.com/MoonCaves/rawclaw/internal/render"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/semantic"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Options holds every parsed flag for one rawclaw invocation, bound to the
// cobra root command.
type Options struct {
	Limit            int
	Dir              string
	ThisProject      bool
	All              bool
	List             bool
	Role             string
	Source           string
	Sort             string
	IncludeTools     bool
	IncludeSubagents bool
	Reindex          bool
	JSON             bool
	Resume           string
	Stats            bool
	Since            string
	Before           string
	NoVector         bool
	ReindexVectors   bool
	IncludePath      string
	ExcludePath      string
	MinMessages      int
	DebugSearch      bool
	Timeout          time.Duration
	DirSet           bool // --dir explicitly passed (the arbitrary-folder opt-in)
}

// params builds the retrieve.SearchParams the search shapes read, carrying the
// boolean→FTS5 raw-match expr (empty when the query has no operators, which
// takes the plain search path).
func (o *Options) params(rawMatch string) retrieve.SearchParams {
	return retrieve.SearchParams{
		Role:             o.Role,
		Sort:             o.Sort,
		IncludeTools:     o.IncludeTools,
		IncludeSubagents: o.IncludeSubagents,
		Since:            o.Since,
		Before:           o.Before,
		RawMatch:         rawMatch,
		MinMessages:      o.MinMessages,
	}
}

// BuildInfo carries the compile-time stamp (set in package main via ldflags)
// down into the command tree so `--version` and the `version` subcommand report
// the real release. The zero value is honest: an un-stamped build shows "dev".
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// versionString renders the one-line version banner shown by `--version` and the
// `version` subcommand. Empty fields fall back to "dev"/"unknown".
func (b BuildInfo) versionString() string {
	v, c, d := b.Version, b.Commit, b.Date
	if v == "" {
		v = "dev"
	}
	if c == "" {
		c = "unknown"
	}
	if d == "" {
		d = "unknown"
	}
	return fmt.Sprintf("rawclaw %s (commit: %s, built: %s)", v, c, d)
}

// NewRootCmd builds the rawclaw cobra command tree (root + the `read`, `outline`,
// `archive`, `live`, `delete`, `setup`, `upgrade`, and `version` subcommands). The root RunE
// dispatches the shape (browse/discovery/stats/resume/reindex-vectors) per the
// parsed flags. The build stamp feeds `--version` (cobra-native) and the
// `version` subcommand.
func NewRootCmd(build BuildInfo) *cobra.Command {
	opts := &Options{}

	root := &cobra.Command{
		Use:   "rawclaw [query...]",
		Short: "Search the Claude Code transcript record",
		Long: "Recall past Claude Code sessions without pasting whole transcripts.\n\n" +
			"  rawclaw \"natural query\"         ranked hits, each with a read-ref\n" +
			"  rawclaw read <sess8>:<uuid8>    bounded excerpt around a ref (--more to widen)\n" +
			"  rawclaw outline <sess8>         a session's goal -> resolution arc\n\n" +
			"Searches every project by default; --this-project (with --dir) or --include-path <regex> to scope. " +
			"Add --json for structured output. Results are raw session history — verify against current state before acting.\n\n" +
			"Retention: when a source tool purges a transcript (e.g. Claude Code's ~30-day cleanup), rawclaw KEEPS its " +
			"indexed copy — searchable and readable, labeled as retained history. `rawclaw delete` still removes a " +
			"session permanently. Set RAWCLAW_RETENTION=mirror to instead drop sessions whose source file is gone. " +
			"Mirror governs live project scans only; history already retained is removed by `rawclaw delete` alone, " +
			"never as a side effect of a search.",
		// Cobra wires a `--version` flag automatically when Version is non-empty,
		// printing this template and exiting 0.
		Version:       build.versionString(),
		SilenceUsage:  true,
		SilenceErrors: true,
		// Positional args are the query terms; any count is valid (no query = browse).
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --this-desk: hidden backward-compat alias for --this-project.
			if cmd.Flags().Changed("this-desk") {
				opts.ThisProject = true
			}
			// An explicit --dir is the opt-in that lets an arbitrary
			// jsonl-bearing folder resolve as a transcripts dir; the cwd
			// default never is (folder guard).
			opts.DirSet = cmd.Flags().Changed("dir")
			return runRoot(cmd, opts, args)
		},
	}

	f := root.Flags()
	f.IntVar(&opts.Limit, "limit", 8, "max hits to return")
	f.StringVar(&opts.Dir, "dir", cwd(),
		"the project's working directory (e.g. ~/code/my-project); encoded to "+
			"find its transcripts. An already-encoded ~/.claude/projects path also works.")
	f.BoolVar(&opts.ThisProject, "this-project", false, "narrow to THIS project only (default searches all projects)")
	f.Bool("this-desk", false, "") // hidden backward-compat alias for --this-project
	_ = f.MarkHidden("this-desk")
	f.BoolVar(&opts.All, "all", false, "cover every project: the search default already, and the widener for bare browse and --stats")
	f.BoolVar(&opts.List, "list", false, "list all searchable projects (with session counts) and exit")
	f.StringVar(&opts.Role, "role", "", "only this author role (user|assistant)")
	f.StringVar(&opts.Source, "source", "", "only this runtime (claude|codex); default searches all")
	f.StringVar(&opts.Sort, "sort", "", "result order (newest|oldest)")
	f.BoolVar(&opts.IncludeTools, "include-tools", false, "also match/show tool calls + tool-only hits")
	f.BoolVar(&opts.IncludeSubagents, "include-subagents", false, "also search delegated subagent threads")
	f.BoolVar(&opts.Reindex, "reindex", false, "force a full re-index before searching")
	f.BoolVar(&opts.JSON, "json", false, "machine-readable JSON output (for agents/scripts)")
	f.StringVar(&opts.Resume, "resume", "", "print the paste-ready `claude --resume` command for a session id (use the 8-char id from search output)")
	f.BoolVar(&opts.Stats, "stats", false, "corpus overview (sessions/messages/date span) for this project, or --all for every project")
	f.StringVar(&opts.Since, "since", "", "only results on/after this date")
	f.StringVar(&opts.Before, "before", "", "only results on/before this date")
	f.BoolVar(&opts.NoVector, "no-vector", false, "force keyword-only (ignore any configured embedder)")
	f.BoolVar(&opts.ReindexVectors, "reindex-vectors", false, "build/update the semantic index for the scope (needs RAWCLAW_EMBED_ENDPOINT)")
	f.StringVar(&opts.IncludePath, "include-path", "", "only search projects whose working dir matches this regex")
	f.StringVar(&opts.ExcludePath, "exclude-path", "", "skip projects whose working dir matches this regex (e.g. /tmp, test)")
	f.IntVar(&opts.MinMessages, "min-messages", 0, "only sessions with >= N messages (drops thin/bootstrap threads)")
	f.BoolVar(&opts.DebugSearch, "debug-search", false, "explain WHY each hit ranked where it did (LLM-free scoring breakdown)")
	_ = f.MarkHidden("debug-search")

	// --timeout is PERSISTENT (every subcommand inherits it): rawclaw must be
	// self-bounding so an agent never needs an external `timeout(1)`. Default 30s;
	// RAWCLAW_TIMEOUT overrides the default; --timeout 0 disables the watchdog.
	// The watchdog itself is armed in Execute (which wraps root.Execute) so it is
	// disarmed on EVERY path — including a command that returns an error, where
	// cobra would skip a PersistentPostRunE hook.
	root.PersistentFlags().DurationVar(&opts.Timeout, "timeout", defaultTimeout,
		"hard wall-clock deadline for the whole run; exits 124 if exceeded (0 disables; env RAWCLAW_TIMEOUT)")

	// Validate the role/sort enums before running: reject anything outside the
	// allowed set with an "invalid choice" message (stderr + exit 2), keeping the
	// validation in cobra's pre-run hook.
	root.PreRunE = func(cmd *cobra.Command, args []string) error {
		if err := validateChoice("role", opts.Role, "user", "assistant"); err != nil {
			return err
		}
		return validateChoice("sort", opts.Sort, "newest", "oldest")
	}

	// `--version` prints the banner verbatim (cobra's default template prefixes
	// "{{.Name}} version", which would double the "rawclaw").
	root.SetVersionTemplate("{{.Version}}\n")

	root.AddCommand(newReadCmd())
	root.AddCommand(newOutlineCmd())
	root.AddCommand(newTopicsCmd())
	root.AddCommand(newTagPrepCmd())
	root.AddCommand(newTagWriteCmd())
	archiveCmd := newArchiveCmd()
	archiveCmd.AddCommand(newArchiveInitCmd())
	archiveCmd.AddCommand(newArchivePushCmd())
	archiveCmd.AddCommand(newArchivePullCmd())
	archiveCmd.AddCommand(newArchiveStatusCmd())
	archiveCmd.AddCommand(newArchiveAutosyncCmd())
	archiveCmd.AddCommand(newArchiveEnableTimerCmd())
	root.AddCommand(archiveCmd)
	root.AddCommand(newLiveCmd())
	root.AddCommand(newDeleteCmd())
	root.AddCommand(newSetupCmd())
	root.AddCommand(newUpgradeCmd(build))
	root.AddCommand(newVersionCmd(build))
	return root
}

// Execute runs the command tree under the self-bounding watchdog. It resolves the
// effective deadline from --timeout / RAWCLAW_TIMEOUT (a lenient pre-parse, so the
// watchdog is armed BEFORE cobra dispatches — covering even a slow PreRun), arms
// the watchdog, then runs root.Execute. The disarm is deferred, so the watchdog
// goroutine is always torn down — on success, on a returned error, or on a panic —
// which keeps the goroutine-leak detector green. main calls this instead of
// root.Execute() directly.
func Execute(root *cobra.Command, args []string) error {
	to := resolveTimeoutFromArgs(args, os.Getenv("RAWCLAW_TIMEOUT"))
	ctx, stop := startWatchdog(to, root.ErrOrStderr(), osExit)
	defer stop()
	root.SetArgs(args)
	// The watchdog's context is the run's context: when the deadline fires it
	// cancels every command — and kills any child started for the run via
	// exec.CommandContext — so a child doesn't outlive the exit(124).
	return root.ExecuteContext(ctx)
}

// resolveTimeoutFromArgs leniently parses just the --timeout value out of args
// (ignoring unknown flags / parse errors) so the watchdog can arm before cobra's
// own parse. Falls back to RAWCLAW_TIMEOUT, then the default.
//
// Special case — `upgrade`/`update`: the self-update path makes up to three serial
// network legs bounded individually by netTimeout (60s each), which the 30s default
// watchdog would kill mid-download. So when the user has NOT explicitly chosen a
// timeout (no --timeout flag, no RAWCLAW_TIMEOUT), the watchdog floor for an upgrade
// is raised to upgradeWatchdog (> the worst-case sum of the legs) — preserving the
// never-hang guarantee (the per-leg netTimeouts still bound the run) while letting a
// legitimate download finish. An explicit --timeout / RAWCLAW_TIMEOUT always wins,
// including `--timeout 0` to disable the watchdog entirely.
func resolveTimeoutFromArgs(args []string, env string) time.Duration {
	probe := pflag.NewFlagSet("rawclaw-timeout-probe", pflag.ContinueOnError)
	probe.ParseErrorsWhitelist.UnknownFlags = true
	probe.SetOutput(io.Discard)
	to := probe.Duration("timeout", defaultTimeout, "")
	_ = probe.Parse(args)

	flagSet := probe.Changed("timeout")
	resolved := resolveTimeout(flagSet, *to, env)

	// Only override when the user expressed no preference at all: an explicit
	// flag or env var is authoritative even for upgrade/archive.
	if !flagSet && env == "" {
		if isUpgradeInvocation(args) && resolved < upgradeWatchdog {
			return upgradeWatchdog
		}
		// The syncing archive verbs (init/push/pull/autosync) run WITHOUT the
		// wall-clock watchdog: no cap fits both a hung transfer and a legit
		// slow multi-GB first push, so — like rsync's --timeout and curl's
		// --speed-time — a hang is caught by STALL detection on the git
		// children instead (http.lowSpeedLimit/Time + ssh keepalives; see
		// archive's git runner). A stalled transfer dies in ~30-60s; a
		// slow-but-moving one runs as long as it keeps moving.
		if isArchiveSyncInvocation(args) {
			return 0
		}
	}
	return resolved
}

// rootValueFlags are the root command's value-taking flags whose
// space-separated value could precede the subcommand token — the lenient
// pre-parse scanners must consume the value so it isn't mistaken for a
// subcommand (`--dir archive pull` is a search, not `archive pull`). Flags
// with `=` carry their own value. Keep in sync with the root flag set.
var rootValueFlags = map[string]bool{
	"--timeout": true, "--dir": true, "--limit": true, "--role": true,
	"--source": true, "--sort": true, "--resume": true, "--since": true,
	"--before": true, "--include-path": true, "--exclude-path": true,
	"--min-messages": true,
}

// leadingSubcommandTokens returns up to n leading non-flag tokens of args —
// the (sub)command path a cobra dispatch would see. Flags are skipped, the
// values of rootValueFlags consumed, and scanning STOPS at `--`: cobra treats
// everything after it as positional args, never as a subcommand, so a token
// there must not steer the watchdog.
func leadingSubcommandTokens(args []string, n int) []string {
	out := []string{}
	for i := 0; i < len(args) && len(out) < n; i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			if rootValueFlags[a] && i+1 < len(args) {
				i++ // consume the flag's space-separated value
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

// isUpgradeInvocation reports whether args target the `upgrade` (alias
// `update`) subcommand — the first non-flag token.
func isUpgradeInvocation(args []string) bool {
	w := leadingSubcommandTokens(args, 1)
	return len(w) == 1 && (w[0] == "upgrade" || w[0] == "update")
}

// isArchiveSyncInvocation reports whether args target a SYNCING archive verb —
// `archive init|push|pull|autosync` — the ones that talk to the git remote and
// run stall-bounded instead of wall-clock-bounded. `archive
// status`/`enable-timer`/`archive <session>` (the local move) keep the
// default watchdog.
func isArchiveSyncInvocation(args []string) bool {
	w := leadingSubcommandTokens(args, 2)
	if len(w) < 2 || w[0] != "archive" {
		return false
	}
	switch w[1] {
	case "init", "push", "pull", "autosync":
		return true
	}
	return false
}

// newVersionCmd wires `rawclaw version`: print the build stamp (same banner as
// the cobra-native `--version` flag) plus the embedding Go toolchain version.
func newVersionCmd(build BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:           "version",
		Short:         "Print version information",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, build.versionString())
			if info, ok := debug.ReadBuildInfo(); ok {
				fmt.Fprintf(out, "go: %s\n", info.GoVersion)
			}
			return nil
		},
	}
}

// verbScope resolves the scope for the read/outline verbs: the full
// all-projects enumeration unless --this-project, in which case the single
// cwd/--dir project — or an explicit empty scope when this-project is asked
// but the dir has no transcript history (so it resolves nothing rather than
// silently going wide). dirSet marks an explicit --dir (the arbitrary-folder
// opt-in resolveTDir honors).
func verbScope(ctx context.Context, thisProject bool, dir string, dirSet bool) []view.Scope {
	if !thisProject {
		// All-projects: built HERE (not via agentproto's nil-scope fallback) so
		// the archive enumeration's git probes run under the run's watchdog ctx.
		return allScope(ctx, "", false)
	}
	td := resolveTDir(dir, dirSet)
	if td == "" || !isDir(td) {
		return []view.Scope{}
	}
	return []view.Scope{{Project: paths.ProjectLabel(td), TDir: td}}
}

// resolveTDir maps a --dir value to its transcripts dir. Only an EXPLICIT
// --dir may resolve an arbitrary jsonl-bearing folder (the /tmp folder guard:
// implicit cwd discovery is location-based only).
func resolveTDir(dir string, explicit bool) string {
	if explicit {
		return paths.FindTranscriptDirExplicit(dir)
	}
	return paths.FindTranscriptDir(dir)
}

// newReadCmd wires the top-level `rawclaw read <session8:uuid8>` verb: a bounded,
// expand-in-place excerpt around a search ref. The agent-native read path,
// promoted out of the `agent` subcommand into its own verb. Thin wrapper over
// agentproto.Read — flag parsing only, no business logic.
func newReadCmd() *cobra.Command {
	var (
		focus        string
		budget       int
		moreLevel    int
		around       int
		includeTools bool
		thisProject  bool
		dir          string
		jsonOut      bool
	)
	cmd := &cobra.Command{
		Use:   "read <session8:uuid8>",
		Short: "Read a bounded excerpt around a search ref (--more to widen)",
		Long: "Read a bounded excerpt around a search ref taken from `rawclaw \"query\"` output.\n\n" +
			"The ref is <session8>:<uuid8> — copy it from a search hit. The excerpt is whole by default; " +
			"--budget N caps it, --more widens the window, --around N shifts it — all on the same ref.",
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
			if err := agentproto.ReadAndRender(cmd.OutOrStdout(), args[0],
				verbScope(cmd.Context(), thisProject, dir, cmd.Flags().Changed("dir")),
				focus, b, includeTools, moreLevel, around, jsonOut); err != nil {
				return err
			}
			maybeAutosync() // after the excerpt is printed; never before
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&focus, "focus", "", "highlight the first match of this term in the window")
	f.IntVar(&budget, "budget", agentproto.DefaultReadBudget, "cap the excerpt to N chars (omit for no cap)")
	f.Lookup("budget").NoOptDefVal = strconv.Itoa(agentproto.DefaultReadBudget)
	f.IntVar(&moreLevel, "more", 0, "widen the window: --more (1 level) or --more=N (N levels)")
	f.Lookup("more").NoOptDefVal = "1"
	f.IntVar(&around, "around", 0, "re-center the window N messages from the anchor")
	f.BoolVar(&includeTools, "include-tools", false, "include tool calls in the excerpt")
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	f.BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}

// newOutlineCmd wires the top-level `rawclaw outline <session8>` verb: a session's
// goal→resolution arc. Thin wrapper over agentproto.Outline.
func newOutlineCmd() *cobra.Command {
	var (
		includeTools bool
		thisProject  bool
		dir          string
		jsonOut      bool
	)
	cmd := &cobra.Command{
		Use:   "outline <session8>",
		Short: "Show a session's goal→resolution arc",
		Long: "Outline a session's arc — its opening goal and closing resolution — to decide where to read next. " +
			"Takes the 8-char session id from a search hit.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := agentproto.OutlineAndRender(cmd.OutOrStdout(), args[0],
				verbScope(cmd.Context(), thisProject, dir, cmd.Flags().Changed("dir")), includeTools, jsonOut); err != nil {
				return err
			}
			maybeAutosync() // after the arc is printed; never before
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&includeTools, "include-tools", false, "include tool calls in the arc")
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	f.BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}

// runRoot dispatches the output shape in priority order:
// list → reindex-vectors → resume → stats → browse → (search).
func runRoot(cmd *cobra.Command, o *Options, args []string) error {
	out := cmd.OutOrStdout()
	ctx := cmd.Context()

	if err := validateChoice("source", o.Source, "claude", "codex"); err != nil {
		return err
	}

	if o.List {
		ListProjects(out)
		return nil
	}

	if o.ReindexVectors {
		return runReindexVectors(ctx, out, o)
	}

	if o.Resume != "" {
		return runResume(out, o)
	}

	if o.Stats {
		return runStats(ctx, out, o)
	}

	if len(args) == 0 {
		return runBrowse(ctx, out, o)
	}

	// Empty (or all-whitespace) query: a distinct coaching line, NOT the
	// no-matches coaching — `rawclaw ""` asked for a search it never spelled.
	// Under --json the same coaching ships as JSON, like every sibling shape.
	if strings.TrimSpace(strings.Join(args, " ")) == "" {
		const emptyQueryHint = "Add a search term, or run bare rawclaw to browse this folder (--all for every project)."
		if o.JSON {
			return EmitJSON(out, struct {
				Error string `json:"error"`
				Hint  string `json:"hint"`
			}{"empty query", emptyQueryHint})
		}
		fmt.Fprintln(out, "Empty query. "+emptyQueryHint)
		return nil
	}

	if err := runSearch(ctx, out, o, args); err != nil {
		return err
	}
	// Results are already printed; the sync-on-invoke trigger fires last so
	// the search never waits on it (and a failed search never syncs).
	maybeAutosync()
	return nil
}

// thisScope resolves --dir to its transcript dir; on miss, it prints the
// "No transcript history" hint (naming both escapes: --list to see the
// projects, --all to cover every project) and returns ok=false.
func thisScope(w io.Writer, o *Options) (scope []view.Scope, td string, ok bool) {
	td = resolveTDir(o.Dir, o.DirSet)
	if td == "" || !isDir(td) {
		fmt.Fprintf(w, "No transcript history for --dir %s. Try --list, or --all for every project.\n", realpathExpand(o.Dir))
		return nil, "", false
	}
	return []view.Scope{{Project: paths.ProjectLabel(td), TDir: td}}, td, true
}

// allScope builds the search scope spanning the requested runtime(s) — Claude
// projects and/or Codex cwd-groups — via the scopes enumerator. source ""
// spans all; "claude"/"codex" narrows. ctx (the run's watchdog context)
// bounds the archive enumeration's git probes.
func allScope(ctx context.Context, source string, reindex bool) []view.Scope {
	return scopes.All(ctx, source, reindex)
}

// runReindexVectors builds/updates the semantic index for the scope.
func runReindexVectors(ctx context.Context, w io.Writer, o *Options) error {
	if !index.FTS5OK() {
		fmt.Fprintln(w, "--reindex-vectors needs FTS5.")
		return nil
	}
	emb := adapters.GetEmbedder()
	if emb == nil {
		fmt.Fprintln(w, "No embedder configured. Set RAWCLAW_EMBED_ENDPOINT (+ RAWCLAW_EMBED_MODEL), e.g.\n"+
			"  export RAWCLAW_EMBED_ENDPOINT=http://localhost:11434/api/embeddings\n"+
			"  export RAWCLAW_EMBED_MODEL=nomic-embed-text")
		return nil
	}

	var scope []view.Scope
	if o.ThisProject {
		sc, _, ok := thisScope(w, o)
		if ok {
			scope = sc
		}
	} else {
		scope = allScope(ctx, o.Source, o.Reindex)
	}

	total := 0
	for _, s := range scope {
		n, err := reindexOne(s, emb)
		if err != nil {
			fmt.Fprintf(w, "  %s: skipped (%s)\n", s.Project, err)
			continue
		}
		total += n
		if n > 0 {
			fmt.Fprintf(w, "  %s: +%d vectors\n", s.Project, n)
		}
	}
	fmt.Fprintf(w, "\nSemantic index updated: +%d new vectors. Run a normal search to use it (RRF-fused).\n", total)
	return nil
}

// reindexOne indexes a scope then refreshes its vectors
// (resolve db → open read-write → vector index → close). Works for any source:
// a Claude scope ensures its TDir, a Codex scope uses its pre-built db.
func reindexOne(sc view.Scope, emb embed.Embedder) (int, error) {
	dbp, _, err := scopes.Resolve(sc, false)
	if err != nil {
		return 0, err
	}
	con, err := store.ConnectRW(dbp)
	if err != nil {
		return 0, err
	}
	defer con.Close()
	return semantic.VecIndex(con, emb, 0)
}

// runResume prints the paste-ready resume command for a session id — `claude
// --resume <id>` for a Claude session, `codex resume <id>` for a Codex one. It
// resolves against Claude projects first, then falls back to the Codex scopes,
// then to the archive replicas: a session recorded on ANOTHER machine can't be
// resumed here, so the hint degrades to the command to run on that machine.
func runResume(w io.Writer, o *Options) error {
	hits := paths.ResolveSession(o.Resume)
	src := "claude"
	if len(hits) == 0 {
		hits = codexResumeHits(o.Resume)
		src = "codex"
	}
	if len(hits) == 0 {
		if handled, err := resumeForeign(w, o); handled {
			return err
		}
		fmt.Fprintf(w, "No session id starts with '%s'. Use the 8-char id from search output, e.g. [… · a1b2c3d4 · …].\n", o.Resume)
		return nil
	}
	if len(hits) > 1 {
		if o.JSON {
			type row struct {
				SessionID string `json:"session_id"`
				CWD       string `json:"cwd"`
				Project   string `json:"project"`
			}
			rows := make([]row, 0, len(hits))
			for _, h := range hits {
				rows = append(rows, row{h.SessionID, h.CWD, h.Project})
			}
			return EmitJSON(w, rows)
		}
		fmt.Fprintf(w, "%d sessions match '%s' — narrow it:\n", len(hits), o.Resume)
		for _, h := range hits {
			fmt.Fprintf(w, "  %s  (%s)\n", h.SessionID, h.Project)
		}
		return nil
	}

	h := hits[0]
	cmd := resumeCommand(src, h)
	if o.JSON {
		return EmitJSON(w, struct {
			SessionID string `json:"session_id"`
			CWD       string `json:"cwd"`
			Project   string `json:"project"`
			Command   string `json:"command"`
		}{h.SessionID, h.CWD, h.Project, cmd})
	}
	fmt.Fprintf(w, "Resume this session (%s):\n\n  %s\n", h.Project, cmd)
	return nil
}

// resumeCommand builds the paste-ready resume command for a session, per source:
// Claude uses `claude --resume`, Codex uses `codex resume`; both prefix a `cd`
// when the working dir is known.
func resumeCommand(src string, h paths.SessionHit) string {
	verb := "claude --resume " + h.SessionID
	if src == "codex" {
		verb = "codex resume " + h.SessionID
	}
	if h.CWD != "" {
		return "cd " + h.CWD + " && " + verb
	}
	return verb
}

// foreignHit is one archive-replica resume match: a session recorded on
// another machine, with everything the degrade message names.
type foreignHit struct {
	sessionID string
	machine   string // the owning machine's display name
	cwd       string // working dir recorded on THAT machine
	project   string
	source    string // "claude" | "codex" — picks the remote-side verb
}

// resumeForeign is the archive fallback for --resume: a session that only
// exists in another machine's archived dir cannot be resumed on this box (the
// runtime's own session state lives there), so instead of a runnable local
// command the hint degrades — clearly — to the machine's name and the command
// to run on it. Returns handled=false when the archive has no match either,
// letting the caller print the ordinary not-found hint.
func resumeForeign(w io.Writer, o *Options) (handled bool, err error) {
	hits := archiveResumeHits(o.Resume)
	if len(hits) == 0 {
		return false, nil
	}
	if len(hits) > 1 {
		if o.JSON {
			type row struct {
				SessionID string `json:"session_id"`
				Machine   string `json:"machine"`
				CWD       string `json:"cwd"`
				Project   string `json:"project"`
			}
			rows := make([]row, 0, len(hits))
			for _, h := range hits {
				rows = append(rows, row{h.sessionID, h.machine, h.cwd, h.project})
			}
			return true, EmitJSON(w, rows)
		}
		fmt.Fprintf(w, "%d sessions match '%s' on other machines — narrow it:\n", len(hits), o.Resume)
		for _, h := range hits {
			fmt.Fprintf(w, "  %s  (%s)\n", h.sessionID, h.project)
		}
		return true, nil
	}

	h := hits[0]
	remote := resumeCommand(h.source, paths.SessionHit{SessionID: h.sessionID, CWD: h.cwd, Project: h.project})
	if o.JSON {
		return true, EmitJSON(w, struct {
			SessionID string `json:"session_id"`
			Machine   string `json:"machine"`
			CWD       string `json:"cwd"`
			Project   string `json:"project"`
			Command   string `json:"command"`
			Note      string `json:"note"`
		}{h.sessionID, h.machine, h.cwd, h.project, remote,
			"session recorded on another machine; the command must run there"})
	}
	fmt.Fprintf(w, "Session %s was recorded on another machine ('%s') — it can only be resumed there.\n", h.sessionID, h.machine)
	fmt.Fprintf(w, "On %s, run:\n\n  %s\n", h.machine, remote)
	return true, nil
}

// archiveResumeHits resolves a session-id prefix against the archive replica
// scopes (other machines' sessions). Only top-level sessions are offered,
// matching the local resume paths. The lookup opens only the replica cache
// dbs EARLIER searches already built (scopes.ArchiveLookup) — re-walking and
// ingesting every foreign tree just to answer a prefix miss would be far too
// heavy, so a session pulled but never yet covered by a search resolves to
// the ordinary not-found hint until one runs.
func archiveResumeHits(prefix string) []foreignHit {
	var out []foreignHit
	for _, sc := range scopes.ArchiveLookup() {
		con, err := store.ConnectRO(sc.DBP)
		if err != nil {
			continue
		}
		ids, qerr := store.SessionsByPrefix(con, prefix, false, 3)
		_ = con.Close()
		if qerr != nil {
			continue
		}
		for _, id := range ids {
			out = append(out, foreignHit{
				sessionID: id,
				machine:   sc.OriginName,
				cwd:       sc.CWD,
				project:   sc.Project,
				source:    sc.Source,
			})
		}
	}
	return out
}

// codexResumeHits resolves a session-id prefix against the Codex scope dbs. A
// Codex session's cwd is its scope's cwd. Only top-level sessions are offered
// (is_subagent=0), matching ResolveSession's Claude behavior.
func codexResumeHits(prefix string) []paths.SessionHit {
	var out []paths.SessionHit
	for _, sc := range scopes.Codex(false) {
		con, err := store.ConnectRO(sc.DBP)
		if err != nil {
			continue
		}
		ids, qerr := store.SessionsByPrefix(con, prefix, false, 3)
		_ = con.Close()
		if qerr != nil {
			continue
		}
		for _, id := range ids {
			out = append(out, paths.SessionHit{SessionID: id, CWD: sc.CWD, Project: sc.Project})
		}
	}
	return out
}

// statsJSON is one project's stats record, in emit order.
type statsJSON struct {
	Sessions  int    `json:"sessions"`
	Subagents int    `json:"subagents"`
	Messages  int    `json:"messages"`
	User      int    `json:"user"`
	Assistant int    `json:"assistant"`
	First     string `json:"first"`
	Last      string `json:"last"`
}

func toStatsJSON(s store.CorpusStats) statsJSON {
	return statsJSON{s.Sessions, s.Subagents, s.Messages, s.User, s.Assistant, s.First, s.Last}
}

// runStats prints the corpus overview for this project, or the all-projects aggregate
// under --all.
func runStats(ctx context.Context, w io.Writer, o *Options) error {
	if !index.FTS5OK() {
		fmt.Fprintln(w, "--stats needs FTS5.")
		return nil
	}

	if (o.All || o.Source != "") && !o.ThisProject {
		return runStatsFleet(ctx, w, o)
	}

	sc, td, ok := thisScope(w, o)
	if !ok {
		return nil
	}
	_ = sc
	dbp, _, _, err := index.EnsureIndexed(td, o.Reindex)
	if err != nil {
		return fmt.Errorf("stats ensure-indexed: %w", err)
	}
	s, err := store.GetCorpusStats(dbp)
	if err != nil {
		return fmt.Errorf("stats corpus: %w", err)
	}
	if o.JSON {
		return EmitJSON(w, struct {
			Scope   string `json:"scope"`
			Project string `json:"project"`
			statsJSON
		}{"project", paths.ProjectLabel(td), toStatsJSON(s)})
	}
	fmt.Fprintf(w, "%s — session stats\n\n", paths.ProjectLabel(td))
	fmt.Fprintf(w, "  sessions   %d  (+%d subagent threads)\n", s.Sessions, s.Subagents)
	fmt.Fprintf(w, "  messages   %d  (%d user / %d assistant)\n", s.Messages, s.User, s.Assistant)
	fmt.Fprintf(w, "  span       %s → %s\n", orQ(s.First), orQ(s.Last))
	return nil
}

// projectStat is a per-project stats row carrying its project label.
type projectStat struct {
	statsJSON
	Project string `json:"project"`
}

// runStatsFleet computes and prints the --all stats aggregate across all projects.
func runStatsFleet(ctx context.Context, w io.Writer, o *Options) error {
	tot := store.CorpusStats{}
	nProjects := 0
	var per []projectStat

	for _, sc := range allScope(ctx, o.Source, o.Reindex) {
		dbp, _, err := scopes.Resolve(sc, o.Reindex)
		if err != nil {
			continue
		}
		s, err := store.GetCorpusStats(dbp)
		if err != nil {
			continue
		}
		nProjects++
		tot.Sessions += s.Sessions
		tot.Subagents += s.Subagents
		tot.Messages += s.Messages
		tot.User += s.User
		tot.Assistant += s.Assistant
		if s.First != "" && (tot.First == "" || s.First < tot.First) {
			tot.First = s.First
		}
		if s.Last != "" && s.Last > tot.Last {
			tot.Last = s.Last
		}
		per = append(per, projectStat{toStatsJSON(s), sc.Project})
	}

	if o.JSON {
		type totalJSON struct {
			Projects int `json:"projects"`
			statsJSON
		}
		return EmitJSON(w, struct {
			Scope    string        `json:"scope"`
			Total    totalJSON     `json:"total"`
			Projects []projectStat `json:"projects"`
		}{"all", totalJSON{nProjects, toStatsJSON(tot)}, per})
	}

	fmt.Fprintf(w, "RawClaw corpus — %d projects\n\n", nProjects)
	fmt.Fprintf(w, "  sessions   %d  (+%d subagent threads)\n", tot.Sessions, tot.Subagents)
	fmt.Fprintf(w, "  messages   %d  (%d user / %d assistant)\n", tot.Messages, tot.User, tot.Assistant)
	fmt.Fprintf(w, "  span       %s → %s\n", orQ(tot.First), orQ(tot.Last))
	return nil
}

// runBrowse handles the no-query case: list recent sessions for this project,
// or — under --all — for every project (same scope enumeration search uses).
// An explicit --this-project wins over --all, same precedence runStats applies.
func runBrowse(ctx context.Context, w io.Writer, o *Options) error {
	if o.All && !o.ThisProject {
		return runBrowseAll(ctx, w, o)
	}
	sc, td, ok := thisScope(w, o)
	if !ok {
		return nil
	}
	_ = sc
	rows := view.Browse(td, o.Limit, o.Since, o.Before)
	if o.JSON {
		return EmitJSON(w, struct {
			Project  string           `json:"project"`
			Sessions []view.BrowseRow `json:"sessions"`
		}{paths.ProjectLabel(td), rows})
	}
	render.PrintBrowse(w, rows, paths.ProjectLabel(td))
	return nil
}

// runBrowseAll is the --all shape of the no-query browse: recent sessions
// across every project (Claude + Codex + retained scopes — the same
// enumeration search uses), merged newest-first and capped at --limit.
func runBrowseAll(ctx context.Context, w io.Writer, o *Options) error {
	rows := []view.BrowseAllRow{} // non-nil so --json emits [] rather than null
	for _, sc := range allScope(ctx, o.Source, o.Reindex) {
		dbp, _, err := scopes.Resolve(sc, o.Reindex)
		if err != nil {
			continue // an unresolvable scope can't contribute rows; others still can
		}
		for _, r := range view.BrowseDB(dbp, o.Limit, o.Since, o.Before) {
			rows = append(rows, view.BrowseAllRow{Project: sc.Project, BrowseRow: r})
		}
	}
	// Newest-first across projects; each scope contributed at most --limit rows,
	// so the merge only has to re-sort and cap.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].LastTS > rows[j].LastTS })
	if len(rows) > o.Limit {
		rows = rows[:o.Limit]
	}
	if o.JSON {
		return EmitJSON(w, struct {
			Scope    string              `json:"scope"`
			Sessions []view.BrowseAllRow `json:"sessions"`
		}{"all", rows})
	}
	render.PrintBrowseAll(w, rows)
	return nil
}

// runSearch dispatches a query to the FALLBACK / BRIEF / DISCOVERY shapes.
func runSearch(ctx context.Context, w io.Writer, o *Options, args []string) error {
	q := strings.Join(args, " ")
	ftsExpr, usedOps := query.BooleanToFTS5(q)
	rawMatch := ""
	if usedOps {
		rawMatch = ftsExpr // no operators → leave empty for the plain search path
	}
	var ppred func(cwd string) bool
	if o.IncludePath != "" || o.ExcludePath != "" {
		ppred = query.PathPredicate(o.IncludePath, o.ExcludePath)
	}
	p := o.params(rawMatch)

	// FTS5 absent → linear fallback (this project, flat). Rarely taken in practice.
	if !index.FTS5OK() {
		sc, td, ok := thisScope(w, o)
		if !ok {
			return nil
		}
		_ = sc
		res := retrieve.LinearFallback(td, q, o.Limit, p)
		if o.JSON {
			return EmitJSON(w, rowsToJSON(res))
		}
		// Note line followed by a blank line (trailing "\n\n").
		fmt.Fprint(w, "[note] FTS5 unavailable on this build — slower linear scan, this project only.\n\n")
		PrintResults(w, res, -1)
		return nil
	}

	// DEBUG-SEARCH — read-only scoring explainer (LLM-free). Composes with --json
	// and --this-project; a pure output mode, no behavior change to the ranking.
	if o.DebugSearch {
		return runDebugSearch(w, o, q, p, ppred)
	}

	// DEFAULT (agent envelope) — a bare `rawclaw "query"` IS the search:
	// ranked refs + never-silent envelope. Search is the default verb.
	// Org-wide unless --this-project. Path include/exclude is applied inside
	// agentproto.Search (via opts), so the unfiltered this/all scope is passed here.
	var scope []view.Scope
	label := ""
	if o.ThisProject {
		sc, td, ok := thisScope(w, o)
		if !ok {
			return nil
		}
		scope = sc
		label = "on " + paths.ProjectLabel(td)
	} else {
		scope = allScope(ctx, o.Source, o.Reindex)
		label = "across all projects"
	}
	var emb embed.Embedder
	if !o.NoVector {
		emb = adapters.GetEmbedder()
	}
	return agentproto.SearchAndRender(w, q, scope, agentproto.SearchOpts{
		Limit:            o.Limit,
		Role:             o.Role,
		Sort:             o.Sort,
		IncludeTools:     o.IncludeTools,
		IncludeSubagents: o.IncludeSubagents,
		Since:            o.Since,
		Before:           o.Before,
		MinMessages:      o.MinMessages,
		IncludePath:      o.IncludePath,
		ExcludePath:      o.ExcludePath,
	}, emb, label, o.JSON)
}

// runDebugSearch handles the --debug-search shape: a read-only LLM-free scoring
// explainer. It runs the SAME ranking as a normal search (retrieve.SearchExplained
// is byte-identical to retrieve.Search) and renders a per-hit breakdown. Single
// project under --this-project; otherwise it loops per-project dbp exactly like
// the cross-project search path, merging the parallel (hits, explains) slices in
// lockstep so explains[i] keeps describing hits[i]. Composes with --json.
func runDebugSearch(w io.Writer, o *Options, q string, p retrieve.SearchParams, ppred func(cwd string) bool) error {
	var hits []retrieve.Hit
	var explains []retrieve.ScoreExplain

	if o.ThisProject {
		_, td, ok := thisScope(w, o)
		if !ok {
			return nil
		}
		dbp, _, _, err := index.EnsureIndexed(td, o.Reindex)
		if err != nil {
			return fmt.Errorf("debug-search ensure-indexed: %w", err)
		}
		hits, explains = retrieve.SearchExplained(dbp, q, o.Limit, p)
	} else {
		for _, d := range paths.AllProjectDirs() {
			if ppred != nil && !ppred(paths.ProjectCWD(d)) {
				continue
			}
			dbp, _, _, err := index.EnsureIndexed(d, false)
			if err != nil {
				continue
			}
			h, ex := retrieve.SearchExplained(dbp, q, o.Limit, p)
			// Append in lockstep so explains[i] keeps explaining hits[i].
			hits = append(hits, h...)
			explains = append(explains, ex...)
		}
	}

	if o.JSON {
		b, err := render.DebugSearchJSON(hits, explains)
		if err != nil {
			return err
		}
		fmt.Fprint(w, string(b))
		return nil
	}
	render.PrintDebugSearch(w, hits, explains)
	return nil
}

// ── flat printers + JSON emitters ──

// PrintResults renders flat one-line hits (the fallback shape).
// nSessions < 0 means "unknown" (rendered as '?').
func PrintResults(w io.Writer, res []retrieve.Hit, nSessions int) {
	if len(res) == 0 {
		fmt.Fprintln(w, "No matches. (Default searches top-level human text only — try --include-subagents "+
			"and/or --include-tools to widen, or rephrase: keyword > full sentence.)")
		return
	}
	scope := "this project's sessions"
	if nSessions >= 0 {
		scope = fmt.Sprintf("%d of this project's sessions", nSessions)
	}
	fmt.Fprintf(w, "Top %d match(es) across %s:\n\n", len(res), scope)
	for _, r := range res {
		label := lastSlice8(r.SessionID)
		tag := ""
		if r.IsSubagent && r.Parent != "" {
			tag = fmt.Sprintf(" · subagent⟵%s", trunc8(r.Parent))
		}
		// timefmt seam: search results are agent-parsed — marked UTC.
		fmt.Fprintf(w, "[%s · %s · %s%s] …%s…\n\n", orQ(timefmt.UTCFromISO(r.ISO)), label, r.Role, tag, r.Snippet)
	}
}

// ListProjects prints the searchable-projects table (with session counts). It
// enumerates the same Claude scopes search does — live project dirs PLUS orphaned
// index dbs whose source dir was purged (D8) — so a retained-but-purged project
// still shows, flagged so it doesn't read as a live source.
func ListProjects(w io.Writer) {
	root := paths.ProjectsRoot()
	type row struct {
		n       int
		label   string
		enc     string
		missing bool // source dir gone; sessions retained from the index
	}
	var rows []row
	for _, sc := range scopes.Claude() {
		if sc.TDir != "" { // live project: count from its transcripts (unchanged)
			matches, _ := filepath.Glob(filepath.Join(sc.TDir, "*.jsonl"))
			rows = append(rows, row{len(matches), paths.ProjectLabel(sc.TDir), baseName(sc.TDir), false})
			continue
		}
		// Orphaned source: no jsonl on disk — count retained sessions from the db.
		n := store.CountTopLevelSessions(sc.DBP)
		if n < 0 {
			n = 0
		}
		rows = append(rows, row{n, sc.Project, strings.TrimSuffix(filepath.Base(sc.DBP), ".db"), true})
	}
	if len(rows) == 0 {
		fmt.Fprintf(w, "No transcript projects found under %s.\n", root)
		return
	}
	// Sort by session count descending, then label ascending; stable.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].label < rows[j].label
	})
	fmt.Fprintf(w, "%d searchable projects under %s  (search one with --dir <working-dir>, or all with --all):\n\n", len(rows), root)
	for _, r := range rows {
		tag := ""
		if r.missing {
			tag = "  [source purged — retained history]"
		}
		fmt.Fprintf(w, "  %4s sessions   %-34s (%s)%s\n", fmt.Sprintf("%d", r.n), r.label, r.enc, tag)
	}
}

// EmitJSON writes obj as indented JSON (machine output, --json): 2-space indent,
// with HTML escaping disabled so <, >, & are emitted literally rather than
// \u-escaped.
func EmitJSON(w io.Writer, obj any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(obj); err != nil {
		return fmt.Errorf("emit json: %w", err)
	}
	return nil
}

// nullableStr maps a Go "" (the zero value our SQLite NULL columns scan to) back
// to a JSON null, so a NULL parent_id is emitted as null rather than "".
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// rowJSON / rowsToJSON shape the search()/brief hits for JSON output.
type rowJSON struct {
	ISO        string  `json:"iso"`
	SessionID  string  `json:"session_id"`
	Role       string  `json:"role"`
	IsSubagent bool    `json:"is_subagent"`
	Parent     *string `json:"parent"`
	Snippet    string  `json:"snippet"`
}

func rowsToJSON(res []retrieve.Hit) []rowJSON {
	out := make([]rowJSON, 0, len(res))
	for _, r := range res {
		out = append(out, rowJSON{r.ISO, r.SessionID, r.Role, r.IsSubagent, nullableStr(r.Parent), r.Snippet})
	}
	return out
}

// ── small helpers ──

// ExitError carries a non-zero process exit code (and an optional usage-style
// message) up to main, which surfaces it via os.Exit.
type ExitError struct {
	Code int
	Msg  string
}

func (e ExitError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

// validateChoice enforces an enum flag: empty = unset (allowed), else the value
// must be one of opts. Returns an ExitError(2) on a bad value.
func validateChoice(flag, val string, opts ...string) error {
	if val == "" {
		return nil
	}
	for _, o := range opts {
		if val == o {
			return nil
		}
	}
	return ExitError{Code: 2, Msg: fmt.Sprintf("argument --%s: invalid choice: %q (choose from %s)", flag, val, strings.Join(opts, ", "))}
}

// orQ returns s, or "?" when s is empty.
func orQ(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// trunc8 returns the first 8 runes of s (rune-safe truncation, no padding).
func trunc8(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return s
	}
	return string(r[:8])
}

// lastSlice8 returns the first 8 runes of the final '/'-separated segment of sid.
func lastSlice8(sid string) string {
	if i := strings.LastIndex(sid, "/"); i >= 0 {
		sid = sid[i+1:]
	}
	return trunc8(sid)
}

// baseName returns the final path element (basename) of p.
func baseName(p string) string {
	return filepath.Base(p)
}

// cwd returns the process working directory ("" on error) — the default for --dir.
func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		return ""
	}
	return d
}

// realpathExpand expands a leading ~ then resolves symlinks to an absolute path,
// used for the "No transcript history" hint.
func realpathExpand(p string) string {
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				p = home
			} else if strings.HasPrefix(p, "~/") {
				p = filepath.Join(home, p[2:])
			}
		}
	}
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		return rp
	}
	return filepath.Clean(p)
}

// isDir reports whether path is an existing directory.
func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
