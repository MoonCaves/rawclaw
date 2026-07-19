// Package index owns ingest orchestration over the on-disk SQLite/FTS5 store:
// schema ensuring (over internal/store's DDL), file fingerprinting, incremental
// reindexing, and corpus stats. Pure-Go via modernc.org/sqlite (no cgo).
package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/retention"
	"github.com/MoonCaves/rawclaw/internal/store"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (FTS5 + bm25 + snippet)
)

// FTS5OK reports whether FTS5 is available on this build (always true for
// modernc.org/sqlite v1.45.0; kept for graceful-degrade callers).
func FTS5OK() bool {
	con, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return false
	}
	defer con.Close()
	if _, err := con.Exec("CREATE VIRTUAL TABLE t USING fts5(x)"); err != nil {
		return false
	}
	return true
}

// ArchiveDBPrefix namespaces the cache dbs of ARCHIVE-replica scopes (foreign
// machines' sessions pulled through the transcript archive). The prefix is
// what keeps those read-only replicas out of the local orphan-db discovery
// and out of the delete path's retained-row scan; local db names are
// path-encodings of absolute dirs (or "codex-..."), so no local scope can
// collide with it.
const ArchiveDBPrefix = "archive-"

// DBPath returns the cache db path for a transcript dir:
// ~/.cache/session-search/<encoded-dir>.db (creating the dir).
func DBPath(transcriptDir string) string {
	enc := filepath.Base(filepath.Clean(transcriptDir))
	return filepath.Join(store.CacheDir(), enc+".db")
}

// EnsureSchema creates the base schema, the FTS table if missing, and rebuilds
// on any SchemaVersion mismatch or missing marker. sourceID is the scope's source
// ("claude"/"codex"), used only to backfill source_tool on an in-place durability
// migration (D6).
func EnsureSchema(con *sql.DB, sourceID string) error {
	// Read the schema-version marker FIRST, before running the full base Schema.
	// Schema creates idx_msg_session_uuid on messages(session_id, uuid); on a
	// pre-v4 db the messages table lacks the uuid column, so running Schema first
	// would fail with "no such column: uuid" BEFORE the rebuild below could
	// migrate it. The version probe must come first. (errors.Is is no longer
	// needed: any read error — incl. a missing meta table — means rebuild.)
	var version string
	verr := con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&version)
	if verr != nil || version != strconv.Itoa(store.SchemaVersion) {
		// Missing meta table / missing marker / version mismatch / any read error
		// → full rebuild. store.Rebuild() drops every versioned object and recreates
		// the current shape (incl. the durability columns), then stamps the version.
		// The JSONL transcript is the source of truth, so a dropped cache is reindexed
		// losslessly. This IS the migration path (e.g. v3 → v4 adds messages.uuid).
		if rerr := store.Rebuild(con); rerr != nil {
			return fmt.Errorf("ensure schema rebuild: %w", rerr)
		}
		return nil
	}
	// Version already current → ensure the base schema + FTS are present
	// (idempotent; covers a current db that somehow lost its FTS table).
	if _, err := con.Exec(store.Schema); err != nil {
		return fmt.Errorf("ensure base schema: %w", err)
	}
	// Add the durable-retention columns in place if a current-version db predates
	// them (D6) — WITHOUT bumping SchemaVersion (a bump would rebuild + re-prune).
	if err := migrateDurabilityColumns(con, sourceID); err != nil {
		return fmt.Errorf("ensure durability columns: %w", err)
	}
	if _, err := con.Exec("SELECT 1 FROM messages_fts LIMIT 1"); err != nil {
		_, _ = con.Exec(store.FTSSQL) // best-effort; raced creation is acceptable
	}
	return nil
}

// durabilityColumns are the D3 provenance/retention columns. They live in Schema
// (fresh/rebuilt dbs) and are added in place to an existing current-version db by
// migrateDurabilityColumns.
var durabilityColumns = []struct{ name, decl string }{
	{"origin_machine", "origin_machine TEXT"},
	{"source_tool", "source_tool TEXT"},
	{"source_path", "source_path TEXT"},
	{"missing_since", "missing_since REAL"},
}

