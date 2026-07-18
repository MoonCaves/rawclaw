// Sessions read surface: the typed queries over the sessions table that the
// view / agentproto / retrieve / cli layers previously issued as inline SQL.
// Every method reproduces its consumer site's WHERE/ORDER BY/LIMIT semantics
// exactly.
package store

import "database/sql"

// BrowseSession is one recent-session row returned by BrowseSessions:
// (id, last_ts, message_count). Preview text is a caller concern.
type BrowseSession struct {
	SessionID    string
	LastTS       float64
	MessageCount int
}

// BrowseSessions returns a project's most-recent TOP-LEVEL sessions
// (is_subagent=0), newest first by last_ts. since/before ("" = no bound) are
// inclusive LOCAL-date bounds on last_ts (date(last_ts,'unixepoch','localtime')).
// The rows are fully drained before returning, so the single connection is free
// for follow-up queries (D3). [view.Browse]
func BrowseSessions(con *sql.DB, since, before string, limit int) ([]BrowseSession, error) {
	where := []string{"s.is_subagent=0"}
	var args []any
	if since != "" {
		where = append(where, "date(s.last_ts,'unixepoch','localtime') >= ?")
		args = append(args, since)
	}
	if before != "" {
		where = append(where, "date(s.last_ts,'unixepoch','localtime') <= ?")
		args = append(args, before)
	}
	args = append(args, limit)

	whereSQL := where[0]
	for _, w := range where[1:] {
		whereSQL += " AND " + w
	}
	q := `SELECT s.id, s.last_ts, s.message_count
	      FROM sessions s WHERE ` + whereSQL + ` ORDER BY s.last_ts DESC LIMIT ?`

	rows, err := con.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BrowseSession
	for rows.Next() {
		var (
			id     string
			lastTS sql.NullFloat64
			n      sql.NullInt64
		)
		if err := rows.Scan(&id, &lastTS, &n); err != nil {
			return nil, err
		}
		out = append(out, BrowseSession{SessionID: id, LastTS: lastTS.Float64, MessageCount: int(n.Int64)})
	}
	return out, rows.Err()
}

// SessionsByPrefix returns up to `limit` session ids with the given id prefix,
// ordered by id. includeSubagents=false adds is_subagent=0 (top-level only).
// Callers pass a small limit (2 for the git-style ambiguity guard, 3 for resume
// candidates) — enough rows to DETECT a collision without fetching the world.
// [agentproto.locateSession, cli.codexResumeHits]
func SessionsByPrefix(con *sql.DB, prefix string, includeSubagents bool, limit int) ([]string, error) {
	q := "SELECT id FROM sessions WHERE id LIKE ? ORDER BY id LIMIT ?"
	if !includeSubagents {
		q = "SELECT id FROM sessions WHERE id LIKE ? AND is_subagent = 0 ORDER BY id LIMIT ?"
	}
	rows, err := con.Query(q, prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SessionMeta reads a session's last_ts + message_count. A missing row (or any
// read error) reads as ok=false. ISO formatting of lastTS stays caller-side.
// A NULL last_ts reads as 0. [agentproto.sessionMeta]
func SessionMeta(con *sql.DB, sid string) (lastTS float64, msgCount int, ok bool) {
	var ts sql.NullFloat64
	var mc sql.NullInt64
	row := con.QueryRow("SELECT last_ts, message_count FROM sessions WHERE id=?", sid)
	if err := row.Scan(&ts, &mc); err != nil {
		return 0, 0, false
	}
	return ts.Float64, int(mc.Int64), true
}

// ParentOf returns a session's parent_id, or "" when the session is missing,
// the parent is NULL/empty, or the read fails — the lineage walk treats all
// three identically as "root reached". [retrieve.LineageRoot]
func ParentOf(con *sql.DB, sid string) string {
	var parent sql.NullString
	err := con.QueryRow("SELECT parent_id FROM sessions WHERE id=?", sid).Scan(&parent)
	if err != nil || !parent.Valid {
		return ""
	}
	return parent.String
}
