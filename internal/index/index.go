// Package index owns the on-disk SQLite/FTS5 store: schema management, file
// fingerprinting, incremental reindexing, the read-only connection helper, and
// corpus stats. Pure-Go via modernc.org/sqlite (no cgo).
package index

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (FTS5 + bm25 + snippet)
)

// SchemaVersion gates a full rebuild on mismatch.
const SchemaVersion = 4

// Schema is the base (non-FTS) DDL.
const Schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY, started_at REAL, last_ts REAL,
    message_count INTEGER DEFAULT 0, is_subagent INTEGER DEFAULT 0, parent_id TEXT
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

// TopicSegment is one tagged segment of a session, returned by TopicsForSession
// for the outline view. Keyed externally by (session_id, start_uuid).
type TopicSegment struct {
	SessionID string
	StartUUID string
	EndUUID   string
	Topic     string
	Summary   string
}

// TopicHit is one topic_fts match resolved to the segment's START message id
// (via a messages join on session_id+start_uuid). MsgID is the live rowid the
// fusion layer scores; SessionID + Topic carry the context to attach to it.
type TopicHit struct {
	MsgID     int
	SessionID string
	Topic     string
}

// EnsureTopicSchema creates the topic sidecar (its own gate, separate from the
// keyword schema) and stamps the topic_schema_version meta key. Idempotent.
// Mirrors EnsureVecSchema: every object is IF NOT EXISTS and lives outside the
// rebuild() drop list, so a base reindex leaves it (and its rows) intact.
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

// UpsertTopicSegment inserts or updates one topic segment, keyed by the stable
// (session_id, start_uuid). The external-content FTS triggers keep topic_fts in
// sync — except an ON CONFLICT UPDATE fires the AFTER UPDATE trigger, which
// re-syncs the changed topic/summary.
func UpsertTopicSegment(con *sql.DB, sessionID, startUUID, endUUID, topic, summary string, taggedAt float64) error {
	_, err := con.Exec(`
INSERT INTO topic_segment(session_id, start_uuid, end_uuid, topic, summary, tagged_at)
VALUES(?,?,?,?,?,?)
ON CONFLICT(session_id, start_uuid) DO UPDATE SET
  end_uuid=excluded.end_uuid, topic=excluded.topic, summary=excluded.summary, tagged_at=excluded.tagged_at`,
		sessionID, startUUID, endUUID, topic, summary, taggedAt)
	if err != nil {
		return fmt.Errorf("upsert topic segment: %w", err)
	}
	return nil
}

