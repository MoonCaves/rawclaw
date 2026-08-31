package index

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestConsolidate_LogsPhaseStartsAndDurations(t *testing.T) {
	isolateCache(t)
	src := seedSessionDB(t, "phase-logs.db", sessionRow{
		id: "phase-log-session", project: "phase-logs", cwd: "/w/phase-logs",
		msgs: []msgRow{{"phase-log-message", "user", "log this fold", 100}},
	})
	recorder := &testLogRecorder{}
	original := slog.Default()
	slog.SetDefault(slog.New(recorder))
	defer slog.SetDefault(original)

	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	recorder.mu.Lock()
	records := append([]slog.Record(nil), recorder.records...)
	recorder.mu.Unlock()

	type phaseKey struct {
		message string
		phase   string
	}
	starts := make(map[phaseKey]bool)
	durations := make(map[phaseKey]bool)
	for _, rec := range records {
		var phase, event string
		var duration slog.Value
		rec.Attrs(func(attr slog.Attr) bool {
			switch attr.Key {
			case "phase":
				phase = attr.Value.String()
			case "event":
				event = attr.Value.String()
			case "duration":
				duration = attr.Value
			}
			return true
		})
		key := phaseKey{message: rec.Message, phase: phase}
		starts[key] = starts[key] || event == "start"
		durations[key] = durations[key] || duration.Kind() == slog.KindDuration
	}

	assertLogged := func(message, phase string) {
		t.Helper()
		key := phaseKey{message: message, phase: phase}
		if !starts[key] {
			t.Errorf("%s phase %q has no start log", message, phase)
		}
		if !durations[key] {
			t.Errorf("%s phase %q has no duration log", message, phase)
		}
	}
	for _, phase := range []string{
		"schema-migrate", "source-migrate", "attach", "prepare", "merge",
		"detach", "tombstone-prune", "watermark-stamp", "connection-close",
	} {
		assertLogged("consolidate fold phase", phase)
	}
	for _, phase := range []string{"acquire", "release"} {
		assertLogged("consolidated fence phase", phase)
	}
}

// TestConsolidate_UnionsEveryProject is the ticket's headline: sessions from
// separate per-project dbs land in ONE store, each keeping the project it came
// from as a column, and one FTS index spans them all.
func TestConsolidate_UnionsEveryProject(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)
	b := indexProject(t, "-w-billing",
		`{"type":"user","cwd":"/w/billing","timestamp":"2026-06-02T10:00:00Z","uuid":"u-b1","message":{"role":"user","content":"invoice retries keep firing"}}`)

	st, err := ConsolidateFrom([]string{a, b}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}
	if st.Sessions != 2 || st.Messages != 2 {
		t.Fatalf("consolidated %d sessions / %d messages, want 2 / 2", st.Sessions, st.Messages)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(DISTINCT project) FROM sessions"); got != "2" {
		t.Errorf("distinct projects = %s, want 2 — the scope did not ride the row", got)
	}
	// One index over both projects: a term appearing in each returns both, which
	// is the thing 112 separate indexes could not do with comparable scores.
	if got := scalar(t, con,
		"SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'invoice'"); got != "2" {
		t.Errorf("FTS matched %s rows for a term in both projects, want 2", got)
	}
}

// TestConsolidate_SameSessionIDMergesToOneRow is the maintainer's ruling made structural:
// one session resumed in a second directory is ONE session. The stale copy here
// was purged upstream and holds an older, shorter prefix; the live copy carries
// the continuation. The merged row must be present, span both, and hold the
// union of the messages with no duplicate.
func TestConsolidate_SameSessionIDMergesToOneRow(t *testing.T) {
	isolateCache(t)
	const id = "resumed"

	// Two per-project dbs, hand-built so the same session id appears in both with
	// the exact asymmetry the real corpus has (one retained-but-purged, one live).
	stale := seedSessionDB(t, "stale.db", sessionRow{
		id: id, project: "ledger", cwd: "/w/ledger", missing: 1784381448.08399,
		msgs: []msgRow{{"u-1", "user", "first turn", 100}, {"u-2", "assistant", "second turn", 200}},
	})
	live := seedSessionDB(t, "live.db", sessionRow{
		id: id, project: "billing", cwd: "/w/billing", missing: 0,
		msgs: []msgRow{{"u-1", "user", "first turn", 100}, {"u-2", "assistant", "second turn", 200}, {"u-3", "user", "resumed turn", 300}},
	})

	st, err := ConsolidateFrom([]string{stale, live}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}
	if st.SessionsSeen != 2 {
		t.Fatalf("sources offered %d session rows, want 2", st.SessionsSeen)
	}
	if st.Sessions != 1 {
		t.Errorf("consolidated %d sessions, want 1 — the same id must not be two sessions", st.Sessions)
	}
	if st.Messages != 3 {
		t.Errorf("consolidated %d messages, want 3 — the union, with the shared prefix counted once", st.Messages)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COALESCE(only_copy_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Errorf("only_copy_since = %s, want NULL — a session present in ANY copy is present", got)
	}
	if got := scalar(t, con, "SELECT project FROM sessions WHERE id=?", id); got != "billing" {
		t.Errorf("project = %q, want %q — the live continuation's scope wins over the purged copy's", got, "billing")
	}
	if got := scalar(t, con, "SELECT message_count FROM sessions WHERE id=?", id); got != "3" {
		t.Errorf("message_count = %s, want 3 — the count must be restated from the merged rows", got)
	}
	if got := scalar(t, con, "SELECT last_ts FROM sessions WHERE id=?", id); got != "300.0" && got != "300" {
		t.Errorf("last_ts = %s, want 300 — the merged span must reach the continuation", got)
	}
}

// TestConsolidate_MergeIsOrderIndependent runs the same two sources in the
// opposite order. The merge rules are written to be commutative, so a re-run
// that happens to enumerate the cache dir differently must not flip which
// copy's scope survives.
func TestConsolidate_MergeIsOrderIndependent(t *testing.T) {
	isolateCache(t)
	const id = "resumed"
	stale := seedSessionDB(t, "stale.db", sessionRow{
		id: id, project: "ledger", cwd: "/w/ledger", missing: 1784381448.08399,
		msgs: []msgRow{{"u-1", "user", "first turn", 100}},
	})
	live := seedSessionDB(t, "live.db", sessionRow{
		id: id, project: "billing", cwd: "/w/billing", missing: 0,
		msgs: []msgRow{{"u-1", "user", "first turn", 100}, {"u-2", "user", "resumed turn", 300}},
	})

	// live FIRST this time — the stale copy arrives second and must not win.
	if _, err := ConsolidateFrom([]string{live, stale}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}
	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT project FROM sessions WHERE id=?", id); got != "billing" {
		t.Errorf("project = %q, want %q — arrival order decided the winner", got, "billing")
	}
	if got := scalar(t, con, "SELECT COALESCE(only_copy_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Errorf("only_copy_since = %s, want NULL — a later stale copy re-flagged a present session", got)
	}
}

// TestConsolidate_CollapsesDuplicateUUIDsWithinASource covers what the real
// per-project dbs actually hold: byte-identical repeats of one message uuid,
// written when a session was replayed into several transcript files. They are
// one message, and the consolidated store must hold one row for them.
func TestConsolidate_CollapsesDuplicateUUIDsWithinASource(t *testing.T) {
	isolateCache(t)
	src := seedSessionDB(t, "dupes.db", sessionRow{
		id: "s", project: "ledger", cwd: "/w/ledger",
		msgs: []msgRow{
			{"u-1", "assistant", "the audit came back clean", 100},
			{"u-1", "assistant", "the audit came back clean", 100},
			{"u-1", "assistant", "the audit came back clean", 100},
			{"u-2", "user", "good", 200},
		},
	})

	st, err := ConsolidateFrom([]string{src}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}
	if st.Messages != 2 {
		t.Errorf("consolidated %d messages, want 2 — three copies of one uuid are one message", st.Messages)
	}
	con := openConsolidated(t)
	// The FTS index must collapse with the table: a duplicated row left in the
	// index would return the same message three times for one query.
	if got := scalar(t, con, "SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'audit'"); got != "1" {
		t.Errorf("FTS matched %s rows, want 1 — duplicates leaked into the search index", got)
	}
}

