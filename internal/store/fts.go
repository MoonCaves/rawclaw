// FTS read surface: the two keyword-recall queries (flat hits and anchor rows)
// over messages_fts + messages + sessions, with dynamic filter composition.
// MATCH-expression building (OR-rewrite, sanitizing) stays in the retrieve
// layer — store receives the finished FTS5 expression and owns only the SQL.
package store

import (
	"database/sql"
	"strings"
)

// Filter is the shared WHERE composition for SearchHits / SearchAnchors (D5).
// The zero value applies only the top-level-sessions filter (IncludeSubagents
// false = s.is_subagent=0, matching the consumers' default).
type Filter struct {
	IncludeSubagents bool   // false = top-level sessions only (s.is_subagent=0)
	Role             string // "" = any; else m.role=Role
	MinMessages      int    // 0 = no minimum; else s.message_count >= MinMessages
	SinceDate        string // "" = no bound; else substr(m.ts_iso,1,10) >= SinceDate (YYYY-MM-DD inclusive)
	BeforeDate       string // "" = no bound; else substr(m.ts_iso,1,10) <= BeforeDate (YYYY-MM-DD inclusive)
}

// Sort selects the ORDER BY for SearchHits / SearchAnchors (D5).
type Sort int

const (
	// SortRelevance orders by FTS5 bm25 rank, then m.id (the default).
	SortRelevance Sort = iota
	// SortNewest orders by m.ts DESC, m.id DESC (a recency overlay — replaces
	// relevance entirely).
	SortNewest
	// SortOldest orders by m.ts ASC, m.id ASC.
	SortOldest
)

// orderClause maps a Sort onto its SQL ORDER BY — byte-identical to the
// retrieve layer's clauses.
func orderClause(s Sort) string {
	switch s {
	case SortNewest:
		return "ORDER BY m.ts DESC, m.id DESC"
	case SortOldest:
		return "ORDER BY m.ts ASC, m.id ASC"
	default:
		return "ORDER BY rank, m.id"
	}
}

// ftsWhere composes the shared WHERE clause list + args for an FTS query:
// MATCH first, then the optional filters in the consumers' exact order
// (subagent, role, min-messages, since, before).
func ftsWhere(match string, f Filter) (where []string, args []any) {
	where = []string{"messages_fts MATCH ?"}
	args = []any{match}
	if !f.IncludeSubagents {
		where = append(where, "s.is_subagent=0")
	}
	if f.Role != "" {
		where = append(where, "m.role=?")
		args = append(args, f.Role)
	}
	if f.MinMessages != 0 {
		where = append(where, "s.message_count >= ?")
		args = append(args, f.MinMessages)
	}
	if f.SinceDate != "" {
		where = append(where, "substr(m.ts_iso,1,10) >= ?")
		args = append(args, f.SinceDate)
	}
	if f.BeforeDate != "" {
		where = append(where, "substr(m.ts_iso,1,10) <= ?")
		args = append(args, f.BeforeDate)
	}
	return where, args
}

// SearchHit is one flat keyword-recall row: the session/message columns plus
// the raw content (for the tool-stripped snippet rebuild + coverage count) and
// the FTS5-built snippet.
type SearchHit struct {
	SessionID  string
	Role       string
	ISO        string
	IsSubagent bool
	Parent     string
	Content    string
	Snippet    string
}

// SearchHits runs the flat FTS5 keyword query and returns up to `limit` rows in
// the requested order. `match` is a finished FTS5 MATCH expression. The snippet
// format — snippet(messages_fts,0,'>>>','<<<','…',16) — is part of the output
// contract and stays byte-identical. [retrieve.searchScored]
func SearchHits(con *sql.DB, match string, f Filter, s Sort, limit int) ([]SearchHit, error) {
	where, args := ftsWhere(match, f)
	sqlText := `SELECT m.session_id, m.role, m.ts_iso, s.is_subagent, s.parent_id, m.content,
	                   snippet(messages_fts,0,'>>>','<<<','…',16) AS snip
	            FROM messages_fts JOIN messages m ON m.id=messages_fts.rowid
	            JOIN sessions s ON s.id=m.session_id
	            WHERE ` + strings.Join(where, " AND ") + " " + orderClause(s) + " LIMIT ?"
	args = append(args, limit)

	rows, err := con.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SearchHit
	for rows.Next() {
		var (
			sid     string
			role    sql.NullString
			iso     sql.NullString
			isSub   int
			parent  sql.NullString
			content sql.NullString
			snip    sql.NullString
		)
		if err := rows.Scan(&sid, &role, &iso, &isSub, &parent, &content, &snip); err != nil {
			return nil, err
		}
		out = append(out, SearchHit{
			SessionID:  sid,
			Role:       role.String,
			ISO:        iso.String,
			IsSubagent: isSub != 0,
			Parent:     parent.String,
			Content:    content.String,
			Snippet:    snip.String,
		})
	}
	return out, rows.Err()
}

// SearchAnchor is one anchor-recall row: a SearchHit shape keyed by message id,
// plus the source uuid (the stable read-ref handle) and the session's
// missing_since watermark (>0 = source file gone but row retained, D7 flag).
type SearchAnchor struct {
	ID           int
	SessionID    string
	UUID         string
	Role         string
	ISO          string
	Parent       string
	Content      string
	MissingSince float64
	Snippet      string
}

// SearchAnchors runs the anchor-recall FTS5 query — the same filters and order
// as SearchHits, returning message ids + uuid + missing_since for the view
// layer to expand into bookend windows. [retrieve.MatchAnchors]
func SearchAnchors(con *sql.DB, match string, f Filter, s Sort, limit int) ([]SearchAnchor, error) {
	where, args := ftsWhere(match, f)
	sqlText := `SELECT m.id, m.session_id, m.uuid, m.role, m.ts_iso, s.parent_id, m.content, s.missing_since,
	                   snippet(messages_fts,0,'>>>','<<<','…',16) AS snip
	            FROM messages_fts JOIN messages m ON m.id=messages_fts.rowid
	            JOIN sessions s ON s.id=m.session_id
	            WHERE ` + strings.Join(where, " AND ") + " " + orderClause(s) + " LIMIT ?"
	args = append(args, limit)

	rows, err := con.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SearchAnchor
	for rows.Next() {
		var (
			mid     int
			sid     string
			uuid    sql.NullString
			role    sql.NullString
			iso     sql.NullString
			parent  sql.NullString
			content sql.NullString
			missing sql.NullFloat64
			snip    sql.NullString
		)
		if err := rows.Scan(&mid, &sid, &uuid, &role, &iso, &parent, &content, &missing, &snip); err != nil {
			return nil, err
		}
		out = append(out, SearchAnchor{
			ID:           mid,
			SessionID:    sid,
			UUID:         uuid.String,
			Role:         role.String,
			ISO:          iso.String,
			Parent:       parent.String,
			Content:      content.String,
			MissingSince: missing.Float64, // 0 when NULL (present)
			Snippet:      snip.String,
		})
	}
	return out, rows.Err()
}

// TopicRowsExist reports whether the topic_segment table holds any row — used
// to distinguish "query matched nothing" from "nothing is tagged yet". A
// missing table / read error reads as false. [agentproto.topicRowsExist]
func TopicRowsExist(con *sql.DB) bool {
	var one int
	err := con.QueryRow("SELECT 1 FROM topic_segment LIMIT 1").Scan(&one)
	return err == nil
}
