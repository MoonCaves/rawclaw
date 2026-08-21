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
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/index"
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

// outlineBookendScan bounds how many rows an outline reads at each end to find
// OutlineBookend that are actually conversation. Same number and same reason as
// view's bookendScan: a session's first and last rows are the densest place in
// the corpus for injected handbooks, command echoes and bare [THINKING] markers.
const outlineBookendScan = 40

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

	// ScopeNotConsolidated marks a project whose own database exists but has
	// never been folded into the one store. A one-store read cannot see it, so it
	// is named rather than left out — the same rule the other statuses follow: a
	// corpus may shrink, but never silently.
	ScopeNotConsolidated = "not_consolidated"
)

// Store values for SearchEnvelope.Store — which index actually answered.
const (
	StoreConsolidated = "consolidated" // one database, one query
	StorePerProject   = "per-project"  // the fan-out: one database per project
)

// ScopeReport records how one project scope fared during a search, so an agent
// reads a partial result AS partial instead of mistaking it for complete (#6).
type ScopeReport struct {
	Project string `json:"project"`
	Dir     string `json:"dir"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
}

// Warning codes. A code is the stable identity of an advisory — the string an
// agent branches on — so the human sentence beside it can be reworded without
// breaking a caller.
const (
	// WarnRecencySkew: relevance put an older hit on top while a much newer match
	// exists. Facts: newest (date of the freshest match).
	WarnRecencySkew = "recency_skew"
	// WarnBroadQuery: the terms are corpus-common, so the hit that matters is
	// buried under incidental mentions. Facts: matches, matches_is_lower_bound.
	WarnBroadQuery = "broad_query"
	// WarnCurrentTurnExcluded: candidates from the caller's own live turn were
	// withheld. Facts: excluded (how many).
	WarnCurrentTurnExcluded = "current_turn_excluded"
	// WarnScopeIncomplete: at least one project was not searched, or was served
	// from a possibly-stale cached index. Facts: scopes, incomplete, errored, stale.
	WarnScopeIncomplete = "scope_incomplete"
	// WarnNotConsolidated: one or more project databases have never been folded
	// into the one store, so their history was outside this answer. A corpus gap
	// with a known fix, which is why it carries the command. Facts: databases.
	WarnNotConsolidated = "not_consolidated"
	// WarnStoreFallback: the one store did not answer, so the per-project fan-out
	// did. The two rank differently, so a caller comparing answers has to know
	// which reader produced this one. Facts: store (which reader answered).
	WarnStoreFallback = "store_fallback"
	// WarnProjectSpread: the hits span several projects, so the set is wider than
	// one context. Facts: projects, sample.
	WarnProjectSpread = "project_spread"
	// WarnRawHistory: the standing reminder that these are raw transcripts, not
	// current truth. No facts — it is a property of the corpus, not of this query.
	WarnRawHistory = "raw_history"
)

// Warning is one advisory carried as data rather than prose (the "warnings are
// data, not prose" doctrine recorded in the prior-art survey under docs/design,
// borrowed from robot-mode output in other agent-facing tools).
//
// Code is what an agent branches on. Facts carries the measurement that made the
// warning fire, so a caller can apply its own threshold instead of inheriting
// ours. Message is the same statement in English, and exists so the text
// renderer holds no copy of its own — the two surfaces cannot drift because
// there is only one string.
type Warning struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Facts   map[string]any `json:"facts,omitempty"`
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

	// Warnings carries every advisory the search wants to raise, as data. Each
	// entry states a code, the fact that triggered it, and the human line — so an
	// agent can branch on Code instead of pattern-matching English, and the text
	// renderer has nothing to decide beyond printing what is present. Empty means
	// the search had nothing to warn about, which is the common case for a narrow
	// query with clean hits.
	Warnings []Warning `json:"warnings,omitempty"`

	// ExcludedCurrentTurn counts the candidates withheld as the caller's own live
	// turn (SearchOpts.CurrentSession). Reported rather than dropped quietly: an
	// agent that knows a record was withheld can ask for it; one that doesn't
	// just sees a hole. 0 = nothing was withheld, which is the case whenever the
	// caller didn't say where it was.
	ExcludedCurrentTurn int `json:"excluded_current_turn,omitempty"`

	// Store names which index answered — the one consolidated store, or the
	// per-project fan-out. The two rank differently (one corpus vs one corpus per
	// project), so a reader comparing two answers has to be able to tell them
	// apart. StoreNote says why the fan-out was used, and is empty otherwise.
	Store     string `json:"store,omitempty"`
	StoreNote string `json:"store_note,omitempty"`
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

	// Project and Source narrow the ONE store by column instead of by choosing
	// which databases to open: Project is the exact project label (the same label
	// paths.ProjectLabel stamps on the row), Source is the source tool. Both empty
	// = the whole corpus. They are read only on the one-store path; the fan-out
	// narrows by the scope list it is given.
	Project string
	Source  string

	// ScopeFallback supplies the per-project scope list, called ONLY if the
	// one-store path cannot answer. It is a function because enumerating every
	// project costs seconds of directory and git probing on a real corpus, and
	// the whole point of the one store is not to pay that on a search that never
	// touches it. Nil falls back to every project under a background context.
	ScopeFallback func() []view.Scope
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

// ScopeFn builds the per-project scope list on demand. Building it is not a
// listing: it opens every per-project index — and after a schema change,
// migrates every one of them — which on a real corpus costs minutes, more than
// the run's watchdog allows. An id the one store can answer needs no list at
// all, so the verbs take the list as a function and call it only if the store
// comes up empty. SearchOpts.ScopeFallback is the same seam for search; a nil
// ScopeFn means "every project", resolved under a background context.
type ScopeFn func() []view.Scope

// resolveScope calls fn, or falls back to every project when fn is nil.
func resolveScope(fn ScopeFn) []view.Scope {
	if fn == nil {
		return allScope()
	}
	return fn()
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

// Search returns rank-ordered conversation refs matching query within scope,
// wrapped in an envelope that reports completeness (#6) so a partial result is
// never mistaken for complete. With a non-nil embedder in relevance mode (no
// --sort), keyword anchors are RRF-fused with vector-KNN anchors — parity with
// the default discovery path.
//
// The scope argument selects which reader answers, and the two rank differently:
//
//   - nil scope means "no project list decided this call" — the one consolidated
//     store answers it, one database and one query, with Project / Source /
//     IncludePath narrowing pushed down as WHERE clauses on the row. Every hit is
//     then scored against ONE corpus, which is what makes the ranking comparable
//     across projects.
//   - a non-nil scope is an explicit list of projects to fan out over, one
//     database each. Scores from separate databases are NOT comparable (bm25 is
//     computed per database), so this ordering is a merge of per-project
//     rankings, and it stays only for the cases the one store cannot serve.
//
// The nil-scope path falls back to the fan-out — via opts.ScopeFallback — when
// the store is absent or empty, or when the requested narrowing names a project
// the store has never heard of. Falling back is always announced in StoreNote.
func Search(rawQuery string, scope []view.Scope, opts SearchOpts, embedder embed.Embedder) SearchEnvelope {
	limit := opts.Limit
	if limit == 0 {
		limit = DefaultSearchLimit
	}

	fetch := limit * 8
	if fetch < 30 {
		fetch = 30
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

	// Embed the query for RRF fusion, only in relevance mode — an explicit
	// --sort stays pure (keyword/recency), matching the discovery path.
	//
	// Deferred, because embedding is the single most expensive thing a search
	// does: it is a blocking network round-trip to the embedding endpoint,
	// ~750ms against a local search that otherwise finishes in ~50ms. Paying it
	// before opening a store meant paying it even when the store held no vectors
	// to compare against — which is exactly what consolidation produced, since
	// the one store carries no chunk_vec table. Fusing against an empty vector
	// set reduces to keyword order, so that round-trip bought nothing and could
	// not change a single result. Each leg now asks its own open store whether it
	// holds vectors and only then resolves this; the result is memoised so a
	// fan-out over many databases still embeds at most once.
	var qvecFn func() []float64
	if embedder != nil && opts.Sort == "" {
		var (
			once sync.Once
			vec  []float64
		)
		qvecFn = func() []float64 {
			once.Do(func() { vec = embedder.Embed(rawQuery) })
			return vec
		}
	}

	var (
		cands      []retrieve.Anchor
		reports    []ScopeReport
		hitCeiling bool
		answered   bool
		storeName  = StoreConsolidated
		storeNote  string
	)

	if scope == nil {
		cands, reports, hitCeiling, storeNote, answered = searchOneStore(rawQuery, fetch, limit, p, qvecFn, opts)
	}

	if !answered {
		storeName = StorePerProject
		if scope == nil {
			scope = fallbackScope(opts)
		}
		if len(scope) == 0 {
			return SearchEnvelope{
				Results: []SearchRef{}, Scopes: []ScopeReport{}, Complete: true,
				Store: storeName, StoreNote: storeNote,
			}
		}
		// Apply the same scope filtering the human path does: a path predicate drops
		// whole projects whose working dir doesn't match include / does match exclude,
		// BEFORE indexing them. Role / date bounds / min-messages push into the SQL
		// WHERE via SearchParams. None of these flag VALUES reach the FTS5 query (#1).
		if opts.IncludePath != "" || opts.ExcludePath != "" {
			scope = scopes.FilterByPath(scope, opts.IncludePath, opts.ExcludePath)
			if len(scope) == 0 {
				return SearchEnvelope{
					Results: []SearchRef{}, Scopes: []ScopeReport{}, Complete: true,
					Store: storeName, StoreNote: storeNote,
				}
			}
		}
		cands, reports, hitCeiling = collectCandidates(scope, rawQuery, fetch, p, qvecFn)
	}

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

	// Newest match in the whole candidate set, for the recency-skew warning below.
	newestISO := ""
	for _, r := range all {
		if r.ISO > newestISO {
			newestISO = r.ISO
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
		Warnings: buildWarnings(warningInputs{
			results:     results,
			reports:     reports,
			sort:        opts.Sort,
			newestISO:   newestISO,
			total:       total,
			hitCeiling:  hitCeiling,
			droppedTurn: droppedTurn,
			storeNote:   storeNote,
		}),
		ExcludedCurrentTurn: droppedTurn,
		Store:               storeName,
		StoreNote:           storeNote,
	}
}

// warningInputs is the measured state buildWarnings decides from. Passing one
// struct keeps the decision in a single pure function that a test can drive
// directly, rather than spreading conditions through the envelope literal.
type warningInputs struct {
	results     []SearchRef
	reports     []ScopeReport
	sort        string
	newestISO   string
	total       int
	hitCeiling  bool
	droppedTurn int
	// storeNote is set only when the one store did not answer and the per-project
	// fan-out did, and it names the reason. Empty on the normal path.
	storeNote string
}

// broadQueryMatches is the distinct-match count at which a query is called
// broad. At this width the hit that matters is buried under incidental
// mentions, because relevance ranks by token match and not by importance.
const broadQueryMatches = 20

// recencySkewGap is how far the freshest match must lead the top-ranked hit
// before relevance ordering is worth flagging. A day is the point where "newer"
// stops being noise inside one working session and starts being a different
// piece of history.
const recencySkewGap = 24 * time.Hour

// buildWarnings turns measured facts into the envelope's advisories. Ordering is
// fixed and meaningful: what changes what the agent should DO next comes first,
// what qualifies the result set comes second, and the standing corpus caveat
// comes last. Nothing here is unconditional except that caveat — a narrow query
// with clean hits carries no advisories at all, which is the point.
func buildWarnings(in warningInputs) []Warning {
	var out []Warning

	// Recency skew: in the default relevance order, a much newer match can sit
	// below an older one. Say so and name the one flag that reorders, rather than
	// letting a "what just happened" result stay buried.
	if in.sort == "" && len(in.results) > 0 && in.newestISO != "" {
		tn, err := time.Parse(time.RFC3339, in.newestISO)
		tt, err2 := time.Parse(time.RFC3339, in.results[0].ISO)
		if err == nil && err2 == nil && tn.Sub(tt) > recencySkewGap {
			out = append(out, Warning{
				Code:  WarnRecencySkew,
				Facts: map[string]any{"newest": in.newestISO[:10]},
				Message: fmt.Sprintf("relevance-ranked; newest match is %s — add --sort newest for latest-first",
					in.newestISO[:10]),
			})
		}
	}

	// Broad query: grounded in the github-search atlas's breadth recipe — narrow
	// with SCOPE FILTERS first (path/project/date are the workhorses), then keep
	// to a few distinctive literal terms, and judge by opening a ref rather than
	// by the snippet, which hides which hit is the important one. Fires on a real
	// boundary (the fetch ceiling) or on many distinct matches.
	if in.hitCeiling || in.total >= broadQueryMatches {
		out = append(out, Warning{
			Code: WarnBroadQuery,
			Facts: map[string]any{
				"matches":                in.total,
				"matches_is_lower_bound": in.hitCeiling,
			},
			Message: "broad query — scope it first: --include-path <re> / --this-project / --since <date>; " +
				"then keep to a few distinctive terms (a filename, flag, error, or \"quoted phrase\") — " +
				"3 or fewer. Open a ref to judge; the snippet hides which hit is the important one.",
		})
	}

	// Current-turn exclusion: reported rather than dropped quietly, so an agent
	// that knows a record was withheld can ask for it instead of seeing a hole.
	if in.droppedTurn > 0 {
		out = append(out, Warning{
			Code:    WarnCurrentTurnExcluded,
			Facts:   map[string]any{"excluded": in.droppedTurn},
			Message: currentTurnLine(in.droppedTurn),
		})
	}

	// Incompleteness stays unconditional: whenever a scope was skipped or served
	// stale, the result MUST NOT read as complete. This is the one warning whose
	// absence would be a correctness bug rather than a missing hint.
	errored, stale, unfolded := 0, 0, 0
	for _, s := range in.reports {
		switch s.Status {
		case ScopeSkippedError:
			errored++
		case ScopeStaleFallback:
			stale++
		case ScopeNotConsolidated:
			unfolded++
		}
	}
	if skipped := errored + stale; skipped > 0 {
		out = append(out, Warning{
			Code: WarnScopeIncomplete,
			Facts: map[string]any{
				"scopes":     len(in.reports),
				"incomplete": skipped,
				"errored":    errored,
				"stale":      stale,
			},
			Message: fmt.Sprintf("%d of %d projects incomplete (%d error, %d stale) — results may be incomplete",
				skipped, len(in.reports), errored, stale),
		})
	}

	// Never folded in: a project database that exists on disk but has never been
	// consolidated was not searched at all. Counted in its own words rather than
	// folded into "N of M projects incomplete", because the gap is a different
	// kind — not a failed read, but history that was never offered to the reader —
	// and it has one known fix, which is why the command travels with it.
	if unfolded > 0 {
		out = append(out, Warning{
			Code:  WarnNotConsolidated,
			Facts: map[string]any{"databases": unfolded},
			Message: fmt.Sprintf("%d project database(s) are not in the one store and were NOT searched — run `rawclaw consolidate`",
				unfolded),
		})
	}

	// Which reader answered: the one store and the fan-out rank differently, so a
	// fallback has to be announced. Only the fallback is worth a warning — the one
	// store answering is the normal path and says nothing.
	if in.storeNote != "" {
		out = append(out, Warning{
			Code:    WarnStoreFallback,
			Facts:   map[string]any{"store": StorePerProject},
			Message: in.storeNote,
		})
	}

	// Project spread: a factual signal, not a heuristic — when the hits cross
	// project boundaries the set is wider than one context, and the narrowing
	// flags are worth naming.
	if projects, sample := projectSpread(in.results); projects >= 2 {
		out = append(out, Warning{
			Code:  WarnProjectSpread,
			Facts: map[string]any{"projects": projects, "sample": sample},
			Message: fmt.Sprintf("matches span %d projects: %s — narrow with --this-project or read a specific ref.",
				projects, strings.Join(sample, ", ")),
		})
	}

	// The standing caveat, last, and only when there is history to caveat: with
	// no results there is nothing to verify against current state.
	if len(in.results) > 0 {
		out = append(out, Warning{Code: WarnRawHistory, Message: freshnessNote})
	}
	return out
}

// searchOneStore answers a search from the consolidated store: one database, one
// query, and the scope filters expressed as WHERE clauses on the project and
// source columns rather than as a choice of which files to open.
//
// The include-path regex is resolved HERE, in Go, against the working dirs the
// store already knows (store.DistinctScopes) — the matching labels then travel
// as an exact-match IN list. The pattern itself never reaches SQL: SQLite has no
// Go-compatible regex, and pushing it down would quietly change what the flag
// means.
//
// It returns ok=false — never a confident empty answer — when the store cannot
// serve the request: absent, empty, unreadable, or asked for a project it has
// never heard of. Each of those hands back a note naming the reason, because the
// fan-out that follows ranks differently and the caller has to know which reader
// produced the list.
func searchOneStore(
	rawQuery string,
	fetch, limit int,
	p retrieve.SearchParams,
	qvecFn func() []float64,
	opts SearchOpts,
) (cands []retrieve.Anchor, reports []ScopeReport, hitCeiling bool, note string, ok bool) {
	con, sessions, err := index.OpenConsolidated()
	if err != nil {
		return nil, nil, false, "one store unavailable (" + err.Error() + ") — searched per project instead", false
	}
	defer con.Close()

	projects, narrowed, err := resolveStoreProjects(con, opts.Project, opts.IncludePath, opts.ExcludePath)
	if err != nil {
		return nil, nil, false, "one store scope lookup failed (" + err.Error() + ") — searched per project instead", false
	}
	if narrowed && len(projects) == 0 {
		// The store holds no project matching this narrowing. That is not the same
		// as "no matches": the project's own database may exist and simply not be
		// folded in yet, so answering empty here would hide a corpus that is on
		// disk. The fan-out can still open it directly.
		return nil, nil, false, "one store knows no project matching this scope — searched per project instead", false
	}
	p.Projects = projects
	p.SourceTool = opts.Source

	rows, exhausted := storeAnchors(con, rawQuery, fetch, limit, p)
	// Unless the widening PROVED there was nothing more to find, the totals
	// derived from these rows are a floor: the window stopped where it had enough
	// conversations, not where the matches ran out. Measured pre-fusion, as the
	// fan-out measures its own ceiling.
	hitCeiling = !exhausted
	// Ask the open store before paying for an embedding: no chunk_vec rows means
	// the fusion below can only reproduce keyword order.
	if qvecFn != nil && store.HasVectors(con) {
		rows = semantic.Fuse(con, rows, qvecFn(), fetch, p.IncludeSubagents)
	}
	dbp := index.ConsolidatedPath()
	for i := range rows {
		rows[i].Root = retrieve.LineageRoot(con, rows[i].SessionID)
		// The project label comes off the ROW, not off a scope: in one store the
		// project is a column, and one query returns rows from many projects.
		rows[i].DBP = dbp
		rows[i].Rank = i
		cands = append(cands, rows[i])
	}

	// The completeness report in its one-store shape: a single row for the store
	// that answered, plus one row per project database that has never been folded
	// into it. Those projects are genuinely outside this answer, and naming them
	// is what stops the corpus from shrinking silently.
	detail := fmt.Sprintf("%d sessions", sessions)
	if narrowed {
		detail += fmt.Sprintf(" · narrowed to %d project(s)", len(projects))
	}
	status := ScopeSearched
	if len(rows) == 0 {
		status = ScopeEmpty
	}
	reports = []ScopeReport{{Dir: dbp, Status: status, Detail: detail}}
	if missing, err := index.UnconsolidatedDBs(con); err == nil {
		for _, m := range missing {
			reports = append(reports, ScopeReport{
				Dir:    m,
				Status: ScopeNotConsolidated,
				Detail: "never folded into the one store — run `rawclaw consolidate`",
			})
		}
	}
	return cands, reports, hitCeiling, "", true
}

// maxStoreWindow caps how far storeAnchors will widen its candidate window. It
// is a bound on work, not on recall: the widening stops on its own as soon as a
// wider window stops finding anything new, and on this corpus a query exhausts
// its matches long before the cap. The cap exists so a term matching a large
// fraction of every message can never turn one search into an unbounded read.
const maxStoreWindow = 20000

// storeAnchors runs the anchor query against the one store, widening its
// candidate window until the window holds enough DISTINCT conversations to fill
// the caller's limit. It returns the anchors and the window that produced them.
//
// The widening exists because one query over the whole corpus spends its
// candidate window very differently from one query per project. The fan-out
// funded EVERY project with the full window, so a search for 8 conversations had
// as many windows as there were projects to find them in. A single global window
// of that size collapses to a handful of conversations: the top anchors are
// often several messages of the SAME conversation, and more are dropped later
// where the match survives only inside stripped tool output. Without the
// widening, moving to one store would hand back a visibly thinner answer than
// the fan-out for the same query — the one outcome this work must not produce.
//
// Distinct SESSIONS is the stopping measure, not distinct results: results
// collapse further, by lineage root, and computing a root costs a query per row.
// Sessions are already on the row, so the loop stays cheap; the two differ only
// where a conversation was resumed.
//
// The second return value says whether the corpus was EXHAUSTED — proved, by a
// wider window that found nothing new, rather than assumed. Anything else leaves
// the totals a floor, because a window that stopped as soon as it had enough
// conversations says nothing about how many more there were.
func storeAnchors(con *sql.DB, rawQuery string, fetch, limit int, p retrieve.SearchParams) ([]retrieve.Anchor, bool) {
	window := fetch
	rows := retrieve.MatchAnchors(con, rawQuery, window, p)
	for distinctSessions(rows) < limit && window < maxStoreWindow {
		window *= 4
		if window > maxStoreWindow {
			window = maxStoreWindow
		}
		wider := retrieve.MatchAnchors(con, rawQuery, window, p)
		if len(wider) <= len(rows) {
			// A wider window found nothing new — every match this query has is
			// already in hand, so the totals derived from it are exact.
			return rows, true
		}
		rows = wider
	}
	return rows, false
}

// distinctSessions counts the conversations an anchor list covers.
func distinctSessions(rows []retrieve.Anchor) int {
	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[r.SessionID] = struct{}{}
	}
	return len(seen)
}

// fallbackScope builds the per-project scope list for a nil-scope search the one
// store could not answer. The caller supplies it as a function because
// enumerating every project costs seconds of directory walking and git probing,
// and a search the store answers must not pay that.
//
// A Project narrowing is re-applied here: on the fan-out, scope IS the filter, so
// a caller that asked for one project must not be widened to all of them just
// because the store was unavailable.
func fallbackScope(opts SearchOpts) []view.Scope {
	var sc []view.Scope
	if opts.ScopeFallback != nil {
		sc = opts.ScopeFallback()
	} else {
		sc = allScope()
	}
	if opts.Project == "" {
		return sc
	}
	out := make([]view.Scope, 0, 1)
	for _, s := range sc {
		if s.Project == opts.Project {
			out = append(out, s)
		}
	}
	return out
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
// scope nil = every project, enumerated through more only if the one store
// cannot answer the ref (see ScopeFn).
func ReadAndRender(
	w io.Writer,
	ref string,
	scope []view.Scope,
	more ScopeFn,
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
		Focus:         focus,
		Budget:        budget,
		IncludeTools:  includeTools,
		Window:        window,
		Around:        around,
		ScopeFallback: more,
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
func OutlineAndRender(w io.Writer, session8 string, scope []view.Scope, more ScopeFn, includeTools, wantJSON bool) error {
	result, err := outline(session8, scope, more, includeTools)
	if err != nil {
		return err
	}
	if wantJSON {
		return emit(w, result)
	}
	renderOutline(w, result)
	return nil
}

// scopesComplete reports whether every scope was searched fresh: none skipped,
// none served from a stale fallback, and — on the one-store path — no project
// database left outside the store. A project that was never folded in is missing
// from the answer just as surely as one whose index failed to open.
func scopesComplete(reports []ScopeReport) bool {
	for _, r := range reports {
		if r.Status == ScopeSkippedError || r.Status == ScopeStaleFallback || r.Status == ScopeNotConsolidated {
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
	qvecFn func() []float64,
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
		// RRF-fuse keyword anchors with vector-KNN when this database actually
		// holds vectors (relevance mode only — qvecFn is nil under --sort).
		// Parity with Discovery. The embedding resolves at most once across the
		// whole fan-out, and not at all if no database here has vectors.
		if qvecFn != nil && store.HasVectors(con) {
			rows = semantic.Fuse(con, rows, qvecFn(), fetch, p.IncludeSubagents)
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
// A tag is the better label when it exists, but it only exists if someone tagged
// the session at the end. Measured on a live corpus of 712 non-subagent sessions,
// 241 were tagged — and the coverage is strongly size-skewed, because tagging
// effort goes to the heavy sessions: 165 of 223 sessions over 200 messages carry
// a tag, against 5 of 181 under ten. So the hit lines left blank are exactly the
// small and mid-sized sessions, which are the ones hardest to recognise from an
// id and a timestamp. Tagging is a bonus, not the thing finding a conversation
// depends on, so an untagged session falls back to the ask that opened it
// (view.SessionPreview): the user's own words, already in the database, no model
// call, always present.
//
// Lookups are grouped by database so a result set spanning several projects
// opens each project's database once. Any failure is silent: a missing topic
// table or an unreadable database leaves the label empty, which renders exactly
// as it did before topics existed.
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
			topic := store.TopicForMessage(con, anchors[i].SessionID, anchors[i].UUID)
			if topic == "" {
				topic = view.SessionPreview(con, anchors[i].SessionID, searchTitleCap)
			}
			refs[i].Topic = topic
		}
		_ = con.Close()
	}
}

// searchTitleCap is the header-line budget for the title. Shorter than a browse
// preview because the search header already spends width on a stamp, a session
// id and a project name.
const searchTitleCap = 70

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

	// ScopeFallback supplies the project list for a nil scope, called ONLY if
	// the one store cannot answer the id (see ScopeFn). Nil = every project
	// under a background context.
	ScopeFallback ScopeFn
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
	dbp, fullSID, proj, locErr := locateSession(scope, opts.ScopeFallback, session8)
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

// locateSession resolves a session8 prefix to the ONE database that answers for
// it, plus the full session id and project label. Shared by Read, Outline and
// the tag verbs.
//
// The primary path is a row lookup in the consolidated store. Project is a
// column there, so scope narrowing is a WHERE clause instead of a choice of
// which files to open, and a session that ran in two working directories is a
// SINGLE row — consolidation already merged the halves, so there is nothing to
// reconcile here and no ranking to guess at.
//
// The per-project sweep is the fallback, for the two cases the one store cannot
// answer: it is missing or empty (nothing has consolidated on this machine
// yet), or it simply has no row for this prefix (it is a derived artifact and
// may lag the per-project indexes, and replicas of other machines are not
// folded into it at all). A miss there is therefore never reported as "session
// not found" while an index still holds the session.
//
// A nil scope means "every project" and is resolved through more (see ScopeFn)
// only if the sweep is reached, so the ordinary lookup — the one the store
// answers — never pays for enumerating the per-project indexes.
//
// Returns an *ErrAmbiguousSession on ≥2 DISTINCT matching ids, an
// *ErrSessionNotFound on none. A failing project is skipped.
func locateSession(scope []view.Scope, more ScopeFn, session8 string) (dbp, fullSID, proj string, err error) {
	everywhere := scope == nil
	// A scope built and found EMPTY means "this directory has no history", not
	// "search everything": callers build it deliberately, so it must resolve
	// nothing. A nil scope is the opposite — nobody narrowed anything.
	if !everywhere && len(scope) == 0 {
		return "", "", "", &ErrSessionNotFound{Prefix: session8}
	}
	// scope nil here narrows the store by nothing, which is what "every
	// project" means as a WHERE clause.
	if cands := oneStoreCands(scope, session8); len(cands) > 0 {
		return decideSession(cands, session8)
	}
	if everywhere {
		scope = resolveScope(more)
	}
	return decideSession(sweepScopes(scope, session8), session8)
}

// decideSession turns candidate rows into the verb's answer: exactly one row
// resolves, none is a not-found, and two DIFFERENT ids is the git-style
// ambiguity only a longer prefix can break.
func decideSession(cands []sessionCand, session8 string) (dbp, fullSID, proj string, err error) {
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

// oneStoreCands looks the prefix up in the consolidated store. An empty result
// means "ask the per-project indexes instead" — either because the store could
// not answer at all (said plainly, below) or because it holds no matching row.
func oneStoreCands(scope []view.Scope, session8 string) []sessionCand {
	dbp := index.ConsolidatedPath()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		slog.Warn("the consolidated store cannot be opened, so this lookup falls back to the per-project indexes; run `rawclaw consolidate` to build it",
			"path", dbp, "err", err)
		return nil
	}
	defer con.Close()

	// One probe answers both "is it there" and "does it hold anything": the file
	// is opened read-only, so a missing store fails the query rather than being
	// created empty.
	var filled int
	switch err := con.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions)").Scan(&filled); {
	case err != nil:
		slog.Warn("the consolidated store is missing or unreadable, so this lookup falls back to the per-project indexes; run `rawclaw consolidate` to build it",
			"path", dbp, "err", err)
		return nil
	case filled == 0:
		slog.Warn("the consolidated store is empty, so this lookup falls back to the per-project indexes; run `rawclaw consolidate` to fill it",
			"path", dbp)
		return nil
	}

	// Fetch up to 2: enough to DETECT a collision without fetching the world.
	// Sub-sessions are excluded first for the reason spelled out in sweepScopes.
	projects := scopeProjects(scope)
	rows, err := store.SessionRowsByPrefix(con, session8, false, projects, 2)
	if err != nil {
		slog.Warn("the consolidated store could not be queried, so this lookup falls back to the per-project indexes", "path", dbp, "err", err)
		return nil
	}
	if len(rows) == 0 {
		rows, err = store.SessionRowsByPrefix(con, session8, true, projects, 2)
		if err != nil {
			return nil
		}
	}
	cands := make([]sessionCand, 0, len(rows))
	for _, r := range rows {
		cands = append(cands, sessionCand{SessionID: r.ID, Project: r.Project, dbp: dbp})
	}
	return cands
}

// scopeProjects lists the project labels a scope narrows to — the one store's
// equivalent of "which project databases would the sweep have opened". A scope
// carrying no label cannot be expressed as a label filter, so one such scope
// drops the filter for the whole lookup: over-matching surfaces as an
// ambiguity the caller can see and break, whereas under-matching would hide a
// session that is reachable today.
func scopeProjects(scope []view.Scope) []string {
	out := make([]string, 0, len(scope))
	seen := make(map[string]bool, len(scope))
	for _, sc := range scope {
		if sc.Project == "" {
			return nil
		}
		if !seen[sc.Project] {
			seen[sc.Project] = true
			out = append(out, sc.Project)
		}
	}
	return out
}

// sweepScopes probes each scope's OWN database for the prefix — the lookup as
// it worked before the one store, kept as the fallback path.
func sweepScopes(scope []view.Scope, session8 string) []sessionCand {
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
				cs = append(cs, sessionCand{SessionID: sid, Project: sc.Project, dbp: dbpC})
			}
		}
		return cs
	}

	cands := collect(true)
	if len(cands) == 0 {
		cands = collect(false)
	}
	return firstRowPerSession(cands)
}

// firstRowPerSession keeps one row per full session id. A session continued in
// a second directory has a row in EACH project's database, and those rows are
// one conversation, so reporting them as an ambiguity was unhelpable advice:
// the ids are byte-identical and no longer prefix can separate them. Which copy
// answers is settled by scope order, which already lists live project
// directories before orphaned ones and local scopes before replicas of other
// machines. This runs only on the fallback path, and it is self-correcting: the
// sweep above resolved every scope, which writes each one through to the
// consolidated store, so the next lookup gets the merged row instead of a half.
func firstRowPerSession(cands []sessionCand) []sessionCand {
	if len(cands) < 2 {
		return cands
	}
	seen := make(map[string]bool, len(cands))
	out := make([]sessionCand, 0, len(cands))
	for _, c := range cands {
		if seen[c.SessionID] {
			continue
		}
		seen[c.SessionID] = true
		out = append(out, c)
	}
	return out
}

// LocateSession resolves a session8 prefix to its (db path, full session id)
// across scope (nil = all projects, enumerated through more only if the sweep
// is reached — see ScopeFn), the exported door onto the private
// locateSession. The `tag` verb uses it to find the db it must open read-write
// and the full session id whose messages it tags. Returns *ErrSessionNotFound /
// *ErrAmbiguousSession unchanged, so callers can render the same hints as Read
// and Outline.
func LocateSession(session8 string, scope []view.Scope, more ScopeFn) (dbPath, fullSID string, err error) {
	dbp, sid, _, err := locateSession(scope, more, normalizeSessionArg(session8))
	return dbp, sid, err
}

// ── verb: outline ────────────────────────────────────────────────────────────

// Outline returns a session's bookend arc (first/last N user+assistant messages).
// Returns an error if the session is not found. A pasted read-ref token
// ("ref=<session8>:<uuid8>" or "<session8>:<uuid8>") resolves via its session half.
func Outline(session8 string, scope []view.Scope, includeTools bool) (*OutlineResult, error) {
	return outline(session8, scope, nil, includeTools)
}

// outline is Outline with the nil-scope enumeration deferred to more (ScopeFn),
// which is what the CLI passes so an id the one store answers never enumerates
// the per-project indexes.
func outline(session8 string, scope []view.Scope, more ScopeFn, includeTools bool) (*OutlineResult, error) {
	session8 = normalizeSessionArg(session8)

	dbp, fullSID, proj, locErr := locateSession(scope, more, session8)
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

	// Trim the over-fetched rows down to what a reader should see BEFORE the
	// overlap dedup below: deduping first would compare forty raw rows against
	// forty raw rows and swallow the whole tail of a short session.
	startRows = view.FilterDisplayable(startRows, OutlineBookend, includeTools)
	endRows = view.FilterDisplayable(endRows, OutlineBookend, includeTools) // DESC: nearest the end first

	startIDs := map[int]struct{}{}
	for _, r := range startRows {
		startIDs[r.ID] = struct{}{}
	}

	// endRows came back DESC; reverse to chronological, then drop any already
	// present in the start bookend so the two ends don't overlap.
	endMsgs := []store.Msg{}
	for _, r := range view.Reversed(endRows) {
		if _, dup := startIDs[r.ID]; dup {
			continue
		}
		endMsgs = append(endMsgs, r)
	}

	startOut := view.RenderMsgs(startRows, includeTools, outlineDispCap)
	endOut := view.RenderMsgs(endMsgs, includeTools, outlineDispCap)

	lastStartID := 0
	if len(startRows) > 0 {
		lastStartID = startRows[len(startRows)-1].ID
	}
	firstEndID := 0 // 0 = no upper bound: count everything after the opening bookend
	if len(endMsgs) > 0 {
		firstEndID = endMsgs[0].ID
	}
	// Count the gap in SQL rather than by subtracting row ids. The two are the
	// same only while a session's rows are contiguous, which they are in a
	// per-project index and are NOT in the one store: ids there run across every
	// project, and a session continued in a second directory is folded in two
	// pieces with other sessions' rows between them.
	midCount, cErr := store.CountMessagesBetween(con, fullSID, lastStartID, firstEndID)
	if cErr != nil {
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
	// Over-fetch: the rows nearest either end of a session are the likeliest in
	// the corpus to be injected handbooks, command echoes and bare [THINKING]
	// markers, and those get dropped before rendering (view.IsDisplayable).
	// Reading only OutlineBookend rows would return a bookend of leftovers.
	return store.BookendMessages(con, fullSID, 0, false, asc, outlineBookendScan)
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
			fmt.Fprintf(w, "No matches outside the turn you are in now — %d record(s) of it matched and were withheld; the rest of that session was searched.\n", env.ExcludedCurrentTurn)
			fmt.Fprintln(w, "To see them anyway, re-run with --current-session off.")
			renderWarnings(w, env.Warnings, WarnCurrentTurnExcluded)
			return
		}
		fmt.Fprintln(w, "No matches · live-indexed on invoke. Lead with a single distinctive term that appears in the text (a filename, flag, or error string), not a topic word — or rephrase.")
		renderWarnings(w, env.Warnings)
		return
	}
	fmt.Fprintf(w, "%d conversation(s) matching '%s' %s · live-indexed on invoke:\n\n", len(env.Results), query, scopeLabel)
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
		// The title answers "what was this session about?" on the header line, so
		// an agent can choose a hit without opening it. It is the tag when the
		// session has one and the opening ask otherwise (attachTopics); quoted
		// either way, to mark it as prose rather than another identifier.
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
	// Every footer line comes from the warnings the envelope already carries, in
	// the order it carries them. The renderer holds no conditions of its own —
	// that is what keeps the text and --json surfaces from drifting.
	renderWarnings(w, env.Warnings)
}

// renderWarnings prints one "note:" line per warning, skipping any code in
// suppress. Suppression exists for the empty-result path, where a warning has
// already been stated in full as the primary message and repeating it as a
// footnote would read as two separate findings.
func renderWarnings(w io.Writer, ws []Warning, suppress ...string) {
	skip := make(map[string]struct{}, len(suppress))
	for _, c := range suppress {
		skip[c] = struct{}{}
	}
	for _, warn := range ws {
		if _, dup := skip[warn.Code]; dup {
			continue
		}
		fmt.Fprintf(w, "note: %s\n", warn.Message)
	}
}

// currentTurnLine states what the current-turn exclusion withheld. It names the
// turn, not the session, because the session's earlier history was searched —
// an agent must not read this as "my own session was skipped".
func currentTurnLine(n int) string {
	return fmt.Sprintf("excluded %d record(s) of the turn you are in now; the rest of that session was searched", n)
}

// freshnessNote is the standing reminder that search/read results are raw session
// history, not current truth. Search carries it as the WarnRawHistory warning;
// Read prints it directly. It holds no "note: " prefix of its own — the prefix
// belongs to whichever renderer emits it, so the string can be reused as a
// warning Message without doubling up.
const freshnessNote = "raw session history — verify against current state before acting."

// projectSpread returns how many distinct projects the results cover and a
// sample of up to five of their names, in first-seen order. 0 or 1 project means
// there is no spread worth reporting.
func projectSpread(results []SearchRef) (int, []string) {
	seen := map[string]struct{}{}
	var distinct []string
	for _, r := range results {
		if _, dup := seen[r.Project]; dup {
			continue
		}
		seen[r.Project] = struct{}{}
		distinct = append(distinct, r.Project)
	}
	sample := distinct
	if len(sample) > 5 {
		sample = sample[:5]
	}
	return len(distinct), sample
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
	fmt.Fprintf(w, "note: %s\n", freshnessNote)
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

// TopicsOpts groups the topic-finder's scope narrowing. Both narrowing fields
// resolve to project labels against the one store: Project is already a label,
// IncludePath is a pattern matched in Go against the (project, working
// directory) pairs the store knows about.
type TopicsOpts struct {
	Limit       int
	Project     string // "" = every project; else the one project to search
	IncludePath string // "" = no filter; else a regex over the project working dir

	// ScopeFallback supplies the project list for a nil scope, called ONLY if
	// the one store cannot answer (see ScopeFn). Nil = every project under a
	// background context.
	ScopeFallback ScopeFn
}

// Topics searches ONLY the topic layer, returning hits ordered by FTS rank and
// capped at Limit. Each hit resolves the segment's START message id (what
// MatchTopics returns) to its uuid for a read-ref. It never touches the
// keyword/vector ranking — this is the separate, on-demand finder. The
// empty-state note distinguishes "no match" from "nothing tagged yet".
//
// Ordering is GLOBAL, across every project at once, because the whole corpus is
// one FTS index: bm25 folds in corpus statistics, and those are only comparable
// when every candidate was scored against the same corpus. Limit is therefore a
// cap on the combined list, not a per-project quota — the old per-project cap
// existed only because a merge across independent databases could not be
// ordered, and it let a weak hit from a small project sit alongside a strong one
// from a large project as though they ranked equally.
//
// scope is the fallback path's project enumeration, used only when the one
// store cannot answer.
func Topics(query string, scope []view.Scope, opts TopicsOpts) (TopicsResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	if con, _, err := index.OpenConsolidated(); err == nil {
		defer con.Close()
		if res, ok := topicsFromStore(con, query, limit, opts); ok {
			return res, nil
		}
	}
	return topicsByFanOut(query, scope, limit, opts)
}

// topicsFromStore is the one-store path: one database, one query, one globally
// ranked list. ok=false means the store could not answer this request — the
// narrowing named projects it has never heard of — and the caller falls back to
// the per-project databases rather than report an empty result the store did
// not earn.
func topicsFromStore(con *sql.DB, query string, limit int, opts TopicsOpts) (TopicsResult, bool) {
	// A store carrying no topic rows at all cannot answer this verb — the topic
	// layer is written into a project db by `tag-write` and folded in after, so
	// an un-folded store would report "nothing tagged" over a corpus that is in
	// fact tagged. Hand those back to the fan-out rather than answer confidently.
	if !store.TopicRowsExist(con) {
		return TopicsResult{}, false
	}
	projects, narrowed, err := resolveStoreProjects(con, opts.Project, opts.IncludePath, "")
	if err != nil || (narrowed && len(projects) == 0) {
		return TopicsResult{}, false
	}

	thits, err := store.MatchTopics(con, query, limit, projects)
	if err != nil {
		return TopicsResult{}, false
	}
	hits := []TopicHit{}
	for _, h := range thits {
		uuid := store.MessageUUID(con, h.MsgID)
		if uuid == "" {
			continue // can't build a read-ref without the message uuid
		}
		hits = append(hits, TopicHit{
			Topic:   h.Topic,
			Project: h.Project,
			ReadRef: fmtRef(h.SessionID, uuid),
		})
	}

	// No empty-state note here: reaching this point means the store DOES carry
	// topic rows, so zero hits is a query that matched nothing, not an untagged
	// corpus.
	return TopicsResult{Query: query, Hits: hits}, true
}

// topicsByFanOut is the pre-consolidation path, kept as the fallback for a
// corpus whose one store is missing or empty. Its ordering is per-project and
// its cap is per-project, because scores from independent databases cannot be
// merged into one ranking — the limitation this fallback inherits and the one
// store removes.
func topicsByFanOut(query string, scope []view.Scope, limit int, opts TopicsOpts) (TopicsResult, error) {
	if scope == nil {
		scope = resolveScope(opts.ScopeFallback)
	}
	if opts.IncludePath != "" {
		scope = scopes.FilterByPath(scope, opts.IncludePath, "")
	}

	hits := []TopicHit{}
	anyTopics := false
	for _, sc := range scope {
		if opts.Project != "" && sc.Project != opts.Project {
			continue
		}
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
		//
		// The projects filter is nil here: this is the fan-out, where one database
		// IS one project, so narrowing by column would be redundant.
		thits, _ := store.MatchTopics(con, query, topicFetch(limit), nil)

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

// resolveStoreProjects turns a request's scope narrowing into the exact project
// labels the one store should be filtered on. narrowed reports whether the
// request asked for a subset at all — the difference between "search
// everything" (no filter, which also keeps rows indexed before the scope
// columns existed) and "search these projects", which an empty result set would
// otherwise be indistinguishable from.
//
// A path pattern is matched HERE, in Go, against the (project, directory) pairs
// the store knows, so the regex keeps Go's semantics rather than SQLite's and
// no pattern ever reaches the SQL.
func resolveStoreProjects(con *sql.DB, project, includePath, excludePath string) (projects []string, narrowed bool, err error) {
	if project == "" && includePath == "" && excludePath == "" {
		return nil, false, nil
	}
	scopeRows, err := store.DistinctScopes(con)
	if err != nil {
		return nil, true, err
	}

	keep := map[string]bool{}
	for _, sr := range scopeRows {
		keep[sr.Project] = keep[sr.Project] // ensure the key exists with a false default
	}
	if includePath != "" || excludePath != "" {
		pred := query.PathPredicate(includePath, excludePath)
		for _, sr := range scopeRows {
			if pred(sr.CWD) {
				keep[sr.Project] = true
			}
		}
	} else {
		for k := range keep {
			keep[k] = true
		}
	}
	if project != "" {
		for k := range keep {
			if k != project {
				keep[k] = false
			}
		}
	}

	for _, sr := range scopeRows {
		if keep[sr.Project] {
			keep[sr.Project] = false // emit each label once
			projects = append(projects, sr.Project)
		}
	}
	sort.Strings(projects)
	return projects, true, nil
}

// TopicsAndRender runs Topics and writes the result to w (JSON when wantJSON, else
// text). The exported entry the top-level `topics` subcommand calls.
func TopicsAndRender(w io.Writer, query string, scope []view.Scope, opts TopicsOpts, wantJSON bool) error {
	result, err := Topics(query, scope, opts)
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
