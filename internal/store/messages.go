// Messages read surface: the typed queries over the messages table (and its
// sessions join) that the view / agentproto / semantic / cli layers previously
// issued as inline SQL. Ordering within a session is by message id (insertion
// order), NOT ts — ts can be non-monotonic, so id is the reliable ordering key.
package store

import "database/sql"

// Msg is the (id, role, content) triple read by the window/bookend queries.
type Msg struct {
	ID      int
	Role    string
	Content string
}

// MessagesBefore returns up to `limit` messages at or before anchorID
// (id<=anchorID — the anchor row is INCLUDED), ordered id DESC (nearest first;
// callers reverse for ascending display). [view.BuildAnchoredView]
func MessagesBefore(con *sql.DB, sid string, anchorID, limit int) ([]Msg, error) {
	return readMsgs(con,
		`SELECT id,role,content FROM messages WHERE session_id=? AND id<=? ORDER BY id DESC LIMIT ?`,
		sid, anchorID, limit)
}

// MessagesAfter returns up to `limit` messages strictly after anchorID
// (id>anchorID), ordered id ASC. [view.BuildAnchoredView]
func MessagesAfter(con *sql.DB, sid string, anchorID, limit int) ([]Msg, error) {
	return readMsgs(con,
		`SELECT id,role,content FROM messages WHERE session_id=? AND id>? ORDER BY id ASC LIMIT ?`,
		sid, anchorID, limit)
}

// BookendMessages returns up to `limit` user/assistant messages with non-empty
// content, ordered by id in the given direction. With hasBound, the window is
// bounded exclusive of boundID on the far side of the scan: ascending reads
// id<boundID (the run-up to a window), descending reads id>boundID (the
// tail after it) — matching the view's bookend queries. Without a bound it is
// the outline's session-start/-end bookend. [view.BuildAnchoredView,
// agentproto.bookendRows]
func BookendMessages(con *sql.DB, sid string, boundID int, hasBound, asc bool, limit int) ([]Msg, error) {
	dir := "DESC"
	bound := " AND id>?"
	if asc {
		dir = "ASC"
		bound = " AND id<?"
	}
	q := `SELECT id,role,content FROM messages WHERE session_id=?`
	args := []any{sid}
	if hasBound {
		q += bound
		args = append(args, boundID)
	}
	q += ` AND role IN ('user','assistant') AND length(content)>0 ORDER BY id ` + dir + ` LIMIT ?`
	args = append(args, limit)
	return readMsgs(con, q, args...)
}

