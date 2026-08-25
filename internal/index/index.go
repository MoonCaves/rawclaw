// Package index owns ingest orchestration over the on-disk SQLite/FTS5 store:
// schema ensuring (over internal/store's DDL), file fingerprinting, incremental
// reindexing, and corpus stats. Pure-Go via modernc.org/sqlite (no cgo).
package index

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/retention"
	"github.com/MoonCaves/rawclaw/internal/source"
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
		// A rebuilt db already has the substring index (store.Rebuild creates it)
		// over an empty messages table, so this only stamps the backfill marker.
		// Without the stamp the very next call would read the db as un-backfilled
		// and re-fill a freshly indexed corpus from scratch.
		if terr := migrateTrigramIndex(con); terr != nil {
			return fmt.Errorf("ensure trigram index: %w", terr)
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
	// Same treatment for the scope columns (project/cwd) — in place, no version
	// bump, for the same reason.
	if err := migrateScopeColumns(con); err != nil {
		return fmt.Errorf("ensure scope columns: %w", err)
	}
	if _, err := con.Exec("SELECT 1 FROM messages_fts LIMIT 1"); err != nil {
		_, _ = con.Exec(store.FTSSQL) // best-effort; raced creation is acceptable
	}
	// Same treatment again for the substring index: added in place, no version
	// bump.
	if err := migrateTrigramIndex(con); err != nil {
		return fmt.Errorf("ensure trigram index: %w", err)
	}
	return nil
}

// trigramBackfillKey marks the substring index as fully populated from the
// messages that were already in the db when it was added.
const trigramBackfillKey = "trigram_backfill_done"

// trigramWatermarkKey holds how far an unfinished backfill got: the highest
// messages.id already copied into the substring index. It is the resume point,
// and it exists only while the backfill is in progress.
const trigramWatermarkKey = "trigram_backfill_at"

// trigramBatch is how many messages one backfill transaction copies. It is the
// unit of work a kill can cost, and the reason the size is modest: measured on
// a corpus of ~345k messages the whole backfill takes ~52s, which is longer
// than the CLI's own 30s watchdog, so the pass MUST survive being killed
// part-way. At this size a batch is a few seconds, so a killed run loses
// seconds of work and the next run resumes where it stopped.
const trigramBatch = 20000

// migrateTrigramIndex adds the substring index to an existing current-version db
// and fills it from the rows already there, in place and WITHOUT bumping
// SchemaVersion — for exactly the reason the durability and scope columns do not
// bump it. A bump sends every db down store.Rebuild, which drops the messages
// table and forces a re-walk of the live transcript tree; every session retained
// after its source transcript was purged upstream would be re-pruned on that
// walk. Durable retention exists to survive that, and a new search index is not
// worth spending it. The substring index is fully derivable from messages, which
// is what makes building it in place possible at all.
//
// Kill-safety (F3): the fill runs in batches, and each batch commits its rows
// and its resume point in ONE transaction. A process killed part-way therefore
// leaves a db whose watermark describes exactly what is in the index, and the
// next call carries on from there instead of starting over. That property is
// not decorative here: the backfill on a large corpus takes longer than the
// CLI's own watchdog allows a single run to live, so a pass that could only
// start over would be killed at the same point forever and never finish.
//
// The done marker is stamped only after the last batch. It is what keeps the
// steady-state cost at one meta read: without it, every invocation would have
// to ask the db how far the fill got.
func migrateTrigramIndex(con *sql.DB) error {
	if err := clearOrphanedTrigramMarker(con); err != nil {
		return err
	}
	if _, err := con.Exec(store.TrigramSQL); err != nil {
		return fmt.Errorf("create trigram index: %w", err)
	}
	var done string
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", trigramBackfillKey).Scan(&done); err == nil && done == "1" {
		return nil // already filled — the triggers keep it current from here
	}
	at, err := trigramResumePoint(con)
	if err != nil {
		return err
	}
	for {
		var bound sql.NullInt64
		if err := con.QueryRow(store.TrigramBatchBoundSQL, at, trigramBatch).Scan(&bound); err != nil {
			return fmt.Errorf("read trigram batch bound: %w", err)
		}
		if !bound.Valid {
			break // no messages left above the watermark
		}
		if err := fillTrigramBatch(con, at, bound.Int64); err != nil {
			return err
		}
		at = bound.Int64
	}
	// Done, so the resume point has nothing left to describe: drop it rather
	// than leave transient state behind in meta.
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO meta(key,value) VALUES(?,'1'); DELETE FROM meta WHERE key='"+trigramWatermarkKey+"'",
		trigramBackfillKey); err != nil {
		return fmt.Errorf("stamp %s: %w", trigramBackfillKey, err)
	}
	return nil
}

