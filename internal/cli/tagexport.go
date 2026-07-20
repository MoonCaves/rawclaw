package cli

import (
	"database/sql"
	"os"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// localTagExporter returns an archive.TagExporter that collects THIS machine's
// locally-authored tags — every topic segment + verdict in the LOCAL index dbs
// (Claude + Codex scopes, which by construction exclude the archive's foreign
// scope dbs) — into archive.TagFile records for the archive's push-time export. It is
// injected into the archive package (which must not import scopes/store — that
// would cycle: scopes imports archive). Read-only: it opens dbs that already
// exist and never forces indexing, so a scope with no db yet simply has no tags.
// Because only LOCAL scopes are enumerated, every row read is this machine's; the
// archive stamps our origin id on the way out.
func localTagExporter() archive.TagExporter {
	return func() ([]archive.TagFile, error) {
		var files []archive.TagFile
		seen := make(map[string]struct{}) // a session id is globally unique; first db wins
		for _, sc := range tagLocalScopes() {
			dbp := tagScopeDBPath(sc)
			if dbp == "" || !fileReadable(dbp) {
				continue
			}
			con, err := store.ConnectRO(dbp)
			if err != nil {
				continue // one unreadable local db must not abort the whole export
			}
			collectScopeTags(con, seen, &files)
			_ = con.Close()
		}
		return files, nil
	}
}

// tagLocalScopes lists the local Claude + Codex scopes (including orphaned
// source-gone dbs, whose retained tags still deserve to ride the archive). The
// archive's foreign scopes are deliberately NOT enumerated here.
func tagLocalScopes() []view.Scope {
	out := scopes.Claude()
	return append(out, scopes.Codex(false)...)
}

// tagScopeDBPath resolves a scope's index db path WITHOUT forcing indexing: a
// pre-resolved (eager/orphan) scope carries DBP; a lazy Claude scope derives it
// from its transcript dir.
func tagScopeDBPath(sc view.Scope) string {
	if sc.DBP != "" {
		return sc.DBP
	}
	if sc.TDir != "" {
		return index.DBPath(sc.TDir)
	}
	return ""
}

// collectScopeTags appends one TagFile per tagged session in con (skipping ids
// already collected from an earlier db). A read error on any single query is
// non-fatal — export is best-effort and the next sync catches what it missed.
func collectScopeTags(con *sql.DB, seen map[string]struct{}, out *[]archive.TagFile) {
	ids, err := store.TaggedSessionIDs(con)
	if err != nil {
		return
	}
	for _, sid := range ids {
		if _, dup := seen[sid]; dup {
			continue
		}
		segs, _ := store.TopicsForSession(con, sid)
		v, hasV, _ := store.VerdictFor(con, sid)
		if len(segs) == 0 && !hasV {
			continue
		}
		tf := archive.TagFile{SessionID: sid}
		for _, s := range segs {
			tf.Segments = append(tf.Segments, archive.TagSegment{
				StartUUID: s.StartUUID, EndUUID: s.EndUUID,
				Topic: s.Topic, Summary: s.Summary, TaggedAt: s.TaggedAt,
			})
		}
		if hasV {
			tf.Verdict = &archive.TagVerdict{Verdict: v.Verdict, Source: v.Source, TaggedAt: v.TaggedAt}
		}
		seen[sid] = struct{}{}
		*out = append(*out, tf)
	}
}

// fileReadable reports whether p exists (a stat succeeds).
func fileReadable(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
