// Package cli is the thin composition root: the cobra command tree, flag
// wiring, the flat-output printers, and the JSON emitters. The engine lives in
// the sibling packages (parse, paths, index, query, retrieve, view, render,
// semantic, adapters, agentproto).
package cli

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/semantic"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

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

	bindRootFlags(root, opts)
	f := root.Flags()
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
	if !dirSet || td != dir {
		if gitRoot := paths.GitRoot(dir); gitRoot != "" {
			all := allScope(ctx, "", false)
			matched := scopes.FilterByProjectDir(all, dir)
			if len(matched) > 0 {
				return matched, nil
			}
		}
	}
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

	if machineStream(out) && !o.JSON {
		o.Oneline = true
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
