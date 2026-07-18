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
// missing_since): a bump forces Rebuild() to re-walk the live tree and re-prune
// every already-retained session, defeating retention on the first upgrade. Those
// columns are added in place by index's migrateDurabilityColumns (D6) instead.
const SchemaVersion = 4

// Schema is the base (non-FTS) DDL. The sessions provenance/retention columns
// (origin_machine/source_tool/source_path/missing_since) are present here so a
// fresh or rebuilt db carries them from the start; an existing current-version db
// gets them via index's in-place migrateDurabilityColumns migration (D6).
const Schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY, started_at REAL, last_ts REAL,
    message_count INTEGER DEFAULT 0, is_subagent INTEGER DEFAULT 0, parent_id TEXT,
    origin_machine TEXT, source_tool TEXT, source_path TEXT, missing_since REAL
);
CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
    role TEXT, content TEXT, ts REAL, ts_iso TEXT, uuid TEXT
);
CREATE INDEX IF NOT EXISTS idx_msg_session ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_msg_session_uuid ON messages(session_id, uuid);
CREATE TABLE IF NOT EXISTS file_index (path TEXT PRIMARY KEY, mtime REAL, size INTEGER, fp TEXT, session_id TEXT);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);
`

// FTSSQL is the FTS5 virtual table + sync triggers (contentful/inline + porter).
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

// dropSQL drops every schema object before a full rebuild.
const dropSQL = `DROP TRIGGER IF EXISTS messages_ai;
DROP TRIGGER IF EXISTS messages_ad;
DROP TRIGGER IF EXISTS messages_au;
DROP TABLE IF EXISTS messages_fts;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS file_index;`

// TopicSchemaVersion gates the topic sidecar tables separately from the keyword
// schema — like VecSchemaVersion, it is its OWN gate and is NEVER in
// Schema/FTSSQL/dropSQL, so a keyword reindex can't nuke topic rows. Topic rows
// are keyed by the source-stable message uuid (start_uuid/end_uuid), so they
// re-map losslessly after a base reindex churns the integer msg ids.
const TopicSchemaVersion = 1

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
  topic TEXT, summary TEXT, tagged_at REAL,
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
END;`
	if _, err := con.Exec(topicDDL); err != nil {
		return fmt.Errorf("create topic schema: %w", err)
	}
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO meta(key,value) VALUES('topic_schema_version',?)",
		strconv.Itoa(TopicSchemaVersion),
	); err != nil {
		return fmt.Errorf("stamp topic_schema_version: %w", err)
	}
	return nil
}

// CacheDir returns the session-search state dir (<cacheHome>/session-search),
// creating it. It holds the per-project index dbs, the tombstone sidecar, and the
// machine-id file — and is the discovery surface for orphaned-source dbs (D8).
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
	_, err := con.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES('schema_version',?)", strconv.Itoa(SchemaVersion))
	if err != nil {
		return fmt.Errorf("rebuild stamp version: %w", err)
	}
	return nil
}

// ConnectRO opens dbp in read-only mode (file:<dbp>?mode=ro). Exported so
// sibling packages can reuse it.
//
// SINGLE-CONN DISCIPLINE (D3): the pool is capped at ONE connection, so a
// caller MUST fully drain + close a result set (rows.Close) before issuing the
// next query on the same *sql.DB. Interleaving — opening a second query while
// rows from the first are still open — blocks forever waiting for a second
// connection (the view.Browse / semantic.VecKNN deadlock class).
func ConnectRO(dbp string) (*sql.DB, error) {
	dsn := "file:" + dbp + "?mode=ro&_pragma=busy_timeout(5000)"
	con, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only db: %w", err)
	}
	con.SetMaxOpenConns(1) // modernc serializes; readers stay single-conn
	return con, nil
}

// ConnectRW opens dbp read-write with WAL + a 10s busy timeout, single-writer.
// (10s is the D2 unification: the superset of index's old 5s and cli's 10s.)
//
// SINGLE-CONN DISCIPLINE (D3): the pool is capped at ONE connection, so a
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
