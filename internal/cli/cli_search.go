package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/adapters"
	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/query"
	"github.com/MoonCaves/rawclaw/internal/render"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/MoonCaves/rawclaw/internal/view"
)

func thisScope(w io.Writer, o *Options) (scope []view.Scope, td string, ok bool) {
	td = resolveTDir(o.Dir, o.DirSet)
	if !o.DirSet || td != o.Dir {
		if gitRoot := paths.GitRoot(o.Dir); gitRoot != "" {
			all := allScope(context.Background(), o.Source, o.Reindex)
			matched := scopes.FilterByProjectDir(all, o.Dir)
			if len(matched) > 0 {
				return matched, td, true
			}
		}
	}
	if td == "" || !isDir(td) {
		if o.DirSet {
			fmt.Fprintf(w, "No transcript history for --dir %s. Try --list, or --all for every project.\n", realpathExpand(o.Dir))
		}
		return nil, "", false
	}
	return []view.Scope{{Project: paths.ProjectLabel(td), TDir: td}}, td, true
}

// allScope builds the search scope spanning the requested runtime(s) — Claude
// projects and/or Codex cwd-groups — via the scopes enumerator. source ""
// spans all; "claude"/"codex" narrows. ctx (the run's watchdog context)
// bounds the archive enumeration's git probes.
func allScope(ctx context.Context, source string, reindex bool, paths ...string) []view.Scope {
	var pathPred func(string) bool
	if len(paths) > 0 {
		include := paths[0]
		exclude := ""
		if len(paths) > 1 {
			exclude = paths[1]
		}
		if include != "" || exclude != "" {
			pathPred = query.PathPredicate(include, exclude)
		}
	}
	return scopes.All(ctx, source, reindex, pathPred)
}

// runReindexVectors builds/updates the semantic index for the scope.
func runBrowse(ctx context.Context, w io.Writer, o *Options) error {
	if o.pathScoped() || o.Source != "" || (o.All && !o.ThisProject) {
		var universe []view.Scope
		if o.ThisProject {
			sc, _, ok := thisScope(w, o)
			if !ok {
				return nil
			}
			universe = sc
		} else {
			universe = allScope(ctx, o.Source, o.Reindex, o.IncludePath, o.ExcludePath)
		}
		return runBrowseScoped(w, o, universe)
	}
	sc, td, ok := thisScope(w, o)
	if !ok {
		if !o.DirSet {
			fmt.Fprintf(w, "No transcript history for %s; showing recent sessions across all projects.\n", realpathExpand(o.Dir))
			universe := allScope(ctx, o.Source, o.Reindex, o.IncludePath, o.ExcludePath)
			return runBrowseScoped(w, o, universe)
		}
		return nil
	}
	_ = sc
	var rows []view.BrowseRow
	usedConsolidated := false
	var (
		indexStale bool
		staleNote  string
	)
	if !o.Reindex {
		refreshActiveSessions(o.currentSession())
		if con, _, err := index.OpenConsolidated(); err == nil {
			defer con.Close()
			if freshness, fErr := index.CheckIndexFreshness(con); fErr == nil && !freshness.Fresh {
				indexStale = true
				staleNote = staleIngestNote()
			}
			if res, err := view.BrowseScoped(con, o.Limit, o.Since, o.Before, o.Source, []string{paths.ProjectLabel(td)}); err == nil && len(res) > 0 {
				rows = make([]view.BrowseRow, 0, len(res))
				for _, r := range res {
					rows = append(rows, r.BrowseRow)
				}
				usedConsolidated = true
			}
		}
	}
	if !usedConsolidated {
		rows = view.Browse(td, o.Limit, o.Since, o.Before)
	}
	if o.JSON {
		return EmitJSON(w, struct {
			Project   string           `json:"project"`
			Stale     bool             `json:"stale,omitempty"`
			StaleNote string           `json:"stale_note,omitempty"`
			Sessions  []view.BrowseRow `json:"sessions"`
		}{
			Project:   paths.ProjectLabel(td),
			Stale:     indexStale,
			StaleNote: staleNote,
			Sessions:  rows,
		})
	}
	if o.oneline() {
		for _, r := range rows {
			ref := lastSlice8(r.SessionID) + ":"
			clean := agentproto.CleanSnippetOneline(r.Preview)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ref, timefmt.UTC(time.Unix(int64(r.LastTS), 0)), paths.ProjectLabel(td), clean)
		}
		return nil
	}
	render.PrintBrowse(w, rows, paths.ProjectLabel(td))
	if staleNote != "" {
		fmt.Fprintf(w, "\nnote: %s\n", staleNote)
	}
	return nil
}

