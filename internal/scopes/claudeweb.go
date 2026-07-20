package scopes

// The claude-web account scope axis. Each Claude account (the account.uuid every
// conversation carries) gets its OWN cache db, exactly as Codex gives each cwd
// group its own db — so an account is a first-class search scope with zero user
// input, the account never has to be pushed as a filter into the search SQL, and
// the one-scope-one-db invariant holds. The account is a separate dimension
// beside source / origin_machine / cwd: `source` stays flat "claude-web"; the
// account surfaces as the scope's slash-free `acct-<uuid8>` label. A legacy
// pre-per-account single db (from an earlier build) is split into per-account
// dbs by MigrateLegacyClaudeWeb, fail-closed and never destructively.

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/claudeweb"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// claudeWebDBStem is the shared prefix of every claude-web cache db — the legacy
// single db ("claude-web.db") and the per-account dbs
// ("claude-web-acct-<uuid8>-<hash8>.db"). orphanClaudeScopes skips this prefix
// (a real Claude project db base is a path-encoding starting with "-", so it
// never collides).
const claudeWebDBStem = claudeweb.ID // "claude-web"

// ClaudeWeb surfaces one eager, read-only scope per imported Claude account.
// Unlike Claude()/Codex() it does NOT ingest — `rawclaw import` already built
// each account db — it only discovers them (globbing the per-account dbs) after
// first splitting any legacy single db. Each scope carries no CWD (the export
// drops the working dir), so it is naturally out of --this-project and present
// on bare/--all; its Project label is the account's slash-free `acct-<uuid8>`.
func ClaudeWeb() []view.Scope {
	if err := MigrateLegacyClaudeWeb(); err != nil {
		// Non-fatal for discovery: the legacy db is left fully intact (no data
		// lost), just not yet surfaced as per-account scopes. A later import or
		// search retries the split.
		slog.Warn("scopes: claude-web legacy migration deferred", "err", err)
	}
	entries, _ := filepath.Glob(filepath.Join(store.CacheDir(), claudeWebDBStem+"-acct-*.db"))
	sort.Strings(entries)
	out := make([]view.Scope, 0, len(entries))
	for _, dbp := range entries {
		out = append(out, view.Scope{Project: accountLabelFromDB(dbp), DBP: dbp, Source: claudeweb.ID})
	}
	return out
}

// ClaudeWebDBPath is the cache db for ONE account: "claude-web-acct-<uuid8>-
// <hash8>.db". The readable `acct-<uuid8>` slug is lossy (only 8 chars), so —
// exactly like codexDBPath — a hash of the FULL account uuid is appended to keep
// the path injective: two accounts sharing the first 8 uuid chars still get
// distinct dbs, so a collision can never mis-route or merge one account's
// conversations into another's. No "/" or ":" ever enters the name (both are
// load-bearing elsewhere): the slug is sanitized to alphanumerics and the hash
// is hex. Both `rawclaw import` (writer) and ClaudeWeb (reader) resolve through
// here.
func ClaudeWebDBPath(account string) string {
	key := claudeWebDBStem + "-" + accountSlug(account) + "-" + cwdHash(account)
	return index.DBPath(key)
}

// legacyClaudeWebDBPath is the pre-per-account single db that held every
// account's conversations ("claude-web.db"). Present only from an earlier build;
// MigrateLegacyClaudeWeb splits and retires it.
func legacyClaudeWebDBPath() string { return index.DBPath(claudeWebDBStem) }

// accountSlug is the slash-free display/naming segment for an account:
// "acct-<uuid8>", where uuid8 is the first 8 alphanumerics of the account uuid.
// Sanitizing to [A-Za-z0-9] guarantees no "/" or ":" reaches a db name or scope
// label (both separators are load-bearing: lineage parent/child, archive
// machine/project, the read-ref uuid8:uuid8).
func accountSlug(account string) string {
	if account == "" {
		return "acct-unknown" // account-less export (allowed only under keep; see F-3)
	}
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return -1 // drop hyphens, "/", ":", and anything else
		}
	}, account)
	r := []rune(cleaned)
	if len(r) > 8 {
		r = r[:8]
	}
	return "acct-" + string(r)
}

