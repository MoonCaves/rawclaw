// Package view does result-shaping: the bookends+window anchored view, org-wide
// discovery (lineage-deduped, RRF-fused when an embedder is wired), scroll
// (keep-reading), and browse (no-query recent sessions).
//
// Ordering within a session is by message id (insertion order), NOT ts — ts can
// be non-monotonic, so id is the reliable ordering key.
package view

import (
	"database/sql"
	"sort"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// dispCap is the default display-text cap used by anchored views and discovery.
const dispCap = 200

// browsePreviewCap is the display-text cap for browse() preview snippets.
const browsePreviewCap = 120

// ViewMsg is one message in a window or bookend. The Anchor field is true only
// for the window's anchor message.
type ViewMsg struct {
	ID     int    `json:"id"`
	Role   string `json:"role"`
	Text   string `json:"text"`
	Anchor bool   `json:"anchor,omitempty"`
}

// AnchoredView is the goal→match→resolution shape around one anchor message.
type AnchoredView struct {
	BookendStart   []ViewMsg `json:"bookend_start"`
	Window         []ViewMsg `json:"window"`
	BookendEnd     []ViewMsg `json:"bookend_end"`
	MessagesBefore int       `json:"messages_before"`
	MessagesAfter  int       `json:"messages_after"`
}

// BrowseRow is one recent-session preview row.
type BrowseRow struct {
	SessionID string  `json:"session_id"`
	LastTS    float64 `json:"last_ts"`
	N         int     `json:"n"`
	Preview   string  `json:"preview"`
}

// Scope is one searchable unit discovery/scroll iterate over. A Claude scope is
// a (project label, transcript dir) pair whose db is resolved lazily from TDir.
// A non-directory source (e.g. Codex) instead sets DBP (a pre-ensured db) and
// CWD (for path filtering), leaving TDir empty. Source names the runtime
// ("claude"/"codex") for the --source filter and display. Resolve/CWD in the
// scopes package pick the right field, so consumers stay source-agnostic.
type Scope struct {
	Project string
	TDir    string // Claude transcript dir; "" for a pre-resolved (DBP) scope
	DBP     string // pre-ensured db path; "" means resolve lazily from TDir
	CWD     string // working dir for path filtering; "" means derive from TDir
	Source  string // "claude" | "codex"
}

// AnchoredViewOpts groups the optional tuning of AnchoredView (window radius,
// bookend size, tool inclusion) to keep the signature small.
// Defaults: Window=5, Bookend=3, IncludeTools=false.
type AnchoredViewOpts struct {
	Window       int
	Bookend      int
	IncludeTools bool
}

// BuildAnchoredView builds the ±window + bookends shape around anchorID in
// session. Returns nil if the window is empty.
// (Named BuildAnchoredView, not AnchoredView, to avoid colliding with the
// AnchoredView result type.)
func BuildAnchoredView(con *sql.DB, sessionID string, anchorID int, opts AnchoredViewOpts) *AnchoredView {
	// before: id<=anchor, nearest first (id DESC), then reversed to ASC below.
	before, err := store.MessagesBefore(con, sessionID, anchorID, opts.Window+1)
	if err != nil {
		return nil
	}
	after, err := store.MessagesAfter(con, sessionID, anchorID, opts.Window)
	if err != nil {
		return nil
	}

	// win = reversed(before) + after (both ascending by id).
	win := make([]store.Msg, 0, len(before)+len(after))
	for i := len(before) - 1; i >= 0; i-- {
		win = append(win, before[i])
	}
	win = append(win, after...)
	if len(win) == 0 {
		return nil
	}
	winMin, winMax := win[0].ID, win[len(win)-1].ID

	wmsgs := make([]ViewMsg, 0, len(win))
	for _, m := range win {
		isAnchor := m.ID == anchorID
		if !opts.IncludeTools && m.Role != "user" && m.Role != "assistant" && !isAnchor {
			continue
		}
		// The anchored message is the one the agent chose to read — render it WHOLE
		// (cap -1 = no truncation). Neighbors stay snippets (dispCap) for context
		// without dumping the window. --more widens; --budget caps if needed.
		cap := dispCap
		if isAnchor {
			cap = -1
		}
		text := parse.Disp(m.Content, opts.IncludeTools, cap)
		if text == "" && !isAnchor { // skip empty turns — keep the anchor
			continue
		}
		wmsgs = append(wmsgs, ViewMsg{ID: m.ID, Role: m.Role, Text: text, Anchor: isAnchor})
	}

	var bs, be []store.Msg
	if opts.Bookend > 0 {
		// bookend_start: the run-up before the window (id<winMin, ASC).
		bs, _ = store.BookendMessages(con, sessionID, winMin, true, true, opts.Bookend)
		// bookend_end: the tail after the window (id>winMax, DESC — reversed below).
		be, _ = store.BookendMessages(con, sessionID, winMax, true, false, opts.Bookend)
	}

	bookendStart := make([]ViewMsg, 0, len(bs))
	for _, m := range bs {
		bookendStart = append(bookendStart, ViewMsg{ID: m.ID, Role: m.Role, Text: parse.Disp(m.Content, opts.IncludeTools, dispCap)})
	}
	// bookend_end: emit reversed(be) (be is DESC, so output is ASC by id).
	bookendEnd := make([]ViewMsg, 0, len(be))
	for i := len(be) - 1; i >= 0; i-- {
		m := be[i]
		bookendEnd = append(bookendEnd, ViewMsg{ID: m.ID, Role: m.Role, Text: parse.Disp(m.Content, opts.IncludeTools, dispCap)})
	}

	messagesBefore := len(before) - 1
	if messagesBefore < 0 {
		messagesBefore = 0
	}
	return &AnchoredView{
		BookendStart:   bookendStart,
		Window:         wmsgs,
		BookendEnd:     bookendEnd,
		MessagesBefore: messagesBefore,
		MessagesAfter:  len(after),
	}
}

// sortCandidates orders discovery candidates per the requested sort mode.
//
//	newest: by iso desc (empty iso sinks)
//	oldest: by iso asc  (empty iso floats)
//	"":     relevance — fused desc, then cov desc, then rank asc
//
// sort.SliceStable keeps the ordering stable for equal keys.
func sortCandidates(cands []retrieve.Anchor, mode string) {
	switch mode {
	case "newest":
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].ISO > cands[j].ISO })
	case "oldest":
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].ISO < cands[j].ISO })
	default:
		sort.SliceStable(cands, func(i, j int) bool {
			a, b := cands[i], cands[j]
			if a.Fused != b.Fused {
				return a.Fused > b.Fused
			}
			if a.Cov != b.Cov {
				return a.Cov > b.Cov
			}
			return a.Rank < b.Rank
		})
	}
}

