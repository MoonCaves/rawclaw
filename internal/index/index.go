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
	"github.com/MoonCaves/rawclaw/internal/store"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (FTS5 + bm25 + snippet)
)

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

// ReindexFile parses the whole file into memory FIRST, then atomically replaces
// this session's rows (an I/O failure can't commit away existing data). Returns
// true on success.
func ReindexFile(con *sql.DB, path, transcriptDir string) bool {
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
		sid, started, last, len(rows), isSub, parentArg, provenance.MachineID(), sourceClaude, realpath(path),
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
		if ReindexFile(con, f, transcriptDir) {
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
	// explicit tombstone deletes; a foreign-origin row is never a candidate (D1/D2/D5).
	if err := reconcileRetention(con, onDisk, tombstoned, nowEpoch(), RetentionMirror()); err != nil {
		return err
	}
	return nil
}

// reconcileRetention reconciles the indexed sessions against the live scan,
// implementing durable retention (D1/D2/D5). It REPLACES the old prune that
// deleted any session whose backing file was absent from the disk walk. For each
// file_index row:
//
//   - file back on disk → clear any stale missing_since (the source reappeared,
//     mirroring Zoekt restoring a repo from .trash).
//   - file absent + session explicitly tombstoned (rawclaw delete) → really
//     DELETE the row; an explicit user delete is the ONLY thing that prunes (D5).
//   - file absent + foreign origin_machine (another machine's row in a shared
//     store) → skip untouched: out of THIS scan's scope, not "missing" (D2).
//   - file absent + this machine's own row → stamp missing_since and RETAIN it,
//     so the content stays searchable/readable after the source tool purges its
//     transcripts (D1). Idempotent: an existing timestamp is left as-is.
//
// onDisk is the realpath set of the live scan; tombstoned is the loaded delete
// sidecar; both are computed once by the caller. mirror is passed in (not read
// here) because the setting only governs LIVE-scope scans: an orphan reconcile
// always passes false — already-retained history is removed by an explicit
// tombstone alone, never as a side effect of a search run with the mirror
// setting in the environment (live-verified data-loss footgun).
func reconcileRetention(con *sql.DB, onDisk, tombstoned map[string]struct{}, now float64, mirror bool) error {
	type fiRow struct {
		path      string
		sessionID string
		origin    sql.NullString
		missing   sql.NullFloat64
	}
	rows, err := con.Query(
		`SELECT fi.path, fi.session_id, s.origin_machine, s.missing_since
		   FROM file_index fi
		   LEFT JOIN sessions s ON s.id = fi.session_id`)
	if err != nil {
		return fmt.Errorf("scan file_index for retention: %w", err)
	}
	// Read fully into memory first so the UPDATE/DELETEs below don't mutate a live
	// cursor.
	var all []fiRow
	for rows.Next() {
		var r fiRow
		if err := rows.Scan(&r.path, &r.sessionID, &r.origin, &r.missing); err != nil {
			rows.Close()
			return fmt.Errorf("scan retention row: %w", err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate retention rows: %w", err)
	}
	rows.Close()

	mid := provenance.MachineID()
	for _, r := range all {
		_, present := onDisk[r.path]
		own := !r.origin.Valid || r.origin.String == mid
		switch decideRetention(present, isMember(tombstoned, r.sessionID), own, r.missing.Valid, mirror) {
		case actClear: // reappeared — un-flag
			if _, err := con.Exec("UPDATE sessions SET missing_since=NULL WHERE id=?", r.sessionID); err != nil {
				return fmt.Errorf("clear missing_since: %w", err)
			}
		case actPrune: // explicit tombstone, or own-source under the mirror setting
			if err := pruneSession(con, r.sessionID, r.path); err != nil {
				return err
			}
		case actStamp: // own-source, newly absent — retain + flag (D1)
			if _, err := con.Exec("UPDATE sessions SET missing_since=? WHERE id=?", now, r.sessionID); err != nil {
				return fmt.Errorf("mark missing_since: %w", err)
			}
		case actNone:
		}
	}
	return nil
}

// retentionAction is the decision for one indexed row during a retention pass:
// what an acting reconcile should do — and, equally, what the read-only orphan
// probe predicts it WOULD do. One tree, two consumers, so precedence
// (present → tombstone → foreign → mirror → stamp) can never silently diverge
// between them.
type retentionAction int

const (
	actNone  retentionAction = iota // present-and-unflagged, foreign-origin (D2), or already flagged
	actClear                        // file reappeared — clear the stale missing_since (Zoekt .trash restore)
	actPrune                        // explicit tombstone (D5), or own-source under the user's mirror setting
	actStamp                        // own-source newly absent — retain + flag missing_since (D1)
)

// decideRetention is the single retention decision tree shared by
// reconcileRetention (acts) and orphanWorkPending (predicts).
func decideRetention(present, tombstoned, own, missingSet, mirror bool) retentionAction {
	switch {
	case present && missingSet:
		return actClear
	case present:
		return actNone
	case tombstoned:
		return actPrune
	case !own:
		return actNone // foreign-origin — out of this scan's scope (D2)
	case mirror:
		return actPrune // v0.2.0 parity: the user opted out of retention
	case !missingSet:
		return actStamp
	default:
		return actNone // already flagged — idempotent
	}
}

// pruneSession removes one session outright: messages, session row, and its
// file_index watermark. Reached only by an explicit tombstone or by the user's
// mirror setting — never by mere absence under the keep default.
func pruneSession(con *sql.DB, sessionID, path string) error {
	if _, err := con.Exec("DELETE FROM messages WHERE session_id=?", sessionID); err != nil {
		return fmt.Errorf("prune messages: %w", err)
	}
	if _, err := con.Exec("DELETE FROM sessions WHERE id=?", sessionID); err != nil {
		return fmt.Errorf("prune sessions: %w", err)
	}
	if _, err := con.Exec("DELETE FROM file_index WHERE path=?", path); err != nil {
		return fmt.Errorf("prune file_index: %w", err)
	}
	return nil
}

// RetentionMirror reports whether RAWCLAW_RETENTION selects mirror mode: an
// absent own-source file prunes its session at the next index pass, matching
// the pre-retention releases. Every other value — including unset and typos —
// is keep (the default): retention is the user's choice, and a typo must never
// silently turn deletion on.
func RetentionMirror() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("RAWCLAW_RETENTION")), "mirror")
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
		return CountTopLevelSessions(dbp), nil // can't write — fall back to a read count
	}
	defer con.Close()

	if err := EnsureSchema(con, sourceClaude); err != nil {
		if isBusy(err) {
			return CountTopLevelSessions(dbp), nil
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
	if err := reconcileRetention(con, map[string]struct{}{}, tombstoned, nowEpoch(), false); err != nil {
		if isBusy(err) {
			return CountTopLevelSessions(dbp), nil
		}
		return 0, fmt.Errorf("orphan reconcile: %w", err)
	}
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE is_subagent=0").Scan(&n); err != nil {
		if isBusy(err) {
			return CountTopLevelSessions(dbp), nil
		}
		return 0, fmt.Errorf("orphan count: %w", err)
	}
	return n, nil
}

// EnsureOrphanReconciled reconciles an orphaned index db read-MOSTLY (F2/F4):
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
		// (present=false — the whole source is gone) with mirror=false (matching
		// ReconcileOrphanDB: retained rows die only by tombstone).
		// Any predicted action is pending work.
		own := !origin.Valid || origin.String == mid
		if decideRetention(false, isMember(tombstoned, id), own, missing.Valid, false) != actNone {
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

// CountSessions opens dbp read-only and returns the session count, or -1 on
// error (callers must treat <0 as unknown).
func CountSessions(dbp string) int {
	con, err := store.ConnectRO(dbp)
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
	con, err := store.ConnectRO(dbp)
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

	con, openErr := store.ConnectRW(dbp)
	if openErr != nil {
		// Treat an open/lock failure as a fall-back to the existing index.
		return dbp, CountSessions(dbp), IndexStale, nil
	}
	defer con.Close()

	if err := EnsureSchema(con, sourceClaude); err != nil {
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
	con, err := store.ConnectRO(dbp)
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
