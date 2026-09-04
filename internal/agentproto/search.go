package agentproto

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
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
	"github.com/MoonCaves/rawclaw/internal/view"
)

func Search(rawQuery string, scope []view.Scope, opts SearchOpts, embedder embed.Embedder) SearchEnvelope {
	limit := opts.Limit
	if limit == 0 {
		limit = DefaultSearchLimit
	}

	fetch := max(limit*8, 30)

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
		Offset:           opts.Offset,
		MinMessages:      opts.MinMessages,
		RawMatch:         rawMatch,
	}

	var qvecFn func() []float64
	if embedder != nil && opts.Sort == "" {
		var (
			once sync.Once
			vec  []float64
		)
		qvecFn = func() []float64 {
			once.Do(func() { vec = embedder.Embed(context.Background(), rawQuery) })
			return vec
		}
	}

	var (
		cands       []retrieve.Anchor
		reports     []ScopeReport
		hitCeiling  bool
		vecCov      VectorCoverage
		answered    bool
		storeName   = StoreConsolidated
		storeNote   string
		pathNoMatch bool
	)

	if scope == nil {
		cands, reports, hitCeiling, vecCov, storeNote, answered = searchOneStore(rawQuery, fetch, limit, p, qvecFn, opts)
	}

	if !answered {
		storeName = StorePerProject
		if scope == nil {
			scope = fallbackScope(opts)
		}
		if len(scope) == 0 {
			pathNoMatch = opts.IncludePath != ""
		} else if opts.IncludePath != "" || opts.ExcludePath != "" {
			scope = scopes.FilterByPath(scope, opts.IncludePath, opts.ExcludePath)
			if len(scope) == 0 {
				pathNoMatch = opts.IncludePath != ""
			}
		}
		cands, reports, hitCeiling, vecCov = collectCandidates(scope, rawQuery, fetch, limit, p, qvecFn)
	}

	cands, droppedTurn := dropCurrentTurn(cands, opts.CurrentSession)
	sortCandidates(cands, opts.Sort)

	seen := map[string]struct{}{}
	all := []SearchRef{}
	picked := []retrieve.Anchor{}
	for _, r := range cands {
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
			Missing:   r.OnlyCopySince > 0,
			Routine:   r.Routine,
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

	enrichSearchResults(results, picked)

	total := len(all)
	hasMore := truncated || hitCeiling
	nextCmd := ""
	if hasMore {
		wider := max(limit*4, 20)
		nextCmd = fmt.Sprintf("rawclaw %q --limit %d", rawQuery, wider)
	}

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
		VectorCoverage:    vecCov,
		Warnings: buildWarnings(warningInputs{
			results:        results,
			reports:        reports,
			sort:           opts.Sort,
			newestISO:      newestISO,
			total:          total,
			hitCeiling:     hitCeiling,
			droppedTurn:    droppedTurn,
			storeNote:      storeNote,
			vectorCoverage: vecCov,
			pathNoMatch:    pathNoMatch,
			includePath:    opts.IncludePath,
		}),
		ExcludedCurrentTurn: droppedTurn,
		Store:               storeName,
		StoreNote:           storeNote,
	}
}

type warningInputs struct {
	results        []SearchRef
	reports        []ScopeReport
	sort           string
	newestISO      string
	total          int
	hitCeiling     bool
	droppedTurn    int
	storeNote      string
	vectorCoverage VectorCoverage
	pathNoMatch    bool
	includePath    string
}

const broadQueryMatches = 20
const recencySkewGap = 24 * time.Hour

