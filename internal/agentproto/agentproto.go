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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/query"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/semantic"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
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
	// Topic is the topic-layer label covering the matched message, attached
	// after ranking as a display-only annotation (see attachTopics). It never
	// participates in matching or ordering. Empty for an untagged session;
	// omitempty then keeps the JSON identical to an untagged corpus.
	Topic string `json:"topic,omitempty"`

	// Last is where the hit's session ENDED UP: its most recent real activity,
	// attached after ranking (see attachLastActivity). A hit is a point in the
	// middle of a conversation — in a 1500-message session the match can predate
	// the session's actual conclusion by hours, and nothing on the hit line said
	// so. Like Topic this is display-only and cannot influence ranking. Empty
	// when the session's whole scanned tail is machinery, which renders as no
	// line at all rather than a hit captioned with a tool result.
	Last string `json:"last,omitempty"`

	// Missing is true when this conversation's backing source file is gone but its
	// content was retained in the index (durable retention, D1). Surfaced so a
	// retained-but-missing hit is not read as current state (D7). omitempty keeps
	// the JSON byte-identical for the common present case.
	Missing bool `json:"missing,omitempty"`
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

	// NarrowHint fires when a query matches MANY conversations — the terms are
	// corpus-common, so the important/derived hit is buried under incidental
	// mentions (relevance ranks by token match, not importance). Steers to a
	// distinctive token + scoping, and warns against skipping on the snippet alone.
	NarrowHint string `json:"narrow_hint,omitempty"`

	// ExcludedCurrentTurn counts the candidates withheld as the caller's own live
	// turn (SearchOpts.CurrentSession). Reported rather than dropped quietly: an
	// agent that knows a record was withheld can ask for it; one that doesn't
	// just sees a hole. 0 = nothing was withheld, which is the case whenever the
	// caller didn't say where it was.
	ExcludedCurrentTurn int `json:"excluded_current_turn,omitempty"`
}

// ReadResult is a bounded excerpt around a ref. Embeds the AnchoredView shape
// plus protocol metadata.
type ReadResult struct {
	Project      string `json:"project"`
	SessionID    string `json:"session_id"`
	AnchorID     int    `json:"anchor_id"`
	FocusSnippet string `json:"focus_snippet"`
	CharBudget   *int   `json:"char_budget"` // nil = no cap
	ReadRef      string `json:"read_ref"`    // the stable "<session8>:<uuid8>" ref this read resolved
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
	Topics       []string       `json:"topics,omitempty"` // topic-layer segment labels for this session, in order
}

// SearchOpts groups the optional search filters (keeps the signature small).
// The scope filters (Role/Since/Before/MinMessages/IncludePath/ExcludePath)
// mirror the default-discovery flags so the default search honors the SAME scoping
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

	// CurrentSession is the session the caller is live in ("" = unknown, the
	// pre-existing behavior). Its CURRENT TURN — and only that — is withheld from
	// the results; see dropCurrentTurn for what the turn is and why the rest of
	// the session stays searchable.
	CurrentSession string
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

// normalizeRefArg drops a leading "ref=" from a pasted ref argument. Search,
// read, and topics output print every ref as `read ref=<session8>:<uuid8>`;
// agents copy that token verbatim, so the verbs must accept exactly what the
// output printed.
func normalizeRefArg(ref string) string {
	return strings.TrimPrefix(ref, "ref=")
}

// normalizeSessionArg normalizes a pasted session argument for the
// session-taking verbs (outline, tag): a leading "ref=" is dropped, and a full
// pasted read-ref keeps only its <session8> half (no real session id contains
// a colon). A paste that normalizes to nothing is returned verbatim, so the
// not-found error names what the user actually typed instead of matching
// every session on an empty prefix.
func normalizeSessionArg(s string) string {
	out := normalizeRefArg(s)
	if i := strings.IndexByte(out, ':'); i >= 0 {
		out = out[:i]
	}
	if out == "" {
		return s
	}
	return out
}

