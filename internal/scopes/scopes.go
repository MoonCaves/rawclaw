// Package scopes builds the search-scope list spanning every runtime — the one
// place that knows about the concrete Source adapters. It unions the lazy Claude
// project scopes with eager Codex scopes (each Codex cwd-group pre-ingested into
// its own distinctly-namespaced db) and resolves a scope to its db + cwd, so
// agentproto and cli stay source-agnostic.
package scopes

import (
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// All returns Claude ∪ Codex scopes, filtered by sourceFilter ("" = all,
// "claude", or "codex"). reindex forces a fresh rebuild of Codex dbs.
func All(sourceFilter string, reindex bool) []view.Scope {
	var out []view.Scope
	if sourceFilter == "" || sourceFilter == "claude" {
		out = append(out, Claude()...)
	}
	if sourceFilter == "" || sourceFilter == "codex" {
		out = append(out, Codex(reindex)...)
	}
	return out
}

// Claude returns the lazy Claude project scopes: TDir set, db resolved on demand
// by Resolve (preserving the original per-project, index-at-search-time timing).
func Claude() []view.Scope {
	dirs := paths.AllProjectDirs()
	out := make([]view.Scope, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, view.Scope{Project: paths.ProjectLabel(d), TDir: d, Source: "claude"})
	}
	return out
}

// Codex discovers Codex sessions, groups them by recorded cwd, ingests each
// group into its OWN db (namespaced so it can never collide with a Claude db —
// see index.EnsureIndexedContainers' complete-set contract), and returns eager
// scopes carrying that db + cwd. Returns nil when Codex has no sessions.
func Codex(reindex bool) []view.Scope {
	a := codex.New()
	containers, err := a.Discover()
	if err != nil {
		slog.Warn("scopes: codex discover failed", "err", err)
		return nil
	}
	if len(containers) == 0 {
		return nil
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
	for _, cwd := range cwds {
		dbp := codexDBPath(cwd)
		if _, _, ierr := index.EnsureIndexedContainers(dbp, reindex, byCWD[cwd], a.Messages); ierr != nil {
			slog.Warn("scopes: codex index failed", "cwd", cwd, "err", ierr)
			// The db path may still hold a prior good index; include the scope so
			// search can open it read-only and degrade gracefully.
		}
		out = append(out, view.Scope{Project: codexLabel(cwd), DBP: dbp, CWD: cwd, Source: "codex"})
	}
	return out
}

// Resolve returns a scope's db path and ensure-status. A pre-ensured scope
// (DBP set, e.g. Codex) is (DBP, IndexFresh, nil); a lazy Claude scope ensures
// its TDir now, exactly as the old inline index.EnsureIndexed(sc.TDir) did.
func Resolve(sc view.Scope, reindex bool) (string, index.IndexStatus, error) {
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
func codexDBPath(cwd string) string {
	key := "codex-" + encodeCWD(cwd)
	return index.DBPath(key)
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
