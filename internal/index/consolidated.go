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

	mark := ""
	if err := con.QueryRow(
		`SELECT (SELECT COUNT(*) FROM src.sessions) || ':' ||
		        (SELECT COUNT(*) FROM src.messages) || ':' ||
		        (SELECT COALESCE(MAX(id),0) FROM src.messages)`,
	).Scan(&mark); err != nil {
		return 0, true, fmt.Errorf("read source watermark: %w", err)
	}
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