// runBrowseScoped is the cross-project shape of the no-query browse: recent
// sessions across the scopes the Scope flags leave standing (Claude + Codex +
// retained — the same enumeration search uses), merged newest-first and capped
// at --limit. It answers from the consolidated store with a single read connection,
// falling back to per-project databases only if the consolidated store is unavailable.
func runBrowseScoped(w io.Writer, o *Options, universe []view.Scope) error {
	scope := scopes.FilterByPath(universe, o.IncludePath, o.ExcludePath)
	if o.pathScoped() && len(scope) == 0 {
		return browseNoScopeMatch(w, o, len(universe))
	}

	var (
		rows             []view.BrowseAllRow
		usedConsolidated bool
		indexStale       bool
		staleNote        string
	)
	var freshness *index.IndexFreshness

	if !o.Reindex {
		refreshActiveSessions(o.currentSession())
		if con, _, err := index.OpenConsolidated(); err == nil {
			defer con.Close()
			if f, fErr := index.CheckIndexFreshness(con); fErr == nil {
				freshness = &f
			}
			var projects []string
			if o.pathScoped() || o.ThisProject {
				seen := make(map[string]bool, len(scope))
				for _, sc := range scope {
					if !seen[sc.Project] {
						seen[sc.Project] = true
						projects = append(projects, sc.Project)
					}
				}
			}
			if res, err := view.BrowseScoped(con, o.Limit, o.Since, o.Before, o.Source, projects); err == nil {
				rows = res
				usedConsolidated = true
			}
		}
	}

	if !usedConsolidated {
		rows = []view.BrowseAllRow{}
		for _, sc := range scope {
			dbp, _, err := scopes.Resolve(sc, o.Reindex)
			if err != nil {
				continue // an unresolvable scope can't contribute rows; others still can
			}
			if freshness == nil {
				if db, dbErr := store.ConnectRO(dbp); dbErr == nil {
					if f, fErr := index.CheckIndexFreshness(db); fErr == nil {
						freshness = &f
					}
					_ = db.Close()
				}
			}
			if o.Source != "" && sc.Source != "" && sc.Source != o.Source {
				continue
			}
			for _, r := range view.BrowseDB(dbp, o.Limit, o.Since, o.Before) {
				rows = append(rows, view.BrowseAllRow{Project: sc.Project, BrowseRow: r})
			}
		}
		// Newest-first across projects; each scope contributed at most --limit rows,
		// so the merge only has to re-sort and cap.
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].LastTS != rows[j].LastTS {
				return rows[i].LastTS > rows[j].LastTS
			}
			return rows[i].SessionID < rows[j].SessionID
		})
		if len(rows) > o.Limit {
			rows = rows[:o.Limit]
		}
	}
	if !o.Reindex && freshness != nil && !freshness.Fresh {
		if freshness.Reason == "no_ingest_watermark" {
			staleNote = freshnessUnknownNote()
		} else {
			indexStale = true
			staleNote = staleIngestNote()
		}
	}

	if o.JSON {
		return EmitJSON(w, browseScopeJSON(o, len(scope), rows, indexStale, staleNote))
	}
	if o.oneline() {
		for _, r := range rows {
			ref := lastSlice8(r.SessionID) + ":"
			clean := agentproto.CleanSnippetOneline(r.Preview)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ref, timefmt.UTC(time.Unix(int64(r.LastTS), 0)), r.Project, clean)
		}
		return nil
	}
	render.PrintBrowseAll(w, rows, browseScopeLabel(o))
	if staleNote != "" {
		fmt.Fprintf(w, "\nnote: %s\n", staleNote)
	}
	return nil
}

