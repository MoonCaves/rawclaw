package index

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/retention"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// vaultOf returns the vaulted session with this id, failing the test if the
// vault does not hold it.
func vaultOf(t *testing.T, id string) durable.Session {
	t.Helper()
	list, err := durable.List()
	if err != nil {
		t.Fatalf("durable.List: %v", err)
	}
	for _, v := range list {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("session %q not in the vault; have %+v", id, list)
	return durable.Session{}
}

// TestIndexVaultsTranscriptVerbatim: indexing is what fills the vault, and the
// copy has to be the source's own bytes — the rebuild re-parses it.
func TestIndexVaultsTranscriptVerbatim(t *testing.T) {
	isolateCache(t)
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f, `{"type":"user","uuid":"u1","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"reconcile the ledger"}]}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	v := vaultOf(t, "s")
	want, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(v.Transcript)
	if err != nil {
		t.Fatalf("read vaulted transcript: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("vaulted bytes differ from the source transcript:\n got: %q\nwant: %q", got, want)
	}
	if v.SourcePath == "" {
		t.Error("sidecar records no source path — a rebuild could not re-key file_index")
	}
	if v.SourceFP == "" {
		t.Error("sidecar records no source fingerprint — a rebuild would re-index a file that never changed")
	}
}

// TestReplicaScopeIsNotVaulted: a replica read out of an archive clone belongs
// to the machine that wrote it. Vaulting one locally would resurrect it here
// after its owner deletes it — the exact propagation the replica rules honor.
func TestReplicaScopeIsNotVaulted(t *testing.T) {
	isolateCache(t)
	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "foreign.jsonl"),
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"from another machine"}}`)

	con, _ := openTestDB(t)
	if err := updateIndexWithOrigin(con, proj, "other-machine"); err != nil {
		t.Fatalf("updateIndexWithOrigin: %v", err)
	}
	list, err := durable.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("replica session was vaulted: %+v", list)
	}
}

// TestVaultFlagFollowsRetention: the retention verdict has to reach the vault,
// because after a rebuild the sidecar is the only place it survives.
func TestVaultFlagFollowsRetention(t *testing.T) {
	isolateCache(t)
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	line := `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`
	writeJSONL(t, f, line)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	if v := vaultOf(t, "s"); v.OnlyCopySince != 0 {
		t.Fatalf("freshly indexed session already flagged: %v", v.OnlyCopySince)
	}

	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	if v := vaultOf(t, "s"); v.OnlyCopySince == 0 {
		t.Error("source purged but the vault sidecar is not flagged only copy")
	}

	// The source comes back: the flag has to clear, or the rebuild would keep
	// labelling a live session as retained-but-gone.
	writeJSONL(t, f, line)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	if v := vaultOf(t, "s"); v.OnlyCopySince != 0 {
		t.Errorf("source reappeared but the vault sidecar is still flagged: %v", v.OnlyCopySince)
	}
}

