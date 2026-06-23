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

// nullableStr maps a Go "" (what a NULL parent_id scans to) back to JSON null.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// BrowseRow is one recent-session preview row.
type BrowseRow struct {
	SessionID string  `json:"session_id"`
	LastTS    float64 `json:"last_ts"`
	N         int     `json:"n"`
	Preview   string  `json:"preview"`
}

// Scope is a (project label, transcript dir) pair — the unit discovery/scroll
// iterate over.
type Scope struct {
	Project string
	TDir    string
}

// AnchoredViewOpts groups the optional tuning of AnchoredView (window radius,
// bookend size, tool inclusion) to keep the signature small.
// Defaults: Window=5, Bookend=3, IncludeTools=false.
type AnchoredViewOpts struct {
	Window       int
	Bookend      int
	IncludeTools bool
}

// rawMsg is the (id, role, content) triple read from the messages table.
type rawMsg struct {
	ID      int
	Role    string
	Content string
}

// BuildAnchoredView builds the ±window + bookends shape around anchorID in
// session. Returns nil if the window is empty.
// (Named BuildAnchoredView, not AnchoredView, to avoid colliding with the
// AnchoredView result type.)
func BuildAnchoredView(con *sql.DB, sessionID string, anchorID int, opts AnchoredViewOpts) *AnchoredView {
	// before: id<=anchor ORDER BY id DESC LIMIT window+1 (then reversed to ASC).
	before, err := readMsgs(con,
		`SELECT id,role,content FROM messages WHERE session_id=? AND id<=? ORDER BY id DESC LIMIT ?`,
		sessionID, anchorID, opts.Window+1)
	if err != nil {
		return nil
	}
	after, err := readMsgs(con,
		`SELECT id,role,content FROM messages WHERE session_id=? AND id>? ORDER BY id ASC LIMIT ?`,
		sessionID, anchorID, opts.Window)
	if err != nil {
		return nil
	}

	// win = reversed(before) + after (both ascending by id).
	win := make([]rawMsg, 0, len(before)+len(after))
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

	var bs, be []rawMsg
	if opts.Bookend > 0 {
		bs, _ = readMsgs(con,
			`SELECT id,role,content FROM messages WHERE session_id=? AND id<? AND role IN ('user','assistant') AND length(content)>0 ORDER BY id ASC LIMIT ?`,
			sessionID, winMin, opts.Bookend)
		be, _ = readMsgs(con,
			`SELECT id,role,content FROM messages WHERE session_id=? AND id>? AND role IN ('user','assistant') AND length(content)>0 ORDER BY id DESC LIMIT ?`,
			sessionID, winMax, opts.Bookend)
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

// readMsgs runs a (id, role, content) query and scans the rows.
func readMsgs(con *sql.DB, query string, args ...any) ([]rawMsg, error) {
	rows, err := con.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rawMsg
	for rows.Next() {
		var m rawMsg
		if err := rows.Scan(&m.ID, &m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
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
	con, err := index.ConnectRO(dbp)
	if err != nil {
		return nil
	}
	defer con.Close()

	where := []string{"s.is_subagent=0"}
	var args []any
	if since != "" {
		where = append(where, "date(s.last_ts,'unixepoch','localtime') >= ?")
		args = append(args, since)
	}
	if before != "" {
		where = append(where, "date(s.last_ts,'unixepoch','localtime') <= ?")
		args = append(args, before)
	}
	args = append(args, limit)

	whereSQL := where[0]
	for _, w := range where[1:] {
		whereSQL += " AND " + w
	}
	q := `SELECT s.id, s.last_ts, s.message_count
	      FROM sessions s WHERE ` + whereSQL + ` ORDER BY s.last_ts DESC LIMIT ?`

	rows, err := con.Query(q, args...)
	if err != nil {
		return nil
	}

	// Drain the session rows fully and CLOSE them before running any per-session
	// preview query. ConnectRO is a single-connection pool (SetMaxOpenConns(1)), so
	// calling sessionPreview (another con.Query) while these rows are still open
	// blocks forever waiting for a second connection — database/sql.(*DB).conn
	// deadlock. Collect first, release the connection, then preview.
	var out []BrowseRow
	for rows.Next() {
		var (
			id     string
			lastTS sql.NullFloat64
			n      sql.NullInt64
		)
		if err := rows.Scan(&id, &lastTS, &n); err != nil {
			_ = rows.Close()
			return nil
		}
		out = append(out, BrowseRow{SessionID: id, LastTS: lastTS.Float64, N: int(n.Int64)})
	}
	rowsErr := rows.Err()
	_ = rows.Close() // release the single connection before the preview queries
	if rowsErr != nil {
		return nil
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
	rows, err := con.Query(
		`SELECT content FROM messages WHERE session_id=? AND role='user'
		   AND length(content)>0 ORDER BY id ASC LIMIT ?`,
		sessionID, previewScan)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var fallback string
	for rows.Next() {
		var content sql.NullString
		if err := rows.Scan(&content); err != nil {
			return fallback
		}
		if fallback == "" {
			fallback = parse.Disp(content.String, false, browsePreviewCap)
		}
		if parse.IsSubstantive(content.String) {
			return parse.Disp(content.String, false, browsePreviewCap)
		}
	}
	if err := rows.Err(); err != nil {
		return fallback
	}
	return fallback
}
