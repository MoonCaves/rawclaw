// Package retrieve holds FTS5 keyword recall: search, the linear-scan fallback,
// cross-project the cross-project search, and the anchor helpers (lineage walk + ranked anchor
// messages) that the view layer composes into bookend windows.
package retrieve

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/query"
)

// reBoolOps strips boolean operators (&& || !) down to spaces so the leftover
// terms can drive snippet highlighting for a raw-match query. Go's RE2 has no
// lookbehind, so the lone-'!' arm is handled by a hand-rolled scan below.
var reBoolOps = regexp.MustCompile(`&&|\|\|`)

// reWhitespace collapses runs of whitespace (the include-tools snippet cleanup).
var reWhitespace = regexp.MustCompile(`\s+`)

// stripBoolOps removes &&, ||, and word-boundary '!' (not preceded by a word
// char), each replaced by a single space.
func stripBoolOps(s string) string {
	s = reBoolOps.ReplaceAllString(s, " ")
	// Remove '!' only when not preceded by a word byte (RE2 lookbehind stand-in).
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '!' && (i == 0 || !isWordByte(s[i-1])) {
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func isWordByte(c byte) bool {
	return c == '_' ||
		(c >= '0' && c <= '9') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z')
}

// Hit is one flat search result returned by Search / LinearFallback:
// (iso, sid, role, is_sub, parent, snip).
type Hit struct {
	ISO        string
	SessionID  string
	Role       string
	IsSubagent bool
	Parent     string
	Snippet    string
}

// AllHit is one cross-project cross-project result: a Hit plus the project label and
// the per-project hit count.
type AllHit struct {
	Hit
	Project string
	Hits    int
}

// Anchor is one ranked anchor message from MatchAnchors — a candidate the view
// layer expands into a bookend window. Carries (id, session_id, role, iso,
// parent, snip, cov) plus the fields the view/discovery layers attach downstream.
type Anchor struct {
	ID        int
	SessionID string
	UUID      string // source message uuid — the stable external read-ref handle
	Role      string // empty for a vector-only synthesized anchor
	ISO       string
	Parent    string
	Snip      string
	Cov       int

	// MissingSince is the session's missing_since watermark: >0 when the backing
	// source file is gone but the row is retained (durable retention, D1). Surfaced
	// so a retained-but-missing hit doesn't read as current (D7). 0 = present.
	MissingSince float64

	// Attached by the fusion / discovery layers (zero until set):
	Fused   float64 // RRF score (semantic.Fuse)
	Topic   string  // topic-layer label for this anchor's segment (Fuse / TopicForMessage)
	Root    string  // lineage root session id (LineageRoot)
	Project string  // project label
	DBP     string  // db path this anchor came from
	Rank    int     // original keyword rank (tiebreak)
}

// SearchParams groups the many optional filters shared by Search / MatchAnchors
// (keeps signatures under the 4-param guideline). A zero value = no filters.
type SearchParams struct {
	Role             string // "" = any; else "user"/"assistant"
	Sort             string // "" = relevance; "newest"/"oldest"
	IncludeTools     bool
	IncludeSubagents bool
	Since            string // "" = no bound; else YYYY-MM-DD inclusive
	Before           string // "" = no bound; else YYYY-MM-DD inclusive
	RawMatch         string // "" = plain path; else explicit FTS5 expr (boolean query)
	MinMessages      int    // 0 = no minimum
}

// dateWhere pushes inclusive YYYY-MM-DD bounds into the SQL WHERE so they filter
// BEFORE the LIMIT/rank (a post-filter would pick the top-N first, then silently
// under-return).
func dateWhere(where *[]string, params *[]any, since, before string) {
	if since != "" {
		*where = append(*where, "substr(m.ts_iso,1,10) >= ?")
		*params = append(*params, since)
	}
	if before != "" {
		*where = append(*where, "substr(m.ts_iso,1,10) <= ?")
		*params = append(*params, before)
	}
}

// buildMatch resolves the FTS5 MATCH expression and the highlight terms shared
// by Search and MatchAnchors. ok=false means "no searchable token" — the caller
// returns an empty result.
func buildMatch(q string, p SearchParams) (match string, terms []string, multi, ok bool) {
	if p.RawMatch != "" {
		// Explicit boolean query: use the pre-built FTS5 expr verbatim — no
		// OR-rewrite, no coverage re-rank (the operators ARE the intent). terms
		// are only for snippet highlighting.
		return p.RawMatch, query.ParseTerms(stripBoolOps(q)), false, true
	}
	clean := query.StripStopwords(query.SanitizeFTS5Query(q))
	if clean == "" || !query.HasSearchableToken(clean) {
		return "", nil, false, false
	}
	terms = query.ParseTerms(clean)
	multi = len(terms) > 1
	if !multi {
		return clean, terms, false, true
	}
	// Multi-word queries OR their tokens (grep-style alternation), instead of
	// FTS5 implicit-AND. Coverage re-rank keeps docs matching MORE terms on top.
	quoted := make([]string, 0, len(terms))
	for _, t := range terms {
		quoted = append(quoted, `"`+strings.ReplaceAll(t, `"`, "")+`"`)
	}
	return strings.Join(quoted, " OR "), terms, true, true
}

// orderClause selects the SQL ORDER BY: rank, m.id (relevance) unless an
// explicit sort is requested.
func orderClause(sortMode string) string {
	switch sortMode {
	case "newest":
		return "ORDER BY m.ts DESC, m.id DESC"
	case "oldest":
		return "ORDER BY m.ts ASC, m.id ASC"
	default:
		return "ORDER BY rank, m.id"
	}
}

// lowerSet returns the distinct, non-empty lowercased terms used for coverage
// counting.
func lowerSet(terms []string) []string {
	seen := make(map[string]struct{}, len(terms))
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		if t == "" {
			continue
		}
		l := strings.ToLower(t)
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
}

// coverage counts how many distinct query terms appear in the (already
// lowercased) haystack; for single-term queries the count is always 1.
func coverage(lterms []string, hayLower string, multi bool) int {
	if !multi {
		return 1
	}
	cov := 0
	for _, t := range lterms {
		if strings.Contains(hayLower, t) {
			cov++
		}
	}
	return cov
}

// scoredHit is an intermediate Hit carrying its coverage for the re-rank.
type scoredHit struct {
	Hit
	cov int
}

// Ranking-regime labels reported by ScoreExplain.Method — the honest name of
// the rule that decided a hit's position. These mirror the three branches in
// Search/MatchAnchors exactly (no invented blend):
//   - bm25 only: single-term relevance — pure FTS5 bm25 order (rank, m.id).
//   - bm25 + coverage re-rank: multi-term relevance — stable re-sort by the
//     count of distinct query terms matched; bm25 is the tiebreak.
//   - sort overlay (newest/oldest): a recency sort REPLACES relevance entirely;
//     bm25 and coverage do not influence position.
const (
	MethodBM25         = "bm25 only"
	MethodBM25Coverage = "bm25 + coverage re-rank"
	MethodSortOverlay  = "sort overlay (recency)"
)

// ScoreExplain is the LLM-free, honest breakdown of WHY one hit landed at its
// rank. It carries only what the real ranking actually uses — it does NOT
// fabricate a composite scalar score, because RawClaw has none.
//
// Honesty notes (read these before trusting a field):
//   - BM25 is NOT selected as a scalar by the live query (the SQL orders by the
//     opaque FTS5 `rank` and never reads its value). So BM25Rank is the hit's
//     ORDINAL position in bm25 order, not the bm25 number. -1 means "bm25 did
//     not order this result" (a recency sort overlay was in force).
//   - Coverage is the REAL integer the re-rank uses: distinct query terms found
//     in the hit's coverage haystack. Always 1 for a single-term query.
//   - Recency is a BOOL-as-float flag, not a weight: 1 when a sort overlay set
//     the order, 0 otherwise. There is no recency term blended into relevance.
//   - Final is the hit's 0-based ordinal in the returned slice (its actual
//     position), NOT a computed score. Method names the rule that produced it.
type ScoreExplain struct {
	BM25Rank int      `json:"bm25_rank"` // ordinal in bm25 order; -1 when a sort overlay ordered the hit
	Coverage int      `json:"coverage"`  // distinct query terms matched (the real re-rank key)
	Recency  float64  `json:"recency"`   // 1 = a recency sort overlay set the order, else 0
	Final    int      `json:"final"`     // 0-based ordinal position in the returned results
	Method   string   `json:"method"`    // ranking regime: one of the Method* constants
	Terms    []string `json:"terms"`     // the lowercased distinct query terms scored against
}

// ExplainInputs are the real ranking inputs an explainer needs, captured at the
// same point Search/MatchAnchors compute them. A caller that already ran the
// search passes the ordered Cov values it observed.
type ExplainInputs struct {
	Terms []string // distinct lowercased query terms (from lowerSet)
	Multi bool     // true when the query OR-expanded (>1 term) — gates coverage re-rank
	Sort  string   // p.Sort: "" relevance, else "newest"/"oldest" overlay
}

// rankMethod returns the ranking-regime label for the given inputs — the exact
// branch Search/MatchAnchors took.
func rankMethod(in ExplainInputs) string {
	if in.Sort != "" {
		return MethodSortOverlay
	}
	if in.Multi {
		return MethodBM25Coverage
	}
	return MethodBM25
}

// Explain builds one honest ScoreExplain per (already-ordered) coverage value,
// in result order. `covs[i]` is the coverage the ranker computed for the hit at
// position i (Anchor.Cov, or scoredHit.cov). The returned breakdowns describe
// the REAL regime — no invented blend, no fake BM25 scalar.
//
// Under a sort overlay, bm25 did not order the hits, so BM25Rank is -1 and
// Recency is 1. Under relevance, BM25Rank is the hit's bm25 ordinal: for the
// single-term (bm25-only) regime it equals the result position; for the
// multi-term regime it is unknown after the stable coverage re-sort, so it is
// reported as -1 (honest: we cannot recover the pre-resort bm25 ordinal here).
func Explain(covs []int, in ExplainInputs) []ScoreExplain {
	method := rankMethod(in)
	terms := append([]string(nil), in.Terms...) // defensive copy; never alias the caller's slice
	out := make([]ScoreExplain, 0, len(covs))
	for i, cov := range covs {
		e := ScoreExplain{
			Coverage: cov,
			Final:    i,
			Method:   method,
			Terms:    append([]string(nil), terms...), // each hit owns its own copy
		}
		switch method {
		case MethodSortOverlay:
			e.BM25Rank = -1
			e.Recency = 1
		case MethodBM25:
			e.BM25Rank = i // single-term relevance: position IS bm25 order
		default: // MethodBM25Coverage
			e.BM25Rank = -1 // pre-resort bm25 ordinal not recoverable post coverage re-sort
		}
		out = append(out, e)
	}
	return out
}

// Search runs the FTS5 keyword query against one project's db and returns up to
// `limit` ranked Hits (OR/coverage re-rank for multi-term plain queries; a
// single-term query is byte-identical to a plain FTS5 MATCH).
func Search(dbp, q string, limit int, p SearchParams) []Hit {
	scored, _ := searchScored(dbp, q, limit, p)
	if limit < 0 {
		limit = 0
	}
	out := make([]Hit, 0, limit)
	for i := 0; i < len(scored) && i < limit; i++ {
		out = append(out, scored[i].Hit)
	}
	return out
}

// SearchExplained runs the same ranking as Search and returns the top-`limit`
// Hits alongside a parallel, honest ScoreExplain per hit (explains[i] explains
// out[i]). It is the clean entrypoint the cli calls behind --debug-search; no
// extra query, no behavior change — the order is byte-identical to Search.
func SearchExplained(dbp, q string, limit int, p SearchParams) (out []Hit, explains []ScoreExplain) {
	scored, in := searchScored(dbp, q, limit, p)
	if limit < 0 {
		limit = 0
	}
	n := len(scored)
	if n > limit {
		n = limit
	}
	out = make([]Hit, 0, n)
	covs := make([]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, scored[i].Hit)
		covs = append(covs, scored[i].cov)
	}
	return out, Explain(covs, in)
}

// searchScored is the shared engine for Search/SearchExplained: it runs the FTS5
// query, applies the coverage re-rank, and returns the FULLY ORDERED scoredHit
// slice (pre-limit) plus the ExplainInputs that describe the regime used. The
// returned slice carries the real cov values the ranking consumed.
func searchScored(dbp, q string, limit int, p SearchParams) ([]scoredHit, ExplainInputs) {
	con, err := openRO(dbp)
	if err != nil {
		return nil, ExplainInputs{}
	}
	defer con.Close()

	match, terms, multi, ok := buildMatch(q, p)
	if !ok {
		return nil, ExplainInputs{}
	}

	where := []string{"messages_fts MATCH ?"}
	params := []any{match}
	if !p.IncludeSubagents {
		where = append(where, "s.is_subagent=0")
	}
	if p.Role != "" {
		where = append(where, "m.role=?")
		params = append(params, p.Role)
	}
	if p.MinMessages != 0 {
		where = append(where, "s.message_count >= ?")
		params = append(params, p.MinMessages)
	}
	dateWhere(&where, &params, p.Since, p.Before)

	// Overfetch so tool-only filtering + coverage re-rank still reach `limit`;
	// wider for multi-term OR (base 8 vs 4, scaled per the cases below).
	base := 4
	if multi {
		base = 8
	}
	var fetch int
	switch {
	case !p.IncludeTools:
		fetch = limit * base
	case multi:
		fetch = limit * 2
	default:
		fetch = limit
	}

	sqlText := `SELECT m.session_id, m.role, m.ts_iso, s.is_subagent, s.parent_id, m.content,
	                   snippet(messages_fts,0,'>>>','<<<','…',16) AS snip
	            FROM messages_fts JOIN messages m ON m.id=messages_fts.rowid
	            JOIN sessions s ON s.id=m.session_id
	            WHERE ` + strings.Join(where, " AND ") + " " + orderClause(p.Sort) + " LIMIT ?"
	params = append(params, fetch)

	rows, err := con.QueryContext(context.Background(), sqlText, params...)
	if err != nil {
		return nil, ExplainInputs{}
	}
	defer rows.Close()

	lterms := lowerSet(terms)
	scored := make([]scoredHit, 0, fetch)
	for rows.Next() {
		var (
			sid     string
			role    sql.NullString
			iso     sql.NullString
			isSub   int
			parent  sql.NullString
			content sql.NullString
			snip    sql.NullString
		)
		if err := rows.Scan(&sid, &role, &iso, &isSub, &parent, &content, &snip); err != nil {
			return nil, ExplainInputs{}
		}
		var disp string
		if p.IncludeTools {
			disp = strings.TrimSpace(reWhitespace.ReplaceAllString(snip.String, " "))
		} else {
			// Rebuild the snippet from tool-stripped content; a tool-ONLY match
			// (no human text) is excluded by default.
			haystack := parse.StripTools(content.String)
			s, present := query.MakeSnippet(haystack, terms)
			if !present {
				continue
			}
			disp = s
		}
		cov := coverage(lterms, strings.ToLower(haystackFor(p.IncludeTools, content.String)), multi)
		scored = append(scored, scoredHit{
			Hit: Hit{
				ISO:        iso.String,
				SessionID:  sid,
				Role:       role.String,
				IsSubagent: isSub != 0,
				Parent:     parent.String,
				Snippet:    disp,
			},
			cov: cov,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, ExplainInputs{}
	}

	// Coverage re-rank (relevance mode only): docs matching more distinct terms
	// float up; bm25 order is the tiebreak (stable sort by original index).
	if multi && p.Sort == "" {
		sort.SliceStable(scored, func(i, j int) bool {
			return scored[i].cov > scored[j].cov
		})
	}

	return scored, ExplainInputs{Terms: lterms, Multi: multi, Sort: p.Sort}
}

// haystackFor returns the coverage haystack: raw content when tools are
// included, else the tool-stripped form (content for include-tools,
// StripTools(content) otherwise).
func haystackFor(includeTools bool, content string) string {
	if includeTools {
		return content
	}
	return parse.StripTools(content)
}

// LinearFallback is the FTS5-absent linear scan over a project's JSONL, honoring
// the same flags + phrase (substring/adjacency) semantics.
//
// NOTE: modernc.org/sqlite always has FTS5, so this path is dead in practice —
// kept for parity with the FTS5 path.
func LinearFallback(transcriptDir, q string, limit int, p SearchParams) []Hit {
	clean := query.StripStopwords(query.SanitizeFTS5Query(q))
	terms := query.ParseTerms(clean)
	if len(terms) == 0 {
		return []Hit{}
	}

	type linHit struct {
		epoch float64
		Hit
	}
	var hits []linHit

	for _, f := range paths.ContainedJSONL(transcriptDir) {
		sid, isSub, parent := index.SessionIDFor(f, transcriptDir)
		if isSub != 0 && !p.IncludeSubagents {
			continue
		}
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(fh)
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			var obj map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &obj); err != nil {
				continue
			}
			typ, _ := obj["type"].(string)
			if !isIndexableType(typ) {
				continue
			}
			rolev := parse.MsgRole(obj)
			if p.Role != "" && rolev != p.Role {
				continue
			}
			text := parse.ExtractText(obj)
			var hay string
			if p.IncludeTools {
				hay = strings.ToLower(text)
			} else {
				hay = strings.ToLower(parse.StripTools(text))
			}
			if !containsAll(hay, terms) { // AND; phrases as substrings (adjacency)
				continue
			}
			iso, _ := obj["timestamp"].(string)
			iso10 := first10(iso)
			if (p.Since != "" && iso10 < p.Since) || (p.Before != "" && iso10 > p.Before) {
				continue
			}
			base := text
			if !p.IncludeTools {
				base = parse.StripTools(text)
			}
			snip, present := query.MakeSnippet(base, terms)
			if !present { // tool-only match — excluded by default
				continue
			}
			hits = append(hits, linHit{
				epoch: parse.ISOToEpoch(iso),
				Hit: Hit{
					ISO:        iso,
					SessionID:  sid,
					Role:       rolev,
					IsSubagent: isSub != 0,
					Parent:     parent,
					Snippet:    snip,
				},
			})
		}
		fh.Close()
	}

	// Sort by epoch; reverse (newest first) unless the sort mode is "oldest".
	reverse := p.Sort != "oldest"
	sort.SliceStable(hits, func(i, j int) bool {
		if reverse {
			return hits[i].epoch > hits[j].epoch
		}
		return hits[i].epoch < hits[j].epoch
	})

	if limit < 0 {
		limit = 0
	}
	out := make([]Hit, 0, limit)
	for i := 0; i < len(hits) && i < limit; i++ {
		out = append(out, hits[i].Hit)
	}
	return out
}

// SearchAll is cross-project discovery: search every project, surface each matching
// project's most-recent hit, ordered by recency. `pathPred` (may be nil) filters
// which projects are touched.
func SearchAll(q string, limit int, p SearchParams, pathPred func(cwd string) bool) []AllHit {
	best := map[string]AllHit{}

	for _, d := range paths.AllProjectDirs() {
		if pathPred != nil && !pathPred(paths.ProjectCWD(d)) {
			continue
		}
		dbp, _, _, err := index.EnsureIndexed(d, false)
		if err != nil {
			continue
		}
		res := Search(dbp, q, limit, p)
		if len(res) == 0 {
			continue
		}
		label := paths.ProjectLabel(d)
		// Most-recent matching message in this project (max by ISO, "" coerced).
		row := res[0]
		for _, r := range res[1:] {
			if r.ISO > row.ISO {
				row = r
			}
		}
		best[label] = AllHit{Hit: row, Project: label, Hits: len(res)}
	}

	out := make([]AllHit, 0, len(best))
	for _, v := range best {
		out = append(out, v)
	}
	// Sort by ISO; reverse (newest first) unless the sort mode is "oldest".
	// Stable so equal keys keep their discovery order.
	reverse := p.Sort != "oldest"
	sort.SliceStable(out, func(i, j int) bool {
		if reverse {
			return out[i].ISO > out[j].ISO
		}
		return out[i].ISO < out[j].ISO
	})

	if limit < 0 {
		limit = 0
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// LineageRoot walks parent_id to the conversation root (capped at 64 hops).
func LineageRoot(con *sql.DB, sid string) string {
	const cap = 64
	seen := map[string]struct{}{}
	cur := sid
	for cur != "" {
		if _, ok := seen[cur]; ok {
			break
		}
		if len(seen) >= cap {
			break
		}
		seen[cur] = struct{}{}
		var parent sql.NullString
		err := con.QueryRowContext(context.Background(),
			"SELECT parent_id FROM sessions WHERE id=?", cur).Scan(&parent)
		if err != nil || !parent.Valid || parent.String == "" {
			break
		}
		cur = parent.String
	}
	return cur
}

// MatchAnchors runs FTS5 recall and returns ranked Anchor messages (the
// OR/coverage logic of Search, returning message ids for the view layer).
// `fetch` is the overfetch ceiling.
func MatchAnchors(con *sql.DB, q string, fetch int, p SearchParams) []Anchor {
	match, terms, multi, ok := buildMatch(q, p)
	if !ok {
		return []Anchor{}
	}

	where := []string{"messages_fts MATCH ?"}
	params := []any{match}
	if !p.IncludeSubagents {
		where = append(where, "s.is_subagent=0")
	}
	if p.Role != "" {
		where = append(where, "m.role=?")
		params = append(params, p.Role)
	}
	if p.MinMessages != 0 {
		where = append(where, "s.message_count >= ?")
		params = append(params, p.MinMessages)
	}
	dateWhere(&where, &params, p.Since, p.Before)

	sqlText := `SELECT m.id, m.session_id, m.uuid, m.role, m.ts_iso, s.parent_id, m.content, s.missing_since,
	                   snippet(messages_fts,0,'>>>','<<<','…',16) AS snip
	            FROM messages_fts JOIN messages m ON m.id=messages_fts.rowid
	            JOIN sessions s ON s.id=m.session_id
	            WHERE ` + strings.Join(where, " AND ") + " " + orderClause(p.Sort) + " LIMIT ?"
	params = append(params, fetch)

	rows, err := con.QueryContext(context.Background(), sqlText, params...)
	if err != nil {
		return []Anchor{}
	}
	defer rows.Close()

	lterms := lowerSet(terms)
	out := []Anchor{}
	for rows.Next() {
		var (
			mid     int
			sid     string
			uuid    sql.NullString
			role    sql.NullString
			iso     sql.NullString
			parent  sql.NullString
			content sql.NullString
			missing sql.NullFloat64
			snip    sql.NullString
		)
		if err := rows.Scan(&mid, &sid, &uuid, &role, &iso, &parent, &content, &missing, &snip); err != nil {
			return []Anchor{}
		}
		var disp string
		if p.IncludeTools {
			disp = strings.TrimSpace(reWhitespace.ReplaceAllString(snip.String, " "))
		} else {
			haystack := parse.StripTools(content.String)
			s, present := query.MakeSnippet(haystack, terms)
			if !present {
				continue
			}
			disp = s
		}
		cov := coverage(lterms, strings.ToLower(haystackFor(p.IncludeTools, content.String)), multi)
		out = append(out, Anchor{
			ID:           mid,
			SessionID:    sid,
			UUID:         uuid.String,
			Role:         role.String,
			ISO:          iso.String,
			Parent:       parent.String,
			Snip:         disp,
			Cov:          cov,
			MissingSince: missing.Float64, // 0 when NULL (present)
		})
	}
	if err := rows.Err(); err != nil {
		return []Anchor{}
	}

	if multi && p.Sort == "" {
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Cov > out[j].Cov
		})
	}
	return out
}

// openRO opens dbp read-only with a single serialized connection, one handle per
// call. Kept inline here so retrieve does not depend on the index package's
// connection helper for its read path.
func openRO(dbp string) (*sql.DB, error) {
	con, err := sql.Open("sqlite", "file:"+dbp+"?mode=ro")
	if err != nil {
		return nil, err
	}
	// modernc serializes; one connection means a single SQLite handle and
	// avoids "database is locked" against a concurrent indexer.
	con.SetMaxOpenConns(1)
	return con, nil
}

// isIndexableType reports whether a JSONL record type is one RawClaw indexes.
func isIndexableType(typ string) bool {
	for _, t := range parse.IndexableTypes {
		if t == typ {
			return true
		}
	}
	return false
}

// containsAll reports whether every term is a substring of hay.
func containsAll(hay string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

// first10 returns the first 10 bytes of an ISO timestamp (the YYYY-MM-DD prefix
// the date bounds compare against, assuming an ASCII date).
func first10(iso string) string {
	if len(iso) < 10 {
		return iso
	}
	return iso[:10]
}