// migrateDurabilityColumns adds any missing D3 column to the sessions table via
// idempotent, PRAGMA-guarded ALTER TABLE, then backfills any row still missing
// provenance — origin_machine = this machine, source_tool = the scope's source,
// source_path = the session's known backing path (file_index.path),
// missing_since = NULL. It deliberately does NOT bump SchemaVersion: that would
// trigger a full rebuild from source, re-walking the live tree and re-pruning
// every already-retained session — exactly the loss durable retention exists to
// prevent (D6). A fresh or rebuilt db already carries the columns via Schema, so
// this is a no-op there.
//
// Kill-safety (F3): the backfill runs off the ROW STATE (any origin_machine
// still NULL), never off an in-call "did I just ADD a column?" flag. A process
// killed after the ALTER TABLEs commit but before the UPDATE runs leaves a db
// with every column already present and every row still NULL; gating the
// backfill on "added this call" would see the columns and skip it forever. The
// WHERE clause below re-detects that pending state on the very next call and
// completes it, so a rerun after a kill at any step boundary finishes the job.
func migrateDurabilityColumns(con *sql.DB, sourceID string) error {
	have, err := sessionColumns(con)
	if err != nil {
		return err
	}
	for _, c := range durabilityColumns {
		if _, ok := have[c.name]; ok {
			continue
		}
		if _, err := con.Exec("ALTER TABLE sessions ADD COLUMN " + c.decl); err != nil {
			return fmt.Errorf("add sessions.%s: %w", c.name, err)
		}
	}
	// Unconditional, idempotent backfill: a no-op UPDATE (matches zero rows) once
	// every row is already stamped, but closes the gap left by a kill between the
	// ADD COLUMNs above and a prior run's UPDATE. A row with no file_index
	// watermark simply stays NULL on source_path; missing_since stays NULL — an
	// existing session is present until a scan proves otherwise.
	if _, err := con.Exec(
		`UPDATE sessions
		    SET origin_machine = ?,
		        source_tool = ?,
		        source_path = (SELECT path FROM file_index WHERE file_index.session_id = sessions.id)
		  WHERE origin_machine IS NULL`,
		provenance.MachineID(), sourceID,
	); err != nil {
		return fmt.Errorf("backfill provenance: %w", err)
	}
	return nil
}

// sessionColumns returns the set of column names on the sessions table (via
// PRAGMA table_info), used to guard the additive migration.
func sessionColumns(con *sql.DB) (map[string]struct{}, error) {
	rows, err := con.Query("PRAGMA table_info(sessions)")
	if err != nil {
		return nil, fmt.Errorf("pragma table_info(sessions): %w", err)
	}
	defer rows.Close()
	have := map[string]struct{}{}
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info: %w", err)
		}
		have[name] = struct{}{}
	}
	return have, rows.Err()
}

// sourceClaude is the source id stamped by the Claude directory-walk ingest
// (UpdateIndex/ReindexFile). That path is inherently Claude — it parses Claude's
// JSONL and subagents/ layout — so its source is a constant, not injected. The
// generalized container path injects its source id alongside its MessagesFunc.
const sourceClaude = "claude"

// reindexRow is one parsed message ready for insertion.
type reindexRow struct {
	role    string
	content string
	ts      float64
	tsISO   string
	uuid    string
}

// originOr resolves the origin_machine to stamp: an explicit origin (a
// replicated tree owned by another machine) wins; "" means this machine.
func originOr(origin string) string {
	if origin == "" {
		return provenance.MachineID()
	}
	return origin
}

// ReindexFile parses the whole file into memory FIRST, then atomically replaces
// this session's rows (an I/O failure can't commit away existing data). Returns
// true on success. Rows are stamped with this machine's identity; a replicated
// tree goes through reindexFileWithOrigin instead.
func ReindexFile(con *sql.DB, path, transcriptDir string) bool {
	return reindexFileWithOrigin(con, path, transcriptDir, "")
}

