package store_test

import (
	"database/sql"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// pragmaHasColumn reports whether table carries column, via PRAGMA table_info.
func pragmaHasColumn(t *testing.T, con *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := con.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("pragma table_info(%s): %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid, notNull, pk int
			name, typ        string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}

// metaValue reads a meta key ("" if absent).
func metaValue(t *testing.T, con *sql.DB, key string) string {
	t.Helper()
	var v string
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", key).Scan(&v); err != nil {
		return ""
	}
	return v
}

// mustTopic ensures the v2 topic schema on a fresh test db.
func mustTopic(t *testing.T, con *sql.DB) {
	t.Helper()
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
}

// mustUpsertSeg authors one segment (start uuid = end uuid for brevity). The
// origin arg documents intent at the call site but the authoring path leaves
// origin_machine NULL (this machine) — only the ingest path stamps origin.
func mustUpsertSeg(t *testing.T, con *sql.DB, sid, startUUID, topic, origin string) {
	t.Helper()
	_ = origin
	if err := store.UpsertTopicSegment(con, sid, startUUID, startUUID, topic, "", 1); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
}

// startUUIDs returns a session's segment start uuids in id order.
func startUUIDs(t *testing.T, con *sql.DB, sid string) []string {
	t.Helper()
	rows, err := con.Query("SELECT start_uuid FROM topic_segment WHERE session_id=? ORDER BY id", sid)
	if err != nil {
		t.Fatalf("read start_uuids: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatalf("scan start_uuid: %v", err)
		}
		out = append(out, u)
	}
	return out
}

// --- fresh v2 schema ---

func TestEnsureTopicSchema_FreshHasV2Shape(t *testing.T) {
	con, _ := storetest.NewDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	// topic_segment carries origin_machine.
	if !pragmaHasColumn(t, con, "topic_segment", "origin_machine") {
		t.Error("fresh topic_segment missing origin_machine column")
	}
	// session_verdict table exists (a select against it must not error).
	if _, err := con.Exec("SELECT session_id FROM session_verdict LIMIT 1"); err != nil {
		t.Errorf("session_verdict missing on fresh schema: %v", err)
	}
	if v := metaValue(t, con, "topic_schema_version"); v != "2" {
		t.Errorf("topic_schema_version = %q, want 2", v)
	}
}

// --- v1 -> v2 in-place migration ---

func TestEnsureTopicSchema_MigratesV1(t *testing.T) {
	con, _ := storetest.NewDB(t)
	// Hand-write the LEGACY v1 topic_segment (no origin_machine, no session_verdict)
	// and stamp version 1 — the migration test owns its raw DDL.
	legacy := `
CREATE TABLE topic_segment (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL, start_uuid TEXT NOT NULL, end_uuid TEXT,
  topic TEXT, summary TEXT, tagged_at REAL,
  UNIQUE(session_id, start_uuid)
);
INSERT INTO topic_segment(session_id,start_uuid,topic,summary,tagged_at)
VALUES('s1','u1','old topic','sum',10.0);
INSERT OR REPLACE INTO meta(key,value) VALUES('topic_schema_version','1');`
	if _, err := con.Exec(legacy); err != nil {
		t.Fatalf("seed legacy v1 schema: %v", err)
	}

	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema (migrate): %v", err)
	}

	if !pragmaHasColumn(t, con, "topic_segment", "origin_machine") {
		t.Error("migration did not add topic_segment.origin_machine")
	}
	if _, err := con.Exec("SELECT session_id FROM session_verdict LIMIT 1"); err != nil {
		t.Errorf("migration did not create session_verdict: %v", err)
	}
	if v := metaValue(t, con, "topic_schema_version"); v != "2" {
		t.Errorf("post-migration version = %q, want 2", v)
	}
	// The legacy row survives (migration is additive), origin_machine NULL.
	var (
		topic  string
		origin interface{}
	)
	if err := con.QueryRow(
		"SELECT topic, origin_machine FROM topic_segment WHERE session_id='s1' AND start_uuid='u1'",
	).Scan(&topic, &origin); err != nil {
		t.Fatalf("read migrated row: %v", err)
	}
	if topic != "old topic" {
		t.Errorf("migrated topic = %q, want %q", topic, "old topic")
	}
	if origin != nil {
		t.Errorf("migrated origin_machine = %v, want NULL", origin)
	}

	// Idempotent: a second call is a clean no-op.
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("second EnsureTopicSchema: %v", err)
	}
}

