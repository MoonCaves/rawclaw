package scopes

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/claudeweb"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// Two realistic-shaped account uuids whose first 8 chars differ (distinct slugs).
const (
	acctA = "aaaaaaaa-1111-2222-3333-444444444444"
	acctB = "bbbbbbbb-5555-6666-7777-888888888888"
)

func cwMsg(uuid, text string, ts float64) model.Message {
	return model.Message{Role: "user", Text: text, TS: ts, TSISO: "", UUID: uuid}
}

// cwSeed imports one account's conversations into the db at dbp (used to build
// both a legacy single db and per-account dbs).
func cwSeed(t *testing.T, dbp, account string, convs map[string][]model.Message, newest float64) {
	t.Helper()
	cs := make([]source.Container, 0, len(convs))
	for id := range convs {
		cs = append(cs, source.Container{ID: id})
	}
	msgs := func(c source.Container) ([]model.Message, error) { return convs[c.ID], nil }
	if _, err := index.ImportClaudeWeb(dbp, cs, msgs, claudeweb.ID, account, newest); err != nil {
		t.Fatalf("seed %s: %v", account, err)
	}
}

// sessionsByAccount returns id -> source_path(account) for every session in dbp.
func sessionsByAccount(t *testing.T, dbp string) map[string]string {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", dbp, err)
	}
	defer con.Close()
	rows, err := con.Query("SELECT id, COALESCE(source_path,'') FROM sessions")
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, acct string
		if err := rows.Scan(&id, &acct); err != nil {
			t.Fatal(err)
		}
		out[id] = acct
	}
	return out
}

// missingSinceOf returns (value, isSet) for a session's retention grace-clock.
func missingSinceOf(t *testing.T, dbp, sid string) (float64, bool) {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", dbp, err)
	}
	defer con.Close()
	var ms sql.NullFloat64
	if err := con.QueryRow("SELECT missing_since FROM sessions WHERE id=?", sid).Scan(&ms); err != nil {
		t.Fatalf("read missing_since for %s: %v", sid, err)
	}
	return ms.Float64, ms.Valid
}

func cwMsgCount(t *testing.T, dbp, sid string) int {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", dbp, err)
	}
	defer con.Close()
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sid).Scan(&n); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	return n
}

// TestMigrateLegacyClaudeWeb_SplitsTwoAccounts is the required migration
// data-safety test: a legacy single db holding two accounts splits into two
// per-account dbs with every conversation + message preserved, correctly mapped,
// the watermark carried, the legacy db retired to .migrated (never deleted), and
// a re-run is a no-op.
func TestMigrateLegacyClaudeWeb_SplitsTwoAccounts(t *testing.T) {
	emptyStore(t)
	legacy := legacyClaudeWebDBPath()

	cwSeed(t, legacy, acctA, map[string][]model.Message{
		"a1": {cwMsg("ma1", "account A first", 1), cwMsg("ma2", "account A second", 2)},
		"a2": {cwMsg("ma3", "account A other", 3)},
	}, 111)
	cwSeed(t, legacy, acctB, map[string][]model.Message{
		"b1": {cwMsg("mb1", "account B only", 4)},
	}, 222)

	// Sanity: the legacy single db really holds both accounts (the 02
	// cross-account fix keeps B's import from pruning A).
	if got := sessionsByAccount(t, legacy); len(got) != 3 {
		t.Fatalf("legacy db has %d sessions, want 3: %v", len(got), got)
	}

	if err := MigrateLegacyClaudeWeb(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Legacy retired to .migrated, NOT deleted.
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy db still present at %s (should be renamed away)", legacy)
	}
	if _, err := os.Stat(legacy + ".migrated"); err != nil {
		t.Errorf("legacy db not preserved as .migrated: %v", err)
	}

	// Account A db: exactly a1,a2, each owned by acctA, messages preserved.
	dbA := ClaudeWebDBPath(acctA)
	gotA := sessionsByAccount(t, dbA)
	if len(gotA) != 2 || gotA["a1"] != acctA || gotA["a2"] != acctA {
		t.Errorf("account A db sessions = %v, want {a1,a2} owned by acctA", gotA)
	}
	if _, leaked := gotA["b1"]; leaked {
		t.Error("account B's b1 mis-mapped into account A's db")
	}
	if cwMsgCount(t, dbA, "a1") != 2 || cwMsgCount(t, dbA, "a2") != 1 {
		t.Errorf("account A message counts wrong: a1=%d a2=%d, want 2,1", cwMsgCount(t, dbA, "a1"), cwMsgCount(t, dbA, "a2"))
	}

	// Account B db: exactly b1, owned by acctB, message preserved.
	dbB := ClaudeWebDBPath(acctB)
	gotB := sessionsByAccount(t, dbB)
	if len(gotB) != 1 || gotB["b1"] != acctB {
		t.Errorf("account B db sessions = %v, want {b1} owned by acctB", gotB)
	}
	if cwMsgCount(t, dbB, "b1") != 1 {
		t.Errorf("account B b1 message count = %d, want 1", cwMsgCount(t, dbB, "b1"))
	}

	// Watermarks carried forward per account.
	if wm := index.ReadClaudeWebWatermark(dbA, acctA); wm != 111 {
		t.Errorf("account A watermark = %v, want 111", wm)
	}
	if wm := index.ReadClaudeWebWatermark(dbB, acctB); wm != 222 {
		t.Errorf("account B watermark = %v, want 222", wm)
	}

	// Two distinct account scopes now enumerate.
	scs := ClaudeWeb()
	if len(scs) != 2 {
		t.Fatalf("ClaudeWeb() = %d scopes, want 2: %+v", len(scs), scs)
	}
	labels := map[string]bool{scs[0].Project: true, scs[1].Project: true}
	if !labels["acct-aaaaaaaa"] || !labels["acct-bbbbbbbb"] {
		t.Errorf("scope labels = %v, want acct-aaaaaaaa + acct-bbbbbbbb", labels)
	}

	// Idempotent re-run: no-op, per-account dbs unchanged.
	if err := MigrateLegacyClaudeWeb(); err != nil {
		t.Fatalf("re-run migrate: %v", err)
	}
	if len(sessionsByAccount(t, dbA)) != 2 || len(sessionsByAccount(t, dbB)) != 1 {
		t.Error("re-run migration changed the per-account dbs (must be idempotent)")
	}
}

