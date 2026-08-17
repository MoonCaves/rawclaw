package scopes

import (
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/antigravity"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// Antigravity discovers Antigravity sessions, groups them by recorded cwd,
// ingests each group into its own db (namespaced with prefix "antigravity-"),
// and returns eager scopes carrying that db + cwd — unioned with
// orphanAntigravityScopes for retained history after transcript purge.
func Antigravity(reindex bool) []view.Scope {
	a := antigravity.New()
	containers, err := a.Discover()
	if err != nil {
		slog.Warn("scopes: antigravity discover failed", "err", err)
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
	liveDBs := make(map[string]struct{}, len(cwds))
	for _, cwd := range cwds {
		dbp := antigravityDBPath(cwd)
		liveDBs[dbp] = struct{}{}
		if _, _, ierr := index.EnsureIndexedContainers(dbp, reindex, byCWD[cwd], a.Messages, antigravity.Registration().ID, ""); ierr != nil {
			slog.Warn("scopes: antigravity index failed", "cwd", cwd, "err", ierr)
		}
		out = append(out, view.Scope{Project: antigravityLabel(cwd), DBP: dbp, CWD: cwd, Source: antigravity.ID})
	}
	out = append(out, orphanAntigravityScopes(liveDBs)...)
	return out
}

// RefreshAntigravityCWD refreshes the Antigravity index db for a given working dir.
func RefreshAntigravityCWD(cwd string) {
	a := antigravity.New()
	containers, err := a.Discover()
	if err != nil || len(containers) == 0 {
		return
	}
	var matched []source.Container
	for _, c := range containers {
		if c.CWD == cwd || (cwd != "" && filepath.Clean(c.CWD) == filepath.Clean(cwd)) {
			matched = append(matched, c)
		}
	}
	if len(matched) == 0 {
		return
	}
	dbp := antigravityDBPath(cwd)
	_, _, _ = index.EnsureIndexedContainers(dbp, false, matched, a.Messages, antigravity.Registration().ID, "")
}

// orphanAntigravityScopes discovers Antigravity index dbs in the cache dir
// whose live cwd group has vanished and surfaces each as an eager read-only scope.
func orphanAntigravityScopes(liveDBs map[string]struct{}) []view.Scope {
	entries, _ := filepath.Glob(filepath.Join(store.CacheDir(), "antigravity-*.db"))
	sort.Strings(entries)

	var out []view.Scope
	for _, dbp := range entries {
		if _, covered := liveDBs[dbp]; covered {
			continue
		}
		n, err := index.EnsureOrphanReconciled(dbp)
		if err != nil {
			slog.Warn("scopes: antigravity orphan reconcile failed", "db", dbp, "err", err)
			continue
		}
		if n <= 0 {
			continue
		}
		out = append(out, view.Scope{Project: antigravityOrphanLabel(filepath.Base(dbp)), DBP: dbp, Source: antigravity.ID})
	}
	return out
}

// antigravityDBPath returns a cache db path for an Antigravity cwd group, prefixed "antigravity-".
func antigravityDBPath(cwd string) string {
	key := "antigravity-" + encodeCWD(cwd) + "-" + cwdHash(cwd)
	return index.DBPath(key)
}

// antigravityLabel is a friendly project label for an Antigravity cwd group.
func antigravityLabel(cwd string) string {
	if cwd == "" {
		return "antigravity"
	}
	return filepath.Base(strings.TrimRight(cwd, "/"))
}

// antigravityOrphanLabel derives a friendly label from an Antigravity db filename.
func antigravityOrphanLabel(dbFileName string) string {
	enc := strings.TrimSuffix(dbFileName, ".db")
	enc = strings.TrimPrefix(enc, "antigravity-")
	if i := strings.LastIndex(enc, "-"); i >= 0 && isHex8(enc[i+1:]) {
		enc = enc[:i]
	}
	return orphanLabel(enc)
}