// browseNoScopeMatch reports a path scope that kept no project at all. Scope
// never relaxes: rather than quietly widening back to the cwd or to every
// project — the silent rewrite this whole contract exists to kill — the empty
// is printed WITH its real boundary, the size of the universe the predicate ran
// against, plus the verb that lists the working dirs it was matched on. Exit 0:
// an honestly empty scope is an answer, not an error.
func browseNoScopeMatch(w io.Writer, o *Options, universe int) error {
	if o.JSON {
		return EmitJSON(w, browseScopeJSON(o, 0, []view.BrowseAllRow{}, false, ""))
	}
	fmt.Fprintf(w, "No project matches %s (0 of %d searchable). Try --list to see their working dirs.\n",
		pathScopePhrase(o), universe)
	if o.IncludePath != "" && agentproto.LooksLikeSessionID(o.IncludePath) {
		fmt.Fprintf(w, "Hint: --include-path filters project paths, not session IDs. Did you mean `rawclaw outline %s` or `rawclaw --resume %s`?\n", o.IncludePath, o.IncludePath)
	}
	return nil
}

// pathScopePhrase echoes the path Scope flags back verbatim, so every message
// about them names what the caller actually typed.
func pathScopePhrase(o *Options) string {
	var parts []string
	if o.IncludePath != "" {
		parts = append(parts, "--include-path "+o.IncludePath)
	}
	if o.ExcludePath != "" {
		parts = append(parts, "--exclude-path "+o.ExcludePath)
	}
	return strings.Join(parts, " ")
}

// browseScopeLabel names the universe a cross-project browse actually covered.
// "all projects" is true only while nothing narrowed it — a header must never
// name a scope the caller did not ask for.
func browseScopeLabel(o *Options) string {
	label := "all projects"
	if o.ThisProject {
		label = "this project"
	}
	if o.Source != "" {
		label += " · source " + o.Source
	}
	if !o.pathScoped() {
		return label
	}
	return label + " matching " + pathScopePhrase(o)
}

// browseScopeJSON is the machine shape of a cross-project browse. Beyond the
// rows it reports the scope actually covered — the path flags verbatim and how
// many projects survived them — so an agent reading `sessions: []` can tell an
// empty corpus from a filter that matched no project at all. Same
// incompleteness-as-data posture as the search envelope's scope reports.
func browseScopeJSON(o *Options, projects int, rows []view.BrowseAllRow, stale bool, staleNote string) any {
	scope := "all"
	if o.ThisProject {
		scope = "project"
	}
	return struct {
		Scope       string              `json:"scope"`
		Source      string              `json:"source,omitempty"`
		IncludePath string              `json:"include_path,omitempty"`
		ExcludePath string              `json:"exclude_path,omitempty"`
		Projects    int                 `json:"projects"`
		Stale       bool                `json:"stale,omitempty"`
		StaleNote   string              `json:"stale_note,omitempty"`
		Sessions    []view.BrowseAllRow `json:"sessions"`
	}{scope, o.Source, o.IncludePath, o.ExcludePath, projects, stale, staleNote, rows}
}

