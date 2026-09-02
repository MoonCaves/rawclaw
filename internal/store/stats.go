// Corpus stats surface (D6): session counts and aggregate stats over one
// indexed project's db, moved verbatim from internal/index. These open their
// own read-only connection from a db path (they are whole-db aggregates, not
// per-connection reads).
package store

import (
	"database/sql"
	"fmt"

	"github.com/MoonCaves/rawclaw/internal/text"
)

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

// CountSessions opens dbp read-only and returns the session count, or -1 on
// error (callers must treat <0 as unknown).
func CountSessions(dbp string) int {
	return countSessions(dbp, "SELECT COUNT(*) FROM sessions")
}

func countSessions(dbp, query string) int {
	con, err := ConnectRO(dbp)
	if err != nil {
		return -1
	}
	defer con.Close()
	var n int
	if err := con.QueryRow(query).Scan(&n); err != nil {
		return -1
	}
	return n
}

// CountTopLevelSessions returns the count of TOP-LEVEL sessions (is_subagent=0)
// — what a user means by "this project's sessions". Use this for display; the
// raw CountSessions above includes subagent threads and is internal bookkeeping.
// Returns -1 on error.
func CountTopLevelSessions(dbp string) int {
	return countSessions(dbp, "SELECT COUNT(*) FROM sessions WHERE is_subagent=0")
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
	if err := con.QueryRow(`
		SELECT
			COUNT(CASE WHEN is_subagent=0 THEN 1 END),
			COUNT(CASE WHEN is_subagent=1 THEN 1 END)
		FROM sessions`).Scan(&cs.Sessions, &cs.Subagents); err != nil {
		return CorpusStats{}, nil // a query error -> zero stats
	}
	var first, last sql.NullString
	if err := con.QueryRow(`
		SELECT
			COUNT(*),
			COUNT(CASE WHEN role='user' THEN 1 END),
			COUNT(CASE WHEN role='assistant' THEN 1 END),
			MIN(CASE WHEN length(ts_iso)>0 THEN ts_iso END),
			MAX(CASE WHEN length(ts_iso)>0 THEN ts_iso END)
		FROM messages`).Scan(
		&cs.Messages, &cs.User, &cs.Assistant, &first, &last); err != nil {
		return CorpusStats{}, nil
	}
	cs.First = first10(first.String)
	cs.Last = first10(last.String)
	return cs, nil
}

// first10 delegates to text.First10.
func first10(s string) string {
	return text.First10(s)
}
