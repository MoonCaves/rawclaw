// Package scopes builds the search-scope list spanning every runtime — the one
// place that knows about the concrete Source adapters. It unions the lazy Claude
// project scopes with eager Codex scopes (each Codex cwd-group pre-ingested into
// its own distinctly-namespaced db) and resolves a scope to its db + cwd, so
// agentproto and cli stay source-agnostic.
package scopes

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/query"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// All returns Claude ∪ Codex ∪ archive scopes, filtered by sourceFilter
// ("" = all, "claude", or "codex"). reindex forces a fresh rebuild of the
// eager (Codex + archive) dbs. Archive scopes are the FOREIGN machine dirs of
// the transcript-archive clone, spliced in so a plain search transparently
// covers other machines' pushed sessions; each carries its Source, so the
// runtime filter applies to them exactly as to local scopes. ctx bounds the
// archive enumeration's git probes (see Archive) — pass the run's watchdog
// context so those children die with the CLI.
// An optional path predicate is applied only to live container CWDs before
// their per-CWD indexes are ensured; Claude, archive, and orphan scopes remain
// in the returned list for their existing later FilterByPath pass.
//
// This union is THE runtime-discovery site: a future runtime (a new source
// tool with its own transcript-dir location) is added here — its known
// location behind a Source adapter, unioned like Claude and Codex below.
// Discovery is location-based only; an arbitrary jsonl-bearing folder never
// enters implicitly (--dir is the explicit opt-in).
func All(ctx context.Context, sourceFilter string, reindex bool, pathPreds ...func(string) bool) []view.Scope {
	var pathPred func(string) bool
	if len(pathPreds) > 0 {
		pathPred = pathPreds[0]
	}
	var out []view.Scope
	for _, reg := range sources.Registered() {
		if sourceFilter != "" && sourceFilter != reg.ID {
			continue
		}
		if reg.ID == "claude" {
			out = append(out, Claude()...)
			continue
		}
		if reg.OptedIn != nil && !reg.OptedIn(sourceFilter) {
			out = append(out, orphanContainerScopes(reg.ID, nil)...)
			continue
		}
		labelFn := reg.Label
		if labelFn == nil {
			labelFn = func(cwd string) string { return defaultContainerLabel(reg.ID, cwd) }
		}
		out = append(out, containerScopes(reg.ID, reg.New(), labelFn, reindex, pathPred)...)
	}
	for _, sc := range Archive(ctx, reindex) {
		if sourceFilter == "" || sc.Source == sourceFilter {
			out = append(out, sc)
		}
	}
	return out
}

// Archive returns the transcript-archive scopes: every foreign machine dir in
// the archive clone, ready to search (see archive.Scopes). An unconfigured
// archive (or one whose clone is absent) yields nil — the zero state costs one
// nil-check. A broken archive config is warned and degrades to local-only
// rather than failing the whole enumeration. ctx bounds the per-machine
// staleness git probes so they die with the caller's watchdog.
func Archive(ctx context.Context, reindex bool) []view.Scope {
	a, err := archive.Load()
	if err != nil {
		slog.Warn("scopes: archive config unreadable; searching local scopes only", "err", err)
		return nil
	}
	if a == nil {
		return nil // feature off
	}
	return a.Scopes(ctx, reindex)
}

// ArchiveLookup returns the archive scopes WITHOUT ingesting or probing
// anything (see archive.LookupScopes): scope DBPs name whatever cache dbs
// earlier searches already built, for cheap point lookups like resolving a
// --resume prefix. Spawns no child processes, hence no ctx.
func ArchiveLookup() []view.Scope {
	a, err := archive.Load()
	if err != nil {
		slog.Warn("scopes: archive config unreadable; searching local scopes only", "err", err)
		return nil
	}
	if a == nil {
		return nil // feature off
	}
	return a.LookupScopes()
}

// Claude returns the union of (a) the lazy live Claude project scopes — TDir set,
// db resolved on demand by Resolve (preserving the original per-project,
// index-at-search-time timing) — and (b) EAGER read-only scopes for orphaned
// index dbs whose source project dir has vanished (D8: the 30-day-purge case).
// Discovery is store-driven, not only disk-driven — the retained rows stay
// reachable even when AllProjectDirs no longer yields their project.
func Claude() []view.Scope {
	dirs := paths.AllProjectDirs()
	out := make([]view.Scope, 0, len(dirs))
	liveDBs := make(map[string]struct{}, len(dirs)) // db paths already covered by a live dir
	for _, d := range dirs {
		out = append(out, view.Scope{Project: paths.ProjectLabel(d), TDir: d, Source: "claude"})
		liveDBs[index.DBPath(d)] = struct{}{}
	}
	out = append(out, orphanClaudeScopes(liveDBs)...)
	return out
}

