// Package index (this file): reaching RETAINED sessions for `rawclaw delete`.
//
// lifecycle.Delete's matchSessions walks the live projects tree only — it can
// never see a row whose backing .jsonl the source tool already purged. That is
// exactly the row set durable retention creates (missing_since set, content
// still indexed), so without this the feature's own contract — "explicit
// delete is the ONLY way retained history dies" — was unreachable in practice
// (live-verified: `delete --project <purged>` reported "no
// sessions match"). lifecycle cannot import this package back (index already
// imports lifecycle, for LoadTombstones) so the retained-side scan lives here
// and the CLI composes the two searches.
package index

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// RetainedSession is one RETAINED top-level session matched by RetainedMatches
// — its backing .jsonl is already gone, so unlike lifecycle.PlanItem it carries
// no file size (there is no file left to size).
type RetainedSession struct {
	DBPath       string  // the index db holding the row
	SessionID    string  // .jsonl stem == the claude --resume id
	Label        string  // friendly project label (source_path's dir, or the db name)
	LastTS       float64 // last message timestamp, epoch seconds (0 if never recorded)
	MessageCount int
}

// RetainedMatches enumerates every RETAINED top-level session (missing_since
// IS NOT NULL, is_subagent=0) across every index db under cacheDir, applying
// the same filter semantics lifecycle.DeleteOpts uses so a delete plan can
// union live matches with retained ones. cacheDir defaults to store.CacheDir() when
// empty, mirroring lifecycle.TombstonePath's own-default convention.
//
// project, when non-empty, is a case-sensitive substring match against EITHER
// the row's source_path or the db's own filename — the same "path contains"
// semantic Delete's Project filter uses on the live transcript-dir path,
// extended to the db filename so an orphaned db still matches after its
// source_path predates a rename (or was never backfilled by
// migrateDurabilityColumns). before, when non-zero, keeps only sessions whose
// last_ts (epoch seconds) is strictly before it — a row with no recorded
// last_ts is excluded rather than guessed at. maxMessages, when > 0, keeps
// only sessions with at most that many messages. A zero-value filter is unset
// and does not constrain the match, same rule as DeleteOpts.
//
// A db that fails to open or query (busy, corrupt, mid-write) is skipped
// rather than failing the whole scan — the same best-effort tolerance
// scopes.orphanClaudeScopes applies to its own db enumeration.
func RetainedMatches(cacheDir string, project string, before time.Time, maxMessages int) ([]RetainedSession, error) {
	if cacheDir == "" {
		cacheDir = store.CacheDir()
	}
	entries, _ := filepath.Glob(filepath.Join(cacheDir, "*.db"))
	sort.Strings(entries)

	out := []RetainedSession{}
	for _, dbp := range entries {
		// Archive-replica dbs (the "archive-" namespace, same predicate the
		// orphan scan uses) hold FOREIGN machines' sessions — read-only
		// replicas that must never enter the delete/tombstone path, even if
		// a row in one ever carried a stray missing_since.
		if strings.HasPrefix(filepath.Base(dbp), "archive-") {
			continue
		}
		matches, err := retainedInDB(dbp, project, before, maxMessages)
		if err != nil {
			continue // unreadable db — best-effort, like orphan scope discovery
		}
		out = append(out, matches...)
	}
	return out, nil
}

// retainedInDB queries one index db read-only for its retained sessions
// passing the filter.
func retainedInDB(dbp, project string, before time.Time, maxMessages int) ([]RetainedSession, error) {
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", dbp, err)
	}
	defer con.Close()

	base := filepath.Base(dbp)

	rows, err := con.Query(
		`SELECT id, source_path, last_ts, message_count FROM sessions
		   WHERE missing_since IS NOT NULL AND is_subagent=0`)
	if err != nil {
		return nil, fmt.Errorf("query retained %q: %w", dbp, err)
	}
	defer rows.Close()

	out := []RetainedSession{}
	for rows.Next() {
		var id string
		var sourcePath sql.NullString
		var lastTS sql.NullFloat64
		var msgCount int
		if err := rows.Scan(&id, &sourcePath, &lastTS, &msgCount); err != nil {
			return nil, fmt.Errorf("scan retained row %q: %w", dbp, err)
		}

		if project != "" && !strings.Contains(sourcePath.String, project) && !strings.Contains(base, project) {
			continue
		}
		if !before.IsZero() && (!lastTS.Valid || !time.Unix(int64(lastTS.Float64), 0).Before(before)) {
			continue
		}
		if maxMessages > 0 && msgCount > maxMessages {
			continue
		}

		out = append(out, RetainedSession{
			DBPath:       dbp,
			SessionID:    id,
			Label:        retainedLabel(sourcePath.String, base),
			LastTS:       lastTS.Float64,
			MessageCount: msgCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retained %q: %w", dbp, err)
	}
	return out, nil
}

// retainedLabel picks a friendly project label for a retained row: the
// source_path's parent-dir basename when recorded — the same value
// lifecycle.matchInDir derives for a live match's Project field — else a label
// recovered from the db's own filename.
func retainedLabel(sourcePath, dbFileName string) string {
	if sourcePath != "" {
		return filepath.Base(filepath.Dir(sourcePath))
	}
	return dbBaseLabel(dbFileName)
}

// dbBaseLabel derives a friendly label from an index db filename, using the
// same "last non-empty '-' segment" rule as scopes.orphanLabel. Duplicated
// rather than imported: scopes imports index, so the reverse would cycle.
func dbBaseLabel(dbFileName string) string {
	enc := strings.TrimSuffix(dbFileName, ".db")
	parts := strings.Split(enc, "-")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return enc
}
