package index

// Import reconciliation for the claude-web source. Unlike claude/codex — a live
// transcript directory re-scanned on every run and watermarked per file in
// file_index — claude-web is fed only by an explicit `rawclaw import`, and its
// unit of identity is the conversation uuid, not a backing file. A re-export is
// a DIFFERENT zip holding the SAME conversations, so a file-path watermark can't
// tell "unchanged" from "moved". This path therefore reconciles by conversation
// identity: it upserts each incoming conversation (merging new messages by
// uuid, never duplicating or shrinking) and reconciles the db's existing
// conversations against the incoming full-export snapshot using the SAME
// retention decision tree the file-scan path uses (retention.DecideRetention),
// so an absent conversation is retained+flagged by default and pruned only under
// the user's mirror setting — guarded so a stale export can't wipe fresher data.

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/retention"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// claudeWebWatermarkPrefix keys, in the db's meta table, the newest conversation
// updated_at seen across imports FOR ONE ACCOUNT (the full key is the prefix +
// account uuid). The mirror-mode prune is gated on the incoming export's newest
// updated_at being >= this value, so re-importing an OLDER export can never
// prune conversations a newer export already established (a mis-clicked stale
// zip is inert). Keying per account keeps one account's freshness from gating
// another's prune.
const claudeWebWatermarkPrefix = "claudeweb_import_newest_updated_at:"

// sourceClaudeWeb is the source_tool stamped on claude-web rows (parallels
// sourceClaude). Used by the in-place schema-ensure's durability backfill.
const sourceClaudeWeb = "claude-web"

// EnsureClaudeWebSchema ensures a claude-web db's schema WITHOUT ever taking the
// destructive DROP+rebuild path (store.Rebuild) that the generic EnsureSchema
// uses on a schema-version mismatch. claude-web is the ONLY source with no
// reconstructible source tree — this db is the SOLE copy of the user's imported
// cloud history — so a rebuild would erase all of it. Schema evolution here is
// therefore IN-PLACE and data-preserving only, mirroring migrateDurabilityColumns
// (additive ALTER + backfill) and EnsureTopicSchema/EnsureVecSchema (own gates,
// objects outside dropSQL).
//
// CONTRACT — loud, not silent:
//   - fresh / current / OLDER version -> ensure the current objects exist
//     additively and re-stamp, preserving every row (all base-schema evolution
//     in this repo is additive: columns added in place, topic/vec on own gates).
//   - NEWER version (a db written by a future binary) -> FAIL CLOSED: this binary
//     can't safely read a shape it doesn't know. Never guess, never rebuild.
//   - unreadable version -> FAIL CLOSED for the same reason.
//
// A future NON-additive base change is the one case this can't auto-handle: the
// maintainer who lands it MUST extend this function with an explicit
// data-preserving migration (and fail closed for versions below its floor)
// rather than let an older db silently re-stamp into a mismatched shape.
func EnsureClaudeWebSchema(con *sql.DB) error {
	var version string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&version); err != nil {
		// No readable version marker: a fresh (or pre-marker) db. The only "build"
		// claude-web does is additive CREATE ... IF NOT EXISTS — never a DROP.
		return ensureClaudeWebSchemaInPlace(con)
	}
	v, convErr := strconv.Atoi(version)
	switch {
	case convErr != nil:
		return fmt.Errorf("claude-web: unreadable db schema_version %q — refusing to open (the destructive rebuild path would erase import-only data)", version)
	case v > store.SchemaVersion:
		return fmt.Errorf("claude-web: db schema_version %d is NEWER than this binary supports (%d) — refusing to open; upgrade rawclaw (a rebuild would erase import-only data)", v, store.SchemaVersion)
	default:
		// current or older -> additive in-place ensure + re-stamp (data-preserving).
		return ensureClaudeWebSchemaInPlace(con)
	}
}

