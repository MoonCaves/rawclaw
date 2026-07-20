package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func setSchemaVersion(t *testing.T, dbp string, v int) {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open rw: %v", err)
	}
	defer con.Close()
	if _, err := con.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES('schema_version',?)", strconv.Itoa(v)); err != nil {
		t.Fatalf("set schema_version: %v", err)
	}
}

func readSchemaVersion(t *testing.T, dbp string) string {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer con.Close()
	var v string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='schema_version'").Scan(&v); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	return v
}

// TestEnsureClaudeWebSchema_SurvivesVersionBump is the F-1 durability guarantee:
// a claude-web db written at an OLDER schema version must SURVIVE a schema-ensure
// by a newer binary — its rows preserved in place, the version re-stamped — NOT
// dropped by a rebuild (claude-web is import-only; this db is the only copy).
func TestEnsureClaudeWebSchema_SurvivesVersionBump(t *testing.T) {
	dbp := cwDB(t)
	cwImport(t, dbp, map[string][]model.Message{"c1": {msg("m1", "survive the bump", 1)}}, 100)
	setSchemaVersion(t, dbp, store.SchemaVersion-1) // simulate a db from a prior binary

	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureClaudeWebSchema(con); err != nil {
		t.Fatalf("EnsureClaudeWebSchema: %v", err)
	}
	con.Close()

	if n := messageCount(t, dbp, "c1"); n != 1 {
		t.Errorf("claude-web data LOST across a version bump: c1 has %d messages, want 1", n)
	}
	if v := readSchemaVersion(t, dbp); v != strconv.Itoa(store.SchemaVersion) {
		t.Errorf("schema_version not re-stamped to current: got %q, want %d", v, store.SchemaVersion)
	}
}

// TestEnsureClaudeWebSchema_ContrastGenericRebuildEmpties proves the fix MATTERS:
// the GENERIC EnsureSchema (what claude/codex use) DESTROYS the same db on a
// version mismatch — so routing claude-web through the in-place path is a real,
// necessary difference, not a no-op.
func TestEnsureClaudeWebSchema_ContrastGenericRebuildEmpties(t *testing.T) {
	dbp := cwDB(t)
	cwImport(t, dbp, map[string][]model.Message{"c1": {msg("m1", "doomed under generic", 1)}}, 100)
	setSchemaVersion(t, dbp, store.SchemaVersion-1)

	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(con, "claude-web"); err != nil { // the destructive path
		t.Fatal(err)
	}
	con.Close()

	if n := messageCount(t, dbp, "c1"); n != 0 {
		t.Fatalf("premise check: generic EnsureSchema should REBUILD (empty) on a mismatch, but c1 has %d messages", n)
	}
}

// TestEnsureClaudeWebSchema_FreshDbStamps: a fresh db (no version marker) is
// created in place and stamped current — the everyday import path.
func TestEnsureClaudeWebSchema_FreshDbStamps(t *testing.T) {
	dbp := filepath.Join(t.TempDir(), "fresh.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureClaudeWebSchema(con); err != nil {
		t.Fatalf("fresh ensure: %v", err)
	}
	con.Close()
	if v := readSchemaVersion(t, dbp); v != strconv.Itoa(store.SchemaVersion) {
		t.Errorf("fresh db schema_version = %q, want %d", v, store.SchemaVersion)
	}
}

// TestEnsureClaudeWebSchema_FailsClosedOnUnknownVersion: a stored version this
// binary can't additively migrate (here a FUTURE version) fails closed with an
// error and leaves the data untouched — LOUD, never silent drift, never a rebuild.
func TestEnsureClaudeWebSchema_FailsClosedOnUnknownVersion(t *testing.T) {
	dbp := cwDB(t)
	cwImport(t, dbp, map[string][]model.Message{"c1": {msg("m1", "keep me", 1)}}, 100)
	setSchemaVersion(t, dbp, store.SchemaVersion+1) // a future, un-migratable version

	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	err = EnsureClaudeWebSchema(con)
	con.Close()
	if err == nil {
		t.Fatal("must fail closed on an un-migratable version, not silently proceed")
	}
	if n := messageCount(t, dbp, "c1"); n != 1 {
		t.Errorf("fail-closed must not touch data: c1 has %d messages, want 1", n)
	}
}