// resolveRef parses "<session8>:<uuid8>" → (session8, uuid8). The second half is
// now an opaque hex prefix (the message uuid), not an integer rowid. A purely
// numeric second half is a pre-migration ref and returns a migration hint.
// A leading "ref=" — the form search output prints — is accepted and dropped.
func resolveRef(ref string) (string, string, error) {
	parts := strings.Split(normalizeRefArg(ref), ":")
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

// allScope returns every search scope across all runtimes (Claude projects +
// Codex cwd-groups), via the scopes enumerator. context.Background() is
// deliberate: this is only the nil-scope fallback of the verb API, and the
// verbs take a scope, not a ctx. Callers with a run deadline (the cli) build
// their scope under the watchdog ctx and pass it in (cli.verbScope/allScope);
// what remains here serves scope-agnostic library/test callers that have no
// run context to bound the archive enumeration with.
func allScope() []view.Scope {
	return scopes.All(context.Background(), "", false)
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
		scope = scopes.FilterByPath(scope, opts.IncludePath, opts.ExcludePath)
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

	// Withhold the caller's own live turn before anything is ranked: the prompt
	// just typed is not recall, and letting it win a slot costs a real result.
	cands, droppedTurn := dropCurrentTurn(cands, opts.CurrentSession)

	sortCandidates(cands, opts.Sort)

	// Build every DISTINCT result first, then cap to `limit`. Capping after the
	// full dedup lets us report Complete=false when the limit hid real candidates
	// (#2), so an agent that sees N of many knows the set is incomplete.
	seen := map[string]struct{}{}
	all := []SearchRef{}
	// picked stays index-parallel to all: the anchor each ref was built from,
	// kept so the post-ranking topic pass can reach that anchor's database and
	// message uuid without re-running the search.
	picked := []retrieve.Anchor{}
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
			Missing:   r.MissingSince > 0,
		})
		picked = append(picked, r)
	}

	results := all
	truncated := false
	if limit >= 0 && len(all) > limit {
		results = all[:limit]
		picked = picked[:limit]
		truncated = true
	}

	// Topic labels and last-activity lines are attached HERE — after collection,
	// fusion, sorting, dedup and capping have all finished — so neither can
	// influence which conversations come back or in what order. See attachTopics
	// and attachLastActivity.
	attachTopics(results, picked)
	attachLastActivity(results, picked)

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
		nextCmd = fmt.Sprintf("rawclaw %q --limit %d", rawQuery, wider)
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
	// Drowning steer (F4): a query matching many conversations has terms too common —
	// the important/derived hit is buried under incidental mentions, and relevance
	// ranks by token match, not importance. Steer to a distinctive token + scoping,
	// and tell the agent NOT to skip on the snippet alone (the snippet hides which
	// hit actually matters).
	// Drowning steer: too-broad query → the derived/important hit is buried under
	// incidental mentions (relevance ranks by token match, not importance). Grounded
	// in the github-search atlas's universal breadth recipe: narrow with SCOPE FILTERS
	// first (the workhorse — path/project/date), keep to a few distinctive/literal
	// terms (atlas "<=3 terms for signal"), and don't judge on the snippet alone.
	// Fires on a real boundary — the fetch ceiling — OR many distinct matches.
	narrowHint := ""
	if hitCeiling || total >= 20 {
		narrowHint = "broad query — scope it first: --include-path <re> / --this-project / --since <date>; " +
			"then keep to a few distinctive terms (a filename, flag, error, or \"quoted phrase\") — " +
			"3 or fewer. Open a ref to judge; the snippet hides which hit is the important one."
	}

	return SearchEnvelope{
		Results:             results,
		Scopes:              reports,
		Complete:            scopesComplete(reports) && !hasMore,
		Count:               len(results),
		TotalMatches:        total,
		TotalIsLowerBound:   hitCeiling,
		HasMore:             hasMore,
		NextCommand:         nextCmd,
		RecencyHint:         recencyHint,
		NarrowHint:          narrowHint,
		ExcludedCurrentTurn: droppedTurn,
	}
}

// SearchAndRender runs Search and writes the result to w: the agent envelope as
// text, or as JSON when wantJSON. This is the exported entry the default CLI path
// calls so a bare `rawclaw "query"` IS the search — search is the default verb.
// scopeLabel is the human-facing "across all projects" / "on <project>"
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
// so reading is a top-level verb (`rawclaw read <ref>`).
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
		dbp, status, err := scopes.Resolve(sc, false)
		if err != nil {
			rep.Status = ScopeSkippedError
			rep.Detail = err.Error()
			reports = append(reports, rep)
			continue
		}
		con, openErr := store.ConnectRO(dbp)
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