// ensureClaudeWebSchemaInPlace creates any missing base objects (tables, indexes,
// FTS) additively, runs the in-place durability-column migration, and stamps the
// current schema_version — never dropping anything. Idempotent on a fresh or a
// current db.
func ensureClaudeWebSchemaInPlace(con *sql.DB) error {
	if _, err := con.Exec(store.Schema); err != nil {
		return fmt.Errorf("claude-web ensure base schema: %w", err)
	}
	if _, err := con.Exec("SELECT 1 FROM messages_fts LIMIT 1"); err != nil {
		if _, err := con.Exec(store.FTSSQL); err != nil {
			return fmt.Errorf("claude-web ensure fts: %w", err)
		}
	}
	// Additive, PRAGMA-guarded durability columns (a no-op for claude-web rows,
	// which ImportClaudeWeb always stamps, but kept for parity with the generic
	// in-place migration).
	if err := migrateDurabilityColumns(con, sourceClaudeWeb); err != nil {
		return fmt.Errorf("claude-web ensure durability columns: %w", err)
	}
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO meta(key,value) VALUES('schema_version',?)", strconv.Itoa(store.SchemaVersion),
	); err != nil {
		return fmt.Errorf("claude-web stamp schema_version: %w", err)
	}
	return nil
}

// ClaudeWebImportStats is one import's reconciliation outcome, for the summary
// line. Conversations partition into added (new) / updated (gained messages) /
// skipped (already fully present); absent-from-this-export conversations are
// either retained (kept + flagged deleted-upstream, the default) or pruned
// (deleted under RAWCLAW_RETENTION=mirror).
type ClaudeWebImportStats struct {
	AddedConversations   int `json:"added_conversations"`
	UpdatedConversations int `json:"updated_conversations"`
	SkippedConversations int `json:"skipped_conversations"`
	AddedMessages        int `json:"added_messages"`
	RetainedAbsent       int `json:"retained_absent"` // kept + flagged (default keep)
	PrunedAbsent         int `json:"pruned_absent"`   // deleted (mirror)
	TotalConversations   int `json:"total_conversations"`
}