// TestImportClaudeWeb_UUIDlessMessageDedup is the F-2 guard: a message with no
// uuid must not re-insert on re-import (it dedups by content hash).
func TestImportClaudeWeb_UUIDlessMessageDedup(t *testing.T) {
	dbp := cwDB(t)
	conv := map[string][]model.Message{"c1": {
		{Role: "user", Text: "has a uuid", TS: 1, UUID: "m1"},
		{Role: "assistant", Text: "no uuid on this one", TS: 2, UUID: ""},
	}}
	cwImport(t, dbp, conv, 100)
	if n := messageCount(t, dbp, "c1"); n != 2 {
		t.Fatalf("first import: c1 has %d messages, want 2", n)
	}
	cwImport(t, dbp, conv, 100) // re-import the same export
	if n := messageCount(t, dbp, "c1"); n != 2 {
		t.Errorf("uuidless message re-inserted on re-import: c1 has %d messages, want 2 (F-2 content-hash dedup)", n)
	}
}

// TestImportClaudeWeb_EmptyAccountRefusedUnderMirror is the F-3 guard: an export
// with no account uuid is refused under mirror (fail-closed, no write) but
// allowed under the keep default (isolated in the acct-unknown bucket).
func TestImportClaudeWeb_EmptyAccountRefusedUnderMirror(t *testing.T) {
	t.Run("mirror refuses", func(t *testing.T) {
		dbp := cwDB(t)
		t.Setenv("RAWCLAW_RETENTION", "mirror")
		cs, msgs := cwSource(map[string][]model.Message{"c1": {msg("m1", "x", 1)}})
		_, err := ImportClaudeWeb(dbp, cs, msgs, "claude-web", "", 100)
		if err == nil {
			t.Fatal("empty account under mirror must be refused")
		}
		if _, statErr := os.Stat(dbp); statErr == nil {
			t.Error("refused import must not create/write the db")
		}
	})
	t.Run("keep allows", func(t *testing.T) {
		dbp := cwDB(t) // no RAWCLAW_RETENTION → keep
		cs, msgs := cwSource(map[string][]model.Message{"c1": {msg("m1", "x", 1)}})
		if _, err := ImportClaudeWeb(dbp, cs, msgs, "claude-web", "", 100); err != nil {
			t.Fatalf("empty account under keep must be allowed: %v", err)
		}
		if n := messageCount(t, dbp, "c1"); n != 1 {
			t.Errorf("account-less import under keep should land: c1 has %d messages, want 1", n)
		}
	})
}

// cwSource turns a convID -> messages map into the (containers, MessagesFunc)
// pair ImportClaudeWeb consumes, in deterministic id order. Container.Path is
// the synthetic per-conversation key the real adapter uses; ImportClaudeWeb
// never stats it.
func cwSource(byConv map[string][]model.Message) ([]source.Container, MessagesFunc) {
	ids := make([]string, 0, len(byConv))
	for id := range byConv {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	cs := make([]source.Container, 0, len(ids))
	for _, id := range ids {
		cs = append(cs, source.Container{ID: id, Path: "claude-web:" + id})
	}
	msgs := func(c source.Container) ([]model.Message, error) { return byConv[c.ID], nil }
	return cs, msgs
}

// msg is a tiny message constructor.
func msg(uuid, text string, ts float64) model.Message {
	return model.Message{Role: "user", Text: text, TS: ts, TSISO: "", UUID: uuid}
}

// cwImport runs one single-account import into dbp (account "acctA").
func cwImport(t *testing.T, dbp string, byConv map[string][]model.Message, newest float64) ClaudeWebImportStats {
	return cwImportAcct(t, dbp, "acctA", byConv, newest)
}

// cwImportAcct runs one import for a specific account.
func cwImportAcct(t *testing.T, dbp, account string, byConv map[string][]model.Message, newest float64) ClaudeWebImportStats {
	t.Helper()
	cs, msgs := cwSource(byConv)
	stats, err := ImportClaudeWeb(dbp, cs, msgs, "claude-web", account, newest)
	if err != nil {
		t.Fatalf("ImportClaudeWeb: %v", err)
	}
	return stats
}

// cwDB returns an isolated (HOME-scoped) db path so lifecycle tombstones and
// the cache dir don't touch the real store.
func cwDB(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(t.TempDir(), "claude-web.db")
}

func sessionIDs(t *testing.T, dbp string) map[string]struct{} {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer con.Close()
	rows, err := con.Query("SELECT id FROM sessions")
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out[id] = struct{}{}
	}
	return out
}

