// Package store owns the SQLite schema (base + FTS + topic sidecar DDL, the
// schema-version gates) and the connection helpers for the per-scope index dbs.
// It sits at the bottom of the index seam: it imports no other internal package,
// so schema text and connection policy have a single, dependency-free home.
// Pure-Go via modernc.org/sqlite (no cgo).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (FTS5 + bm25 + snippet)
)

// SchemaVersion gates a full rebuild on mismatch. It is deliberately NOT bumped
// for the durable-retention columns (origin_machine/source_tool/source_path/
// only_copy_since) nor for the scope columns (project/cwd): a bump forces
// Rebuild() to re-walk the live tree and re-prune every already-retained
// session, defeating retention on the first upgrade. Those columns are added in
// place by index's migrateDurabilityColumns / migrateScopeColumns instead.
const SchemaVersion = 4

// Schema is the base (non-FTS) DDL. The sessions provenance/retention columns
// (origin_machine/source_tool/source_path/only_copy_since) and the scope columns
// (project/cwd) are present here so a fresh or rebuilt db carries them from the
// start; an existing current-version db gets them via index's in-place
// migrateDurabilityColumns / migrateScopeColumns migrations.
//
// project/cwd make a session's scope readable from the ROW rather than inferred
// from which per-project db file it lives in. Today that inference is exact
// (one db per project), which is precisely why the filename is load-bearing and
// a shared store is impossible; carrying the scope on the row is the prefactor
// that removes the dependency. origin_machine already answers "which machine",
// so the row triple is (project, cwd, origin_machine).
const Schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY, started_at REAL, last_ts REAL,
    message_count INTEGER DEFAULT 0, is_subagent INTEGER DEFAULT 0, parent_id TEXT,
    origin_machine TEXT, source_tool TEXT, source_path TEXT, only_copy_since REAL,
    project TEXT, cwd TEXT
);
CREATE TABLE IF NOT EXISTS session_sources (
    session_id TEXT NOT NULL, source_db TEXT NOT NULL,
    started_at REAL, last_ts REAL, message_count INTEGER DEFAULT 0,
    is_subagent INTEGER DEFAULT 0, parent_id TEXT,
    origin_machine TEXT, source_tool TEXT, source_path TEXT, only_copy_since REAL,
    project TEXT, cwd TEXT,
    PRIMARY KEY (session_id, source_db)
);
CREATE INDEX IF NOT EXISTS idx_session_sources_session ON session_sources(session_id);
CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
    role TEXT, content TEXT, ts REAL, ts_iso TEXT, uuid TEXT
);
CREATE INDEX IF NOT EXISTS idx_msg_session ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_msg_session_uuid ON messages(session_id, uuid);
CREATE TABLE IF NOT EXISTS file_index (path TEXT PRIMARY KEY, mtime REAL, size INTEGER, fp TEXT, session_id TEXT);
CREATE INDEX IF NOT EXISTS idx_file_index_session ON file_index(session_id);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);
`

// // FTSSQL is the unified FTS5 virtual table + sync triggers (porter + unicode61).
const FTSSQL = `
CREATE VIRTUAL TABLE messages_fts USING fts5(content, tokenize='porter unicode61');
CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
END;
CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
  DELETE FROM messages_fts WHERE rowid = old.id;
END;
CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
  DELETE FROM messages_fts WHERE rowid = old.id;
  INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
END;
`

// TrigramSQL is the SUBSTRING index: a second FTS5 table over the same content
// of the same messages, tokenized into overlapping three-character sequences,
// plus the triggers that keep it in step with messages exactly as the word
// index's triggers do.
//
// It exists because an FTS5 table fixes one tokenizer, and a word tokenizer only
// ever matches on token boundaries. A query landing mid-token is not something
// messages_fts ranks badly — it is something messages_fts cannot answer at all.
// The two tables therefore answer disjoint question shapes, and both are needed.
//
// The shape is copied verbatim from Hermes' schema — a plain
// fts5(content, tokenize='trigram'), no detail=none, no external content —
// because that is what a session-search tool with a large user base runs in
// production. Tuning it is something to do once there is evidence it is needed.
//
// Unlike the word DDL above, every object here is IF NOT EXISTS: these arrive at
// an already-populated db through an additive migration
// (index.migrateTrigramIndex) as well as through a rebuild, so the DDL has to be
// safe to re-run against a db that already has some of it.
const TrigramSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts_trigram USING fts5(content, tokenize='trigram');
CREATE TRIGGER IF NOT EXISTS messages_tri_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts_trigram(rowid, content) VALUES (new.id, new.content);
END;
CREATE TRIGGER IF NOT EXISTS messages_tri_ad AFTER DELETE ON messages BEGIN
  DELETE FROM messages_fts_trigram WHERE rowid = old.id;
END;
CREATE TRIGGER IF NOT EXISTS messages_tri_au AFTER UPDATE ON messages BEGIN
  DELETE FROM messages_fts_trigram WHERE rowid = old.id;
  INSERT INTO messages_fts_trigram(rowid, content) VALUES (new.id, new.content);
END;
`