// ImportClaudeWeb ingests one export's COMPLETE conversation set (cs) into the
// import-sourced db at dbp, reconciling by conversation identity WITHIN one
// account. msgs pulls a container's normalized messages; sourceID is stamped as
// source_tool; account is the export's owning account uuid (each export is
// single-account); newestUpdatedAt is the export's newest conversation
// updated_at (the staleness signal for the mirror guard). It is idempotent:
// re-importing the same export changes nothing; re-importing a superset appends
// only the new messages.
//
// The account is the reconciliation boundary: Pass 2 (absent-conversation
// retention) only ever considers sessions of THIS account, and the staleness
// watermark is per-account, so importing account B never flags or prunes
// account A's conversations. Until slice 03's per-account db, the account rides
// in each row's source_path (which has no filesystem meaning for an import
// source).
//
// A busy/locked db is a soft no-op (returns zero stats, no error) mirroring the
// file-scan path's degrade-don't-fail posture; any other write failure is
// returned (fail-closed — a real error never masquerades as a clean import).
func ImportClaudeWeb(dbp string, cs []source.Container, msgs MessagesFunc, sourceID, account string, newestUpdatedAt float64) (ClaudeWebImportStats, error) {
	var stats ClaudeWebImportStats

	// F-3: an export with no account uuid gives every account-less import the
	// SAME reconcile bucket + watermark (source_path=""), so under mirror one
	// such import could cross-prune another's conversations. Refuse it under
	// mirror (fail-closed, before any db side effect); under the keep default the
	// shared bucket is harmless (keep never prunes), so it is allowed.
	if account == "" && retention.RetentionMirror() {
		return stats, fmt.Errorf("claude-web import: refusing an export with no account uuid under RAWCLAW_RETENTION=mirror — account-less imports share one reconcile bucket, so mirror could cross-prune unrelated conversations; import under the default (keep) instead")
	}

	con, openErr := store.ConnectRW(dbp)
	if openErr != nil {
		return stats, nil // can't open (locked) — degrade to a no-op, like the file-scan path
	}
	defer con.Close()

	// EnsureClaudeWebSchema — NOT the generic EnsureSchema: claude-web is
	// import-only (this db is the sole copy), so it must NEVER take the
	// destructive DROP+rebuild path a schema-version mismatch triggers.
	if err := EnsureClaudeWebSchema(con); err != nil {
		if isBusy(err) {
			return stats, nil
		}
		return stats, fmt.Errorf("claude-web import: ensure schema: %w", err)
	}

	existing, err := loadClaudeWebSessions(con, sourceID, account)
	if err != nil {
		return stats, fmt.Errorf("claude-web import: load existing sessions: %w", err)
	}
	existingMsgs, err := loadMessageUUIDs(con)
	if err != nil {
		return stats, fmt.Errorf("claude-web import: load message ids: %w", err)
	}

	tombstoned, terr := lifecycle.LoadTombstones("")
	if terr != nil {
		tombstoned = map[string]struct{}{} // best-effort: never block an import
	}

	// Staleness guard (per account): the mirror prune is honored only when this
	// export is at least as fresh as the freshest one already imported FOR THIS
	// account — account B's freshness must never gate account A's prune.
	stored := readWatermark(con, account)
	effectiveMirror := retention.RetentionMirror() && newestUpdatedAt >= stored

	// Pass 1: upsert every incoming conversation (present = un-flag + merge).
	incoming := make(map[string]struct{}, len(cs))
	for _, c := range cs {
		incoming[c.ID] = struct{}{}
		if isMember(tombstoned, c.ID) {
			// A user-deleted conversation stays deleted even if re-exported:
			// never (re)index it, and remove any existing row so a later export
			// can't resurrect it (pass 2 skips it as "present", so prune here).
			if _, existed := existingMsgs[c.ID]; existed {
				if err := deleteSession(con, c.ID); err != nil {
					return stats, err
				}
			}
			continue
		}
		ms, mErr := msgs(c)
		if mErr != nil {
			continue // bad container: leave any existing rows untouched, never partial-write
		}
		priorUUIDs, existed := existingMsgs[c.ID]
		added, err := upsertClaudeWebSession(con, c, ms, sourceID, account, priorUUIDs, existed)
		if err != nil {
			return stats, err
		}
		switch {
		case !existed:
			stats.AddedConversations++
			stats.AddedMessages += added
		case added > 0:
			stats.UpdatedConversations++
			stats.AddedMessages += added
		default:
			stats.SkippedConversations++
		}
	}

	// Pass 2: reconcile conversations absent from THIS export against the shared
	// retention decision tree (present=false). `existing` holds ONLY this
	// account's sessions, so a conversation from another account (a separate
	// import) is never seen here as "absent" — importing account B can neither
	// flag nor prune account A's conversations. Within the account, only rows not
	// in the incoming set reach a decision, so a same-account superset re-export
	// prunes nothing. This is the F2 data-loss class the shared-Path file_index
	// watermark caused, closed on both the per-conversation and per-account axes.
	now := nowEpoch()
	mid := provenance.MachineID()
	for id, s := range existing {
		if _, present := incoming[id]; present {
			continue
		}
		own := !s.origin.Valid || s.origin.String == mid
		switch retention.DecideRetention(false, isMember(tombstoned, id), own, s.missing.Valid, effectiveMirror, false) {
		case retention.ActPrune:
			if err := deleteSession(con, id); err != nil {
				return stats, err
			}
			stats.PrunedAbsent++
		case retention.ActStamp:
			if _, err := con.Exec("UPDATE sessions SET missing_since=? WHERE id=?", now, id); err != nil {
				return stats, fmt.Errorf("claude-web import: flag missing_since: %w", err)
			}
			stats.RetainedAbsent++
		case retention.ActClear, retention.ActNone:
		}
	}

	// Advance this account's staleness watermark (only ever forward).
	if newestUpdatedAt > stored {
		if err := writeWatermark(con, account, newestUpdatedAt); err != nil {
			return stats, fmt.Errorf("claude-web import: write watermark: %w", err)
		}
	}

	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&stats.TotalConversations); err != nil {
		return stats, fmt.Errorf("claude-web import: count sessions: %w", err)
	}
	return stats, nil
}