// --- atomic session-unit replace (no franken-set) ---

func TestReplaceSessionSegments_AtomicNoFrankenset(t *testing.T) {
	con, _ := storetest.NewDB(t)
	mustTopic(t, con)

	// Authoring tag: two segments {u1,u2}.
	mustUpsertSeg(t, con, "s1", "u1", "topic A", "box-a")
	mustUpsertSeg(t, con, "s1", "u2", "topic B", "box-a")

	// An ingest from another authoring with a DIFFERENT boundary set {u3}.
	if err := store.ReplaceSessionSegments(con, "s1", []store.TopicSegment{
		{SessionID: "s1", StartUUID: "u3", Topic: "topic C", TaggedAt: 5, OriginMachine: "box-b"},
	}); err != nil {
		t.Fatalf("ReplaceSessionSegments: %v", err)
	}

	// Only the incoming set remains — u1,u2 are gone (no interleaved franken-set).
	got := startUUIDs(t, con, "s1")
	if len(got) != 1 || got[0] != "u3" {
		t.Errorf("after replace, start_uuids = %v, want [u3]", got)
	}
}

// --- real-beats-routine read resolution ---

func TestIsEffectivelyRoutine_RealTagDemotesRoutine(t *testing.T) {
	con, _ := storetest.NewDB(t)
	mustTopic(t, con)

	// Floor marks s1 routine.
	if err := store.UpsertVerdict(con, store.Verdict{
		SessionID: "s1", Verdict: store.VerdictRoutine, Source: store.VerdictSourceFloor,
		OriginMachine: "box-a", TaggedAt: 1,
	}); err != nil {
		t.Fatalf("UpsertVerdict: %v", err)
	}
	if r, _ := store.IsEffectivelyRoutine(con, "s1"); !r {
		t.Error("routine verdict with no real segment should be effectively routine")
	}

	// A real topic segment arrives — it beats routine.
	mustUpsertSeg(t, con, "s1", "u1", "real topic", "box-a")
	if r, _ := store.IsEffectivelyRoutine(con, "s1"); r {
		t.Error("a real topic segment must demote the routine verdict (real beats routine)")
	}
}

// --- verdict LWW (tie = latest tagged_at) ---

func TestMergeVerdict_LWW(t *testing.T) {
	con, _ := storetest.NewDB(t)
	mustTopic(t, con)

	base := store.Verdict{SessionID: "s1", Verdict: store.VerdictRoutine, Source: store.VerdictSourceFloor, OriginMachine: "box-a", TaggedAt: 10}
	if err := store.UpsertVerdict(con, base); err != nil {
		t.Fatalf("seed verdict: %v", err)
	}

	// Older incoming = no-op.
	_ = store.MergeVerdict(con, store.Verdict{SessionID: "s1", Verdict: store.VerdictRoutine, Source: store.VerdictSourceAgent, OriginMachine: "box-b", TaggedAt: 5})
	if v, _, _ := store.VerdictFor(con, "s1"); v.OriginMachine != "box-a" {
		t.Errorf("older merge changed origin to %q, want mac (no-op)", v.OriginMachine)
	}

	// Newer incoming wins.
	_ = store.MergeVerdict(con, store.Verdict{SessionID: "s1", Verdict: store.VerdictRoutine, Source: store.VerdictSourceAgent, OriginMachine: "box-b", TaggedAt: 20})
	if v, _, _ := store.VerdictFor(con, "s1"); v.OriginMachine != "box-b" || v.TaggedAt != 20 {
		t.Errorf("newer merge = (%s,%v), want (box-b,20)", v.OriginMachine, v.TaggedAt)
	}

	// Equal tagged_at, higher origin_machine wins (deterministic tie-break).
	_ = store.MergeVerdict(con, store.Verdict{SessionID: "s1", Verdict: store.VerdictRoutine, Source: store.VerdictSourceFloor, OriginMachine: "zeta", TaggedAt: 20})
	if v, _, _ := store.VerdictFor(con, "s1"); v.OriginMachine != "zeta" {
		t.Errorf("equal-tie merge origin = %q, want zeta (higher)", v.OriginMachine)
	}
}