// accountLabelFromDB recovers the `acct-<uuid8>` label from a per-account db
// path by stripping the "claude-web-" prefix and the trailing injective
// "-<hash8>" (mirroring codexOrphanLabel's filename parse). Cheap — no db open.
func accountLabelFromDB(dbp string) string {
	base := strings.TrimSuffix(filepath.Base(dbp), ".db")
	trimmed := strings.TrimPrefix(base, claudeWebDBStem+"-")
	if i := strings.LastIndex(trimmed, "-"); i >= 0 && isHex8(trimmed[i+1:]) {
		trimmed = trimmed[:i] // strip the "-<hash8>" injective suffix
	}
	return trimmed
}

// legacySession is one session read from the legacy single db, tagged with its
// account (carried in source_path by the pre-per-account build), its messages,
// and its retention state (missing_since). The migration is a MOVE, not a fresh
// import, so this retention state must be carried forward faithfully — a
// soft-tombstoned conversation must stay tombstoned with its grace-clock
// unchanged, never resurrected to "present".
type legacySession struct {
	id      string
	account string
	missing sql.NullFloat64 // retention grace-clock; NULL = present
	msgs    []model.Message
}

// MigrateLegacyClaudeWeb splits a legacy single claude-web.db into per-account
// dbs. It is idempotent and FAIL-CLOSED, because claude-web data is import-only
// (the db is the ONLY copy — a re-export is unavailable), so a migration bug
// would be permanent loss:
//
//   - No legacy db (fresh install, or already migrated) -> no-op.
//   - Each account's sessions are written into its per-account db via the
//     idempotent import upsert (re-running never duplicates).
//   - Before retiring the legacy db, EVERY legacy session is verified present in
//     its account db with at least its messages. Any shortfall ABORTS and leaves
//     the legacy db fully intact — no partial/destructive state.
//   - On success the legacy db is RENAMED to "<db>.migrated" (NEVER deleted), so
//     it remains a recovery copy. A concurrent migrator that already renamed it
//     is treated as success.
func MigrateLegacyClaudeWeb() error {
	legacy := legacyClaudeWebDBPath()
	if _, err := os.Stat(legacy); err != nil {
		return nil // no legacy db — nothing to migrate
	}

	sessions, err := readLegacyClaudeWeb(legacy)
	if err != nil {
		return fmt.Errorf("read legacy db: %w", err)
	}

	byAccount := map[string][]legacySession{}
	for _, s := range sessions {
		byAccount[s.account] = append(byAccount[s.account], s)
	}

	// Write pass: each account's rows into its own db (idempotent upsert).
	for account, group := range byAccount {
		if err := migrateAccount(account, group, index.ReadClaudeWebWatermark(legacy, account)); err != nil {
			return fmt.Errorf("write account %s: %w", accountSlug(account), err)
		}
	}

	// Verify pass: nothing lost or mis-mapped. Abort (legacy intact) on any gap.
	for account, group := range byAccount {
		if err := verifyAccount(account, group); err != nil {
			return fmt.Errorf("verify account %s: %w — legacy db left intact at %s", accountSlug(account), err, legacy)
		}
	}

	// Retire by rename (keep the copy). A vanished source = another migrator won.
	migrated := legacy + ".migrated"
	if err := os.Rename(legacy, migrated); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("retire legacy db: %w", err)
	}
	slog.Info("scopes: split legacy claude-web db into per-account dbs",
		"accounts", len(byAccount), "legacy_kept_at", migrated)
	return nil
}

// migrateAccount writes one account's legacy sessions into its per-account db
// through the same idempotent import upsert a live import uses (carrying the
// account's staleness watermark forward), THEN restores each session's
// missing_since retention state. The upsert hardcodes missing_since=NULL — right
// for a live import where inserting means "present", but WRONG for a move: a
// soft-tombstoned conversation would be resurrected and its grace-clock reset.
// Restoring it after the write keeps the reorg state-preserving.
func migrateAccount(account string, group []legacySession, watermark float64) error {
	dbp := ClaudeWebDBPath(account)
	cs := make([]source.Container, 0, len(group))
	msgByID := make(map[string][]model.Message, len(group))
	for _, s := range group {
		cs = append(cs, source.Container{ID: s.id})
		msgByID[s.id] = s.msgs
	}
	msgs := func(c source.Container) ([]model.Message, error) { return msgByID[c.ID], nil }
	stats, err := index.ImportClaudeWeb(dbp, cs, msgs, claudeweb.ID, account, watermark)
	if err != nil {
		return err
	}
	// The import soft-no-ops if the destination db can't be opened (e.g. a
	// read-only cache dir). For a migration that MUST persist every conversation,
	// treat a short write as a HARD failure HERE — so a write problem aborts
	// fail-closed at the WRITE (before the verify pass), and the legacy db is
	// never retired.
	if persisted := stats.AddedConversations + stats.UpdatedConversations + stats.SkippedConversations; persisted < len(group) {
		return fmt.Errorf("write persisted %d of %d conversations", persisted, len(group))
	}
	return restoreMissingSince(dbp, group)
}