// runSearch dispatches a query to the FALLBACK / BRIEF / DISCOVERY shapes.
func runSearch(ctx context.Context, w io.Writer, o *Options, args []string) error {
	q := strings.Join(args, " ")
	ftsExpr, usedOps := query.BooleanToFTS5(q)
	rawMatch := ""
	if usedOps {
		rawMatch = ftsExpr // no operators → leave empty for the plain search path
	}
	var ppred func(cwd string) bool
	if o.IncludePath != "" || o.ExcludePath != "" {
		ppred = query.PathPredicate(o.IncludePath, o.ExcludePath)
	}
	p := o.params(rawMatch)

	// FTS5 absent → linear fallback (this project, flat). Rarely taken in practice.
	if !index.FTS5OK() {
		sc, td, ok := thisScope(w, o)
		if !ok {
			return nil
		}
		_ = sc
		res := retrieve.LinearFallback(td, q, o.Limit, p)
		if o.JSON {
			return EmitJSON(w, rowsToJSON(res))
		}
		if o.oneline() {
			for _, r := range res {
				ref := lastSlice8(r.SessionID) + ":"
				clean := agentproto.CleanSnippetOneline(r.Snippet)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ref, r.ISO, paths.ProjectLabel(td), clean)
			}
			return nil
		}
		// Note line followed by a blank line (trailing "\n\n").
		fmt.Fprint(w, "[note] FTS5 unavailable on this build — slower linear scan, this project only.\n\n")
		PrintResults(w, res, -1)
		return nil
	}

	// DEBUG-SEARCH — read-only scoring explainer (LLM-free). Composes with --json
	// and --this-project; a pure output mode, no behavior change to the ranking.
	if o.DebugSearch {
		return runDebugSearch(w, o, q, p, ppred)
	}

	// DEFAULT (agent envelope) — a bare `rawclaw "query"` IS the search:
	// ranked refs + never-silent envelope. Search is the default verb.
	// Org-wide unless --this-project. Path include/exclude is applied inside
	// agentproto.Search (via opts).
	//
	// The scope is passed as a FUNCTION, not a list: the one consolidated store
	// answers this search with a single query, and enumerating every project to
	// build a scope list it would never use costs seconds of directory walking and
	// git probing. The function is called only if the store cannot answer.
	sopts := agentproto.SearchOpts{
		Limit:            o.Limit,
		Offset:           o.Offset,
		Role:             o.Role,
		Sort:             o.Sort,
		IncludeTools:     o.IncludeTools,
		IncludeSubagents: o.IncludeSubagents,
		Since:            o.Since,
		Before:           o.Before,
		MinMessages:      o.MinMessages,
		IncludePath:      o.IncludePath,
		ExcludePath:      o.ExcludePath,
		Source:           o.Source,
		CurrentSession:   o.currentSession(),
		Oneline:          o.oneline(),
	}
	label := ""
	if o.ThisProject {
		td := resolveTDir(o.Dir, o.DirSet)
		projLabel := ""
		if td != "" && isDir(td) {
			projLabel = paths.ProjectLabel(td)
		} else {
			projLabel = filepath.Base(realpathExpand(o.Dir))
		}
		sopts.ProjectDir = o.Dir
		sopts.ScopeFallback = func() []view.Scope {
			sc, _, _ := thisScope(w, o)
			return sc
		}
		if projLabel != "" {
			label = "on " + projLabel
		} else {
			label = "on this project"
		}
	} else {
		sopts.ScopeFallback = func() []view.Scope {
			return allScope(ctx, o.Source, o.Reindex, o.IncludePath, o.ExcludePath)
		}
		label = "across all projects"
	}

	var (
		indexStale bool
		staleNote  string
	)

	// An explicit --reindex or explicit --dir refreshes the targeted project.
	// Default and --this-project search is answer-first: queries run directly against
	// the consolidated store in single-digit milliseconds. Stale signal returns the
	// existing answers immediately while a throttled background ingest nudge self-heals.
	// Only explicit --reindex or explicit --dir overrides trigger synchronous refresh.
	if o.Reindex || o.DirSet {
		refreshThisProject(o)
	} else {
		refreshActiveSessions(o.currentSession())
		td := resolveTDir(o.Dir, o.DirSet)
		projLabel := ""
		if td != "" {
			projLabel = paths.ProjectLabel(td)
		}
		if con, _, err := index.OpenConsolidated(); err == nil {
			var freshness index.IndexFreshness
			var fErr error
			if td != "" {
				freshness, fErr = index.CheckProjectFreshness(con, projLabel, td, o.Source)
			} else {
				freshness, fErr = index.CheckIndexFreshness(con)
			}
			_ = con.Close()
			if fErr != nil || !freshness.Fresh {
				indexStale = true
				staleNote = staleIngestNote()
			}
		} else {
			indexStale = true
			staleNote = staleIngestNote()
		}
	}

	var emb embed.Embedder
	if o.Vector {
		emb = adapters.GetEmbedder()
	}

	var scope []view.Scope
	if o.Reindex {
		scope = sopts.ScopeFallback()
		if len(scope) == 0 {
			scope = []view.Scope{}
		}
	}

	if o.JSON {
		env := agentproto.Search(q, scope, sopts, emb)
		if indexStale {
			env.Complete = false
			env.Warnings = append(env.Warnings, agentproto.Warning{
				Code:    "index_stale",
				Message: staleNote,
				Facts:   map[string]any{"stale": true},
			})
		}
		return EmitJSON(w, struct {
			agentproto.SearchEnvelope
			Stale     bool   `json:"stale,omitempty"`
			StaleNote string `json:"stale_note,omitempty"`
		}{
			SearchEnvelope: env,
			Stale:          indexStale,
			StaleNote:      staleNote,
		})
	}

	if err := agentproto.SearchAndRender(w, q, scope, sopts, emb, label, false); err != nil {
		return err
	}
	if !o.oneline() && indexStale {
		fmt.Fprintf(w, "note: %s\n", staleNote)
	}
	return nil
}

