// Package scopes builds the search-scope list spanning every runtime — the one
// place that knows about the concrete Source adapters. It unions the lazy Claude
// project scopes with eager Codex scopes (each Codex cwd-group pre-ingested into
// its own distinctly-namespaced db) and resolves a scope to its db + cwd, so
// agentproto and cli stay source-agnostic.
package scopes

import (
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// All returns Claude ∪ Codex ∪ archive scopes, filtered by sourceFilter
// ("" = all, "claude", or "codex"). reindex forces a fresh rebuild of the
// eager (Codex + archive) dbs. Archive scopes are the FOREIGN machine dirs of
// the transcript-archive clone, spliced in so a plain search transparently
// covers other machines' pushed sessions; each carries its Source, so the
// runtime filter applies to them exactly as to local scopes.
func All(sourceFilter string, reindex bool) []view.Scope {
	var out []view.Scope
	if sourceFilter == "" || sourceFilter == "claude" {
		out = append(out, Claude()...)
	}
	if sourceFilter == "" || sourceFilter == "codex" {
		out = append(out, Codex(reindex)...)
	}
	for _, sc := range Archive(reindex) {
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
// rather than failing the whole enumeration.
func Archive(reindex bool) []view.Scope {
	a, err := archive.Load()
	if err != nil {
		slog.Warn("scopes: archive config unreadable; searching local scopes only", "err", err)
		return nil
	}
	if a == nil {
		return nil // feature off
	}
	return a.Scopes(reindex)
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
// against an empty live scan (stamping missing_since, deleting tombstoned rows)
// and included only if it still holds >=1 non-tombstoned top-level session — so a
// db whose only sessions were deleted still reads as deleted. Codex dbs (prefix
// "codex-") are left to Codex(), which reindexes them from live discovery.
func orphanClaudeScopes(liveDBs map[string]struct{}) []view.Scope {
	entries, _ := filepath.Glob(filepath.Join(store.CacheDir(), "*.db"))
	sort.Strings(entries)

	var out []view.Scope
	for _, dbp := range entries {
		base := filepath.Base(dbp)
		if strings.HasPrefix(base, "codex-") {
			continue // codex dbs are enumerated + reconciled by Codex()
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
// Discover erroring or finding zero live containers does NOT skip the orphan
// scan: a cwd's rollouts can vanish entirely while its retained db still holds
// searchable history, and that must stay reachable even when Discover comes
// back empty.
func Codex(reindex bool) []view.Scope {
	a := codex.New()
	containers, err := a.Discover()
	if err != nil {
		slog.Warn("scopes: codex discover failed", "err", err)
		// containers is nil; fall through so the orphan scan below still runs.
	}

	byCWD := map[string][]source.Container{}
	for _, c := range containers {
		byCWD[c.CWD] = append(byCWD[c.CWD], c)
	}
	cwds := make([]string, 0, len(byCWD))
	for k := range byCWD {
		cwds = append(cwds, k)
	}
	sort.Strings(cwds)

	out := make([]view.Scope, 0, len(cwds))
	liveDBs := make(map[string]struct{}, len(cwds)) // db paths already covered by a live cwd group
	for _, cwd := range cwds {
		dbp := codexDBPath(cwd)
		liveDBs[dbp] = struct{}{}
		if _, _, ierr := index.EnsureIndexedContainers(dbp, reindex, byCWD[cwd], a.Messages, codex.Registration().ID, ""); ierr != nil {
			slog.Warn("scopes: codex index failed", "cwd", cwd, "err", ierr)
			// The db path may still hold a prior good index; include the scope so
			// search can open it read-only and degrade gracefully.
		}
		out = append(out, view.Scope{Project: codexLabel(cwd), DBP: dbp, CWD: cwd, Source: "codex"})
	}
	out = append(out, orphanCodexScopes(liveDBs)...)
	return out
}

// orphanCodexScopes discovers Codex index dbs in the cache dir whose live cwd
// group has vanished entirely and surfaces each as an eager read-only scope,
// mirroring orphanClaudeScopes: once every rollout for a cwd is purged, that
// cwd drops out of Discover()'s byCWD grouping and Codex() would otherwise
// never open its retained db again (the exact gap D8 closed for Claude).
//
// liveDBs is the set of db paths already covered by a live cwd group this
// call; those are skipped so a cwd is never listed twice. Each candidate is
// reconciled against an empty live scan (stamping missing_since, deleting
// tombstoned rows) and included only if it still holds >=1 non-tombstoned
// top-level session — so a db whose only sessions were deleted still reads as
// deleted.
func orphanCodexScopes(liveDBs map[string]struct{}) []view.Scope {
	entries, _ := filepath.Glob(filepath.Join(store.CacheDir(), "codex-*.db"))
	sort.Strings(entries)

	var out []view.Scope
	for _, dbp := range entries {
		if _, covered := liveDBs[dbp]; covered {
			continue // already a live cwd scope — don't list it twice
		}
		n, err := index.EnsureOrphanReconciled(dbp)
		if err != nil {
			slog.Warn("scopes: codex orphan reconcile failed", "db", dbp, "err", err)
			continue
		}
		if n <= 0 {
			continue // only tombstoned/empty sessions — reads as deleted
		}
		out = append(out, view.Scope{Project: codexOrphanLabel(filepath.Base(dbp)), DBP: dbp, Source: "codex"})
	}
	return out
}

// codexOrphanLabel derives a friendly label from a Codex db filename when the
// live cwd (whose codexLabel would read) is gone. The stem is
// "codex-<encodeCWD(cwd)>-<8-hex-hash>" (see codexDBPath): strip the "codex-"
// prefix and the trailing injective hash, then reuse orphanLabel's
// last-non-empty "-" segment logic on what's left — otherwise that logic
// would return the opaque hash segment instead of a readable cwd fragment.
func codexOrphanLabel(dbFileName string) string {
	enc := strings.TrimSuffix(dbFileName, ".db")
	enc = strings.TrimPrefix(enc, "codex-")
	if i := strings.LastIndex(enc, "-"); i >= 0 && isHex8(enc[i+1:]) {
		enc = enc[:i]
	}
	return orphanLabel(enc)
}

// isHex8 reports whether s is exactly 8 lowercase-hex characters — the shape
// cwdHash always produces, used to detect and strip codexDBPath's
// disambiguation suffix from a db filename.
func isHex8(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// Resolve returns a scope's db path and ensure-status. A pre-ensured scope
// (DBP set, e.g. Codex) is (DBP, IndexFresh, nil); a lazy Claude scope ensures
// its TDir now, exactly as the old inline index.EnsureIndexed(sc.TDir) did.
// A scope flagged Stale (a replica lagging its origin machine) resolves to its
// db with IndexStale, feeding the existing stale-fallback posture: searched,
// served, and reported as possibly incomplete.
func Resolve(sc view.Scope, reindex bool) (string, index.IndexStatus, error) {
	if sc.Stale && sc.DBP != "" {
		return sc.DBP, index.IndexStale, nil
	}
	if sc.DBP != "" {
		return sc.DBP, index.IndexFresh, nil
	}
	dbp, _, status, err := index.EnsureIndexed(sc.TDir, reindex)
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

// codexDBPath returns a cache db path for a Codex cwd group, prefixed "codex-"
// so it never collides with the Claude project db for the same cwd. An empty
// cwd groups under a stable "unknown" key.
//
// The readable slug from encodeCWD is lossy ('/', '.', '-' all fold to '-'), so
// two distinct cwds can collapse onto one slug. That would put two cwd groups on
// one db and violate index.EnsureIndexedContainers' complete-set contract — the
// groups would cross-prune each other. We append a short hash of the FULL cwd so
// the key stays human-readable but is injective (distinct cwd => distinct db).
func codexDBPath(cwd string) string {
	key := "codex-" + encodeCWD(cwd) + "-" + cwdHash(cwd)
	return index.DBPath(key)
}

// cwdHash is a short, stable, collision-resistant tag of the full cwd, used to
// disambiguate cwds that share a lossy encodeCWD slug. Deterministic across runs
// so a cwd group keeps the same db.
func cwdHash(cwd string) string {
	sum := sha1.Sum([]byte(cwd))
	return hex.EncodeToString(sum[:])[:8]
}

// codexLabel is a friendly project label for a Codex cwd group.
func codexLabel(cwd string) string {
	if cwd == "" {
		return "codex"
	}
	return filepath.Base(strings.TrimRight(cwd, "/"))
}

// encodeCWD flattens a cwd into a slash-free db-name segment: "/" and "." → "-".
// "" yields "unknown" so empty-cwd sessions share one stable db.
func encodeCWD(cwd string) string {
	if cwd == "" {
		return "unknown"
	}
	return strings.NewReplacer("/", "-", ".", "-").Replace(cwd)
}