// TestApplyRetentionToVaultMirrorsEveryVerdict exercises the mirror directly,
// because two of its three branches cannot be provoked through an indexing pass:
// the vault write-through already leaves a correct sidecar behind, so the mirror
// is the second line of defense for the pass where that write failed. The
// origin gate is checked here too — a replica verdict must not reach the vault's
// delete path, where a colliding id would destroy an own session's only copy.
func TestApplyRetentionToVaultMirrorsEveryVerdict(t *testing.T) {
	isolateCache(t)
	for _, id := range []string{"stamp", "clear", "prune", "replica"} {
		if err := durable.StoreMessages(durable.Meta{ID: id, Source: "claude"},
			[]model.Message{{Role: "user", Text: "x", TSISO: "2026-06-01T10:00:00Z"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := durable.SetOnlyCopySince("clear", 1780000000); err != nil {
		t.Fatal(err)
	}

	res := retention.Result{Stamped: []string{"stamp"}, Cleared: []string{"clear"}, Pruned: []string{"prune"}}
	// A replica verdict is dropped wholesale, whichever branch names it.
	applyRetentionToVault(retention.Result{
		Stamped: []string{"replica"}, Cleared: []string{"clear"}, Pruned: []string{"replica"},
	}, 1790000000, "other-machine")
	applyRetentionToVault(res, 1790000000, "")

	got := map[string]durable.Session{}
	list, err := durable.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range list {
		got[v.ID] = v
	}
	if v, ok := got["stamp"]; !ok || v.OnlyCopySince != 1790000000 {
		t.Errorf("stamped session sidecar = %+v, want only_copy_since 1790000000", v)
	}
	if v, ok := got["clear"]; !ok || v.OnlyCopySince != 0 {
		t.Errorf("cleared session sidecar = %+v, want only_copy_since 0", v)
	}
	if _, ok := got["prune"]; ok {
		t.Error("pruned session still has a durable copy — a rebuild would resurrect it")
	}
	if v, ok := got["replica"]; !ok || v.OnlyCopySince != 0 {
		t.Errorf("a replica verdict reached the vault: %+v (present=%v)", v, ok)
	}
}

// TestRebuildReplacesTheOldStore: the rebuilt store is a pure function of the
// vault. A row left over from the store being replaced would otherwise survive
// as a session no transcript backs.
func TestRebuildReplacesTheOldStore(t *testing.T) {
	isolateCache(t)
	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "kept.jsonl"), `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"vaulted"}}`)
	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	// A session that exists ONLY in the store — never vaulted, so the rebuild
	// has no reason to keep it.
	if _, err := con.Exec("INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent) VALUES('stale',1,1,0,0)"); err != nil {
		t.Fatal(err)
	}
	con.Close()

	// This rebuild deliberately shrinks the store, which is exactly what the
	// lose-history guard refuses by default. Opt in, the way a user recovering
	// from a store they know is junk would.
	t.Setenv("RAWCLAW_REBUILD_FORCE", "1")
	if _, err := RebuildFromTranscripts(dbp); err != nil {
		t.Fatalf("RebuildFromTranscripts: %v", err)
	}
	rcon, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rcon.Close() })
	if got := scalar(t, rcon, "SELECT COUNT(*) FROM sessions WHERE id='stale'"); got != "0" {
		t.Errorf("a session with no transcript survived the rebuild (count=%s)", got)
	}
	if got := scalar(t, rcon, "SELECT COUNT(*) FROM sessions WHERE id='kept'"); got != "1" {
		t.Errorf("the vaulted session is missing from the rebuilt store (count=%s)", got)
	}
}

// TestTombstonedSessionLosesItsVaultCopy: a user delete has to really delete.
// Leaving the raw copy behind would let the next rebuild bring it back.
func TestTombstonedSessionLosesItsVaultCopy(t *testing.T) {
	isolateCache(t)
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"delete me"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	vaultOf(t, "s") // present before the delete

	// A user delete: the file goes, the id is tombstoned.
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.TombstoneIDs("", []string{"s"}); err != nil {
		t.Fatalf("TombstoneIDs: %v", err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}

	list, err := durable.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("deleted session still in the vault: %+v", list)
	}
}