// clearOrphanedTrigramMarker forgets a done marker the db can no longer honor.
//
// A binary older than the substring index rebuilds a db by dropping messages,
// which takes OUR triggers with it, while leaving messages_fts_trigram and the
// done marker standing — its drop list never named them. What survives claims to
// be a filled index but describes messages that no longer exist, and the damage
// is not merely stale results: re-indexing restarts ids from 1, so once the new
// ids reach the old range the re-created insert trigger hits a rowid the orphan
// table already holds and the INSERT INTO messages fails outright, breaking
// indexing rather than degrading search.
//
// The triggers are what the marker actually vouches for, so a table standing
// without its triggers means the marker is lying. Clearing both meta keys sends
// this pass down the reset-and-refill path, which is the only state that is
// knowably correct. The check is two lookups in sqlite_master, which SQLite
// answers from the schema it already has in memory.
func clearOrphanedTrigramMarker(con *sql.DB) error {
	var table, trigger int
	if err := con.QueryRow(`SELECT
	  COALESCE(SUM(name='messages_fts_trigram'),0),
	  COALESCE(SUM(name='messages_tri_ai'),0)
	FROM sqlite_master WHERE name IN ('messages_fts_trigram','messages_tri_ai')`).Scan(&table, &trigger); err != nil {
		return fmt.Errorf("inspect trigram objects: %w", err)
	}
	if table == 0 || trigger == 1 {
		return nil // never created, or intact — nothing to disbelieve
	}
	if _, err := con.Exec("DELETE FROM meta WHERE key IN (?,?)", trigramBackfillKey, trigramWatermarkKey); err != nil {
		return fmt.Errorf("clear orphaned trigram markers: %w", err)
	}
	return nil
}

// trigramResumePoint reads where an interrupted backfill stopped. A missing
// watermark means no batch ever committed, so the fill starts at zero — and
// then any entries already in the index came from somewhere this function
// cannot account for, which makes emptying it the only knowably correct state.
func trigramResumePoint(con *sql.DB) (int64, error) {
	var raw string
	err := con.QueryRow("SELECT value FROM meta WHERE key=?", trigramWatermarkKey).Scan(&raw)
	if err == nil {
		if at, cerr := strconv.ParseInt(raw, 10, 64); cerr == nil {
			return at, nil
		}
	}
	if _, err := con.Exec(store.TrigramResetSQL); err != nil {
		return 0, fmt.Errorf("clear trigram index: %w", err)
	}
	return 0, nil
}