// orphanClaudeScopes discovers index dbs in the session-search cache dir whose
// Claude source dir is gone and surfaces each as an eager read-only scope (DBP
// set, like a Codex scope) so search/read/list reach the retained rows without
// re-walking a source that no longer exists (D8). Prior art: Zoekt enumerates the
// index shards themselves and reconciles them against the source listing
// (cleanup.go:134-146) — the index is a first-class discovery surface.
//
// liveDBs is the set of db paths already covered by a live project scope; those
// are skipped so a project is never listed twice. Each candidate is reconciled
// against an empty live scan (stamping only_copy_since, deleting tombstoned rows)
// and included only if it still holds >=1 non-tombstoned top-level session — so a
// db whose only sessions were deleted still reads as deleted. Codex dbs (prefix
// "codex-") are left to Codex(), which reindexes them from live discovery.
func orphanClaudeScopes(liveDBs map[string]struct{}) []view.Scope {
	entries, _ := filepath.Glob(filepath.Join(store.CacheDir(), "*.db"))

	var out []view.Scope
	for _, dbp := range entries {
		base := filepath.Base(dbp)
		if index.IsConsolidatedDB(base) {
			continue // the consolidated store is a SUPERSET of these dbs, not a
			// peer of them: listing it here would search every row a second time
		}
		if strings.HasPrefix(base, "codex-") {
			continue // codex dbs are enumerated + reconciled by Codex()
		}
		if strings.HasPrefix(base, "antigravity-") {
			continue // antigravity dbs are enumerated + reconciled by Antigravity()
		}
		if strings.HasPrefix(base, "goose-") {
			continue // goose dbs are enumerated + reconciled by Goose()
		}
		if strings.HasPrefix(base, "pi-") {
			continue // pi dbs are enumerated + reconciled by Pi()
		}
		if strings.HasPrefix(base, "opencode-") {
			continue // opencode dbs are enumerated + reconciled by OpenCode()
		}
		if strings.HasPrefix(base, index.ArchiveDBPrefix) {
			continue // archive-replica dbs are enumerated by Archive(); their
			// live source is the clone's machine dir, never an orphaned project
		}
		if _, covered := liveDBs[dbp]; covered {
			continue // already a live project scope — don't list it twice
		}
		n, err := index.EnsureOrphanReconciled(dbp)
		if err != nil {
			slog.Warn("scopes: orphan reconcile failed", "db", dbp, "err", err)
			continue
		}
		if n <= 0 {
			continue // only tombstoned/empty sessions — reads as deleted
		}
		out = append(out, view.Scope{Project: orphanLabel(base), DBP: dbp, Source: "claude"})
	}
	return out
}

// orphanLabel derives a friendly project label from an index db filename when the
// source dir (whose recorded cwd ProjectLabel would read) is gone. The db name is
// the encoded project dir (Claude's "/"→"-", "."→"-" encoding), so the last
// non-empty "-" segment is the closest recoverable basename — e.g.
// "-tmp-demoproj.db" → "demoproj". Falls back to the whole stem.
func orphanLabel(dbFileName string) string {
	enc := strings.TrimSuffix(dbFileName, ".db")
	parts := strings.Split(enc, "-")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return enc
}

// Codex discovers Codex sessions, groups them by recorded cwd, ingests each
// group into its OWN db (namespaced so it can never collide with a Claude db —
// see index.EnsureIndexedContainers' complete-set contract), and returns eager
// scopes carrying that db + cwd — unioned with orphanCodexScopes (D8: the same
// 30-day-purge store-driven discovery Claude() does via orphanClaudeScopes).
func Codex(reindex bool) []view.Scope {
	if reg := sources.Get("codex"); reg != nil {
		return containerScopes(reg.ID, reg.New(), codexLabel, reindex)
	}
	return nil
}

// RefreshCodexCWD refreshes the Codex index db for a given working dir.
func RefreshCodexCWD(cwd string) {
	if reg := sources.Get("codex"); reg != nil {
		refreshContainerCWD(reg.ID, reg.New(), cwd)
	}
}

func codexDBPath(cwd string) string {
	return containerDBPath("codex", cwd)
}

func codexLabel(cwd string) string {
	return defaultContainerLabel("codex", cwd)
}

func codexOrphanLabel(dbFileName string) string {
	return containerOrphanLabel("codex", dbFileName)
}

