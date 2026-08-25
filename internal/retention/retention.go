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

// Result reports which sessions a reconcile pass acted on, by id. The pass
// owns the decision but not everything that has to follow it: the durable
// transcript vault keeps its own copy of the only-copy watermark, and a pruned
// session's vault copy has to go too or the next rebuild resurrects it. Rather
// than teach this package about the vault, hand the caller the outcome.
type Result struct {
	Cleared []string // only_copy_since un-flagged: the source reappeared
	Stamped []string // retained + flagged: own source newly absent (RawClaw is now the only copy)
	Pruned  []string // rows deleted: tombstone, replica absence, or mirror mode
}

// ReconcileRetention reconciles the indexed sessions against the live scan,
// implementing durable retention (D1/D2/D5). It REPLACES the old prune that
// deleted any session whose backing file was absent from the disk walk. For each
// file_index row:
//
//   - file back on disk → clear any stale only_copy_since (the source reappeared,
//     mirroring Zoekt restoring a repo from .trash).
//   - file absent + REPLICA scope → DELETE the row: the scanned tree is the
//     synced archive clone, whose only file-removal mechanism is the owner's
//     explicit delete propagating (E5) — so this is still a user delete
//     acting, one hop removed, never mere local absence.
//   - file absent + session explicitly tombstoned (rawclaw delete) → really
//     DELETE the row; in a LOCAL scope an explicit user delete is the ONLY
//     thing that prunes (D5).
//   - file absent + foreign origin_machine (another machine's row in a shared
//     store) → skip untouched: out of THIS scan's scope, not "missing" (D2).
//   - file absent + this machine's own row → stamp only_copy_since and RETAIN it,
//     so RawClaw is now the only holder after the source tool purges its
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
// scan is authoritative — see DecideRetention. The returned Result names the
// sessions each branch acted on, for callers that keep state outside the db.
func ReconcileRetention(con *sql.DB, onDisk, tombstoned map[string]struct{}, now float64, mirror, replica bool) (Result, error) {
	var res Result
	type fiRow struct {
		path      string
		sessionID string
		origin    sql.NullString
		onlyCopy  sql.NullFloat64
	}
	rows, err := con.Query(
		`SELECT fi.path, fi.session_id, s.origin_machine, s.only_copy_since
		   FROM file_index fi
		   LEFT JOIN sessions s ON s.id = fi.session_id`)
	if err != nil {
		return res, fmt.Errorf("scan file_index for retention: %w", err)
	}
	// Read fully into memory first so the UPDATE/DELETEs below don't mutate a live
	// cursor.
	var all []fiRow
	for rows.Next() {
		var r fiRow
		if err := rows.Scan(&r.path, &r.sessionID, &r.origin, &r.onlyCopy); err != nil {
			rows.Close()
			return res, fmt.Errorf("scan retention row: %w", err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, fmt.Errorf("iterate retention rows: %w", err)
	}
	rows.Close()

	mid := provenance.MachineID()
	for _, r := range all {
		_, present := onDisk[r.path]
		own := !r.origin.Valid || r.origin.String == mid
		switch DecideRetention(present, isMember(tombstoned, r.sessionID), own, r.onlyCopy.Valid, mirror, replica) {
		case ActClear: // reappeared — un-flag
			if _, err := con.Exec("UPDATE sessions SET only_copy_since=NULL WHERE id=?", r.sessionID); err != nil {
				return res, fmt.Errorf("clear only_copy_since: %w", err)
			}
			res.Cleared = append(res.Cleared, r.sessionID)
		case ActPrune: // replica absence, explicit tombstone, or own-source under mirror
			if err := pruneSession(con, r.sessionID, r.path); err != nil {
				return res, err
			}
			res.Pruned = append(res.Pruned, r.sessionID)
		case ActStamp: // own-source, newly absent — retain + flag only_copy_since (D1)
			if _, err := con.Exec("UPDATE sessions SET only_copy_since=? WHERE id=?", now, r.sessionID); err != nil {
				return res, fmt.Errorf("mark only_copy_since: %w", err)
			}
			res.Stamped = append(res.Stamped, r.sessionID)
		case ActNone:
		}
	}
	return res, nil
}

// RetentionAction is the decision for one indexed row during a retention pass:
// what an acting reconcile should do — and, equally, what the read-only orphan
// probe predicts it WOULD do. One tree, two consumers, so precedence
// (present → replica → tombstone → foreign → mirror → stamp) can never
// silently diverge between them.
type RetentionAction int

const (
	ActNone  RetentionAction = iota // present-and-unflagged, foreign-origin (D2), or already flagged
	ActClear                        // file reappeared — clear the stale only_copy_since (Zoekt .trash restore)
	ActPrune                        // replica absence (propagated E5 delete), explicit tombstone (D5), or own-source under the user's mirror setting
	ActStamp                        // own-source newly absent — retain + flag only_copy_since (RawClaw is now the only copy) (D1)
)

// DecideRetention is the single retention decision tree shared by
// ReconcileRetention (acts) and index's orphanWorkPending (predicts).
// replica marks an ARCHIVE-replica scope: the scanned tree is the local copy
// of the archive clone — the source of truth for its foreign sessions — and
// the db is a rebuildable cache of it, so a file absent from the scan means
// the owner's delete propagated through the archive (E5) and the rows die
// here too. Durable retention (D1) protects LOCAL sources from upstream
// purges; it must never let a replica resurrect a session its owner deleted.
func DecideRetention(present, tombstoned, own, onlyCopySet, mirror, replica bool) RetentionAction {
	switch {
	case present && onlyCopySet:
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
	case !onlyCopySet:
		return ActStamp
	default:
		return ActNone // already flagged — idempotent
	}
}

// pruneSession removes one session outright: messages, session row, and its
// file_index watermark. Reached by an explicit tombstone, the user's mirror
// setting, or replica-scope absence (an owner's delete propagated through the
// archive — E5). In a LOCAL scope under the keep default, mere absence never
// reaches here.
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
