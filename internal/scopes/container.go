package scopes

import (
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// containerScopes discovers sessions from a ContainerAdapter, groups them by
// recorded cwd, ingests each group into its own db (namespaced with "<sourceID>-"),
// and returns eager scopes carrying that db + cwd — unioned with orphanContainerScopes
// for retained history after transcript purge. An optional path predicate drops
// nonmatching live CWDs before any per-CWD index is ensured; orphan discovery is
// deliberately unchanged.
func containerScopes(sourceID string, adapter source.Source, labelFn func(string) string, reindex bool, pathPreds ...func(string) bool) []view.Scope {
	var pathPred func(string) bool
	if len(pathPreds) > 0 {
		pathPred = pathPreds[0]
	}
	containers, err := adapter.Discover()
	if err != nil {
		slog.Warn("scopes: "+sourceID+" discover failed", "err", err)
	}

	byCWD := map[string][]source.Container{}
	for _, c := range containers {
		if pathPred != nil && !pathPred(c.CWD) {
			continue
		}
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
		dbp := containerDBPath(sourceID, cwd)
		liveDBs[dbp] = struct{}{}
		_, istatus, ierr := index.EnsureIndexedContainers(dbp, reindex, byCWD[cwd], adapter.Messages, sourceID, "")
		if ierr != nil {
			slog.Warn("scopes: "+sourceID+" index failed", "cwd", cwd, "err", ierr)
		}
		out = append(out, view.Scope{
			Project: labelFn(cwd),
			DBP:     dbp,
			CWD:     cwd,
			Source:  sourceID,
			Stale:   istatus == index.IndexStale || ierr != nil,
		})
	}
	out = append(out, orphanContainerScopes(sourceID, liveDBs)...)
	return out
}

// RefreshCWD refreshes the index db for a given working dir and Source.
func RefreshCWD(sourceID string, adapter source.Source, cwd string) {
	refreshContainerCWD(sourceID, adapter, cwd)
}

// refreshContainerCWD refreshes the index db for a given working dir and Source.
func refreshContainerCWD(sourceID string, adapter source.Source, cwd string) {
	containers, err := adapter.Discover()
	if err != nil || len(containers) == 0 {
		return
	}
	cleanCWD := filepath.Clean(cwd)
	var matched []source.Container
	for _, c := range containers {
		if c.CWD == cwd || (cwd != "" && filepath.Clean(c.CWD) == cleanCWD) {
			matched = append(matched, c)
		}
	}
	if len(matched) == 0 {
		return
	}
	dbp := containerDBPath(sourceID, cwd)
	if _, _, ierr := index.EnsureIndexedContainers(dbp, false, matched, adapter.Messages, sourceID, ""); ierr != nil {
		slog.Debug("scopes: "+sourceID+" current-cwd refresh failed", "cwd", cwd, "err", ierr)
	}
}

// orphanContainerScopes discovers index dbs in the cache dir for a given container
// source whose live cwd group has vanished and surfaces each as an eager read-only scope.
func orphanContainerScopes(sourceID string, liveDBs map[string]struct{}) []view.Scope {
	entries, _ := filepath.Glob(filepath.Join(store.CacheDir(), sourceID+"-*.db"))

	var out []view.Scope
	for _, dbp := range entries {
		if _, covered := liveDBs[dbp]; covered {
			continue
		}
		n, err := index.EnsureOrphanReconciled(dbp)
		if err != nil {
			slog.Warn("scopes: "+sourceID+" orphan reconcile failed", "db", dbp, "err", err)
			continue
		}
		if n <= 0 {
			continue
		}
		out = append(out, view.Scope{Project: containerOrphanLabel(sourceID, filepath.Base(dbp)), DBP: dbp, Source: sourceID})
	}
	return out
}

// containerDBPath returns a cache db path for a container source cwd group.
func containerDBPath(sourceID, cwd string) string {
	key := sourceID + "-" + encodeCWD(cwd) + "-" + cwdHash(cwd)
	return index.DBPath(key)
}

// defaultContainerLabel returns a friendly label for a container cwd group.
func defaultContainerLabel(sourceID, cwd string) string {
	if cwd == "" {
		return sourceID
	}
	return filepath.Base(strings.TrimRight(cwd, "/"))
}

// containerOrphanLabel derives a friendly label from a container db filename.
func containerOrphanLabel(sourceID, dbFileName string) string {
	enc := strings.TrimSuffix(dbFileName, ".db")
	enc = strings.TrimPrefix(enc, sourceID+"-")
	if i := strings.LastIndex(enc, "-"); i >= 0 && isHex8(enc[i+1:]) {
		enc = enc[:i]
	}
	return orphanLabel(enc)
}

// cwdHash is a short, stable, collision-resistant tag of the full cwd.
func cwdHash(cwd string) string {
	sum := sha1.Sum([]byte(cwd))
	return hex.EncodeToString(sum[:])[:8]
}

// encodeCWD flattens a cwd into a slash-free db-name segment.
func encodeCWD(cwd string) string {
	if cwd == "" {
		return "unknown"
	}
	return strings.NewReplacer("/", "-", ".", "-").Replace(cwd)
}

// isHex8 reports whether s is exactly 8 lowercase-hex characters.
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