// dropCurrentTurn removes the caller's CURRENT TURN from the candidate pool and
// reports how many rows it withheld.
//
// The problem: a live session's records are indexed as they are written, so the
// prompt the operator just typed is in the index by the time the agent runs a
// search on it. It matches its own words better than anything in the archive
// does, and it wins the top slot with a record that tells the caller nothing it
// doesn't already know — displacing a real result.
//
// The scope here is deliberately the narrowest thing that fixes that, and is NOT
// the session and NOT its lineage. In a long session the earlier parts of that
// same session are legitimate, valuable history — often the most relevant
// history there is, because it is the same thread of work — and they stay fully
// searchable. Only the turn in flight goes.
//
// "The turn in flight" is the run from the newest message a PERSON typed to the
// end of the session: currentTurnStart finds that message and everything at or
// after its id is withheld. What sits after it is this turn's own tool results,
// injected envelopes and reasoning — records the caller produced seconds ago,
// not recall. What sits before it is history, and is left alone.
//
// The boundary is resolved once per database rather than once per candidate. A
// session continued from a second directory has a row in each project's
// database, so there can legitimately be more than one.
func dropCurrentTurn(cands []retrieve.Anchor, currentSession string) ([]retrieve.Anchor, int) {
	if currentSession == "" || len(cands) == 0 {
		return cands, 0
	}
	starts := map[string]int{}
	for i := range cands {
		dbp := cands[i].DBP
		if dbp == "" || !isCurrentSession(cands[i].SessionID, currentSession) {
			continue
		}
		if _, done := starts[dbp]; !done {
			starts[dbp] = currentTurnStart(dbp, cands[i].SessionID)
		}
	}
	if len(starts) == 0 {
		return cands, 0
	}
	out := make([]retrieve.Anchor, 0, len(cands))
	dropped := 0
	for _, a := range cands {
		start := starts[a.DBP]
		if start > 0 && a.ID >= start && isCurrentSession(a.SessionID, currentSession) {
			dropped++
			continue
		}
		out = append(out, a)
	}
	return out, dropped
}

// isCurrentSession reports whether candSID is the session the caller named.
// A full session id and a pasted <session8> both resolve, because 8 hex chars is
// the same handle the read-ref vocabulary already hands agents; below 8 the
// prefix is too loose to act on and only an exact id counts.
//
// An agent sub-session ("<parent>/agent-…") shares its parent's whole id and so
// matches every prefix of it — but it is a DIFFERENT conversation, never the
// caller's own turn, so a match that has to cross a "/" is refused.
func isCurrentSession(candSID, arg string) bool {
	if candSID == arg {
		return true
	}
	if len(arg) < 8 || !strings.HasPrefix(candSID, arg) {
		return false
	}
	return !strings.Contains(candSID[len(arg):], "/")
}

// currentTurnStart returns the message id the caller's live turn begins at: the
// newest record in the session that a person actually typed. 0 means there is no
// such record at all, and 0 disables the exclusion — nothing to anchor the turn
// on means nothing to withhold, and withholding something else would be worse
// than doing nothing.
//
// The boundary is found by a predicate over the whole session (store.NewestHuman
// MessageID), not by walking a fixed window back from the tail. An earlier
// version scanned the last 40 records, on the assumption that the caller's
// prompt sits within a handful of records of the end. Measured against the live
// corpus that assumption is wrong by a wide margin: roughly 85% of role=user rows
// are machinery — tool results and injected envelopes — and in the session that
// exposed this the newest human-typed message was 65 records back. The window
// found nothing, returned 0, and silently turned the feature off while its tests
// stayed green. Any fixed window is a guess about a ratio that varies per turn,
// so there is no window here.
//
// "A person typed it" is role=user minus the machinery: tool results, injected
// envelopes, and the runtime's interruption marker are all excluded, the last for
// the same reason view.isInterruptionMarker excludes it from the browse tail —
// it is the runtime reporting a stop, not anything either party said.
func currentTurnStart(dbp, sessionID string) int {
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return 0
	}
	defer con.Close()
	id, err := store.NewestHumanMessageID(con, sessionID)
	if err != nil {
		return 0
	}
	return id
}

