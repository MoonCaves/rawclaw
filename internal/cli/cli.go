// Package cli is the thin composition root: the cobra command tree, flag
// wiring, the flat-output printers, and the JSON emitters. The engine lives in
// the sibling packages (parse, paths, index, query, retrieve, view, render,
// semantic, adapters, agentproto).
package cli

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/adapters"
	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/query"
	"github.com/MoonCaves/rawclaw/internal/render"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/semantic"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
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
	Offset           int
	Query            string // flag form of query (--query / -q)
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
	Until            string // alias for Before
	Days             int    // filter to last N days
	Today            bool   // filter to today only
	Yesterday        bool   // filter to yesterday only
	Week             bool   // filter to last 7 days
	NoVector         bool
	ReindexVectors   bool
	IncludePath      string
	ExcludePath      string
	MinMessages      int
	DebugSearch      bool
	Timeout          time.Duration
	DirSet           bool // --dir explicitly passed (the arbitrary-folder opt-in)
	Oneline          bool
	Format           string

	// CurrentSession is the caller's own live session ("" = fall back to the
	// runtime's env, "off" = don't exclude anything). Resolved by currentSession.
	CurrentSession string
}

// normalizeDates handles convenience aliases (--until, --days, --today, --yesterday, --week)
// and parses relative date expressions (-7d, -24h, etc.).
func (o *Options) normalizeDates() {
	if o.Until != "" && o.Before == "" {
		o.Before = o.Until
	}
	now := time.Now().UTC()
	if o.Days > 0 && o.Since == "" {
		o.Since = now.AddDate(0, 0, -o.Days).Format(timefmt.DateLayout)
	}
	if o.Today && o.Since == "" {
		o.Since = now.Format(timefmt.DateLayout)
	}
	if o.Yesterday {
		y := now.AddDate(0, 0, -1).Format(timefmt.DateLayout)
		if o.Since == "" {
			o.Since = y
		}
		if o.Before == "" {
			o.Before = y
		}
	}
	if o.Week && o.Since == "" {
		o.Since = now.AddDate(0, 0, -7).Format(timefmt.DateLayout)
	}
	if o.Since != "" {
		o.Since = timefmt.ParseDateFilter(o.Since)
	}
	if o.Before != "" {
		o.Before = timefmt.ParseDateFilter(o.Before)
	}
}

// oneline reports whether oneline format was requested via --oneline or --format oneline/line.
func (o *Options) oneline() bool {
	return o.Oneline || o.Format == "oneline" || o.Format == "line"
}

// currentSessionEnvs lists the runtime session-identity environment variables in
// documented precedence order (Claude Code, then Antigravity). Reading them is
// how the exclusion works without an agent remembering to pass a flag — the
// whole point is that the caller does NOT have to know it just typed something.
var currentSessionEnvs = []string{
	"CLAUDE_CODE_SESSION_ID",
	"ANTIGRAVITY_CONVERSATION_ID",
}

type sessionEnvMatch struct {
	env string
	sid string
}

// currentSession resolves which session the caller is live in, for the
// current-turn exclusion: the explicit flag first, then "off" disables it, then
// runtime environment variables in documented order (currentSessionEnvs). If
// multiple runtime session variables are inherited, a structured warning is
// logged and the first in documented order is used.
func (o *Options) currentSession() string {
	v := strings.TrimSpace(o.CurrentSession)
	if strings.EqualFold(v, "off") {
		return ""
	}
	if v != "" {
		return v
	}
	var matches []sessionEnvMatch
	for _, env := range currentSessionEnvs {
		if sid := strings.TrimSpace(os.Getenv(env)); sid != "" {
			matches = append(matches, sessionEnvMatch{env: env, sid: sid})
		}
	}
	if len(matches) == 0 {
		return ""
	}
	if len(matches) > 1 {
		var ignored []string
		for _, m := range matches[1:] {
			ignored = append(ignored, m.env)
		}
		slog.Warn("multiple session environment variables set; using documented precedence",
			"chosen_env", matches[0].env,
			"chosen_session", matches[0].sid,
			"ignored_envs", strings.Join(ignored, ", "),
		)
	}
	return matches[0].sid
}

