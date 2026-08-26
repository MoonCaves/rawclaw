package cli

import (
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// locateTagWriteFast probes already-known scope databases without indexing or
// consulting the derived store. A unique source hit is authoritative for the
// write; multiple hits deliberately fall back to guarded lookup so its exact
// ambiguity rendering remains unchanged.
func locateTagWriteFast(session8 string, scope []view.Scope) (dbp, fullSID string, found bool) {
	if len(scope) == 0 {
		hits := paths.ResolveSession(session8)
		if len(hits) != 1 || hits[0].Path == "" || hits[0].CWD == "" {
			return "", "", false
		}
		tdir := paths.ProjectDirOf(hits[0].Path)
		if tdir == "" {
			return "", "", false
		}
		db := index.DBPath(tdir)
		if _, status, err := index.EnsureIndexedTreeSource(db, tdir); err != nil || status != index.IndexFresh {
			return "", "", false
		}
		con, err := store.ConnectRO(db)
		if err != nil {
			return "", "", false
		}
		ids, err := store.SessionsByPrefix(con, session8, false, 2)
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
			if _, status, err := index.EnsureIndexedTreeSource(db, sc.TDir); err != nil || status != index.IndexFresh {
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
