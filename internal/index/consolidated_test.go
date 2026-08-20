package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// isolateCache points HOME at a temp dir so ConsolidatedPath, the per-project
// dbs, and the tombstone sidecar all land under the test's own tree. The XDG
// data dir is pinned under the same tree, or the durable transcript vault would
// keep resolving to whatever XDG_DATA_HOME the process inherited and every test
// would share one vault.
func isolateCache(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
}

// indexProject writes a one-file project dir, indexes it to its own cache db
// the way a real run does, and returns that db's path. The transcript is named
// after the project so the derived session ids stay distinct — a shared stem
// would make two projects' sessions one session, which is a different test.
func indexProject(t *testing.T, name string, lines ...string) string {
	t.Helper()
	proj := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(proj, strings.TrimPrefix(name, "-")+".jsonl"), lines...)
	dbp, _, _, err := EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("index %s: %v", name, err)
	}
	return dbp
}

// openConsolidated opens the consolidated store read-only for assertions.
func openConsolidated(t *testing.T) *sql.DB {
	t.Helper()
	con, err := store.ConnectRO(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	t.Cleanup(func() { con.Close() })
	return con
}

// scalar runs a single-value query and returns it as a string, so one helper
// serves counts and text columns alike.
func scalar(t *testing.T, con *sql.DB, q string, args ...any) string {
	t.Helper()
	var v sql.NullString
	if err := con.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	if !v.Valid {
		return "<NULL>"
	}
	return v.String
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
	if got := scalar(t, con, "SELECT COALESCE(missing_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Errorf("missing_since = %s, want NULL — a session present in ANY copy is present", got)
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
	if got := scalar(t, con, "SELECT COALESCE(missing_since,'<NULL>') FROM sessions WHERE id=?", id); got != "<NULL>" {
		t.Errorf("missing_since = %s, want NULL — a later stale copy re-flagged a present session", got)
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

// rewindScopeColumns puts a db back in the shape it had before the scope
// migration: current schema version, no project/cwd columns.
func rewindScopeColumns(t *testing.T, dbp string) string {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	for _, c := range scopeColumns {
		if _, err := con.Exec("ALTER TABLE sessions DROP COLUMN " + c.name); err != nil {
			t.Fatalf("rewind %s: %v", c.name, err)
		}
	}
	// Clear the backfill stamp too — a rewound db has never been backfilled.
	if _, err := con.Exec("DELETE FROM meta WHERE key=?", scopeBackfillKey); err != nil {
		t.Fatalf("clear backfill stamp: %v", err)
	}
	return dbp
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

// --- fixtures -------------------------------------------------------------

// msgRow is one message in a seeded source db.
type msgRow struct {
	uuid, role, content string
	ts                  float64
}

// sessionRow is one session in a seeded source db. missing=0 means present.
type sessionRow struct {
	id, project, cwd string
	missing          float64
	msgs             []msgRow
}

// seedSessionDB writes a per-project cache db directly, bypassing the
// transcript reader. The cases that matter here — the same session id in two
// dbs, repeated uuids, a pre-migration schema — are states of the STORE, and
// building them from rows says exactly that instead of hiding it behind
// transcript fixtures that have to be reverse-engineered to read.
func seedSessionDB(t *testing.T, name string, rows ...sessionRow) string {
	t.Helper()
	dbp := filepath.Join(t.TempDir(), name)
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("schema %s: %v", name, err)
	}
	for _, r := range rows {
		var missing any
		if r.missing != 0 {
			missing = r.missing
		}
		var started, last float64
		for i, m := range r.msgs {
			if i == 0 || m.ts < started {
				started = m.ts
			}
			if m.ts > last {
				last = m.ts
			}
			if _, err := con.Exec(
				"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
				r.id, m.role, m.content, m.ts, "", m.uuid,
			); err != nil {
				t.Fatalf("seed message in %s: %v", name, err)
			}
		}
		if _, err := con.Exec(
			`INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,
			 origin_machine,source_tool,source_path,missing_since,project,cwd)
			 VALUES(?,?,?,?,0,NULL,'m',?,?,?,?,?)`,
			r.id, started, last, len(r.msgs), sourceClaude, "/t/"+r.id+".jsonl", missing, r.project, r.cwd,
		); err != nil {
			t.Fatalf("seed session in %s: %v", name, err)
		}
	}
	return dbp
}

// tagSession writes one topic segment into a per-project db, the way the
// tagging path does, so the consolidation tests have a topic layer to fold.
func tagSession(t *testing.T, dbp, sessionID, startUUID, topic, summary string, taggedAt float64) {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open %s for tagging: %v", dbp, err)
	}
	defer con.Close()
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("ensure topic schema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, sessionID, startUUID, "", topic, summary, taggedAt); err != nil {
		t.Fatalf("tag %s: %v", sessionID, err)
	}
}

// firstSessionID returns the one session id a single-transcript project db
// holds, so a test can tag it without hardcoding the derived id.
func firstSessionID(t *testing.T, dbp string) string {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", dbp, err)
	}
	defer con.Close()
	return scalar(t, con, "SELECT id FROM sessions LIMIT 1")
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