// attachTopics fills in each result's Topic label, in place. refs and anchors
// are index-parallel: anchors[i] is the anchor refs[i] was built from.
//
// The label is a DISPLAY-ONLY annotation. It is looked up strictly after the
// result set has been chosen and ordered, keyed on the message that was already
// selected, so it cannot change which conversations are returned or where they
// rank. This is what makes putting the label back on the hit line safe: the
// original objection to carrying topics in search was that a label sharing a
// word with the query would mis-route the ranking, and a lookup that runs after
// ranking has no way to do that. TestTopicLabelDoesNotAffectOrdering holds the
// property.
//
// Lookups are grouped by database so a result set spanning several projects
// opens each project's database once. Any failure is silent: an untagged
// session, a missing topic table, or an unreadable database all leave the label
// empty, which renders exactly as it did before topics existed.
func attachTopics(refs []SearchRef, anchors []retrieve.Anchor) {
	if len(refs) == 0 || len(refs) != len(anchors) {
		return
	}
	byDB := map[string][]int{}
	for i := range anchors {
		if anchors[i].DBP == "" || anchors[i].UUID == "" {
			continue
		}
		byDB[anchors[i].DBP] = append(byDB[anchors[i].DBP], i)
	}
	for dbp, idxs := range byDB {
		con, err := store.ConnectRO(dbp)
		if err != nil {
			continue
		}
		for _, i := range idxs {
			refs[i].Topic = store.TopicForMessage(con, anchors[i].SessionID, anchors[i].UUID)
		}
		_ = con.Close()
	}
}

// attachLastActivity fills in each result's Last line, in place. refs and
// anchors are index-parallel, exactly as in attachTopics.
//
// A search hit is a point in the MIDDLE of a conversation. The hit line said
// when the match happened and what it said, but never whether the session went
// anywhere afterwards — in a long session the match can sit hours before the
// real conclusion. This is the same question a browse row answers with its "now"
// line, so it reuses the same reader: view.SessionLastActivity, the newest
// message that still has content once tool runs and injected envelopes are
// stripped, skipping the runtime's interruption marker. One definition of "real
// activity", or the two surfaces drift.
//
// It carries ba0c430's honesty rule with it: when the whole scanned tail is
// machinery the lookup returns "" and the hit gets no line, rather than being
// captioned with a tool result.
//
// Like the topic label this is DISPLAY ONLY — run after collection, fusion,
// sorting, dedup and capping, keyed on results already chosen, so it cannot
// reach the ranking. TestLastActivityDoesNotAffectOrdering holds the property.
// The tail is read for the hit's OWN session id, not its lineage root: the hit
// names one session, and "where that session ended up" is the honest claim.
//
// Lookups are grouped by database, so a result set spanning several projects
// opens each project's database once — this is a per-result query and the
// grouping is what keeps it one connection per project rather than per hit. Any
// failure is silent and renders as no line.
func attachLastActivity(refs []SearchRef, anchors []retrieve.Anchor) {
	if len(refs) == 0 || len(refs) != len(anchors) {
		return
	}
	byDB := map[string][]int{}
	for i := range anchors {
		if anchors[i].DBP == "" || anchors[i].SessionID == "" {
			continue
		}
		byDB[anchors[i].DBP] = append(byDB[anchors[i].DBP], i)
	}
	for dbp, idxs := range byDB {
		con, err := store.ConnectRO(dbp)
		if err != nil {
			continue
		}
		// Sessions repeat across hits only when the dedup let them through; the
		// cache spares the duplicate tail read either way.
		cache := map[string]string{}
		for _, i := range idxs {
			sid := anchors[i].SessionID
			last, done := cache[sid]
			if !done {
				last = view.SessionLastActivity(con, sid)
				cache[sid] = last
			}
			refs[i].Last = last
		}
		_ = con.Close()
	}
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

	con, err := store.ConnectRO(dbp)
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
	// foreign marks a scope replicated from another machine (view.Scope.Origin
	// non-empty). A foreign database is a read replica: tag export deliberately
	// skips it, so writing a tag there would be dropped at sync time.
	foreign bool
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
		var cs []sessionCand
		for _, sc := range scope {
			dbpC, _, ensureErr := scopes.Resolve(sc, false)
			if ensureErr != nil {
				continue
			}
			con, openErr := store.ConnectRO(dbpC)
			if openErr != nil {
				continue
			}
			// Fetch up to 2 per project: enough to detect an in-project collision;
			// cross-project collisions surface in the aggregate below.
			sids, qErr := store.SessionsByPrefix(con, session8, !excludeSub, 2)
			_ = con.Close()
			if qErr != nil {
				continue
			}
			for _, sid := range sids {
				cs = append(cs, sessionCand{SessionID: sid, Project: sc.Project, dbp: dbpC, foreign: sc.Origin != ""})
			}
		}
		return cs
	}

	cands := collect(true)
	if len(cands) == 0 {
		cands = collect(false)
	}
	cands = coalesceSameSession(cands)

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