func messageCount(t *testing.T, dbp, sid string) int {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer con.Close()
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sid).Scan(&n); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	return n
}

// missingSince returns (value, isSet) for a session's missing_since flag.
func missingSince(t *testing.T, dbp, sid string) (float64, bool) {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer con.Close()
	var ms sql.NullFloat64
	if err := con.QueryRow("SELECT missing_since FROM sessions WHERE id=?", sid).Scan(&ms); err != nil {
		t.Fatalf("read missing_since for %q: %v", sid, err)
	}
	return ms.Float64, ms.Valid
}

func present(ids map[string]struct{}, id string) bool { _, ok := ids[id]; return ok }

// TestImportClaudeWeb_Idempotent: re-importing the identical export changes
// nothing — no duplicate messages, all conversations reported skipped.
func TestImportClaudeWeb_Idempotent(t *testing.T) {
	dbp := cwDB(t)
	exp := map[string][]model.Message{"c1": {msg("m1", "hello", 1), msg("m2", "world", 2)}}

	s1 := cwImport(t, dbp, exp, 100)
	if s1.AddedConversations != 1 || s1.AddedMessages != 2 {
		t.Fatalf("first import stats = %+v, want +1 conv / +2 msgs", s1)
	}

	s2 := cwImport(t, dbp, exp, 100)
	if s2.AddedConversations != 0 || s2.UpdatedConversations != 0 || s2.SkippedConversations != 1 || s2.AddedMessages != 0 {
		t.Errorf("re-import stats = %+v, want 0 added / 0 updated / 1 skipped / 0 msgs", s2)
	}
	if n := messageCount(t, dbp, "c1"); n != 2 {
		t.Errorf("c1 message count = %d after re-import, want 2 (no duplication)", n)
	}
}

// TestImportClaudeWeb_SupersetNoLossUnderMirror is the F2 regression: a second
// import of a SUPERSET export (all of export1's conversations plus a new one),
// under RAWCLAW_RETENTION=mirror, must keep EVERY prior conversation. The old
// shared-Path file_index watermark pruned an arbitrary prior conversation here;
// per-conversation reconciliation loses none.
func TestImportClaudeWeb_SupersetNoLossUnderMirror(t *testing.T) {
	dbp := cwDB(t)
	t.Setenv("RAWCLAW_RETENTION", "mirror")

	cwImport(t, dbp, map[string][]model.Message{
		"c1": {msg("m1", "auth decision", 1)},
		"c2": {msg("m2", "deploy plan", 2)},
	}, 100)

	s2 := cwImport(t, dbp, map[string][]model.Message{
		"c1": {msg("m1", "auth decision", 1)},
		"c2": {msg("m2", "deploy plan", 2)},
		"c3": {msg("m3", "new chat", 3)},
	}, 200)

	ids := sessionIDs(t, dbp)
	for _, id := range []string{"c1", "c2", "c3"} {
		if !present(ids, id) {
			t.Errorf("conversation %q missing after superset re-import under mirror (F2 data loss)", id)
		}
	}
	if s2.PrunedAbsent != 0 {
		t.Errorf("superset re-import pruned %d conversation(s); a superset omits nothing, so nothing must prune", s2.PrunedAbsent)
	}
	if s2.AddedConversations != 1 || s2.SkippedConversations != 2 {
		t.Errorf("superset stats = %+v, want +1 new (c3) / 2 skipped (c1,c2)", s2)
	}
}

