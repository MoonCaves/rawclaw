package agentproto

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/MoonCaves/rawclaw/internal/view"
)

const freshnessNote = "raw session history — verify against current state before acting."

func SearchAndRender(
	w io.Writer,
	query string,
	scope []view.Scope,
	opts SearchOpts,
	embedder embed.Embedder,
	scopeLabel string,
	wantJSON bool,
) error {
	env := Search(query, scope, opts, embedder)
	if wantJSON {
		return emit(w, env)
	}
	if opts.Oneline {
		RenderSearchOneline(w, env)
		return nil
	}
	renderSearch(w, env, query, scopeLabel)
	return nil
}

func ReadAndRender(
	w io.Writer,
	ref string,
	scope []view.Scope,
	more ScopeFn,
	opts ReadOpts,
	wantJSON bool,
) error {
	opts.ScopeFallback = more
	result, err := Read(ref, scope, opts)
	if err != nil {
		return err
	}
	if wantJSON {
		return emit(w, result)
	}
	renderRead(w, result)
	return nil
}

func OutlineAndRender(w io.Writer, session8 string, scope []view.Scope, more ScopeFn, opts OutlineOpts, wantJSON bool) error {
	opts.ScopeFallback = more
	result, err := outline(session8, scope, more, opts)
	if err != nil {
		return err
	}
	if wantJSON {
		return emit(w, result)
	}
	renderOutline(w, result)
	return nil
}

func renderSearch(w io.Writer, env SearchEnvelope, query, scopeLabel string) {
	if len(env.Results) == 0 {
		if env.ExcludedCurrentTurn > 0 {
			fmt.Fprintf(w, "No matches outside the turn you are in now — %d record(s) of it matched and were withheld; the rest of that session was searched.\n", env.ExcludedCurrentTurn)
			fmt.Fprintln(w, "To see them anyway, re-run with --current-session off.")
			renderWarnings(w, env.Warnings, WarnCurrentTurnExcluded)
			return
		}
		fmt.Fprintln(w, "No matches · answers from local store; refreshes in background. Lead with a single distinctive term that appears in the text (a filename, flag, or error string), not a topic word — or rephrase.")
		renderWarnings(w, env.Warnings, "")
		return
	}
	fmt.Fprintf(w, "%d conversation(s) matching '%s' %s · answers from local store; refreshes in background:\n\n", len(env.Results), query, scopeLabel)
	for _, r := range env.Results {
		iso := timefmt.UTCShortFromISO(r.ISO)
		if iso == "" {
			iso = "?"
		}
		miss := ""
		if r.Missing {
			miss = " · source file gone — retained history"
		}
		routine := ""
		if r.Routine {
			routine = " · routine"
		}
		topic := ""
		if r.Topic != "" {
			topic = fmt.Sprintf(" · %q", r.Topic)
		}
		fmt.Fprintf(w, "  ━━ %s · %s · %s%s%s%s\n", iso, sid8(r.SessionID), r.Project, topic, miss, routine)
		fmt.Fprintf(w, "     …%s…\n", r.Snippet)
		if r.Last != "" {
			fmt.Fprintf(w, "     now → %s\n", r.Last)
		}
		fmt.Fprintf(w, "     read ref=%s\n", r.ReadRef)
		fmt.Fprintln(w)
	}
	if env.HasMore {
		total := strconv.Itoa(env.TotalMatches)
		if env.TotalIsLowerBound {
			total = "≥" + total
		}
		fmt.Fprintf(w, "showing %d of %s matches — see more: %s\n", env.Count, total, env.NextCommand)
	}
	renderWarnings(w, env.Warnings, "")
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\].*?\x07|\x1b[@-Z\\-_]`)

func CleanSnippetOneline(s string) string {
	s = ansiRegex.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 32 || r == 127 || unicode.IsControl(r) {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func RenderSearchOneline(w io.Writer, env SearchEnvelope) {
	for _, r := range env.Results {
		clean := CleanSnippetOneline(r.Snippet)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.ReadRef, r.ISO, r.Project, clean)
	}
}

func renderWarnings(w io.Writer, ws []Warning, suppress string) {
	for _, warn := range ws {
		if suppress != "" && warn.Code == suppress {
			continue
		}
		fmt.Fprintf(w, "note: %s\n", warn.Message)
	}
}

func currentTurnLine(n int) string {
	return fmt.Sprintf("excluded %d record(s) of the turn you are in now; the rest of that session was searched", n)
}

func renderRead(w io.Writer, r *ReadResult) {
	v := r.AnchoredView
	truncNote := ""
	if r.Truncated {
		truncNote = trimNote(r)
	}
	fmt.Fprintf(w, "━━ %s · %s · anchor #%d (%d before / %d after) ━━\n",
		sid8(r.SessionID), r.Project, r.AnchorID, v.MessagesBefore, v.MessagesAfter)
	if r.FocusSnippet != "" {
		fmt.Fprintf(w, "  focus match: %s\n", r.FocusSnippet)
		fmt.Fprintln(w)
	}
	sections := []struct {
		msgs  []view.ViewMsg
		label string
	}{
		{v.BookendStart, "─ session start ─"},
		{v.Window, ""},
		{v.BookendEnd, "─ session end ─"},
	}
	for _, sec := range sections {
		if len(sec.msgs) > 0 && sec.label != "" {
			fmt.Fprintf(w, "  %s\n", sec.label)
		}
		for _, m := range sec.msgs {
			star := " "
			if m.Anchor {
				star = "▶"
			}
			fmt.Fprintf(w, "     %s [%s #%d] %s\n", star, m.Role, m.ID, m.Text)
		}
	}
	if truncNote != "" {
		fmt.Fprintln(w, truncNote)
	} else {
		fmt.Fprintf(w, "\n  keep reading:  rawclaw read %s --more   (or --around N to shift)\n", r.ReadRef)
	}
	if len(r.Subagents) > 0 {
		fmt.Fprintln(w, "  ─ subagents ─")
		for _, s := range r.Subagents {
			fmt.Fprintf(w, "     %s (%d msgs)\n", sid8(s.SessionID), s.MessageCount)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "note: %s\n", freshnessNote)
}

func renderOutline(w io.Writer, r *OutlineResult) {
	iso := r.ISO
	if iso == "" {
		iso = "?"
	}
	fmt.Fprintf(w, "━━ %s · %s · %s · %d messages ━━\n\n",
		iso, sid8(r.SessionID), r.Project, r.MessageCount)
	if len(r.Topics) > 0 {
		fmt.Fprintf(w, "  topics: %s\n\n", strings.Join(r.Topics, " · "))
	}
	fmt.Fprintln(w, "  ── GOAL (session opening) ──")
	for _, m := range r.Start {
		fmt.Fprintf(w, "     [%s #%d] %s\n", m.Role, m.ID, m.Text)
	}
	if r.MidCount > 0 {
		fmt.Fprintf(w, "\n  … %d messages in between …\n\n", r.MidCount)
	}
	if len(r.End) > 0 {
		fmt.Fprintln(w, "  ── RESOLUTION (session close) ──")
		for _, m := range r.End {
			fmt.Fprintf(w, "     [%s #%d] %s\n", m.Role, m.ID, m.Text)
		}
	}
	if len(r.Subagents) > 0 {
		fmt.Fprintln(w, "\n  ── SUBAGENTS ──")
		for _, s := range r.Subagents {
			fmt.Fprintf(w, "     %s (%d msgs)\n", sid8(s.SessionID), s.MessageCount)
		}
	}
}