// sessionRow is the existing-session state the reconciliation reads.
type sessionRow struct {
	origin  sql.NullString
	missing sql.NullFloat64
}

// loadClaudeWebSessions snapshots THIS account's existing sessions (origin +
// missing_since), keyed by id, BEFORE the upsert pass mutates them. The account
// filter (source_path = account) is the reconciliation boundary: another
// account's sessions are never loaded, so Pass 2 can never treat them as absent.
func loadClaudeWebSessions(con *sql.DB, sourceID, account string) (map[string]sessionRow, error) {
	rows, err := con.Query("SELECT id, origin_machine, missing_since FROM sessions WHERE source_tool=? AND source_path=?", sourceID, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]sessionRow{}
	for rows.Next() {
		var id string
		var r sessionRow
		if err := rows.Scan(&id, &r.origin, &r.missing); err != nil {
			return nil, err
		}
		out[id] = r
	}
	return out, rows.Err()
}

// loadMessageUUIDs returns, per session id, the set of message uuids already
// indexed — the dedup key for the append-only merge. A session present here
// (even with an empty set) "existed"; absent means a brand-new conversation.
func loadMessageUUIDs(con *sql.DB) (map[string]map[string]struct{}, error) {
	rows, err := con.Query("SELECT session_id, uuid FROM messages")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]struct{}{}
	for rows.Next() {
		var sid string
		var uuid sql.NullString
		if err := rows.Scan(&sid, &uuid); err != nil {
			return nil, err
		}
		set, ok := out[sid]
		if !ok {
			set = map[string]struct{}{}
			out[sid] = set
		}
		if uuid.Valid && uuid.String != "" {
			set[uuid.String] = struct{}{}
		}
	}
	return out, rows.Err()
}

// upsertClaudeWebSession merges one conversation into the db: it appends only
// messages whose uuid is not already present (so a re-import never duplicates,
// and an older/smaller re-export never drops existing messages), then rewrites
// the session row with refreshed watermarks and missing_since=NULL — a present
// conversation is, by definition, not missing. Returns the number of messages
// appended. A new session inserts all its messages.
func upsertClaudeWebSession(con *sql.DB, c source.Container, ms []model.Message, sourceID, account string, priorUUIDs map[string]struct{}, existed bool) (added int, err error) {
	// F-2: a message with a uuid dedups on it (the common path). A uuidLESS
	// message can't, so on a RE-import it dedups on a content hash against the
	// session's existing uuidless rows — without this a re-import re-inserts every
	// uuidless message every time. Within a SINGLE import, identical uuidless
	// messages are NOT collapsed (they may be genuinely distinct turns — keep
	// everything); a subsequent re-import still won't duplicate them, since they
	// all hash to the one existing identity.
	var emptyHashes map[string]struct{} // existing uuidless content hashes (lazy)
	for _, m := range ms {
		if m.UUID != "" {
			if existed {
				if _, dup := priorUUIDs[m.UUID]; dup {
					continue // already indexed (idempotent skip)
				}
			}
		} else if existed {
			if emptyHashes == nil {
				if emptyHashes, err = loadEmptyUUIDHashes(con, c.ID); err != nil {
					return added, err
				}
			}
			if _, dup := emptyHashes[messageContentHash(m)]; dup {
				continue // already indexed on a prior import (content-hash dedup)
			}
		}
		if _, err := con.Exec(
			"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
			c.ID, m.Role, m.Text, m.TS, m.TSISO, m.UUID,
		); err != nil {
			return added, fmt.Errorf("claude-web import: insert message: %w", err)
		}
		added++
	}

	// Refresh the session watermarks from the (now-merged) message set.
	var started, last sql.NullFloat64
	var count int
	if err := con.QueryRow(
		"SELECT MIN(NULLIF(ts,0)), MAX(ts), COUNT(*) FROM messages WHERE session_id=?", c.ID,
	).Scan(&started, &last, &count); err != nil {
		return added, fmt.Errorf("claude-web import: recompute watermarks: %w", err)
	}
	var parentArg any
	if c.ParentID != "" {
		parentArg = c.ParentID
	}
	// source_path holds the owning ACCOUNT uuid — for an import source it has no
	// filesystem meaning, so it carries the reconciliation-scoping account
	// instead (also how the per-account db self-identifies its account and how the
	// legacy split groups). missing_since NULL: a present conversation is not missing.
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,origin_machine,source_tool,source_path,missing_since) VALUES(?,?,?,?,?,?,?,?,?,NULL)",
		c.ID, nullFloat(started), nullFloat(last), count, b2i(c.IsSubagent), parentArg, originOr(""), sourceID, account,
	); err != nil {
		return added, fmt.Errorf("claude-web import: upsert session: %w", err)
	}
	return added, nil
}