// fillTrigramBatch copies one id window into the substring index and advances
// the resume point in the SAME transaction, so the watermark can never claim
// more than the index holds.
func fillTrigramBatch(con *sql.DB, from, to int64) error {
	tx, err := con.Begin()
	if err != nil {
		return fmt.Errorf("begin trigram batch: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // no-op after a successful commit
	}()
	if _, err := tx.Exec(store.TrigramBatchFillSQL, from, to); err != nil {
		return fmt.Errorf("backfill trigram index: %w", err)
	}
	if _, err := tx.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES(?,?)",
		trigramWatermarkKey, strconv.FormatInt(to, 10)); err != nil {
		return fmt.Errorf("stamp %s: %w", trigramWatermarkKey, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit trigram batch: %w", err)
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
	// Idempotent backfill, gated on the ROW STATE (any origin_machine still NULL),
	// never on an in-call "did I just ADD a column?" flag (F3 kill-safety): a process
	// killed after the ALTER TABLEs commit but before the UPDATE runs leaves every
	// column present and every row still NULL, and a flag-gated backfill would see
	// the columns and skip it forever. The probe below re-detects that pending state
	// on the next call, so a rerun after a kill at any step boundary finishes the job.
	// A row with no file_index watermark simply stays NULL on source_path;
	// missing_since stays NULL — an existing session is present until a scan proves
	// otherwise.
	//
	// The UPDATE runs unconditionally. Its own WHERE clause is the row-state gate, so
	// on an already-stamped db it matches zero rows and costs nothing worth guarding.
	// A SELECT probe in front of it was tried and removed: it could only skip a no-op,
	// and it turned every probe-time failure — SQLITE_BUSY, an I/O error — into a
	// silent success, because a non-nil Scan error fell through to `return nil` and
	// reported a fully migrated schema to callers that then wrote against it.
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

// scopeColumns are the columns that move a session's scope off the db FILENAME
// and onto the ROW. Today the filename is exact (one db per project), which is
// precisely why it is load-bearing and a shared store is impossible; carrying
// (project, cwd) on the row — alongside origin_machine, already there — is the
// prefactor that removes the dependency.
var scopeColumns = []struct{ name, decl string }{
	{"project", "project TEXT"},
	{"cwd", "cwd TEXT"},
}

// scopeBackfillKey marks the one-time backfill of pre-existing rows as complete.
const scopeBackfillKey = "scope_backfill_done"

// inventedScopeRepairKey marks the one-time repair of the labels an earlier
// backfill invented from a date-sharded directory.
const inventedScopeRepairKey = "scope_invented_repair_done"

// migrateScopeColumns adds project/cwd to an existing current-version db and
// backfills them once, WITHOUT bumping SchemaVersion — a bump forces a rebuild
// from source, which re-walks the live tree and re-prunes every already-retained
// session, i.e. destroys exactly what durable retention exists to protect (D6).
// A fresh or rebuilt db carries the columns via Schema, so the ALTERs no-op there.
//
// The backfill reads each session's own recorded cwd from its source_path,
// memoized per directory: every session in a Claude project dir shares one cwd,
// so this is one small file read per project, not one per session. A session
// whose source file is gone (retained after an upstream purge) keeps NULL cwd
// and takes its project from the enclosing directory name — but only where that
// directory IS a project, which is the gate inside backfillScope. NULL is
// deliberate: it reads as "not known", where "" would claim the session ran
// somewhere nameless.
//
// A second pass repairs the labels an earlier binary invented before that gate
// existed; see repairInventedScope.
//
// Kill-safety (F3): each pass's completion marker is written only AFTER the pass
// commits, so a process killed mid-pass simply redoes it. The markers are what
// stop an already-complete db from re-reading files on every invocation — rows
// that legitimately resolve to nothing would otherwise be retried forever.
func migrateScopeColumns(con *sql.DB) error {
	have, err := sessionColumns(con)
	if err != nil {
		return err
	}
	for _, c := range scopeColumns {
		if _, ok := have[c.name]; ok {
			continue
		}
		if _, err := con.Exec("ALTER TABLE sessions ADD COLUMN " + c.decl); err != nil {
			return fmt.Errorf("add sessions.%s: %w", c.name, err)
		}
	}
	// The write path keeps new rows stamped, so each pass runs once and is then
	// marked done. The repair is its own step because a db backfilled by an
	// earlier binary already carries the marker for the first one.
	if err := runOnce(con, scopeBackfillKey, backfillScope); err != nil {
		return err
	}
	return runOnce(con, inventedScopeRepairKey, repairInventedScope)
}

// runOnce runs a one-time migration step unless its completion key is already
// stamped. The stamp is written only AFTER the step commits, so a process
// killed mid-pass simply redoes the pass (F3).
func runOnce(con *sql.DB, key string, step func(*sql.DB) error) error {
	var done string
	// No row means "never run" and is the normal first-time state. Any OTHER
	// read error is a real fault, and reporting it beats treating it as
	// "not run" — that reading would re-scan the whole corpus on every
	// invocation and hide the fault behind the cost.
	switch err := con.QueryRow("SELECT value FROM meta WHERE key=?", key).Scan(&done); {
	case err == nil && done == "1":
		return nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("read %s marker: %w", key, err)
	}
	if err := step(con); err != nil {
		return err
	}
	if _, err := con.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES(?,'1')", key); err != nil {
		return fmt.Errorf("stamp %s: %w", key, err)
	}
	return nil
}

// repairInventedScope clears the project labels an earlier backfill invented
// from a transcript's parent directory when that directory was a date shard
// rather than a project dir. A Codex rollout lives under YYYY/MM/DD, so those
// rows came out labeled with a day number — "09" standing in as a project name,
// answering --include-path and naming itself in the scope footer.
//
// The predicate is exactly "a label with nothing behind it": a row from a source
// other than the directory-walk ingest, carrying a project but no cwd. A session
// that DID record a working directory took its label from that cwd and keeps it.
// Clearing to NULL restores "not known", which the write path fills the next
// time that session is indexed and can prove an answer.
func repairInventedScope(con *sql.DB) error {
	if _, err := con.Exec(
		`UPDATE sessions SET project=NULL
		  WHERE source_tool IS NOT NULL AND source_tool <> ?
		    AND cwd IS NULL AND project IS NOT NULL`, sourceClaude); err != nil {
		return fmt.Errorf("repair invented scope labels: %w", err)
	}
	return nil
}

// backfillScope stamps project/cwd on every session row still missing both. It
// reads the whole candidate set before writing so the scan is not invalidated by
// its own updates.
func backfillScope(con *sql.DB) error {
	rows, err := con.Query(
		`SELECT id, source_path, COALESCE(source_tool,'') FROM sessions
		  WHERE project IS NULL AND cwd IS NULL AND source_path IS NOT NULL AND source_path <> ''`)
	if err != nil {
		return fmt.Errorf("scan sessions for scope backfill: %w", err)
	}
	type target struct{ id, path, source string }
	var todo []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.path, &t.source); err != nil {
			rows.Close()
			return fmt.Errorf("scan scope backfill row: %w", err)
		}
		todo = append(todo, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("scan sessions for scope backfill: %w", err)
	}
	rows.Close()

	byDir := map[string]projectScope{}
	for _, t := range todo {
		// The PROJECT dir, not the file's parent: a subagent or workflow
		// transcript sits in a subdirectory, and its parent's name ("subagents",
		// "wf_<id>") is not a project.
		dir := paths.ProjectDirOf(t.path)
		if dir == "" {
			// Outside the projects root. The file's parent stands in for a
			// project only when the source lays its transcripts out that way —
			// the directory-walk ingest does, and an explicit --dir tree does.
			// A source that shards by date has no project dir at all, and its
			// parent's name is a day number, not a project. The live path
			// already refuses that guess; the backfill agrees with it here.
			//
			// The gate needs the row to SAY it came from another source. An
			// unstamped row is not evidence of one, and reading it as such would
			// silently stop labeling rows this has always labeled correctly.
			if t.source != "" && t.source != sourceClaude {
				continue
			}
			dir = filepath.Dir(t.path)
		}
		scope, seen := byDir[dir]
		if !seen {
			scope = dirScope(dir)
			byDir[dir] = scope
		}
		// The dir's own cwd answers for every session in it (one dir, one working
		// directory), so the backfill reads one transcript per PROJECT rather
		// than one per session — the difference between a handful of reads and
		// thousands on a large index.
		projectArg, cwdArg := scopeOf("", scope)
		if projectArg == nil && cwdArg == nil {
			continue // nothing provable — leave NULL rather than invent a scope
		}
		if _, err := con.Exec(
			"UPDATE sessions SET project=?, cwd=? WHERE id=?", projectArg, cwdArg, t.id,
		); err != nil {
			return fmt.Errorf("backfill scope for %s: %w", t.id, err)
		}
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
// ReindexFile parses the whole file into memory FIRST, then atomically replaces
// this session's rows under a single transaction (messages, session row, watermark).
// Returns true on success. Rows are stamped with this machine's identity; a replicated
// tree goes through reindexFileWithOrigin instead.
func ReindexFile(con *sql.DB, path, transcriptDir string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	rp := realpath(path)
	return reindexFileWithOrigin(con, path, transcriptDir, "", dirScope(transcriptDir), rp, mtimeOf(st), st.Size()) == nil
}

// reindexFileWithOrigin atomically replaces one session's rows under a single
// transaction: messages, sessions row, and file_index watermark are written
// together. If any statement or the durable vault write fails, the transaction
// is rolled back. Returns an error on any failure.
func reindexFileWithOrigin(con *sql.DB, path, transcriptDir, origin string, scope projectScope, rp string, mtime float64, size int64) error {
	sid, isSub, parent := provenance.SessionIDFor(path, transcriptDir)

	rows, started, last, cwd, ok := parseTranscript(path, sid)
	if !ok {
		return nil // parse failed -> leave existing rows + watermark untouched
	}

	tx, err := con.Begin()
	if err != nil {
		return fmt.Errorf("session %s begin tx: %w", sid, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec("DELETE FROM messages WHERE session_id=?", sid); err != nil {
		return fmt.Errorf("session %s delete messages: %w", sid, err)
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE id=?", sid); err != nil {
		return fmt.Errorf("session %s delete session: %w", sid, err)
	}
	for _, r := range rows {
		if _, err := tx.Exec(
			"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
			sid, r.role, r.content, r.ts, r.tsISO, r.uuid,
		); err != nil {
			return fmt.Errorf("session %s insert message: %w", sid, err)
		}
	}
	var parentArg any
	if parent != "" {
		parentArg = parent
	} // else nil -> SQL NULL for a missing parent
	projectArg, cwdArg := scopeOf(cwd, scope)
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,origin_machine,source_tool,source_path,missing_since,project,cwd) VALUES(?,?,?,?,?,?,?,?,?,NULL,?,?)",
		sid, started, last, len(rows), isSub, parentArg, originOr(origin), sourceClaude, realpath(path), projectArg, cwdArg,
	); err != nil {
		return fmt.Errorf("session %s insert session: %w", sid, err)
	}

	// Drop any watermark this session held under a DIFFERENT path before writing
	// the current one — see the same guard in reindexContainer. Keyed by path, an
	// orphaned row looks to retention like a purged source and prunes a session
	// that is still present.
	if _, err := tx.Exec("DELETE FROM file_index WHERE session_id=? AND path<>?", sid, rp); err != nil {
		return fmt.Errorf("delete stale file_index: %w", err)
	}
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO file_index(path,mtime,size,fp,session_id) VALUES(?,?,?,?,?)",
		rp, mtime, size, provenance.FileFingerprint(path, size), sid,
	); err != nil {
		return fmt.Errorf("session %s insert file_index: %w", sid, err)
	}

	// Vault rawclaw's own copy inside the atomic success gate.
	if origin == "" {
		if err := vaultFile(path, sid, isSub, parent, projectArg, cwdArg); err != nil {
			return fmt.Errorf("session %s vault file: %w", sid, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("session %s commit tx: %w", sid, err)
	}
	return nil
}

// vaultFile stores rawclaw's own copy of a Claude-shape transcript, carrying the
// index facts the file itself cannot: the scope columns and the source-file
// watermark a rebuild needs to re-key file_index on the ORIGINAL path.
//
// Only own sessions are vaulted. A replica read out of an archive clone belongs
// to the machine that wrote it; keeping a durable local copy would resurrect it
// here after its owner deletes it, which is exactly the propagation the replica
// rules exist to honor.
func vaultFile(path, sid string, isSub int, parent string, projectArg, cwdArg any) error {
	m := durable.Meta{
		ID:         sid,
		Source:     sourceClaude,
		Project:    strOf(projectArg),
		CWD:        strOf(cwdArg),
		IsSubagent: isSub != 0,
		ParentID:   parent,
		SourcePath: realpath(path),
	}
	if st, err := os.Stat(path); err == nil {
		m.SourceMTime = mtimeOf(st)
		m.SourceSize = st.Size()
		m.SourceFP = provenance.FileFingerprint(path, st.Size())
	}
	return durable.StoreFile(m, path)
}

// strOf unwraps a nullable scope column into a plain string ("" = SQL NULL).
func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// applyRetentionToVault mirrors a retention pass onto the durable transcript
// vault, so the vault survives a delete-and-rebuild of the index with the same
// verdicts: a flagged session stays flagged, a reappeared one un-flags, and a
// pruned one loses its raw copy too (a user delete has to really delete, or the
// next rebuild would bring it back).
//
// Replica scopes are skipped wholesale: nothing in one is ours, so there is
// nothing of ours to mirror — and a replica id colliding with an own session
// must never reach the vault's delete path.
func applyRetentionToVault(res retention.Result, now float64, origin string) {
	if origin != "" {
		return
	}
	for _, id := range res.Stamped {
		if err := durable.SetMissingSince(id, now); err != nil {
			slog.Warn("durable transcript flag not written", "session", id, "err", err)
		}
	}
	for _, id := range res.Cleared {
		if err := durable.SetMissingSince(id, 0); err != nil {
			slog.Warn("durable transcript flag not cleared", "session", id, "err", err)
		}
	}
	for _, id := range res.Pruned {
		if err := durable.Remove(id); err != nil {
			slog.Warn("durable transcript not removed", "session", id, "err", err)
		}
	}
}

// parseTranscript reads and flattens one JSONL transcript into rows, computing
// the started/last timestamp watermarks and the working directory the session
// ran in. Returns ok=false if the file cannot be opened (a parse-time read
// error), leaving existing rows untouched. Malformed individual lines are
// skipped.
//
// cwd is read from the FIRST line that carries one, INCLUDING lines that are
// not indexable as messages: Claude records cwd on attachment/meta lines too,
// and a session whose only cwd-bearing line is non-indexable would otherwise
// land with a NULL scope. It is the session's own recorded value, never a
// decode of the enclosing directory name — that decode is lossy for any path
// segment containing "-" or ".".
func parseTranscript(path, sid string) (rows []reindexRow, started, last float64, cwd string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, "", false
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
		if cwd == "" {
			cwd = lineCWD(o)
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
	return rows, started, last, cwd, true
}

// lineCWD returns the working dir one transcript line records — top level, else
// nested under "message" — or "" when it records none. Mirrors paths.FileCWD's
// lookup so the two agree on where a cwd may hide.
func lineCWD(o map[string]any) string {
	if cwd, ok := o["cwd"].(string); ok && cwd != "" {
		return cwd
	}
	if msg, ok := o["message"].(map[string]any); ok {
		if cwd, ok := msg["cwd"].(string); ok && cwd != "" {
			return cwd
		}
	}
	return ""
}

// scopeOf resolves the (project, cwd) pair to stamp on a session row. project is
// the basename of the recorded cwd — the same label paths.ProjectLabel shows —
// falling back to the enclosing directory's name when the transcript records no
// cwd. Both are returned as `any` so an unresolvable scope writes SQL NULL
// rather than an empty string: NULL means "not known", "" would claim the
// session ran in a directory with no name.
func scopeOf(own string, dir projectScope) (projectArg, cwdArg any) {
	cwd := own
	if cwd == "" {
		cwd = dir.cwd
	}
	if cwd != "" {
		cwdArg = cwd
		if base := filepath.Base(strings.TrimRight(cwd, "/")); usableScope(base) {
			return base, cwdArg
		}
	}
	if usableScope(dir.label) {
		return dir.label, cwdArg
	}
	return nil, cwdArg
}

// projectScope is a project dir's identity: the working directory its
// transcripts record (empty when none does) and the friendly label
// paths.ProjectLabel shows for it. Resolved once per dir and reused, because
// resolving reads a transcript off disk and one dir holds hundreds of sessions.
type projectScope struct{ cwd, label string }

// dirScope resolves a project dir's identity. An empty dir yields an empty
// scope, so a source with no project dir at all (a Codex rollout shards by
// date) simply contributes no fallback.
func dirScope(tdir string) projectScope {
	if tdir == "" {
		return projectScope{}
	}
	return projectScope{cwd: paths.DirCWD(tdir), label: paths.ProjectLabel(tdir)}
}

// usableScope rejects the degenerate basenames that carry no scope information.
func usableScope(s string) bool {
	return s != "" && s != "." && s != string(filepath.Separator)
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

	// Resolve this dir's scope ONCE: every file in the walk shares it, including
	// the subagent and workflow threads nested below it, and resolving reads a
	// transcript off disk.
	scope := dirScope(transcriptDir)

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
		var prev fileMeta
		var found bool
		if prev, found = cur[rp]; found {
			if absDiff(prev.mtime, mtime) < 0.001 && prev.size == size {
				if prev.fp == provenance.FileFingerprint(f, size) {
					continue // genuinely unchanged
				}
			}
		}

		if found && prev.size > 0 && size > prev.size {
			headFP := provenance.FileFingerprint(f, prev.size)
			if headFP != "" && headFP == prev.fp {
				sid, isSub, parent := provenance.SessionIDFor(f, transcriptDir)
				c := source.Container{
					ID:         sid,
					Path:       f,
					IsSubagent: isSub == 1,
					ParentID:   parent,
				}
				tailMs, newOffset, ok := parseTailMessages(con, c, sourceClaude, f, prev.size, size)
				if ok {
					newFP := provenance.FileFingerprint(f, newOffset)
					if err := appendContainer(con, c, tailMs, sourceClaude, origin, rp, mtime, newOffset, newFP); err == nil {
						IncrementalIngestCount.Add(1)
						continue
					}
				}
			}
		}

		FullReindexCount.Add(1)
		if err := reindexFileWithOrigin(con, f, transcriptDir, origin, scope, rp, mtime, size); err != nil {
			return err
		}
	}

	// Retention pass (replaces the old "absent from the walk → DELETE" prune): an
	// absent own-source file is flagged missing_since and RETAINED; only an
	// explicit tombstone deletes; a foreign-origin row is never a candidate
	// (D1/D2/D5). An ARCHIVE-replica scan (origin set — the tree is a synced
	// copy inside the archive clone) instead treats absence as authoritative:
	// the owner's delete propagated through the archive (E5), so the replica
	// rows die rather than resurrect the session in search.
	now := nowEpoch()
	res, err := retention.ReconcileRetention(con, onDisk, tombstoned, now, retention.RetentionMirror(), origin != "")
	if err != nil {
		return err
	}
	applyRetentionToVault(res, now, origin)
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
	now := nowEpoch()
	res, rerr := retention.ReconcileRetention(con, map[string]struct{}{}, tombstoned, now, false, false)
	if rerr != nil {
		if isBusy(rerr) {
			return store.CountTopLevelSessions(dbp), nil
		}
		return 0, fmt.Errorf("orphan reconcile: %w", rerr)
	}
	applyRetentionToVault(res, now, "")
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
	// IndexStatusUnknown: uninitialised or indeterminate status (e.g. error returned).
	IndexStatusUnknown IndexStatus = iota
	// IndexFresh: the index was built/updated this call (the result is current).
	IndexFresh
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

// fireVectorTopup reads the hook under the read lock and calls it OUTSIDE the
// lock: the hook spawns a detached child, and holding a lock across that would
// serialise every indexing pass behind one spawn for no benefit.
func fireVectorTopup(dbp string) {
	vectorTopupMu.RLock()
	fn := vectorTopupHook
	vectorTopupMu.RUnlock()
	if fn != nil {
		fn(dbp)
	}
}

// vectorTopupHook is semantic.MaybeVectorTopup, injected at init by package
// semantic rather than imported, so index keeps no dependency on the embedding
// side and the vector lane stays a bolt-on the keyword engine never needs.
// Guarded even though the only production write is package semantic's init
// (which happens-before main, so that write alone could not race): tests swap
// the hook while other goroutines are indexing, and an unsynchronised func
// value read concurrently with a write is a data race the detector will call.
// The sibling --no-vector flag in package semantic is RWMutex-guarded for the
// same reason; matching it keeps one idiom across the feature.
var (
	vectorTopupMu   sync.RWMutex
	vectorTopupHook = func(string) {}
)

// SetVectorTopupHook installs the post-index vector top-up. Called once from
// package semantic's init; a nil fn restores the no-op.
func SetVectorTopupHook(fn func(string)) {
	vectorTopupMu.Lock()
	defer vectorTopupMu.Unlock()
	if fn == nil {
		vectorTopupHook = func(string) {}
		return
	}
	vectorTopupHook = fn
}

// EnsureIndexedTree builds/updates the FTS index for one Claude-shaped
// transcript tree at an EXPLICIT db path, stamping origin as every row's
// origin_machine ("" = this machine). This is EnsureIndexed with both halves of
// the identity made injectable: a replicated tree (another machine's transcripts
// synced onto this disk) indexes into its own namespaced db and carries its
// owner's identity, while the local path keeps its derived db and local stamp.
// Reindex + busy-lock semantics are identical to EnsureIndexed.
func EnsureIndexedTree(dbp, tdir string, reindex bool, origin string) (nSessions int, status IndexStatus, err error) {
	nSessions, status, err = ensureIndexedTree(dbp, tdir, reindex, origin)
	writeThroughConsolidated(dbp, err)
	return nSessions, status, err
}

func ensureIndexedTree(dbp, tdir string, reindex bool, origin string) (nSessions int, status IndexStatus, err error) {
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
		return 0, IndexStatusUnknown, fmt.Errorf("ensure schema: %w", err)
	}
	if err := updateIndexWithOrigin(con, tdir, origin); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexStatusUnknown, fmt.Errorf("update index: %w", err)
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&nSessions); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexStatusUnknown, fmt.Errorf("count sessions: %w", err)
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