// TestMigrateLegacyClaudeWeb_FailClosedLeavesLegacyIntact is the crown data-
// safety guarantee: if the split can't complete (here the per-account dbs can't
// be written because the cache dir is read-only, so the verify pass finds them
// missing), the migration ABORTS with an error and leaves the legacy db fully
// intact — never renamed, never partially destroyed. claude-web data is the only
// copy, so a failed migration must lose nothing.
func TestMigrateLegacyClaudeWeb_FailClosedLeavesLegacyIntact(t *testing.T) {
	emptyStore(t)
	legacy := legacyClaudeWebDBPath()
	cwSeed(t, legacy, acctA, map[string][]model.Message{"a1": {cwMsg("ma1", "must survive", 1)}}, 111)

	// Make the cache dir read-only so per-account dbs cannot be created. The
	// legacy db already exists and stays readable; the write/verify fails.
	cacheDir := store.CacheDir()
	if err := os.Chmod(cacheDir, 0o500); err != nil {
		t.Fatalf("chmod cache dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(cacheDir, 0o700) }) // restore so TempDir cleanup can remove it

	err := MigrateLegacyClaudeWeb()
	if err == nil {
		t.Fatal("migration must FAIL when it cannot write+verify the per-account dbs")
	}

	// Restore write access and confirm the legacy db is untouched (present, not
	// renamed) with all its data.
	_ = os.Chmod(cacheDir, 0o700)
	if _, statErr := os.Stat(legacy); statErr != nil {
		t.Errorf("legacy db was removed/renamed despite a failed migration: %v", statErr)
	}
	if _, statErr := os.Stat(legacy + ".migrated"); statErr == nil {
		t.Error("legacy db was retired (.migrated) despite a failed migration")
	}
	if got := sessionsByAccount(t, legacy); len(got) != 1 || got["a1"] != acctA {
		t.Errorf("legacy db data altered by a failed migration: %v", got)
	}
}

// TestMigrateLegacyClaudeWeb_PreservesTombstone: a soft-tombstoned conversation
// (missing_since set in the legacy db) must stay tombstoned after the split —
// same grace-clock, NOT resurrected to present. The migration moves data; it
// must not change retention semantics.
func TestMigrateLegacyClaudeWeb_PreservesTombstone(t *testing.T) {
	emptyStore(t)
	legacy := legacyClaudeWebDBPath()

	// a1 + a2 imported, then a re-import (keep) drops a2 → a2 gets missing_since.
	cwSeed(t, legacy, acctA, map[string][]model.Message{
		"a1": {cwMsg("ma1", "present", 1)},
		"a2": {cwMsg("ma2", "will be tombstoned", 2)},
	}, 100)
	cwSeed(t, legacy, acctA, map[string][]model.Message{"a1": {cwMsg("ma1", "present", 1)}}, 200)

	legacyMS, set := missingSinceOf(t, legacy, "a2")
	if !set {
		t.Fatal("test premise broken: a2 should be tombstoned in the legacy db")
	}

	if err := MigrateLegacyClaudeWeb(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	dbA := ClaudeWebDBPath(acctA)
	// a2 stays tombstoned with the SAME grace-clock (not reset, not resurrected).
	gotMS, gotSet := missingSinceOf(t, dbA, "a2")
	if !gotSet {
		t.Error("a2 was RESURRECTED to present by the migration (missing_since cleared)")
	}
	if gotMS != legacyMS {
		t.Errorf("a2 grace-clock reset by migration: got %v, want %v (unchanged)", gotMS, legacyMS)
	}
	// a1 stays present (missing_since NULL).
	if _, set := missingSinceOf(t, dbA, "a1"); set {
		t.Error("a1 (present) wrongly flagged missing_since after migration")
	}
}

// TestMigrateLegacyClaudeWeb_NoLegacyNoOp: a fresh install (no legacy db) is a
// clean no-op.
func TestMigrateLegacyClaudeWeb_NoLegacyNoOp(t *testing.T) {
	emptyStore(t)
	if err := MigrateLegacyClaudeWeb(); err != nil {
		t.Fatalf("no-legacy migrate must be a no-op, got %v", err)
	}
	if scs := ClaudeWeb(); len(scs) != 0 {
		t.Errorf("no import yet, want 0 scopes, got %+v", scs)
	}
}

// TestMigrateLegacyClaudeWeb_EmptyLegacy: an empty legacy db (schema only, no
// sessions) is retired cleanly with no per-account dbs and no error.
func TestMigrateLegacyClaudeWeb_EmptyLegacy(t *testing.T) {
	emptyStore(t)
	legacy := legacyClaudeWebDBPath()
	// Create the db with schema but zero sessions.
	cwSeed(t, legacy, "", map[string][]model.Message{}, 0)
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("empty legacy db not created: %v", err)
	}

	if err := MigrateLegacyClaudeWeb(); err != nil {
		t.Fatalf("empty-legacy migrate: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("empty legacy db should be retired to .migrated")
	}
	if _, err := os.Stat(legacy + ".migrated"); err != nil {
		t.Errorf("empty legacy db not preserved as .migrated: %v", err)
	}
	if scs := ClaudeWeb(); len(scs) != 0 {
		t.Errorf("empty legacy → want 0 account scopes, got %+v", scs)
	}
}

// TestClaudeWeb_TwoAccountsTwoScopes: importing two accounts yields two distinct
// acct-<uuid8> scopes with zero user input (ticket checkbox 1).
func TestClaudeWeb_TwoAccountsTwoScopes(t *testing.T) {
	emptyStore(t)
	cwSeed(t, ClaudeWebDBPath(acctA), acctA, map[string][]model.Message{"a1": {cwMsg("ma1", "A", 1)}}, 1)
	cwSeed(t, ClaudeWebDBPath(acctB), acctB, map[string][]model.Message{"b1": {cwMsg("mb1", "B", 2)}}, 2)

	scs := ClaudeWeb()
	if len(scs) != 2 {
		t.Fatalf("want 2 account scopes, got %d: %+v", len(scs), scs)
	}
	for _, s := range scs {
		if s.Source != claudeweb.ID {
			t.Errorf("scope source = %q, want claude-web", s.Source)
		}
		if s.CWD != "" {
			t.Errorf("claude-web scope must have empty CWD (excluded from --this-project), got %q", s.CWD)
		}
		if !strings.HasPrefix(s.Project, "acct-") {
			t.Errorf("scope label %q must be an acct-<uuid8> id", s.Project)
		}
	}
	if scs[0].Project == scs[1].Project {
		t.Errorf("two accounts collapsed to one label: %q", scs[0].Project)
	}
}

// TestClaudeWebDBPath_SlashFree pins ticket checkbox 2: no "/" or ":" in any
// account identifier, verified against db-name AND scope-label formation, even
// for pathological account strings containing those separators.
func TestClaudeWebDBPath_SlashFree(t *testing.T) {
	for _, account := range []string{
		acctA,
		"weird/slashes:colons-in-id",
		"acct/../escape",
		"::::",
	} {
		dbp := ClaudeWebDBPath(account)
		base := filepath.Base(dbp)
		if strings.ContainsAny(base, "/:") {
			t.Errorf("db name %q for account %q contains / or :", base, account)
		}
		if slug := accountSlug(account); strings.ContainsAny(slug, "/:") {
			t.Errorf("account slug %q for %q contains / or :", slug, account)
		}
		if label := accountLabelFromDB(dbp); strings.ContainsAny(label, "/:") {
			t.Errorf("scope label %q for %q contains / or :", label, account)
		}
	}
}

// TestClaudeWebDBPath_Injective: two accounts sharing the first 8 uuid chars
// still map to DISTINCT dbs (the full-uuid hash), so one account's conversations
// can never merge into another's.
func TestClaudeWebDBPath_Injective(t *testing.T) {
	a := "abcd1234-0000-0000-0000-000000000001"
	b := "abcd1234-0000-0000-0000-000000000002" // same first 8, different uuid
	if accountSlug(a) != accountSlug(b) {
		t.Fatalf("test premise broken: slugs differ (%q vs %q)", accountSlug(a), accountSlug(b))
	}
	if ClaudeWebDBPath(a) == ClaudeWebDBPath(b) {
		t.Errorf("accounts sharing an 8-char slug collided on one db: %q", ClaudeWebDBPath(a))
	}
}
