package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

func refreshTagWriteTDir(tdir, session8 string) (dbp, fullSID string, found bool) {
	prefix := agentproto.NormalizeSessionArg(session8)
	entries, err := os.ReadDir(tdir)
	if err != nil {
		return "", "", false
	}
	var path, sid string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		candidate := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !strings.HasPrefix(candidate, prefix) {
			continue
		}
		if sid != "" {
			return "", "", false
		}
		sid, path = candidate, filepath.Join(tdir, entry.Name())
	}
	if sid == "" {
		return "", "", false
	}

	var reg source.Registration
	for _, candidate := range sources.Registered() {
		if candidate.Detect != nil && candidate.Detect(path) {
			reg = candidate
			break
		}
	}
	if reg.ID == "" {
		for _, candidate := range sources.Registered() {
			if candidate.ID == "claude" {
				reg = candidate
				break
			}
		}
	}
	if reg.ID == "" || reg.New == nil {
		return "", "", false
	}
	adapter := reg.New()
	if adapter == nil {
		return "", "", false
	}
	c := source.Container{
		ID: sid, Path: path, CWD: paths.FileCWD(path),
	}
	refreshDB := index.RefreshDBPath(reg.ID, sid, path)
	_, err = index.PrepareFreshContainer(refreshDB, c, adapter.Messages, reg.ID)
	return refreshDB, sid, err == nil
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
			var candidateDB string
			candidateDB, candidateSID, ok = refreshTagWriteTDir(sc.TDir, session8)
			if !ok {
				continue
			}
			hits++
			if hits > 1 && fullSID != candidateSID {
				return "", "", false
			}
			dbp, fullSID = candidateDB, candidateSID
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
