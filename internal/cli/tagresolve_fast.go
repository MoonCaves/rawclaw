package cli

import (
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// locateTagWriteFast probes already-known scope databases without indexing or
// consulting the derived store. A unique source hit is authoritative for the
// write; multiple hits deliberately fall back to guarded lookup so its exact
// ambiguity rendering remains unchanged.
func locateTagWriteFast(session8 string, scope []view.Scope) (dbp, fullSID string, found bool) {
	if len(scope) == 0 {
		return "", "", false
	}
	var hits int
	for _, sc := range scope {
		db := sc.DBP
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