// reindexFileWithOrigin is ReindexFile with an explicit origin_machine ("" = this
// machine) — the provenance seam the archive-scope ingest stamps foreign
// machine ids through.
func reindexFileWithOrigin(con *sql.DB, path, transcriptDir, origin string) bool {
	sid, isSub, parent := provenance.SessionIDFor(path, transcriptDir)

	rows, started, last, ok := parseTranscript(path, sid)
	if !ok {
		return false // parse failed -> leave existing rows + watermark untouched
	}

	// parse succeeded -> atomically replace this session's rows.
	if _, err := con.Exec("DELETE FROM messages WHERE session_id=?", sid); err != nil {
		return false
	}
	if _, err := con.Exec("DELETE FROM sessions WHERE id=?", sid); err != nil {
		return false
	}
	for _, r := range rows {
		if _, err := con.Exec(
			"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
			sid, r.role, r.content, r.ts, r.tsISO, r.uuid,
		); err != nil {
			return false
		}
	}
	var parentArg any
	if parent != "" {
		parentArg = parent
	} // else nil -> SQL NULL for a missing parent
	// Stamp provenance (D3) and clear missing_since — a freshly (re)indexed
	// session is present by definition, so a reappeared source file un-flags here.
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,origin_machine,source_tool,source_path,missing_since) VALUES(?,?,?,?,?,?,?,?,?,NULL)",
		sid, started, last, len(rows), isSub, parentArg, originOr(origin), sourceClaude, realpath(path),
	); err != nil {
		return false
	}
	return true
}

// parseTranscript reads and flattens one JSONL transcript into rows, computing
// the started/last timestamp watermarks. Returns ok=false if the file cannot be
// opened (a parse-time read error), leaving existing rows untouched. Malformed
// individual lines are skipped.
func parseTranscript(path, sid string) (rows []reindexRow, started, last float64, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, false
	}
	// []byte is already lossless and json.Unmarshal tolerates invalid UTF-8 in
	// strings, so no transform is needed.
	var startedSet, lastSet bool
	for _, line := range splitLines(data) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var o map[string]any
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			continue // skip malformed / incomplete trailing line
		}
		if !indexable(o) {
			continue
		}
		text := parse.ExtractText(o)
		if text == "" {
			continue
		}
		iso, _ := o["timestamp"].(string)
		ts := parse.ISOToEpoch(iso)
		rows = append(rows, reindexRow{role: parse.MsgRole(o), content: text, ts: ts, tsISO: iso, uuid: parse.MsgUUID(o)})
		if ts != 0 {
			if !startedSet || ts < started {
				started, startedSet = ts, true
			}
			if !lastSet || ts > last {
				last, lastSet = ts, true
			}
		}
	}
	return rows, started, last, true
}

// indexable reports whether o's "type" is in parse.IndexableTypes.
func indexable(o map[string]any) bool {
	t, _ := o["type"].(string)
	for _, it := range parse.IndexableTypes {
		if t == it {
			return true
		}
	}
	return false
}

// splitLines splits on "\n" (each line is then stripped by the caller). A
// trailing newline yields a final empty segment, which the caller skips after
// TrimSpace.
func splitLines(data []byte) []string {
	return strings.Split(string(data), "\n")
}

// fileMeta is the file_index watermark row.
type fileMeta struct {
	mtime float64
	size  int64
	fp    string
}

// UpdateIndex performs the incremental reindex of transcriptDir: fingerprint
// each contained file, reindex changed ones, prune deleted sessions. Writes
// commit under database/sql autocommit.
func UpdateIndex(con *sql.DB, transcriptDir string) error {
	return updateIndexWithOrigin(con, transcriptDir, "")
}

