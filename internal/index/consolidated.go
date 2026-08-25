package index

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// ConsolidatedDBName is the one store every session and message lands in,
// regardless of which project, source, or machine it came from. It is the shape
// Hermes' state.db has: project is a COLUMN, not a filename.
//
// The name is deliberately not a path encoding, so scope discovery can tell it
// apart from a per-project db by name alone (see scopes.orphanClaudeScopes,
// which must skip it — otherwise every consolidated row would also be searched
// a second time as an "orphaned" project scope).
const ConsolidatedDBName = "consolidated.db"

// ConsolidatedPath returns the consolidated store's path in the cache dir.
func ConsolidatedPath() string {
	return filepath.Join(store.CacheDir(), ConsolidatedDBName)
}

// IsConsolidatedDB reports whether a cache db filename is the consolidated
// store. Callers that enumerate cache dbs as scopes MUST skip it: it is a
// superset of them, not a peer.
func IsConsolidatedDB(dbFileName string) bool {
	return filepath.Base(dbFileName) == ConsolidatedDBName
}

// OpenConsolidated opens the one store read-only for a read verb and reports
// how many sessions it holds. It returns an error — never a usable connection —
// when the store is absent, unreadable, or empty. Those three states look
// identical to "nothing matched" once a query has run against them, and a
// confident empty answer from a store that was never filled is the one failure
// a reader must not produce. A caller that gets an error falls back to the
// per-project databases and says which store answered.
func OpenConsolidated() (*sql.DB, int, error) {
	path := ConsolidatedPath()
	if _, err := os.Stat(path); err != nil {
		return nil, 0, fmt.Errorf("no consolidated store at %s", path)
	}
	con, err := store.ConnectRO(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open consolidated store: %w", err)
	}
	var sessions int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions); err != nil {
		_ = con.Close()
		return nil, 0, fmt.Errorf("read consolidated store: %w", err)
	}
	if sessions == 0 {
		_ = con.Close()
		return nil, 0, fmt.Errorf("consolidated store holds no sessions")
	}
	return con, sessions, nil
}

// UnconsolidatedDBs returns the per-project databases the one store has never
// folded in, by comparing the cache directory against the fold-in watermarks
// the store stamps. This is what keeps a one-store read honest: a project whose
// database exists but was never merged is missing from every answer, and a
// reader has to be able to name it rather than let the corpus quietly shrink.
//
// It compares presence, not freshness — a source that changed after its
// fold-in is not detected here, because proving that would mean opening every
// source database, which is the per-project fan-out this work exists to remove.
func UnconsolidatedDBs(con *sql.DB) ([]string, error) {
	dbs, err := PerProjectDBs()
	if err != nil {
		return nil, err
	}
	return unconsolidatedDBs(con, dbs)
}

func unconsolidatedDBs(con *sql.DB, dbs []string) ([]string, error) {
	rows, err := con.Query("SELECT key FROM meta WHERE key LIKE 'sync:%'")
	if err != nil {
		return nil, fmt.Errorf("read fold-in watermarks: %w", err)
	}
	defer rows.Close()
	folded := map[string]struct{}{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		folded[strings.TrimPrefix(key, "sync:")] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	basenameCount := make(map[string]int)
	for _, dbp := range dbs {
		basenameCount[filepath.Base(dbp)]++
	}
	var missing []string
	for _, dbp := range dbs {
		if _, ok := folded[sourceIdentity(dbp)]; !ok {
			// Accept pre-path-identity watermarks written by older builds only
			// when the current source set has one candidate with that name.
			// Immediately rewrite the watermark under its full-path identity
			// so the ambiguous basename form cannot be accepted a second time
			// if this database is later removed and replaced by another with
			// the same basename.
			base := filepath.Base(dbp)
			if basenameCount[base] == 1 {
				if _, ok := folded[base]; ok {
					rewriteLegacyWatermark(con, dbp, base)
					delete(folded, base)
					folded[sourceIdentity(dbp)] = struct{}{}
					continue
				}
			}
			missing = append(missing, dbp)
		}
	}
	return missing, nil
}

func rewriteLegacyWatermark(con *sql.DB, dbp, base string) {
	oldKey := "sync:" + base
	newKey := syncMarkKey(dbp)
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO meta(key, value) SELECT ?, value FROM meta WHERE key = ?",
		newKey, oldKey,
	); err != nil {
		if rw, rwErr := store.ConnectRW(ConsolidatedPath()); rwErr == nil {
			_, _ = rw.Exec(
				"INSERT OR REPLACE INTO meta(key, value) SELECT ?, value FROM meta WHERE key = ?",
				newKey, oldKey,
			)
			_, _ = rw.Exec("DELETE FROM meta WHERE key = ?", oldKey)
			_ = rw.Close()
		}
		return
	}
	_, _ = con.Exec("DELETE FROM meta WHERE key = ?", oldKey)
}

// SyncStats reports what one consolidation pass moved.
type SyncStats struct {
	Sources      int // per-project dbs read
	Skipped      int // of those, dbs too old to read (reported, never silent)
	SessionsSeen int // session rows offered by those dbs (sum, duplicates included)
	Sessions     int // distinct sessions in the consolidated store afterwards
	Messages     int // distinct messages in the consolidated store afterwards
}

// recordSessionSourcesSQL records one attached db's view of each session into
// main.session_sources. This tracks per-contribution provenance, scope, and missing_since
// status so the merged session row in main.sessions reflects the honest union of all
// contributing sources.
const recordSessionSourcesSQL = `
INSERT INTO main.session_sources
  (session_id,source_db,started_at,last_ts,message_count,is_subagent,parent_id,
   origin_machine,source_tool,source_path,missing_since,project,cwd)
SELECT id, ?, started_at, last_ts, message_count, is_subagent, parent_id,
       origin_machine, source_tool, source_path, missing_since, project, cwd
FROM src.sessions
WHERE true
ON CONFLICT(session_id, source_db) DO UPDATE SET
  started_at     = excluded.started_at,
  last_ts        = excluded.last_ts,
  message_count  = excluded.message_count,
  is_subagent    = excluded.is_subagent,
  parent_id      = excluded.parent_id,
  origin_machine = excluded.origin_machine,
  source_tool    = excluded.source_tool,
  source_path    = excluded.source_path,
  missing_since  = excluded.missing_since,
  project        = excluded.project,
  cwd            = excluded.cwd
`

