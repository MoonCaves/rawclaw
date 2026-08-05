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
	Topic     string `json:"topic,omitempty"` // unused in search output (topics are a separate on-demand `topics` command); omitempty keeps JSON identical to pre-topic

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

	// Embed the query once for RRF fusion, only in relevance mode — an explicit
	// --sort stays pure (keyword/recency), matching the discovery path.
	var qvec []float64
	if embedder != nil && opts.Sort == "" {
		qvec = embedder.Embed(rawQuery)
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
		cands, reports, hitCeiling, storeNote, answered = searchOneStore(rawQuery, fetch, p, qvec, opts)
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
			scope = filterScopeByPath(scope, opts.IncludePath, opts.ExcludePath)
			if len(scope) == 0 {
				return SearchEnvelope{
					Results: []SearchRef{}, Scopes: []ScopeReport{}, Complete: true,
					Store: storeName, StoreNote: storeNote,
				}
			}
		}
		cands, reports, hitCeiling = collectCandidates(scope, rawQuery, fetch, p, qvec)
	}

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
			Missing:   r.MissingSince > 0,
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
		Results:           results,
		Scopes:            reports,
		Complete:          scopesComplete(reports) && !hasMore,
		Count:             len(results),
		TotalMatches:      total,
		TotalIsLowerBound: hitCeiling,
		HasMore:           hasMore,
		NextCommand:       nextCmd,
		RecencyHint:       recencyHint,
		NarrowHint:        narrowHint,
		Store:             storeName,
		StoreNote:         storeNote,
	}
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
	fetch int,
	p retrieve.SearchParams,
	qvec []float64,
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

	rows := retrieve.MatchAnchors(con, rawQuery, fetch, p)
	if len(rows) >= fetch {
		// The single query filled its fetch window, so any total derived from these
		// rows is a floor. Measured pre-fusion, like the fan-out does.
		hitCeiling = true
	}
	if qvec != nil {
		rows = semantic.Fuse(con, rows, qvec, fetch, p.IncludeSubagents)
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
		if pred(scopes.CWD(sc)) {
			out = append(out, sc)
		}
	}
	return out
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
		fmt.Fprintln(w, "No matches. Lead with a single distinctive term that appears in the text (a filename, flag, or error string), not a topic word — or rephrase.")
		renderScopeFooter(w, env)
		return
	}
	fmt.Fprintf(w, "%d conversation(s) matching '%s' %s:\n\n", len(env.Results), query, scopeLabel)
	for _, r := range env.Results {
		// timefmt seam: search results are agent-parsed — render the stored ISO
		// as marked UTC (unparseable stamps pass through verbatim).
		iso := timefmt.UTCFromISO(r.ISO)
		if iso == "" {
			iso = "?"
		}
		miss := ""
		if r.Missing {
			miss = " · source file gone — retained history"
		}
		fmt.Fprintf(w, "  ━━ %s · %s · %s%s\n", iso, sid8(r.SessionID), r.Project, miss)
		fmt.Fprintf(w, "     …%s…\n", r.Snippet)
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

// renderScopeFooter prints the incompleteness footer: which reader answered when
// it was not the one store, and what was left out of the answer. The two shapes
// of report both land here — the fan-out reports one row per project, the one
// store reports itself plus the project databases it has never folded in — so
// each kind of gap is counted in its own words rather than as "N of M projects".
func renderScopeFooter(w io.Writer, env SearchEnvelope) {
	if env.StoreNote != "" {
		fmt.Fprintf(w, "note: %s\n", env.StoreNote)
	}
	if env.Complete {
		return
	}
	errored, stale, unfolded := 0, 0, 0
	for _, s := range env.Scopes {
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
		fmt.Fprintf(w, "note: %d of %d projects incomplete (%d error, %d stale) — results may be incomplete\n",
			skipped, len(env.Scopes), errored, stale)
	}
	if unfolded > 0 {
		fmt.Fprintf(w, "note: %d project database(s) are not in the one store and were NOT searched — run `rawclaw consolidate`\n",
			unfolded)
	}
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
		scope = allScope()
	}
	if opts.IncludePath != "" {
		scope = filterScopeByPath(scope, opts.IncludePath, "")
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
		thits, _ := store.MatchTopics(con, query, limit, nil)
		for _, h := range thits {
			uuid := store.MessageUUID(con, h.MsgID)
			if uuid == "" {
				continue // can't build a read-ref without the message uuid
			}
			hits = append(hits, TopicHit{
				Topic:   h.Topic,
				Project: sc.Project,
				ReadRef: fmtRef(h.SessionID, uuid),
			})
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