// updateIndexWithOrigin is UpdateIndex with an explicit origin_machine ("" = this
// machine) stamped onto every (re)indexed session — the archive-scope path.
func updateIndexWithOrigin(con *sql.DB, transcriptDir, origin string) error {
	files := paths.ContainedJSONL(transcriptDir)

	onDisk := make(map[string]struct{}, len(files))
	for _, f := range files {
		onDisk[realpath(f)] = struct{}{}
	}

	// Consult the lifecycle tombstone ONCE so a user-deleted session is not
	// resurrected on reindex. cacheDir "" resolves to lifecycle's default
	// (~/.cache/session-search) — the same cache dir DBPath uses, so the
	// tombstone sidecar and the cache db live together. LoadTombstones never
	// returns a nil map (a missing sidecar yields an empty set), and a read
	// error is non-fatal: degrade to "nothing tombstoned" rather than block the
	// whole index pass on a malformed sidecar.
	tombstoned, terr := lifecycle.LoadTombstones("")
	if terr != nil {
		tombstoned = map[string]struct{}{} // best-effort: never block indexing
	}

	cur, err := loadFileIndex(con)
	if err != nil {
		return fmt.Errorf("load file_index: %w", err)
	}

	for _, f := range files {
		rp := realpath(f)
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		// Skip a tombstoned session: its file may have been re-created (or never
		// removed from disk), but the user deleted it — honor that across reindex.
		if sid, _, _ := provenance.SessionIDFor(f, transcriptDir); isMember(tombstoned, sid) {
			continue
		}
		mtime := mtimeOf(st)
		size := st.Size()
		if prev, found := cur[rp]; found {
			if absDiff(prev.mtime, mtime) < 0.001 && prev.size == size {
				if prev.fp == provenance.FileFingerprint(f, size) {
					continue // genuinely unchanged
				}
			}
		}
		if reindexFileWithOrigin(con, f, transcriptDir, origin) {
			sid, _, _ := provenance.SessionIDFor(f, transcriptDir)
			if _, err := con.Exec(
				"INSERT OR REPLACE INTO file_index(path,mtime,size,fp,session_id) VALUES(?,?,?,?,?)",
				rp, mtime, size, provenance.FileFingerprint(f, size), sid,
			); err != nil {
				return fmt.Errorf("update file_index: %w", err)
			}
		}
	}

	// Retention pass (replaces the old "absent from the walk → DELETE" prune): an
	// absent own-source file is flagged missing_since and RETAINED; only an
	// explicit tombstone deletes; a foreign-origin row is never a candidate
	// (D1/D2/D5). An ARCHIVE-replica scan (origin set — the tree is a synced
	// copy inside the archive clone) instead treats absence as authoritative:
	// the owner's delete propagated through the archive (E5), so the replica
	// rows die rather than resurrect the session in search.
	if err := retention.ReconcileRetention(con, onDisk, tombstoned, nowEpoch(), retention.RetentionMirror(), origin != ""); err != nil {
		return err
	}
	return nil
}

