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
	"github.com/MoonCaves/rawclaw/internal/source/codex"
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
//
// This union is THE runtime-discovery site: a future runtime (a new source
// tool with its own transcript-dir location) is added here — its known
// location behind a Source adapter, unioned like Claude and Codex below.
// Discovery is location-based only; an arbitrary jsonl-bearing folder never
// enters implicitly (--dir is the explicit opt-in).
func All(ctx context.Context, sourceFilter string, reindex bool) []view.Scope {
	var out []view.Scope
	if sourceFilter == "" || sourceFilter == "claude" {
		out = append(out, Claude()...)
	}
	if sourceFilter == "" || sourceFilter == "codex" {
		out = append(out, Codex(reindex)...)
	}
	if sourceFilter == "" || sourceFilter == "antigravity" {
		out = append(out, Antigravity(reindex)...)
	}
	if sourceFilter == "" || sourceFilter == "goose" {
		out = append(out, Goose(reindex)...)
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
	return containerScopes(codex.Registration().ID, codex.New(), codexLabel, reindex)
}

// RefreshCodexCWD refreshes the Codex index db for a given working dir.
func RefreshCodexCWD(cwd string) {
	refreshContainerCWD(codex.Registration().ID, codex.New(), cwd)
}

func codexDBPath(cwd string) string {
	return containerDBPath(codex.Registration().ID, cwd)
}

func codexLabel(cwd string) string {
	return defaultContainerLabel(codex.Registration().ID, cwd)
}

func codexOrphanLabel(dbFileName string) string {
	return containerOrphanLabel(codex.Registration().ID, dbFileName)
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
