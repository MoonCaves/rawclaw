package scopes

import (
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/goose"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// Goose discovers Goose sessions, groups them by recorded cwd,
// ingests each group into its own db (namespaced with prefix "goose-"),
// and returns eager scopes carrying that db + cwd — unioned with
// orphanGooseScopes for retained history after transcript purge.
func Goose(reindex bool) []view.Scope {
	a := goose.New()
	containers, err := a.Discover()
	if err != nil {
		slog.Warn("scopes: goose discover failed", "err", err)
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
		dbp := gooseDBPath(cwd)
		liveDBs[dbp] = struct{}{}
		if _, _, ierr := index.EnsureIndexedContainers(dbp, reindex, byCWD[cwd], a.Messages, goose.Registration().ID, ""); ierr != nil {
			slog.Warn("scopes: goose index failed", "cwd", cwd, "err", ierr)
		}
		out = append(out, view.Scope{Project: gooseLabel(cwd), DBP: dbp, CWD: cwd, Source: goose.ID})
	}
	out = append(out, orphanGooseScopes(liveDBs)...)
	return out
}

// RefreshGooseCWD refreshes the Goose index db for a given working dir.
func RefreshGooseCWD(cwd string) {
	a := goose.New()
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
	dbp := gooseDBPath(cwd)
	_, _, _ = index.EnsureIndexedContainers(dbp, false, matched, a.Messages, goose.Registration().ID, "")
}

// orphanGooseScopes discovers Goose index dbs in the cache dir
// whose live cwd group has vanished and surfaces each as an eager read-only scope.
func orphanGooseScopes(liveDBs map[string]struct{}) []view.Scope {
	entries, _ := filepath.Glob(filepath.Join(store.CacheDir(), "goose-*.db"))
	sort.Strings(entries)

	var out []view.Scope
	for _, dbp := range entries {
		if _, covered := liveDBs[dbp]; covered {
			continue
		}
		n, err := index.EnsureOrphanReconciled(dbp)
		if err != nil {
			slog.Warn("scopes: goose orphan reconcile failed", "db", dbp, "err", err)
			continue
		}
		if n <= 0 {
			continue
		}
		out = append(out, view.Scope{Project: gooseOrphanLabel(filepath.Base(dbp)), DBP: dbp, Source: goose.ID})
	}
	return out
}

// gooseDBPath returns a cache db path for a Goose cwd group, prefixed "goose-".
func gooseDBPath(cwd string) string {
	key := "goose-" + encodeCWD(cwd) + "-" + cwdHash(cwd)
	return index.DBPath(key)
}

// gooseLabel formats a human project label for a Goose scope.
func gooseLabel(cwd string) string {
	if cwd == "" {
		return "goose (unscoped)"
	}
	return filepath.Base(cwd)
}

// gooseOrphanLabel extracts a label from a goose-*.db filename.
func gooseOrphanLabel(dbFileName string) string {
	enc := strings.TrimSuffix(dbFileName, ".db")
	enc = strings.TrimPrefix(enc, "goose-")
	if i := strings.LastIndex(enc, "-"); i >= 0 && isHex8(enc[i+1:]) {
		enc = enc[:i]
	}
	return orphanLabel(enc)
}