// Pi discovers Pi agent sessions, groups them by recorded cwd, ingests each
// group into its OWN db, and returns eager scopes carrying that db + cwd.
func Pi(reindex bool) []view.Scope {
	if reg := sources.Get("pi"); reg != nil {
		return containerScopes(reg.ID, reg.New(), piLabel, reindex)
	}
	return nil
}

// RefreshPiCWD refreshes the Pi index db for a given working dir.
func RefreshPiCWD(cwd string) {
	if reg := sources.Get("pi"); reg != nil {
		refreshContainerCWD(reg.ID, reg.New(), cwd)
	}
}

func piLabel(cwd string) string {
	return defaultContainerLabel("pi", cwd)
}

// OpenCode discovers OpenCode / Crush sessions, groups them by recorded cwd,
// ingests each group into its OWN db, and returns eager scopes carrying that db + cwd.
func OpenCode(reindex bool) []view.Scope {
	if reg := sources.Get("opencode"); reg != nil {
		return containerScopes(reg.ID, reg.New(), opencodeLabel, reindex)
	}
	return nil
}

// RefreshOpenCodeCWD refreshes the OpenCode index db for a given working dir.
func RefreshOpenCodeCWD(cwd string) {
	if reg := sources.Get("opencode"); reg != nil {
		refreshContainerCWD(reg.ID, reg.New(), cwd)
	}
}

func opencodeLabel(cwd string) string {
	return defaultContainerLabel("opencode", cwd)
}

// Resolve returns a scope's db path and ensure-status. A pre-ensured scope
// (DBP set, e.g. Codex) is (DBP, IndexFresh, nil); a lazy Claude scope ensures
// its TDir now, exactly as the old inline index.EnsureIndexed(sc.TDir) did.
// A scope flagged Stale (a replica lagging its origin machine, or an index pass
// that hit lock contention) resolves to its db with IndexStale, feeding the
// existing stale-fallback posture: searched, served, and reported as possibly
// incomplete.
func Resolve(sc view.Scope, reindex bool) (string, index.IndexStatus, error) {
	if sc.Stale && sc.DBP != "" {
		return sc.DBP, index.IndexStale, nil
	}
	if sc.DBP != "" {
		return sc.DBP, index.IndexFresh, nil
	}
	dbp, _, status, err := index.EnsureIndexed(sc.TDir, reindex)
	if sc.Stale && status == index.IndexFresh {
		status = index.IndexStale
	}
	return dbp, status, err
}

// CWD returns the working dir used for path filtering: the scope's own CWD if
// set (Codex), else derived from the Claude transcript dir.
func CWD(sc view.Scope) string {
	if sc.CWD != "" {
		return sc.CWD
	}
	return paths.ProjectCWD(sc.TDir)
}

// FilterByPath keeps only the scopes whose working dir satisfies the
// include/exclude path predicate — the structural Scope filter behind
// --include-path / --exclude-path. Both patterns empty returns scope unchanged.
//
// It lives here, next to CWD, because more than one shape needs it (search and
// the no-query browse) and it must prune BEFORE Resolve: a dropped scope is one
// fewer db to index and open, so narrowing a run also makes it cheaper.
func FilterByPath(scope []view.Scope, include, exclude string) []view.Scope {
	if include == "" && exclude == "" {
		return scope
	}
	pred := query.PathPredicate(include, exclude)
	out := make([]view.Scope, 0, len(scope))
	for _, sc := range scope {
		if pred(CWD(sc)) {
			out = append(out, sc)
		}
	}
	return out
}

// FilterByProjectDir narrows scopes to those belonging to the project at dir.
// If dir is inside a Git repository (worktree or primary), it matches any scope
// whose working directory shares the same Git root repository.
// If not a git repository, it matches by exact normalized working directory.
func FilterByProjectDir(scopes []view.Scope, dir string) []view.Scope {
	if dir == "" {
		return scopes
	}
	targetDir := paths.Realpath(paths.ExpandHome(dir))
	gitRoot := paths.GitRoot(targetDir)

	var out []view.Scope
	seen := make(map[string]struct{})
	for _, sc := range scopes {
		scCWD := paths.Realpath(CWD(sc))
		matched := false
		if gitRoot != "" {
			if scGitRoot := paths.GitRoot(scCWD); scGitRoot != "" && scGitRoot == gitRoot {
				matched = true
			}
		}
		if !matched && scCWD != "" && scCWD == targetDir {
			matched = true
		}
		if matched {
			key := sc.Project + "|" + sc.TDir + "|" + sc.DBP
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				out = append(out, sc)
			}
		}
	}
	return out
}