// coalesceSameSession collapses candidates that share the SAME full session id
// down to one row, because they ARE one session.
//
// This is not a prefix collision. Continue a session from a different directory
// and the agent writes a second transcript under the new project while keeping
// the session id, so the scope sweep finds a row in each project. Delete the
// first directory and durable retention keeps its row with a missing_since
// watermark. Either way it is one conversation, and reporting it as an
// ambiguity was unhelpable advice: the error said "give a longer prefix" when
// the full ids are byte-identical and no prefix can ever separate them, leaving
// the session unreachable to read, outline and tag.
//
// Which row represents it: prefer one whose backing source file is still live
// over a retained stub, then the one holding more messages. That picks the
// current transcript rather than the abandoned half. Rows for genuinely
// DIFFERENT ids are left alone, so a real prefix collision still raises.
func coalesceSameSession(cands []sessionCand) []sessionCand {
	if len(cands) < 2 {
		return cands
	}
	type ranked struct {
		cand  sessionCand
		live  bool
		count int
	}
	// better reports whether a beats b. Order matters and is deliberate:
	//   1. LOCAL beats foreign. A foreign scope is a replica of another machine's
	//      database; tag export skips foreign databases, so resolving here to a
	//      foreign row would send `tag-write` at a database whose new rows never
	//      sync back — the tag would be silently dropped or overwritten on the
	//      next ingest.
	//   2. A live source beats a retained stub (the transcript still exists).
	//   3. More messages wins (the fuller half of a continued session).
	better := func(a, b ranked) bool {
		if a.cand.foreign != b.cand.foreign {
			return !a.cand.foreign
		}
		if a.live != b.live {
			return a.live
		}
		return a.count > b.count
	}
	best := make(map[string]ranked, len(cands))
	order := make([]string, 0, len(cands))

	for _, c := range cands {
		live, count := false, 0
		if con, err := store.ConnectRO(c.dbp); err == nil {
			if mc, isLive, ok := store.SessionRowQuality(con, c.SessionID); ok {
				live, count = isLive, mc
			}
			_ = con.Close()
		}
		prev, seen := best[c.SessionID]
		if !seen {
			order = append(order, c.SessionID)
			best[c.SessionID] = ranked{cand: c, live: live, count: count}
			continue
		}
		if cur := (ranked{cand: c, live: live, count: count}); better(cur, prev) {
			best[c.SessionID] = cur
		}
	}

	out := make([]sessionCand, 0, len(order))
	for _, sid := range order {
		out = append(out, best[sid].cand)
	}
	return out
}

// LocateSession resolves a session8 prefix to its (db path, full session id)
// across scope (nil = all projects), the exported door onto the private
// locateSession. The `tag` verb uses it to find the db it must open read-write
// and the full session id whose messages it tags. Returns *ErrSessionNotFound /
// *ErrAmbiguousSession unchanged, so callers can render the same hints as Read
// and Outline.
func LocateSession(session8 string, scope []view.Scope) (dbPath, fullSID string, err error) {
	if scope == nil {
		scope = allScope()
	}
	dbp, sid, _, err := locateSession(scope, normalizeSessionArg(session8))
	return dbp, sid, err
}

// ── verb: outline ────────────────────────────────────────────────────────────

// Outline returns a session's bookend arc (first/last N user+assistant messages).
// Returns an error if the session is not found. A pasted read-ref token
// ("ref=<session8>:<uuid8>" or "<session8>:<uuid8>") resolves via its session half.
func Outline(session8 string, scope []view.Scope, includeTools bool) (*OutlineResult, error) {
	if scope == nil {
		scope = allScope()
	}
	session8 = normalizeSessionArg(session8)

	dbp, fullSID, proj, locErr := locateSession(scope, session8)
	if locErr != nil {
		return nil, locErr
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", dbp, err)
	}
	defer con.Close()

	iso, nmsg := sessionMeta(con, fullSID)

	startRows, err := bookendRows(con, fullSID, true)
	if err != nil {
		return nil, fmt.Errorf("outline start rows: %w", err)
	}
	endRows, err := bookendRows(con, fullSID, false)
	if err != nil {
		return nil, fmt.Errorf("outline end rows: %w", err)
	}

	startIDs := map[int]struct{}{}
	for _, r := range startRows {
		startIDs[r.ID] = struct{}{}
	}

	// endRows came back DESC; reverse to chronological, then drop any already
	// present in the start bookend so the two ends don't overlap.
	endMsgs := []store.Msg{}
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

	// Topic layer: list the session's tagged segments (empty when untagged or the
	// topic table is absent — a non-fatal "no topics").
	var topics []string
	if segs, terr := store.TopicsForSession(con, fullSID); terr == nil {
		for _, s := range segs {
			if s.Topic != "" {
				topics = append(topics, s.Topic)
			}
		}
	}

	return &OutlineResult{
		Project:      proj,
		SessionID:    fullSID,
		ISO:          iso,
		MessageCount: nmsg,
		Start:        startOut,
		End:          endOut,
		MidCount:     midCount,
		Topics:       topics,
	}, nil
}