// pathScoped reports whether either path Scope flag is set — the flags that
// bound WHICH projects a run covers, as opposed to which rows come back.
func (o *Options) pathScoped() bool {
	return o.IncludePath != "" || o.ExcludePath != ""
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
		Offset:           o.Offset,
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
	return fmt.Sprintf("rawclaw %s (commit: %s, built: %s)",
		cmp.Or(b.Version, "dev"),
		cmp.Or(b.Commit, "unknown"),
		cmp.Or(b.Date, "unknown"),
	)
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
			"  rawclaw [--this-project|--all] [--limit N]  recent sessions, newest first (no query)\n\n" +
			"Searches every project by default; --this-project (with --dir) or --include-path <regex> to scope. " +
			"Add --json for structured output. A search that finds nothing prints a no-matches note and exits 0 — " +
			"empty is a valid answer, not an error. Results are raw session history — verify against current state before acting.\n\n" +
			"Session ids use the durable session catalog for fast exact/prefix lookup when available, then fall back to transcript discovery if the catalog misses; a catalog miss is not itself a failure.\n\n" +
			"Bare browse normally answers from the consolidated store; use --reindex to bypass it and refresh the selected source indexes before browsing.\n\n" +
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
			opts.normalizeDates()
			// An explicit --dir is the opt-in that lets an arbitrary
			// jsonl-bearing folder resolve as a transcripts dir; the cwd
			// default never is (folder guard).
			opts.DirSet = cmd.Flags().Changed("dir")
			return runRoot(cmd, opts, args)
		},
	}

	f := root.Flags()
	f.IntVarP(&opts.Limit, "limit", "n", 8, "max hits to return")
	f.StringVarP(&opts.Query, "query", "q", "", "search query terms (flag alternative to positional args)")
	f.IntVar(&opts.Offset, "offset", 0, "skip the first N hits (pagination)")
	f.StringVar(&opts.Dir, "dir", cwd(),
		"the project's working directory (e.g. ~/code/my-project); encoded to "+
			"find its transcripts. An already-encoded ~/.claude/projects path also works.")
	f.BoolVar(&opts.ThisProject, "this-project", false, "narrow to THIS project only (default searches all projects)")
	f.Bool("this-desk", false, "") // hidden backward-compat alias for --this-project
	_ = f.MarkHidden("this-desk")
	f.BoolVar(&opts.All, "all", false, "cover every project: the search default already, and the widener for bare browse and --stats")
	f.BoolVar(&opts.List, "list", false, "list all searchable projects (with session counts) and exit")
	f.StringVar(&opts.Role, "role", "", "only this author role (user|assistant)")
	f.StringVar(&opts.Source, "source", "", "only this runtime (claude|codex|antigravity|goose|pi|opencode); default searches all (goose discovery is opt-in: pass --source goose or set RAWCLAW_GOOSE=1)")
	f.StringVar(&opts.Sort, "sort", "", "result order (newest|oldest)")
	f.BoolVar(&opts.IncludeTools, "include-tools", false, "also match/show tool calls + tool-only hits")
	f.BoolVar(&opts.IncludeSubagents, "include-subagents", false, "also search delegated subagent threads")
	f.BoolVar(&opts.Reindex, "reindex", false, "force a full re-index before searching or browsing")
	f.BoolVar(&opts.JSON, "json", false, "machine-readable JSON output (for agents/scripts)")
	f.StringVar(&opts.Resume, "resume", "", "print the paste-ready resume command (claude/codex/agy/goose/pi/opencode) for a session id (use the 8-char id from search output)")
	f.BoolVar(&opts.Stats, "stats", false, "corpus overview (sessions/messages/date span) for this project, or --all for every project")
	f.StringVar(&opts.Since, "since", "", "only results on/after this date (YYYY-MM-DD or relative like -7d)")
	f.StringVar(&opts.Before, "before", "", "only results on/before this date (YYYY-MM-DD or relative like -24h)")
	f.StringVar(&opts.Until, "until", "", "only results on/before this date (alias for --before)")
	f.IntVar(&opts.Days, "days", 0, "filter to results within the last N days")
	f.BoolVar(&opts.Today, "today", false, "filter to results from today only")
	f.BoolVar(&opts.Yesterday, "yesterday", false, "filter to results from yesterday only")
	f.BoolVar(&opts.Week, "week", false, "filter to results from the last 7 days")
	f.BoolVar(&opts.NoVector, "no-vector", false, "force keyword-only (ignore any configured embedder)")
	f.BoolVar(&opts.ReindexVectors, "reindex-vectors", false, "build/update the semantic index for the scope (needs RAWCLAW_EMBED_ENDPOINT)")
	f.StringVar(&opts.IncludePath, "include-path", "", "only cover projects whose working dir matches this regex (search AND bare browse)")
	f.StringVar(&opts.ExcludePath, "exclude-path", "", "skip projects whose working dir matches this regex, e.g. /tmp (search AND bare browse)")
	f.IntVar(&opts.MinMessages, "min-messages", 0, "only sessions with >= N messages (drops thin/bootstrap threads)")
	f.BoolVar(&opts.DebugSearch, "debug-search", false, "explain WHY each hit ranked where it did (LLM-free scoring breakdown)")
	_ = f.MarkHidden("debug-search")
	f.BoolVar(&opts.Oneline, "oneline", false, "output search hits in one-line format (<read_ref>\t<started_iso>\t<project>\t<snippet>)")
	f.StringVar(&opts.Format, "format", "", "output format (text|oneline|line|json)")
	f.StringVar(&opts.CurrentSession, "current-session", "",
		"the session you are searching FROM (id or 8-char prefix); its CURRENT TURN — the prompt "+
			"just typed and this turn's tool output — is withheld, since it is not recall and it "+
			"outranks the archive on its own words. That session's earlier history stays searchable. "+
			"Defaults to $CLAUDE_CODE_SESSION_ID or $ANTIGRAVITY_CONVERSATION_ID; pass `off` to search your own live turn too.")

	// --timeout is PERSISTENT (every subcommand inherits it): rawclaw must be
	// self-bounding so an agent never needs an external `timeout(1)`. Default 30s;
	// RAWCLAW_TIMEOUT overrides the default; --timeout 0 disables the watchdog.
	// The watchdog itself is armed in Execute (which wraps root.Execute) so it is
	// disarmed on EVERY path — including a command that returns an error, where
	// cobra would skip a PersistentPostRunE hook.
	root.PersistentFlags().DurationVar(&opts.Timeout, "timeout", defaultTimeout,
		"hard wall-clock deadline for the whole run; exits 124 if exceeded (0 disables; env RAWCLAW_TIMEOUT)")

	// Validate the role/sort/format enums before running: reject anything outside the
	// allowed set with an "invalid choice" message (stderr + exit 2), keeping the
	// validation in cobra's pre-run hook.
	root.PreRunE = func(cmd *cobra.Command, args []string) error {
		if err := validateChoice("role", opts.Role, "user", "assistant"); err != nil {
			return err
		}
		if err := validateChoice("sort", opts.Sort, "newest", "oldest"); err != nil {
			return err
		}
		return validateChoice("format", opts.Format, "text", "oneline", "line", "json")
	}

	// `--version` prints the banner verbatim (cobra's default template prefixes
	// "{{.Name}} version", which would double the "rawclaw").
	root.SetVersionTemplate("{{.Version}}\n")

	root.AddCommand(newSearchCmd(opts, f))
	root.AddCommand(newReadCmd())
	root.AddCommand(newOutlineCmd())
	root.AddCommand(newTopicsCmd())
	root.AddCommand(newConsolidateCmd())
	root.AddCommand(newIngestCmd())
	root.AddCommand(newTagPrepCmd())
	root.AddCommand(newPrewarmCmd())
	root.AddCommand(newTagWriteCmd())
	root.AddCommand(newCloseoutCmd())
	root.AddCommand(newVectorTopupCmd())
	root.AddCommand(newTagPublishCmd())
	archiveCmd := newArchiveCmd()
	archiveCmd.AddCommand(newArchiveInitCmd())
	archiveCmd.AddCommand(newArchiveExportBundleCmd())
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
	root.AddCommand(newCompletionCmd())

	_ = root.RegisterFlagCompletionFunc("role", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"user", "assistant"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = root.RegisterFlagCompletionFunc("sort", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"newest", "oldest"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = root.RegisterFlagCompletionFunc("source", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return registeredSourceIDs(), cobra.ShellCompDirectiveNoFileComp
	})
	return root
}