// messageContentHash is the dedup identity for a uuidless message: a stable hash
// of its role + content + iso timestamp (a message with none of a uuid, text, or
// timestamp is degenerate and hashes the same as its twin — correctly deduped).
func messageContentHash(m model.Message) string {
	return hashParts(m.Role, m.Text, m.TSISO)
}

// loadEmptyUUIDHashes returns the content hashes of a session's already-indexed
// uuidless messages — the existing side of the F-2 content-hash dedup. Usually
// empty (real exports carry uuids), so this query normally returns no rows.
func loadEmptyUUIDHashes(con *sql.DB, sessionID string) (map[string]struct{}, error) {
	rows, err := con.Query(
		"SELECT role, content, COALESCE(ts_iso,'') FROM messages WHERE session_id=? AND (uuid IS NULL OR uuid='')", sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("claude-web import: load uuidless hashes: %w", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var role, content, tsISO string
		if err := rows.Scan(&role, &content, &tsISO); err != nil {
			return nil, err
		}
		out[hashParts(role, content, tsISO)] = struct{}{}
	}
	return out, rows.Err()
}

// hashParts hashes the identity fields of a message, NUL-separated so distinct
// field splits can't collide.
func hashParts(role, content, tsISO string) string {
	sum := sha1.Sum([]byte(role + "\x00" + content + "\x00" + tsISO))
	return hex.EncodeToString(sum[:])
}

// deleteSession removes one session's messages and row (mirror-mode prune or an
// honored tombstone). claude-web writes no file_index rows, so there is no
// watermark to clean up.
func deleteSession(con *sql.DB, sessionID string) error {
	if _, err := con.Exec("DELETE FROM messages WHERE session_id=?", sessionID); err != nil {
		return fmt.Errorf("claude-web import: prune messages: %w", err)
	}
	if _, err := con.Exec("DELETE FROM sessions WHERE id=?", sessionID); err != nil {
		return fmt.Errorf("claude-web import: prune session: %w", err)
	}
	return nil
}

// nullFloat unwraps a nullable aggregate to a plain float (0 when NULL — e.g. a
// session whose every message carried no parseable timestamp).
func nullFloat(v sql.NullFloat64) float64 {
	if v.Valid {
		return v.Float64
	}
	return 0
}

// ReadClaudeWebWatermark reads an account's stored newest-updated_at watermark
// from the db at dbp (0 when the db, meta row, or value is absent/unreadable).
// Exported for the legacy single-db -> per-account-db migration, which carries
// each account's staleness watermark forward into its new db.
func ReadClaudeWebWatermark(dbp, account string) float64 {
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return 0
	}
	defer con.Close()
	return readWatermark(con, account)
}

// readWatermark reads an account's stored newest-updated_at watermark (0 when
// unset).
func readWatermark(con *sql.DB, account string) float64 {
	var s string
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", claudeWebWatermarkPrefix+account).Scan(&s); err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// writeWatermark stores an account's newest-updated_at watermark.
func writeWatermark(con *sql.DB, account string, v float64) error {
	_, err := con.Exec(
		"INSERT OR REPLACE INTO meta(key,value) VALUES(?,?)",
		claudeWebWatermarkPrefix+account, strconv.FormatFloat(v, 'f', -1, 64),
	)
	return err
}
