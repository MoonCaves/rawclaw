// Package view does result-shaping: the bookends+window anchored view, org-wide
// discovery (lineage-deduped, RRF-fused when an embedder is wired), scroll
// (keep-reading), and browse (no-query recent sessions).
//
// Ordering within a session is by message id (insertion order), NOT ts — ts can
// be non-monotonic, so id is the reliable ordering key.
package view

import (
	"database/sql"
	"slices"
	"sort"
	"strings"

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

// BrowseRow is one recent-session row.
//
// Preview and Last answer two different questions and neither substitutes for
// the other. Preview is the session's OPENING — what it was set up to do, which
// is its identity and stays true forever. Last is its most recent real activity
// — what it is doing NOW, which is the whole point of a recency-ordered list.
//
// Keeping both is OpenClaw's split: its session store carries `firstUserMessage`
// and `lastMessagePreview` as separate fields and its session-list tool exposes
// them as `derivedTitle` and `lastMessagePreview` (src/gateway/session-utils.fs.ts,
// src/agents/tools/sessions-list-tool.ts). Showing only the opening — which is
// what this did, and what Hermes' `sessions list` still does — answers "what is
// this desk saying right now" with the prompt it was given a thousand messages
// ago.
type BrowseRow struct {
	SessionID string  `json:"session_id"`
	LastTS    float64 `json:"last_ts"`
	N         int     `json:"n"`
	Preview   string  `json:"preview"`
	Last      string  `json:"last,omitempty"`
}

// Scope is one searchable unit discovery/scroll iterate over. A Claude scope is
// a (project label, transcript dir) pair whose db is resolved lazily from TDir.
// A non-directory source (e.g. Codex) instead sets DBP (a pre-ensured db) and
// CWD (for path filtering), leaving TDir empty. Source names the runtime
// ("claude"/"codex") for the --source filter and display. Resolve/CWD in the
// scopes package pick the right field, so consumers stay source-agnostic.
//
// A scope replicated from ANOTHER machine (a transcript archive dir) also
// carries its origin: Origin is the owning machine's stable id (what its rows
// stamp as origin_machine) and OriginName its human-readable machine dir name.
// Both empty = a local scope. Stale marks a replica whose last sync is old —
// the search layer reports it through the stale-fallback posture while still
// serving its results.
type Scope struct {
	Project    string
	TDir       string // Claude transcript dir; "" for a pre-resolved (DBP) scope
	DBP        string // pre-ensured db path; "" means resolve lazily from TDir
	CWD        string // working dir for path filtering; "" means derive from TDir
	Source     string // "claude" | "codex"
	Origin     string // owning machine id for a replicated scope; "" = local
	OriginName string // owning machine display name; "" = local
	Stale      bool   // replica may lag its origin — report, still serve
}

// bookendScan bounds how far past the requested bookend size we read looking
// for records that are actually conversation. A session's first rows are the
// worst case in the whole corpus for this: a runtime injects the handbook, the
// environment block and a slash-command echo before the human's first word, and
// a redacting model then writes bare [THINKING] rows between every turn. Reusing
// the browse scan bound (lastActivityScan, measured at 40 against five live
// stores) keeps one measured number in this file instead of two guesses.
const bookendScan = 40

// bookendFetch is how many rows to read to satisfy a bookend of opts.Bookend
// displayable ones. Asking for tools or thinking back means fewer rows get dropped,
// so the fetch is exact or bounded by bookendScan.
func bookendFetch(opts AnchoredViewOpts) int {
	if opts.IncludeTools {
		return opts.Bookend
	}
	return max(opts.Bookend, bookendScan)
}

// IsDisplayable reports whether a record still says something after everything
// a runtime generated is removed. It is the display-side twin of the rule
// search already applies to its haystack, and it is the single place that rule
// lives for rendering: strip tool runs and injected envelopes, then reject what
// is left if it is empty, a bare block label (the ~99.5% redacted-thinking
// case), or the runtime's interruption note.
//
// Nothing is deleted or hidden from the store by this — `--include-tools` puts
// every dropped row back, exactly as it does for search.
func IsDisplayable(content string) bool {
	if strings.TrimSpace(parse.StripGenerated(content)) == "" {
		return false
	}
	if IsInterruptionMarker(content) {
		return false
	}
	return !isBareBlockMarker(parse.Disp(content, false, -1))
}

// IsDisplayableWith reports whether a record still says something according to
// the requested tool and thinking inclusions.
func IsDisplayableWith(content string, includeTools, includeThinking bool) bool {
	if includeTools {
		return strings.TrimSpace(content) != ""
	}
	t := parse.DispWith(content, includeTools, includeThinking, -1)
	if strings.TrimSpace(t) == "" {
		return false
	}
	if IsInterruptionMarker(content) {
		return false
	}
	return !isBareBlockMarker(t)
}

// FilterDisplayable keeps the first want displayable records of msgs, in the
// order given. Callers that want the records nearest the END of a session pass
// them newest-first and reverse the result. With includeTools set nothing is
// dropped and this is a plain take-want.
func FilterDisplayable(msgs []store.Msg, want int, includeTools bool) []store.Msg {
	return FilterDisplayableWith(msgs, want, includeTools, false)
}

// FilterDisplayableWith keeps the first want displayable records of msgs with
// granular tool and thinking inclusion.
func FilterDisplayableWith(msgs []store.Msg, want int, includeTools, includeThinking bool) []store.Msg {
	out := make([]store.Msg, 0, want)
	for _, m := range msgs {
		if len(out) == want {
			break
		}
		if !includeTools && !IsDisplayableWith(m.Content, includeTools, includeThinking) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// RenderMsgs renders store rows for display at the given cap.
func RenderMsgs(msgs []store.Msg, includeTools bool, cap int) []ViewMsg {
	return RenderMsgsWith(msgs, includeTools, false, cap)
}

// RenderMsgsWith renders store rows for display with granular tool/thinking inclusion.
func RenderMsgsWith(msgs []store.Msg, includeTools, includeThinking bool, cap int) []ViewMsg {
	out := make([]ViewMsg, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, ViewMsg{ID: m.ID, Role: m.Role, Text: parse.DispWith(m.Content, includeTools, includeThinking, cap)})
	}
	return out
}

// TakeDisplayable renders the FIRST want displayable records of msgs.
func TakeDisplayable(msgs []store.Msg, want int, includeTools bool, cap int) []ViewMsg {
	return TakeDisplayableWith(msgs, want, includeTools, false, cap)
}

// TakeDisplayableWith renders the FIRST want displayable records with granular inclusions.
func TakeDisplayableWith(msgs []store.Msg, want int, includeTools, includeThinking bool, cap int) []ViewMsg {
	return RenderMsgsWith(FilterDisplayableWith(msgs, want, includeTools, includeThinking), includeTools, includeThinking, cap)
}

// TakeDisplayableTail renders the LAST want displayable records of msgs, still
// in chronological order. Used for a closing bookend, where the interesting
// records are the ones nearest the end.
func TakeDisplayableTail(msgs []store.Msg, want int, includeTools bool, cap int) []ViewMsg {
	return TakeDisplayableTailWith(msgs, want, includeTools, false, cap)
}

// TakeDisplayableTailWith renders the LAST want displayable records with granular inclusions.
func TakeDisplayableTailWith(msgs []store.Msg, want int, includeTools, includeThinking bool, cap int) []ViewMsg {
	return RenderMsgsWith(Reversed(FilterDisplayableWith(Reversed(msgs), want, includeTools, includeThinking)), includeTools, includeThinking, cap)
}

// Reversed returns msgs in the opposite order.
func Reversed(msgs []store.Msg) []store.Msg {
	out := slices.Clone(msgs)
	slices.Reverse(out)
	return out
}

// AnchoredViewOpts groups the optional tuning of AnchoredView (window radius,
// bookend size, tool inclusion) to keep the signature small.
// Defaults: Window=5, Bookend=3, IncludeTools=false.
type AnchoredViewOpts struct {
	Window          int
	Bookend         int
	IncludeTools    bool
	IncludeThinking bool
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
	slices.Reverse(before)
	win := append(before, after...)
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
		text := parse.DispWith(m.Content, opts.IncludeTools, opts.IncludeThinking, cap)
		if !isAnchor { // neighbours are context: conversation only
			if !opts.IncludeTools && !IsDisplayableWith(m.Content, opts.IncludeTools, opts.IncludeThinking) {
				continue
			}
			if text == "" {
				continue
			}
		} else if text == "" {
			// The caller named this record by ref. If everything in it was
			// generated, show it raw rather than an empty line — refusing to
			// render the row someone asked for by id would be the worse lie.
			text = parse.DispWith(m.Content, true, true, cap)
		}
		wmsgs = append(wmsgs, ViewMsg{ID: m.ID, Role: m.Role, Text: text, Anchor: isAnchor})
	}

	var bs, be []store.Msg
	if opts.Bookend > 0 {
		// bookend_start: the run-up before the window (id<winMin, ASC).
		bs, _ = store.BookendMessages(con, sessionID, winMin, true, true, bookendFetch(opts))
		// bookend_end: the tail after the window (id>winMax, DESC — reversed below).
		be, _ = store.BookendMessages(con, sessionID, winMax, true, false, bookendFetch(opts))
	}

	bookendStart := TakeDisplayableWith(bs, opts.Bookend, opts.IncludeTools, opts.IncludeThinking, dispCap)
	// bookend_end: emit reversed(be) (be is DESC, so output is ASC by id).
	bookendEnd := TakeDisplayableTailWith(Reversed(be), opts.Bookend, opts.IncludeTools, opts.IncludeThinking, dispCap)

	messagesBefore := max(0, len(before)-1)
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
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].ISO != cands[j].ISO {
				return cands[i].ISO > cands[j].ISO
			}
			if cands[i].Routine != cands[j].Routine {
				return !cands[i].Routine && cands[j].Routine
			}
			return false
		})
	case "oldest":
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].ISO != cands[j].ISO {
				return cands[i].ISO < cands[j].ISO
			}
			if cands[i].Routine != cands[j].Routine {
				return !cands[i].Routine && cands[j].Routine
			}
			return false
		})
	default:
		sort.SliceStable(cands, func(i, j int) bool {
			a, b := cands[i], cands[j]
			if a.Fused != b.Fused {
				return a.Fused > b.Fused
			}
			if a.Cov != b.Cov {
				return a.Cov > b.Cov
			}
			if a.Routine != b.Routine {
				return !a.Routine && b.Routine
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
	return BrowseDB(dbp, limit, since, before)
}

// BrowseAllRow is one recent-session row of the cross-project (--all) browse:
// a BrowseRow tagged with the project it came from.
type BrowseAllRow struct {
	Project string `json:"project"`
	BrowseRow
}

// BrowseDB is Browse over an already-resolved index db (a pre-ensured scope —
// Codex, retained/orphaned, or a Claude scope the caller resolved itself).
func BrowseDB(dbp string, limit int, since, before string) []BrowseRow {
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

	// Connection is now free — fill each row's two texts with their own queries.
	for i := range out {
		out[i].Preview = SessionPreview(con, out[i].SessionID, browsePreviewCap)
		out[i].Last = SessionLastActivity(con, out[i].SessionID)
	}
	return out
}

// BrowseScoped queries recent sessions from an open connection (draining rows
// first) and populates preview and last activity using the same connection.
func BrowseScoped(con *sql.DB, limit int, since, before, sourceTool string, projects []string) ([]BrowseAllRow, error) {
	sessions, err := store.BrowseScopedSessions(con, since, before, sourceTool, projects, limit)
	if err != nil {
		return nil, err
	}

	out := make([]BrowseAllRow, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, BrowseAllRow{
			Project: s.Project,
			BrowseRow: BrowseRow{
				SessionID: s.SessionID,
				LastTS:    s.LastTS,
				N:         s.MessageCount,
			},
		})
	}

	// Connection is now free — fill each row's two texts with their own queries.
	for i := range out {
		out[i].Preview = SessionPreview(con, out[i].SessionID, browsePreviewCap)
		out[i].Last = SessionLastActivity(con, out[i].SessionID)
	}
	return out, nil
}

// lastActivityScan is how many trailing messages SessionLastActivity inspects
// before giving up. A busy session's tail is mostly tool results and injected
// envelopes, so the window has to clear those to reach real conversation —
// but it stays bounded, matching OpenClaw's 20-line tail cap
// (LAST_MSG_MAX_LINES, src/gateway/session-utils.fs.ts). Wider than 20 here
// because our rows include tool results that its transcript lines don't.
//
// A fixed window is the shape that once shipped the current-turn exclusion
// inert, so this one was measured rather than assumed. Across ~550 sessions of
// >50 records in five live project stores (four Claude, one Codex), the newest
// displayable record sat at depth 1 for the median session, 4 at p99, and 12 at
// the worst case — no session in the sample came back empty at 40, and none had
// no displayable record at any depth. The reason the margin is this wide:
// unlike the current-turn scan, this one accepts ANY side of the conversation,
// and an assistant message almost always sits within a few rows of the tail
// even when the trailing user rows are all machinery.
const lastActivityScan = 40

// SessionLastActivity returns the session's most recent REAL activity: the
// newest message that still has content once tool runs and injected envelopes
// are stripped (parse.StripGenerated). Walking newest-first and skipping
// generated rows is OpenClaw's selectBoundedActiveTailRecords in miniature.
//
// Returns "" when the whole scanned tail is machinery — honest silence beats
// captioning a session with a tool result. The row still renders; it just has
// no "now" line, and Preview still says what the session was for.
//
// Exported because a browse row is not the only place the question "and then
// what?" gets asked: a search hit is a point in the middle of a conversation,
// so the same tail read answers where that conversation ended up
// (agentproto.attachLastActivity). One definition of "real activity" for both,
// or the two surfaces drift.
func SessionLastActivity(con *sql.DB, sessionID string) string {
	msgs, err := store.LastMessages(con, sessionID, lastActivityScan)
	if err != nil {
		return ""
	}
	for _, m := range msgs {
		if !IsDisplayable(m.Content) {
			continue
		}
		if text := parse.Disp(m.Content, false, browsePreviewCap); text != "" {
			return text
		}
	}
	return ""
}

// isBareBlockMarker reports whether a record's whole display text is one of
// parse's block labels with nothing after it — the record announces a block and
// then carries no content.
//
// This is overwhelmingly the redacted-thinking case. A model that withholds its
// reasoning still emits a thinking block, and the parser writes the label with
// an empty body, so the row stores literally "[THINKING] " and eleven bytes.
// Measured on the live corpus: 15,535 of 15,613 thinking records in the largest
// store sampled are bare, 11,339 of 11,357 in the next, and 2,676 of 2,684 in a
// third — about 99.5% everywhere. So this is the common shape, not an edge
// case, and a tail that ends on one used to caption its session "now →
// [THINKING]", which tells a reader nothing at all.
//
// Only the EMPTY ones are skipped. Thinking that actually carries text is often
// the truest statement of what a session is doing right now, and it still
// captions the row.
func isBareBlockMarker(text string) bool {
	t := strings.TrimSpace(text)
	switch t {
	case "[THINKING]", "[SYSTEM]", "[TOOL_RESULT]":
		return true
	}
	return strings.HasPrefix(t, "[TOOL:") && strings.HasSuffix(t, "]") &&
		!strings.Contains(t[1:len(t)-1], "]")
}

// IsInterruptionMarker reports whether a record is the runtime's "the operator
// stopped me" note rather than anything either party said. It survives
// StripGenerated (it carries no tool marker and no envelope tag) but captioning
// a session with it says nothing about what that session is doing.
//
// Measured across the live corpus by sampling the last three records of every
// session: 51 tails ended on this marker. [THINKING] (470) and [SYSTEM] (53) are
// deliberately NOT filtered here — reasoning that carries text is often the
// truest statement of what a session is working on right now, and a [SYSTEM]
// note is real injected content.
//
// Corrected 2026-08-05: that reasoning holds only for blocks that carry text,
// and on this corpus almost none do — about 99.5% of thinking records are the
// bare label with an empty body (redacted reasoning). Those are skipped by
// isBareBlockMarker, which filters on emptiness rather than on block type, so
// the rule above is unchanged for any block that actually says something.
//
// Exported alongside SessionLastActivity because every reader that walks a
// session tail looking for something a PERSON said has to step over it —
// including the one that finds where the caller's live turn began
// (agentproto.currentTurnStart).
func IsInterruptionMarker(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), "[Request interrupted by user")
}

// previewScan is how many early user messages sessionPreview considers before
// giving up — enough to skip a warmup ('hi') / '/clear' opener and reach the
// first substantive turn, without scanning a whole session.
const previewScan = 8

// SessionPreview returns the first SUBSTANTIVE user message's display text for a
// session — the ask that started it (low-signal openers like 'hi' or '/clear' are
// skipped via parse.IsSubstantive). The session is never dropped — if no early
// message is substantive, the first non-empty user message is shown as a
// fallback so the row still previews something.
//
// browse renders it as the row preview; search uses it as the header-line title
// when a session was never tagged, which is most of them. Both want the same
// thing — "what was this about, in the user's own words" — so both read it here.
// cap is the caller's line budget: a browse row affords more width than a search
// header line that already carries a stamp, an id and a project.
func SessionPreview(con *sql.DB, sessionID string, cap int) string {
	contents, err := store.FirstUserMessages(con, sessionID, previewScan)
	if err != nil {
		return ""
	}

	var fallback string
	for _, content := range contents {
		if fallback == "" {
			fallback = parse.Disp(content, false, cap)
		}
		if parse.IsSubstantive(content) {
			return parse.Disp(content, false, cap)
		}
	}
	return fallback
}