// newSearchCmd builds the explicit `search` subcommand as an alias for bare search.
func newSearchCmd(opts *Options, rootFlags *pflag.FlagSet) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "search transcripts (explicit alias for bare rawclaw <query>)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("this-desk") {
				opts.ThisProject = true
			}
			opts.normalizeDates()
			opts.DirSet = cmd.Flags().Changed("dir")
			return runRoot(cmd, opts, args)
		},
	}
	cmd.Flags().AddFlagSet(rootFlags)
	return cmd
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
		if (isConsolidateInvocation(args) || isIngestAllInvocation(args)) && resolved < consolidateWatchdog {
			return consolidateWatchdog
		}
		if isArchiveExportBundleInvocation(args) && resolved < exportBundleWatchdog {
			return exportBundleWatchdog
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
		// --reindex-vectors runs without the wall-clock watchdog for the same
		// reason: no cap fits both a hung endpoint and a legitimate first embed
		// of a whole corpus, whose cost scales with the history and the
		// embedder's throughput. The run stays bounded because EVERY embedding
		// request is individually bounded (adapters' 15s serial / 120s batch
		// HTTP timeouts), so a dead endpoint fails fast per call rather than
		// parking the run. A 30s default here made the README's documented
		// "embed your history once" flow impossible to finish on any real
		// corpus — it self-terminated partway through, every time.
		if isReindexVectorsInvocation(args) {
			return 0
		}
	}
	return resolved
}