// TrigramResetSQL empties the substring index. It is the recovery path for a
// db whose index holds rows the backfill watermark cannot account for, where
// starting over is the only state that is knowably correct.
const TrigramResetSQL = `DELETE FROM messages_fts_trigram`

// TrigramBatchBoundSQL returns the highest messages.id in the next backfill
// batch — the upper bound of a half-open id window — or NULL when nothing is
// left to copy. Taking the bound first means the INSERT that follows is a plain
// range scan over the rowid index rather than a LIMIT whose row set depends on
// evaluation order. Args: the watermark, the batch size.
const TrigramBatchBoundSQL = `SELECT max(id) FROM (SELECT id FROM messages WHERE id > ? ORDER BY id LIMIT ?)`

// TrigramBatchFillSQL copies one id window of messages into the substring
// index. Args: the watermark (exclusive), the batch bound (inclusive).
//
// OR REPLACE makes the copy idempotent, which matters because an entry can
// already be there for a row this window covers: another process writing a
// message during the backfill fires the insert trigger for it. Re-writing an
// identical entry is a no-op in effect, whereas a plain INSERT would fail the
// whole migration on a rowid collision.
const TrigramBatchFillSQL = `INSERT OR REPLACE INTO messages_fts_trigram(rowid, content) SELECT id, content FROM messages WHERE id > ? AND id <= ?`

// ExactSQL is the UNSTEMMED, CODE-AWARE exact-token index: an external-content
// FTS5 table over messages, tokenized with unicode61 and custom tokenchars so
// code symbols, flags, paths, and punctuation remain intact as single tokens.
//
// Triggers use the ccrider / SQLite FTS5 external-content 'delete' command:
// deleting an external-content row MUST supply 'delete', old.id, old.content
// because the underlying content row is already gone in AFTER DELETE.
const ExactSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts_exact USING fts5(
    content,
    content='messages',
    content_rowid='id',
    tokenize="unicode61"
);
CREATE TRIGGER IF NOT EXISTS messages_exact_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts_exact(rowid, content) VALUES (new.id, new.content);
END;
CREATE TRIGGER IF NOT EXISTS messages_exact_ad AFTER DELETE ON messages BEGIN
  INSERT INTO messages_fts_exact(messages_fts_exact, rowid, content) VALUES ('delete', old.id, old.content);
END;
CREATE TRIGGER IF NOT EXISTS messages_exact_au AFTER UPDATE ON messages BEGIN
  INSERT INTO messages_fts_exact(messages_fts_exact, rowid, content) VALUES ('delete', old.id, old.content);
  INSERT INTO messages_fts_exact(rowid, content) VALUES (new.id, new.content);
