// Package retention is the durable-retention policy tree (D1/D2/D5) applied to
// an indexed scope: the single decision function shared by acting reconciles and
// read-only probes, the reconcile pass that acts on it, and the user's
// mirror-mode opt-out. It imports store (schema home) transitively via
// provenance, which supplies the machine identity that scopes "own" rows.
package retention

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/provenance"
)

// ReconcileRetention reconciles the indexed sessions against the live scan,
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
// setting in the environment (a live-verified data-loss footgun). replica
// marks an ARCHIVE-replica scope (the scanned tree is a synced copy of
// another machine's data inside the archive clone): there, absence from the
// scan is authoritative — see DecideRetention.
func ReconcileRetention(con *sql.DB, onDisk, tombstoned map[string]struct{}, now float64, mirror, replica bool) error {
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
		switch DecideRetention(present, isMember(tombstoned, r.sessionID), own, r.missing.Valid, mirror, replica) {
		case ActClear: // reappeared — un-flag
			if _, err := con.Exec("UPDATE sessions SET missing_since=NULL WHERE id=?", r.sessionID); err != nil {
				return fmt.Errorf("clear missing_since: %w", err)
			}
		case ActPrune: // explicit tombstone, or own-source under the mirror setting
			if err := pruneSession(con, r.sessionID, r.path); err != nil {
				return err
			}
		case ActStamp: // own-source, newly absent — retain + flag (D1)
			if _, err := con.Exec("UPDATE sessions SET missing_since=? WHERE id=?", now, r.sessionID); err != nil {
				return fmt.Errorf("mark missing_since: %w", err)
			}
		case ActNone:
		}
	}
	return nil
}

// RetentionAction is the decision for one indexed row during a retention pass:
// what an acting reconcile should do — and, equally, what the read-only orphan
// probe predicts it WOULD do. One tree, two consumers, so precedence
// (present → tombstone → foreign → mirror → stamp) can never silently diverge
// between them.
type RetentionAction int

const (
	ActNone  RetentionAction = iota // present-and-unflagged, foreign-origin (D2), or already flagged
	ActClear                        // file reappeared — clear the stale missing_since (Zoekt .trash restore)
	ActPrune                        // explicit tombstone (D5), or own-source under the user's mirror setting
	ActStamp                        // own-source newly absent — retain + flag missing_since (D1)
)

// DecideRetention is the single retention decision tree shared by
// ReconcileRetention (acts) and index's orphanWorkPending (predicts).
// replica marks an ARCHIVE-replica scope: the scanned tree is the local copy
// of the archive clone — the source of truth for its foreign sessions — and
// the db is a rebuildable cache of it, so a file absent from the scan means
// the owner's delete propagated through the archive (E5) and the rows die
// here too. Durable retention (D1) protects LOCAL sources from upstream
// purges; it must never let a replica resurrect a session its owner deleted.
func DecideRetention(present, tombstoned, own, missingSet, mirror, replica bool) RetentionAction {
	switch {
	case present && missingSet:
		return ActClear
	case present:
		return ActNone
	case replica:
		return ActPrune // absent from the replica tree: propagated delete (E5)
	case tombstoned:
		return ActPrune
	case !own:
		return ActNone // foreign-origin — out of this scan's scope (D2)
	case mirror:
		return ActPrune // v0.2.0 parity: the user opted out of retention
	case !missingSet:
		return ActStamp
	default:
		return ActNone // already flagged — idempotent
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

// isMember reports whether id is in set (comma-ok membership; a nil set is
// simply empty and never panics on read). Duplicated from index rather than
// imported: index imports this package, so the reverse would cycle.
func isMember(set map[string]struct{}, id string) bool {
	_, ok := set[id]
	return ok
}