// refreshThisProject indexes the project this command is running in, so a search
// answered from the one store still sees the session happening right now. It is
// advisory: a project with no transcript history, or an index that fails or is
// locked, leaves the store as it was — the search still runs, it just answers
// from what was already folded in. The indexing run's own write-through is what
// carries the new rows into the store.
func refreshThisProject(o *Options) {
	expDir := realpathExpand(o.Dir)
	for _, reg := range sources.Registered() {
		if o.Source != "" && o.Source != reg.ID {
			continue
		}
		if reg.ID == "claude" {
			if (o.ThisProject || o.DirSet) && paths.GitRoot(o.Dir) != "" {
				for _, sc := range scopes.FilterByProjectDir(scopes.ClaudeLive(), o.Dir) {
					if _, _, err := scopes.Resolve(sc, false); err != nil {
						slog.Debug("search: current-project refresh failed", "project", sc.Project, "err", err)
					}
				}
			} else {
				td := resolveTDir(o.Dir, o.DirSet)
				if td != "" && isDir(td) {
					if _, _, err := scopes.Resolve(view.Scope{Project: paths.ProjectLabel(td), TDir: td}, false); err != nil {
						slog.Debug("search: current-project refresh failed", "project", paths.ProjectLabel(td), "err", err)
					}
				}
			}
			continue
		}
		if reg.OptedIn == nil || reg.OptedIn(o.Source) {
			scopes.RefreshCWD(reg.ID, reg.New(), expDir)
		}
	}
}

// runDebugSearch handles the --debug-search shape: a read-only LLM-free scoring
// explainer. It runs the SAME ranking as a normal search (retrieve.SearchExplained
// is byte-identical to retrieve.Search) and renders a per-hit breakdown. Single
// project under --this-project; otherwise it loops per-project dbp exactly like
// the cross-project search path, merging the parallel (hits, explains) slices in
// lockstep so explains[i] keeps describing hits[i]. Composes with --json.
func runDebugSearch(w io.Writer, o *Options, q string, p retrieve.SearchParams, ppred func(cwd string) bool) error {
	var hits []retrieve.Hit
	var explains []retrieve.ScoreExplain

	if o.ThisProject {
		_, td, ok := thisScope(w, o)
		if !ok {
			return nil
		}
		dbp, _, _, err := index.EnsureIndexed(td, o.Reindex)
		if err != nil {
			return fmt.Errorf("debug-search ensure-indexed: %w", err)
		}
		hits, explains = retrieve.SearchExplained(dbp, q, o.Limit, p)
	} else {
		for _, d := range paths.AllProjectDirs() {
			if ppred != nil && !ppred(paths.ProjectCWD(d)) {
				continue
			}
			dbp, _, _, err := index.EnsureIndexed(d, false)
			if err != nil {
				continue
			}
			h, ex := retrieve.SearchExplained(dbp, q, o.Limit, p)
			// Append in lockstep so explains[i] keeps explaining hits[i].
			hits = append(hits, h...)
			explains = append(explains, ex...)
		}
	}

	if o.JSON {
		b, err := render.DebugSearchJSON(hits, explains)
		if err != nil {
			return err
		}
		fmt.Fprint(w, string(b))
		return nil
	}
	render.PrintDebugSearch(w, hits, explains)
	return nil
}