// mergeSessionsSQL aggregates all recorded session contributions in session_sources
// for sessions present in src.sessions or removed from this source during the
// current pass, and writes the merged rows into main.sessions.
//
// Semantics per contribution:
//   - A session present in ANY contributing copy is present/live (missing_since IS NULL).
//   - Only when EVERY contributing copy has vanished is the merged session marked missing.
//     Where all copies are purged, missing_since is MAX(missing_since) — the timestamp
//     when the LAST live copy vanished from disk.
//   - Scope and provenance follow the winning contribution (live beats purged; between
//     equal presence, highest message count wins; tie-broken by latest last_ts and source_db).
//     A known non-null value (project/cwd/source_tool/origin_machine) never loses to a NULL.
//   - started_at is the earliest non-zero start timestamp across contributions.
//   - last_ts is the latest activity across contributions.
const mergeSessionsSQL = `
WITH ranked AS (
  SELECT
    session_id,
    project,
    cwd,
    source_path,
    source_tool,
    origin_machine,
    ROW_NUMBER() OVER (
      PARTITION BY session_id
      ORDER BY
        (missing_since IS NULL) DESC,
        COALESCE(message_count, 0) DESC,
        COALESCE(last_ts, 0) DESC,
        source_db DESC
    ) AS rank
  FROM main.session_sources
  WHERE session_id IN (SELECT id FROM src.sessions)
     OR session_id IN (SELECT session_id FROM temp.consolidation_affected_sessions)
),
agg AS (
  SELECT
    session_id,
    MIN(CASE WHEN started_at > 0 THEN started_at END) AS started_at,
    MAX(COALESCE(last_ts, 0)) AS last_ts,
    MAX(COALESCE(is_subagent, 0)) AS is_subagent,
    MAX(parent_id) AS parent_id,
    CASE
      WHEN COUNT(CASE WHEN missing_since IS NULL THEN 1 END) > 0 THEN NULL
      ELSE MAX(missing_since)
    END AS missing_since
  FROM main.session_sources
  WHERE session_id IN (SELECT id FROM src.sessions)
     OR session_id IN (SELECT session_id FROM temp.consolidation_affected_sessions)
  GROUP BY session_id
)
INSERT INTO main.sessions (
  id, started_at, last_ts, message_count, is_subagent, parent_id,
  origin_machine, source_tool, source_path, missing_since, project, cwd
)
SELECT
  a.session_id,
  COALESCE(a.started_at, 0),
  a.last_ts,
  0,
  a.is_subagent,
  a.parent_id,
  COALESCE(r.origin_machine, (SELECT origin_machine FROM main.session_sources s2 WHERE s2.session_id = a.session_id AND s2.origin_machine IS NOT NULL LIMIT 1)),
  COALESCE(r.source_tool, (SELECT source_tool FROM main.session_sources s2 WHERE s2.session_id = a.session_id AND s2.source_tool IS NOT NULL LIMIT 1)),
  r.source_path,
  a.missing_since,
  COALESCE(r.project, (SELECT project FROM main.session_sources s2 WHERE s2.session_id = a.session_id AND s2.project IS NOT NULL LIMIT 1)),
  COALESCE(r.cwd, (SELECT cwd FROM main.session_sources s2 WHERE s2.session_id = a.session_id AND s2.cwd IS NOT NULL LIMIT 1))
FROM agg a
JOIN ranked r ON a.session_id = r.session_id AND r.rank = 1
ON CONFLICT(id) DO UPDATE SET
  started_at     = excluded.started_at,
  last_ts        = excluded.last_ts,
  is_subagent    = excluded.is_subagent,
  parent_id      = excluded.parent_id,
  origin_machine = excluded.origin_machine,
  source_tool    = excluded.source_tool,
  source_path    = excluded.source_path,
  missing_since  = excluded.missing_since,
  project        = excluded.project,
  cwd            = excluded.cwd
`

// mergeMessagesSQL unions one attached db's messages in, keyed by
// (session_id, uuid) — the identity a message carries from its source, stable
// across reindexes and across the copies of a resumed session.
//
// The GROUP BY collapses duplicates WITHIN the source as well. Those exist:
// per-project stores hold byte-identical repeats of the same uuid where a
// session was replayed into several transcript files. MIN(id) makes the pick
// deterministic (SQLite takes the bare columns from the row matching a lone
// MIN/MAX), so re-running the pass yields the same rows.
//
// The message id is deliberately NOT carried over: it is an AUTOINCREMENT
// rowid, unique only within its own db, and it is the FTS rowid — letting two
// sources' id 1 collide would silently overwrite one message's search entry.
// Inserting without it lets the FTS triggers assign a fresh, unique rowid.
//
// ORDER BY first_id is load-bearing, not cosmetic. The new rowid IS the reading
// order: every session view walks messages by id, because timestamps are not
// reliably monotonic. So whatever order these rows are inserted in becomes the
// order the conversation is replayed in forever. Without the ORDER BY, the rows
// arrive in whatever order the GROUP BY left them — which SQLite satisfies by
// sorting on the grouping key, i.e. by uuid. A uuid is random, so the
// conversation was being reassembled in alphabetical order of a random string:
// measured on one 3k-message session, 1490 of 2978 adjacent pairs disagreed
// with the source file, and the session's first message came back as a Bash
// call from the middle instead of the human's opening line.
//
// first_id (MIN(id) in the SOURCE db) is that source's own insertion order,
// which is file order — the very thing the reading order is meant to be. It was
// already computed here for the dedup pick and then discarded.
const mergeMessagesSQL = `
INSERT INTO main.messages(session_id,role,content,ts,ts_iso,uuid)
SELECT session_id, role, content, ts, ts_iso, uuid FROM (
  SELECT s.session_id AS session_id, s.role AS role, s.content AS content,
         s.ts AS ts, s.ts_iso AS ts_iso, s.uuid AS uuid, MIN(s.id) AS first_id
  FROM src.messages s
  WHERE NOT EXISTS (
    SELECT 1 FROM main.messages m
    WHERE m.session_id = s.session_id AND m.uuid = s.uuid
  )
  GROUP BY s.session_id, s.uuid
)
ORDER BY first_id
`

// topicNewer is the precedence rule between two taggings of the SAME segment
// (one session, one start anchor): origin authority wins (higher origin_machine);
// on equal origin_machine, the later tagging wins (larger tagged_at). Equal or
// missing values leave the copy already in the store alone, which keeps the
// pass order-independent.
const topicNewer = `(COALESCE(excluded.origin_machine, '') > COALESCE(topic_segment.origin_machine, '')) OR (COALESCE(excluded.origin_machine, '') = COALESCE(topic_segment.origin_machine, '') AND COALESCE(excluded.tagged_at, 0) > COALESCE(topic_segment.tagged_at, 0))`