// restoreMissingSince re-applies each legacy session's missing_since to the
// destination, so a tombstoned conversation stays tombstoned with the SAME
// grace-clock after the move (the import upsert reset it to NULL). Idempotent: a
// re-run writes the same value.
func restoreMissingSince(dbp string, group []legacySession) error {
	con, err := store.ConnectRW(dbp)
	if err != nil {
		return fmt.Errorf("open dest for retention restore: %w", err)
	}
	defer con.Close()
	for _, s := range group {
		if !s.missing.Valid {
			continue // present in the legacy db — the upsert's NULL is already correct
		}
		if _, err := con.Exec("UPDATE sessions SET missing_since=? WHERE id=?", s.missing.Float64, s.id); err != nil {
			return fmt.Errorf("restore missing_since for %s: %w", s.id, err)
		}
	}
	return nil
}

// verifyAccount confirms every legacy session of this account landed in its
// account db (correct mapping) with at least its messages (no loss).
func verifyAccount(account string, group []legacySession) error {
	con, err := store.ConnectRO(ClaudeWebDBPath(account))
	if err != nil {
		return fmt.Errorf("open dest: %w", err)
	}
	defer con.Close()
	for _, s := range group {
		var owned int
		if err := con.QueryRow(
			"SELECT COUNT(*) FROM sessions WHERE id=? AND source_path=?", s.id, account,
		).Scan(&owned); err != nil {
			return fmt.Errorf("check session %s: %w", s.id, err)
		}
		if owned == 0 {
			return fmt.Errorf("session %s absent from this account's db (mis-mapped)", s.id)
		}
		var msgs int
		if err := con.QueryRow(
			"SELECT COUNT(*) FROM messages WHERE session_id=?", s.id,
		).Scan(&msgs); err != nil {
			return fmt.Errorf("count messages for %s: %w", s.id, err)
		}
		if msgs < len(s.msgs) {
			return fmt.Errorf("session %s has %d messages, legacy had %d (loss)", s.id, msgs, len(s.msgs))
		}
	}
	return nil
}

// readLegacyClaudeWeb reads every session (with its account from source_path)
// and its messages from the legacy db.
func readLegacyClaudeWeb(dbp string) ([]legacySession, error) {
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return nil, err
	}
	defer con.Close()

	rows, err := con.Query("SELECT id, COALESCE(source_path,''), missing_since FROM sessions")
	if err != nil {
		return nil, err
	}
	var out []legacySession
	for rows.Next() {
		var s legacySession
		if err := rows.Scan(&s.id, &s.account, &s.missing); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for i := range out {
		msgs, err := readLegacyMessages(con, out[i].id)
		if err != nil {
			return nil, err
		}
		out[i].msgs = msgs
	}
	return out, nil
}

// readLegacyMessages reads one session's messages from the legacy db in row
// order, reconstructing model.Message verbatim (content preserved, not re-parsed).
func readLegacyMessages(con *sql.DB, sessionID string) ([]model.Message, error) {
	rows, err := con.Query(
		"SELECT role, content, ts, ts_iso, COALESCE(uuid,'') FROM messages WHERE session_id=? ORDER BY id", sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Message
	for rows.Next() {
		var m model.Message
		var ts sql.NullFloat64
		var tsISO sql.NullString
		if err := rows.Scan(&m.Role, &m.Text, &ts, &tsISO, &m.UUID); err != nil {
			return nil, err
		}
		m.TS = ts.Float64
		m.TSISO = tsISO.String
		out = append(out, m)
	}
	return out, rows.Err()
}
