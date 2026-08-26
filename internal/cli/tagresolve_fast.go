package cli

import (
	"os"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// refreshTagWriteTDir refreshes only the catalog-resolved session in a TDir.
// The source/container path is the exact-session refresh seam; using it here
// avoids walking every JSONL file in the project tree on the tag-write path.
func refreshTagWriteTDir(dbp, tdir, session8 string) bool {
	targetDir, err := os.Stat(tdir)
	if err != nil {
		return false
	}
	prefix := agentproto.NormalizeSessionArg(session8)
	for _, hit := range paths.ResolveSession(prefix) {
		if hit.Path == "" || !strings.HasPrefix(hit.SessionID, prefix) {
			continue
		}
		projectDir := paths.ProjectDirOf(hit.Path)
		projectInfo, err := os.Stat(projectDir)
		if err != nil || !os.SameFile(targetDir, projectInfo) {
			continue
		}
		for _, reg := range sources.Registered() {
			if reg.New == nil || reg.Detect == nil || !reg.Detect(hit.Path) {
				continue
			}
			adapter := reg.New()
			if adapter == nil {
				continue
			}
			_, status, err := index.EnsureIndexedContainers(dbp, false, []source.Container{{
				ID: hit.SessionID, Path: hit.Path, CWD: hit.CWD,
			}}, adapter.Messages, reg.ID, "")
			return err == nil && status == index.IndexFresh
		}
	}
	return false
}

// locateTagWriteFast probes already-known scope databases without indexing or
// consulting the derived store. A unique source hit is authoritative for the
// write; multiple hits deliberately fall back to guarded lookup so its exact
// ambiguity rendering remains unchanged.
func locateTagWriteFast(session8 string, scope []view.Scope) (dbp, fullSID string, found bool) {
	if scope == nil {
		normalized := agentproto.NormalizeSessionArg(session8)
		hits := paths.ResolveSession(normalized)
		if len(hits) != 1 || hits[0].Path == "" {
			return "", "", false
		}
		tdir := paths.ProjectDirOf(hits[0].Path)
		if tdir == "" {
			return "", "", false
		}
		db := index.DBPath(tdir)
		if !refreshTagWriteTDir(db, tdir, normalized) {
			return "", "", false
		}
		con, err := store.ConnectRO(db)
		if err != nil {
			return "", "", false
		}
		ids, err := store.SessionsByPrefix(con, normalized, false, 2)
		con.Close()
		if err != nil || len(ids) != 1 || ids[0] != hits[0].SessionID {
			return "", "", false
		}
		return db, ids[0], true
	}
	var hits int
	for _, sc := range scope {
		db := sc.DBP
		if db == "" && sc.TDir != "" {
			db = index.DBPath(sc.TDir)
			if !refreshTagWriteTDir(db, sc.TDir, session8) {
				continue
			}
		}
		if db == "" {
			continue
		}
		con, err := store.ConnectRO(db)
		if err != nil {
			continue
		}
		ids, queryErr := store.SessionsByPrefix(con, session8, false, 2)
		_ = con.Close()
		if queryErr != nil || len(ids) == 0 {
			continue
		}
		if len(ids) > 1 {
			return "", "", false
		}
		hits++
		if hits > 1 && fullSID != ids[0] {
			return "", "", false
		}
		dbp, fullSID = db, ids[0]
	}
	return dbp, fullSID, hits == 1
}
