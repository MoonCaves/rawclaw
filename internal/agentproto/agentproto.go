// Package agentproto is the search→narrow→bounded-read protocol for LLM agents:
// an agent recalls prior conversations WITHOUT pasting whole transcripts — it
// gets ranked conversation refs, then reads BOUNDED excerpts on demand.
//
// Three verbs: Search (ranked refs), Read (bounded excerpt around a ref),
// Outline (a session's goal→resolution arc).
package agentproto

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/adapters"
	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/query"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/semantic"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// Protocol constants.
const (
	DefaultSearchLimit = 8    // top-N conversations to surface (matches the human --limit default)
	DefaultReadBudget  = 4000 // chars; the ceiling a bare --budget uses (no longer a default cap — reads are whole by default, #3)
	OutlineBookend     = 4    // messages each end for the arc summary
	ReadWindow         = 8    // ±messages around the anchor for Read
)

// readBookend is the number of bookend messages included at each end of the
// read window.
const readBookend = 3

// outlineDispCap caps the per-message display length in outline output.
const outlineDispCap = 300

// truncateMarker is appended to the last in-budget message when it overflows
// (note the leading space).
const truncateMarker = " …[truncated]"

// SearchRef is one ranked conversation ref. The ReadRef token
// ("<session8>:<uuid8>") is what an agent passes to Read.
type SearchRef struct {
	Project   string `json:"project"`
	SessionID string `json:"session_id"`
	ISO       string `json:"iso"`
	Snippet   string `json:"snippet"`
	ReadRef   string `json:"read_ref"`
}

// Scope status values for incompleteness-as-data (#6).
const (
	ScopeSearched      = "searched"       // indexed fresh + searched
	ScopeEmpty         = "empty"          // searched fresh, zero rows
	ScopeSkippedError  = "skipped_error"  // index/open failed — not searched
	ScopeStaleFallback = "stale_fallback" // busy-lock → searched a possibly-stale cached index
)