// sessionMeta reads last_ts + message_count and formats the ISO through the
// timefmt seam: the outline header is an agent-parsed surface, so the instant
// is marked UTC (RFC3339 "Z") — never an unmarked local time. A missing row →
// ("", 0); a missing/zero last_ts → "" iso.
func sessionMeta(con *sql.DB, fullSID string) (iso string, nmsg int) {
	lastTS, mc, ok := store.SessionMeta(con, fullSID)
	if !ok {
		return "", 0
	}
	if lastTS != 0 {
		iso = timefmt.UTC(time.Unix(int64(lastTS), 0))
	}
	return iso, mc
}

// bookendRows reads up to OutlineBookend user/assistant messages with non-empty
// content, ordered by id ascending (asc=true) or descending.
func bookendRows(con *sql.DB, fullSID string, asc bool) ([]store.Msg, error) {
	return store.BookendMessages(con, fullSID, 0, false, asc, OutlineBookend)
}

// ── text renderers ───────────────────────────────────────────────────────────

// renderSearch prints the human-readable search output. When the envelope is
// incomplete it appends a one-line footer naming how many scopes were skipped or
// stale, so the agent reads a partial result AS partial (#6).
func renderSearch(w io.Writer, env SearchEnvelope, query, scopeLabel string) {
	if len(env.Results) == 0 {
		// An empty result set with rows withheld is the one case where the
		// standard "rephrase" advice is actively wrong: the query DID match, and
		// telling the caller to rewrite it sends them chasing a wording problem
		// that does not exist. Say what happened instead, and name the flag that
		// shows the withheld rows. This is also the case where staying silent
		// would be a lie — "No matches" over a set we chose not to return.
		if env.ExcludedCurrentTurn > 0 {
			fmt.Fprintf(w, "No matches outside the turn you are in now — %s\n", currentTurnLine(env.ExcludedCurrentTurn))
			fmt.Fprintln(w, "To see them anyway, re-run with --current-session off.")
			renderScopeFooter(w, env)
			return
		}
		fmt.Fprintln(w, "No matches. Lead with a single distinctive term that appears in the text (a filename, flag, or error string), not a topic word — or rephrase.")
		renderScopeFooter(w, env)
		return
	}
	fmt.Fprintf(w, "%d conversation(s) matching '%s' %s:\n\n", len(env.Results), query, scopeLabel)
	for _, r := range env.Results {
		// timefmt seam: search results are agent-parsed — render the stored ISO
		// as marked UTC (unparseable stamps pass through verbatim).
		// Compact marked-UTC stamp: seconds cost line width that the topic label
		// uses better, and the "Z" stays so the stamp is never ambiguous.
		iso := timefmt.UTCShortFromISO(r.ISO)
		if iso == "" {
			iso = "?"
		}
		miss := ""
		if r.Missing {
			miss = " · source file gone — retained history"
		}
		// The topic label answers "what was this session about?" on the header
		// line, so an agent can choose a hit without opening it. Quoted to mark
		// it as an authored label rather than another identifier, and omitted
		// entirely for an untagged session rather than rendered as an empty slot.
		topic := ""
		if r.Topic != "" {
			topic = fmt.Sprintf(" · %q", r.Topic)
		}
		fmt.Fprintf(w, "  ━━ %s · %s · %s%s%s\n", iso, sid8(r.SessionID), r.Project, topic, miss)
		fmt.Fprintf(w, "     …%s…\n", r.Snippet)
		// Where this conversation ended up, under the point it matched at. Same
		// "now →" vocabulary browse uses for the same fact (render.printLastActivity),
		// and omitted entirely when the tail held nothing but machinery.
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
	if env.RecencyHint != "" {
		fmt.Fprintf(w, "note: %s\n", env.RecencyHint)
	}
	if env.NarrowHint != "" {
		fmt.Fprintf(w, "note: %s\n", env.NarrowHint)
	}
	if env.ExcludedCurrentTurn > 0 {
		fmt.Fprintf(w, "note: %s\n", currentTurnLine(env.ExcludedCurrentTurn))
	}
	renderScopeFooter(w, env)

	// Cheap disambiguation: when the matches span 2+ distinct projects, say so and
	// point at the narrowing flags — a factual spread signal, no new heuristic.
	if line := projectSpreadLine(env.Results); line != "" {
		fmt.Fprintln(w, line)
	}
	// Freshness: the last footer line, always — these are raw transcripts, not the
	// current state of the world.
	fmt.Fprintln(w, freshnessNote)
}

// currentTurnLine states what the current-turn exclusion withheld. It names the
// turn, not the session, because the session's earlier history was searched —
// an agent must not read this as "my own session was skipped".
func currentTurnLine(n int) string {
	return fmt.Sprintf("excluded %d record(s) of the turn you are in now; the rest of that session was searched", n)
}

// freshnessNote is the standing reminder that search/read results are raw session
// history, not current truth — appended as the last footer line of both renderers.
const freshnessNote = "note: raw session history — verify against current state before acting."

// projectSpreadLine returns a one-line "matches span N projects: …" hint when the
// results cover 2+ distinct projects (listing up to 5), or "" for a single project.
func projectSpreadLine(results []SearchRef) string {
	seen := map[string]struct{}{}
	var distinct []string
	for _, r := range results {
		if _, dup := seen[r.Project]; dup {
			continue
		}
		seen[r.Project] = struct{}{}
		distinct = append(distinct, r.Project)
	}
	if len(distinct) < 2 {
		return ""
	}
	shown := distinct
	if len(shown) > 5 {
		shown = shown[:5]
	}
	return fmt.Sprintf("matches span %d projects: %s — narrow with --this-project or read a specific ref.",
		len(distinct), strings.Join(shown, ", "))
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
	if skipped == 0 {
		return
	}
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
		next = "rawclaw read " + sid8(r.SessionID) + ":" + " --more"
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
		// keep-reading hint (the note already names the literal next command).
		fmt.Fprintln(w, truncNote)
	} else {
		fmt.Fprintf(w, "\n  keep reading:  rawclaw read %s --more   (or --around N to shift)\n", r.ReadRef)
	}
	// Freshness: the last footer line, always.
	fmt.Fprintln(w, freshnessNote)
}