// TestDeleteTheStoreAndRebuild_RestoresPurgedSession is the ticket's guarantee,
// end to end and on real files: index two sessions, let the source tool purge
// one, then delete every database rawclaw has and rebuild from the transcripts
// alone. Both sessions come back — including the one whose original file no
// longer exists anywhere on disk — and the purged one is still labelled as
// retained-but-gone.
//
// No archive is configured anywhere in this test. Durability must not depend on
// one.
func TestDeleteTheStoreAndRebuild_RestoresPurgedSession(t *testing.T) {
	isolateCache(t)
	proj := filepath.Join(t.TempDir(), "ledger")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(proj, "live.jsonl")
	purged := filepath.Join(proj, "purged.jsonl")
	writeJSONL(t, live,
		`{"type":"user","uuid":"u1","timestamp":"2026-06-01T10:00:00Z","cwd":"/w/ledger","message":{"role":"user","content":[{"type":"text","text":"reconcile the ledger"}]}}`)
	writeJSONL(t, purged,
		`{"type":"user","uuid":"u2","timestamp":"2026-05-01T09:00:00Z","cwd":"/w/ledger","message":{"role":"user","content":[{"type":"text","text":"the billing export vanished"}]}}`,
		`{"type":"assistant","uuid":"u3","timestamp":"2026-05-01T09:00:30Z","message":{"role":"assistant","content":[{"type":"text","text":"restored from the backup"}]}}`)

	dbp, _, _, err := EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("first index: %v", err)
	}
	// The source tool purges one transcript, the way a real cleanup does.
	if err := os.Remove(purged); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := EnsureIndexed(proj, false); err != nil {
		t.Fatalf("index after purge: %v", err)
	}

	// Pre-conditions, read off the store that owns the retention verdict.
	before, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, before, "SELECT COUNT(*) FROM sessions"); got != "2" {
		t.Fatalf("indexed sessions before the wipe = %s, want 2", got)
	}
	if got := scalar(t, before, "SELECT only_copy_since IS NOT NULL FROM sessions WHERE id='purged'"); got != "1" {
		t.Fatalf("purged session not flagged before the wipe (got %s)", got)
	}
	before.Close()
	if got := scalar(t, openConsolidated(t), "SELECT COUNT(*) FROM sessions"); got != "2" {
		t.Fatalf("consolidated sessions before the wipe = %s, want 2", got)
	}

	// Delete every database rawclaw owns. This is the disaster the vault exists
	// for: the cache dir holds the consolidated store, every per-project db, and
	// the tombstone sidecar.
	if err := os.RemoveAll(store.CacheDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(purged); !os.IsNotExist(err) {
		t.Fatalf("the purged transcript is still on disk (err=%v) — the test would prove nothing", err)
	}

	st, err := RebuildFromTranscripts(ConsolidatedPath())
	if err != nil {
		t.Fatalf("RebuildFromTranscripts: %v", err)
	}
	if st.Sessions != 2 {
		t.Errorf("rebuilt %d sessions, want 2 (%+v)", st.Sessions, st)
	}
	if st.Messages != 3 {
		t.Errorf("rebuilt %d messages, want 3 (%+v)", st.Messages, st)
	}
	if st.Missing != 1 {
		t.Errorf("rebuild reports %d retained-but-purged sessions, want 1 (%+v)", st.Missing, st)
	}
	if st.Unreadable != 0 {
		t.Errorf("rebuild could not read %d transcripts (%+v)", st.Unreadable, st)
	}

	after := openConsolidated(t)
	if got := scalar(t, after, "SELECT COUNT(*) FROM sessions"); got != "2" {
		t.Fatalf("sessions after the rebuild = %s, want 2", got)
	}
	// The purged session's CONTENT is back, not just its row.
	if got := scalar(t, after, "SELECT content FROM messages WHERE session_id='purged' AND uuid='u2'"); got != "the billing export vanished" {
		t.Errorf("purged session's message = %q, want it restored verbatim", got)
	}
	if got := scalar(t, after, "SELECT COUNT(*) FROM messages WHERE session_id='purged'"); got != "2" {
		t.Errorf("purged session has %s messages after the rebuild, want 2", got)
	}
	// And it is still searchable, which is the whole reason to retain it.
	if got := scalar(t, after, "SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'billing'"); got != "1" {
		t.Errorf("full-text search finds %s hits for a restored session, want 1", got)
	}
	// The watermark survived the round trip through the transcripts.
	if got := scalar(t, after, "SELECT only_copy_since IS NOT NULL FROM sessions WHERE id='purged'"); got != "1" {
		t.Errorf("purged session lost its only_copy_since label in the rebuild (got %s)", got)
	}
	if got := scalar(t, after, "SELECT only_copy_since FROM sessions WHERE id='live'"); got != "<NULL>" {
		t.Errorf("live session came back flagged as only copy (only_copy_since=%s)", got)
	}
	// Scope came back too, so the rebuilt store is still filterable by project.
	if got := scalar(t, after, "SELECT project FROM sessions WHERE id='purged'"); got != "ledger" {
		t.Errorf("project = %q after the rebuild, want %q", got, "ledger")
	}
	// file_index is keyed on the ORIGINAL source path, never the vault path.
	if got := scalar(t, after, "SELECT path FROM file_index WHERE session_id='live'"); got != realpath(live) {
		t.Errorf("file_index path = %q, want the original source path %q", got, realpath(live))
	}
}