// Browse returns a project's most-recent top-level sessions (no query).
// since/before ("" = no bound) are local-date filters on last_ts.
func Browse(tdir string, limit int, since, before string) []BrowseRow {
	dbp, _, _, err := index.EnsureIndexed(tdir, false)
	if err != nil {
		return nil
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return nil
	}
	defer con.Close()

	// BrowseSessions drains and closes its rows before returning (D3), so the
	// single connection (ConnectRO sets SetMaxOpenConns(1)) is free for the
	// per-session preview queries below. Running a preview query while the
	// session rows were still open was the v0.1.0 database/sql.(*DB).conn
	// deadlock — sessions first, then previews, never interleaved.
	sessions, err := store.BrowseSessions(con, since, before, limit)
	if err != nil {
		return nil
	}

	out := make([]BrowseRow, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, BrowseRow{SessionID: s.SessionID, LastTS: s.LastTS, N: s.MessageCount})
	}

	// Connection is now free — fill each preview with its own query.
	for i := range out {
		out[i].Preview = sessionPreview(con, out[i].SessionID)
	}
	return out
}

// previewScan is how many early user messages sessionPreview considers before
// giving up — enough to skip a warmup ('hi') / '/clear' opener and reach the
// first substantive turn, without scanning a whole session.
const previewScan = 8

// sessionPreview returns the browse preview for a session: the first SUBSTANTIVE
// user message's display text (low-signal openers like 'hi' or '/clear' are
// skipped via parse.IsSubstantive). The session is never dropped — if no early
// message is substantive, the first non-empty user message is shown as a
// fallback so the row still previews something.
func sessionPreview(con *sql.DB, sessionID string) string {
	contents, err := store.FirstUserMessages(con, sessionID, previewScan)
	if err != nil {
		return ""
	}

	var fallback string
	for _, content := range contents {
		if fallback == "" {
			fallback = parse.Disp(content, false, browsePreviewCap)
		}
		if parse.IsSubstantive(content) {
			return parse.Disp(content, false, browsePreviewCap)
		}
	}
	return fallback
}
