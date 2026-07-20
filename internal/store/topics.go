// Topic query surface: the topic_segment / topic_fts SQL, moved verbatim
// from internal/index so the sidecar's table/column names live beside its DDL
// (EnsureTopicSchema in store.go). Ingest orchestration stays in index.
package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// TopicSegment is one tagged segment of a session, returned by TopicsForSession
// for the outline view. Keyed externally by (session_id, start_uuid).
// OriginMachine records which machine authored the tag (provenance.MachineID of
// the tagging machine) — the attribution the cross-machine ingest resolves on.
type TopicSegment struct {
	SessionID     string
	StartUUID     string
	EndUUID       string
	Topic         string
	Summary       string
	TaggedAt      float64
	OriginMachine string
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
// (session_id, start_uuid). This is the AUTHORING path (local tag-write): the
// local author's intent wins unconditionally. origin_machine is left NULL — a
// locally-authored tag is "this machine" by construction, and a NULL origin is
// interpreted as this machine at export; only the cross-machine INGEST
// path (ReplaceSessionSegments) ever stamps a non-NULL, foreign origin. The
// external-content FTS triggers keep topic_fts in sync — an ON CONFLICT UPDATE
// fires the AFTER UPDATE trigger, which re-syncs the changed topic/summary.
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

// ReplaceSessionSegments is the cross-machine INGEST primitive. It replaces
// a session's ENTIRE segment set atomically — DELETE the session's rows, then
// INSERT the incoming set — mirroring index.ReindexFile's per-session atomic
// replace. This is deliberately NOT a per-segment merge: two independent taggings
// of one session with different segment boundaries would interleave into a
// franken-set under per-key union. The session's tagging is ONE authored unit, so
// it is applied as one unit.
//
// WHICH set wins is decided by the caller (archive ingest) via PROVENANCE
// AUTHORITY — the machine the session's transcript lives under owns its tags —
// NOT wall-clock: independent authorings differ in QUALITY, not freshness, and a
// clock cannot rank quality (and skews across machines). This function just
// applies the chosen set. Idempotent: replacing a set with an identical set is a
// no-op net of the FTS trigger churn, so re-ingesting the same files converges.
//
// segs carry their own OriginMachine (the authoring machine); an empty segs
// clears the session's segments (the caller does this only when authority says so).
func ReplaceSessionSegments(con *sql.DB, sessionID string, segs []TopicSegment) error {
	tx, err := con.Begin()
	if err != nil {
		return fmt.Errorf("begin replace segments: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit
	if _, err := tx.Exec("DELETE FROM topic_segment WHERE session_id=?", sessionID); err != nil {
		return fmt.Errorf("clear session segments: %w", err)
	}
	for _, s := range segs {
		if _, err := tx.Exec(
			`INSERT INTO topic_segment(session_id, start_uuid, end_uuid, topic, summary, tagged_at, origin_machine)
			 VALUES(?,?,?,?,?,?,?)`,
			sessionID, s.StartUUID, s.EndUUID, s.Topic, s.Summary, s.TaggedAt, s.OriginMachine,
		); err != nil {
			return fmt.Errorf("insert ingested segment: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace segments: %w", err)
	}
	return nil
}

// SessionHasRealSegments reports whether a session carries at least one real topic
// segment (a non-empty topic) — the read-time signal that "a real tag beats
// routine" (a routine verdict is inert when real segments exist) and the
// cross-machine authority tie-break "real beats routine/empty". A missing topic
// table reads as false (no real tags).
func SessionHasRealSegments(con *sql.DB, sessionID string) (bool, error) {
	var n int
	err := con.QueryRow(
		"SELECT COUNT(*) FROM topic_segment WHERE session_id=? AND topic IS NOT NULL AND topic<>''",
		sessionID,
	).Scan(&n)
	if err != nil {
		return false, nil // missing table / read error reads as "no real segments"
	}
	return n > 0, nil
}

// TopicsForSession returns the topic segments for one session, ordered by id
// (insertion order — roughly chronological as the tagger walks the session).
// Used by the outline view. A missing topic table reads as "no topics".
func TopicsForSession(con *sql.DB, sessionID string) ([]TopicSegment, error) {
	rows, err := con.Query(
		"SELECT session_id, start_uuid, end_uuid, topic, summary, tagged_at, origin_machine FROM topic_segment WHERE session_id=? ORDER BY id",
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
			at      sql.NullFloat64
			origin  sql.NullString
		)
		if err := rows.Scan(&seg.SessionID, &seg.StartUUID, &endU, &topic, &summary, &at, &origin); err != nil {
			return nil, fmt.Errorf("scan topic segment: %w", err)
		}
		seg.EndUUID = endU.String
		seg.Topic = topic.String
		seg.Summary = summary.String
		seg.TaggedAt = at.Float64
		seg.OriginMachine = origin.String
		out = append(out, seg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topic segments: %w", err)
	}
	return out, nil
}

// SessionIDsIn returns every session id present in a db's sessions table — the
// set the archive tag-ingest walks to attach pulled tags to their message-bearing
// session. A missing table reads as none (non-fatal).
func SessionIDsIn(con *sql.DB) ([]string, error) {
	rows, err := con.Query("SELECT id FROM sessions")
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan session id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session ids: %w", err)
	}
	return out, nil
}

// TaggedSessionIDs returns the distinct session ids that carry ANY tag — a topic
// segment or a verdict — in this db. The archive-export walk enumerates
// these to write one tag file per tagged session. Missing tables read as "none"
// (non-fatal): a db that predates the topic sidecar simply exports no tags.
func TaggedSessionIDs(con *sql.DB) ([]string, error) {
	rows, err := con.Query(`
SELECT session_id FROM topic_segment
UNION
SELECT session_id FROM session_verdict
ORDER BY session_id`)
	if err != nil {
		return nil, nil // missing sidecar tables read as no tagged sessions
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			return nil, fmt.Errorf("scan tagged session id: %w", err)
		}
		out = append(out, sid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tagged session ids: %w", err)
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
