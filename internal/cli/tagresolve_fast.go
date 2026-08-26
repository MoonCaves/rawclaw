package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// refreshTagWriteTDir refreshes only the matching top-level container inside
// tdir. It deliberately uses the explicit directory as the discovery scope;
// catalog and global source discovery are not part of this path.
func refreshTagWriteTDir(tdir, session8 string) (dbp, fullSID string, ok bool) {
	prefix := agentproto.NormalizeSessionArg(session8)
	db := index.DBPath(tdir)
	if match, found := locateTagWriteTDirDB(db, tdir, prefix); found {
		return refreshTagWriteMatch(match)
	}

	dir := realpathExpand(tdir)
	var matches []tagSourceMatch
	for _, path := range paths.ContainedJSONL(dir) {
		if !pathInDir(path, dir) {
			continue
		}
		sid, isSubagent, parent := provenance.SessionIDFor(path, dir)
		if isSubagent != 0 || !strings.HasPrefix(sid, prefix) {
			continue
		}
		for _, reg := range sources.Registered() {
			if reg.New == nil || reg.Detect == nil || !reg.Detect(path) {
				continue
			}
			adapter := reg.New()
			if adapter == nil {
				continue
			}
			matches = append(matches, tagSourceMatch{
				registration: reg,
				adapter:      adapter,
				container: source.Container{
					ID: sid, Path: path, CWD: paths.ProjectCWD(dir),
					IsSubagent: false, ParentID: parent,
				},
			})
		}
	}
	if len(matches) != 1 {
		return "", "", false
	}
	return refreshTagWriteMatch(matches[0])
}

func locateTagWriteTDirDB(dbp, tdir, prefix string) (tagSourceMatch, bool) {
	if dbp == "" {
		return tagSourceMatch{}, false
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return tagSourceMatch{}, false
	}
	ids, err := store.SessionsByPrefix(con, prefix, false, 2)
	_ = con.Close()
	if err != nil || len(ids) != 1 {
		return tagSourceMatch{}, false
	}
	match, ok := locatedTagSource(dbp, ids[0], sources.Registered())
	if !ok || !pathInDir(match.container.Path, realpathExpand(tdir)) {
		return tagSourceMatch{}, false
	}
	return match, true
}

func refreshTagWriteMatch(match tagSourceMatch) (string, string, bool) {
	dbp := index.RefreshDBPath(match.registration.ID, match.container.ID, match.container.Path)
	_, err := index.PrepareFreshContainer(dbp, match.container, match.adapter.Messages, match.registration.ID)
	return dbp, match.container.ID, err == nil
}

func pathInDir(path, dir string) bool {
	rp := realpathExpand(path)
	rel, err := filepath.Rel(strings.TrimRight(realpathExpand(dir), string(os.PathSeparator)), rp)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
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
		if _, status, err := index.EnsureIndexedTreeSource(db, tdir); err != nil || status != index.IndexFresh {
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
			var ok bool
			var candidateSID string
			db, candidateSID, ok = refreshTagWriteTDir(sc.TDir, session8)
			if !ok {
				continue
			}
			if hits > 0 && fullSID != candidateSID {
				return "", "", false
			}
			hits++
			dbp = db
			fullSID = candidateSID
			continue
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