func buildWarnings(in warningInputs) []Warning {
	var out []Warning

	if in.pathNoMatch && LooksLikeSessionID(in.includePath) {
		out = append(out, Warning{
			Code:    WarnIncludePathNoMatch,
			Facts:   map[string]any{"include_path": in.includePath},
			Message: fmt.Sprintf("--include-path matched no project working directory; it filters paths, not session IDs — did you mean `rawclaw outline %s` or `rawclaw --resume %s`?", in.includePath, in.includePath),
		})
	}

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

	if in.droppedTurn > 0 {
		out = append(out, Warning{
			Code:    WarnCurrentTurnExcluded,
			Facts:   map[string]any{"excluded": in.droppedTurn},
			Message: currentTurnLine(in.droppedTurn),
		})
	}

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

	if unfolded > 0 {
		out = append(out, Warning{
			Code:    WarnNotConsolidated,
			Facts:   map[string]any{"databases": unfolded},
			Message: fmt.Sprintf("%d project database(s) are not in the one store and were NOT searched — run `rawclaw consolidate`", unfolded),
		})
	}

	if in.vectorCoverage.Ran && in.vectorCoverage.MissingMsgs > 0 {
		out = append(out, Warning{
			Code: WarnVectorGap,
			Facts: map[string]any{
				"candidate_msgs": in.vectorCoverage.CandidateMsgs,
				"vectored_msgs":  in.vectorCoverage.VectoredMsgs,
				"missing_msgs":   in.vectorCoverage.MissingMsgs,
			},
			Message: fmt.Sprintf("semantic tier has partial coverage (%d of %d candidate messages vectored, %d missing) — run `rawclaw --reindex-vectors`",
				in.vectorCoverage.VectoredMsgs, in.vectorCoverage.CandidateMsgs, in.vectorCoverage.MissingMsgs),
		})
	}

	if in.storeNote != "" {
		out = append(out, Warning{
			Code:    WarnStoreFallback,
			Facts:   map[string]any{"store": StorePerProject},
			Message: in.storeNote,
		})
	}

	if projects, sample := projectSpread(in.results); projects >= 2 {
		out = append(out, Warning{
			Code:  WarnProjectSpread,
			Facts: map[string]any{"projects": projects, "sample": sample},
			Message: fmt.Sprintf("matches span %d projects: %s — narrow with --this-project or read a specific ref.",
				projects, strings.Join(sample, ", ")),
		})
	}

	if len(in.results) > 0 {
		out = append(out, Warning{Code: WarnRawHistory, Message: freshnessNote})
	}
	return out
}

func searchOneStore(
	rawQuery string,
	fetch, limit int,
	p retrieve.SearchParams,
	qvecFn func() []float64,
	opts SearchOpts,
) (cands []retrieve.Anchor, reports []ScopeReport, hitCeiling bool, vecCov VectorCoverage, note string, ok bool) {
	con, sessions, err := index.OpenConsolidated()
	if err != nil {
		return nil, nil, false, VectorCoverage{}, "one store unavailable (" + err.Error() + ") — searched per project instead", false
	}
	defer con.Close()

	projects, narrowed, err := resolveStoreProjects(con, opts.Project, opts.Projects, opts.ProjectDir, opts.IncludePath, opts.ExcludePath)
	if err != nil {
		return nil, nil, false, VectorCoverage{}, "one store scope lookup failed (" + err.Error() + ") — searched per project instead", false
	}
	if narrowed && len(projects) == 0 {
		return nil, nil, false, VectorCoverage{}, "one store knows no project matching this scope — searched per project instead", false
	}
	p.Projects = projects
	p.SourceTool = opts.Source

	rows, exhausted := storeAnchors(con, rawQuery, fetch, limit, p)
	hitCeiling = !exhausted

	if qvecFn != nil {
		if len(rows) >= limit {
			qvecFn = nil
		}
	}
	if qvecFn != nil {
		vecCov.Ran = true
		var cov semantic.CoverageStats
		if narrowed {
			cov, _ = semantic.MeasureCoverage(con, projects...)
		} else {
			cov, _ = semantic.MeasureCoverage(con)
		}
		vecCov.CandidateMsgs = cov.Candidates
		vecCov.VectoredMsgs = cov.Vectored
		vecCov.MissingMsgs = cov.Missing
	}

	if qvecFn != nil && store.HasVectors(con) {
		rows = semantic.Fuse(con, rows, qvecFn(), fetch, p.IncludeSubagents)
	}

	dbp := index.ConsolidatedPath()
	routines, _ := store.RoutineSet(con)
	for i := range rows {
		rows[i].Root = retrieve.LineageRoot(con, rows[i].SessionID)
		rows[i].DBP = dbp
		rows[i].Rank = i
		rows[i].Routine = routines[rows[i].SessionID]
		cands = append(cands, rows[i])
	}

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
	return cands, reports, hitCeiling, vecCov, "", true
}

const maxStoreWindow = 20000

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
			return rows, true
		}
		rows = wider
	}
	return rows, false
}

func distinctSessions(rows []retrieve.Anchor) int {
	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[r.SessionID] = struct{}{}
	}
	return len(seen)
}