// TopicsForSession returns the topic segments for one session, ordered by id
// (insertion order — roughly chronological as the tagger walks the session).
// Used by the outline view. A missing topic table reads as "no topics".
func TopicsForSession(con *sql.DB, sessionID string) ([]TopicSegment, error) {
	rows, err := con.Query(
		"SELECT session_id, start_uuid, end_uuid, topic, summary FROM topic_segment WHERE session_id=? ORDER BY id",
		sessionID)
	if err != nil {
		return nil, nil // missing table / read error reads as no topics (non-fatal)
	}
	defer rows.Close()
	var out []TopicSegment
	for rows.Next() {
		var (
			seg     TopicSegment
			endU    sql.NullString
			topic   sql.NullString
			summary sql.NullString
		)
		if err := rows.Scan(&seg.SessionID, &seg.StartUUID, &endU, &topic, &summary); err != nil {
			return nil, fmt.Errorf("scan topic segment: %w", err)
		}
		seg.EndUUID = endU.String
		seg.Topic = topic.String
		seg.Summary = summary.String
		out = append(out, seg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topic segments: %w", err)
	}
	return out, nil
}

// MatchTopics runs an FTS query over topic_fts and, for each matched segment,
// resolves its START message to a live rowid (messages join on
// session_id+start_uuid). A segment whose start message is gone (churned/never
// indexed) is skipped — it has no anchor to surface. A missing topic table reads
// as no hits. Ordered by FTS rank, capped at limit.
func MatchTopics(con *sql.DB, query string, limit int) ([]TopicHit, error) {
	if strings.TrimSpace(query) == "" || limit <= 0 {
		return nil, nil
	}
	// OR the query terms (each quoted as a literal) so a query whose words don't ALL
	// appear in a terse topic label still matches on the ones that do; FTS5 `rank`
	// (bm25) then orders by term overlap + rarity. Topic rows are few per project, so
	// OR cannot drown — and this is an on-demand disambiguation tool, recall > precision.
	var terms []string
	for _, t := range strings.Fields(query) {
		t = strings.Trim(strings.ReplaceAll(t, `"`, ""), "*()-:^")
		if t != "" {
			terms = append(terms, `"`+t+`"`)
		}
	}
	if len(terms) == 0 {
		return nil, nil
	}
	match := strings.Join(terms, " OR ")
	rows, err := con.Query(`
SELECT m.id, ts.session_id, ts.topic
FROM topic_fts
JOIN topic_segment ts ON ts.id = topic_fts.rowid
JOIN messages m ON m.session_id = ts.session_id AND m.uuid = ts.start_uuid
WHERE topic_fts MATCH ?
ORDER BY rank
LIMIT ?`, match, limit)
	if err != nil {
		return nil, nil // missing table / malformed query reads as no hits (non-fatal)
	}
	defer rows.Close()
	var out []TopicHit
	for rows.Next() {
		var (
			h     TopicHit
			topic sql.NullString
		)
		if err := rows.Scan(&h.MsgID, &h.SessionID, &topic); err != nil {
			return nil, fmt.Errorf("scan topic hit: %w", err)
		}
		h.Topic = topic.String
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topic hits: %w", err)
	}
	return out, nil
}

// TopicForMessage returns the topic of the segment whose [start_uuid, end_uuid]
// range contains the given message uuid, for the non-fused (no embedder) path
// where there is no FTS topic match to attach. It uses the message's id order:
// the segment is the latest one in the session whose start message id is <= the
// target message id, and (if end_uuid is set) whose end message id is >= the
// target. When no range matches, it falls back to the session's single topic if
// there is exactly one — otherwise "". Kept deliberately simple; a missing topic
// table reads as "".
func TopicForMessage(con *sql.DB, sessionID, msgUUID string) string {
	// Resolve the target message's rowid (the ordering key within a session).
	var targetID int
	if err := con.QueryRow(
		"SELECT id FROM messages WHERE session_id=? AND uuid=?", sessionID, msgUUID,
	).Scan(&targetID); err != nil {
		return ""
	}
	rows, err := con.Query(`
SELECT ts.topic, sm.id AS start_id, em.id AS end_id
FROM topic_segment ts
JOIN messages sm ON sm.session_id = ts.session_id AND sm.uuid = ts.start_uuid
LEFT JOIN messages em ON em.session_id = ts.session_id AND em.uuid = ts.end_uuid
WHERE ts.session_id=?
ORDER BY sm.id`, sessionID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var (
		segCount int
		soleTop  string
		bestTop  string
		bestSt   = -1
	)
	for rows.Next() {
		var (
			topic   sql.NullString
			startID int
			endID   sql.NullInt64
		)
		if err := rows.Scan(&topic, &startID, &endID); err != nil {
			return ""
		}
		segCount++
		soleTop = topic.String
		// Range containment: start <= target, and (no end OR end >= target).
		if startID <= targetID && (!endID.Valid || int(endID.Int64) >= targetID) {
			if startID > bestSt { // latest qualifying start wins
				bestSt = startID
				bestTop = topic.String
			}
		}
	}
	if err := rows.Err(); err != nil {
		return ""
	}
	if bestSt >= 0 {
		return bestTop
	}
	if segCount == 1 {
		return soleTop // session has a single topic — attach it
	}
	return ""
}

// CorpusStats is the aggregate-counts result for one indexed project's db.
type CorpusStats struct {
	Sessions  int // top-level sessions (is_subagent=0)
	Subagents int // subagent threads (is_subagent=1)
	Messages  int
	User      int
	Assistant int
	First     string // earliest ts_iso[:10]
	Last      string // latest ts_iso[:10]
}

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

// DBPath returns the cache db path for a transcript dir:
// ~/.cache/session-search/<encoded-dir>.db (creating the dir).
func DBPath(transcriptDir string) string {
	enc := filepath.Base(filepath.Clean(transcriptDir))
	d := filepath.Join(cacheHome(), "session-search")
	_ = os.MkdirAll(d, 0o755) // best-effort; ignore an existing dir
	return filepath.Join(d, enc+".db")
}

// cacheHome resolves ~/.cache.
func cacheHome() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".cache" // degrade to a relative path rather than panic
	}
	return filepath.Join(home, ".cache")
}