// TestConsolidate_IsIdempotent proves a second pass over unchanged sources adds
// nothing. This is what makes the write-through safe to run on every indexing
// invocation.
func TestConsolidate_IsIdempotent(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"one"}}`)
	b := indexProject(t, "-w-billing",
		`{"type":"user","cwd":"/w/billing","timestamp":"2026-06-02T10:00:00Z","uuid":"u-b1","message":{"role":"user","content":"two"}}`)

	first, err := ConsolidateFrom([]string{a, b}, false)
	if err != nil {
		t.Fatalf("pass 1: %v", err)
	}
	second, err := ConsolidateFrom([]string{a, b}, false)
	if err != nil {
		t.Fatalf("pass 2: %v", err)
	}
	if first.Sessions != second.Sessions || first.Messages != second.Messages {
		t.Errorf("pass 2 = %d sessions / %d messages, pass 1 = %d / %d — the merge is not idempotent",
			second.Sessions, second.Messages, first.Sessions, first.Messages)
	}
}

// TestConsolidate_HonorsTombstones guards the one absence that must propagate:
// the merge is additive, so without an explicit prune a user-deleted session
// would be resurrected out of any per-project db not yet reconciled.
func TestConsolidate_HonorsTombstones(t *testing.T) {
	isolateCache(t)
	src := seedSessionDB(t, "p.db",
		sessionRow{id: "keep", project: "ledger", cwd: "/w/ledger", msgs: []msgRow{{"u-1", "user", "keep me", 100}}},
		sessionRow{id: "drop", project: "ledger", cwd: "/w/ledger", msgs: []msgRow{{"u-2", "user", "delete me", 200}}},
		sessionRow{id: "drop/agent-1", project: "ledger", cwd: "/w/ledger", msgs: []msgRow{{"u-3", "assistant", "child of the deleted", 300}}},
	)
	// Tag the doomed session so the prune has real sidecar rows to take:
	// mergeTopicsSQL folds the segment in during the same pass that prunes.
	tagSession(t, src, "drop", "u-2", "secrets", "the deleted conversation's summary", 400)
	if err := lifecycle.TombstoneIDs("", []string{"drop"}); err != nil {
		t.Fatalf("write tombstone: %v", err)
	}

	st, err := ConsolidateFrom([]string{src}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}
	if st.Sessions != 1 {
		t.Errorf("consolidated %d sessions, want 1 — a deleted session came back", st.Sessions)
	}
	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions WHERE id LIKE 'drop%'"); got != "0" {
		t.Errorf("%s deleted rows survived, including the subagent threads beneath the session", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'delete'"); got != "0" {
		t.Error("a deleted session's text is still searchable in the consolidated index")
	}
	// The tagging sidecars summarize the conversation's content, so a user
	// delete must take them too — a surviving topic row IS surviving content.
	if got := scalar(t, con, "SELECT COUNT(*) FROM topic_segment WHERE session_id LIKE 'drop%'"); got != "0" {
		t.Errorf("%s topic segments of a deleted session survived the prune", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_verdict WHERE session_id LIKE 'drop%'"); got != "0" {
		t.Errorf("%s verdicts of a deleted session survived the prune", got)
	}
}

// TestConsolidate_PrunesTombstonesWhenSourceUnchanged verifies that tombstones
// are pruned even when no source database has changed since its last fold-in
// (tracker #217: pruning must be independent of source-change detection).
func TestConsolidate_PrunesTombstonesWhenSourceUnchanged(t *testing.T) {
	isolateCache(t)
	src := seedSessionDB(t, "p.db",
		sessionRow{id: "sess-1", project: "ledger", cwd: "/w/ledger", msgs: []msgRow{{"u-1", "user", "secret plans", 100}}},
	)

	// First pass: fold the source DB in
	st, err := ConsolidateFrom([]string{src}, false)
	if err != nil {
		t.Fatalf("first ConsolidateFrom: %v", err)
	}
	if st.Sessions != 1 {
		t.Fatalf("pass 1 consolidated %d sessions, want 1", st.Sessions)
	}

	// Add tombstone without modifying the source DB at all
	if err := lifecycle.TombstoneIDs("", []string{"sess-1"}); err != nil {
		t.Fatalf("write tombstone: %v", err)
	}

	// Second pass: re-run consolidation without --rebuild
	st2, err := ConsolidateFrom([]string{src}, false)
	if err != nil {
		t.Fatalf("second ConsolidateFrom: %v", err)
	}
	if st2.Sessions != 0 {
		t.Errorf("pass 2 consolidated %d sessions, want 0 after tombstone", st2.Sessions)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions WHERE id='sess-1'"); got != "0" {
		t.Errorf("%s deleted session rows survived in consolidated store", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'secret'"); got != "0" {
		t.Error("a deleted session's text is still searchable in messages_fts after no-op source pass")
	}
}

// TestConsolidate_MigratesAPreScopeSource is the case the whole real corpus was
// in: every cache db predated the scope migration, so none carried project/cwd.
// Consolidation has to migrate a source it can migrate — stepping over them
// instead means a full pass moves nothing while reporting success.
func TestConsolidate_MigratesAPreScopeSource(t *testing.T) {
	isolateCache(t)
	src := rewindScopeColumns(t, seedSessionDB(t, "old.db", sessionRow{
		id: "s", project: "ledger", cwd: "/w/ledger", msgs: []msgRow{{"u-1", "user", "older shape", 100}},
	}))

	st, err := ConsolidateFrom([]string{src}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}
	if st.Skipped != 0 {
		t.Errorf("skipped %d sources, want 0 — a current-version db is migrated, not skipped", st.Skipped)
	}
	if st.Sessions != 1 || st.Messages != 1 {
		t.Errorf("consolidated %d sessions / %d messages, want 1 / 1", st.Sessions, st.Messages)
	}
}

// TestConsolidate_SkipsAndCountsABehindVersionSource covers the source
// consolidation must NOT touch: one whose schema is behind. Rebuilding it here
// would drop the rows this pass came to read, so it is left for the next
// indexing run — and counted, because a skipped source's rows are absent from
// the store and a silent skip reads as success.
func TestConsolidate_SkipsAndCountsABehindVersionSource(t *testing.T) {
	isolateCache(t)
	old := rewindScopeColumns(t, seedSessionDB(t, "old.db", sessionRow{
		id: "s", project: "ledger", cwd: "/w/ledger", msgs: []msgRow{{"u-1", "user", "older shape", 100}},
	}))
	oldCon, err := store.ConnectRW(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oldCon.Exec("UPDATE meta SET value=? WHERE key='schema_version'", store.SchemaVersion-1); err != nil {
		t.Fatalf("rewind schema version: %v", err)
	}
	oldCon.Close()

	good := seedSessionDB(t, "good.db", sessionRow{
		id: "t", project: "billing", cwd: "/w/billing", msgs: []msgRow{{"u-2", "user", "current shape", 200}},
	})

	st, err := ConsolidateFrom([]string{old, good}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom: %v — a behind-version db must be skipped, not fatal", err)
	}
	if st.Sessions != 1 {
		t.Errorf("consolidated %d sessions, want 1 (only the current-version source)", st.Sessions)
	}
	if st.Skipped != 1 {
		t.Errorf("reported %d skipped sources, want 1 — a skip the caller cannot see is a silent one", st.Skipped)
	}
	// The behind-version source must come out of the pass intact: consolidation
	// declining to read it is not a licence to rebuild it out from under the
	// transcripts.
	var left int
	skipCon, err := store.ConnectRW(old)
	if err != nil {
		t.Fatal(err)
	}
	defer skipCon.Close()
	if err := skipCon.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Errorf("skipped source holds %d sessions, want 1 — its rows were destroyed, not stepped over", left)
	}
}

// TestPerProjectDBs_ExcludesTheConsolidatedStore is the loop guard: the
// consolidated store lives in the same cache dir as its sources, so anything
// enumerating that dir would otherwise feed the store back into itself and
// double every row.
func TestPerProjectDBs_ExcludesTheConsolidatedStore(t *testing.T) {
	isolateCache(t)
	src := seedSessionDB(t, "p.db", sessionRow{
		id: "s", project: "ledger", cwd: "/w/ledger", msgs: []msgRow{{"u-1", "user", "hi", 100}},
	})
	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatal(err)
	}
	// Put a copy of a source in the cache dir so the glob has real work to do.
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.CacheDir(), "-w-ledger.db"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	dbs, err := PerProjectDBs()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dbs {
		if IsConsolidatedDB(d) {
			t.Fatalf("PerProjectDBs returned the consolidated store itself (%s)", filepath.Base(d))
		}
	}
	if len(dbs) != 1 {
		t.Errorf("PerProjectDBs returned %d dbs, want 1", len(dbs))
	}
}

// TestConsolidate_FoldsTheTopicLayer is the precondition for topics reading the
// one store: tags live only in the per-project db that produced them, so
// without this fold a reader on the store reports an untagged corpus. The
// searchable half matters as much as the rows — topic_fts is an external-content
// index, so a fold that inserted rows without driving the triggers would leave
// the labels present but unfindable.
func TestConsolidate_FoldsTheTopicLayer(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)
	b := indexProject(t, "-w-billing",
		`{"type":"user","cwd":"/w/billing","timestamp":"2026-06-02T10:00:00Z","uuid":"u-b1","message":{"role":"user","content":"invoice retries keep firing"}}`)
	tagSession(t, a, firstSessionID(t, a), "u-a1", "invoice reconciliation", "totals did not line up", 100)
	tagSession(t, b, firstSessionID(t, b), "u-b1", "retry storm", "retries fired in a loop", 100)

	if _, err := ConsolidateFrom([]string{a, b}, false); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM topic_segment"); got != "2" {
		t.Fatalf("topic_segment holds %s rows, want both projects' tags", got)
	}
	if got := scalar(t, con,
		"SELECT COUNT(*) FROM topic_fts WHERE topic_fts MATCH 'retry'"); got != "1" {
		t.Errorf("topic_fts matched %s rows for a folded label, want 1 — the fold bypassed the triggers", got)
	}
}

func TestConsolidate_DeletesTopicsRemovedFromSource(t *testing.T) {
	isolateCache(t)
	src := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`,
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:01:00Z","uuid":"u-b1","message":{"role":"user","content":"then verify the payment"}}`)
	sid := firstSessionID(t, src)
	con, err := store.ConnectRW(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		con.Close()
		t.Fatal(err)
	}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{
		{SessionID: sid, StartUUID: "u-a1", EndUUID: "u-b1", Topic: "A", Summary: "first"},
		{SessionID: sid, StartUUID: "u-b1", Topic: "B", Summary: "second"},
	}); err != nil {
		con.Close()
		t.Fatal(err)
	}
	con.Close()
	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatalf("first consolidate: %v", err)
	}

	con, err = store.ConnectRW(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{
		{SessionID: sid, StartUUID: "u-a1", EndUUID: "u-b1", Topic: "A", Summary: "first"},
	}); err != nil {
		con.Close()
		t.Fatal(err)
	}
	con.Close()
	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatalf("second consolidate: %v", err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM topic_segment WHERE session_id=? AND start_uuid='u-b1'", sid); got != "0" {
		t.Fatalf("removed topic B remains in consolidated store: %s rows", got)
	}
}

func TestConsolidate_DeletesSidecarsWhenSourceRemovesWholeSession(t *testing.T) {
	isolateCache(t)
	src := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)
	sid := firstSessionID(t, src)
	con, err := store.ConnectRW(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		con.Close()
		t.Fatal(err)
	}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{{SessionID: sid, StartUUID: "u-a1", Topic: "invoice", Summary: "summary"}}); err != nil {
		con.Close()
		t.Fatal(err)
	}
	if err := store.UpsertVerdict(con, store.Verdict{SessionID: sid, Verdict: store.VerdictRoutine, Source: store.VerdictSourceFloor, TaggedAt: 1}); err != nil {
		con.Close()
		t.Fatal(err)
	}
	con.Close()
	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatalf("first consolidate: %v", err)
	}

	con, err = store.ConnectRW(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("DELETE FROM sessions WHERE id=?", sid); err != nil {
		con.Close()
		t.Fatal(err)
	}
	con.Close()
	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatalf("second consolidate: %v", err)
	}

	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions WHERE id=?", sid); got != "0" {
		t.Errorf("removed session remains in consolidated store: %s rows", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM topic_segment WHERE session_id=?", sid); got != "0" {
		t.Errorf("topic sidecar for removed session remains: %s rows", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_verdict WHERE session_id=?", sid); got != "0" {
		t.Errorf("verdict sidecar for removed session remains: %s rows", got)
	}
}

func TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor(t *testing.T) {
	isolateCache(t)
	shared := sessionRow{id: "shared-sidecar", project: "shared", cwd: "/shared", msgs: []msgRow{{"shared-u1", "user", "shared", 100}}}
	orphan := sessionRow{id: "orphan-sidecar", project: "orphan", cwd: "/orphan", msgs: []msgRow{{"orphan-u1", "user", "orphan", 100}}}
	a := seedSessionDB(t, "sidecar-a.db", shared, orphan)
	b := seedSessionDB(t, "sidecar-b.db", shared)
	if _, err := ConsolidateFrom([]string{a, b}, false); err != nil {
		t.Fatalf("first consolidate: %v", err)
	}

	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range [][2]string{{"shared-sidecar", "shared-u1"}, {"orphan-sidecar", "orphan-u1"}} {
		if _, err := con.Exec("INSERT INTO topic_segment(session_id, start_uuid, topic, summary) VALUES (?, ?, 'topic', 'summary')", row[0], row[1]); err != nil {
			con.Close()
			t.Fatalf("seed topic sidecar: %v", err)
		}
		if _, err := con.Exec("INSERT INTO session_verdict(session_id, verdict, source) VALUES (?, 'routine', 'floor')", row[0]); err != nil {
			con.Close()
			t.Fatalf("seed verdict sidecar: %v", err)
		}
	}
	if err := con.Close(); err != nil {
		t.Fatal(err)
	}

	con, err = store.ConnectRW(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("DELETE FROM sessions WHERE id IN (?, ?)", shared.id, orphan.id); err != nil {
		con.Close()
		t.Fatalf("remove source sessions: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ConsolidateFrom([]string{a, b}, false); err != nil {
		t.Fatalf("second consolidate: %v", err)
	}

	con = openConsolidated(t)
	for _, table := range []string{"topic_segment", "session_verdict"} {
		if got := scalar(t, con, "SELECT COUNT(*) FROM "+table+" WHERE session_id=?", orphan.id); got != "0" {
			t.Errorf("%s orphan rows = %s, want 0", table, got)
		}
		if got := scalar(t, con, "SELECT COUNT(*) FROM "+table+" WHERE session_id=?", shared.id); got != "1" {
			t.Errorf("%s co-contributor rows = %s, want 1", table, got)
		}
	}
}

func TestConsolidate_PreservesTopicsWhenCoContributorRemains(t *testing.T) {
	isolateCache(t)
	sid := "shared-topic-session"
	a := seedSessionDB(t, "a.db", sessionRow{id: sid, project: "a", cwd: "/a", msgs: []msgRow{{"u-a", "user", "a", 100}}})
	b := seedSessionDB(t, "b.db", sessionRow{id: sid, project: "b", cwd: "/b", msgs: []msgRow{{"u-b", "user", "b", 100}}})
	for _, dbp := range []string{a, b} {
		con, err := store.ConnectRW(dbp)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.EnsureTopicSchema(con); err != nil {
			con.Close()
			t.Fatal(err)
		}
		segs := []store.TopicSegment{{SessionID: sid, StartUUID: "u-a", Topic: "A"}, {SessionID: sid, StartUUID: "u-b", Topic: "B"}}
		if err := store.ReplaceSessionSegments(con, sid, segs); err != nil {
			con.Close()
			t.Fatal(err)
		}
		con.Close()
	}
	if _, err := ConsolidateFrom([]string{a, b}, false); err != nil {
		t.Fatalf("first consolidate: %v", err)
	}
	con, err := store.ConnectRW(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{{SessionID: sid, StartUUID: "u-a", Topic: "A"}}); err != nil {
		con.Close()
		t.Fatal(err)
	}
	con.Close()
	if _, err := ConsolidateFrom([]string{a, b}, false); err != nil {
		t.Fatalf("second consolidate: %v", err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM topic_segment WHERE session_id=? AND start_uuid='u-b'", sid); got != "1" {
		t.Fatalf("co-contributor topic B was removed: %s rows, want 1", got)
	}
}

func TestConsolidate_DoesNotRewriteUnchangedSoleSourceTopics(t *testing.T) {
	isolateCache(t)
	a := seedSessionDB(t, "a.db", sessionRow{id: "session-a", project: "a", cwd: "/a", msgs: []msgRow{{"u-a", "user", "a", 100}}})
	b := seedSessionDB(t, "b.db", sessionRow{id: "session-b", project: "b", cwd: "/b", msgs: []msgRow{{"u-b", "user", "b", 100}}})
	for dbp, sid := range map[string]string{a: "session-a", b: "session-b"} {
		con, err := store.ConnectRW(dbp)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.EnsureTopicSchema(con); err != nil {
			con.Close()
			t.Fatal(err)
		}
		segs := []store.TopicSegment{{SessionID: sid, StartUUID: "u-a", Topic: "A"}}
		if sid == "session-b" {
			segs[0].StartUUID = "u-b"
		}
		if err := store.ReplaceSessionSegments(con, sid, segs); err != nil {
			con.Close()
			t.Fatal(err)
		}
		con.Close()
	}
	if _, err := ConsolidateFrom([]string{a, b}, false); err != nil {
		t.Fatal(err)
	}
	con := openConsolidated(t)
	rowID := scalar(t, con, "SELECT id FROM topic_segment WHERE session_id='session-b'")
	con.Close()
	con, err := store.ConnectRW(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceSessionSegments(con, "session-a", nil); err != nil {
		con.Close()
		t.Fatal(err)
	}
	con.Close()
	if _, err := ConsolidateFrom([]string{a, b}, false); err != nil {
		t.Fatal(err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT id FROM topic_segment WHERE session_id='session-b'"); got != rowID {
		t.Fatalf("unchanged sole-source topic rowid changed from %s to %s", rowID, got)
	}
}

func TestConsolidate_RebuildPreservesStoreOnlyTags(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)
	if _, err := ConsolidateFrom([]string{a}, false); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	sid := firstSessionID(t, a)
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTopicSegment(con, sid, "u-a1", "u-a1", "store-only tag", "must survive", 100); err != nil {
		t.Fatal(err)
	}
	con.Close()

	if _, err := ConsolidateFrom([]string{a}, true); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT topic FROM topic_segment WHERE session_id=?", sid); got != "store-only tag" {
		t.Fatalf("topic after rebuild = %q, want preserved store-only tag", got)
	}
	con.Close()
}

// TestConsolidate_RefoldsAfterRetagging pins the watermark: tagging changes a
// source without touching one session or message row, so a watermark built from
// those counts alone would read a re-tagged project as unchanged and its new
// labels would never arrive.
func TestConsolidate_RefoldsAfterRetagging(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)
	sid := firstSessionID(t, a)

	if _, err := ConsolidateFrom([]string{a}, false); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	tagSession(t, a, sid, "u-a1", "invoice reconciliation", "totals did not line up", 100)
	if _, err := ConsolidateFrom([]string{a}, false); err != nil {
		t.Fatalf("second pass: %v", err)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT topic FROM topic_segment WHERE session_id=?", sid); got != "invoice reconciliation" {
		t.Fatalf("topic after re-tag = %q, want the label added between passes", got)
	}
}

// TestConsolidate_RefoldsAfterVerdictOnlyChange pins the verdict watermark:
// changing or adding a verdict without altering messages or topic segments
// must update the watermark so consolidateOne re-folds and updates consolidated.db.
func TestConsolidate_RefoldsAfterVerdictOnlyChange(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)
	sid := firstSessionID(t, a)

	setVerdict(t, a, store.Verdict{
		SessionID:     sid,
		Verdict:       store.VerdictRoutine,
		Source:        store.VerdictSourceFloor,
		OriginMachine: "local",
		TaggedAt:      100,
	})

	if _, err := ConsolidateFrom([]string{a}, false); err != nil {
		t.Fatalf("first pass: %v", err)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT source FROM session_verdict WHERE session_id=?", sid); got != store.VerdictSourceFloor {
		t.Fatalf("verdict source after first pass = %q, want %q", got, store.VerdictSourceFloor)
	}
	con.Close()

	// Update only the verdict on the source DB (tagged_at moves forward, source changes to agent).
	// No session or message or topic_segment rows change.
	setVerdict(t, a, store.Verdict{
		SessionID:     sid,
		Verdict:       store.VerdictRoutine,
		Source:        store.VerdictSourceAgent,
		OriginMachine: "local",
		TaggedAt:      200,
	})

	if _, err := ConsolidateFrom([]string{a}, false); err != nil {
		t.Fatalf("second pass: %v", err)
	}

	con2 := openConsolidated(t)
	if got := scalar(t, con2, "SELECT source FROM session_verdict WHERE session_id=?", sid); got != store.VerdictSourceAgent {
		t.Errorf("verdict source after verdict-only update = %q, want %q", got, store.VerdictSourceAgent)
	}
	if got := scalar(t, con2, "SELECT tagged_at FROM session_verdict WHERE session_id=?", sid); got != "200" && got != "200.0" {
		t.Errorf("verdict tagged_at after verdict-only update = %s, want 200", got)
	}
	con2.Close()

	// Also verify SyncConsolidatedFrom works for verdict-only update
	setVerdict(t, a, store.Verdict{
		SessionID:     sid,
		Verdict:       store.VerdictRoutine,
		Source:        store.VerdictSourceFloor,
		OriginMachine: "local",
		TaggedAt:      300,
	})
	if err := SyncConsolidatedFrom(a); err != nil {
		t.Fatalf("SyncConsolidatedFrom: %v", err)
	}
	con3 := openConsolidated(t)
	if got := scalar(t, con3, "SELECT source FROM session_verdict WHERE session_id=?", sid); got != store.VerdictSourceFloor {
		t.Errorf("verdict source after SyncConsolidatedFrom update = %q, want %q", got, store.VerdictSourceFloor)
	}
	if got := scalar(t, con3, "SELECT tagged_at FROM session_verdict WHERE session_id=?", sid); got != "300" && got != "300.0" {
		t.Errorf("verdict tagged_at after SyncConsolidatedFrom update = %s, want 300", got)
	}
	con3.Close()
}

// TestConsolidate_LaterTaggingWinsForOneSegment covers the re-tag rule: the
// same segment tagged twice keeps the later label, whichever order the sources
// are folded in, so the pass stays order-independent.
func TestConsolidate_LaterTaggingWinsForOneSegment(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)
	sid := firstSessionID(t, a)

	tagSession(t, a, sid, "u-a1", "first guess", "", 100)
	if _, err := ConsolidateFrom([]string{a}, false); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	tagSession(t, a, sid, "u-a1", "better label", "", 200)
	if _, err := ConsolidateFrom([]string{a}, false); err != nil {
		t.Fatalf("second pass: %v", err)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT topic FROM topic_segment WHERE session_id=?", sid); got != "better label" {
		t.Errorf("topic = %q, want the later tagging to win", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM topic_segment"); got != "1" {
		t.Errorf("topic_segment holds %s rows, want the re-tag to update in place", got)
	}
	// The external-content index must follow the update, or the store keeps
	// answering with a label the rows no longer carry.
	if got := scalar(t, con, "SELECT COUNT(*) FROM topic_fts WHERE topic_fts MATCH 'guess'"); got != "0" {
		t.Errorf("topic_fts still matched the replaced label %s times, want 0", got)
	}
}

// TestConsolidate_FoldsTopicsFromAnOlderTopicSchema is the guard on the oldest
// tags in the corpus. The topic table gained origin_machine after it shipped, so
// a project tagged before that has a topic_segment without the column — and a
// merge that names it unconditionally fails on precisely those sources, leaving
// the longest-standing labels invisible to every one-store reader while the
// newer ones fold in fine. The source is rebuilt in the pre-provenance shape
// here rather than mocked, because the shape is what the merge trips over.
func TestConsolidate_FoldsTopicsFromAnOlderTopicSchema(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)
	sid := firstSessionID(t, a)
	tagSession(t, a, sid, "u-a1", "invoice reconciliation", "totals did not line up", 100)

	// Drop the source back to the pre-provenance topic table, keeping the row.
	con, err := store.ConnectRW(a)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE topic_old (
		   id INTEGER PRIMARY KEY AUTOINCREMENT,
		   session_id TEXT NOT NULL, start_uuid TEXT NOT NULL, end_uuid TEXT,
		   topic TEXT, summary TEXT, tagged_at REAL,
		   UNIQUE(session_id, start_uuid))`,
		`INSERT INTO topic_old(session_id,start_uuid,end_uuid,topic,summary,tagged_at)
		   SELECT session_id,start_uuid,end_uuid,topic,summary,tagged_at FROM topic_segment`,
		`DROP TABLE topic_segment`,
		`ALTER TABLE topic_old RENAME TO topic_segment`,
	} {
		if _, err := con.Exec(stmt); err != nil {
			t.Fatalf("reshape source topic table: %v", err)
		}
	}
	con.Close()

	if _, err := ConsolidateFrom([]string{a}, false); err != nil {
		t.Fatalf("ConsolidateFrom over a pre-provenance topic table: %v", err)
	}
	dst := openConsolidated(t)
	if got := scalar(t, dst, "SELECT topic FROM topic_segment WHERE session_id=?", sid); got != "invoice reconciliation" {
		t.Fatalf("folded topic = %q, want the old-schema source's label", got)
	}
	// An untracked origin is stored as NULL, which is what it already means
	// everywhere else here — not an empty string that would read as a machine.
	if got := scalar(t, dst, "SELECT origin_machine IS NULL FROM topic_segment WHERE session_id=?", sid); got != "1" {
		t.Errorf("origin_machine IS NULL = %s, want 1 for a source that never recorded one", got)
	}
}

// TestConsolidate_SkipsASourceWithNoTopicLayer keeps an untagged project from
// failing the whole pass: the topic tables are created on demand, so a project
// nobody tagged genuinely has none.
func TestConsolidate_SkipsASourceWithNoTopicLayer(t *testing.T) {
	isolateCache(t)
	a := indexProject(t, "-w-ledger",
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","uuid":"u-a1","message":{"role":"user","content":"reconcile the invoice totals"}}`)

	st, err := ConsolidateFrom([]string{a}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom over an untagged source: %v", err)
	}
	if st.Sessions != 1 {
		t.Fatalf("consolidated %d sessions, want 1", st.Sessions)
	}
	// The store still has the topic tables, so a topic query answers "nothing is
	// tagged yet" rather than failing on a missing table.
	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM topic_segment"); got != "0" {
		t.Errorf("topic_segment = %s rows, want 0", got)
	}
}