func fallbackScope(opts SearchOpts) []view.Scope {
	var sc []view.Scope
	if opts.ScopeFallback != nil {
		sc = opts.ScopeFallback()
	} else {
		sc = allScope()
	}
	if opts.Project == "" && len(opts.Projects) == 0 {
		return sc
	}
	if len(opts.Projects) > 0 {
		projSet := make(map[string]struct{}, len(opts.Projects))
		for _, p := range opts.Projects {
			projSet[p] = struct{}{}
		}
		var out []view.Scope
		for _, s := range sc {
			if _, ok := projSet[s.Project]; ok {
				out = append(out, s)
			}
		}
		return out
	}
	out := make([]view.Scope, 0, 1)
	for _, s := range sc {
		if s.Project == opts.Project {
			out = append(out, s)
		}
	}
	return out
}

func scopesComplete(reports []ScopeReport) bool {
	for _, r := range reports {
		if r.Status == ScopeSkippedError || r.Status == ScopeStaleFallback || r.Status == ScopeNotConsolidated {
			return false
		}
	}
	return true
}

func collectCandidates(
	scope []view.Scope,
	query string,
	fetch int,
	limit int,
	p retrieve.SearchParams,
	qvecFn func() []float64,
) ([]retrieve.Anchor, []ScopeReport, bool, VectorCoverage) {
	cands := []retrieve.Anchor{}
	reports := make([]ScopeReport, 0, len(scope))
	hitCeiling := false
	var vecCov VectorCoverage
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
			hitCeiling = true
		}
		lexicalEnough := len(rows) >= limit
		if qvecFn != nil && !lexicalEnough {
			vecCov.Ran = true
			cov, _ := semantic.MeasureCoverage(con)
			vecCov.CandidateMsgs += cov.Candidates
			vecCov.VectoredMsgs += cov.Vectored
			vecCov.MissingMsgs += cov.Missing
		}
		if qvecFn != nil && !lexicalEnough && store.HasVectors(con) {
			rows = semantic.Fuse(con, rows, qvecFn(), fetch, p.IncludeSubagents)
		}
		routines, _ := store.RoutineSet(con)
		for i := range rows {
			rows[i].Root = retrieve.LineageRoot(con, rows[i].SessionID)
			rows[i].Project = sc.Project
			rows[i].DBP = dbp
			rows[i].Rank = i
			rows[i].Routine = routines[rows[i].SessionID]
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
	return cands, reports, hitCeiling, vecCov
}

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

func isCurrentSession(candSID, arg string) bool {
	if candSID == arg {
		return true
	}
	if len(arg) < 8 || !strings.HasPrefix(candSID, arg) {
		return false
	}
	return !strings.Contains(candSID[len(arg):], "/")
}

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

type dbConnCache struct {
	conns map[string]*sql.DB
}

func (c *dbConnCache) get(dbp string) (*sql.DB, error) {
	if c.conns == nil {
		c.conns = make(map[string]*sql.DB)
	}
	if db, ok := c.conns[dbp]; ok && db != nil {
		return db, nil
	}
	db, err := store.ConnectRO(dbp)
	if err != nil {
		return nil, err
	}
	c.conns[dbp] = db
	return db, nil
}

func (c *dbConnCache) close() {
	for _, db := range c.conns {
		if db != nil {
			_ = db.Close()
		}
	}
	c.conns = nil
}

func attachTopicsWithCache(refs []SearchRef, anchors []retrieve.Anchor, cache *dbConnCache) {
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
		con, err := cache.get(dbp)
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
	}
}

const searchTitleCap = 70

func attachLastActivityWithCache(refs []SearchRef, anchors []retrieve.Anchor, cache *dbConnCache) {
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
		con, err := cache.get(dbp)
		if err != nil {
			continue
		}
		seenLast := map[string]string{}
		for _, i := range idxs {
			sid := anchors[i].SessionID
			last, done := seenLast[sid]
			if !done {
				last = view.SessionLastActivity(con, sid)
				seenLast[sid] = last
			}
			refs[i].Last = last
		}
	}
}

func enrichSearchResults(refs []SearchRef, anchors []retrieve.Anchor) {
	if len(refs) == 0 || len(refs) != len(anchors) {
		return
	}
	cache := &dbConnCache{}
	defer cache.close()
	attachTopicsWithCache(refs, anchors, cache)
	attachLastActivityWithCache(refs, anchors, cache)
}

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