// mergeTopicsSQL folds one attached db's topic segments into the consolidated
// store, keyed by the (session_id, start_uuid) identity the segment already
// carries. Without this the one store answers every topic query with nothing:
// the tags live only in the per-project dbs that produced them, so a reader
// that resolves to the store would report an untagged corpus.
//
// As with messages, the segment id is deliberately not carried over — it is an
// AUTOINCREMENT rowid that doubles as the topic_fts rowid, so two sources' id 1
// would silently overwrite each other's search entry. Inserting without it lets
// the topic_ai / topic_au triggers assign a fresh rowid and keep topic_fts in
// step, which is why nothing here writes topic_fts directly.
// The origin column is selected through a placeholder because the topic table
// predates it: a project tagged before provenance was added has a topic_segment
// with no origin_machine at all, and naming it unconditionally makes the merge
// fail on exactly the OLDEST tags — the ones with the most history behind them,
// and the ones a one-store reader would otherwise never see. Where the source
// has no such column the merge substitutes NULL, which is what an untracked
// origin already means everywhere else in this store.
func mergeTopicsSQLFor(srcHasOrigin bool) string {
	origin := "NULL"
	if srcHasOrigin {
		origin = "origin_machine"
	}
	return strings.Replace(mergeTopicsSQL, "{{origin}}", origin, 1)
}

var mergeTopicsSQL = `
INSERT INTO main.topic_segment
  (session_id,start_uuid,end_uuid,topic,summary,tagged_at,origin_machine)
SELECT session_id,start_uuid,end_uuid,topic,summary,tagged_at,{{origin}}
FROM src.topic_segment
WHERE true
ON CONFLICT(session_id,start_uuid) DO UPDATE SET
  end_uuid       = CASE WHEN ` + topicNewer + ` THEN excluded.end_uuid       ELSE topic_segment.end_uuid END,
  topic          = CASE WHEN ` + topicNewer + ` THEN excluded.topic          ELSE topic_segment.topic END,
  summary        = CASE WHEN ` + topicNewer + ` THEN excluded.summary        ELSE topic_segment.summary END,
  origin_machine = CASE WHEN ` + topicNewer + ` THEN excluded.origin_machine ELSE topic_segment.origin_machine END,
  tagged_at      = CASE WHEN ` + topicNewer + ` THEN excluded.tagged_at      ELSE topic_segment.tagged_at END
`

// verdictNewer is the precedence rule for session verdicts: latest tagged_at
// wins, tie broken by higher origin_machine (cosmetic attribution tie-break).
const verdictNewer = `(COALESCE(excluded.tagged_at,0) > COALESCE(session_verdict.tagged_at,0)) OR (COALESCE(excluded.tagged_at,0) = COALESCE(session_verdict.tagged_at,0) AND COALESCE(excluded.origin_machine,'') > COALESCE(session_verdict.origin_machine,''))`

// mergeVerdictsSQL folds the session-verdict sidecar in on the same rule: one
// row per session, the later verdict wins. It rides with the topic merge
// because it is the same tagging act's other half.
const mergeVerdictsSQL = `
INSERT INTO main.session_verdict(session_id,verdict,source,origin_machine,tagged_at)
SELECT session_id,verdict,source,origin_machine,tagged_at FROM src.session_verdict
WHERE true
ON CONFLICT(session_id) DO UPDATE SET
  verdict        = CASE WHEN ` + verdictNewer + ` THEN excluded.verdict        ELSE session_verdict.verdict END,
  source         = CASE WHEN ` + verdictNewer + ` THEN excluded.source         ELSE session_verdict.source END,
  origin_machine = CASE WHEN ` + verdictNewer + ` THEN excluded.origin_machine ELSE session_verdict.origin_machine END,
  tagged_at      = CASE WHEN ` + verdictNewer + ` THEN excluded.tagged_at      ELSE session_verdict.tagged_at END
`

// recountSQL restates each session's message_count from the union that actually
// landed, replacing the per-source counts carried in by the merge.
const recountSQL = `
UPDATE sessions SET message_count =
  (SELECT COUNT(*) FROM messages WHERE messages.session_id = sessions.id)
`

// ConsolidateFrom fills the consolidated store from the given per-project db
// paths. Passing rebuild drops the store first, so a full re-run costs one pass
// over the existing dbs rather than a re-read of every transcript on disk.
//
// Sources are read through SQLite's ATTACH, so nothing is parsed twice: the
// transcripts were already turned into rows once, and this moves those rows.
func ConsolidateFrom(srcPaths []string, rebuild bool) (st SyncStats, err error) {
	dst := ConsolidatedPath()
	var preserved tagState
	if rebuild {
		preserved, err = readTagState(dst)
		if err != nil {
			return st, fmt.Errorf("preserve consolidated tags: %w", err)
		}
	}
	if rebuild {
		// Build the replacement BESIDE the live store and swap only once it is
		// complete. Deleting first meant any later failure — connect, schema,
		// heal, or a single bad source mid-fold — left the user with no store
		// at all until another full rebuild happened to succeed.
		live := dst
		dst = live + ".rebuild"
		for _, suffix := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(dst + suffix) // clear a previous interrupted attempt, never the live store
		}
		defer func() {
			// Registered BEFORE `defer con.Close()`, so it runs after the
			// connection is closed and the files are quiescent.
			if err != nil {
				for _, suffix := range []string{"", "-wal", "-shm"} {
					_ = os.Remove(dst + suffix)
				}
				return // live store untouched
			}
			if rErr := os.Rename(dst, live); rErr != nil {
				err = fmt.Errorf("swap rebuilt store into place: %w", rErr)
				return
			}
			// The old sidecars describe the replaced file, not the new one.
			for _, suffix := range []string{"-wal", "-shm"} {
				_ = os.Remove(live + suffix)
				_ = os.Remove(dst + suffix)
			}
		}()
	}

	con, err := store.ConnectRW(dst)
	if err != nil {
		return st, fmt.Errorf("open consolidated store: %w", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		return st, fmt.Errorf("ensure consolidated schema: %w", err)
	}
	if err := healUpgradedConsolidatedStore(con); err != nil {
		// Healing MUST precede backfill: do not populate session_sources if healing did not succeed.
		return st, fmt.Errorf("heal upgraded store: %w", err)
	}
	// The topic layer is gated separately from the keyword schema, so it has to
	// be asked for explicitly. Creating it unconditionally means a topic query
	// against the one store can answer "nothing is tagged yet" instead of
	// failing on a missing table.
	if err := store.EnsureTopicSchema(con); err != nil {
		return st, fmt.Errorf("ensure consolidated topic schema: %w", err)
	}
	if rebuild {
		if err := restoreTagState(con, preserved); err != nil {
			return st, fmt.Errorf("restore consolidated tags: %w", err)
		}
	}
	if err := migrateSessionSources(con); err != nil {
		return st, fmt.Errorf("migrate session sources: %w", err)
	}

	for _, src := range srcPaths {
		n, _, skipped, err := consolidateOne(con, src)
		if err != nil {
			return st, fmt.Errorf("consolidate %s: %w", filepath.Base(src), err)
		}
		st.Sources++
		if skipped {
			st.Skipped++
		}
		st.SessionsSeen += n
	}
	// Prune aggregate legacy baseline rows only when every source candidate
	// successfully folded this pass. If any source was skipped (e.g. behind-version
	// schema), its provenance is unrecorded and dropping legacy provenance could
	// cause a later fold of a co-contributor to erroneously delete live sessions.
	if st.Skipped == 0 && st.Sources > 0 {
		if _, err := con.Exec(`
			DELETE FROM main.session_sources
			WHERE source_db = ''
			  AND EXISTS (
				SELECT 1 FROM main.session_sources real
				WHERE real.session_id = session_sources.session_id
				  AND real.source_db <> ''
			  )
		`); err != nil {
			return st, fmt.Errorf("prune legacy session sources: %w", err)
		}
	}

	if rebuild {
		if _, err := con.Exec(recountSQL); err != nil {
			return st, fmt.Errorf("recount messages: %w", err)
		}
	}
	if err := pruneTombstoned(con); err != nil {
		return st, err
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&st.Sessions); err != nil {
		return st, fmt.Errorf("count sessions: %w", err)
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM messages").Scan(&st.Messages); err != nil {
		return st, fmt.Errorf("count messages: %w", err)
	}
	if err := StampIngestWatermark(con); err != nil {
		slog.Debug("stamp ingest watermark failed", "err", err)
	}
	return st, nil
}