// TestImportClaudeWeb_ContinuedConversationAppends: a continued conversation
// (same uuid, more messages) gains only the new messages — no duplication.
func TestImportClaudeWeb_ContinuedConversationAppends(t *testing.T) {
	dbp := cwDB(t)
	cwImport(t, dbp, map[string][]model.Message{"c1": {msg("m1", "start", 1)}}, 100)

	s2 := cwImport(t, dbp, map[string][]model.Message{
		"c1": {msg("m1", "start", 1), msg("m2", "continued", 2)},
	}, 200)

	if s2.UpdatedConversations != 1 || s2.AddedMessages != 1 {
		t.Errorf("continued import stats = %+v, want 1 updated / +1 msg", s2)
	}
	if n := messageCount(t, dbp, "c1"); n != 2 {
		t.Errorf("c1 message count = %d, want 2 (m1 kept + m2 appended, no dup)", n)
	}
}

// TestImportClaudeWeb_MissingDefaultRetains: default (keep) retains a
// conversation absent from a newer export and flags it missing_since — still
// present/searchable, not deleted.
func TestImportClaudeWeb_MissingDefaultRetains(t *testing.T) {
	dbp := cwDB(t) // no RAWCLAW_RETENTION -> keep
	cwImport(t, dbp, map[string][]model.Message{
		"c1": {msg("m1", "kept", 1)},
		"c2": {msg("m2", "will vanish from export", 2)},
	}, 100)

	s2 := cwImport(t, dbp, map[string][]model.Message{"c1": {msg("m1", "kept", 1)}}, 200)

	ids := sessionIDs(t, dbp)
	if !present(ids, "c2") {
		t.Fatal("c2 deleted under default keep; it must be retained")
	}
	if _, set := missingSince(t, dbp, "c2"); !set {
		t.Error("c2 not flagged missing_since after dropping out of the export")
	}
	if _, set := missingSince(t, dbp, "c1"); set {
		t.Error("c1 (still present) must not be flagged missing_since")
	}
	if s2.RetainedAbsent != 1 || s2.PrunedAbsent != 0 {
		t.Errorf("stats = %+v, want 1 retained / 0 pruned under keep", s2)
	}
}

// TestImportClaudeWeb_MissingMirrorPrunes: under mirror, a fresher export that
// drops a conversation prunes it.
func TestImportClaudeWeb_MissingMirrorPrunes(t *testing.T) {
	dbp := cwDB(t)
	t.Setenv("RAWCLAW_RETENTION", "mirror")
	cwImport(t, dbp, map[string][]model.Message{
		"c1": {msg("m1", "kept", 1)},
		"c2": {msg("m2", "dropped", 2)},
	}, 100)

	s2 := cwImport(t, dbp, map[string][]model.Message{"c1": {msg("m1", "kept", 1)}}, 200)

	ids := sessionIDs(t, dbp)
	if present(ids, "c2") {
		t.Error("c2 survived under mirror after dropping from a FRESHER export; it must be pruned")
	}
	if !present(ids, "c1") {
		t.Error("c1 must survive")
	}
	if s2.PrunedAbsent != 1 {
		t.Errorf("stats = %+v, want 1 pruned under mirror", s2)
	}
}

// TestImportClaudeWeb_StaleMirrorDoesNotPrune is the staleness-guard proof: a
// STALE re-export (newest updated_at older than what's already imported) under
// mirror must NOT prune the conversations it omits — a mis-clicked old zip is
// inert, never destructive.
func TestImportClaudeWeb_StaleMirrorDoesNotPrune(t *testing.T) {
	dbp := cwDB(t)
	t.Setenv("RAWCLAW_RETENTION", "mirror")

	// Fresh import establishes newest=200.
	cwImport(t, dbp, map[string][]model.Message{
		"c1": {msg("m1", "kept", 1)},
		"c2": {msg("m2", "fresher data", 2)},
	}, 200)

	// Stale re-import (newest=100 < 200) omits c2.
	s2 := cwImport(t, dbp, map[string][]model.Message{"c1": {msg("m1", "kept", 1)}}, 100)

	ids := sessionIDs(t, dbp)
	if !present(ids, "c2") {
		t.Error("c2 pruned by a STALE export under mirror; the staleness guard must keep it")
	}
	if s2.PrunedAbsent != 0 {
		t.Errorf("stale import pruned %d; a stale export must prune nothing", s2.PrunedAbsent)
	}
	if s2.RetainedAbsent != 1 {
		t.Errorf("stats = %+v, want c2 retained (guard downgraded the mirror prune)", s2)
	}
}