// TestRebuiltStoreSurvivesTheNextLivePass is the other half of the guarantee: a
// rebuilt store has to behave like one that was never lost. The retention pass
// reconciles file_index paths against the live walk, so a store whose watermarks
// pointed at the vault would stamp every live session as only copy on the very
// next search.
func TestRebuiltStoreSurvivesTheNextLivePass(t *testing.T) {
	isolateCache(t)
	proj := t.TempDir()
	live := filepath.Join(proj, "live.jsonl")
	purged := filepath.Join(proj, "purged.jsonl")
	writeJSONL(t, live, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"still here"}}`)
	writeJSONL(t, purged, `{"type":"user","timestamp":"2026-05-01T09:00:00Z","message":{"role":"user","content":"gone upstream"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(purged); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}

	rebuilt := filepath.Join(t.TempDir(), "rebuilt.db")
	if _, err := RebuildFromTranscripts(rebuilt); err != nil {
		t.Fatalf("RebuildFromTranscripts: %v", err)
	}
	rcon, err := store.ConnectRW(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rcon.Close() })

	// A normal indexing pass over the still-live project dir.
	if err := UpdateIndex(rcon, proj); err != nil {
		t.Fatalf("UpdateIndex on the rebuilt store: %v", err)
	}
	if got := scalar(t, rcon, "SELECT only_copy_since FROM sessions WHERE id='live'"); got != "<NULL>" {
		t.Errorf("a live session was flagged only copy after a pass over the rebuilt store (only_copy_since=%s)", got)
	}
	if got := scalar(t, rcon, "SELECT only_copy_since IS NOT NULL FROM sessions WHERE id='purged'"); got != "1" {
		t.Errorf("the purged session lost its flag on the next live pass (got %s)", got)
	}
}

