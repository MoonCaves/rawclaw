package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// Options holds every parsed flag for one rawclaw invocation, bound to the
// cobra root command.
type Options struct {
	Limit            int
	Offset           int
	Query            string
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
	Until            string
	Days             int
	Today            bool
	Yesterday        bool
	Week             bool
	Vector           bool
	NoVector         bool
	ReindexVectors   bool
	IncludePath      string
	ExcludePath      string
	MinMessages      int
	DebugSearch      bool
	Timeout          time.Duration
	DirSet           bool
	Oneline          bool
	Format           string
	CurrentSession   string
}

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

func (o *Options) oneline() bool {
	return o.Oneline || o.Format == "oneline" || o.Format == "line"
}

func machineStream(w io.Writer) bool {
	for _, env := range currentSessionEnvs {
		if strings.TrimSpace(os.Getenv(env)) != "" {
			return true
		}
	}
	f, ok := w.(*os.File)
	return ok && !isatty.IsTerminal(f.Fd()) && !isatty.IsCygwinTerminal(f.Fd())
}

var currentSessionEnvs = []string{
	"CLAUDE_CODE_SESSION_ID",
	"ANTIGRAVITY_CONVERSATION_ID",
}

type sessionEnvMatch struct {
	env string
	sid string
}

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

func (o *Options) pathScoped() bool {
	return o.IncludePath != "" || o.ExcludePath != ""
}

func (o *Options) params(rawMatch string) retrieve.SearchParams {
	return retrieve.SearchParams{
		Role: o.Role, Sort: o.Sort, IncludeTools: o.IncludeTools,
		IncludeSubagents: o.IncludeSubagents, Since: o.Since, Before: o.Before,
		Offset: o.Offset, RawMatch: rawMatch, MinMessages: o.MinMessages,
	}
}

// bindRootFlags defines the root flags. Keeping the definitions together
// preserves the root command's public flag surface while leaving command
// composition in cli.go.
func bindRootFlags(root *cobra.Command, opts *Options) {
	f := root.Flags()
	f.IntVarP(&opts.Limit, "limit", "n", 8, "max hits to return")
	f.StringVarP(&opts.Query, "query", "q", "", "search query terms (flag alternative to positional args); quote multi-word phrases (e.g. \"exact phrase\") for literal phrase matching")
	f.IntVar(&opts.Offset, "offset", 0, "skip the first N hits (pagination)")
	f.StringVar(&opts.Dir, "dir", cwd(), "the project's working directory (e.g. ~/code/my-project); encoded to find its transcripts. An already-encoded ~/.claude/projects path also works.")
	f.BoolVar(&opts.ThisProject, "this-project", false, "narrow to THIS project only (default searches all projects)")
	f.Bool("this-desk", false, "")
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
	f.BoolVar(&opts.Vector, "vector", false, "enable semantic/vector search hybrid tier (opt-in; needs configured embedder)")
	f.BoolVar(&opts.NoVector, "no-vector", false, "force keyword-only (deprecated; keyword-only is now the default)")
	_ = f.MarkHidden("no-vector")
	f.BoolVar(&opts.ReindexVectors, "reindex-vectors", false, "build/update the semantic index for the scope (needs RAWCLAW_EMBED_ENDPOINT)")
	f.StringVar(&opts.IncludePath, "include-path", "", "only cover projects whose working dir matches this regex (search AND bare browse)")
	f.StringVar(&opts.ExcludePath, "exclude-path", "", "skip projects whose working dir matches this regex, e.g. /tmp (search AND bare browse)")
	f.IntVar(&opts.MinMessages, "min-messages", 0, "only sessions with >= N messages (drops thin/bootstrap threads)")
	f.BoolVar(&opts.DebugSearch, "debug-search", false, "explain WHY each hit ranked where it did (LLM-free scoring breakdown)")
	_ = f.MarkHidden("debug-search")
	f.BoolVar(&opts.Oneline, "oneline", false, "output search hits in one-line format (<read_ref>\t<started_iso>\t<project>\t<snippet>)")
	f.StringVar(&opts.Format, "format", "", "output format (text|oneline|line|json)")
	f.StringVar(&opts.CurrentSession, "current-session", "", "the session you are searching FROM (id or 8-char prefix); its CURRENT TURN — the prompt just typed and this turn's tool output — is withheld, since it is not recall and it outranks the archive on its own words. That session's earlier history stays searchable. Defaults to $CLAUDE_CODE_SESSION_ID or $ANTIGRAVITY_CONVERSATION_ID; pass `off` to search your own live turn too.")
}

func PrintResults(w io.Writer, res []retrieve.Hit, nSessions int) {
	if len(res) == 0 {
		fmt.Fprintln(w, "No matches. (Default searches top-level human text only — try --include-subagents and/or --include-tools to widen, or rephrase: keyword > full sentence.)")
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
		fmt.Fprintf(w, "[%s · %s · %s%s%s] …%s…\n\n", orQ(timefmt.UTCFromISO(r.ISO)), label, r.Role, tag, routine, r.Snippet)
	}
}

func EmitJSON(w io.Writer, obj any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(obj); err != nil {
		return fmt.Errorf("emit json: %w", err)
	}
	return nil
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

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