// isReindexVectorsInvocation reports whether args ask for the vector backfill.
// It is a root FLAG, not a subcommand, so this probes the flag rather than the
// leading tokens.
func isReindexVectorsInvocation(args []string) bool {
	probe := pflag.NewFlagSet("rawclaw-reindex-probe", pflag.ContinueOnError)
	probe.ParseErrorsWhitelist.UnknownFlags = true
	probe.SetOutput(io.Discard)
	rv := probe.Bool("reindex-vectors", false, "")
	_ = probe.Parse(args)
	return *rv
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
	"--min-messages": true, "--format": true,
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

// isConsolidateInvocation reports whether args target `consolidate`, the bulk
// fold of every per-project index into the single store — the one read-side
// command whose cost is measured in minutes on a large corpus rather than
// milliseconds.
func isConsolidateInvocation(args []string) bool {
	w := leadingSubcommandTokens(args, 1)
	return len(w) == 1 && w[0] == "consolidate"
}

// isIngestAllInvocation reports whether args target `ingest` without a specific
// session ID, triggering a full corpus discovery and consolidation sweep whose
// cost scales with the entire history on disk.
func isIngestAllInvocation(args []string) bool {
	w := leadingSubcommandTokens(args, 2)
	return len(w) == 1 && w[0] == "ingest"
}

// isArchiveExportBundleInvocation reports whether args target `archive export-bundle`.
func isArchiveExportBundleInvocation(args []string) bool {
	w := leadingSubcommandTokens(args, 2)
	return len(w) == 2 && w[0] == "archive" && w[1] == "export-bundle"
}

// isArchiveSyncInvocation reports whether args target a SYNCING archive verb —
// `archive init|push|pull|autosync` — the ones that talk to the git remote and
// run stall-bounded instead of wall-clock-bounded. `archive
// status`/`enable-timer`/`archive <session>` (the local move) keep the
// default watchdog.
func isArchiveSyncInvocation(args []string) bool {
	w := leadingSubcommandTokens(args, 2)
	if len(w) < 2 {
		return false
	}
	if w[0] == "setup" && w[1] == "live" {
		return true
	}
	if w[0] == "archive" {
		switch w[1] {
		case "init", "push", "pull", "autosync":
			return true
		}
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

// verbScope resolves the scope for the read/outline/topics/tag verbs, as a
// pair: the scope itself, and the function that builds the all-projects list
// if one turns out to be needed.
//
// With --this-project it returns the single cwd/--dir project — or an explicit
// EMPTY scope when the dir has no transcript history, so the verb resolves
// nothing rather than silently going wide. dirSet marks an explicit --dir (the
// arbitrary-folder opt-in resolveTDir honors).
//
// Without it the scope is nil, meaning every project, and the list is NOT
// built here. Building it opens — and after a schema change, migrates — every
// per-project index, which on a real corpus costs minutes against a verb the
// watchdog stops in thirty seconds. The one store answers an id lookup with no
// list at all, so the verbs call this function only if that lookup comes up
// empty. The closure still captures the run's ctx, so when it does run, the
// archive enumeration's git probes are under the watchdog as before.
func verbScope(ctx context.Context, thisProject bool, dir string, dirSet bool) ([]view.Scope, agentproto.ScopeFn) {
	if !thisProject {
		return nil, func() []view.Scope { return allScope(ctx, "", false) }
	}
	td := resolveTDir(dir, dirSet)
	if td == "" || !isDir(td) {
		return []view.Scope{}, nil
	}
	return []view.Scope{{Project: paths.ProjectLabel(td), TDir: td}}, nil
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
func parseWithFlags(withFlags []string, includeTools, includeSubagents bool) (tools, thinking, subagents bool, err error) {
	tools = includeTools
	subagents = includeSubagents
	for _, w := range withFlags {
		for _, part := range strings.Split(w, ",") {
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
	f.Lookup("budget").NoOptDefVal = strconv.Itoa(agentproto.DefaultReadBudget)
	f.IntVar(&moreLevel, "more", 0, "widen the window: --more (1 level) or --more=N (N levels)")
	f.Lookup("more").NoOptDefVal = "1"
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
func runRoot(cmd *cobra.Command, o *Options, args []string) error {
	out := cmd.OutOrStdout()
	ctx := cmd.Context()

	// FIRST thing in the run: --no-vector switches off the whole vector lane,
	// including the background top-up that indexing fires. Indexing happens
	// deep inside scope resolution below, so setting this any later reads the
	// PREVIOUS run's value — and because it is process-global, in a test binary
	// that means the previous test's value. Set unconditionally, never only in
	// the true branch, so each invocation starts from its own flag.
	semantic.SetNoVector(o.NoVector)
	if err := validateChoice("source", o.Source, registeredSourceIDs()...); err != nil {
		return err
	}
	// --source narrows the runtime axis. --list enumerates Claude project dirs
	// only; --resume resolves a session id whose runtime is already fixed, so
	// those shapes still refuse an irrelevant source filter.
	if o.Source != "" {
		switch {
		case o.List:
			return ExitError{Code: 2, Msg: "--source does not apply to --list (it enumerates Claude project dirs); drop --source"}
		case o.Resume != "":
			return ExitError{Code: 2, Msg: "--source does not apply to --resume (the session id already fixes the runtime); drop --source"}
		}
	}

	if o.Format == "json" {
		o.JSON = true
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

	if strings.TrimSpace(o.Query) != "" {
		if len(args) == 0 {
			args = []string{o.Query}
		} else {
			args = append(args, o.Query)
		}
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
		if o.oneline() {
			return nil
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
func allScope(ctx context.Context, source string, reindex bool, paths ...string) []view.Scope {
	var pathPred func(string) bool
	if len(paths) > 0 {
		include := paths[0]
		exclude := ""
		if len(paths) > 1 {
			exclude = paths[1]
		}
		if include != "" || exclude != "" {
			pathPred = query.PathPredicate(include, exclude)
		}
	}
	return scopes.All(ctx, source, reindex, pathPred)
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

	// Index each scope FIRST, for its side effect only: resolving a scope folds
	// its rows into the consolidated store. The vectors are not written here.
	for _, s := range scope {
		if _, _, err := scopes.Resolve(s, false); err != nil {
			fmt.Fprintf(w, "  %s: skipped (%s)\n", s.Project, err)
		}
	}

	// Then embed ONCE, against the store a search actually opens. Embedding each
	// scope's own db instead is what made this command a no-op: default search
	// reads the consolidated store, so vectors written per project were never
	// read by anything and the command still printed a success line.
	total, err := reindexConsolidated(ctx, emb)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nSemantic index updated: +%d new vectors in %s. Run a normal search to use it (RRF-fused).\n",
		total, index.ConsolidatedPath())
	return nil
}

// reindexConsolidated embeds every not-yet-vectored message in the consolidated
// store (open read-write → vector index → close). One store, one pass: the
// per-project databases are a staging cache the reader never opens.
func reindexConsolidated(ctx context.Context, emb embed.Embedder) (int, error) {
	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		return 0, err
	}
	defer con.Close()
	return semantic.VecIndex(ctx, con, emb, 0)
}

// runResume prints the paste-ready resume command for a session id across all
// runtimes (Claude, Codex, Antigravity, Goose). It collects candidate matches
// across all registered sources; if matches span multiple sessions (even across
// different runtimes), it reports global ambiguity. If local runtimes yield
// no match, it falls back to archive replicas.
type resumeCandidate struct {
	hit paths.SessionHit
	src string
}

func runResume(w io.Writer, o *Options) error {
	var matches []resumeCandidate
	if isFullResumeID(o.Resume) {
		var complete, metadataGuard bool
		matches, complete, metadataGuard = resumeExactMetadata(o.Resume)
		if complete || metadataGuard {
			if metadataGuard && len(matches) == 0 {
				fmt.Fprintf(w, "Session %s is known, but RawClaw cannot produce a safe local resume command for it.\n", o.Resume)
				return nil
			}
			return emitResumeMatches(w, o, matches, false)
		}
	}
	for _, h := range paths.ResolveSession(o.Resume) {
		matches = appendResumeCandidate(matches, resumeCandidate{hit: h, src: "claude"})
	}
	for _, entry := range []struct {
		src    string
		scopes []view.Scope
	}{
		{"codex", scopes.Codex(false)},
		{"antigravity", scopes.Antigravity(false)},
		{"pi", scopes.Pi(false)},
		{"opencode", scopes.OpenCode(false)},
		{"goose", gooseResumeScopes()},
	} {
		for _, h := range scopeResumeHits(entry.scopes, o.Resume) {
			matches = appendResumeCandidate(matches, resumeCandidate{hit: h, src: entry.src})
		}
	}

	return emitResumeMatches(w, o, matches, true)
}

func emitResumeMatches(w io.Writer, o *Options, matches []resumeCandidate, allowArchiveFallback bool) error {
	if len(matches) == 0 {
		if allowArchiveFallback {
			if handled, err := resumeForeign(w, o); handled {
				return err
			}
		}
		fmt.Fprintf(w, "No session id starts with '%s'. Use the 8-char id from search output, e.g. [… · a1b2c3d4 · …].\n", o.Resume)
		return nil
	}
	if len(matches) > 1 {
		if o.JSON {
			type row struct {
				SessionID string `json:"session_id"`
				CWD       string `json:"cwd"`
				Project   string `json:"project"`
			}
			rows := make([]row, 0, len(matches))
			for _, m := range matches {
				rows = append(rows, row{m.hit.SessionID, m.hit.CWD, m.hit.Project})
			}
			return EmitJSON(w, rows)
		}
		fmt.Fprintf(w, "%d sessions match '%s' — narrow it:\n", len(matches), o.Resume)
		for _, m := range matches {
			fmt.Fprintf(w, "  %s  (%s)\n", m.hit.SessionID, m.hit.Project)
		}
		return nil
	}

	m := matches[0]
	cmd := resumeCommand(m.src, m.hit)
	if o.JSON {
		return EmitJSON(w, struct {
			SessionID string `json:"session_id"`
			CWD       string `json:"cwd"`
			Project   string `json:"project"`
			Command   string `json:"command"`
		}{m.hit.SessionID, m.hit.CWD, m.hit.Project, cmd})
	}
	fmt.Fprintf(w, "Resume this session (%s):\n\n  %s\n", m.hit.Project, cmd)
	return nil
}

func isFullResumeID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

// resumeExactMetadata answers known full IDs without constructing scopes. The
// durable vault is checked first because it survives source-file deletion.
func resumeExactMetadata(id string) ([]resumeCandidate, bool, bool) {
	regs := sources.Registered()
	supported := make(map[string]bool, len(regs))
	for _, r := range regs {
		supported[r.ID] = true
	}
	meta := resumeMetadata{namedSources: make(map[string]struct{})}
	if m, _, found := durable.Exact(id); found {
		classifyResumeMetadata(&meta, id, m.Source, m.SourcePath, m.CWD,
			m.OnlyCopySince != 0, m.Origin != "", m.IsSubagent, m.ParentID, supported)
	}
	conMeta := resumeConsolidatedMetadata(id, supported)
	meta.blocked = meta.blocked || conMeta.blocked
	meta.unknown = meta.unknown || conMeta.unknown
	for src := range conMeta.namedSources {
		meta.namedSources[src] = struct{}{}
	}
	for _, candidate := range conMeta.matches {
		meta.matches = appendResumeCandidate(meta.matches, candidate)
	}
	if len(meta.matches) > 0 {
		return meta.matches, true, true
	}
	if meta.blocked {
		return nil, true, true
	}

	complete := true
	for _, r := range regs {
		if len(meta.namedSources) > 0 {
			if _, ok := meta.namedSources[r.ID]; !ok {
				continue
			}
		}
		if r.ID == "goose" && !scopes.GooseOptedIn("") {
			continue
		}
		if r.Lookup == nil {
			complete = false
			continue
		}
		containers, err := r.Lookup(id)
		if err != nil {
			complete = false
			continue
		}
		for _, c := range containers {
			if c.ID == id && !c.IsSubagent && c.ParentID == "" {
				meta.matches = appendResumeCandidate(meta.matches, resumeCandidate{hit: paths.SessionHit{SessionID: id, CWD: c.CWD, Project: filepath.Base(c.CWD)}, src: r.ID})
			}
		}
	}
	return meta.matches, complete && !meta.unknown, false
}

type resumeMetadata struct {
	matches      []resumeCandidate
	blocked      bool
	unknown      bool
	namedSources map[string]struct{}
}

func classifyResumeMetadata(meta *resumeMetadata, id, src, path, cwd string, retained, foreign, subagent bool, parent string, supported map[string]bool) {
	switch {
	case retained, foreign, subagent, parent != "":
		meta.blocked = true
	case src == "" || !supported[src]:
		meta.unknown = true
	default:
		meta.namedSources[src] = struct{}{}
		if regularFile(strings.Split(path, "#")[0]) {
			meta.matches = appendResumeCandidate(meta.matches, resumeCandidate{hit: paths.SessionHit{SessionID: id, CWD: cwd, Project: filepath.Base(cwd)}, src: src})
		}
	}
}

func resumeConsolidatedMetadata(id string, supported map[string]bool) resumeMetadata {
	meta := resumeMetadata{namedSources: make(map[string]struct{})}
	con, _, err := index.OpenConsolidated()
	if err != nil {
		return meta
	}
	defer con.Close()
	rows, err := con.Query(`SELECT session_id, COALESCE(project,''), COALESCE(source_tool,''),
		COALESCE(source_path,''), COALESCE(cwd,''), COALESCE(origin_machine,''),
		only_copy_since, is_subagent, COALESCE(parent_id,'') FROM session_sources WHERE session_id=?
		UNION ALL
		SELECT id, COALESCE(project,''), COALESCE(source_tool,''), COALESCE(source_path,''),
			COALESCE(cwd,''), COALESCE(origin_machine,''), only_copy_since, is_subagent,
			COALESCE(parent_id,'')
		FROM sessions WHERE id=? AND NOT EXISTS (SELECT 1 FROM session_sources WHERE session_id=?)`, id, id, id)
	if err != nil {
		return meta
	}
	defer rows.Close()
	for rows.Next() {
		var sid, project, src, path, cwd, origin string
		var onlyCopy sql.NullFloat64
		var sub int
		var parent string
		if err := rows.Scan(&sid, &project, &src, &path, &cwd, &origin, &onlyCopy, &sub, &parent); err != nil {
			return resumeMetadata{namedSources: make(map[string]struct{})}
		}
		before := len(meta.matches)
		classifyResumeMetadata(&meta, sid, src, path, cwd, onlyCopy.Valid, origin != "", sub != 0, parent, supported)
		if len(meta.matches) > before && project != "" {
			meta.matches[len(meta.matches)-1].hit.Project = project
		}
	}
	if err := rows.Err(); err != nil {
		return resumeMetadata{namedSources: make(map[string]struct{})}
	}
	return meta
}

func regularFile(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func appendResumeCandidate(matches []resumeCandidate, candidate resumeCandidate) []resumeCandidate {
	for _, m := range matches {
		if m.src == candidate.src && m.hit.SessionID == candidate.hit.SessionID {
			return matches
		}
	}
	return append(matches, candidate)
}

// resumeCommand builds the paste-ready resume command for a session using the
// runtime's registered resume verb table.
func resumeCommand(src string, h paths.SessionHit) string {
	return source.ResumeCommand(src, h.SessionID, h.CWD)
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

// scopeResumeHits resolves a session-id prefix against a slice of scope dbs.
// A session's cwd is its scope's cwd. Only top-level sessions are offered
// (is_subagent=0), matching ResolveSession's Claude behavior.

// gooseResumeScopes lists goose scopes for resume-candidate matching only when
// goose is opted in — the listing walks discovery, which default-off goose
// must not pay. Resume itself opts in via --source goose like everything else.
func gooseResumeScopes() []view.Scope {
	if !scopes.GooseOptedIn("") {
		return scopes.GooseOrphanScopes()
	}
	return scopes.Goose(false)
}

func scopeResumeHits(scopeList []view.Scope, prefix string) []paths.SessionHit {
	var out []paths.SessionHit
	for _, sc := range scopeList {
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
// or — under --all or a path scope — across the projects those flags select
// (the same scope enumeration search uses). An explicit --this-project wins
// over --all, same precedence runStats applies.
//
// --include-path / --exclude-path are structural SCOPE flags: they bound which
// projects a run covers, and they compose with the rest rather than being
// consumed by one shape. They name projects by working dir, so — like search,
// whose universe is every project unless --this-project — a path flag browses
// ACROSS projects; --this-project first narrows the universe to the cwd and the
// predicate then applies to that one project alone. Browse used to accept both
// flags and browse the cwd anyway: `rawclaw --include-path myproject` answered
// from /tmp with two throwaway /tmp sessions under the header "2 most-recent
// sessions on tmp", i.e. a different question's answer wearing the caller's
// flags. A flag accepted and silently ignored is the worst outcome for an agent
// caller, which trusts it and moves on wrong.
//
// --source is the same kind of structural scope flag: `rawclaw --source
// antigravity` used to drop the flag and browse the cwd's Claude sessions —
// so it routes through the scoped path too, where allScope already filters
// the universe by runtime.
func runBrowse(ctx context.Context, w io.Writer, o *Options) error {
	if o.pathScoped() || o.Source != "" || (o.All && !o.ThisProject) {
		var universe []view.Scope
		if o.ThisProject {
			sc, _, ok := thisScope(w, o)
			if !ok {
				return nil
			}
			universe = sc
		} else {
			universe = allScope(ctx, o.Source, o.Reindex, o.IncludePath, o.ExcludePath)
		}
		return runBrowseScoped(w, o, universe)
	}
	sc, td, ok := thisScope(w, o)
	if !ok {
		return nil
	}
	_ = sc
	var rows []view.BrowseRow
	usedConsolidated := false
	var (
		indexStale bool
		staleNote  string
	)
	if !o.Reindex {
		if con, _, err := index.OpenConsolidated(); err == nil {
			defer con.Close()
			if freshness, fErr := index.CheckIndexFreshness(con); fErr == nil && !freshness.Fresh {
				indexStale = true
				staleNote = staleIngestNote()
			}
			if res, err := view.BrowseScoped(con, o.Limit, o.Since, o.Before, o.Source, []string{paths.ProjectLabel(td)}); err == nil && len(res) > 0 {
				rows = make([]view.BrowseRow, 0, len(res))
				for _, r := range res {
					rows = append(rows, r.BrowseRow)
				}
				usedConsolidated = true
			}
		}
	}
	if !usedConsolidated {
		rows = view.Browse(td, o.Limit, o.Since, o.Before)
	}
	if o.JSON {
		return EmitJSON(w, struct {
			Project   string           `json:"project"`
			Stale     bool             `json:"stale,omitempty"`
			StaleNote string           `json:"stale_note,omitempty"`
			Sessions  []view.BrowseRow `json:"sessions"`
		}{
			Project:   paths.ProjectLabel(td),
			Stale:     indexStale,
			StaleNote: staleNote,
			Sessions:  rows,
		})
	}
	if o.oneline() {
		for _, r := range rows {
			ref := lastSlice8(r.SessionID) + ":"
			clean := agentproto.CleanSnippetOneline(r.Preview)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ref, timefmt.UTC(time.Unix(int64(r.LastTS), 0)), paths.ProjectLabel(td), clean)
		}
		return nil
	}
	render.PrintBrowse(w, rows, paths.ProjectLabel(td))
	if staleNote != "" {
		fmt.Fprintf(w, "\nnote: %s\n", staleNote)
	}
	return nil
}

// runBrowseScoped is the cross-project shape of the no-query browse: recent
// sessions across the scopes the Scope flags leave standing (Claude + Codex +
// retained — the same enumeration search uses), merged newest-first and capped
// at --limit. It answers from the consolidated store with a single read connection,
// falling back to per-project databases only if the consolidated store is unavailable.
func runBrowseScoped(w io.Writer, o *Options, universe []view.Scope) error {
	scope := scopes.FilterByPath(universe, o.IncludePath, o.ExcludePath)
	if o.pathScoped() && len(scope) == 0 {
		return browseNoScopeMatch(w, o, len(universe))
	}

	var (
		rows             []view.BrowseAllRow
		usedConsolidated bool
		indexStale       bool
		staleNote        string
	)
	var freshness *index.IndexFreshness

	if !o.Reindex {
		if con, _, err := index.OpenConsolidated(); err == nil {
			defer con.Close()
			if f, fErr := index.CheckIndexFreshness(con); fErr == nil {
				freshness = &f
			}
			var projects []string
			if o.pathScoped() || o.ThisProject {
				seen := make(map[string]bool, len(scope))
				for _, sc := range scope {
					if !seen[sc.Project] {
						seen[sc.Project] = true
						projects = append(projects, sc.Project)
					}
				}
			}
			if res, err := view.BrowseScoped(con, o.Limit, o.Since, o.Before, o.Source, projects); err == nil {
				rows = res
				usedConsolidated = true
			}
		}
	}

	if !usedConsolidated {
		rows = []view.BrowseAllRow{}
		for _, sc := range scope {
			dbp, _, err := scopes.Resolve(sc, o.Reindex)
			if err != nil {
				continue // an unresolvable scope can't contribute rows; others still can
			}
			if freshness == nil {
				if db, dbErr := store.ConnectRO(dbp); dbErr == nil {
					if f, fErr := index.CheckIndexFreshness(db); fErr == nil {
						freshness = &f
					}
					_ = db.Close()
				}
			}
			if o.Source != "" && sc.Source != "" && sc.Source != o.Source {
				continue
			}
			for _, r := range view.BrowseDB(dbp, o.Limit, o.Since, o.Before) {
				rows = append(rows, view.BrowseAllRow{Project: sc.Project, BrowseRow: r})
			}
		}
		// Newest-first across projects; each scope contributed at most --limit rows,
		// so the merge only has to re-sort and cap.
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].LastTS != rows[j].LastTS {
				return rows[i].LastTS > rows[j].LastTS
			}
			return rows[i].SessionID < rows[j].SessionID
		})
		if len(rows) > o.Limit {
			rows = rows[:o.Limit]
		}
	}
	if !o.Reindex && freshness != nil && !freshness.Fresh {
		if freshness.Reason == "no_ingest_watermark" {
			staleNote = freshnessUnknownNote()
		} else {
			indexStale = true
			staleNote = staleIngestNote()
		}
	}

	if o.JSON {
		return EmitJSON(w, browseScopeJSON(o, len(scope), rows, indexStale, staleNote))
	}
	if o.oneline() {
		for _, r := range rows {
			ref := lastSlice8(r.SessionID) + ":"
			clean := agentproto.CleanSnippetOneline(r.Preview)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ref, timefmt.UTC(time.Unix(int64(r.LastTS), 0)), r.Project, clean)
		}
		return nil
	}
	render.PrintBrowseAll(w, rows, browseScopeLabel(o))
	if staleNote != "" {
		fmt.Fprintf(w, "\nnote: %s\n", staleNote)
	}
	return nil
}

// browseNoScopeMatch reports a path scope that kept no project at all. Scope
// never relaxes: rather than quietly widening back to the cwd or to every
// project — the silent rewrite this whole contract exists to kill — the empty
// is printed WITH its real boundary, the size of the universe the predicate ran
// against, plus the verb that lists the working dirs it was matched on. Exit 0:
// an honestly empty scope is an answer, not an error.
func browseNoScopeMatch(w io.Writer, o *Options, universe int) error {
	if o.JSON {
		return EmitJSON(w, browseScopeJSON(o, 0, []view.BrowseAllRow{}, false, ""))
	}
	fmt.Fprintf(w, "No project matches %s (0 of %d searchable). Try --list to see their working dirs.\n",
		pathScopePhrase(o), universe)
	if o.IncludePath != "" && agentproto.LooksLikeSessionID(o.IncludePath) {
		fmt.Fprintf(w, "Hint: --include-path filters project paths, not session IDs. Did you mean `rawclaw outline %s` or `rawclaw --resume %s`?\n", o.IncludePath, o.IncludePath)
	}
	return nil
}

// pathScopePhrase echoes the path Scope flags back verbatim, so every message
// about them names what the caller actually typed.
func pathScopePhrase(o *Options) string {
	var parts []string
	if o.IncludePath != "" {
		parts = append(parts, "--include-path "+o.IncludePath)
	}
	if o.ExcludePath != "" {
		parts = append(parts, "--exclude-path "+o.ExcludePath)
	}
	return strings.Join(parts, " ")
}

// browseScopeLabel names the universe a cross-project browse actually covered.
// "all projects" is true only while nothing narrowed it — a header must never
// name a scope the caller did not ask for.
func browseScopeLabel(o *Options) string {
	label := "all projects"
	if o.ThisProject {
		label = "this project"
	}
	if o.Source != "" {
		label += " · source " + o.Source
	}
	if !o.pathScoped() {
		return label
	}
	return label + " matching " + pathScopePhrase(o)
}

// browseScopeJSON is the machine shape of a cross-project browse. Beyond the
// rows it reports the scope actually covered — the path flags verbatim and how
// many projects survived them — so an agent reading `sessions: []` can tell an
// empty corpus from a filter that matched no project at all. Same
// incompleteness-as-data posture as the search envelope's scope reports.
func browseScopeJSON(o *Options, projects int, rows []view.BrowseAllRow, stale bool, staleNote string) any {
	scope := "all"
	if o.ThisProject {
		scope = "project"
	}
	return struct {
		Scope       string              `json:"scope"`
		Source      string              `json:"source,omitempty"`
		IncludePath string              `json:"include_path,omitempty"`
		ExcludePath string              `json:"exclude_path,omitempty"`
		Projects    int                 `json:"projects"`
		Stale       bool                `json:"stale,omitempty"`
		StaleNote   string              `json:"stale_note,omitempty"`
		Sessions    []view.BrowseAllRow `json:"sessions"`
	}{scope, o.Source, o.IncludePath, o.ExcludePath, projects, stale, staleNote, rows}
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
		if o.oneline() {
			for _, r := range res {
				ref := lastSlice8(r.SessionID) + ":"
				clean := agentproto.CleanSnippetOneline(r.Snippet)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ref, r.ISO, paths.ProjectLabel(td), clean)
			}
			return nil
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
	// agentproto.Search (via opts).
	//
	// The scope is passed as a FUNCTION, not a list: the one consolidated store
	// answers this search with a single query, and enumerating every project to
	// build a scope list it would never use costs seconds of directory walking and
	// git probing. The function is called only if the store cannot answer.
	sopts := agentproto.SearchOpts{
		Limit:            o.Limit,
		Offset:           o.Offset,
		Role:             o.Role,
		Sort:             o.Sort,
		IncludeTools:     o.IncludeTools,
		IncludeSubagents: o.IncludeSubagents,
		Since:            o.Since,
		Before:           o.Before,
		MinMessages:      o.MinMessages,
		IncludePath:      o.IncludePath,
		ExcludePath:      o.ExcludePath,
		Source:           o.Source,
		CurrentSession:   o.currentSession(),
		Oneline:          o.oneline(),
	}
	label := ""
	if o.ThisProject {
		sc, td, ok := thisScope(w, o)
		if !ok {
			return nil
		}
		// --this-project narrows the one store by project LABEL. The label is the
		// same paths.ProjectLabel value the indexer stamps on the row, so it matches
		// the column exactly; the scope list still travels for the fallback.
		sopts.Project = paths.ProjectLabel(td)
		sopts.ScopeFallback = func() []view.Scope { return sc }
		label = "on " + paths.ProjectLabel(td)
	} else {
		sopts.ScopeFallback = func() []view.Scope {
			return allScope(ctx, o.Source, o.Reindex, o.IncludePath, o.ExcludePath)
		}
		label = "across all projects"
	}

	var (
		indexStale bool
		staleNote  string
	)

	// An explicit --reindex or explicit --dir refreshes the targeted project.
	// --this-project evaluates O(1) project freshness before paying the multi-runtime
	// re-consolidation fold. Default search is answer-first: the O(1) per-project
	// freshness check skips discovery entirely, and stale or UNKNOWN answers from
	// the consolidated store while a throttled background ingest nudge repairs freshness.
	switch {
	case o.Reindex || o.DirSet:
		refreshThisProject(o)
	case o.ThisProject:
		td := resolveTDir(o.Dir, o.DirSet)
		projLabel := ""
		if td != "" {
			projLabel = paths.ProjectLabel(td)
		}
		fresh := false
		if con, _, err := index.OpenConsolidated(); err == nil {
			freshness, fErr := index.CheckProjectFreshness(con, projLabel, td, o.Source)
			_ = con.Close()
			fresh = fErr == nil && freshness.Fresh
		}
		if !fresh {
			refreshThisProject(o)
		}
	default:
		td := resolveTDir(o.Dir, o.DirSet)
		projLabel := ""
		if td != "" {
			projLabel = paths.ProjectLabel(td)
		}
		stale := false
		if con, _, err := index.OpenConsolidated(); err == nil {
			freshness, fErr := index.CheckProjectFreshness(con, projLabel, td, o.Source)
			_ = con.Close()
			stale = fErr != nil || !freshness.Fresh
		} else {
			stale = true
		}
		if stale {
			refreshThisProject(o)
			if con, _, err := index.OpenConsolidated(); err == nil {
				freshness, fErr := index.CheckProjectFreshness(con, projLabel, td, o.Source)
				_ = con.Close()
				if fErr != nil || !freshness.Fresh {
					indexStale = true
					staleNote = staleIngestNote()
				}
			} else {
				indexStale = true
				staleNote = staleIngestNote()
			}
		}
	}

	var emb embed.Embedder
	if !o.NoVector {
		emb = adapters.GetEmbedder()
	}

	var scope []view.Scope
	if o.Reindex {
		scope = sopts.ScopeFallback()
		if len(scope) == 0 {
			scope = []view.Scope{}
		}
	}

	if o.JSON {
		env := agentproto.Search(q, scope, sopts, emb)
		if indexStale {
			env.Complete = false
			env.Warnings = append(env.Warnings, agentproto.Warning{
				Code:    "index_stale",
				Message: staleNote,
				Facts:   map[string]any{"stale": true},
			})
		}
		return EmitJSON(w, struct {
			agentproto.SearchEnvelope
			Stale     bool   `json:"stale,omitempty"`
			StaleNote string `json:"stale_note,omitempty"`
		}{
			SearchEnvelope: env,
			Stale:          indexStale,
			StaleNote:      staleNote,
		})
	}

	if err := agentproto.SearchAndRender(w, q, scope, sopts, emb, label, false); err != nil {
		return err
	}
	if !o.oneline() && indexStale {
		fmt.Fprintf(w, "note: %s\n", staleNote)
	}
	return nil
}

// refreshThisProject indexes the project this command is running in, so a search
// answered from the one store still sees the session happening right now. It is
// advisory: a project with no transcript history, or an index that fails or is
// locked, leaves the store as it was — the search still runs, it just answers
// from what was already folded in. The indexing run's own write-through is what
// carries the new rows into the store.
func refreshThisProject(o *Options) {
	expDir := realpathExpand(o.Dir)
	if o.Source == "" || o.Source == "claude" {
		td := resolveTDir(o.Dir, o.DirSet)
		if td != "" && isDir(td) {
			if _, _, err := scopes.Resolve(view.Scope{Project: paths.ProjectLabel(td), TDir: td}, false); err != nil {
				slog.Debug("search: current-project refresh failed", "project", paths.ProjectLabel(td), "err", err)
			}
		}
	}
	if o.Source == "" || o.Source == "antigravity" {
		scopes.RefreshAntigravityCWD(expDir)
	}
	if (o.Source == "" || o.Source == "goose") && scopes.GooseOptedIn(o.Source) {
		scopes.RefreshGooseCWD(expDir)
	}
	if o.Source == "" || o.Source == "codex" {
		scopes.RefreshCodexCWD(expDir)
	}
	if o.Source == "" || o.Source == "pi" {
		scopes.RefreshPiCWD(expDir)
	}
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
		routine := ""
		if r.Routine {
			routine = " · routine"
		}
		// timefmt seam: search results are agent-parsed — marked UTC.
		fmt.Fprintf(w, "[%s · %s · %s%s%s] …%s…\n\n", orQ(timefmt.UTCFromISO(r.ISO)), label, r.Role, tag, routine, r.Snippet)
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
			rows = append(rows, row{len(matches), paths.ProjectLabel(sc.TDir), filepath.Base(sc.TDir), false})
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

// registeredSourceIDs returns the IDs of all registered source adapters.
func registeredSourceIDs() []string {
	regs := sources.Registered()
	out := make([]string, len(regs))
	for i, r := range regs {
		out[i] = r.ID
	}
	return out
}

// staleIngestNote returns the status note when an index is stale, triggering a background ingest.
func staleIngestNote() string {
	if maybeSpawnIngest("") {
		return "sessions not yet ingested — background ingest triggered"
	}
	return "sessions not yet ingested — run 'rawclaw ingest' to refresh"
}

func freshnessUnknownNote() string {
	if maybeSpawnIngest("") {
		return "index freshness unknown — background ingest triggered"
	}
	return "index freshness unknown — run 'rawclaw ingest' to establish it"
}

// sessionStaleNote resolves the staleness state and note for a specific session.
func sessionStaleNote(sf index.SessionFreshness) (bool, string) {
	if sf.Status == index.SessionStale {
		if maybeSpawnIngest(sf.SessionID) {
			return true, "session may be stale (transcript updated) — background ingest triggered"
		}
		return true, sf.Note
	}
	return false, sf.Note
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
	return cmp.Or(s, "?")
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
