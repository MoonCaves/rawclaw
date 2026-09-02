package agentproto

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

type ReadOpts struct {
	Focus            string
	Budget           *int
	IncludeTools     bool
	IncludeThinking  bool
	IncludeSubagents bool
	Window           int
	Around           int
	ScopeFallback    ScopeFn
}

func MoreWindow(level int) int {
	return moreWindow(level)
}

func moreWindow(level int) int {
	if level <= 0 {
		return ReadWindow
	}
	return (level + 1) * ReadWindow
}

func Read(ref string, scope []view.Scope, opts ReadOpts) (*ReadResult, error) {
	session8, uuid8, err := resolveRef(ref)
	if err != nil {
		return nil, err
	}
	dbp, fullSID, proj, locErr := locateSession(scope, opts.ScopeFallback, session8)
	if locErr != nil {
		return nil, locErr
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", dbp, err)
	}
	defer con.Close()

	msgID, err := resolveUUID(con, fullSID, uuid8)
	if err != nil {
		return nil, err
	}

	window := opts.Window
	if window <= 0 {
		window = ReadWindow
	}
	center := msgID + opts.Around

	av := view.BuildAnchoredView(con, fullSID, center, view.AnchoredViewOpts{
		Window:          window,
		Bookend:         readBookend,
		IncludeTools:    opts.IncludeTools,
		IncludeThinking: opts.IncludeThinking,
	})
	if av == nil {
		return nil, fmt.Errorf("message %q not found in session %q", uuid8, session8)
	}

	st := applyBudget(av, opts.Budget)
	focusSnippet := focusHighlight(av.Window, opts.Focus)

	var subagents []SubagentInfo
	if opts.IncludeSubagents {
		if subs, err := store.SubagentsForSession(con, fullSID); err == nil {
			for _, s := range subs {
				subagents = append(subagents, SubagentInfo{
					SessionID:    s.ID,
					MessageCount: s.MessageCount,
				})
			}
		}
	}

	nextCmd := ""
	if st.Truncated {
		nextCmd = "rawclaw read " + fmtRef(fullSID, uuid8) + " --more"
	}

	return &ReadResult{
		Project:      proj,
		SessionID:    fullSID,
		AnchorID:     msgID,
		FocusSnippet: focusSnippet,
		CharBudget:   opts.Budget,
		ReadRef:      fmtRef(fullSID, uuid8),
		Truncated:    st.Truncated,
		TrimmedChars: st.OmittedChars,
		TrimmedMsgs:  st.OmittedMsgs,
		NextCommand:  nextCmd,
		Subagents:    subagents,
		AnchoredView: av,
	}, nil
}

type ErrMsgNotFound struct{ UUID8 string }

func (e *ErrMsgNotFound) Error() string {
	return fmt.Sprintf("message %q not found in session", e.UUID8)
}

type ErrAmbiguousUUID struct{ UUID8 string }

func (e *ErrAmbiguousUUID) Error() string {
	return fmt.Sprintf("ambiguous message ref %q — matches multiple messages; give a longer uuid prefix", e.UUID8)
}

func resolveUUID(con *sql.DB, fullSID, uuid8 string) (int, error) {
	ids, err := store.ResolveMessageUUID(con, fullSID, uuid8, 2)
	if err != nil {
		return 0, fmt.Errorf("resolve message %q: %w", uuid8, err)
	}
	switch len(ids) {
	case 0:
		return 0, &ErrMsgNotFound{UUID8: uuid8}
	case 1:
		return ids[0], nil
	default:
		return 0, &ErrAmbiguousUUID{UUID8: uuid8}
	}
}

type trimStat struct {
	Truncated    bool
	OmittedChars int
	OmittedMsgs  int
}

func applyBudget(av *view.AnchoredView, budget *int) trimStat {
	if budget == nil {
		return trimStat{}
	}
	maxChars := *budget
	total := 0
	st := trimStat{}
	capped := make([]view.ViewMsg, 0, len(av.Window))
	for i, m := range av.Window {
		if total >= maxChars {
			st.Truncated = true
			for _, dropped := range av.Window[i:] {
				st.OmittedChars += runeLen(dropped.Text)
				st.OmittedMsgs++
			}
			break
		}
		available := maxChars - total
		orig := runeLen(m.Text)
		t := runeSlice(m.Text, available)
		if orig > available {
			t = strings.TrimRight(t, " \t\n\r\f\v") + truncateMarker
			st.Truncated = true
			kept := runeLen(strings.TrimSuffix(t, truncateMarker))
			st.OmittedChars += orig - kept
		}
		nm := m
		nm.Text = t
		capped = append(capped, nm)
		total += runeLen(t)
	}
	av.Window = capped
	return st
}

func focusHighlight(window []view.ViewMsg, focus string) string {
	if focus == "" {
		return ""
	}
	needle := strings.ToLower(focus)
	highlight := regexp.MustCompile("(?i)(" + regexp.QuoteMeta(focus) + ")")
	for _, m := range window {
		idx := strings.Index(strings.ToLower(m.Text), needle)
		if idx < 0 {
			continue
		}
		runeIdx := runeLen(m.Text[:idx])
		s := max(runeIdx-60, 0)
		chunk := runeRange(m.Text, s, runeIdx+120)
		return fmt.Sprintf("[#%d %s] %s", m.ID, m.Role, highlight.ReplaceAllString(chunk, ">>>$1<<<"))
	}
	return ""
}

func runeRange(s string, lo, hi int) string {
	r := []rune(s)
	lo = max(lo, 0)
	if hi > len(r) {
		hi = len(r)
	}
	if lo >= hi {
		return ""
	}
	return string(r[lo:hi])
}

func trimNote(r *ReadResult) string {
	parts := "+" + fmtChars(r.TrimmedChars) + " chars"
	if r.TrimmedMsgs > 0 {
		parts += fmt.Sprintf(" · %d msgs hidden", r.TrimmedMsgs)
	}
	next := r.NextCommand
	if next == "" {
		next = "rawclaw read " + sid8(r.SessionID) + ":" + " --more"
	}
	return "  [" + parts + " — " + next + "]"
}

func fmtChars(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return strconv.Itoa(n)
}