// TestImportClaudeWeb_BornRetained: an imported conversation is never flagged
// missing_since just for lacking a live source file — it is flagged only by a
// later export that omits it. A single import (and an identical re-import) keeps
// it clear. (The search-time orphan reaper never scans the claude-web db — that
// exclusion is guarded in scopes; here we pin the import-side half.)
func TestImportClaudeWeb_BornRetained(t *testing.T) {
	dbp := cwDB(t)
	exp := map[string][]model.Message{"c1": {msg("m1", "born retained", 1)}}
	cwImport(t, dbp, exp, 100)
	if _, set := missingSince(t, dbp, "c1"); set {
		t.Error("freshly imported c1 flagged missing_since; imports are born retained")
	}
	cwImport(t, dbp, exp, 100) // present again -> stays clear
	if _, set := missingSince(t, dbp, "c1"); set {
		t.Error("c1 flagged missing_since after a present re-import; must stay clear")
	}
}

// TestImportClaudeWeb_DifferentAccountNeverTouched is the cross-account
// regression: importing account B must never flag or prune account A's
// conversations — a different account is a separate import, not an "absent"
// conversation. Under BOTH keep and mirror, A's rows are untouched.
func TestImportClaudeWeb_DifferentAccountNeverTouched(t *testing.T) {
	t.Run("keep", func(t *testing.T) {
		dbp := cwDB(t) // default keep
		cwImportAcct(t, dbp, "acctA", map[string][]model.Message{"a1": {msg("m1", "account A chat", 1)}}, 100)
		cwImportAcct(t, dbp, "acctB", map[string][]model.Message{"b1": {msg("m2", "account B chat", 2)}}, 200)

		ids := sessionIDs(t, dbp)
		if !present(ids, "a1") || !present(ids, "b1") {
			t.Fatalf("both accounts' conversations must survive: %v", ids)
		}
		if _, set := missingSince(t, dbp, "a1"); set {
			t.Error("account A's a1 was flagged missing_since by account B's import (cross-account leak)")
		}
	})

	t.Run("mirror", func(t *testing.T) {
		dbp := cwDB(t)
		t.Setenv("RAWCLAW_RETENTION", "mirror")
		// A imported first; B is FRESHER (newest 200 > A's 100) — the exact
		// condition that, unscoped, would prune A under mirror.
		cwImportAcct(t, dbp, "acctA", map[string][]model.Message{"a1": {msg("m1", "account A chat", 1)}}, 100)
		cwImportAcct(t, dbp, "acctB", map[string][]model.Message{"b1": {msg("m2", "account B chat", 2)}}, 200)

		ids := sessionIDs(t, dbp)
		if !present(ids, "a1") {
			t.Error("account A's a1 was PRUNED by account B's fresher import under mirror (cross-account data loss — F2 class)")
		}
		if !present(ids, "b1") {
			t.Error("account B's b1 must be present")
		}
	})
}

// TestImportClaudeWeb_TombstonedStaysDeleted: a conversation the user deleted
// (`rawclaw delete`) is not resurrected by a later export that still contains
// it.
func TestImportClaudeWeb_TombstonedStaysDeleted(t *testing.T) {
	dbp := cwDB(t)
	cwImport(t, dbp, map[string][]model.Message{
		"c1": {msg("m1", "keep", 1)},
		"c2": {msg("m2", "delete me", 2)},
	}, 100)

	// Tombstone c2 (the shape `rawclaw delete` writes into the cache sidecar).
	if err := os.WriteFile(filepath.Join(store.CacheDir(), ".deleted"), []byte("c2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-import STILL containing c2.
	cwImport(t, dbp, map[string][]model.Message{
		"c1": {msg("m1", "keep", 1)},
		"c2": {msg("m2", "delete me", 2)},
	}, 200)

	if ids := sessionIDs(t, dbp); present(ids, "c2") {
		t.Error("tombstoned c2 was resurrected by a later export; a user delete must stick")
	}
}