// CountMessagesBetween counts ONE session's messages strictly between two row
// ids; beforeID <= 0 means no upper bound. Scoped by session_id on purpose: in
// a database holding every project, row ids run across all of them and a
// session that ran in two working directories has its rows split around other
// sessions' rows, so subtracting two ids counts strangers. [agentproto.Outline]
func CountMessagesBetween(con *sql.DB, sid string, afterID, beforeID int) (int, error) {
	q := "SELECT COUNT(*) FROM messages WHERE session_id=? AND id>?"
	args := []any{sid, afterID}
	if beforeID > 0 {
		q += " AND id<?"
		args = append(args, beforeID)
	}
	var n int
	if err := con.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// readMsgs runs a (id, role, content) query and scans the rows.
func readMsgs(con *sql.DB, query string, args ...any) ([]Msg, error) {
	rows, err := con.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Msg
	for rows.Next() {
		var m Msg
		if err := rows.Scan(&m.ID, &m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// FirstUserMessages returns the contents of a session's first `limit` non-empty
// user messages, in id order — the browse-preview scan window. A NULL content
// reads as "". [view.sessionPreview]
func FirstUserMessages(con *sql.DB, sid string, limit int) ([]string, error) {
	rows, err := con.Query(
		`SELECT content FROM messages WHERE session_id=? AND role='user'
		   AND length(content)>0 ORDER BY id ASC LIMIT ?`,
		sid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var content sql.NullString
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		out = append(out, content.String)
	}
	return out, rows.Err()
}

// SessionMessage is one full session-spine row: (id, uuid, role, content).
type SessionMessage struct {
	ID      int
	UUID    string
	Role    string
	Content string
}

// SessionMessages reads a session's messages in id order (id ascending) — the
// chronological spine the tag dump and segment-range mapping walk.
// [cli.loadSessionMessages]
func SessionMessages(con *sql.DB, sid string) ([]SessionMessage, error) {
	rows, err := con.Query(
		"SELECT id, uuid, role, content FROM messages WHERE session_id=? ORDER BY id",
		sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionMessage
	for rows.Next() {
		var m SessionMessage
		if err := rows.Scan(&m.ID, &m.UUID, &m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MessageRow is one corpus-wide (id, session_id, content) row.
type MessageRow struct {
	ID        int
	SessionID string
	Content   string
}

// AllMessages returns every message's (id, session_id, content) — the vector
// indexer's full corpus scan. A NULL content reads as "". Unordered (table
// scan), matching the consumer. [semantic.VecIndex]
func AllMessages(con *sql.DB) ([]MessageRow, error) {
	rows, err := con.Query("SELECT id, session_id, content FROM messages")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MessageRow
	for rows.Next() {
		var (
			m       MessageRow
			content sql.NullString
		)
		if err := rows.Scan(&m.ID, &m.SessionID, &content); err != nil {
			return nil, err
		}
		m.Content = content.String
		out = append(out, m)
	}
	return out, rows.Err()
}

// ResolveMessageUUID returns up to `limit` message ids in sid whose uuid has
// the given prefix, ordered by id. Callers pass limit=2 (the git-style
// ambiguity idiom: 0 matches = not found, 1 = resolved, 2 = ambiguous — never
// silently pick one). [agentproto.resolveUUID]
func ResolveMessageUUID(con *sql.DB, sid, uuidPrefix string, limit int) ([]int, error) {
	rows, err := con.Query(
		"SELECT id FROM messages WHERE session_id=? AND uuid LIKE ? ORDER BY id LIMIT ?",
		sid, uuidPrefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MessageUUID resolves a message rowid to its uuid. A missing row (or any read
// error, or a NULL uuid) reads as "". [agentproto.msgUUID]
func MessageUUID(con *sql.DB, msgID int) string {
	var uuid sql.NullString
	if err := con.QueryRow("SELECT uuid FROM messages WHERE id=?", msgID).Scan(&uuid); err != nil {
		return ""
	}
	return uuid.String
}

// MessageMeta reads one message's ts_iso plus its session's parent_id,
// is_subagent, and missing_since (the vector-candidate existence check). A
// missing/churned row reads as ok=false; NULL fields read as their zero
// values. [semantic.VecKNN]
func MessageMeta(con *sql.DB, msgID int) (iso, parent string, isSubagent bool, missingSince float64, ok bool) {
	var (
		isoN    sql.NullString
		parentN sql.NullString
		isSub   int
		missing sql.NullFloat64
	)
	err := con.QueryRow(
		"SELECT m.ts_iso, s.parent_id, s.is_subagent, s.missing_since FROM messages m "+
			"JOIN sessions s ON s.id=m.session_id WHERE m.id=?", msgID,
	).Scan(&isoN, &parentN, &isSub, &missing)
	if err != nil {
		return "", "", false, 0, false
	}
	return isoN.String, parentN.String, isSub != 0, missing.Float64, true
}

// LastMessages returns a session's final `limit` messages as (role, content),
// newest first. Bounded on purpose: answering "what is this session doing now"
// only needs the tail, and a session can hold thousands of rows.
//
// Ported from OpenClaw's tail-preview reader
// (src/gateway/session-utils.fs.ts, readLastMessagePreviewFromOpenTranscript),
// which seeks to size-16KB in the transcript file and keeps the last 20 lines.
// We have an index, so the bounded byte read becomes a bounded ORDER BY DESC —
// same contract, cheaper. Both roles are returned: the newest thing that
// happened is as often the agent's reply as the operator's instruction.
func LastMessages(con *sql.DB, sid string, limit int) ([]SessionMessage, error) {
	rows, err := con.Query(
		`SELECT id, role, content FROM messages WHERE session_id=?
		   AND length(content)>0 ORDER BY id DESC LIMIT ?`,
		sid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionMessage
	for rows.Next() {
		var (
			id           int
			role, conten sql.NullString
		)
		if err := rows.Scan(&id, &role, &conten); err != nil {
			return nil, err
		}
		out = append(out, SessionMessage{ID: id, Role: role.String, Content: conten.String})
	}
	return out, rows.Err()
}

// NewestHumanMessageID returns the id of the newest message in a session that a
// PERSON actually typed, or 0 when there is none.
//
// Why a query and not a scan: an earlier version walked back a fixed window of
// trailing records looking for a human turn, and it silently found nothing.
// Measured on the live corpus, ~85% of role='user' rows are machinery — tool
// results, notifications, slash-command plumbing, reminder blocks — so in a
// working session the newest human turn can sit far behind the tail. It was 65
// records back in the session that exposed this, against a 40-record window, and
// the caller degraded to "no turn found" without saying so. A predicate has no
// window to be wrong about.
//
// The NOT LIKE list is the one the untagged-session census already validated on
// this corpus, plus the interruption marker. length>2 drops bare punctuation
// acks.
func NewestHumanMessageID(con *sql.DB, sid string) (int, error) {
	var id sql.NullInt64
	err := con.QueryRow(`
SELECT id FROM messages
WHERE session_id=? AND role='user' AND length(trim(content))>2
  AND content NOT LIKE '[TOOL_RESULT]%'
  AND content NOT LIKE '[TOOL:%'
  AND content NOT LIKE '[Request interrupted%'
  AND content NOT LIKE '<command-%'
  AND content NOT LIKE '<local-command-%'
  AND content NOT LIKE '<task-notification>%'
  AND content NOT LIKE '<system-reminder>%'
  AND content NOT LIKE '<bash-%'
  AND content NOT LIKE '<environment_context>%'
ORDER BY id DESC LIMIT 1`, sid).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int(id.Int64), nil
}