// SyncConsolidatedFrom folds ONE per-project db into the consolidated store.
// This is the write-through half: an indexing run updates its own project db,
// then hands that db here, so the consolidated store tracks without a separate
// pass over the transcripts.
func SyncConsolidatedFrom(srcPath string) error {
	if IsConsolidatedDB(srcPath) {
		return nil
	}
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		return fmt.Errorf("open consolidated store: %w", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		return fmt.Errorf("ensure consolidated schema: %w", err)
	}
	if err := healUpgradedConsolidatedStore(con); err != nil {
		// Healing MUST precede backfill: do not populate session_sources if healing did not succeed.
		return fmt.Errorf("heal upgraded store: %w", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		return fmt.Errorf("ensure consolidated topic schema: %w", err)
	}
	if err := migrateSessionSources(con); err != nil {
		return fmt.Errorf("migrate session sources: %w", err)
	}
	_, changed, skipped, err := consolidateOne(con, srcPath)
	if err != nil {
		return fmt.Errorf("consolidate %s: %w", filepath.Base(srcPath), err)
	}
	if skipped || !changed {
		return nil
	}
	if err := pruneTombstoned(con); err != nil {
		return err
	}
	if err := StampIngestWatermark(con); err != nil {
		slog.Debug("stamp ingest watermark failed", "err", err)
	}
	return nil
}

// writeThroughConsolidated folds a per-project db into the consolidated store
// right after an indexing run updated it, so the one store tracks the many
// without a second pass over the transcripts. Call it AFTER the indexing
// connection is closed.
//
// It is advisory in both directions: a failed index run is not folded in (its
// db may be inconsistent), and a failed write-through does not fail the index
// run (the next pass catches up). If the consolidated store is locked by
// another process, write-through logs and proceeds rather than blocking the
// indexing command: the reader falls back to per-project dbs when the one
// store is missing, and the next normal command folds it in.
func writeThroughConsolidated(dbp string, indexErr error) {
	if indexErr != nil || IsConsolidatedDB(dbp) {
		return
	}
	if err := SyncConsolidatedFrom(dbp); err != nil {
		slog.Debug("consolidate: write-through failed (will retry on next pass)", "db", filepath.Base(dbp), "err", err)
		return
	}
	// A new or appended project may have brought in messages that need
	// embeddings. Fire the top-up in the background so the indexing command
	// does not wait on vector latency: the vector index is an accelerator,
	// not a correctness gate.
	fireVectorTopup(ConsolidatedPath())
}

// consolidateOne attaches src, merges its sessions and messages, and detaches.
// It is atomic per source: if a merge fails, the detach still runs and the one
// store's transaction rolls back. It does not modify src — the one store is a
// derived view, not a second home.
//
// The source is attached read-only: SQLite enforces that no merge step can mutate
// it, and consolidation must never be the thing that blocks indexing. The one
// write it makes to a source is the additive column migration below, on its own
// connection, before the read-only attach.
func consolidateOne(con *sql.DB, src string) (offered int, changed bool, skipped bool, err error) {
	if _, err := os.Stat(src); err != nil {
		return 0, false, true, fmt.Errorf("source unreadable: %w", err)
	}
	// Bring the source up to the columns the merge reads BEFORE attaching it
	// read-only. Failure here is not fatal — the guard below decides whether the
	// source is usable — but without this step a corpus indexed before the scope
	// migration has no project/cwd columns anywhere and the whole pass skips
	// every source while reporting success.
	if mErr := migrateSourceScope(src); mErr != nil {
		slog.Debug("consolidate: source not migrated", "db", filepath.Base(src), "err", mErr)
	}
	if _, err := con.Exec("ATTACH DATABASE ? AS src", "file:"+src+"?mode=ro"); err != nil {
		return 0, false, true, fmt.Errorf("attach: %w", err)
	}
	defer func() {
		if _, dErr := con.Exec("DETACH DATABASE src"); dErr != nil && err == nil {
			err = fmt.Errorf("detach: %w", dErr)
		}
	}()

	// A db the migration could not bring forward — one still behind the current
	// schema version, or mid-creation with no tables at all — has no project/cwd
	// columns for the merge to select. Skip it rather than fail the whole pass:
	// the next indexing run rebuilds it from its transcripts, and this is
	// re-runnable. The caller counts the skip so a pass that moves nothing says
	// so instead of printing a success line.
	if ok, cErr := hasScopeColumns(con); cErr != nil || !ok {
		return 0, false, true, cErr
	}

	// Whether this source has a topic layer at all: the topic tables are created
	// on demand by the tagging path, so a project nobody has tagged simply has
	// no such table and its merge steps are skipped.
	hasTopics, err := srcHasTable(con, "topic_segment")
	if err != nil {
		return 0, false, true, err
	}
	hasVerdicts, err := srcHasTable(con, "session_verdict")
	if err != nil {
		return 0, false, true, err
	}

	mark := ""
	if err := con.QueryRow(
		`SELECT (SELECT COUNT(*) FROM src.sessions) || ':' ||
		        (SELECT COUNT(*) FROM src.messages) || ':' ||
		        (SELECT COALESCE(MAX(id),0) FROM src.messages) || ':' ||
		        (SELECT COUNT(missing_since) || '@' || COALESCE(MAX(missing_since),0) FROM src.sessions)`,
	).Scan(&mark); err != nil {
		return 0, false, true, fmt.Errorf("read source watermark: %w", err)
	}
	// The topic layer and session verdicts join the watermark, because tagging
	// changes a source without touching a single session or message row. Left
	// out, a re-tagged project or updated verdict would read as unchanged and
	// its new labels or verdicts would never arrive. The latest tagging time
	// rides along with the count: re-tagging a segment in place or updating a
	// verdict replaces a row without changing how many there are, so a count
	// alone would still miss it.
	topicMark := "0"
	if hasTopics {
		if err := con.QueryRow(
			`SELECT COUNT(*) || '@' || COALESCE(MAX(tagged_at),0) FROM src.topic_segment`,
		).Scan(&topicMark); err != nil {
			return 0, false, true, fmt.Errorf("read source topic watermark: %w", err)
		}
	}
	mark += ":t" + topicMark
	verdictMark := "0"
	if hasVerdicts {
		if err := con.QueryRow(
			`SELECT COUNT(*) || '@' || COALESCE(MAX(tagged_at),0) FROM src.session_verdict`,
		).Scan(&verdictMark); err != nil {
			return 0, false, true, fmt.Errorf("read source verdict watermark: %w", err)
		}
	}
	mark += ":v" + verdictMark
	if err := con.QueryRow("SELECT COUNT(*) FROM src.sessions").Scan(&offered); err != nil {
		return 0, false, true, fmt.Errorf("count source sessions: %w", err)
	}
	srcOrigin := false
	if hasTopics {
		srcOrigin, err = srcHasColumn(con, "topic_segment", "origin_machine")
		if err != nil {
			return 0, false, true, err
		}
	}

	// Skip a source that has not changed since its last fold-in. Without this
	// the write-through would re-scan every message of every project on every
	// invocation, which is the cost the per-project layout was hiding.
	key := syncMarkKey(src)
	var prev string
	switch err := con.QueryRow("SELECT value FROM meta WHERE key=?", key).Scan(&prev); {
	case err == nil && prev == mark:
		return offered, false, false, nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return 0, false, true, fmt.Errorf("read sync watermark: %w", err)
	}

	srcID := sourceIdentity(src)
	if err := migrateSessionSources(con); err != nil {
		return 0, false, true, fmt.Errorf("migrate session sources: %w", err)
	}

	tx, err := con.Begin()
	if err != nil {
		return 0, false, true, fmt.Errorf("begin fold: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rErr := tx.Rollback(); rErr != nil && err == nil {
				err = fmt.Errorf("rollback fold: %w", rErr)
			}
		}
	}()

	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS consolidation_affected_sessions (
		session_id TEXT PRIMARY KEY
	)`); err != nil {
		return 0, false, true, fmt.Errorf("create affected session set: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM temp.consolidation_affected_sessions"); err != nil {
		return 0, false, true, fmt.Errorf("clear affected session set: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO temp.consolidation_affected_sessions(session_id)
		SELECT session_id FROM main.session_sources
		WHERE source_db = ?
		  AND session_id NOT IN (SELECT id FROM src.sessions)
	`, srcID); err != nil {
		return 0, false, true, fmt.Errorf("record deleted session sources: %w", err)
	}
	// Inside tx: these writes must roll back with the fold, not survive it.
	if _, err := tx.Exec(recordSessionSourcesSQL, srcID); err != nil {
		return 0, false, true, fmt.Errorf("record session sources: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM main.session_sources WHERE source_db = ? AND session_id NOT IN (SELECT id FROM src.sessions)", srcID); err != nil {
		return 0, false, true, fmt.Errorf("prune stale session sources: %w", err)
	}
	if _, err := tx.Exec(mergeSessionsSQL); err != nil {
		return 0, false, true, fmt.Errorf("merge sessions: %w", err)
	}
	if _, err := tx.Exec(mergeMessagesSQL); err != nil {
		return 0, false, true, fmt.Errorf("merge messages: %w", err)
	}
	if hasTopics {
		if _, err := tx.Exec(mergeTopicsSQLFor(srcOrigin)); err != nil {
			return 0, false, true, fmt.Errorf("merge topics: %w", err)
		}
	}
	if hasVerdicts {
		if _, err := tx.Exec(mergeVerdictsSQL); err != nil {
			return 0, false, true, fmt.Errorf("merge verdicts: %w", err)
		}
	}
	if _, err := tx.Exec(`
		DELETE FROM main.messages
		WHERE session_id IN (
			SELECT a.session_id
			FROM temp.consolidation_affected_sessions a
			WHERE NOT EXISTS (
				SELECT 1 FROM main.session_sources s WHERE s.session_id = a.session_id
			)
		)
	`); err != nil {
		return 0, false, true, fmt.Errorf("prune deleted session messages: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM main.sessions
		WHERE id IN (
			SELECT a.session_id
			FROM temp.consolidation_affected_sessions a
			WHERE NOT EXISTS (
				SELECT 1 FROM main.session_sources s WHERE s.session_id = a.session_id
			)
		)
	`); err != nil {
		return 0, false, true, fmt.Errorf("prune deleted sessions: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE main.sessions SET message_count =
		  (SELECT COUNT(*) FROM main.messages WHERE main.messages.session_id = main.sessions.id)
		WHERE main.sessions.id IN (SELECT id FROM src.sessions)
	`); err != nil {
		return 0, false, true, fmt.Errorf("recount source sessions: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO main.file_index(path,mtime,size,fp,session_id)
		SELECT path,mtime,size,fp,session_id FROM src.file_index
		WHERE true
		ON CONFLICT(path) DO UPDATE SET
		  mtime = excluded.mtime,
		  size = excluded.size,
		  fp = excluded.fp,
			 session_id = excluded.session_id
	`); err != nil {
		return 0, false, true, fmt.Errorf("merge file_index: %w", err)
	}
	if _, err := tx.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES(?,?)", key, mark); err != nil {
		return 0, false, true, fmt.Errorf("stamp sync watermark: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, true, fmt.Errorf("commit fold: %w", err)
	}
	committed = true
	return offered, true, false, nil
}

// migrateSourceScope brings a per-project db up to the scope columns the merge
// reads. Consolidation is otherwise read-only on its sources, but a db last
// written before the scope migration has no project/cwd columns at all, so
// without this an entire corpus indexed before that change is skipped whole.
//
// It runs the same additive, PRAGMA-guarded migration an indexing run does
// (migrateScopeColumns): add the columns, backfill them from each row's
// source_path. It deliberately does NOT go through EnsureSchema, because that
// REBUILDS a db whose schema_version is behind — dropping the very rows
// consolidation was about to read. A behind-version source is refused here and
// left for the next indexing run, which rebuilds it from its transcripts.
func migrateSourceScope(src string) error {
	con, err := store.ConnectRW(src)
	if err != nil {
		return fmt.Errorf("open source for migration: %w", err)
	}
	defer con.Close()
	var version string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&version); err != nil {
		return fmt.Errorf("read source schema version: %w", err)
	}
	if want := strconv.Itoa(store.SchemaVersion); version != want {
		return fmt.Errorf("source at schema v%s, want v%s", version, want)
	}
	return migrateScopeColumns(con)
}

// sourceIdentity is the stable identity of a source database. A basename is
// not sufficient: two projects can both have a sessions.db, and treating them
// as one source lets one project's purge delete the other's contribution.
func sourceIdentity(src string) string {
	abs, err := filepath.Abs(src)
	if err != nil {
		return filepath.Clean(src)
	}
	return filepath.Clean(abs)
}

// syncMarkKey names the meta row holding a source db's last-folded-in
// watermark. Keyed by the full path, so same-named source databases do not
// share a watermark and a source rebuilt from scratch (new row ids, new
// counts) re-folds rather than being mistaken for unchanged.
func syncMarkKey(src string) string { return "sync:" + sourceIdentity(src) }

// hasScopeColumns reports whether the ATTACHed source carries the project/cwd
// columns the merge selects.
// srcHasTable reports whether the ATTACHed source carries a given table. The
// topic layer is created on demand rather than by the base schema, so a source
// that was never tagged genuinely has no topic tables and its topic merge must
// be skipped rather than fail the whole pass.
func srcHasTable(con *sql.DB, name string) (bool, error) {
	var one int
	err := con.QueryRow(
		"SELECT 1 FROM src.sqlite_master WHERE type='table' AND name=?", name).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("look for source table %s: %w", name, err)
	}
	return true, nil
}

// srcHasColumn reports whether an attached source table carries a column. The
// sidecar tables grew columns after they shipped, so a source written by an
// older build is a normal state to find on disk, not a corruption — the merge
// asks rather than assumes.
func srcHasColumn(con *sql.DB, table, col string) (bool, error) {
	rows, err := con.Query("PRAGMA src.table_info(" + table + ")")
	if err != nil {
		return false, fmt.Errorf("read source %s columns: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid         int
			name, typ   string
			notNull, pk int
			dflt        sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("scan source %s column: %w", table, err)
		}
		if name == col {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate source %s columns: %w", table, err)
	}
	return false, nil
}

func hasScopeColumns(con *sql.DB) (bool, error) {
	rows, err := con.Query("PRAGMA src.table_info(sessions)")
	if err != nil {
		return false, fmt.Errorf("read source columns: %w", err)
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var (
			cid         int
			name, typ   string
			notNull, pk int
			dflt        sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("scan source column: %w", err)
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate source columns: %w", err)
	}
	return have["project"] && have["cwd"], nil
}

// pruneTombstoned removes sessions the user explicitly deleted. The merge is
// additive by construction, so without this a deleted session would come back
// the moment its old rows were read out of a per-project db that had not been
// reconciled yet. A user delete is the one absence that must propagate.
// escapeLike makes s safe to embed as the LITERAL prefix of a LIKE pattern that
// is evaluated with ESCAPE '\\'. Without it a session id containing _ or %
// behaves as a wildcard and matches sibling ids.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "_", `\_`, "%", `\%`)
	return r.Replace(s)
}

func pruneTombstoned(con *sql.DB) error {
	tombstoned, err := lifecycle.LoadTombstones("")
	if err != nil || len(tombstoned) == 0 {
		return nil // best-effort: a missing sidecar never blocks consolidation
	}
	ids := make([]string, 0, len(tombstoned))
	for id := range tombstoned {
		ids = append(ids, id)
	}
	return pruneTombstonedIDs(con, ids)
}

// pruneTombstonedIDs is the deletion core, split out so the wildcard-escaping
// behaviour is testable without a tombstone sidecar on disk.
func pruneTombstonedIDs(con *sql.DB, ids []string) error {
	for _, id := range ids {
		// Tombstones cover a session and the subagent threads beneath it, whose
		// ids are "<parent>/<stem>".
		//
		// The id is DATA, not a pattern: SQLite LIKE treats _ as a single-char
		// wildcard and % as any-run, so an unescaped id containing either would
		// match NEIGHBOURING sessions and delete their rows from all four tables
		// on every consolidation pass. Escape the literal part, then anchor.
		like := escapeLike(id) + "/%"
		if _, err := con.Exec(`DELETE FROM messages WHERE session_id = ? OR session_id LIKE ? ESCAPE '\'`, id, like); err != nil {
			return fmt.Errorf("prune tombstoned messages: %w", err)
		}
		if _, err := con.Exec(`DELETE FROM sessions WHERE id = ? OR id LIKE ? ESCAPE '\'`, id, like); err != nil {
			return fmt.Errorf("prune tombstoned sessions: %w", err)
		}
		if _, err := con.Exec(`DELETE FROM session_sources WHERE session_id = ? OR session_id LIKE ? ESCAPE '\'`, id, like); err != nil {
			return fmt.Errorf("prune tombstoned session sources: %w", err)
		}
		if _, err := con.Exec(`DELETE FROM file_index WHERE session_id = ? OR session_id LIKE ? ESCAPE '\'`, id, like); err != nil {
			return fmt.Errorf("prune tombstoned file_index: %w", err)
		}
	}
	return nil
}

// migrateSessionSources backfills session_sources from existing sessions rows in
// main.sessions before an aggregate merge runs.
//
// On an existing store predating session_sources, sessions rows represent the
// pre-migration consolidated baseline (potentially merged from multiple sources).
// Backfilling them with an empty source_db ("") records this legacy contribution
// so incremental syncs of individual source dbs do not clobber multi-source metadata
// (message_count, missing_since, scope/provenance) from an incomplete contribution set.
func migrateSessionSources(con *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS session_sources (
    session_id TEXT NOT NULL, source_db TEXT NOT NULL,
    started_at REAL, last_ts REAL, message_count INTEGER DEFAULT 0,
    is_subagent INTEGER DEFAULT 0, parent_id TEXT,
    origin_machine TEXT, source_tool TEXT, source_path TEXT, missing_since REAL,
    project TEXT, cwd TEXT,
    PRIMARY KEY (session_id, source_db)
);
CREATE INDEX IF NOT EXISTS idx_session_sources_session ON session_sources(session_id);
`
	if _, err := con.Exec(ddl); err != nil {
		return fmt.Errorf("ensure session_sources schema: %w", err)
	}

	var hasSessions int
	if err := con.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sessions'").Scan(&hasSessions); err != nil {
		return fmt.Errorf("check sessions table: %w", err)
	}
	if hasSessions == 0 {
		return nil
	}

	const backfillSQL = `
INSERT OR IGNORE INTO main.session_sources (
  session_id, source_db, started_at, last_ts, message_count,
  is_subagent, parent_id, origin_machine, source_tool, source_path,
  missing_since, project, cwd
)
SELECT
  id, '', started_at, last_ts, message_count,
  is_subagent, parent_id, origin_machine, source_tool, source_path,
  missing_since, project, cwd
FROM main.sessions
WHERE NOT EXISTS (
  SELECT 1 FROM main.session_sources WHERE main.session_sources.session_id = main.sessions.id
)
`
	if _, err := con.Exec(backfillSQL); err != nil {
		return fmt.Errorf("backfill session_sources: %w", err)
	}
	return nil
}

// PerProjectDBs lists the per-project cache dbs that feed the consolidated
// store: every *.db in the cache dir except the consolidated store itself and
// the non-index sidecars. Archive replicas ARE included — a foreign machine's
// sessions carry origin_machine on the row, so they belong in the one store
// exactly like local ones.
func PerProjectDBs() ([]string, error) {
	entries, err := filepath.Glob(filepath.Join(store.CacheDir(), "*.db"))
	if err != nil {
		return nil, fmt.Errorf("list cache dbs: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		base := filepath.Base(e)
		if IsConsolidatedDB(base) || strings.HasPrefix(base, "tombstones") {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// Meta keys for index-level freshness watermarks.
const (
	MetaLastIngestTime         = "last_ingest_time"
	MetaLastIngestCatalogMTime = "last_ingest_catalog_mtime"
)

// StampIngestWatermark records the current epoch and catalog directory mtime in
// the consolidated store's meta table so subsequent read verbs can verify freshness in O(1).
func StampIngestWatermark(con *sql.DB) error {
	now := strconv.FormatFloat(nowEpoch(), 'f', -1, 64)
	if _, err := con.Exec("INSERT OR REPLACE INTO meta(key, value) VALUES(?, ?)", MetaLastIngestTime, now); err != nil {
		return fmt.Errorf("stamp %s: %w", MetaLastIngestTime, err)
	}
	catDir := paths.CatalogDir()
	if st, err := os.Stat(catDir); err == nil {
		catMTime := strconv.FormatFloat(mtimeOf(st), 'f', -1, 64)
		if _, err := con.Exec("INSERT OR REPLACE INTO meta(key, value) VALUES(?, ?)", MetaLastIngestCatalogMTime, catMTime); err != nil {
			return fmt.Errorf("stamp %s: %w", MetaLastIngestCatalogMTime, err)
		}
	}
	return nil
}

// healUpgradedConsolidatedStore checks if the consolidated store was upgraded from an
// older schema where session_sources or file_index were not populated. If sessions exist
// but session_sources is empty, it invalidates the fold-in watermarks (deleting sync: keys)
// to force a full re-fold that populates session_sources and file_index.
func healUpgradedConsolidatedStore(con *sql.DB) error {
	var (
		nSessions int
		nSources  int
	)
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&nSessions); err != nil {
		return err
	}
	if nSessions == 0 {
		return nil
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM session_sources").Scan(&nSources); err != nil {
		return err
	}
	if nSources == 0 {
		if _, err := con.Exec("DELETE FROM meta WHERE key LIKE 'sync:%'"); err != nil {
			return fmt.Errorf("invalidate fold-in watermarks: %w", err)
		}
	}
	return nil
}

// IndexFreshness reports the result of the O(1) index-level freshness check.
type IndexFreshness struct {
	Fresh  bool
	Reason string
}

// CheckIndexFreshness evaluates whether the consolidated store is current by
// comparing the last-ingest watermark in meta against a single stat of the session catalog dir.
// It is strictly O(1): at most 1 stat of the catalog directory and 1 DB meta query.
func CheckIndexFreshness(con *sql.DB) (IndexFreshness, error) {
	var (
		ingestTimeStr string
		catMTimeStr   string
	)
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", MetaLastIngestTime).Scan(&ingestTimeStr); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IndexFreshness{Fresh: false, Reason: "read_ingest_watermark_failed"}, fmt.Errorf("read %s watermark: %w", MetaLastIngestTime, err)
	}
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", MetaLastIngestCatalogMTime).Scan(&catMTimeStr); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IndexFreshness{Fresh: false, Reason: "read_catalog_watermark_failed"}, fmt.Errorf("read %s watermark: %w", MetaLastIngestCatalogMTime, err)
	}

	if ingestTimeStr == "" && catMTimeStr == "" {
		return IndexFreshness{Fresh: false, Reason: "no_ingest_watermark"}, nil
	}

	catDir := paths.CatalogDir()
	st, err := os.Stat(catDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Catalog does not exist on disk (hooks absent); missing signal must report not fresh.
			return IndexFreshness{Fresh: false, Reason: "catalog_dir_missing"}, nil
		}
		return IndexFreshness{Fresh: false, Reason: "catalog_stat_failed"}, fmt.Errorf("stat catalog directory %q: %w", catDir, err)
	}

	curCatMTime := mtimeOf(st)
	if catMTimeStr != "" {
		lastCatMTime, pErr := strconv.ParseFloat(catMTimeStr, 64)
		if pErr == nil {
			if curCatMTime > lastCatMTime+0.001 {
				return IndexFreshness{Fresh: false, Reason: "catalog_modified_after_ingest"}, nil
			}
			return IndexFreshness{Fresh: true}, nil
		}
	}

	if ingestTimeStr != "" {
		lastIngestTime, pErr := strconv.ParseFloat(ingestTimeStr, 64)
		if pErr == nil {
			if curCatMTime > lastIngestTime+0.001 {
				return IndexFreshness{Fresh: false, Reason: "catalog_newer_than_ingest"}, nil
			}
			return IndexFreshness{Fresh: true}, nil
		}
	}

	return IndexFreshness{Fresh: true}, nil
}

// CheckProjectFreshness checks whether a specific project's transcript files have changed
// since they were last indexed into the consolidated store.
// Missing signal (e.g. hooks absent, unindexed transcripts, or missing watermarks) reports Fresh: false.
func CheckProjectFreshness(con *sql.DB, projectLabel, tdir string, sourceTool ...string) (IndexFreshness, error) {
	if con == nil {
		return IndexFreshness{Fresh: false, Reason: "no_connection"}, nil
	}
	globalFresh, err := CheckIndexFreshness(con)
	if err != nil || !globalFresh.Fresh {
		return globalFresh, err
	}
	if tdir == "" {
		return IndexFreshness{Fresh: true}, nil
	}
	_, err = os.Stat(tdir)
	if err != nil {
		if os.IsNotExist(err) {
			return IndexFreshness{Fresh: true}, nil
		}
		return IndexFreshness{Fresh: false, Reason: "stat_tdir_failed"}, fmt.Errorf("stat project directory %q: %w", tdir, err)
	}
	selectedSource := ""
	if len(sourceTool) > 0 {
		selectedSource = sourceTool[0]
	}

	rows, err := con.Query(`
		SELECT path, mtime, size, fp
		FROM file_index
		WHERE (? = '' AND path LIKE ?)
		   OR session_id IN (
				SELECT id FROM sessions
				WHERE project = ? AND (? = '' OR source_tool = ?)
			)
	`, selectedSource, tdir+"/%", projectLabel, selectedSource, selectedSource)
	if err != nil {
		return IndexFreshness{Fresh: false, Reason: "query_file_index_failed"}, err
	}
	defer rows.Close()

	nRows := 0
	for rows.Next() {
		nRows++
		var (
			p  string
			mt float64
			sz int64
			fp string
		)
		if err := rows.Scan(&p, &mt, &sz, &fp); err != nil {
			return IndexFreshness{Fresh: false, Reason: "scan_file_index_failed"}, err
		}
		rawPath := backingFilePath(p)
		curMTime, curSize, curFP, statErr := backingFileState(rawPath)
		if statErr != nil {
			return IndexFreshness{Fresh: false, Reason: "transcript_stat_changed"}, nil
		}
		if absDiff(mt, curMTime) >= 0.001 || sz != curSize || (fp != "" && fp != curFP) {
			return IndexFreshness{Fresh: false, Reason: "transcript_content_changed"}, nil
		}
	}
	if err := rows.Err(); err != nil {
		return IndexFreshness{Fresh: false, Reason: "iterate_file_index_failed"}, err
	}
	if nRows == 0 {
		entries, err := os.ReadDir(tdir)
		if err != nil {
			return IndexFreshness{Fresh: false, Reason: "read_project_dir_failed"}, fmt.Errorf("read project directory %q: %w", tdir, err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				return IndexFreshness{Fresh: false, Reason: "unindexed_transcripts_exist"}, nil
			}
		}
	}

	return IndexFreshness{Fresh: true}, nil
}

// SessionFreshnessStatus discriminates the freshness of an individual session.
type SessionFreshnessStatus int

const (
	SessionFresh SessionFreshnessStatus = iota
	SessionStale
	SessionMissingBacking
	SessionNotFound
)

// SessionFreshness contains the detailed outcome of an O(1) session freshness check.
type SessionFreshness struct {
	Status     SessionFreshnessStatus
	SessionID  string
	SourcePath string
	Note       string
}

// CheckSessionFreshness checks a specific session's freshness in O(1) by comparing
// its stored file_index watermark against a single stat of its backing transcript.
func CheckSessionFreshness(con *sql.DB, sessionID string) (SessionFreshness, error) {
	var (
		fullID       string
		sourcePath   sql.NullString
		missingSince sql.NullFloat64
		mtime        sql.NullFloat64
		size         sql.NullInt64
		fp           sql.NullString
	)

	err := con.QueryRow(`
		SELECT s.id, s.source_path, s.missing_since, f.mtime, f.size, f.fp
		FROM sessions s
		LEFT JOIN file_index f ON (
			f.path = s.source_path
			OR (instr(s.source_path, '#') > 0 AND f.path = substr(s.source_path, 1, instr(s.source_path, '#') - 1))
		)
		WHERE s.id = ? OR s.id LIKE ?
		ORDER BY (s.id = ?) DESC, (f.path = s.source_path) DESC, LENGTH(s.id) ASC
		LIMIT 1
	`, sessionID, sessionID+"%", sessionID).Scan(&fullID, &sourcePath, &missingSince, &mtime, &size, &fp)

	if errors.Is(err, sql.ErrNoRows) {
		return SessionFreshness{Status: SessionNotFound, SessionID: sessionID}, nil
	}
	if err != nil {
		return SessionFreshness{Status: SessionNotFound, SessionID: sessionID}, fmt.Errorf("query session watermark: %w", err)
	}

	if missingSince.Valid && missingSince.Float64 > 0 {
		return SessionFreshness{
			Status:     SessionMissingBacking,
			SessionID:  fullID,
			SourcePath: sourcePath.String,
			Note:       "source file gone — retained history",
		}, nil
	}

	if !sourcePath.Valid || sourcePath.String == "" {
		return SessionFreshness{
			Status:     SessionMissingBacking,
			SessionID:  fullID,
			SourcePath: "",
			Note:       "backing transcript path unknown — answering from indexed history",
		}, nil
	}

	rawPath := backingFilePath(sourcePath.String)
	if (!mtime.Valid || !size.Valid) && rawPath != "" {
		_ = con.QueryRow("SELECT mtime, size, fp FROM file_index WHERE path = ? OR path = ? LIMIT 1", rawPath, sourcePath.String).Scan(&mtime, &size, &fp)
	}

	curMTime, curSize, curFP, statErr := backingFileState(rawPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return SessionFreshness{
				Status:     SessionMissingBacking,
				SessionID:  fullID,
				SourcePath: rawPath,
				Note:       "source file missing on disk — retained history",
			}, nil
		}
		return SessionFreshness{
			Status:     SessionStale,
			SessionID:  fullID,
			SourcePath: rawPath,
			Note:       fmt.Sprintf("cannot inspect live transcript %s: %v", rawPath, statErr),
		}, nil
	}

	if mtime.Valid && size.Valid && absDiff(mtime.Float64, curMTime) < 0.001 && size.Int64 == curSize {
		if !fp.Valid || fp.String == "" || fp.String == curFP {
			return SessionFreshness{
				Status:     SessionFresh,
				SessionID:  fullID,
				SourcePath: rawPath,
			}, nil
		}
	}

	return SessionFreshness{
		Status:     SessionStale,
		SessionID:  fullID,
		SourcePath: rawPath,
		Note:       "session may be stale (transcript updated) — run 'rawclaw ingest' to refresh",
	}, nil
}