END;
`

// dropSQL drops every schema object before a full rebuild. Dropping messages
// would take its triggers with it, but they are named here anyway so the drop
// list reads as the complete inventory of what a rebuild removes.
const dropSQL = `DROP TRIGGER IF EXISTS messages_ai;
DROP TRIGGER IF EXISTS messages_ad;
DROP TRIGGER IF EXISTS messages_au;
DROP TRIGGER IF EXISTS messages_tri_ai;
DROP TRIGGER IF EXISTS messages_tri_ad;
DROP TRIGGER IF EXISTS messages_tri_au;
DROP TRIGGER IF EXISTS messages_exact_ai;
DROP TRIGGER IF EXISTS messages_exact_ad;
DROP TRIGGER IF EXISTS messages_exact_au;
DROP TABLE IF EXISTS messages_fts;
DROP TABLE IF EXISTS messages_code_fts;
DROP TABLE IF EXISTS messages_fts_trigram;
DROP TABLE IF EXISTS messages_fts_exact;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS session_sources;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS file_index;`

// TopicSchemaVersion gates the topic sidecar tables separately from the keyword
// schema — like VecSchemaVersion, it is its OWN gate and is NEVER in
// Schema/FTSSQL/dropSQL, so a keyword reindex can't nuke topic rows. Topic rows
// are keyed by the source-stable message uuid (start_uuid/end_uuid), so they
// re-map losslessly after a base reindex churns the integer msg ids.
//
// v2 (tags-ride-the-archive): topic_segment gains origin_machine
// (per-machine attribution for the cross-machine LWW ingest) and a new
// session_verdict sidecar (the routine verdict + its floor|agent source) joins
// the gate. Bumping re-runs EnsureTopicSchema, which adds the column in place
// (PRAGMA-guarded ALTER) and creates session_verdict — NOT a base rebuild, so no
// transcript re-walk. Like the durability columns, existing NULL-origin rows are
// this machine's and backfill to MachineID().
const TopicSchemaVersion = 2

// EnsureTopicSchema creates the topic sidecar (its own gate, separate from the
// keyword schema) and stamps the topic_schema_version meta key. Idempotent.
// Mirrors EnsureVecSchema: every object is IF NOT EXISTS and lives outside the
// Rebuild() drop list, so a base reindex leaves it (and its rows) intact.
func EnsureTopicSchema(con *sql.DB) error {
	var version string
	verr := con.QueryRow("SELECT value FROM meta WHERE key='topic_schema_version'").Scan(&version)
	if verr == nil && version == strconv.Itoa(TopicSchemaVersion) {
		return nil // already current — nothing to (re)create
	}
	const topicDDL = `
CREATE TABLE IF NOT EXISTS topic_segment (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL, start_uuid TEXT NOT NULL, end_uuid TEXT,
  topic TEXT, summary TEXT, tagged_at REAL, origin_machine TEXT,
  UNIQUE(session_id, start_uuid)
);
CREATE INDEX IF NOT EXISTS idx_topic_session ON topic_segment(session_id);
CREATE VIRTUAL TABLE IF NOT EXISTS topic_fts USING fts5(topic, summary, content='topic_segment', content_rowid='id', tokenize='porter unicode61');
CREATE TRIGGER IF NOT EXISTS topic_ai AFTER INSERT ON topic_segment BEGIN
  INSERT INTO topic_fts(rowid, topic, summary) VALUES (new.id, new.topic, new.summary);
END;
CREATE TRIGGER IF NOT EXISTS topic_ad AFTER DELETE ON topic_segment BEGIN
  INSERT INTO topic_fts(topic_fts, rowid, topic, summary) VALUES ('delete', old.id, old.topic, old.summary);
END;
CREATE TRIGGER IF NOT EXISTS topic_au AFTER UPDATE ON topic_segment BEGIN
  INSERT INTO topic_fts(topic_fts, rowid, topic, summary) VALUES ('delete', old.id, old.topic, old.summary);
  INSERT INTO topic_fts(rowid, topic, summary) VALUES (new.id, new.topic, new.summary);