// renderOutline prints the human-readable outline output.
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
}

// ── verb: topics ─────────────────────────────────────────────────────────────

// TopicHit is one on-demand topic-finder result: a tagged topic label, its
// project, and a read-ref (<session8>:<uuid8>) pointing at where that topic
// BEGINS (the segment's start message). Topics are deliberately OUT of the
// default search ranking — this is the separate tool an agent reaches for only
// when a normal search is ambiguous.
type TopicHit struct {
	Topic   string `json:"topic"`
	Project string `json:"project"`
	ReadRef string `json:"read_ref"`
}

// TopicsResult wraps the topic-finder hits with the query and a note. Note is
// the helpful empty-state hint when no topics are tagged anywhere in scope.
type TopicsResult struct {
	Query string     `json:"query"`
	Hits  []TopicHit `json:"hits"`
	Note  string     `json:"note,omitempty"`
}

// topicFetch is how many topic rows to pull from each project's database for a
// requested result limit. The surplus is what collapsing repeats consumes; it
// mirrors the keyword search fetch window (8x, floor 30) rather than inventing
// a second rule.
func topicFetch(limit int) int {
	f := limit * 8
	if f < 30 {
		f = 30
	}
	return f
}

// topicsEmptyNote is the empty-state hint printed/emitted when no topic rows
// exist anywhere in scope — tells the agent how topics get tagged.
const topicsEmptyNote = "no topics tagged yet — a session is tagged via the rawclaw-topic-tagger subagent"

