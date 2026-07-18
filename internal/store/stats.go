// Corpus stats surface (D6): session counts and aggregate stats over one
// indexed project's db, moved verbatim from internal/index. These open their
// own read-only connection from a db path (they are whole-db aggregates, not
// per-connection reads).
package store

import (
	"database/sql"
	"fmt"
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
