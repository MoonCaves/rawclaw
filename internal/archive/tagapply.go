package archive

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// ingestForeignTags applies pulled cross-machine tags into the foreign scope dbs
// just enumerated (their messages are already indexed, so the topic rows have an
// anchor). It runs under the sync lock the ingest path already holds. Gated by
// tagsChangedSinceIngest (unless reindex) so a steady-state search pays only a
// few stats, not a walk of every session. Conflicts (≥2 distinct real sets) are
// recorded to the state file AND emitted as a named log line. The losing
// sets are never lost: their tag files persist in the archive; this only chooses
// what the local index surfaces.
func (a *Archive) ingestForeignTags(scopes []view.Scope, reindex bool) {
	if !reindex && !tagIngestDue() {
		return
	}
	machineDirs := a.allMachineDirs()
	var conflicts []string
	skipped := false
	for _, sc := range scopes {
		if sc.DBP == "" {
			continue
		}
		con, err := store.ConnectRW(sc.DBP)
		if err != nil {
			skipped = true // an unreadable (e.g. briefly locked) scope: retry it later
			continue
		}
		sids, _ := store.SessionIDsIn(con)
		for _, sid := range sids {
			files := gatherTagFiles(a.clone, machineDirs, sid)
			if len(files) == 0 {
				continue
			}
			conflict, err := applyResolvedTags(con, sid, files)
			if err != nil {
				slog.Warn("archive: tag ingest failed", "session", sid, "err", err)
				continue
			}
			if conflict {
				conflicts = append(conflicts, sid)
				slog.Warn("archive: cross-machine tag conflict resolved (kept the deterministic winner; every machine's tag file is retained in the archive)",
					"session", sid)
			}
		}
		_ = con.Close()
	}
	if skipped {
		// A scope was unreadable this pass, so `conflicts` is NOT the full picture:
		// union with the prior recorded set so a conflict in the skipped scope never
		// vanishes from `archive status`, and DON'T advance the ingest stamp — the
		// next enumerate retries the skipped scope. (Trade-off: a since-resolved
		// conflict may linger until one clean full pass; not losing a real conflict
		// wins over not over-reporting — an intentional trade-off.)
		conflicts = append(conflicts, readTagConflicts()...)
		writeTagConflicts(conflicts)
		return
	}
	writeTagConflicts(conflicts)
	stampTagIngest()
}

// gatherTagFiles collects every machine's tag file for one session — the
// cross-machine union resolveTags resolves over. Each machine writes only its own
// `<machine>/tags/<sid>.json`, so a session tagged on several machines yields one
// file per machine here; the common case yields exactly one.
func gatherTagFiles(cloneDir string, machineDirs []string, sid string) []TagFile {
	var out []TagFile
	for _, md := range machineDirs {
		p := filepath.Join(cloneDir, md, tagsDirName, sid+".json")
		if tf, ok := loadTagFile(p); ok {
			out = append(out, tf)
		}
	}
	return out
}

// loadTagFile reads and decodes one tag file. A missing/corrupt file is a clean
// skip (false) — a torn JSON from a killed writer must not abort ingest. The
// session id defaults to the filename when the JSON omits it.
func loadTagFile(path string) (TagFile, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return TagFile{}, false
	}
	var tf TagFile
	if json.Unmarshal(b, &tf) != nil {
		return TagFile{}, false
	}
	if tf.SessionID == "" {
		tf.SessionID = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	return tf, true
}

// applyResolvedTags resolves one session's gathered tag files and writes the
// winner into con (the db holding that session's messages). Idempotent: the
// segment set is replaced only when its content actually differs from what the db
// already holds (a content-hash compare), so re-ingesting unchanged tags on every
// search is a cheap read, not a write + FTS churn. The verdict upsert is LWW, so
// it self-no-ops on an equal-or-older row. Returns whether the win resolved a
// cross-machine conflict (for status surfacing).
func applyResolvedTags(con *sql.DB, sid string, files []TagFile) (conflict bool, err error) {
	segs, segOrigin, verdict, verdictOrigin, conflict := resolveTags(files)

	// Rewrite when the CONTENT or the winning ORIGIN changed. Origin matters even
	// at equal content: two machines can author the identical set, and the
	// deterministic winner is the higher origin (resolveSegments) — if the guard
	// compared content only, an already-stored lower-origin copy would never be
	// corrected, so attribution would diverge from a machine that ingested both at
	// once. toTagSegments drops origin by design (it feeds
	// the content hash), so origin is compared separately here.
	cur, _ := store.TopicsForSession(con, sid)
	curOrigin := ""
	if len(cur) > 0 {
		curOrigin = cur[0].OriginMachine
	}
	if segHash(toTagSegments(cur)) != segHash(segs) || (len(segs) > 0 && curOrigin != segOrigin) {
		if err := store.ReplaceSessionSegments(con, sid, toStoreSegments(sid, segOrigin, segs)); err != nil {
			return conflict, err
		}
	}
	if verdict != nil {
		if err := store.MergeVerdict(con, store.Verdict{
			SessionID: sid, Verdict: verdict.Verdict, Source: verdict.Source,
			OriginMachine: verdictOrigin, TaggedAt: verdict.TaggedAt,
		}); err != nil {
			return conflict, err
		}
	}
	return conflict, nil
}

// toTagSegments projects store rows to TagSegments for the content-hash compare
// (origin/id are not content, so they are dropped — matching segHash's fields).
func toTagSegments(segs []store.TopicSegment) []TagSegment {
	out := make([]TagSegment, 0, len(segs))
	for _, s := range segs {
		out = append(out, TagSegment{StartUUID: s.StartUUID, EndUUID: s.EndUUID, Topic: s.Topic, Summary: s.Summary, TaggedAt: s.TaggedAt})
	}
	return out
}

// toStoreSegments builds the rows to write for the winning set, stamping the
// winner's origin machine on every segment (the attribution this preserves).
func toStoreSegments(sid, origin string, segs []TagSegment) []store.TopicSegment {
	out := make([]store.TopicSegment, 0, len(segs))
	for _, s := range segs {
		out = append(out, store.TopicSegment{
			SessionID: sid, StartUUID: s.StartUUID, EndUUID: s.EndUUID,
			Topic: s.Topic, Summary: s.Summary, TaggedAt: s.TaggedAt, OriginMachine: origin,
		})
	}
	return out
}