END;
CREATE TABLE IF NOT EXISTS session_verdict (
  session_id TEXT PRIMARY KEY,
  verdict TEXT NOT NULL, source TEXT NOT NULL,
  origin_machine TEXT, tagged_at REAL
);`
	if _, err := con.Exec(topicDDL); err != nil {
		return fmt.Errorf("create topic schema: %w", err)
	}
	// v1→v2 in-place migration: topic_segment predating v2 lacks origin_machine
	// (the IF NOT EXISTS DDL above leaves an existing table untouched). Add it
	// PRAGMA-guarded — pure DDL, no provenance. Existing rows keep origin_machine
	// NULL; a NULL origin is interpreted as "this machine" at export (cross-machine
	// tags only ever arrive via the explicit-origin ingest path, which always stamps
	// a non-NULL id), so no provenance-dependent backfill belongs at this layer —
	// store imports no internal package, matching migrateDurabilityColumns living
	// in index, not here.
	if err := migrateTopicOriginColumn(con); err != nil {
		return err
	}
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO meta(key,value) VALUES('topic_schema_version',?)",
		strconv.Itoa(TopicSchemaVersion),
	); err != nil {
		return fmt.Errorf("stamp topic_schema_version: %w", err)
	}
	return nil
}

// migrateTopicOriginColumn adds topic_segment.origin_machine in place when an
// existing (v1) table lacks it. Pure, idempotent DDL: a fresh/v2 db already
// carries the column via the DDL above, so the guard skips the ALTER. No
// provenance backfill here — see EnsureTopicSchema.
func migrateTopicOriginColumn(con *sql.DB) error {
	have, err := columnSet(con, "topic_segment")
	if err != nil {
		return err
	}
	if _, ok := have["origin_machine"]; !ok {
		if _, err := con.Exec("ALTER TABLE topic_segment ADD COLUMN origin_machine TEXT"); err != nil {
			return fmt.Errorf("add topic_segment.origin_machine: %w", err)
		}
	}
	return nil
}

// columnSet returns the column names of a table via PRAGMA table_info — the
// guard for additive ALTER TABLE migrations.
func columnSet(con *sql.DB, table string) (map[string]struct{}, error) {
	rows, err := con.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, fmt.Errorf("read %s columns: %w", table, err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var (
			cid     int
			name    string
			typ     string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan %s column: %w", table, err)
		}
		out[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s columns: %w", table, err)
	}
	return out, nil
}

// CacheDir returns the session-search state dir (<cacheHome>/session-search),
// creating it. It holds the per-project index dbs, the tombstone sidecar, and the
// machine-id file — and is the discovery surface for orphaned-source dbs.
func CacheDir() string {
	d := filepath.Join(cacheHome(), "session-search")
	_ = os.MkdirAll(d, 0o755) // best-effort; ignore an existing dir
	return d
}

// cacheHome resolves ~/.cache.
func cacheHome() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".cache" // degrade to a relative path rather than panic
	}
	return filepath.Join(home, ".cache")
}

// Rebuild drops and recreates the full schema + FTS, then stamps the version.
func Rebuild(con *sql.DB) error {
	if _, err := con.Exec(dropSQL); err != nil {
		return fmt.Errorf("rebuild drop: %w", err)
	}
	if _, err := con.Exec(Schema); err != nil {
		return fmt.Errorf("rebuild schema: %w", err)
	}
	if _, err := con.Exec(FTSSQL); err != nil {
		return fmt.Errorf("rebuild fts: %w", err)
	}
	// The substring index is part of the rebuilt shape, so a fresh db carries it
	// from the start and only an already-populated db needs the migration.
	if _, err := con.Exec(TrigramSQL); err != nil {
		return fmt.Errorf("rebuild trigram fts: %w", err)
	}
	// The exact-token index is part of the rebuilt shape.
	if _, err := con.Exec(ExactSQL); err != nil {
		return fmt.Errorf("rebuild exact fts: %w", err)
	}
	_, err := con.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES('schema_version',?)", strconv.Itoa(SchemaVersion))
	if err != nil {
		return fmt.Errorf("rebuild stamp version: %w", err)
	}
	return nil
}

// ROMmapSize is the memory-mapped I/O size for read-only connections (SQLite SQLITE_MAX_MMAP_SIZE ceiling).
const ROMmapSize = 0x7fff0000 // ~2 GiB read-only mmap window

// ConnectRO opens dbp in read-only mode (file:<dbp>?mode=ro). Exported so
// sibling packages can reuse it. Configured with a 5s busy timeout, 64MB page cache,
// and a 2GB mmap_size so hot queries serve directly from memory-mapped pages.
//
// SINGLE-CONN DISCIPLINE: the pool is capped at ONE connection, so a
// caller MUST fully drain + close a result set (rows.Close) before issuing the
// next query on the same *sql.DB. Interleaving — opening a second query while
// rows from the first are still open — blocks forever waiting for a second
// connection (the view.Browse / semantic.VecKNN deadlock class).
func ConnectRO(dbp string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)&_pragma=cache_size(-64000)&_pragma=mmap_size(%d)", dbp, ROMmapSize)
	con, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only db: %w", err)
	}
	con.SetMaxOpenConns(1) // modernc serializes; readers stay single-conn
	return con, nil
}

// ConnectRW opens dbp read-write with WAL + a 10s busy timeout, single-writer.
// (10s is the unification of index's old 5s and cli's 10s timeouts.)
//
// SINGLE-CONN DISCIPLINE: the pool is capped at ONE connection, so a
// caller MUST fully drain + close a result set (rows.Close) before issuing the
// next query on the same *sql.DB. Interleaving — opening a second query while
// rows from the first are still open — blocks forever waiting for a second
// connection (the view.Browse / semantic.VecKNN deadlock class).
func ConnectRW(dbp string) (*sql.DB, error) {
	dsn := "file:" + dbp + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)"
	con, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	con.SetMaxOpenConns(1) // SQLite single-writer
	if err := con.Ping(); err != nil {
		con.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return con, nil
}
