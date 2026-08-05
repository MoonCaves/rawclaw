package index

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
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
	var missing []string
	for _, dbp := range dbs {
		if _, ok := folded[filepath.Base(dbp)]; !ok {
			missing = append(missing, dbp)
		}
	}
	return missing, nil
}

// SyncStats reports what one consolidation pass moved.
type SyncStats struct {
	Sources      int // per-project dbs read
	Skipped      int // of those, dbs too old to read (reported, never silent)
	SessionsSeen int // session rows offered by those dbs (sum, duplicates included)
	Sessions     int // distinct sessions in the consolidated store afterwards
	Messages     int // distinct messages in the consolidated store afterwards
}

// takeNewer is the precedence rule for two rows carrying the SAME session id —
// one session that ran in more than one project directory, which is a resume,
// not two sessions (ruling: same id == one session). A copy still present on
// disk beats a copy whose transcript was purged upstream; between two copies of
// equal presence, the longer one is the continuation and wins. It decides only
// which copy's scope/provenance to keep — no message is ever dropped by it,
// because messages merge as a union.
const takeNewer = `(
    (excluded.missing_since IS NULL AND sessions.missing_since IS NOT NULL)
 OR ((excluded.missing_since IS NULL) = (sessions.missing_since IS NULL)
     AND COALESCE(excluded.message_count,0) > COALESCE(sessions.message_count,0))
)`

// mergeSessionsSQL upserts one attached db's sessions into the consolidated
// store. Every clause is order-independent — the result is the same whichever
// order the source dbs are read in, which is what makes a partial re-run safe.
//
// message_count is carried across as the source's own count so takeNewer can
// compare like with like; the true post-merge count is recomputed once at the
// end of the pass (recountSQL), after the union of messages is known.
var mergeSessionsSQL = `
INSERT INTO main.sessions
  (id,started_at,last_ts,message_count,is_subagent,parent_id,
   origin_machine,source_tool,source_path,missing_since,project,cwd)
SELECT id,started_at,last_ts,message_count,is_subagent,parent_id,
       origin_machine,source_tool,source_path,missing_since,project,cwd
FROM src.sessions
WHERE true
ON CONFLICT(id) DO UPDATE SET
  -- Earliest start and latest activity across the copies: the session's real span.
  started_at = CASE
      WHEN COALESCE(excluded.started_at,0) > 0
       AND (COALESCE(sessions.started_at,0) = 0 OR excluded.started_at < sessions.started_at)
      THEN excluded.started_at ELSE sessions.started_at END,
  last_ts = MAX(COALESCE(sessions.last_ts,0), COALESCE(excluded.last_ts,0)),
  -- A session present anywhere is present: only "gone from every copy" is gone.
  -- Two-argument MIN() gives exactly that — it returns NULL if EITHER side is
  -- NULL, and NULL here means "still on disk". Where both copies are purged, it
  -- keeps the earlier watermark.
  missing_since = MIN(sessions.missing_since, excluded.missing_since),
  is_subagent = MAX(COALESCE(sessions.is_subagent,0), COALESCE(excluded.is_subagent,0)),
  parent_id   = COALESCE(sessions.parent_id, excluded.parent_id),
  message_count = MAX(COALESCE(sessions.message_count,0), COALESCE(excluded.message_count,0)),
  -- Scope and provenance follow the winning copy, but a known value never
  -- loses to an unknown one.
  project = CASE WHEN excluded.project IS NOT NULL
                  AND (sessions.project IS NULL OR ` + takeNewer + `)
                 THEN excluded.project ELSE sessions.project END,
  cwd = CASE WHEN excluded.cwd IS NOT NULL
              AND (sessions.cwd IS NULL OR ` + takeNewer + `)
             THEN excluded.cwd ELSE sessions.cwd END,
  source_path = CASE WHEN ` + takeNewer + ` THEN excluded.source_path ELSE sessions.source_path END,
  source_tool = COALESCE(sessions.source_tool, excluded.source_tool),
  origin_machine = COALESCE(sessions.origin_machine, excluded.origin_machine)
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
`

// topicNewer is the precedence rule between two taggings of the SAME segment
// (one session, one start anchor). Tagging is a re-runnable act — a segment can
// be re-tagged with a better label — so the later tagging wins. Equal or
// missing timestamps leave the copy already in the store alone, which keeps the
// pass order-independent.
const topicNewer = `COALESCE(excluded.tagged_at,0) > COALESCE(topic_segment.tagged_at,0)`

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
var mergeTopicsSQL = `
INSERT INTO main.topic_segment
  (session_id,start_uuid,end_uuid,topic,summary,tagged_at,origin_machine)
SELECT session_id,start_uuid,end_uuid,topic,summary,tagged_at,origin_machine
FROM src.topic_segment
WHERE true
ON CONFLICT(session_id,start_uuid) DO UPDATE SET
  end_uuid       = CASE WHEN ` + topicNewer + ` THEN excluded.end_uuid       ELSE topic_segment.end_uuid END,
  topic          = CASE WHEN ` + topicNewer + ` THEN excluded.topic          ELSE topic_segment.topic END,
  summary        = CASE WHEN ` + topicNewer + ` THEN excluded.summary        ELSE topic_segment.summary END,
  origin_machine = CASE WHEN ` + topicNewer + ` THEN excluded.origin_machine ELSE topic_segment.origin_machine END,
  tagged_at      = MAX(COALESCE(topic_segment.tagged_at,0), COALESCE(excluded.tagged_at,0))
`