// FileFingerprint is a cheap content fingerprint (sha1 of first 4KB + "|" + last
// 4KB, hex[:16]) catching a same-mtime+same-size in-place rewrite at either end.
// Returns "" on any I/O error.
func FileFingerprint(path string, size int64) string {
	fh, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer fh.Close()

	head := make([]byte, 4096)
	n, err := io.ReadFull(fh, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return ""
	}
	head = head[:n]

	var tail []byte
	if size > 8192 {
		if _, err := fh.Seek(-4096, io.SeekEnd); err != nil {
			return ""
		}
		tail = make([]byte, 4096)
		m, err := io.ReadFull(fh, tail)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return ""
		}
		tail = tail[:m]
	}

	h := sha1.New()
	h.Write(head)
	h.Write([]byte("|"))
	h.Write(tail)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// rebuild drops and recreates the full schema + FTS, then stamps the version.
func rebuild(con *sql.DB) error {
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

// EnsureSchema creates the base schema, the FTS table if missing, and rebuilds
// on any SchemaVersion mismatch or missing marker.
func EnsureSchema(con *sql.DB) error {
	// Read the schema-version marker FIRST, before running the full base Schema.
	// Schema creates idx_msg_session_uuid on messages(session_id, uuid); on a
	// pre-v4 db the messages table lacks the uuid column, so running Schema first
	// would fail with "no such column: uuid" BEFORE the rebuild below could
	// migrate it. The version probe must come first. (errors.Is is no longer
	// needed: any read error — incl. a missing meta table — means rebuild.)
	var version string
	verr := con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&version)
	if verr != nil || version != strconv.Itoa(SchemaVersion) {
		// Missing meta table / missing marker / version mismatch / any read error
		// → full rebuild. rebuild() drops every versioned object and recreates the
		// current shape, then stamps the version. The JSONL transcript is the source
		// of truth, so a dropped cache is reindexed losslessly. This IS the migration
		// path (e.g. v3 → v4 adds messages.uuid).
		if rerr := rebuild(con); rerr != nil {
			return fmt.Errorf("ensure schema rebuild: %w", rerr)
		}
		return nil
	}
	// Version already current → ensure the base schema + FTS are present
	// (idempotent; covers a current db that somehow lost its FTS table).
	if _, err := con.Exec(Schema); err != nil {
		return fmt.Errorf("ensure base schema: %w", err)
	}
	if _, err := con.Exec("SELECT 1 FROM messages_fts LIMIT 1"); err != nil {
		_, _ = con.Exec(FTSSQL) // best-effort; raced creation is acceptable
	}
	return nil
}