// nowEpoch is the current time as fractional Unix seconds (the missing_since /
// mtime unit).
func nowEpoch() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// ReconcileOrphanDB reconciles an existing index db whose source dir has vanished
// entirely — the 30-day-purge case where AllProjectDirs no longer yields the
// project, so the normal source→index pass never runs for it (D8). It reconciles
// against an EMPTY live scan: every own-source session is stamped missing_since
// and RETAINED, an explicit tombstone deletes, a foreign row is untouched — the
// same rules as an in-place UpdateIndex, minus the reindex (there is no source to
// read). Returns the surviving top-level session count so the caller can drop a
// db that reads as fully deleted. A busy/locked db is a soft no-op that degrades
// to the current read count rather than erroring the whole discovery pass.
func ReconcileOrphanDB(dbp string) (nSessions int, err error) {
	con, openErr := store.ConnectRW(dbp)
	if openErr != nil {
		return store.CountTopLevelSessions(dbp), nil // can't write — fall back to a read count
	}
	defer con.Close()

	if err := EnsureSchema(con, sourceClaude); err != nil {
		if isBusy(err) {
			return store.CountTopLevelSessions(dbp), nil
		}
		return 0, fmt.Errorf("orphan ensure schema: %w", err)
	}
	tombstoned, terr := lifecycle.LoadTombstones("")
	if terr != nil {
		tombstoned = map[string]struct{}{} // best-effort: never block discovery
	}
	// Empty onDisk: the whole source is gone, so every backing file is "absent".
	// mirror=false ALWAYS: the mirror setting governs live scans; an orphaned
	// archive's retained rows are removed only by explicit tombstone (D5) — a
	// search run with RAWCLAW_RETENTION=mirror must never wipe them.
	// replica=false too: this pass covers LOCAL orphaned dbs (archive-replica
	// dbs are excluded from orphan discovery by their name prefix), and an
	// empty scan under replica semantics would wipe the db wholesale.
	if err := retention.ReconcileRetention(con, map[string]struct{}{}, tombstoned, nowEpoch(), false, false); err != nil {
		if isBusy(err) {
			return store.CountTopLevelSessions(dbp), nil
		}
		return 0, fmt.Errorf("orphan reconcile: %w", err)
	}
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE is_subagent=0").Scan(&n); err != nil {
		if isBusy(err) {
			return store.CountTopLevelSessions(dbp), nil
		}
		return 0, fmt.Errorf("orphan count: %w", err)
	}
	return n, nil
}

// EnsureOrphanReconciled reconciles an orphaned index db read-MOSTLY:
// a read-only probe first decides whether a reconcile would change anything —
// a tombstoned session still present, an own-source row not yet stamped
// missing_since, or (mirror mode) an own-source row awaiting the prune. Only
// pending work opens the db read-write (ReconcileOrphanDB); the common case —
// re-discovering an already-reconciled archive on every search — is a pure
// read that never touches the file. A probe failure (e.g. a pre-durability
// schema without the provenance columns) falls through to the read-write
// reconcile, whose EnsureSchema migrates it.
func EnsureOrphanReconciled(dbp string) (int, error) {
	tombstoned, terr := lifecycle.LoadTombstones("")
	if terr != nil {
		tombstoned = map[string]struct{}{} // best-effort: never block discovery
	}
	pending, n, err := orphanWorkPending(dbp, tombstoned)
	if err != nil || pending {
		return ReconcileOrphanDB(dbp)
	}
	return n, nil
}

// orphanWorkPending answers, from a read-only connection, whether a reconcile
// pass would change this db, plus the current surviving top-level count.
func orphanWorkPending(dbp string, tombstoned map[string]struct{}) (pending bool, n int, err error) {
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return false, 0, err
	}
	defer con.Close()
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE is_subagent=0").Scan(&n); err != nil {
		return false, 0, fmt.Errorf("orphan probe count: %w", err)
	}
	rows, err := con.Query("SELECT id, origin_machine, missing_since FROM sessions")
	if err != nil {
		return false, 0, fmt.Errorf("orphan probe scan: %w", err)
	}
	defer rows.Close()
	mid := provenance.MachineID()
	for rows.Next() {
		var id string
		var origin sql.NullString
		var missing sql.NullFloat64
		if err := rows.Scan(&id, &origin, &missing); err != nil {
			return false, 0, fmt.Errorf("orphan probe row: %w", err)
		}
		// Same tree as the acting reconcile, against an empty live scan
		// (present=false — the whole source is gone) with mirror=false and
		// replica=false (matching ReconcileOrphanDB: retained rows die only
		// by tombstone). Any predicted action is pending work.
		own := !origin.Valid || origin.String == mid
		if retention.DecideRetention(false, isMember(tombstoned, id), own, missing.Valid, false, false) != retention.ActNone {
			return true, n, nil
		}
	}
	return false, n, rows.Err()
}