// TestRebuildHonorsTombstone: the tombstone sidecar outlives the store it was
// applied to, so a rebuild must not undo a user's delete.
func TestRebuildHonorsTombstone(t *testing.T) {
	isolateCache(t)
	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "keep.jsonl"), `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"keep"}}`)
	writeJSONL(t, filepath.Join(proj, "drop.jsonl"), `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"drop"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	// Tombstone WITHOUT going through the index pass that would also evict the
	// vault copy — the case where a stale raw copy could resurrect a session.
	if err := lifecycle.TombstoneIDs("", []string{"drop"}); err != nil {
		t.Fatalf("TombstoneIDs: %v", err)
	}

	rebuilt := filepath.Join(t.TempDir(), "rebuilt.db")
	st, err := RebuildFromTranscripts(rebuilt)
	if err != nil {
		t.Fatalf("RebuildFromTranscripts: %v", err)
	}
	if st.Tombstoned != 1 {
		t.Errorf("rebuild skipped %d tombstoned sessions, want 1 (%+v)", st.Tombstoned, st)
	}
	rcon, err := store.ConnectRO(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rcon.Close() })
	if got := scalar(t, rcon, "SELECT COUNT(*) FROM sessions WHERE id='drop'"); got != "0" {
		t.Errorf("a deleted session came back in the rebuild (count=%s)", got)
	}
	if got := scalar(t, rcon, "SELECT COUNT(*) FROM sessions WHERE id='keep'"); got != "1" {
		t.Errorf("the kept session did not come back (count=%s)", got)
	}
}

// TestRebuildRestoresContainerSession covers the non-Claude producer end to end:
// a source that hands rawclaw flattened messages instead of a Claude-shaped file
// still round-trips through the vault.
func TestRebuildRestoresContainerSession(t *testing.T) {
	isolateCache(t)
	dir := t.TempDir()
	roll := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(roll, []byte("not claude shape at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cs := []source.Container{{ID: "c1", Path: roll, CWD: "/w/billing"}}
	msgs := func(source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "where did the billing export go", TS: 1780000000, TSISO: "2026-06-01T10:00:00Z", UUID: "m1"},
			{Role: "assistant", Text: "it is in the ledger folder", TS: 1780000060, TSISO: "2026-06-01T10:01:00Z", UUID: "m2"},
		}, nil
	}
	dbp := filepath.Join(t.TempDir(), "codexish.db")
	if _, _, err := EnsureIndexedContainers(dbp, false, cs, msgs, "codexish", ""); err != nil {
		t.Fatalf("EnsureIndexedContainers: %v", err)
	}

	// The rollout itself is purged, exactly like a real cleanup.
	if err := os.Remove(roll); err != nil {
		t.Fatal(err)
	}
	rebuilt := filepath.Join(t.TempDir(), "rebuilt.db")
	if _, err := RebuildFromTranscripts(rebuilt); err != nil {
		t.Fatalf("RebuildFromTranscripts: %v", err)
	}
	rcon, err := store.ConnectRO(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rcon.Close() })

	if got := scalar(t, rcon, "SELECT COUNT(*) FROM messages WHERE session_id='c1'"); got != "2" {
		t.Fatalf("container session has %s messages after the rebuild, want 2", got)
	}
	if got := scalar(t, rcon, "SELECT content FROM messages WHERE uuid='m2'"); got != "it is in the ledger folder" {
		t.Errorf("message text = %q, want it restored verbatim", got)
	}
	if got := scalar(t, rcon, "SELECT role FROM messages WHERE uuid='m2'"); got != "assistant" {
		t.Errorf("role = %q, want assistant", got)
	}
	if got := scalar(t, rcon, "SELECT source_tool FROM sessions WHERE id='c1'"); got != "codexish" {
		t.Errorf("source_tool = %q, want the original source id", got)
	}
	if got := scalar(t, rcon, "SELECT cwd FROM sessions WHERE id='c1'"); got != "/w/billing" {
		t.Errorf("cwd = %q, want /w/billing", got)
	}
}

// TestRebuildRefusesToLoseHistory is the guard for the upgrade case: a machine
// with a large existing store and a vault that has not filled yet. The vault
// only gains a session when that session is indexed, so running the recovery
// path too early would delete real history and restore almost none of it.
func TestRebuildRefusesToLoseHistory(t *testing.T) {
	isolateCache(t)
	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "kept.jsonl"), `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"vaulted"}}`)
	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatal(err)
	}
	// Two sessions the vault never saw — the shape of a store that predates it.
	for _, id := range []string{"older-a", "older-b"} {
		if _, err := con.Exec("INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent) VALUES(?,1,1,0,0)", id); err != nil {
			t.Fatal(err)
		}
	}
	con.Close()

	_, err := RebuildFromTranscripts(dbp)
	if !errors.Is(err, ErrRebuildWouldLoseHistory) {
		t.Fatalf("rebuild should have refused to shrink the store; err = %v", err)
	}

	// The refusal has to leave the store alone: a guard that still deleted the
	// db would be worse than no guard.
	rcon, cerr := store.ConnectRO(dbp)
	if cerr != nil {
		t.Fatalf("store gone after a refused rebuild: %v", cerr)
	}
	t.Cleanup(func() { rcon.Close() })
	if got := scalar(t, rcon, "SELECT COUNT(*) FROM sessions"); got != "3" {
		t.Errorf("refused rebuild changed the store (sessions=%s, want 3)", got)
	}

	// And the override still works, because a user whose store is junk has to
	// be able to say so.
	t.Setenv("RAWCLAW_REBUILD_FORCE", "1")
	if _, ferr := RebuildFromTranscripts(dbp); ferr != nil {
		t.Fatalf("forced rebuild: %v", ferr)
	}
}