// mergeVerdictsSQL folds the session-verdict sidecar in on the same rule: one
// row per session, the later verdict wins. It rides with the topic merge
// because it is the same tagging act's other half.
const mergeVerdictsSQL = `
INSERT INTO main.session_verdict(session_id,verdict,source,origin_machine,tagged_at)
SELECT session_id,verdict,source,origin_machine,tagged_at FROM src.session_verdict
WHERE true
ON CONFLICT(session_id) DO UPDATE SET
  verdict        = CASE WHEN COALESCE(excluded.tagged_at,0) > COALESCE(session_verdict.tagged_at,0) THEN excluded.verdict        ELSE session_verdict.verdict END,
  source         = CASE WHEN COALESCE(excluded.tagged_at,0) > COALESCE(session_verdict.tagged_at,0) THEN excluded.source         ELSE session_verdict.source END,
  origin_machine = CASE WHEN COALESCE(excluded.tagged_at,0) > COALESCE(session_verdict.tagged_at,0) THEN excluded.origin_machine ELSE session_verdict.origin_machine END,
  tagged_at      = MAX(COALESCE(session_verdict.tagged_at,0), COALESCE(excluded.tagged_at,0))
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
func ConsolidateFrom(srcPaths []string, rebuild bool) (SyncStats, error) {
	var st SyncStats
	dst := ConsolidatedPath()
	if rebuild {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(dst + suffix) // best-effort; a missing file is the goal state
		}
	}

	con, err := store.ConnectRW(dst)
	if err != nil {
		return st, fmt.Errorf("open consolidated store: %w", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		return st, fmt.Errorf("ensure consolidated schema: %w", err)
	}
	// The topic layer is gated separately from the keyword schema, so it has to
	// be asked for explicitly. Creating it unconditionally means a topic query
	// against the one store can answer "nothing is tagged yet" instead of
	// failing on a missing table.
	if err := store.EnsureTopicSchema(con); err != nil {
		return st, fmt.Errorf("ensure consolidated topic schema: %w", err)
	}

	for _, src := range srcPaths {
		n, skipped, err := consolidateOne(con, src)
		if err != nil {
			return st, fmt.Errorf("consolidate %s: %w", filepath.Base(src), err)
		}
		st.Sources++
		if skipped {
			st.Skipped++
		}
		st.SessionsSeen += n
	}

	if _, err := con.Exec(recountSQL); err != nil {
		return st, fmt.Errorf("recount messages: %w", err)
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
	return st, nil
}

// SyncConsolidatedFrom folds ONE per-project db into the consolidated store.
// This is the write-through half: an indexing run updates its own project db,
// then hands that db here, so the consolidated store tracks without a separate
// pass over the transcripts.
func SyncConsolidatedFrom(srcPath string) error {
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		return fmt.Errorf("open consolidated store: %w", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		return fmt.Errorf("ensure consolidated schema: %w", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		return fmt.Errorf("ensure consolidated topic schema: %w", err)
	}
	if _, _, err := consolidateOne(con, srcPath); err != nil {
		return fmt.Errorf("consolidate %s: %w", filepath.Base(srcPath), err)
	}
	if _, err := con.Exec(recountSQL + " WHERE id IN (SELECT id FROM sessions)"); err != nil {
		return fmt.Errorf("recount messages: %w", err)
	}
	return pruneTombstoned(con)
}

// writeThroughConsolidated folds a per-project db into the consolidated store
// right after an indexing run updated it, so the one store tracks the many
// without a second pass over the transcripts. Call it AFTER the indexing
// connection is closed.
//
// It is advisory in both directions: a failed index run is not folded in (its
// db is in an unknown state), and a failed fold-in is logged, never returned.
// The consolidated store is a derived artifact — `rawclaw consolidate` rebuilds
// it from the same sources — so a lost update is a stale cache, not lost data,
// and it must never be the reason a search fails.
func writeThroughConsolidated(dbp string, indexErr error) {
	if indexErr != nil || IsConsolidatedDB(dbp) {
		return
	}
	if err := SyncConsolidatedFrom(dbp); err != nil {
		slog.Debug("consolidate: write-through failed", "db", filepath.Base(dbp), "err", err)
	}
}

// consolidateOne attaches src, merges its sessions and messages, and detaches.
// It returns the number of session rows the source offered (before merging), so
// a caller can report how many rows collapsed into how many sessions, and
// whether the source was skipped as unreadable.
//
// The source is attached READ-ONLY: an indexing run may hold the writer lock on
// it, and consolidation must never be the thing that blocks indexing. The one
// write it makes to a source is the additive column migration below, on its own
// connection, before the read-only attach.
func consolidateOne(con *sql.DB, src string) (offered int, skipped bool, err error) {
	if _, err := os.Stat(src); err != nil {
		return 0, true, fmt.Errorf("source unreadable: %w", err)
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
		return 0, true, fmt.Errorf("attach: %w", err)
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
		return 0, true, cErr
	}

	// Whether this source has a topic layer at all: the topic tables are created
	// on demand by the tagging path, so a project nobody has tagged simply has
	// no such table and its merge steps are skipped.
	hasTopics, err := srcHasTable(con, "topic_segment")
	if err != nil {
		return 0, true, err
	}
	hasVerdicts, err := srcHasTable(con, "session_verdict")
	if err != nil {
		return 0, true, err
	}

	mark := ""
	if err := con.QueryRow(
		`SELECT (SELECT COUNT(*) FROM src.sessions) || ':' ||
		        (SELECT COUNT(*) FROM src.messages) || ':' ||
		        (SELECT COALESCE(MAX(id),0) FROM src.messages)`,
	).Scan(&mark); err != nil {
		return 0, true, fmt.Errorf("read source watermark: %w", err)
	}
	// The topic layer joins the watermark, because tagging changes a source
	// without touching a single session or message row. Left out, a re-tagged
	// project would read as unchanged and its new labels would never arrive.
	// The latest tagging time rides along with the count: re-tagging a segment
	// in place replaces a label without changing how many there are, so a count
	// alone would still miss it.
	topicMark := "0"
	if hasTopics {
		if err := con.QueryRow(
			`SELECT COUNT(*) || '@' || COALESCE(MAX(tagged_at),0) FROM src.topic_segment`,
		).Scan(&topicMark); err != nil {
			return 0, true, fmt.Errorf("read source topic watermark: %w", err)
		}
	}
	mark += ":t" + topicMark
	if err := con.QueryRow("SELECT COUNT(*) FROM src.sessions").Scan(&offered); err != nil {
		return 0, true, fmt.Errorf("count source sessions: %w", err)
	}

	// Skip a source that has not changed since its last fold-in. Without this
	// the write-through would re-scan every message of every project on every
	// invocation, which is the cost the per-project layout was hiding.
	key := syncMarkKey(src)
	var prev string
	switch err := con.QueryRow("SELECT value FROM meta WHERE key=?", key).Scan(&prev); {
	case err == nil && prev == mark:
		return offered, false, nil
	case err != nil && err != sql.ErrNoRows:
		return 0, true, fmt.Errorf("read sync watermark: %w", err)
	}

	if _, err := con.Exec(mergeSessionsSQL); err != nil {
		return 0, true, fmt.Errorf("merge sessions: %w", err)
	}
	if _, err := con.Exec(mergeMessagesSQL); err != nil {
		return 0, true, fmt.Errorf("merge messages: %w", err)
	}
	if hasTopics {
		if _, err := con.Exec(mergeTopicsSQL); err != nil {
			return 0, true, fmt.Errorf("merge topics: %w", err)
		}
	}
	if hasVerdicts {
		if _, err := con.Exec(mergeVerdictsSQL); err != nil {
			return 0, true, fmt.Errorf("merge verdicts: %w", err)
		}
	}
	if _, err := con.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES(?,?)", key, mark); err != nil {
		return 0, true, fmt.Errorf("stamp sync watermark: %w", err)
	}
	return offered, false, nil
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

// syncMarkKey names the meta row holding a source db's last-folded-in
// watermark. Keyed by file name, so a source that is rebuilt from scratch (new
// row ids, new counts) re-folds rather than being mistaken for unchanged.
func syncMarkKey(src string) string { return "sync:" + filepath.Base(src) }

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
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, fmt.Errorf("look for source table %s: %w", name, err)
	}
	return true, nil
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
func pruneTombstoned(con *sql.DB) error {
	tombstoned, err := lifecycle.LoadTombstones("")
	if err != nil || len(tombstoned) == 0 {
		return nil // best-effort: a missing sidecar never blocks consolidation
	}
	for id := range tombstoned {
		// Tombstones cover a session and the subagent threads beneath it, whose
		// ids are "<parent>/<stem>".
		like := id + "/%"
		if _, err := con.Exec("DELETE FROM messages WHERE session_id = ? OR session_id LIKE ?", id, like); err != nil {
			return fmt.Errorf("prune tombstoned messages: %w", err)
		}
		if _, err := con.Exec("DELETE FROM sessions WHERE id = ? OR id LIKE ?", id, like); err != nil {
			return fmt.Errorf("prune tombstoned sessions: %w", err)
		}
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