// SessionIDFor returns the unique session id for a transcript path, plus whether
// it is a subagent (1/0) and its parent id. Top-level: filename stem. Subagent
// (under a subagents/ subdir): "<parent>/<stem>".
func SessionIDFor(path, transcriptDir string) (sid string, isSubagent int, parent string) {
	stem := stemOf(path)
	rel, err := filepath.Rel(transcriptDir, path)
	if err != nil {
		rel = path
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	for i, p := range parts {
		if p == "subagents" {
			if i > 0 {
				par := parts[i-1]
				return par + "/" + stem, 1, par
			}
			return "subagents/" + stem, 1, "" // empty parent -> SQL NULL
		}
	}
	return stem, 0, ""
}

// stemOf returns the filename with its final extension stripped.
func stemOf(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// reindexRow is one parsed message ready for insertion.
type reindexRow struct {
	role    string
	content string
	ts      float64
	tsISO   string
	uuid    string
}

// ReindexFile parses the whole file into memory FIRST, then atomically replaces
// this session's rows (an I/O failure can't commit away existing data). Returns
// true on success.
func ReindexFile(con *sql.DB, path, transcriptDir string) bool {
	sid, isSub, parent := SessionIDFor(path, transcriptDir)

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
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id) VALUES(?,?,?,?,?,?)",
		sid, started, last, len(rows), isSub, parentArg,
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
		if sid, _, _ := SessionIDFor(f, transcriptDir); isMember(tombstoned, sid) {
			continue
		}
		mtime := mtimeOf(st)
		size := st.Size()
		if prev, found := cur[rp]; found {
			if absDiff(prev.mtime, mtime) < 0.001 && prev.size == size {
				if prev.fp == FileFingerprint(f, size) {
					continue // genuinely unchanged
				}
			}
		}
		if ReindexFile(con, f, transcriptDir) {
			sid, _, _ := SessionIDFor(f, transcriptDir)
			if _, err := con.Exec(
				"INSERT OR REPLACE INTO file_index(path,mtime,size,fp,session_id) VALUES(?,?,?,?,?)",
				rp, mtime, size, FileFingerprint(f, size), sid,
			); err != nil {
				return fmt.Errorf("update file_index: %w", err)
			}
		}
	}

	// Prune sessions whose backing file is gone.
	stale, err := loadStaleFiles(con, onDisk)
	if err != nil {
		return fmt.Errorf("scan file_index for prune: %w", err)
	}
	for _, s := range stale {
		if _, err := con.Exec("DELETE FROM messages WHERE session_id=?", s.sessionID); err != nil {
			return fmt.Errorf("prune messages: %w", err)
		}
		if _, err := con.Exec("DELETE FROM sessions WHERE id=?", s.sessionID); err != nil {
			return fmt.Errorf("prune sessions: %w", err)
		}
		if _, err := con.Exec("DELETE FROM file_index WHERE path=?", s.path); err != nil {
			return fmt.Errorf("prune file_index: %w", err)
		}
	}
	return nil
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

// staleFile is a file_index row whose backing file is no longer on disk.
type staleFile struct {
	path      string
	sessionID string
}

// loadStaleFiles returns file_index rows whose path is not in onDisk. The rows
// are read fully into memory first so the subsequent DELETEs don't mutate a live
// cursor.
func loadStaleFiles(con *sql.DB, onDisk map[string]struct{}) ([]staleFile, error) {
	rows, err := con.Query("SELECT path,session_id FROM file_index")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []staleFile
	for rows.Next() {
		var s staleFile
		if err := rows.Scan(&s.path, &s.sessionID); err != nil {
			return nil, err
		}
		all = append(all, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var stale []staleFile
	for _, s := range all {
		if _, ok := onDisk[s.path]; !ok {
			stale = append(stale, s)
		}
	}
	return stale, nil
}

// CountSessions opens dbp read-only and returns the session count, or -1 on
// error (callers must treat <0 as unknown).
func CountSessions(dbp string) int {
	con, err := ConnectRO(dbp)
	if err != nil {
		return -1
	}
	defer con.Close()
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
		return -1
	}
	return n
}

// CountTopLevelSessions returns the count of TOP-LEVEL sessions (is_subagent=0)
// — what a user means by "this project's sessions". Use this for display; the
// raw CountSessions above includes subagent threads and is internal bookkeeping.
// Returns -1 on error.
func CountTopLevelSessions(dbp string) int {
	con, err := ConnectRO(dbp)
	if err != nil {
		return -1
	}
	defer con.Close()
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE is_subagent=0").Scan(&n); err != nil {
		return -1
	}
	return n
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
	if reindex {
		if _, statErr := os.Stat(dbp); statErr == nil {
			_ = os.Remove(dbp) // best-effort; ignore a remove error
		}
	}

	con, openErr := openRW(dbp)
	if openErr != nil {
		// Treat an open/lock failure as a fall-back to the existing index.
		return dbp, CountSessions(dbp), IndexStale, nil
	}
	defer con.Close()

	if err := EnsureSchema(con); err != nil {
		if isBusy(err) {
			return dbp, CountSessions(dbp), IndexStale, nil
		}
		return dbp, 0, IndexFresh, fmt.Errorf("ensure schema: %w", err)
	}
	if err := UpdateIndex(con, tdir); err != nil {
		if isBusy(err) {
			return dbp, CountSessions(dbp), IndexStale, nil
		}
		return dbp, 0, IndexFresh, fmt.Errorf("update index: %w", err)
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&nSessions); err != nil {
		if isBusy(err) {
			return dbp, CountSessions(dbp), IndexStale, nil
		}
		return dbp, 0, IndexFresh, fmt.Errorf("count sessions: %w", err)
	}
	return dbp, nSessions, IndexFresh, nil
}

// GetCorpusStats returns aggregate counts for one indexed project's db
// (read-only). On a query error it returns a zero-value CorpusStats and nil
// error.
func GetCorpusStats(dbp string) (CorpusStats, error) {
	con, err := ConnectRO(dbp)
	if err != nil {
		return CorpusStats{}, fmt.Errorf("open corpus db: %w", err)
	}
	defer con.Close()

	var cs CorpusStats
	scan := func(q string, dest ...any) error {
		return con.QueryRow(q).Scan(dest...)
	}
	if err := scan("SELECT COUNT(*) FROM sessions WHERE is_subagent=0", &cs.Sessions); err != nil {
		return CorpusStats{}, nil // a query error -> zero stats
	}
	if err := scan("SELECT COUNT(*) FROM sessions WHERE is_subagent=1", &cs.Subagents); err != nil {
		return CorpusStats{}, nil
	}
	if err := scan("SELECT COUNT(*) FROM messages", &cs.Messages); err != nil {
		return CorpusStats{}, nil
	}
	if err := scan("SELECT COUNT(*) FROM messages WHERE role='user'", &cs.User); err != nil {
		return CorpusStats{}, nil
	}
	if err := scan("SELECT COUNT(*) FROM messages WHERE role='assistant'", &cs.Assistant); err != nil {
		return CorpusStats{}, nil
	}
	var first, last sql.NullString
	if err := scan("SELECT MIN(ts_iso), MAX(ts_iso) FROM messages WHERE length(ts_iso)>0", &first, &last); err != nil {
		return CorpusStats{}, nil
	}
	cs.First = first10(first.String)
	cs.Last = first10(last.String)
	return cs, nil
}

// first10 returns the first 10 runes of s (the date portion of an ISO string).
func first10(s string) string {
	r := []rune(s)
	if len(r) > 10 {
		return string(r[:10])
	}
	return string(r)
}

// ConnectRO opens dbp in read-only mode (file:<dbp>?mode=ro). Exported so
// sibling packages can reuse it.
func ConnectRO(dbp string) (*sql.DB, error) {
	dsn := "file:" + dbp + "?mode=ro&_pragma=busy_timeout(5000)"
	con, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only db: %w", err)
	}
	con.SetMaxOpenConns(1) // modernc serializes; readers stay single-conn
	return con, nil
}

// openRW opens dbp read-write with WAL + a 5s busy timeout, single-writer.
func openRW(dbp string) (*sql.DB, error) {
	dsn := "file:" + dbp + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
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