// ScopeReport records how one project scope fared during a search, so an agent
// reads a partial result AS partial instead of mistaking it for complete (#6).
type ScopeReport struct {
	Project string `json:"project"`
	Dir     string `json:"dir"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
}

// SearchEnvelope wraps the ranked results with the per-scope completeness
// report. Complete is false if any scope was skipped, served from a stale
// fallback, or had matches the limit/fetch window hid.
//
// The truncation block is the never-silent counterpart of ReadResult's trim
// fields: an agent must never receive N results without learning the set is
// larger. Count is len(Results). TotalMatches is the DISTINCT candidates found
// within the fetch window; TotalIsLowerBound is true when a scope hit the fetch
// ceiling, so the true total is >= TotalMatches (it's a floor, never claimed as
// exact). HasMore is set whenever anything was hidden, and NextCommand is the
// literal command an agent re-issues to widen.
type SearchEnvelope struct {
	Results  []SearchRef   `json:"results"`
	Scopes   []ScopeReport `json:"scopes_report"`
	Complete bool          `json:"complete"`

	Count             int    `json:"count"`
	TotalMatches      int    `json:"total_matches"`
	TotalIsLowerBound bool   `json:"total_is_lower_bound,omitempty"`
	HasMore           bool   `json:"has_more"`
	NextCommand       string `json:"next_command,omitempty"`

	// RecencyHint fires in the default relevance order when the freshest match is
	// well newer than the top-ranked one — so a buried "what just happened" result
	// announces itself instead of staying hidden behind relevance.
	RecencyHint string `json:"recency_hint,omitempty"`
}

// ReadResult is a bounded excerpt around a ref. Embeds the AnchoredView shape
// plus protocol metadata.
type ReadResult struct {
	Project      string `json:"project"`
	SessionID    string `json:"session_id"`
	AnchorID     int    `json:"anchor_id"`
	FocusSnippet string `json:"focus_snippet"`
	CharBudget   *int   `json:"char_budget"` // nil = no cap
	Truncated    bool   `json:"truncated"`
	// Never-silent trim (#5): when Truncated, these carry the machine counts AND
	// the literal command an agent re-issues to recover the hidden content. Empty
	// when nothing was trimmed.
	TrimmedChars int    `json:"trimmed_chars,omitempty"`
	TrimmedMsgs  int    `json:"trimmed_msgs,omitempty"`
	NextCommand  string `json:"next_command,omitempty"`
	*view.AnchoredView
}

// trimStat is what applyBudget reports back: whether it truncated and how much
// it dropped, so Read can build the never-silent recovery note.
type trimStat struct {
	Truncated    bool
	OmittedChars int // chars dropped across the window (incl. the cut tail)
	OmittedMsgs  int // whole messages dropped after the budget was exhausted
}

// OutlineResult is a session's bookend arc.
type OutlineResult struct {
	Project      string         `json:"project"`
	SessionID    string         `json:"session_id"`
	ISO          string         `json:"iso"`
	MessageCount int            `json:"message_count"`
	Start        []view.ViewMsg `json:"start"`
	End          []view.ViewMsg `json:"end"`
	MidCount     int            `json:"mid_count"`
}

// SearchOpts groups the optional search filters (keeps the signature small).
// The scope filters (Role/Since/Before/MinMessages/IncludePath/ExcludePath)
// mirror the default-discovery flags so `agent search` honors the SAME scoping
// the human path does, instead of leaking their values into the FTS5 query.
type SearchOpts struct {
	Limit            int
	Role             string
	Sort             string
	IncludeTools     bool
	IncludeSubagents bool
	Since            string // "" = no bound; else YYYY-MM-DD inclusive
	Before           string // "" = no bound; else YYYY-MM-DD inclusive
	MinMessages      int    // 0 = no minimum
	IncludePath      string // "" = no filter; else a regex over the project working dir
	ExcludePath      string // "" = no filter; else a regex over the project working dir
}

// ── helpers ──────────────────────────────────────────────────────────────────

// fmtRef builds the copyable read-ref token an agent pastes into Read,
// formatted as "<session8>:<uuid8>" — both halves source-stable (the session
// filename stem + the message's own uuid), so the ref survives reindex/append.
func fmtRef(sessionID, uuid string) string {
	return sid8(sessionID) + ":" + uuid8(uuid)
}

// sid8 truncates a session id to its first 8 runes (code points) without
// padding.
func sid8(sessionID string) string {
	r := []rune(sessionID)
	if len(r) > 8 {
		return string(r[:8])
	}
	return string(r)
}

// uuid8 returns the first 8 hex chars of a message uuid (the short, copyable
// half of a read-ref). A uuid like "9f3e1c20-aaaa-..." yields "9f3e1c20". An
// empty uuid yields "" (such a record is not anchorable — see MsgUUID).
func uuid8(uuid string) string {
	r := []rune(uuid)
	if len(r) > 8 {
		return string(r[:8])
	}
	return string(r)
}

// reNumericRef matches a ref whose second half is purely numeric — an old
// rowid-based ref from before the uuid migration.
var reNumericRef = regexp.MustCompile(`^[0-9]+$`)

// reHexPrefix matches a valid uuid8 prefix: 1+ lowercase hex chars.
var reHexPrefix = regexp.MustCompile(`^[0-9a-f]+$`)

// resolveRef parses "<session8>:<uuid8>" → (session8, uuid8). The second half is
// now an opaque hex prefix (the message uuid), not an integer rowid. A purely
// numeric second half is a pre-migration ref and returns a migration hint.
func resolveRef(ref string) (string, string, error) {
	parts := strings.Split(ref, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("bad ref %q — expected <session8>:<uuid8> (e.g. a1b2c3d4:9f3e1c20)", ref)
	}
	session8, uuidPrefix := parts[0], strings.ToLower(parts[1])
	if uuidPrefix == "" {
		return "", "", fmt.Errorf("bad ref %q — expected <session8>:<uuid8> (e.g. a1b2c3d4:9f3e1c20)", ref)
	}
	if reNumericRef.MatchString(uuidPrefix) {
		return "", "", fmt.Errorf("ref %q looks like an old numeric ref; re-run search to get a uuid ref", ref)
	}
	if !reHexPrefix.MatchString(uuidPrefix) {
		return "", "", fmt.Errorf("bad ref %q — uuid8 must be hex [0-9a-f] (e.g. a1b2c3d4:9f3e1c20)", ref)
	}
	return session8, uuidPrefix, nil
}

// allScope returns (label, tdir) pairs for every project.
func allScope() []view.Scope {
	dirs := paths.AllProjectDirs()
	out := make([]view.Scope, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, view.Scope{Project: paths.ProjectLabel(d), TDir: d})
	}
	return out
}

// thisScope returns the single (label, tdir) pair for cwd's project, or nil if
// the directory has no transcript history.
func thisScope() []view.Scope {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	td := paths.FindTranscriptDir(cwd)
	if td == "" {
		return nil
	}
	if info, statErr := os.Stat(td); statErr != nil || !info.IsDir() {
		return nil
	}
	return []view.Scope{{Project: paths.ProjectLabel(td), TDir: td}}
}

// runeLen counts code points in s.
func runeLen(s string) int {
	return len([]rune(s))
}

// runeSlice returns the first n runes (code points) of s, or all of s if it is
// shorter.
func runeSlice(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if n >= len(r) {
		return s
	}
	return string(r[:n])
}

// ── verb: search ─────────────────────────────────────────────────────────────

// Search returns rank-ordered conversation refs matching query within scope
// (nil scope = all projects), wrapped in an envelope that reports per-scope
// completeness (#6) so a partial result is never mistaken for complete. With a
// non-nil embedder in relevance mode (no --sort), keyword anchors are RRF-fused
// with vector-KNN anchors — parity with the default discovery path.
func Search(rawQuery string, scope []view.Scope, opts SearchOpts, embedder embed.Embedder) SearchEnvelope {
	limit := opts.Limit
	if limit == 0 {
		limit = DefaultSearchLimit
	}
	if scope == nil {
		scope = allScope()
	}
	if len(scope) == 0 {
		return SearchEnvelope{Results: []SearchRef{}, Scopes: []ScopeReport{}, Complete: true}
	}

	fetch := limit * 8
	if fetch < 30 {
		fetch = 30
	}

	// Apply the same scope filtering the human path does: a path predicate drops
	// whole projects whose working dir doesn't match include / does match exclude,
	// BEFORE indexing them. Role / date bounds / min-messages push into the SQL
	// WHERE via SearchParams. None of these flag VALUES reach the FTS5 query (#1).
	if opts.IncludePath != "" || opts.ExcludePath != "" {
		scope = filterScopeByPath(scope, opts.IncludePath, opts.ExcludePath)
		if len(scope) == 0 {
			return SearchEnvelope{Results: []SearchRef{}, Scopes: []ScopeReport{}, Complete: true}
		}
	}

	// Translate human boolean operators (NOT/&&/||/!) into an explicit FTS5 expr,
	// exactly as the default discovery path does — without this the agent path
	// drops "not" as a stopword and a documented `deploy NOT staging` exclusion
	// silently no-ops (#4). A query with no operators leaves RawMatch empty and
	// takes the plain OR/coverage path, byte-identical to before.
	rawMatch := ""
	if ftsExpr, usedOps := query.BooleanToFTS5(rawQuery); usedOps {
		rawMatch = ftsExpr
	}

	p := retrieve.SearchParams{
		Role:             opts.Role,
		Sort:             opts.Sort,
		IncludeTools:     opts.IncludeTools,
		IncludeSubagents: opts.IncludeSubagents,
		Since:            opts.Since,
		Before:           opts.Before,
		MinMessages:      opts.MinMessages,
		RawMatch:         rawMatch,
	}

	// Embed the query once for RRF fusion, only in relevance mode — an explicit
	// --sort stays pure (keyword/recency), matching the discovery path.
	var qvec []float64
	if embedder != nil && opts.Sort == "" {
		qvec = embedder.Embed(rawQuery)
	}

	cands, reports, hitCeiling := collectCandidates(scope, rawQuery, fetch, p, qvec)

	sortCandidates(cands, opts.Sort)

	// Build every DISTINCT result first, then cap to `limit`. Capping after the
	// full dedup lets us report Complete=false when the limit hid real candidates
	// (#2), so an agent that sees N of many knows the set is incomplete.
	seen := map[string]struct{}{}
	all := []SearchRef{}
	for _, r := range cands {
		// A uuid-less anchor (e.g. a summary record) is searchable but not a
		// citeable read anchor — skip it rather than emit an unresolvable ref.
		if r.UUID == "" {
			continue
		}
		key := r.Project + "\x00" + r.Root
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		all = append(all, SearchRef{
			Project:   r.Project,
			SessionID: r.SessionID,
			ISO:       r.ISO,
			Snippet:   r.Snip,
			ReadRef:   fmtRef(r.SessionID, r.UUID),
		})
	}

	results := all
	truncated := false
	if limit >= 0 && len(all) > limit {
		results = all[:limit]
		truncated = true
	}

	// Never-silent truncation: an agent that sees N must learn the set is larger.
	// total is the distinct count within the fetch window; if a scope hit the fetch
	// ceiling the true total is higher, so we report it as a lower bound rather than
	// claim a precision we don't have. hasMore is set whenever anything was hidden —
	// by the limit OR by the fetch window — and carries the literal widen command.
	total := len(all)
	hasMore := truncated || hitCeiling
	nextCmd := ""
	if hasMore {
		wider := limit * 4
		if wider < 20 {
			wider = 20
		}
		nextCmd = fmt.Sprintf("rawclaw agent search %q --limit %d", rawQuery, wider)
	}

	// Recency hint: in the default relevance order, if the freshest match is well
	// newer than the top-ranked hit, recency is buried — surface it and offer the
	// one flag that reorders, rather than silently ranking by relevance alone.
	recencyHint := ""
	if opts.Sort == "" && len(results) > 0 {
		newest := ""
		for _, r := range all {
			if r.ISO > newest {
				newest = r.ISO
			}
		}
		if tn, err := time.Parse(time.RFC3339, newest); err == nil {
			if tt, err2 := time.Parse(time.RFC3339, results[0].ISO); err2 == nil && tn.Sub(tt) > 24*time.Hour {
				recencyHint = fmt.Sprintf("relevance-ranked; newest match is %s — add --sort newest for latest-first", newest[:10])
			}
		}
	}
	return SearchEnvelope{
		Results:           results,
		Scopes:            reports,
		Complete:          scopesComplete(reports) && !hasMore,
		Count:             len(results),
		TotalMatches:      total,
		TotalIsLowerBound: hitCeiling,
		HasMore:           hasMore,
		NextCommand:       nextCmd,
		RecencyHint:       recencyHint,
	}
}

// SearchAndRender runs Search and writes the result to w: the agent envelope as
// text, or as JSON when wantJSON. This is the exported entry the default CLI path
// calls so a bare `rawclaw "query"` IS the agent search (no `agent` subcommand to
// discover). scopeLabel is the human-facing "across all projects" / "on <project>"
// suffix in the text header.
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
	renderSearch(w, env, query, scopeLabel)
	return nil
}

// ReadAndRender resolves ref within scope and writes the bounded excerpt to w
// (JSON when wantJSON). The exported entry the top-level `read` subcommand calls,
// so reading is a top-level verb (`rawclaw read <ref>`) rather than `agent read`.
// moreLevel 0 = the default window; >0 widens it via the expand-in-place ladder.
func ReadAndRender(
	w io.Writer,
	ref string,
	scope []view.Scope,
	focus string,
	budget *int,
	includeTools bool,
	moreLevel, around int,
	wantJSON bool,
) error {
	window := 0
	if moreLevel > 0 {
		window = moreWindow(moreLevel)
	}
	result, err := Read(ref, scope, ReadOpts{
		Focus:        focus,
		Budget:       budget,
		IncludeTools: includeTools,
		Window:       window,
		Around:       around,
	})
	if err != nil {
		return err
	}
	if wantJSON {
		return emit(w, result)
	}
	renderRead(w, result)
	return nil
}

// OutlineAndRender resolves session8 within scope and writes its goal→resolution
// arc to w (JSON when wantJSON). The exported entry the top-level `outline`
// subcommand calls.
func OutlineAndRender(w io.Writer, session8 string, scope []view.Scope, includeTools, wantJSON bool) error {
	result, err := Outline(session8, scope, includeTools)
	if err != nil {
		return err
	}
	if wantJSON {
		return emit(w, result)
	}
	renderOutline(w, result)
	return nil
}

// filterScopeByPath keeps only the scopes whose project working dir satisfies the
// include/exclude path predicate — the same predicate the default discovery path
// applies, evaluated against paths.ProjectCWD(scope.TDir).
func filterScopeByPath(scope []view.Scope, include, exclude string) []view.Scope {
	pred := query.PathPredicate(include, exclude)
	out := make([]view.Scope, 0, len(scope))
	for _, sc := range scope {
		if pred(paths.ProjectCWD(sc.TDir)) {
			out = append(out, sc)
		}
	}
	return out
}

// scopesComplete reports whether every scope was searched fresh (none skipped or
// served from a stale fallback).
func scopesComplete(reports []ScopeReport) bool {
	for _, r := range reports {
		if r.Status == ScopeSkippedError || r.Status == ScopeStaleFallback {
			return false
		}
	}
	return true
}

// collectCandidates indexes each scope dir, runs MatchAnchors, and attaches the
// lineage root + project + rank to each anchor. Instead of silently dropping a
// failed/locked project, it records a per-scope ScopeReport (#6): an
// index/open failure → skipped_error; a busy-lock fallback → stale_fallback;
// success with rows → searched; success with zero rows → empty.
func collectCandidates(
	scope []view.Scope,
	query string,
	fetch int,
	p retrieve.SearchParams,
	qvec []float64,
) ([]retrieve.Anchor, []ScopeReport, bool) {
	cands := []retrieve.Anchor{}
	reports := make([]ScopeReport, 0, len(scope))
	hitCeiling := false
	for _, sc := range scope {
		rep := ScopeReport{Project: sc.Project, Dir: sc.TDir}
		dbp, _, status, err := index.EnsureIndexed(sc.TDir, false)
		if err != nil {
			rep.Status = ScopeSkippedError
			rep.Detail = err.Error()
			reports = append(reports, rep)
			continue
		}
		con, openErr := index.ConnectRO(dbp)
		if openErr != nil {
			rep.Status = ScopeSkippedError
			rep.Detail = openErr.Error()
			reports = append(reports, rep)
			continue
		}
		rows := retrieve.MatchAnchors(con, query, fetch, p)
		if len(rows) >= fetch {
			// This scope filled the fetch window — there may be more matches we
			// never pulled, so any total derived from these rows is a floor. The
			// ceiling is measured on the keyword fetch (pre-fusion), since that is
			// what saturated.
			hitCeiling = true
		}
		// RRF-fuse keyword anchors with vector-KNN when a query vector is present
		// (relevance mode only — qvec is nil under --sort). Parity with Discovery.
		if qvec != nil {
			rows = semantic.Fuse(con, rows, qvec, fetch, p.IncludeSubagents)
		}
		for i := range rows {
			rows[i].Root = retrieve.LineageRoot(con, rows[i].SessionID)
			rows[i].Project = sc.Project
			rows[i].DBP = dbp
			rows[i].Rank = i
			cands = append(cands, rows[i])
		}
		_ = con.Close()

		switch {
		case status == index.IndexStale:
			rep.Status = ScopeStaleFallback
			rep.Detail = "index busy, used cached"
		case len(rows) == 0:
			rep.Status = ScopeEmpty
		default:
			rep.Status = ScopeSearched
		}
		reports = append(reports, rep)
	}
	return cands, reports, hitCeiling
}

// sortCandidates orders the merged candidates: newest/oldest sort by ISO;
// relevance sorts by (-fused, -cov, rank). fused is always zero here (agentproto
// never fuses) — kept for parity with the shared anchor ordering.
func sortCandidates(cands []retrieve.Anchor, mode string) {
	switch mode {
	case "newest":
		sort.SliceStable(cands, func(i, j int) bool {
			return cands[i].ISO > cands[j].ISO
		})
	case "oldest":
		sort.SliceStable(cands, func(i, j int) bool {
			return cands[i].ISO < cands[j].ISO
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
			return a.Rank < b.Rank
		})
	}
}

// ── verb: read ───────────────────────────────────────────────────────────────

// ReadOpts groups the read-verb options (keeps Read's signature small, like
// view.AnchoredViewOpts). Window/Around express the expand-in-place ladder (#4):
//   - Window == 0           → the default ±ReadWindow context (rung 2).
//   - Window  > 0           → an explicit window radius (rung 3 --more / rung 4).
//   - Around  > 0           → re-center the window `Around` messages after the
//     anchor (scroll within the session on the SAME stable ref).
type ReadOpts struct {
	Focus        string
	Budget       *int // nil = no cap (the default since #3)
	IncludeTools bool
	Window       int
	Around       int
}

// moreWindow maps a --more level (0 = none) to a window radius. Level 0 keeps the
// default ±ReadWindow; each level widens by another ReadWindow (level 1 = 2×,
// level 2 = 3×, …) so --more strictly expands the SAME ref's window — never a
// re-search.
func moreWindow(level int) int {
	if level <= 0 {
		return ReadWindow
	}
	return (level + 1) * ReadWindow
}

// Read returns a bounded excerpt around the message identified by ref
// ("<session8>:<uuid8>"). opts.Budget of nil = no cap. Returns an error on a bad
// ref or a session/message not found. Expansion (--more/--around) is a cheap
// follow-up on the SAME resolved ref — it never re-runs search (#4).
func Read(ref string, scope []view.Scope, opts ReadOpts) (*ReadResult, error) {
	session8, uuid8, err := resolveRef(ref)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		scope = allScope()
	}

	dbp, fullSID, proj, locErr := locateSession(scope, session8)
	if locErr != nil {
		return nil, locErr
	}

	con, err := index.ConnectRO(dbp)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", dbp, err)
	}
	defer con.Close()

	// Translate the stable uuid8 prefix → the internal rowid the view layer
	// windows on. The view layer keeps working on integer ids; uuid is purely
	// the external, reindex-stable handle.
	msgID, err := resolveUUID(con, fullSID, uuid8)
	if err != nil {
		return nil, err
	}

	// Resolve the window radius and re-centered anchor from the ladder opts.
	window := opts.Window
	if window <= 0 {
		window = ReadWindow
	}
	center := msgID + opts.Around // --around shifts the window center on the same ref

	av := view.BuildAnchoredView(con, fullSID, center, view.AnchoredViewOpts{
		Window:       window,
		Bookend:      readBookend,
		IncludeTools: opts.IncludeTools,
	})
	if av == nil {
		return nil, fmt.Errorf("message %q not found in session %q", uuid8, session8)
	}

	st := applyBudget(av, opts.Budget)
	focusSnippet := focusHighlight(av.Window, opts.Focus)

	// Build the never-silent recovery command on the SAME stable ref (#5): an
	// agent re-issues it verbatim to widen the window and recover the hidden
	// content.
	nextCmd := ""
	if st.Truncated {
		nextCmd = "rawclaw agent read " + fmtRef(fullSID, uuid8) + " --more"
	}

	return &ReadResult{
		Project:      proj,
		SessionID:    fullSID,
		AnchorID:     msgID,
		FocusSnippet: focusSnippet,
		CharBudget:   opts.Budget,
		Truncated:    st.Truncated,
		TrimmedChars: st.OmittedChars,
		TrimmedMsgs:  st.OmittedMsgs,
		NextCommand:  nextCmd,
		AnchoredView: av,
	}, nil
}

// ErrMsgNotFound is returned when no message in the located session carries the
// uuid8 prefix.
type ErrMsgNotFound struct{ UUID8 string }

func (e *ErrMsgNotFound) Error() string {
	return fmt.Sprintf("message %q not found in session", e.UUID8)
}

// ErrAmbiguousUUID is returned when a uuid8 prefix matches more than one message
// in the session (a 32-bit prefix collision) — resolving none, git-style.
type ErrAmbiguousUUID struct{ UUID8 string }

func (e *ErrAmbiguousUUID) Error() string {
	return fmt.Sprintf("ambiguous message ref %q — matches multiple messages; give a longer uuid prefix", e.UUID8)
}

// resolveUUID maps a uuid8 prefix to the internal rowid within one session.
// 0 matches → ErrMsgNotFound; ≥2 → ErrAmbiguousUUID (never silently pick one).
// A real query/scan error is distinguished from "not found" per the DB rubric.
func resolveUUID(con *sql.DB, fullSID, uuid8 string) (int, error) {
	rows, err := con.QueryContext(
		context.Background(),
		"SELECT id FROM messages WHERE session_id=? AND uuid LIKE ? ORDER BY id LIMIT 2",
		fullSID, uuid8+"%",
	)
	if err != nil {
		return 0, fmt.Errorf("resolve message %q: %w", uuid8, err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if scanErr := rows.Scan(&id); scanErr != nil {
			return 0, fmt.Errorf("resolve message %q: %w", uuid8, scanErr)
		}
		ids = append(ids, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return 0, fmt.Errorf("resolve message %q: %w", uuid8, rowsErr)
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

// applyBudget caps the total characters across av.Window in place, truncating
// the last message that overflows and dropping the rest. Returns a trimStat with
// the omitted char/msg counts so the caller can emit a never-silent recovery
// note (#5). budget nil = no cap.
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
			// Budget exhausted: this and every remaining message is dropped whole.
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
			// Dropped chars on this cut message = original minus the kept body
			// (the marker is added text, not original content).
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

// focusHighlight finds the first window message containing focus (case-folded)
// and returns a "[#id role] …>>>match<<<…" snippet around it, or "" if focus is
// empty or unmatched.
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
		// Convert the byte offset to a rune index, then slice [s : idx+120] in
		// rune (code-point) space so the snippet bounds align to runes.
		runeIdx := runeLen(m.Text[:idx])
		s := runeIdx - 60
		if s < 0 {
			s = 0
		}
		chunk := runeRange(m.Text, s, runeIdx+120)
		return fmt.Sprintf("[#%d %s] %s", m.ID, m.Role, highlight.ReplaceAllString(chunk, ">>>$1<<<"))
	}
	return ""
}

// runeRange returns s[lo:hi] in rune (code-point) space, clamping to bounds.
func runeRange(s string, lo, hi int) string {
	r := []rune(s)
	if lo < 0 {
		lo = 0
	}
	if hi > len(r) {
		hi = len(r)
	}
	if lo >= hi {
		return ""
	}
	return string(r[lo:hi])
}

// sessionCand is one session that matched a prefix, with its project (for the
// git-style ambiguity list).
type sessionCand struct {
	SessionID string
	Project   string
	dbp       string
}

// ErrAmbiguousSession is returned when a session8 prefix matches more than one
// session across scope. It mirrors the resume path (cli.runResume): list the
// candidates, resolve none.
type ErrAmbiguousSession struct {
	Prefix     string
	Candidates []sessionCand
}

func (e *ErrAmbiguousSession) Error() string {
	ids := make([]string, 0, len(e.Candidates))
	for _, c := range e.Candidates {
		ids = append(ids, fmt.Sprintf("%s (%s)", sid8(c.SessionID), c.Project))
	}
	return fmt.Sprintf("ambiguous session prefix %q — %d matches: %s; give a longer prefix",
		e.Prefix, len(e.Candidates), strings.Join(ids, ", "))
}

// ErrSessionNotFound is returned when a session8 prefix matches nothing in scope.
type ErrSessionNotFound struct{ Prefix string }

func (e *ErrSessionNotFound) Error() string {
	return fmt.Sprintf("session %q not found in scope", e.Prefix)
}

// locateSession walks scope, indexing each project and probing the sessions
// table for session ids with the session8 prefix. It aggregates matches ACROSS
// all scope (not just within one project), so a prefix that collides across
// projects is caught too. Returns the db path, full session id, and project
// label on a unique match; an *ErrAmbiguousSession on ≥2 matches; an
// *ErrSessionNotFound on 0. A failing project is skipped. Shared by Read and
// Outline.
func locateSession(scope []view.Scope, session8 string) (dbp, fullSID, proj string, err error) {
	// collect resolves the prefix against every scope. When excludeSub is set we
	// drop agent sub-sessions (id "<parent>/agent-...", is_subagent=1): a session
	// and its sub-agents share the UUID prefix — and even the full UUID, since a
	// subagent id is the parent UUID plus "/agent-..." — so without this filter a
	// bare OR full session ref false-trips the ambiguity guard against the
	// session's own subagent transcripts, breaking agent read/outline for any
	// session that spawned a subagent. Fall back to including sub-sessions only
	// when nothing top-level matched, so a full "<parent>/agent-..." ref still resolves.
	collect := func(excludeSub bool) []sessionCand {
		q := "SELECT id FROM sessions WHERE id LIKE ? ORDER BY id LIMIT 2"
		if excludeSub {
			q = "SELECT id FROM sessions WHERE id LIKE ? AND is_subagent = 0 ORDER BY id LIMIT 2"
		}
		var cs []sessionCand
		for _, sc := range scope {
			dbpC, _, _, ensureErr := index.EnsureIndexed(sc.TDir, false)
			if ensureErr != nil {
				continue
			}
			con, openErr := index.ConnectRO(dbpC)
			if openErr != nil {
				continue
			}
			// Fetch up to 2 per project: enough to detect an in-project collision;
			// cross-project collisions surface in the aggregate below.
			rows, qErr := con.QueryContext(context.Background(), q, session8+"%")
			if qErr != nil {
				_ = con.Close()
				continue
			}
			for rows.Next() {
				var sid string
				if scanErr := rows.Scan(&sid); scanErr != nil {
					break
				}
				cs = append(cs, sessionCand{SessionID: sid, Project: sc.Project, dbp: dbpC})
			}
			_ = rows.Close()
			_ = con.Close()
		}
		return cs
	}

	cands := collect(true)
	if len(cands) == 0 {
		cands = collect(false)
	}

	switch len(cands) {
	case 0:
		return "", "", "", &ErrSessionNotFound{Prefix: session8}
	case 1:
		c := cands[0]
		return c.dbp, c.SessionID, c.Project, nil
	default:
		return "", "", "", &ErrAmbiguousSession{Prefix: session8, Candidates: cands}
	}
}

// ── verb: outline ────────────────────────────────────────────────────────────

// Outline returns a session's bookend arc (first/last N user+assistant messages).
// Returns an error if the session is not found.
func Outline(session8 string, scope []view.Scope, includeTools bool) (*OutlineResult, error) {
	if scope == nil {
		scope = allScope()
	}

	dbp, fullSID, proj, locErr := locateSession(scope, session8)
	if locErr != nil {
		return nil, locErr
	}

	con, err := index.ConnectRO(dbp)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", dbp, err)
	}
	defer con.Close()

	iso, nmsg := sessionMeta(con, fullSID)

	startRows, err := bookendRows(con, fullSID, "ASC")
	if err != nil {
		return nil, fmt.Errorf("outline start rows: %w", err)
	}
	endRows, err := bookendRows(con, fullSID, "DESC")
	if err != nil {
		return nil, fmt.Errorf("outline end rows: %w", err)
	}

	startIDs := map[int]struct{}{}
	for _, r := range startRows {
		startIDs[r.ID] = struct{}{}
	}

	// endRows came back DESC; reverse to chronological, then drop any already
	// present in the start bookend so the two ends don't overlap.
	endMsgs := []rawRow{}
	for i := len(endRows) - 1; i >= 0; i-- {
		if _, dup := startIDs[endRows[i].ID]; dup {
			continue
		}
		endMsgs = append(endMsgs, endRows[i])
	}

	startOut := make([]view.ViewMsg, 0, len(startRows))
	for _, r := range startRows {
		startOut = append(startOut, view.ViewMsg{
			ID:   r.ID,
			Role: r.Role,
			Text: parse.Disp(r.Content, includeTools, outlineDispCap),
		})
	}
	endOut := make([]view.ViewMsg, 0, len(endMsgs))
	for _, r := range endMsgs {
		endOut = append(endOut, view.ViewMsg{
			ID:   r.ID,
			Role: r.Role,
			Text: parse.Disp(r.Content, includeTools, outlineDispCap),
		})
	}

	lastStartID := 0
	if len(startRows) > 0 {
		lastStartID = startRows[len(startRows)-1].ID
	}
	firstEndID := nmsg + 1
	if len(endMsgs) > 0 {
		firstEndID = endMsgs[0].ID
	}
	midCount := firstEndID - lastStartID - 1
	if midCount < 0 {
		midCount = 0
	}

	return &OutlineResult{
		Project:      proj,
		SessionID:    fullSID,
		ISO:          iso,
		MessageCount: nmsg,
		Start:        startOut,
		End:          endOut,
		MidCount:     midCount,
	}, nil
}

// rawRow is one (id, role, content) bookend row.
type rawRow struct {
	ID      int
	Role    string
	Content string
}

// sessionMeta reads last_ts + message_count and formats the local-time ISO.
// A missing row → ("", 0); a missing/zero last_ts → "" iso. Uses local time at
// seconds precision.
func sessionMeta(con *sql.DB, fullSID string) (iso string, nmsg int) {
	var lastTS sql.NullFloat64
	var mc sql.NullInt64
	row := con.QueryRowContext(
		context.Background(),
		"SELECT last_ts, message_count FROM sessions WHERE id=?",
		fullSID,
	)
	if err := row.Scan(&lastTS, &mc); err != nil {
		return "", 0
	}
	nmsg = int(mc.Int64)
	if lastTS.Valid && lastTS.Float64 != 0 {
		iso = time.Unix(int64(lastTS.Float64), 0).Local().Format("2006-01-02T15:04:05")
	}
	return iso, nmsg
}

// bookendRows reads up to OutlineBookend user/assistant messages with non-empty
// content, ordered by id in the given direction.
func bookendRows(con *sql.DB, fullSID, dir string) ([]rawRow, error) {
	q := "SELECT id, role, content FROM messages " +
		"WHERE session_id=? AND role IN ('user','assistant') AND length(content)>0 " +
		"ORDER BY id " + dir + " LIMIT ?"
	rows, err := con.QueryContext(context.Background(), q, fullSID, OutlineBookend)
	if err != nil {
		return nil, fmt.Errorf("query bookend rows: %w", err)
	}
	defer rows.Close()

	out := []rawRow{}
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.ID, &r.Role, &r.Content); err != nil {
			return nil, fmt.Errorf("scan bookend row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bookend rows: %w", err)
	}
	return out, nil
}

// ── text renderers ───────────────────────────────────────────────────────────

// renderSearch prints the human-readable search output. When the envelope is
// incomplete it appends a one-line footer naming how many scopes were skipped or
// stale, so the agent reads a partial result AS partial (#6).
func renderSearch(w io.Writer, env SearchEnvelope, query, scopeLabel string) {
	if len(env.Results) == 0 {
		fmt.Fprintln(w, "No matches. Lead with a single distinctive term that appears in the text (a filename, flag, or error string), not a topic word — or rephrase.")
		renderScopeFooter(w, env)
		return
	}
	fmt.Fprintf(w, "%d conversation(s) matching '%s' %s:\n\n", len(env.Results), query, scopeLabel)
	for _, r := range env.Results {
		iso := r.ISO
		if iso == "" {
			iso = "?"
		}
		fmt.Fprintf(w, "  ━━ %s · %s · %s\n", iso, sid8(r.SessionID), r.Project)
		fmt.Fprintf(w, "     …%s…\n", r.Snippet)
		fmt.Fprintf(w, "     read ref=%s\n\n", r.ReadRef)
	}
	if env.HasMore {
		total := strconv.Itoa(env.TotalMatches)
		if env.TotalIsLowerBound {
			total = "≥" + total
		}
		fmt.Fprintf(w, "showing %d of %s matches — see more: %s\n", env.Count, total, env.NextCommand)
	}
	if env.RecencyHint != "" {
		fmt.Fprintf(w, "note: %s\n", env.RecencyHint)
	}
	renderScopeFooter(w, env)
}

// renderScopeFooter prints the incompleteness footer when any scope was skipped
// or served from a stale fallback.
func renderScopeFooter(w io.Writer, env SearchEnvelope) {
	if env.Complete {
		return
	}
	errored, stale := 0, 0
	for _, s := range env.Scopes {
		switch s.Status {
		case ScopeSkippedError:
			errored++
		case ScopeStaleFallback:
			stale++
		}
	}
	skipped := errored + stale
	fmt.Fprintf(w, "note: %d of %d projects incomplete (%d error, %d stale) — results may be incomplete\n",
		skipped, len(env.Scopes), errored, stale)
}

// fmtChars renders a char count compactly: 1800 → "1.8k", 950 → "950".
func fmtChars(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return strconv.Itoa(n)
}

// trimNote builds the never-silent human trim note from a ReadResult's trim
// metadata: "[+1.8k chars · 2 msgs hidden — <next command>]". The next command
// is the literal recovery string, so the signal is never a bare "…[truncated]".
func trimNote(r *ReadResult) string {
	parts := "+" + fmtChars(r.TrimmedChars) + " chars"
	if r.TrimmedMsgs > 0 {
		parts += fmt.Sprintf(" · %d msgs hidden", r.TrimmedMsgs)
	}
	next := r.NextCommand
	if next == "" {
		next = "rawclaw agent read " + sid8(r.SessionID) + ":" + " --more"
	}
	return "  [" + parts + " — " + next + "]"
}

// renderRead prints the human-readable read output.
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
		// When truncated, the never-silent recovery note replaces the generic
		// scroll hint (the note already names the literal next command).
		fmt.Fprintln(w, truncNote)
		return
	}
	fmt.Fprintf(w, "\n  scroll more:  rawclaw --scroll %s --around %d\n", sid8(r.SessionID), r.AnchorID)
}

// renderOutline prints the human-readable outline output.
func renderOutline(w io.Writer, r *OutlineResult) {
	iso := r.ISO
	if iso == "" {
		iso = "?"
	}
	fmt.Fprintf(w, "━━ %s · %s · %s · %d messages ━━\n\n",
		iso, sid8(r.SessionID), r.Project, r.MessageCount)
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
}

// ── CLI entry ────────────────────────────────────────────────────────────────

// Run dispatches `search|read|outline` from args (the tokens AFTER `agent`),
// writing human output to out / errors to errw. Returns the process exit code.
// Uses a hand-rolled flag parse to stay dependency-free.
func Run(args []string, out, errw io.Writer) int {
	a := append([]string(nil), args...) // local copy; popFlag mutates it

	wantJSON := popBool(&a, "--json")
	thisProject := popBool(&a, "--this-project")
	includeTools := popBool(&a, "--include-tools")
	includeSubagents := popBool(&a, "--include-subagents")
	noVector := popBool(&a, "--no-vector") // force keyword-only (skip vector fusion)
	_ = popBool(&a, "--no-budget")         // deprecated no-op: no cap is now the default
	limitS := popVal(&a, "--limit")
	budgetS := popVal(&a, "--budget")
	focus := popVal(&a, "--focus")
	// Scope filters (#1): honor the SAME discovery flags the human path does.
	// Popping their VALUES here is what stops them leaking into the FTS5 query as
	// junk OR-terms (e.g. "assistant", "999999").
	role := popVal(&a, "--role")
	sort := popVal(&a, "--sort")
	since := popVal(&a, "--since")
	before := popVal(&a, "--before")
	includePath := popVal(&a, "--include-path")
	excludePath := popVal(&a, "--exclude-path")
	minMessagesS := popVal(&a, "--min-messages")
	// Expand-in-place ladder (#4): --more [level] widens the window on the SAME
	// ref; --around N re-centers it N messages from the anchor. Both are pure
	// follow-ups — never a re-search.
	moreS := popVal(&a, "--more")
	bareMore := popBool(&a, "--more")
	aroundS := popVal(&a, "--around")

	limit := DefaultSearchLimit
	if limitS != "" {
		// Ignore a parse error and keep the default (the only safe headless
		// behavior for a bad --limit value).
		if n, err := strconv.Atoi(limitS); err == nil {
			limit = n
		}
	}
	// Budget flip (#3): a read returns whole by DEFAULT (budget == nil). --budget
	// is an opt-in ceiling, meaningful mainly across a multi-message window. A
	// bare --budget with no number (left in args by popVal) falls back to the
	// documented DefaultReadBudget.
	bareBudget := popBool(&a, "--budget")
	var budget *int
	if budgetS != "" || bareBudget {
		b := DefaultReadBudget
		if n, err := strconv.Atoi(budgetS); err == nil {
			b = n
		}
		budget = &b
	}

	// --more level: bare --more = level 1; --more N = level N. Resolve to a
	// window radius via moreWindow (0 ⇒ default ±ReadWindow).
	readWindow := 0
	if moreS != "" {
		if n, err := strconv.Atoi(moreS); err == nil {
			readWindow = moreWindow(n)
		} else {
			readWindow = moreWindow(1)
		}
	} else if bareMore {
		readWindow = moreWindow(1)
	}
	// --around N: re-center the window N messages from the anchor.
	readAround := 0
	if aroundS != "" {
		if n, err := strconv.Atoi(aroundS); err == nil {
			readAround = n
		}
	}
	// --min-messages N: a bad value is ignored (kept at 0 = no minimum), the only
	// safe headless behavior — matching how --limit treats a bad value.
	minMessages := 0
	if minMessagesS != "" {
		if n, err := strconv.Atoi(minMessagesS); err == nil {
			minMessages = n
		}
	}

	// Enum flags: never-silent but graceful. A bad value would otherwise filter by a
	// role that matches nothing (silent empty) or be ignored without a word — so note
	// it and fall back to unset/default rather than hard-erroring an agent's run.
	if role != "" && role != "user" && role != "assistant" {
		fmt.Fprintf(errw, "note: ignoring --role %q (choose from user, assistant)\n", role)
		role = ""
	}
	if sort != "" && sort != "newest" && sort != "oldest" {
		fmt.Fprintf(errw, "note: ignoring --sort %q (choose from newest, oldest)\n", sort)
		sort = ""
	}

	if len(a) == 0 {
		fmt.Fprintln(errw, "usage: rawclaw agent <search|read|outline> [args]  (--json for machine output)")
		return 1
	}

	verb := strings.ToLower(a[0])
	a = a[1:]
	positional := []string{}
	for _, x := range a {
		if !strings.HasPrefix(x, "--") {
			positional = append(positional, x)
		}
	}

	scope, scopeLabel, ok := buildScope(thisProject, errw)
	if !ok {
		return 1
	}

	switch verb {
	case "search":
		var embedder embed.Embedder
		if !noVector {
			embedder = adapters.GetEmbedder()
		}
		return runSearch(out, errw, positional, scope, scopeLabel, wantJSON, embedder,
			SearchOpts{
				Limit:            limit,
				Role:             role,
				Sort:             sort,
				IncludeTools:     includeTools,
				IncludeSubagents: includeSubagents,
				Since:            since,
				Before:           before,
				MinMessages:      minMessages,
				IncludePath:      includePath,
				ExcludePath:      excludePath,
			})
	case "read":
		return runRead(out, errw, positional, scope, wantJSON, ReadOpts{
			Focus:        focus,
			Budget:       budget,
			IncludeTools: includeTools,
			Window:       readWindow,
			Around:       readAround,
		})
	case "outline":
		return runOutline(out, errw, positional, scope, wantJSON, includeTools)
	default:
		fmt.Fprintf(errw, "unknown verb '%s' — expected search|read|outline\n", verb)
		return 1
	}
}

// buildScope resolves the active scope from --this-project. On --this-project
// with no transcript history it prints the hint and reports ok=false (exit 1).
func buildScope(thisProject bool, errw io.Writer) (scope []view.Scope, label string, ok bool) {
	if thisProject {
		sc := thisScope()
		if sc == nil {
			fmt.Fprintln(errw, "No transcript history for this directory. Try without --this-project.")
			return nil, "", false
		}
		return sc, "on this project", true
	}
	return allScope(), "across all projects", true
}

func runSearch(
	out, errw io.Writer,
	positional []string,
	scope []view.Scope,
	scopeLabel string,
	wantJSON bool,
	embedder embed.Embedder,
	opts SearchOpts,
) int {
	if len(positional) == 0 {
		fmt.Fprintln(errw, "usage: rawclaw agent search <query>")
		return 1
	}
	query := strings.Join(positional, " ")
	env := Search(query, scope, opts, embedder)
	if wantJSON {
		if err := emit(out, env); err != nil {
			fmt.Fprintf(errw, "error: %v\n", err)
			return 1
		}
		return 0
	}
	renderSearch(out, env, query, scopeLabel)
	return 0
}

func runRead(
	out, errw io.Writer,
	positional []string,
	scope []view.Scope,
	wantJSON bool,
	opts ReadOpts,
) int {
	if len(positional) == 0 {
		fmt.Fprintln(errw, "usage: rawclaw agent read <session8:uuid8> [--focus TERM] [--budget N] [--more [level]] [--around N]")
		return 1
	}
	result, err := Read(positional[0], scope, opts)
	if err != nil {
		fmt.Fprintf(errw, "error: %v\n", err)
		return 1
	}
	if wantJSON {
		if err := emit(out, result); err != nil {
			fmt.Fprintf(errw, "error: %v\n", err)
			return 1
		}
		return 0
	}
	renderRead(out, result)
	return 0
}

func runOutline(
	out, errw io.Writer,
	positional []string,
	scope []view.Scope,
	wantJSON bool,
	includeTools bool,
) int {
	if len(positional) == 0 {
		fmt.Fprintln(errw, "usage: rawclaw agent outline <session8>")
		return 1
	}
	result, err := Outline(positional[0], scope, includeTools)
	if err != nil {
		fmt.Fprintf(errw, "error: %v\n", err)
		return 1
	}
	if wantJSON {
		if err := emit(out, result); err != nil {
			fmt.Fprintf(errw, "error: %v\n", err)
			return 1
		}
		return 0
	}
	renderOutline(out, result)
	return 0
}

// emit writes obj as pretty JSON: two-space indent, no HTML escaping, and a
// trailing newline.
func emit(w io.Writer, obj any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(obj); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

// popBool removes flag from a if present and reports whether it was found.
func popBool(a *[]string, flag string) bool {
	for i, x := range *a {
		if x == flag {
			*a = append((*a)[:i], (*a)[i+1:]...)
			return true
		}
	}
	return false
}

// popVal removes flag and its following value from a, returning the value (or ""
// if absent / no value follows).
func popVal(a *[]string, flag string) string {
	for i, x := range *a {
		if x != flag {
			continue
		}
		if i+1 < len(*a) {
			val := (*a)[i+1]
			*a = append((*a)[:i], (*a)[i+2:]...)
			return val
		}
		return ""
	}
	return ""
}
