// Sessions read surface: the typed queries over the sessions table that the
// view / agentproto / retrieve / cli layers previously issued as inline SQL.
// Every method reproduces its consumer site's WHERE/ORDER BY/LIMIT semantics
// exactly, so moving a consumer onto them is behavior-preserving.
package store

import "database/sql"

// BrowseSession is one recent-session row returned by BrowseSessions:
// (id, project, last_ts, message_count). Preview text is a caller concern.
type BrowseSession struct {
	SessionID    string
	Project      string
	LastTS       float64
	MessageCount int
}

// BrowseSessions returns a project's most-recent TOP-LEVEL sessions
// (is_subagent=0), newest first by last_ts. since/before ("" = no bound) are
// inclusive LOCAL-date bounds on last_ts (date(last_ts,'unixepoch','localtime')).
// The rows are fully drained before returning, so the single connection is free
// for follow-up queries (D3). [view.Browse]
func BrowseSessions(con *sql.DB, since, before string, limit int) ([]BrowseSession, error) {
	return BrowseScopedSessions(con, since, before, "", nil, limit)
}

// BrowseScopedSessions returns the most-recent TOP-LEVEL sessions (is_subagent=0)
// matching the optional date, source_tool, and project filters, newest first by
// last_ts. The rows are fully drained before returning.
func BrowseScopedSessions(con *sql.DB, since, before, sourceTool string, projects []string, limit int) ([]BrowseSession, error) {
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
	if sourceTool != "" {
		where = append(where, "s.source_tool = ?")
		args = append(args, sourceTool)
	}
	if len(projects) > 0 {
		where = append(where, "s.project IN ("+placeholders(len(projects))+")")
		for _, p := range projects {
			args = append(args, p)
		}
	}
	args = append(args, limit)

	whereSQL := where[0]
	for _, w := range where[1:] {
		whereSQL += " AND " + w
	}
	q := `SELECT s.id, COALESCE(s.project,''), s.last_ts, s.message_count
	      FROM sessions s WHERE ` + whereSQL + ` ORDER BY s.last_ts DESC LIMIT ?`

	rows, err := con.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BrowseSession
	for rows.Next() {
		var (
			id      string
			project string
			lastTS  sql.NullFloat64
			n       sql.NullInt64
		)
		if err := rows.Scan(&id, &project, &lastTS, &n); err != nil {
			return nil, err
		}
		out = append(out, BrowseSession{
			SessionID:    id,
			Project:      project,
			LastTS:       lastTS.Float64,
			MessageCount: int(n.Int64),
		})
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

// SessionRow is one session matched by id prefix, carrying the project label
// the row itself records. [agentproto.locateSession]
type SessionRow struct {
	ID      string
	Project string
}

// SessionBacking is the live transcript identity recorded on a session row.
// It lets a caller re-read the exact backing container without rediscovering
// every transcript tree first.
type SessionBacking struct {
	SourceTool string
	SourcePath string
	CWD        string
	ParentID   string
	IsSubagent bool
}

// SessionBackingFor returns the source metadata for sid. ok=false means the
// session row is absent; query errors are returned so callers can fall back to
// source discovery without mistaking a broken row for a live path.
func SessionBackingFor(con *sql.DB, sid string) (backing SessionBacking, ok bool, err error) {
	var isSubagent int
	err = con.QueryRow(`
		SELECT COALESCE(source_tool,''), COALESCE(source_path,''),
		       COALESCE(cwd,''), COALESCE(parent_id,''), COALESCE(is_subagent,0)
		FROM sessions WHERE id=?`, sid).Scan(
		&backing.SourceTool,
		&backing.SourcePath,
		&backing.CWD,
		&backing.ParentID,
		&isSubagent,
	)
	if err == sql.ErrNoRows {
		return SessionBacking{}, false, nil
	}
	if err != nil {
		return SessionBacking{}, false, err
	}
	backing.IsSubagent = isSubagent != 0
	return backing, true, nil
}

// SessionRowsByPrefix answers "which session is this" against ONE database that
// holds every project. Because project is a column here, narrowing to a subset
// of projects is a WHERE clause rather than a choice of which file to open, and
// a session continued in a second directory is a single row rather than one row
// per project database — so the caller gets the merged session with nothing to
// reconcile afterwards.
//
// projects narrows to those labels; an empty list means every project. limit
// bounds the read the same way SessionsByPrefix does: fetch just enough rows to
// DETECT a collision. includeSubagents=false adds is_subagent=0 (top-level
// only). [agentproto.locateSession]
func SessionRowsByPrefix(con *sql.DB, prefix string, includeSubagents bool, projects []string, limit int) ([]SessionRow, error) {
	q := "SELECT id, COALESCE(project,'') FROM sessions WHERE id LIKE ?"
	args := []any{prefix + "%"}
	if !includeSubagents {
		q += " AND is_subagent = 0"
	}
	if len(projects) > 0 {
		q += " AND project IN (" + placeholders(len(projects)) + ")"
		for _, p := range projects {
			args = append(args, p)
		}
	}
	q += " ORDER BY id LIMIT ?"
	args = append(args, limit)

	rows, err := con.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRow
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.ID, &r.Project); err != nil {
			return nil, err
		}
		out = append(out, r)
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

// SubagentRow is one subagent child session of a parent session.
type SubagentRow struct {
	ID           string
	MessageCount int
}

// SubagentsForSession returns the child subagent sessions for a parent session ID.
func SubagentsForSession(con *sql.DB, parentSID string) ([]SubagentRow, error) {
	q := "SELECT id, message_count FROM sessions WHERE (parent_id = ? OR (id LIKE ? AND is_subagent = 1)) ORDER BY id"
	rows, err := con.Query(q, parentSID, parentSID+"/%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubagentRow
	for rows.Next() {
		var (
			id string
			n  sql.NullInt64
		)
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out = append(out, SubagentRow{ID: id, MessageCount: int(n.Int64)})
	}
	return out, rows.Err()
}