// TestConsolidateKeepsSourceMessageOrder pins the reading order across the
// fold. The consolidated rowid IS the reading order (every session view walks
// by id, because timestamps are not reliably monotonic), so the merge must
// insert in the source's own insertion order. Without ORDER BY first_id the
// GROUP BY hands rows over sorted by uuid — random — and the conversation is
// replayed shuffled: measured 1490 of 2978 adjacent pairs out of place on a
// real 3k-message session, with a mid-session tool call served as the opening.
func TestConsolidateKeepsSourceMessageOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	// uuids chosen so ALPHABETICAL order is the exact REVERSE of insert order:
	// if the merge ever sorts by uuid again, this test reads back backwards.
	src := filepath.Join(dir, "src-order.db")
	con, err := store.ConnectRW(src)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := con.Exec(
		`INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent,source_path,project,cwd)
		 VALUES('s-order',1,9,4,0,'/tmp/s-order.jsonl','p','/tmp')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	want := []string{"zzzz-first", "yyyy-second", "xxxx-third", "wwww-fourth"}
	for i, u := range want {
		if _, err := con.Exec(
			`INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)`,
			"s-order", "user", "message body "+u, float64(i+1), "2026-01-0"+strconv.Itoa(i+1), u); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}
	_ = con.Close()

	if _, err := ConsolidateFrom([]string{src}, true); err != nil {
		t.Fatalf("consolidate: %v", err)
	}

	dst, _, err := OpenConsolidated()
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	defer dst.Close()
	rows, err := dst.Query(`SELECT uuid FROM messages WHERE session_id='s-order' ORDER BY id`)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, u)
	}
	if len(got) != len(want) {
		t.Fatalf("read back %d messages, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message %d is %q, want %q — the fold reordered the conversation (got %v)",
				i, got[i], want[i], got)
		}
	}
}

// TestConsolidate_OriginAuthorityWinsForConflictingTopicSegments verifies that
// when two source dbs have conflicting topic segments for the same session and
// start_uuid, the one with higher origin_machine wins in consolidated.db,
// regardless of which one has a larger tagged_at timestamp (and on equal
// origin_machine, larger tagged_at wins).
func TestConsolidate_OriginAuthorityWinsForConflictingTopicSegments(t *testing.T) {
	t.Run("HigherOriginMachineWinsRegardlessOfTimestamp", func(t *testing.T) {
		isolateCache(t)
		transcript := `{"type":"user","cwd":"/w/shared","timestamp":"2026-06-01T10:00:00Z","uuid":"u-1","message":{"role":"user","content":"shared conversation"}}`
		// Two source dbs representing two machines indexing the same session.
		a := indexProject(t, "-w-peer-a", transcript)
		b := indexProject(t, "-w-peer-b", transcript)
		sid := firstSessionID(t, a)

		// Machine A has lower origin authority ("machine-a"), but newer timestamp (2000).
		tagSessionWithOrigin(t, a, sid, "u-1", "peer a topic", "peer a summary", "machine-a", 2000)
		// Machine B has higher origin authority ("machine-b"), but older timestamp (1000).
		tagSessionWithOrigin(t, b, sid, "u-1", "peer b topic", "peer b summary", "machine-b", 1000)

		// Order 1: fold [a, b]
		if _, err := ConsolidateFrom([]string{a, b}, true); err != nil {
			t.Fatalf("consolidate [a, b]: %v", err)
		}
		con := openConsolidated(t)
		if got := scalar(t, con, "SELECT topic FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "peer b topic" {
			t.Errorf("order [a, b]: topic = %q, want %q (higher origin_machine should win)", got, "peer b topic")
		}
		if got := scalar(t, con, "SELECT origin_machine FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "machine-b" {
			t.Errorf("order [a, b]: origin_machine = %q, want %q", got, "machine-b")
		}
		if got := scalar(t, con, "SELECT tagged_at FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "1000" && got != "1000.0" {
			t.Errorf("order [a, b]: tagged_at = %s, want 1000", got)
		}
		if got := scalar(t, con, "SELECT COUNT(*) FROM topic_fts WHERE topic_fts MATCH 'peer AND b'"); got != "1" {
			t.Errorf("order [a, b]: topic_fts match for 'peer AND b' = %s, want 1", got)
		}
		if got := scalar(t, con, "SELECT COUNT(*) FROM topic_fts WHERE topic_fts MATCH 'peer AND a'"); got != "0" {
			t.Errorf("order [a, b]: topic_fts match for 'peer AND a' = %s, want 0", got)
		}
		con.Close()

		// Order 2: fold [b, a] (reverse order must converge to same winner)
		if _, err := ConsolidateFrom([]string{b, a}, true); err != nil {
			t.Fatalf("consolidate [b, a]: %v", err)
		}
		con2 := openConsolidated(t)
		if got := scalar(t, con2, "SELECT topic FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "peer b topic" {
			t.Errorf("order [b, a]: topic = %q, want %q (higher origin_machine should win)", got, "peer b topic")
		}
		if got := scalar(t, con2, "SELECT origin_machine FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "machine-b" {
			t.Errorf("order [b, a]: origin_machine = %q, want %q", got, "machine-b")
		}
		if got := scalar(t, con2, "SELECT tagged_at FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "1000" && got != "1000.0" {
			t.Errorf("order [b, a]: tagged_at = %s, want 1000", got)
		}
		if got := scalar(t, con2, "SELECT COUNT(*) FROM topic_fts WHERE topic_fts MATCH 'peer AND b'"); got != "1" {
			t.Errorf("order [b, a]: topic_fts match for 'peer AND b' = %s, want 1", got)
		}
		if got := scalar(t, con2, "SELECT COUNT(*) FROM topic_fts WHERE topic_fts MATCH 'peer AND a'"); got != "0" {
			t.Errorf("order [b, a]: topic_fts match for 'peer AND a' = %s, want 0", got)
		}
		con2.Close()
	})

	t.Run("EqualOriginMachineNewerTaggedAtWins", func(t *testing.T) {
		isolateCache(t)
		transcript := `{"type":"user","cwd":"/w/shared","timestamp":"2026-06-01T10:00:00Z","uuid":"u-1","message":{"role":"user","content":"shared conversation"}}`
		c1 := indexProject(t, "-w-peer-c1", transcript)
		c2 := indexProject(t, "-w-peer-c2", transcript)
		sid := firstSessionID(t, c1)

		// Both have same origin authority ("machine-c"), but c2 is newer.
		tagSessionWithOrigin(t, c1, sid, "u-1", "early label", "early summary", "machine-c", 100)
		tagSessionWithOrigin(t, c2, sid, "u-1", "newer label", "newer summary", "machine-c", 300)

		// Order 1: fold [c1, c2]
		if _, err := ConsolidateFrom([]string{c1, c2}, true); err != nil {
			t.Fatalf("consolidate [c1, c2]: %v", err)
		}
		con := openConsolidated(t)
		if got := scalar(t, con, "SELECT topic FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "newer label" {
			t.Errorf("order [c1, c2]: topic = %q, want %q (newer tagged_at on tie)", got, "newer label")
		}
		if got := scalar(t, con, "SELECT tagged_at FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "300" && got != "300.0" {
			t.Errorf("order [c1, c2]: tagged_at = %s, want 300", got)
		}
		con.Close()

		// Order 2: fold [c2, c1]
		if _, err := ConsolidateFrom([]string{c2, c1}, true); err != nil {
			t.Fatalf("consolidate [c2, c1]: %v", err)
		}
		con2 := openConsolidated(t)
		if got := scalar(t, con2, "SELECT topic FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "newer label" {
			t.Errorf("order [c2, c1]: topic = %q, want %q (newer tagged_at on tie)", got, "newer label")
		}
		if got := scalar(t, con2, "SELECT tagged_at FROM topic_segment WHERE session_id=? AND start_uuid=?", sid, "u-1"); got != "300" && got != "300.0" {
			t.Errorf("order [c2, c1]: tagged_at = %s, want 300", got)
		}
		con2.Close()
	})
}

// TestConsolidate_VerdictMergePrecedence verifies that verdicts merge by
// latest tagged_at, and on tie broken by higher origin_machine.
func TestConsolidate_VerdictMergePrecedence(t *testing.T) {
	isolateCache(t)
	transcript := `{"type":"user","cwd":"/w/shared","timestamp":"2026-06-01T10:00:00Z","uuid":"u-1","message":{"role":"user","content":"shared conversation"}}`
	a := indexProject(t, "-w-peer-a", transcript)
	b := indexProject(t, "-w-peer-b", transcript)
	sid := firstSessionID(t, a)

	// Equal tagged_at (100), machine-b should win tie over machine-a.
	setVerdict(t, a, store.Verdict{
		SessionID:     sid,
		Verdict:       store.VerdictRoutine,
		Source:        store.VerdictSourceFloor,
		OriginMachine: "machine-a",
		TaggedAt:      100,
	})
	setVerdict(t, b, store.Verdict{
		SessionID:     sid,
		Verdict:       store.VerdictRoutine,
		Source:        store.VerdictSourceAgent,
		OriginMachine: "machine-b",
		TaggedAt:      100,
	})

	if _, err := ConsolidateFrom([]string{a, b}, true); err != nil {
		t.Fatalf("consolidate [a, b]: %v", err)
	}
	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT source FROM session_verdict WHERE session_id=?", sid); got != store.VerdictSourceAgent {
		t.Errorf("order [a, b]: source = %q, want %q (machine-b wins tie)", got, store.VerdictSourceAgent)
	}
	if got := scalar(t, con, "SELECT origin_machine FROM session_verdict WHERE session_id=?", sid); got != "machine-b" {
		t.Errorf("order [a, b]: origin_machine = %q, want %q", got, "machine-b")
	}
	con.Close()

	if _, err := ConsolidateFrom([]string{b, a}, true); err != nil {
		t.Fatalf("consolidate [b, a]: %v", err)
	}
	con2 := openConsolidated(t)
	if got := scalar(t, con2, "SELECT source FROM session_verdict WHERE session_id=?", sid); got != store.VerdictSourceAgent {
		t.Errorf("order [b, a]: source = %q, want %q (machine-b wins tie)", got, store.VerdictSourceAgent)
	}
	if got := scalar(t, con2, "SELECT origin_machine FROM session_verdict WHERE session_id=?", sid); got != "machine-b" {
		t.Errorf("order [b, a]: origin_machine = %q, want %q", got, "machine-b")
	}
	con2.Close()
}

// TestConsolidate_SingleSourcePurgePropagatesMissing tests that when a session's
// only source db is purged, the consolidated store learns about it and stamps only_copy_since.
func TestConsolidate_SingleSourcePurgePropagatesMissing(t *testing.T) {
	isolateCache(t)
	const id = "single-source-sess"
	db := seedSessionDB(t, "single.db", sessionRow{
		id: id, project: "ledger", cwd: "/w/ledger", missing: 0,
		msgs: []msgRow{{"u-1", "user", "hello", 100}},
	})

	if err := SyncConsolidatedFrom(db); err != nil {
		t.Fatalf("SyncConsolidatedFrom (live): %v", err)
	}
	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COALESCE(only_copy_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Fatalf("initial only_copy_since = %s, want NULL", got)
	}
	con.Close()

	// Source is purged upstream and stamped with only_copy_since.
	conSrc, err := store.ConnectRW(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	const purgeTS = 1784381448.0
	if _, err := conSrc.Exec("UPDATE sessions SET only_copy_since=? WHERE id=?", purgeTS, id); err != nil {
		t.Fatalf("update only_copy_since: %v", err)
	}
	conSrc.Close()

	if err := SyncConsolidatedFrom(db); err != nil {
		t.Fatalf("SyncConsolidatedFrom (purged): %v", err)
	}
	con = openConsolidated(t)
	var onlyCopy sql.NullFloat64
	if err := con.QueryRow("SELECT only_copy_since FROM sessions WHERE id=?", id).Scan(&onlyCopy); err != nil {
		t.Fatalf("query only_copy_since: %v", err)
	}
	if !onlyCopy.Valid || onlyCopy.Float64 != purgeTS {
		t.Errorf("purged only_copy_since = %v, want %v", onlyCopy.Float64, purgeTS)
	}
	con.Close()
}

// TestConsolidate_DeletesSessionRemovedFromSource proves that a physical
// source deletion does not leave a ghost session or orphaned messages in the
// consolidated store.
func TestConsolidate_DeletesSessionRemovedFromSource(t *testing.T) {
	isolateCache(t)
	const id = "deleted-source-session"
	db := seedSessionDB(t, "deleted.db", sessionRow{
		id: id, project: "ledger", cwd: "/w/ledger", missing: 0,
		msgs: []msgRow{{"u-1", "user", "remove this session", 100}},
	})

	if err := SyncConsolidatedFrom(db); err != nil {
		t.Fatalf("SyncConsolidatedFrom (initial): %v", err)
	}

	src, err := store.ConnectRW(db)
	if err != nil {
		t.Fatalf("open source db: %v", err)
	}
	if _, err := src.Exec("DELETE FROM messages WHERE session_id=?", id); err != nil {
		t.Fatalf("delete source messages: %v", err)
	}
	if _, err := src.Exec("DELETE FROM sessions WHERE id=?", id); err != nil {
		t.Fatalf("delete source session: %v", err)
	}
	src.Close()

	if err := SyncConsolidatedFrom(db); err != nil {
		t.Fatalf("SyncConsolidatedFrom (deleted): %v", err)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions WHERE id=?", id); got != "0" {
		t.Errorf("deleted source session count = %s, want 0", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM messages WHERE session_id=?", id); got != "0" {
		t.Errorf("deleted source message count = %s, want 0", got)
	}
}

// TestConsolidate_DistinguishesSourcesWithTheSameBasename proves that two
// source databases in different directories keep independent contributions.
// Otherwise syncing the second sessions.db overwrites the first contribution,
// and deleting the second source incorrectly removes the still-live session.
func TestConsolidate_DistinguishesSourcesWithTheSameBasename(t *testing.T) {
	isolateCache(t)
	const id = "same-basename-session"
	first := seedSessionDB(t, "sessions.db", sessionRow{
		id: id, project: "first", cwd: "/w/first", missing: 0,
		msgs: []msgRow{{"first", "user", "first source", 100}},
	})
	second := seedSessionDB(t, "sessions.db", sessionRow{
		id: id, project: "second", cwd: "/w/second", missing: 0,
		msgs: []msgRow{{"first", "user", "first source", 100}, {"second", "user", "second source", 200}},
	})

	for _, src := range []string{first, second} {
		if err := SyncConsolidatedFrom(src); err != nil {
			t.Fatalf("initial sync %s: %v", src, err)
		}
	}

	src, err := store.ConnectRW(second)
	if err != nil {
		t.Fatalf("open second source: %v", err)
	}
	if _, err := src.Exec("DELETE FROM messages WHERE session_id=?", id); err != nil {
		src.Close()
		t.Fatalf("delete second source messages: %v", err)
	}
	if _, err := src.Exec("DELETE FROM sessions WHERE id=?", id); err != nil {
		src.Close()
		t.Fatalf("delete second source session: %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatalf("close second source: %v", err)
	}

	if err := SyncConsolidatedFrom(second); err != nil {
		t.Fatalf("sync deleted second source: %v", err)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions WHERE id=?", id); got != "1" {
		t.Errorf("session count after deleting one same-basename source = %s, want 1", got)
	}
	if got := scalar(t, con, "SELECT COALESCE(only_copy_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Errorf("only_copy_since after deleting one same-basename source = %s, want NULL", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_sources WHERE session_id=?", id); got != "1" {
		t.Errorf("session source count after deleting one same-basename source = %s, want 1", got)
	}
}

// TestUnconsolidatedDBs_DoesNotTrustAmbiguousLegacyBasename keeps a legacy
// basename watermark from hiding a different source with the same basename.
func TestUnconsolidatedDBs_DoesNotTrustAmbiguousLegacyBasename(t *testing.T) {
	isolateCache(t)
	first := seedSessionDB(t, "sessions.db", sessionRow{
		id: "first-legacy-source", project: "first", cwd: "/w/first",
		msgs: []msgRow{{"first", "user", "first source", 100}},
	})
	second := seedSessionDB(t, "sessions.db", sessionRow{
		id: "second-unprocessed-source", project: "second", cwd: "/w/second",
		msgs: []msgRow{{"second", "user", "second source", 200}},
	})

	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if _, err := con.Exec("INSERT INTO meta(key, value) VALUES('sync:sessions.db', 'legacy-mark')"); err != nil {
		t.Fatalf("seed legacy watermark: %v", err)
	}

	missing, err := unconsolidatedDBs(con, []string{first, second})
	if err != nil {
		t.Fatalf("unconsolidatedDBs: %v", err)
	}
	if len(missing) != 2 {
		t.Fatalf("missing databases = %v, want both same-basename sources", missing)
	}
	for _, want := range []string{first, second} {
		found := false
		for _, got := range missing {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing databases = %v, want %s included", missing, want)
		}
	}
}

// TestUnconsolidatedDBs_RemoveAndReplaceLegacyBasename tests issue #14:
// When database A (with legacy basename watermark) is removed and replaced by an
// unrelated database B with the same basename, B must NOT inherit A's stale watermark.
// B must be reported unconsolidated and successfully consolidated.
func TestUnconsolidatedDBs_RemoveAndReplaceLegacyBasename(t *testing.T) {
	isolateCache(t)
	first := seedSessionDB(t, "sessions.db", sessionRow{
		id: "first-session", project: "first", cwd: "/w/first",
		msgs: []msgRow{{"first-msg", "user", "first message", 100}},
	})

	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if _, err := con.Exec("INSERT INTO meta(key, value) VALUES('sync:sessions.db', 'legacy-mark')"); err != nil {
		t.Fatalf("seed legacy watermark: %v", err)
	}

	// First pass: sole candidate 'first' carries the legacy basename.
	missingA, err := unconsolidatedDBs(con, []string{first})
	if err != nil {
		t.Fatalf("unconsolidatedDBs first: %v", err)
	}
	if len(missingA) != 0 {
		t.Fatalf("missing databases for first pass = %v, want empty (accepted legacy watermark)", missingA)
	}

	// Remove database A.
	if err := os.Remove(first); err != nil {
		t.Fatalf("remove first: %v", err)
	}

	// Introduce an unrelated database B with the same basename "sessions.db".
	second := seedSessionDB(t, "sessions.db", sessionRow{
		id: "second-session", project: "second", cwd: "/w/second",
		msgs: []msgRow{{"second-msg", "user", "second message", 200}},
	})

	// Second pass: B is now the sole candidate, but must NOT inherit A's stale watermark.
	missingB, err := unconsolidatedDBs(con, []string{second})
	if err != nil {
		t.Fatalf("unconsolidatedDBs second: %v", err)
	}
	if len(missingB) != 1 || missingB[0] != second {
		t.Fatalf("missing databases for replaced second = %v, want [%s]", missingB, second)
	}

	// Consolidate second and assert its content is actually merged rather than skipped.
	if _, err := ConsolidateFrom([]string{second}, false); err != nil {
		t.Fatalf("consolidate second: %v", err)
	}
	var count int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='second-session'").Scan(&count); err != nil {
		t.Fatalf("query second session: %v", err)
	}
	if count != 1 {
		t.Errorf("second session count = %d, want 1 (database B should be consolidated)", count)
	}
}

// TestConsolidate_MergedSessionHonestPerContributionSemantics tests ticket #181:
//   - A session merged from 3 sources where 1 or 2 are purged remains LIVE (only_copy_since = NULL).
//   - When ALL 3 contributing sources are purged, the merged row learns of it and stamps
//     only_copy_since with the latest purge timestamp (the moment the last live copy vanished).
//   - When 1 source is restored, the merged session becomes live again (only_copy_since = NULL).
func TestConsolidate_MergedSessionHonestPerContributionSemantics(t *testing.T) {
	isolateCache(t)
	const id = "multi-source-sess"
	p1 := seedSessionDB(t, "p1.db", sessionRow{
		id: id, project: "proj1", cwd: "/w/proj1", missing: 0,
		msgs: []msgRow{{"u-1", "user", "turn 1", 100}},
	})
	p2 := seedSessionDB(t, "p2.db", sessionRow{
		id: id, project: "proj2", cwd: "/w/proj2", missing: 0,
		msgs: []msgRow{{"u-1", "user", "turn 1", 100}, {"u-2", "assistant", "turn 2", 200}},
	})
	p3 := seedSessionDB(t, "p3.db", sessionRow{
		id: id, project: "proj3", cwd: "/w/proj3", missing: 0,
		msgs: []msgRow{{"u-1", "user", "turn 1", 100}, {"u-3", "user", "turn 3", 300}},
	})

	// Initial fold: all 3 live -> consolidated must be live.
	if err := SyncConsolidatedFrom(p1); err != nil {
		t.Fatalf("sync p1: %v", err)
	}
	if err := SyncConsolidatedFrom(p2); err != nil {
		t.Fatalf("sync p2: %v", err)
	}
	if err := SyncConsolidatedFrom(p3); err != nil {
		t.Fatalf("sync p3: %v", err)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COALESCE(only_copy_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Fatalf("initial only_copy_since = %s, want NULL", got)
	}
	con.Close()

	// Step 1: p1 is purged at t=1000. p2 and p3 are still live.
	conP1, _ := store.ConnectRW(p1)
	_, _ = conP1.Exec("UPDATE sessions SET only_copy_since=? WHERE id=?", 1000.0, id)
	conP1.Close()

	if err := SyncConsolidatedFrom(p1); err != nil {
		t.Fatalf("sync p1 (purged 1000): %v", err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COALESCE(only_copy_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Errorf("only_copy_since = %s, want NULL (p2 and p3 are still live on disk)", got)
	}
	con.Close()

	// Step 2: p2 is ALSO purged at t=2000. p3 is still live.
	conP2, _ := store.ConnectRW(p2)
	_, _ = conP2.Exec("UPDATE sessions SET only_copy_since=? WHERE id=?", 2000.0, id)
	conP2.Close()

	if err := SyncConsolidatedFrom(p2); err != nil {
		t.Fatalf("sync p2 (purged 2000): %v", err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COALESCE(only_copy_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Errorf("only_copy_since = %s, want NULL (p3 is still live on disk)", got)
	}
	con.Close()

	// Step 3: p3 is ALSO purged at t=3000. Now ALL contributing copies are purged.
	conP3, _ := store.ConnectRW(p3)
	_, _ = conP3.Exec("UPDATE sessions SET only_copy_since=? WHERE id=?", 3000.0, id)
	conP3.Close()

	if err := SyncConsolidatedFrom(p3); err != nil {
		t.Fatalf("sync p3 (purged 3000): %v", err)
	}
	con = openConsolidated(t)
	var onlyCopy3 sql.NullFloat64
	if err := con.QueryRow("SELECT only_copy_since FROM sessions WHERE id=?", id).Scan(&onlyCopy3); err != nil {
		t.Fatalf("query only_copy_since: %v", err)
	}
	if !onlyCopy3.Valid || onlyCopy3.Float64 != 3000.0 {
		t.Errorf("all sources purged: only_copy_since = %v, want 3000 (latest purge watermark)", onlyCopy3.Float64)
	}
	con.Close()

	// Step 4: p2 is restored on disk (undeleted) -> only_copy_since becomes NULL in p2.db.
	conP2, _ = store.ConnectRW(p2)
	_, _ = conP2.Exec("UPDATE sessions SET only_copy_since=NULL WHERE id=?", id)
	conP2.Close()

	if err := SyncConsolidatedFrom(p2); err != nil {
		t.Fatalf("sync p2 (restored): %v", err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COALESCE(only_copy_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Errorf("restored p2: only_copy_since = %s, want NULL", got)
	}
	con.Close()
}

// TestConsolidate_LegacyStoreSessionSourcesBackfill exercises the upgrade path:
// an existing store populated before table session_sources was introduced.
// It verifies that:
//   - An unscanned session's message_count, only_copy_since, project, and cwd remain UNCHANGED.
//   - session_sources is backfilled with a legacy contribution for unscanned sessions.
//   - A multi-source session previously merged does not get falsely marked only-copy or have its
//     scope clobbered when an incremental pass scans a partial/purged source copy.
func TestConsolidate_LegacyStoreSessionSourcesBackfill(t *testing.T) {
	isolateCache(t)
	conPath := ConsolidatedPath()
	con, err := store.ConnectRW(conPath)
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	// Simulate pre-bd6cda9 store by dropping session_sources
	if _, err := con.Exec("DROP TABLE IF EXISTS session_sources"); err != nil {
		t.Fatalf("drop session_sources: %v", err)
	}

	const unscannedID = "sess-unscanned"
	if _, err := con.Exec(`
		INSERT INTO sessions (
			id, started_at, last_ts, message_count, is_subagent, parent_id,
			origin_machine, source_tool, source_path, only_copy_since, project, cwd
		) VALUES (?, 100, 200, 5, 0, NULL, 'mach-1', 'claude', '/w/unscanned/a.jsonl', NULL, 'unscanned-proj', '/w/unscanned')
	`, unscannedID); err != nil {
		t.Fatalf("seed unscanned session: %v", err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := con.Exec("INSERT INTO messages (session_id, role, content, ts, ts_iso, uuid) VALUES (?, 'user', 'msg', ?, '', ?)",
			unscannedID, float64(100+i), "u-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("seed message: %v", err)
		}
	}

	const multiID = "sess-multi"
	if _, err := con.Exec(`
		INSERT INTO sessions (
			id, started_at, last_ts, message_count, is_subagent, parent_id,
			origin_machine, source_tool, source_path, only_copy_since, project, cwd
		) VALUES (?, 100, 400, 8, 0, NULL, 'mach-1', 'claude', '/w/multi/b.jsonl', NULL, 'multi-proj', '/w/multi')
	`, multiID); err != nil {
		t.Fatalf("seed multi session: %v", err)
	}
	for i := 1; i <= 8; i++ {
		if _, err := con.Exec("INSERT INTO messages (session_id, role, content, ts, ts_iso, uuid) VALUES (?, 'user', 'msg', ?, '', ?)",
			multiID, float64(100+i), "m-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("seed multi message: %v", err)
		}
	}
	con.Close()

	// Create a new source DB containing sess-multi (purged in this source at t=9999, 2 msgs)
	// and a new session sess-new. unscannedID is NOT in this source DB.
	srcDB := seedSessionDB(t, "new-pass.db",
		sessionRow{
			id: multiID, project: "sub-proj", cwd: "/w/sub-proj", missing: 9999.0,
			msgs: []msgRow{{"m-1", "user", "msg", 101}, {"m-2", "user", "msg", 102}},
		},
		sessionRow{
			id: "sess-new", project: "new-proj", cwd: "/w/new", missing: 0,
			msgs: []msgRow{{"n-1", "user", "hello", 600}},
		},
	)

	// Run the new merge
	if err := SyncConsolidatedFrom(srcDB); err != nil {
		t.Fatalf("SyncConsolidatedFrom: %v", err)
	}

	conRO := openConsolidated(t)

	// Assert unscanned session is completely UNCHANGED:
	var msgCount int
	var onlyCopySince sql.NullFloat64
	var project, cwd string
	err = conRO.QueryRow("SELECT message_count, only_copy_since, project, cwd FROM sessions WHERE id=?", unscannedID).
		Scan(&msgCount, &onlyCopySince, &project, &cwd)
	if err != nil {
		t.Fatalf("query unscanned session: %v", err)
	}
	if msgCount != 5 {
		t.Errorf("unscanned message_count = %d, want 5", msgCount)
	}
	if onlyCopySince.Valid {
		t.Errorf("unscanned only_copy_since = %v, want NULL", onlyCopySince.Float64)
	}
	if project != "unscanned-proj" {
		t.Errorf("unscanned project = %q, want 'unscanned-proj'", project)
	}
	if cwd != "/w/unscanned" {
		t.Errorf("unscanned cwd = %q, want '/w/unscanned'", cwd)
	}

	// Assert session_sources has a backfilled entry for unscannedID
	var srcCount int
	if err := conRO.QueryRow("SELECT COUNT(*) FROM session_sources WHERE session_id=?", unscannedID).Scan(&srcCount); err != nil {
		t.Fatalf("count unscanned session_sources: %v", err)
	}
	if srcCount != 1 {
		t.Errorf("unscanned session_sources count = %d, want 1 (backfilled)", srcCount)
	}

	// Assert multi-source session preserved its live state and scope despite partial purged scan:
	err = conRO.QueryRow("SELECT message_count, only_copy_since, project, cwd FROM sessions WHERE id=?", multiID).
		Scan(&msgCount, &onlyCopySince, &project, &cwd)
	if err != nil {
		t.Fatalf("query multi session: %v", err)
	}
	if msgCount != 8 {
		t.Errorf("multi message_count = %d, want 8", msgCount)
	}
	if onlyCopySince.Valid {
		t.Errorf("multi only_copy_since = %v, want NULL (legacy contribution was live)", onlyCopySince.Float64)
	}
	if project != "multi-proj" {
		t.Errorf("multi project = %q, want 'multi-proj' (legacy richer contribution)", project)
	}
	if cwd != "/w/multi" {
		t.Errorf("multi cwd = %q, want '/w/multi'", cwd)
	}

	// Assert session_sources for multiID has both the legacy backfill and the new pass source
	if err := conRO.QueryRow("SELECT COUNT(*) FROM session_sources WHERE session_id=?", multiID).Scan(&srcCount); err != nil {
		t.Fatalf("count multi session_sources: %v", err)
	}
	if srcCount != 2 {
		t.Errorf("multi session_sources count = %d, want 2", srcCount)
	}
	conRO.Close()
}

// TestConsolidate_PartialSessionSourcesBackfill tests backfilling an existing store
// where some sessions already have session_sources records (e.g. from post-bd6cda9 syncs)
// and other sessions do not.
func TestConsolidate_PartialSessionSourcesBackfill(t *testing.T) {
	isolateCache(t)
	conPath := ConsolidatedPath()
	con, err := store.ConnectRW(conPath)
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	// Seed 3 sessions in sessions table
	for _, id := range []string{"s1", "s2", "s3"} {
		if _, err := con.Exec(`
			INSERT INTO sessions (
				id, started_at, last_ts, message_count, is_subagent, parent_id,
				origin_machine, source_tool, source_path, only_copy_since, project, cwd
			) VALUES (?, 100, 200, 3, 0, NULL, 'mach-1', 'claude', '/w/p/s.jsonl', NULL, 'proj', '/w/p')
		`, id); err != nil {
			t.Fatalf("seed session %s: %v", id, err)
		}
	}

	// Pre-populate session_sources ONLY for s1 (e.g. from an incremental sync)
	if _, err := con.Exec(`
		INSERT INTO session_sources (
			session_id, source_db, started_at, last_ts, message_count, is_subagent, parent_id,
			origin_machine, source_tool, source_path, only_copy_since, project, cwd
		) VALUES ('s1', 'existing.db', 100, 200, 3, 0, NULL, 'mach-1', 'claude', '/w/p/s.jsonl', NULL, 'proj', '/w/p')
	`); err != nil {
		t.Fatalf("seed session_sources for s1: %v", err)
	}

	// Run migration
	if err := migrateSessionSources(con); err != nil {
		t.Fatalf("migrateSessionSources: %v", err)
	}

	// s1 should only have 1 source ('existing.db'), not duplicated with empty source_db
	var s1Sources int
	if err := con.QueryRow("SELECT COUNT(*) FROM session_sources WHERE session_id='s1'").Scan(&s1Sources); err != nil {
		t.Fatalf("count s1 sources: %v", err)
	}
	if s1Sources != 1 {
		t.Errorf("s1 session_sources count = %d, want 1", s1Sources)
	}
	var s1DB string
	if err := con.QueryRow("SELECT source_db FROM session_sources WHERE session_id='s1'").Scan(&s1DB); err != nil {
		t.Fatalf("query s1 source_db: %v", err)
	}
	if s1DB != "existing.db" {
		t.Errorf("s1 source_db = %q, want 'existing.db'", s1DB)
	}

	// s2 and s3 should each have 1 source (with source_db = '')
	for _, id := range []string{"s2", "s3"} {
		var cnt int
		if err := con.QueryRow("SELECT COUNT(*) FROM session_sources WHERE session_id=?", id).Scan(&cnt); err != nil {
			t.Fatalf("count %s sources: %v", id, err)
		}
		if cnt != 1 {
			t.Errorf("%s session_sources count = %d, want 1", id, cnt)
		}
		var db string
		if err := con.QueryRow("SELECT source_db FROM session_sources WHERE session_id=?", id).Scan(&db); err != nil {
			t.Fatalf("query %s source_db: %v", id, err)
		}
		if db != "" {
			t.Errorf("%s source_db = %q, want ''", id, db)
		}
	}
}

func TestConsolidateFrom_PrunesLegacySourceAfterFullPass(t *testing.T) {
	isolateCache(t)
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	if err := EnsureSchema(con, sourceClaude); err != nil {
		con.Close()
		t.Fatalf("ensure schema: %v", err)
	}
	for _, id := range []string{"gains-real", "legacy-only"} {
		if _, err := con.Exec(`
			INSERT INTO sessions (
				id, started_at, last_ts, message_count, is_subagent, parent_id,
				origin_machine, source_tool, source_path, only_copy_since, project, cwd
			) VALUES (?, 100, 200, 1, 0, NULL, 'machine', 'claude', '/tmp/' || ? || '.jsonl', NULL, 'project', '/tmp')
		`, id, id); err != nil {
			con.Close()
			t.Fatalf("seed session %s: %v", id, err)
		}
	}
	con.Close()

	src := seedSessionDB(t, "real.db", sessionRow{
		id: "gains-real", project: "project", cwd: "/tmp",
		msgs: []msgRow{{"real-1", "user", "hello", 100}},
	})
	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_sources WHERE session_id='gains-real' AND source_db='' "); got != "0" {
		t.Errorf("legacy row for session with real source = %s, want 0", got)
	}
	// source_db holds the source's full-path identity (sourceIdentity), not its
	// basename. This assertion used to hard-code "real.db" and silently began
	// failing when identity moved from basename to absolute path — the row was
	// always written correctly, the test was just looking under the old name.
	var realRows int
	if err := con.QueryRow(
		"SELECT COUNT(*) FROM session_sources WHERE session_id='gains-real' AND source_db=?",
		sourceIdentity(src),
	).Scan(&realRows); err != nil {
		t.Fatalf("count real rows: %v", err)
	}
	if realRows != 1 {
		t.Errorf("real row for session with real source = %d, want 1", realRows)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_sources WHERE session_id='legacy-only' AND source_db='' "); got != "1" {
		t.Errorf("legacy-only row = %s, want 1", got)
	}
}

func TestConsolidateFrom_PreservesLegacySourceWhenCoContributorSkipped(t *testing.T) {
	isolateCache(t)
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	if err := EnsureSchema(con, sourceClaude); err != nil {
		con.Close()
		t.Fatalf("ensure schema: %v", err)
	}
	// Seed legacy consolidated store with a session before session_sources exists.
	if _, err := con.Exec(`
		INSERT INTO sessions (
			id, started_at, last_ts, message_count, is_subagent, parent_id,
			origin_machine, source_tool, source_path, only_copy_since, project, cwd
		) VALUES ('shared-s', 100, 200, 1, 0, NULL, 'machine', 'claude', '/tmp/shared-s.jsonl', NULL, 'project', '/tmp')
	`); err != nil {
		con.Close()
		t.Fatalf("seed session: %v", err)
	}
	if _, err := con.Exec(`
		INSERT INTO messages (session_id, role, content, ts, ts_iso, uuid)
		VALUES ('shared-s', 'user', 'legacy message', 100, '2026-01-01T00:00:00Z', 'u-leg')
	`); err != nil {
		con.Close()
		t.Fatalf("seed message: %v", err)
	}
	con.Close()

	// Create good.db (current schema version) containing shared-s.
	good := seedSessionDB(t, "good.db", sessionRow{
		id: "shared-s", project: "good-proj", cwd: "/tmp/good",
		msgs: []msgRow{{"u-good", "user", "good message", 100}},
	})

	// Create behind.db (behind version schema) containing shared-s.
	behind := rewindScopeColumns(t, seedSessionDB(t, "behind.db", sessionRow{
		id: "shared-s", project: "behind-proj", cwd: "/tmp/behind",
		msgs: []msgRow{{"u-behind", "user", "behind message", 100}},
	}))
	behindCon, err := store.ConnectRW(behind)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := behindCon.Exec("UPDATE meta SET value=? WHERE key='schema_version'", store.SchemaVersion-1); err != nil {
		behindCon.Close()
		t.Fatalf("rewind schema version: %v", err)
	}
	behindCon.Close()

	// Consolidate both sources. behind.db will be skipped due to schema version.
	st, err := ConsolidateFrom([]string{good, behind}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}
	if st.Skipped != 1 {
		t.Fatalf("st.Skipped = %d, want 1", st.Skipped)
	}

	con = openConsolidated(t)
	// Because behind.db was skipped, legacy provenance for shared-s MUST NOT be pruned.
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_sources WHERE session_id='shared-s' AND source_db='' "); got != "1" {
		t.Errorf("legacy row for session with skipped co-contributor = %s, want 1", got)
	}
	con.Close()

	// Now good.db drops shared-s (e.g. session was pruned from good project).
	goodCon, err := store.ConnectRW(good)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goodCon.Exec("DELETE FROM messages WHERE session_id='shared-s'"); err != nil {
		goodCon.Close()
		t.Fatal(err)
	}
	if _, err := goodCon.Exec("DELETE FROM sessions WHERE id='shared-s'"); err != nil {
		goodCon.Close()
		t.Fatal(err)
	}
	goodCon.Close()

	// Sync good.db into consolidated store.
	if err := SyncConsolidatedFrom(good); err != nil {
		t.Fatalf("SyncConsolidatedFrom: %v", err)
	}

	// shared-s must still survive because behind.db holds it and legacy provenance protected it.
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions WHERE id='shared-s'"); got != "1" {
		t.Errorf("session count for shared-s = %s, want 1 (session must survive drop from good.db because skipped behind.db holds it)", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM messages WHERE session_id='shared-s'"); got == "0" {
		t.Errorf("message count for shared-s = 0, want > 0")
	}
	con.Close()

	// Later: behind.db is reindexed to current schema.
	behind = seedSessionDB(t, "behind.db", sessionRow{
		id: "shared-s", project: "behind-proj", cwd: "/tmp/behind",
		msgs: []msgRow{{"u-behind", "user", "behind message", 100}},
	})

	// Re-run ConsolidateFrom with no skipped sources.
	st, err = ConsolidateFrom([]string{good, behind}, false)
	if err != nil {
		t.Fatalf("ConsolidateFrom after upgrade: %v", err)
	}
	if st.Skipped != 0 {
		t.Fatalf("st.Skipped after upgrade = %d, want 0", st.Skipped)
	}

	con = openConsolidated(t)
	// Now that all sources folded cleanly, legacy row is pruned and behind.db's real row is recorded.
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_sources WHERE session_id='shared-s' AND source_db='' "); got != "0" {
		t.Errorf("legacy row after full pass without skips = %s, want 0", got)
	}
	var behindRows int
	if err := con.QueryRow("SELECT COUNT(*) FROM session_sources WHERE session_id='shared-s' AND source_db=?", sourceIdentity(behind)).Scan(&behindRows); err != nil {
		t.Fatalf("count behind rows: %v", err)
	}
	if behindRows != 1 {
		t.Errorf("behind row count = %d, want 1", behindRows)
	}
	con.Close()

	// Finally, behind.db drops shared-s.
	behindCon, err = store.ConnectRW(behind)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := behindCon.Exec("DELETE FROM messages WHERE session_id='shared-s'"); err != nil {
		behindCon.Close()
		t.Fatal(err)
	}
	if _, err := behindCon.Exec("DELETE FROM sessions WHERE id='shared-s'"); err != nil {
		behindCon.Close()
		t.Fatal(err)
	}
	behindCon.Close()

	if err := SyncConsolidatedFrom(behind); err != nil {
		t.Fatalf("SyncConsolidatedFrom behind: %v", err)
	}

	// Now that no source holds shared-s, it is cleanly pruned.
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions WHERE id='shared-s'"); got != "0" {
		t.Errorf("session count for shared-s after all sources dropped = %s, want 0", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM messages WHERE session_id='shared-s'"); got != "0" {
		t.Errorf("message count for shared-s after all sources dropped = %s, want 0", got)
	}
	con.Close()
}

func TestConsolidateFrom_HealFailureAbortsBeforeBackfill(t *testing.T) {
	isolateCache(t)
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	if err := EnsureSchema(con, sourceClaude); err != nil {
		con.Close()
		t.Fatalf("ensure schema: %v", err)
	}
	if _, err := con.Exec("INSERT INTO sessions(id, source_path, message_count) VALUES('heal-failure', '/tmp/heal.jsonl', 1)"); err != nil {
		con.Close()
		t.Fatalf("seed session: %v", err)
	}
	if _, err := con.Exec("INSERT INTO meta(key, value) VALUES('sync:old.db', 'old-mark')"); err != nil {
		con.Close()
		t.Fatalf("seed watermark: %v", err)
	}
	if _, err := con.Exec(`
		CREATE TRIGGER inject_heal_failure
		BEFORE DELETE ON meta
		WHEN OLD.key LIKE 'sync:%'
		BEGIN
			SELECT RAISE(ABORT, 'injected heal failure');
		END
	`); err != nil {
		con.Close()
		t.Fatalf("install heal failure trigger: %v", err)
	}
	con.Close()

	if _, err := ConsolidateFrom(nil, false); err == nil || !strings.Contains(err.Error(), "heal upgraded store") {
		t.Fatalf("ConsolidateFrom error = %v, want heal failure", err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_sources"); got != "0" {
		t.Errorf("session_sources after failed heal = %s, want 0", got)
	}
	con.Close()

	con, err = store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("reopen consolidated: %v", err)
	}
	if _, err := con.Exec("DROP TRIGGER inject_heal_failure"); err != nil {
		con.Close()
		t.Fatalf("remove heal failure trigger: %v", err)
	}
	con.Close()
	if _, err := ConsolidateFrom(nil, false); err != nil {
		t.Fatalf("ConsolidateFrom after heal failure: %v", err)
	}
	con = openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM session_sources WHERE session_id='heal-failure'"); got != "1" {
		t.Errorf("session_sources after retry = %s, want 1", got)
	}
}

// TestConsolidate_RollsBackA midFoldFailure verifies that a failure after the
// session merge does not leave provenance or session rows committed ahead of
// the message merge.
func TestConsolidate_RollsBackAMidFoldFailure(t *testing.T) {
	isolateCache(t)
	src := seedSessionDB(t, "mid-fold.db", sessionRow{
		id:      "mid-fold-session",
		project: "ledger",
		cwd:     "/w/ledger",
		msgs:    []msgRow{{"mid-fold-message", "user", "hello", 100}},
	})

	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	if err := EnsureSchema(con, sourceClaude); err != nil {
		con.Close()
		t.Fatalf("ensure schema: %v", err)
	}
	if _, err := con.Exec(`
		CREATE TRIGGER abort_consolidated_messages
		BEFORE INSERT ON messages
		BEGIN
			SELECT RAISE(ABORT, 'injected mid-fold failure');
		END
	`); err != nil {
		con.Close()
		t.Fatalf("install mid-fold failure trigger: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close consolidated: %v", err)
	}

	if _, err := ConsolidateFrom([]string{src}, false); err == nil || !strings.Contains(err.Error(), "merge messages") {
		t.Fatalf("ConsolidateFrom error = %v, want injected message merge failure", err)
	}

	con = openConsolidated(t)
	for _, tc := range []struct {
		name  string
		query string
	}{
		{name: "session sources", query: "SELECT COUNT(*) FROM session_sources WHERE session_id='mid-fold-session'"},
		{name: "sessions", query: "SELECT COUNT(*) FROM sessions WHERE id='mid-fold-session'"},
		{name: "messages", query: "SELECT COUNT(*) FROM messages WHERE session_id='mid-fold-session'"},
		{name: "file index", query: "SELECT COUNT(*) FROM file_index WHERE session_id='mid-fold-session'"},
		{name: "watermark", query: "SELECT COUNT(*) FROM meta WHERE key LIKE 'sync:%'"},
	} {
		if got := scalar(t, con, tc.query); got != "0" {
			t.Errorf("%s after failed fold = %s, want 0", tc.name, got)
		}
	}
}

// TestConsolidate_FaultInjectionHelper is the child half of
// TestConsolidate_RetryAfterAbruptPostMergeExit. It must exit from the merge
// phase defer so the enclosing DETACH and connection cleanup defers do not run.
func TestConsolidate_FaultInjectionHelper(t *testing.T) {
	if os.Getenv("RAWCLAW_CONSOLIDATE_FAULT_CHILD") != "1" {
		return
	}
	src := os.Getenv("RAWCLAW_CONSOLIDATE_FAULT_SOURCE")
	if src == "" {
		t.Fatal("missing RAWCLAW_CONSOLIDATE_FAULT_SOURCE")
	}
	home := os.Getenv("RAWCLAW_CONSOLIDATE_FAULT_HOME")
	if home == "" {
		t.Fatal("missing RAWCLAW_CONSOLIDATE_FAULT_HOME")
	}
	// TestMain gives every test process its own HOME. Restore the parent's
	// isolated home here so the child and parent truly share one store.
	t.Setenv("HOME", home)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	consolidateAfterMergeHook = func() { os.Exit(124) }
	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatalf("fault-injected consolidation: %v", err)
	}
	t.Fatal("fault injection did not exit")
}

// TestConsolidate_RetryAfterAbruptPostMergeExit models issue #32's kill-then-
// retry sequence. The child exits after the merge timing log and before the
// DETACH defer; the parent then folds the same source and records the timing.
func TestConsolidate_RetryAfterAbruptPostMergeExit(t *testing.T) {
	home := isolateCache(t)
	src := seedSessionDB(t, "fault-repro.db", sessionRow{
		id: "fault-repro-session", project: "ledger", cwd: "/w/ledger",
		msgs: []msgRow{{"fault-repro-message", "user", "reproduce the retry", 100}},
	})

	cmd := exec.Command(os.Args[0], "-test.run", "^TestConsolidate_FaultInjectionHelper$", "-test.v")
	cmd.Env = append(os.Environ(),
		"RAWCLAW_CONSOLIDATE_FAULT_CHILD=1",
		"RAWCLAW_CONSOLIDATE_FAULT_SOURCE="+src,
		"RAWCLAW_CONSOLIDATE_FAULT_HOME="+home,
	)
	childOutput, childErr := cmd.CombinedOutput()
	if childErr == nil {
		t.Fatalf("fault child exited successfully; output:\n%s", childOutput)
	}
	exitErr, ok := childErr.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 124 {
		t.Fatalf("fault child error = %v, want exit status 124; output:\n%s", childErr, childOutput)
	}
	if !strings.Contains(string(childOutput), "phase=merge duration=") {
		t.Fatalf("fault child output has no merge completion log:\n%s", childOutput)
	}
	if strings.Contains(string(childOutput), "phase=detach") {
		t.Fatalf("fault child ran DETACH after forced exit:\n%s", childOutput)
	}
	t.Logf("fault child output:\n%s", childOutput)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := ConsolidatedPath() + suffix
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Logf("post-exit artifact %s: absent (%v)", filepath.Base(path), statErr)
			continue
		}
		t.Logf("post-exit artifact %s: present size=%d", filepath.Base(path), info.Size())
	}
	lockPath := filepath.Join(store.CacheDir(), "consolidated.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("post-exit consolidated lock artifact missing: %v", err)
	}
	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions WHERE id='fault-repro-session'"); got != "1" {
		t.Fatalf("post-exit committed session count = %s, want 1", got)
	}
	if got := scalar(t, con, "SELECT COUNT(*) FROM meta WHERE key LIKE 'sync:%'"); got != "1" {
		t.Fatalf("post-exit sync watermark count = %s, want 1", got)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close post-exit assertion connection: %v", err)
	}

	// Change the same source after the child committed. An unchanged source
	// would be a watermark no-op and would make the retry timing meaningless.
	sourceCon, err := store.ConnectRW(src)
	if err != nil {
		t.Fatalf("open source for retry mutation: %v", err)
	}
	if _, err := sourceCon.Exec(
		"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
		"fault-repro-session", "assistant", "retry must fold this new row", 200, "", "fault-repro-retry-message",
	); err != nil {
		sourceCon.Close()
		t.Fatalf("append source message: %v", err)
	}
	if _, err := sourceCon.Exec(
		"UPDATE sessions SET last_ts=200, message_count=2 WHERE id=?", "fault-repro-session",
	); err != nil {
		sourceCon.Close()
		t.Fatalf("update source message count: %v", err)
	}
	if err := sourceCon.Close(); err != nil {
		t.Fatalf("close mutated source: %v", err)
	}

	started := time.Now()
	st, err := ConsolidateFrom([]string{src}, false)
	if err != nil {
		t.Fatalf("retry consolidation: %v", err)
	}
	if st.Messages != 2 {
		t.Fatalf("retry consolidated messages = %d, want 2; retry may have been a watermark no-op", st.Messages)
	}
	t.Logf("retry after post-merge exit completed in %s", time.Since(started))
}

// TestPruneTombstoned_UnderscoreIdDoesNotDeleteNeighbour covers issue #15: the
// tombstone prune built a LIKE pattern by concatenating a session id, so an id
// containing SQLite's single-char wildcard (_) matched NEIGHBOURING ids and
// deleted their rows from messages/sessions/session_sources/file_index.
func TestPruneTombstoned_UnderscoreIdDoesNotDeleteNeighbour(t *testing.T) {
	isolateCache(t)
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	// "vic_im" is tombstoned. "vicXim" is an UNRELATED live session whose id
	// differs only where the wildcard would match. Its subagent thread is the
	// row that an unescaped LIKE "vic_im/%" wrongly sweeps up.
	for _, id := range []string{"vic_im", "vic_im/sub", "vicXim/sub"} {
		if _, err := con.Exec(`
			INSERT INTO sessions (
				id, started_at, last_ts, message_count, is_subagent, parent_id,
				origin_machine, source_tool, source_path, only_copy_since, project, cwd
			) VALUES (?, 100, 200, 1, 1, NULL, 'm', 'claude', '/tmp/x.jsonl', NULL, 'p', '/tmp')
		`, id); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	if err := pruneTombstonedIDs(con, []string{"vic_im"}); err != nil {
		t.Fatalf("prune: %v", err)
	}

	var gone int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='vic_im/sub'").Scan(&gone); err != nil {
		t.Fatalf("query tombstoned: %v", err)
	}
	if gone != 0 {
		t.Errorf("tombstoned subagent survived = %d, want 0", gone)
	}

	var neighbour int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='vicXim/sub'").Scan(&neighbour); err != nil {
		t.Fatalf("query neighbour: %v", err)
	}
	if neighbour != 1 {
		t.Errorf("UNRELATED neighbour session deleted: count = %d, want 1", neighbour)
	}
}

func TestPruneTombstonedIDs_SkipsMissingIDsQuicklyAndPrunesExistingThreads(t *testing.T) {
	isolateCache(t)
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("ensure topic schema: %v", err)
	}

	for _, id := range []string{"victim", "victim/agent-1"} {
		if _, err := con.Exec(`INSERT INTO sessions (id) VALUES (?)`, id); err != nil {
			t.Fatalf("seed session %s: %v", id, err)
		}
		if _, err := con.Exec(`INSERT INTO messages (session_id, uuid) VALUES (?, ?)`, id, id+"-u"); err != nil {
			t.Fatalf("seed message %s: %v", id, err)
		}
		if _, err := con.Exec(`INSERT INTO session_sources (session_id, source_db) VALUES (?, ?)`, id, "source.db"); err != nil {
			t.Fatalf("seed source %s: %v", id, err)
		}
		if _, err := con.Exec(`INSERT INTO file_index (path, session_id) VALUES (?, ?)`, "/tmp/"+id, id); err != nil {
			t.Fatalf("seed file %s: %v", id, err)
		}
		if _, err := con.Exec(`INSERT INTO topic_segment (session_id, start_uuid) VALUES (?, ?)`, id, id+"-u"); err != nil {
			t.Fatalf("seed topic %s: %v", id, err)
		}
		if _, err := con.Exec(`INSERT INTO session_verdict (session_id, verdict, source) VALUES (?, 'routine', 'floor')`, id); err != nil {
			t.Fatalf("seed verdict %s: %v", id, err)
		}
	}

	bulk, err := con.Begin()
	if err != nil {
		t.Fatalf("begin bulk seed: %v", err)
	}
	for i := range 20000 {
		id := fmt.Sprintf("unrelated-%d", i)
		if _, err := bulk.Exec(`INSERT INTO messages (session_id, uuid) VALUES (?, ?)`, id, id+"-u"); err != nil {
			bulk.Rollback()
			t.Fatalf("seed unrelated message: %v", err)
		}
	}
	if err := bulk.Commit(); err != nil {
		t.Fatalf("commit bulk seed: %v", err)
	}

	ids := make([]string, 2000)
	for i := range ids {
		ids[i] = fmt.Sprintf("missing-%d", i)
	}
	ids = append(ids, "victim")
	started := time.Now()
	if err := pruneTombstonedIDs(con, ids); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("pruning mostly missing IDs took %s", elapsed)
	}

	for _, query := range []string{
		"SELECT COUNT(*) FROM messages WHERE session_id LIKE 'victim%'",
		"SELECT COUNT(*) FROM sessions WHERE id LIKE 'victim%'",
		"SELECT COUNT(*) FROM session_sources WHERE session_id LIKE 'victim%'",
		"SELECT COUNT(*) FROM file_index WHERE session_id LIKE 'victim%'",
		"SELECT COUNT(*) FROM topic_segment WHERE session_id LIKE 'victim%'",
		"SELECT COUNT(*) FROM session_verdict WHERE session_id LIKE 'victim%'",
	} {
		if got := scalar(t, con, query); got != "0" {
			t.Errorf("rows survived query %q: %s", query, got)
		}
	}
}

// TestConsolidateFrom_RebuildFailureLeavesLiveStoreIntact covers issue #18: a
// rebuild deleted the consolidated store BEFORE connecting, so any later
// failure left the user with no searchable store at all until another full
// rebuild happened to succeed. The replacement is now built beside the live
// store and swapped only once it is complete.
func TestConsolidateFrom_RebuildFailureLeavesLiveStoreIntact(t *testing.T) {
	isolateCache(t)
	dst := ConsolidatedPath()

	// Stand up a live store holding one session.
	con, err := store.ConnectRW(dst)
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	if err := EnsureSchema(con, sourceClaude); err != nil {
		con.Close()
		t.Fatalf("ensure schema: %v", err)
	}
	if _, err := con.Exec(`
		INSERT INTO sessions (
			id, started_at, last_ts, message_count, is_subagent, parent_id,
			origin_machine, source_tool, source_path, only_copy_since, project, cwd
		) VALUES ('precious', 100, 200, 1, 0, NULL, 'm', 'claude', '/tmp/p.jsonl', NULL, 'proj', '/tmp')
	`); err != nil {
		con.Close()
		t.Fatalf("seed live session: %v", err)
	}
	con.Close()

	// Rebuild from a source path that cannot be read. The fold must fail.
	// A missing source is a real error ("source unreadable"), so pin that
	// contract here rather than only logging it. The survival assertions below
	// would still catch a silent success, but this names WHICH guarantee broke.
	if _, err := ConsolidateFrom([]string{filepath.Join(t.TempDir(), "does-not-exist.db")}, true); err == nil {
		t.Errorf("rebuild from a missing source returned no error, want one")
	}

	// The live store must still be there, with its session.
	after, err := store.ConnectRW(dst)
	if err != nil {
		t.Fatalf("live store unreadable after a failed rebuild: %v", err)
	}
	defer after.Close()
	var n int
	if err := after.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='precious'").Scan(&n); err != nil {
		t.Fatalf("query live store after failed rebuild: %v", err)
	}
	if n != 1 {
		t.Errorf("session lost to a failed rebuild: count = %d, want 1", n)
	}
}

// TestConsolidateRebuild_CarriesForwardStoreOnlySessions is issue #22's test: a
// session that survives ONLY in the consolidated store — its per-project db
// gone along with its transcript — must outlive a --rebuild. The sessions
// absent from every source are exactly the purged history the store exists to
// keep, and before the carry-forward a rebuild dropped them silently.
func TestConsolidateRebuild_CarriesForwardStoreOnlySessions(t *testing.T) {
	isolateCache(t)
	keep := seedSessionDB(t, "-w-keep.db", sessionRow{
		id: "kept", project: "keep", cwd: "/w/keep",
		msgs: []msgRow{{"u-k1", "user", "still has a live index", 100}},
	})
	gone := seedSessionDB(t, "-w-gone.db", sessionRow{
		id: "orphan", project: "gone", cwd: "/w/gone", missing: 200,
		msgs: []msgRow{{"u-g1", "user", "index and transcript both purged", 100}},
	})
	if _, err := ConsolidateFrom([]string{keep, gone}, false); err != nil {
		t.Fatal(err)
	}
	// The orphan's source db vanishes — the state after a cache wipe, or a
	// per-project rebuild from a live tree whose transcript was purged.
	for _, sfx := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(gone + sfx)
	}

	st, err := ConsolidateFrom([]string{keep}, true)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if st.CarriedForward != 1 {
		t.Errorf("CarriedForward = %d, want 1", st.CarriedForward)
	}

	con := openConsolidated(t)
	if got := scalar(t, con, "SELECT COUNT(*) FROM sessions"); got != "2" {
		t.Fatalf("rebuild kept %s sessions, want 2 — the store-only session was dropped", got)
	}
	if got := scalar(t, con, "SELECT content FROM messages WHERE session_id='orphan'"); got != "index and transcript both purged" {
		t.Errorf("orphan message = %q — the carried session lost its body", got)
	}
	if got := scalar(t, con, "SELECT only_copy_since FROM sessions WHERE id='orphan'"); !strings.HasPrefix(got, "200") {
		t.Errorf("orphan only_copy_since = %s, want 200 — the retention flag did not ride along", got)
	}
	if got := scalar(t, con, "SELECT source_db FROM session_sources WHERE session_id='orphan'"); got != "" {
		t.Errorf("orphan source_db = %q, want the legacy-baseline '' row", got)
	}
	// Present but unfindable would be a quieter version of the same loss: the
	// carried message must be in the FTS index too.
	if got := scalar(t, con, "SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'purged'"); got != "1" {
		t.Errorf("FTS matched %s rows for the carried message, want 1", got)
	}
}

// TestConsolidate_CurrentIdentityFormWinsMetadataTie is issue #19's test: when
// a legacy bare-filename contribution and a current absolute-path contribution
// tie on presence, message count, and last_ts, the current form must win the
// merge. In ASCII a letter outranks '/' on a bare DESC sort, so without the
// form-first ordering the legacy row's stale project/cwd is displayed forever.
func TestConsolidate_CurrentIdentityFormWinsMetadataTie(t *testing.T) {
	isolateCache(t)
	src := seedSessionDB(t, "-w-current.db", sessionRow{
		id: "tied", project: "current-proj", cwd: "/w/current",
		msgs: []msgRow{{"u-t1", "user", "hello", 100}},
	})
	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatal(err)
	}

	// Inject the legacy contribution by hand: bare-filename identity, stale
	// scope, and values that tie with the real fold on every rank above the
	// identity — then clear the watermark so the next pass re-merges.
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec(`
		INSERT INTO session_sources
		  (session_id,source_db,started_at,last_ts,message_count,is_subagent,parent_id,
		   origin_machine,source_tool,source_path,only_copy_since,project,cwd)
		VALUES('tied','w-current.db',100,100,1,0,NULL,'m',?, '/t/tied.jsonl',NULL,'stale-proj','/w/stale')
	`, sourceClaude); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("DELETE FROM meta WHERE key LIKE 'sync:%'"); err != nil {
		t.Fatal(err)
	}
	if err := con.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := ConsolidateFrom([]string{src}, false); err != nil {
		t.Fatal(err)
	}
	ro := openConsolidated(t)
	if got := scalar(t, ro, "SELECT project FROM sessions WHERE id='tied'"); got != "current-proj" {
		t.Errorf("merged project = %q, want current-proj — the legacy bare-name contribution won the tie (#19)", got)
	}
	if got := scalar(t, ro, "SELECT cwd FROM sessions WHERE id='tied'"); got != "/w/current" {
		t.Errorf("merged cwd = %q, want /w/current", got)
	}
}