// Topics searches ONLY the topic layer across scope (nil = all projects),
// returning hits ordered per-project by FTS rank, capped at limit per project,
// at one row per conversation per label. Each hit resolves the segment's START
// message id (what MatchTopics returns) to its uuid for a read-ref. It never
// touches the keyword/vector ranking — this is the separate, on-demand finder.
// anyTopics reports whether ANY topic rows exist in scope (regardless of query
// match), so the caller can tell "no match" apart from "nothing tagged yet".
//
// Ordering is deliberately PER-PROJECT, not global. Each project is its own
// SQLite file, and raw bm25 scores are NOT comparable across independent
// databases — bm25 folds in per-database corpus statistics, so a weak hit in a
// small database can outscore a strong hit in a large one. Merging by score was
// tried and reverted for exactly that reason. A correct global ranking needs one
// shared index, not a smarter merge; that is tracked as the consolidation work.
func Topics(query string, scope []view.Scope, limit int, includePath string) (TopicsResult, error) {
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if scope == nil {
		scope = allScope()
	}
	if includePath != "" {
		scope = scopes.FilterByPath(scope, includePath, "")
	}

	hits := []TopicHit{}
	anyTopics := false
	for _, sc := range scope {
		dbp, _, err := scopes.Resolve(sc, false)
		if err != nil {
			continue // a failing project is skipped (mirrors locateSession)
		}
		con, openErr := store.ConnectRO(dbp)
		if openErr != nil {
			continue
		}
		if err := store.EnsureTopicSchema(con); err != nil {
			_ = con.Close()
			continue
		}
		if store.TopicRowsExist(con) {
			anyTopics = true
		}
		// Over-fetch per project, the same way search does, so collapsing repeats
		// has material to work with: a project whose top `limit` segments all
		// carry ONE label would otherwise dedup down to a single row and hide
		// every distinct topic sitting below them.
		thits, _ := store.MatchTopics(con, query, topicFetch(limit))

		// Collapse repeats BEFORE this project's cap, so duplicates cannot eat
		// its result slots — the same ordering search uses (build every distinct
		// hit, then cap).
		//
		// A long conversation is often cut into many segments that a tagger gave
		// the SAME label, one per stretch of the discussion. Each of those
		// segments is a separate row here, so one heavily-segmented session could
		// fill the list: observed on the live corpus, `topics "tagging"` returned
		// four identical rows for one session, differing only in which message
		// each read-ref pointed at. Keying on lineage root plus label collapses
		// those to one row — the root rather than the session id, so a resumed or
		// forked session counts as the one conversation it is — while leaving a
		// session that genuinely carries several DIFFERENT labels with one row per
		// label. MatchTopics returns best-first, so the surviving row is that
		// conversation's strongest match for the label.
		//
		// The map is per-project because that is the only place dedup is sound
		// here: it is keyed on a lineage root read from THIS database, and the
		// rank order it consumes is only comparable within it.
		seenTopic := map[string]struct{}{}
		kept := 0
		for _, h := range thits {
			uuid := store.MessageUUID(con, h.MsgID)
			if uuid == "" {
				continue // can't build a read-ref without the message uuid
			}
			root := retrieve.LineageRoot(con, h.SessionID)
			if root == "" {
				root = h.SessionID // no lineage resolved: collapse only exact repeats
			}
			key := root + "\x00" + h.Topic
			if _, dup := seenTopic[key]; dup {
				continue
			}
			seenTopic[key] = struct{}{}
			hits = append(hits, TopicHit{
				Topic:   h.Topic,
				Project: sc.Project,
				ReadRef: fmtRef(h.SessionID, uuid),
			})
			kept++
			if kept == limit {
				break // the cap is per-project, matching the documented contract
			}
		}
		_ = con.Close()
	}

	res := TopicsResult{Query: query, Hits: hits}
	if len(hits) == 0 && !anyTopics {
		res.Note = topicsEmptyNote
	}
	return res, nil
}

// TopicsAndRender runs Topics and writes the result to w (JSON when wantJSON, else
// text). The exported entry the top-level `topics` subcommand calls.
func TopicsAndRender(w io.Writer, query string, scope []view.Scope, limit int, includePath string, wantJSON bool) error {
	result, err := Topics(query, scope, limit, includePath)
	if err != nil {
		return err
	}
	if wantJSON {
		return emit(w, result)
	}
	renderTopics(w, result)
	return nil
}

// renderTopics prints the human-readable topic-finder output: a one-line header
// then one `<topic>  ·  <project>  ·  read ref=<sess8>:<uuid8>` line per hit. The
// empty-state note prints when nothing is tagged anywhere in scope.
func renderTopics(w io.Writer, r TopicsResult) {
	if len(r.Hits) == 0 {
		if r.Note != "" {
			fmt.Fprintf(w, "%s\n", r.Note)
			return
		}
		fmt.Fprintf(w, "No topics matching '%s'. Try a different concept word, or widen scope.\n", r.Query)
		return
	}
	fmt.Fprintf(w, "%d topic(s) matching '%s':\n\n", len(r.Hits), r.Query)
	for _, h := range r.Hits {
		fmt.Fprintf(w, "  %s  ·  %s  ·  read ref=%s\n", h.Topic, h.Project, h.ReadRef)
	}
}

// ── JSON emit ────────────────────────────────────────────────────────────────

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