// loadFileIndex reads the file_index watermark rows keyed by path.
func loadFileIndex(con *sql.DB) (map[string]fileMeta, error) {
	rows, err := con.Query("SELECT path,mtime,size,fp FROM file_index")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]fileMeta)
	for rows.Next() {
		var path, fp string
		var mtime float64
		var size int64
		if err := rows.Scan(&path, &mtime, &size, &fp); err != nil {
			return nil, err
		}
		out[path] = fileMeta{mtime: mtime, size: size, fp: fp}
	}
	return out, rows.Err()
}

// IndexStatus discriminates how EnsureIndexed obtained its result, so callers
// can honestly report incompleteness (#6) instead of silently treating a stale
// busy-lock fallback as a fresh index.
type IndexStatus int

const (
	// IndexFresh: the index was built/updated this call (the result is current).
	IndexFresh IndexStatus = iota
	// IndexStale: a busy/lock collision forced a fall-back to the EXISTING
	// (possibly out-of-date) cached index — the result may be incomplete.
	IndexStale
)

// EnsureIndexed builds/updates one project's FTS index and returns
// (db_path, n_sessions, status). On busy-lock it falls back to the existing
// index with CountSessions and reports IndexStale. If reindex is true and the db
// exists, it is removed first.
func EnsureIndexed(tdir string, reindex bool) (dbp string, nSessions int, status IndexStatus, err error) {
	dbp = DBPath(tdir)
	nSessions, status, err = EnsureIndexedTree(dbp, tdir, reindex, "")
	return dbp, nSessions, status, err
}

// EnsureIndexedTree builds/updates the FTS index for one Claude-shaped
// transcript tree at an EXPLICIT db path, stamping origin as every row's
// origin_machine ("" = this machine). This is EnsureIndexed with both halves of
// the identity made injectable: a replicated tree (another machine's transcripts
// synced onto this disk) indexes into its own namespaced db and carries its
// owner's identity, while the local path keeps its derived db and local stamp.
// Reindex + busy-lock semantics are identical to EnsureIndexed.
func EnsureIndexedTree(dbp, tdir string, reindex bool, origin string) (nSessions int, status IndexStatus, err error) {
	if reindex {
		if _, statErr := os.Stat(dbp); statErr == nil {
			_ = os.Remove(dbp) // best-effort; ignore a remove error
		}
	}

	con, openErr := store.ConnectRW(dbp)
	if openErr != nil {
		// Treat an open/lock failure as a fall-back to the existing index.
		return store.CountSessions(dbp), IndexStale, nil
	}
	defer con.Close()

	if err := EnsureSchema(con, sourceClaude); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexFresh, fmt.Errorf("ensure schema: %w", err)
	}
	if err := updateIndexWithOrigin(con, tdir, origin); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexFresh, fmt.Errorf("update index: %w", err)
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&nSessions); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexFresh, fmt.Errorf("count sessions: %w", err)
	}
	return nSessions, IndexFresh, nil
}

// isBusy reports whether err is a SQLite busy/locked condition.
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "(5)") || // SQLITE_BUSY
		strings.Contains(msg, "(6)") // SQLITE_LOCKED
}

// realpath resolves a path without ever erroring: it resolves the existing
// prefix and lexically appends any missing tail. Used by the paths port for
// containment checks.
func realpath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	tail := []string{}
	cur := abs
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

// isMember reports whether id is in set (comma-ok membership; a nil set is
// simply empty and never panics on read).
func isMember(set map[string]struct{}, id string) bool {
	_, ok := set[id]
	return ok
}

// absDiff returns |a-b| for the mtime equality check.
func absDiff(a, b float64) float64 {
	if a < b {
		return b - a
	}
	return a - b
}

// mtimeOf returns the file mtime as fractional Unix seconds. Sub-second
// precision is preserved so the |prev.mtime - mtime| < 0.001 unchanged-check
// works as intended.
func mtimeOf(st os.FileInfo) float64 {
	return float64(st.ModTime().UnixNano()) / 1e9
}
