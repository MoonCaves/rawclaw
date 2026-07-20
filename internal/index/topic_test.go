package index

import (
	"database/sql"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// addTopicMsg inserts a session (if new) + a message carrying a uuid, matching
// the layout the indexer populates. Returns the message id. Topic helpers key off
// (session_id, uuid), so a uuid is mandatory here.
func addTopicMsg(t *testing.T, con *sql.DB, sid, role, content, uuid string) int {
	t.Helper()
	if _, err := con.Exec(
		"INSERT OR IGNORE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id) VALUES(?,?,?,?,?,?)",
		sid, 0.0, 0.0, 0, 0, nil); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	res, err := con.Exec(
		"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
		sid, role, content, 0.0, "", uuid)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last id: %v", err)
	}
	return int(id)
}

func TestEnsureTopicSchemaIdempotentAndStamped(t *testing.T) {
	con, _ := openTestDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	// Idempotent: a second call (and a third, exercising the version-current path)
	// must not error.
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema (2nd): %v", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema (3rd): %v", err)
	}
	var v string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='topic_schema_version'").Scan(&v); err != nil {
		t.Fatalf("read topic_schema_version: %v", err)
	}
	if v != "2" {
		t.Fatalf("topic_schema_version = %q, want \"2\"", v)
	}
}

func TestUpsertAndTopicsForSession(t *testing.T) {
	con, _ := openTestDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}

	if err := store.UpsertTopicSegment(con, "sess1", "u-start", "u-end", "vector fusion", "RRF blends keyword and vector recall", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
	// Second segment in the same session.
	if err := store.UpsertTopicSegment(con, "sess1", "u-start2", "u-end2", "schema gating", "sidecar tables survive reindex", 2.0); err != nil {
		t.Fatalf("UpsertTopicSegment 2: %v", err)
	}

	segs, err := store.TopicsForSession(con, "sess1")
	if err != nil {
		t.Fatalf("TopicsForSession: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("TopicsForSession = %d segments, want 2", len(segs))
	}
	if segs[0].Topic != "vector fusion" || segs[0].StartUUID != "u-start" || segs[0].EndUUID != "u-end" {
		t.Errorf("seg[0] = %+v, want vector fusion/u-start/u-end", segs[0])
	}
	if segs[1].Topic != "schema gating" {
		t.Errorf("seg[1].Topic = %q, want schema gating", segs[1].Topic)
	}

	// Upsert on the same (session_id, start_uuid) updates in place, not a new row.
	if err := store.UpsertTopicSegment(con, "sess1", "u-start", "u-end3", "vector fusion v2", "updated summary text", 3.0); err != nil {
		t.Fatalf("UpsertTopicSegment update: %v", err)
	}
	segs, _ = store.TopicsForSession(con, "sess1")
	if len(segs) != 2 {
		t.Fatalf("after update: %d segments, want 2 (upsert must not insert a dup)", len(segs))
	}
	if segs[0].Topic != "vector fusion v2" || segs[0].EndUUID != "u-end3" {
		t.Errorf("seg[0] after update = %+v, want vector fusion v2/u-end3", segs[0])
	}
}

func TestMatchTopicsResolvesStartMessage(t *testing.T) {
	con, _ := openTestDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}

	// Two messages in a session; the topic segment starts at the first.
	startID := addTopicMsg(t, con, "sess1", "user", "how do we blend rankings", "uuid-aaa")
	addTopicMsg(t, con, "sess1", "assistant", "use reciprocal rank fusion", "uuid-bbb")

	if err := store.UpsertTopicSegment(con, "sess1", "uuid-aaa", "uuid-bbb", "ranking fusion", "blending keyword and vector recall with RRF", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}

	hits, err := store.MatchTopics(con, "fusion", 10)
	if err != nil {
		t.Fatalf("MatchTopics: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("MatchTopics(fusion) = %d hits, want 1", len(hits))
	}
	if hits[0].MsgID != startID {
		t.Errorf("MatchTopics hit MsgID = %d, want start message id %d", hits[0].MsgID, startID)
	}
	if hits[0].SessionID != "sess1" || hits[0].Topic != "ranking fusion" {
		t.Errorf("MatchTopics hit = %+v, want sess1/ranking fusion", hits[0])
	}

	// A query matching the summary column also surfaces (porter stemming on "recall").
	if h, _ := store.MatchTopics(con, "recall", 10); len(h) != 1 {
		t.Errorf("MatchTopics(recall) over summary = %d, want 1", len(h))
	}

	// A non-matching query returns nothing (not an error).
	if h, _ := store.MatchTopics(con, "kubernetes", 10); len(h) != 0 {
		t.Errorf("MatchTopics(kubernetes) = %d, want 0", len(h))
	}

	// A segment whose start message is missing has no anchor → skipped.
	if err := store.UpsertTopicSegment(con, "sess1", "uuid-orphan", "", "orphan topic", "no backing message", 2.0); err != nil {
		t.Fatalf("UpsertTopicSegment orphan: %v", err)
	}
	if h, _ := store.MatchTopics(con, "orphan", 10); len(h) != 0 {
		t.Errorf("MatchTopics(orphan) with no start message = %d, want 0", len(h))
	}
}

func TestTopicForMessageRangeContainment(t *testing.T) {
	con, _ := openTestDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}

	addTopicMsg(t, con, "sess1", "user", "m1", "u1")
	addTopicMsg(t, con, "sess1", "assistant", "m2", "u2")
	addTopicMsg(t, con, "sess1", "user", "m3", "u3")
	addTopicMsg(t, con, "sess1", "assistant", "m4", "u4")

	// Segment A covers u1..u2, segment B covers u3..u4.
	if err := store.UpsertTopicSegment(con, "sess1", "u1", "u2", "topic A", "", 1.0); err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	if err := store.UpsertTopicSegment(con, "sess1", "u3", "u4", "topic B", "", 2.0); err != nil {
		t.Fatalf("upsert B: %v", err)
	}

	if got := store.TopicForMessage(con, "sess1", "u2"); got != "topic A" {
		t.Errorf("TopicForMessage(u2) = %q, want topic A", got)
	}
	if got := store.TopicForMessage(con, "sess1", "u3"); got != "topic B" {
		t.Errorf("TopicForMessage(u3) = %q, want topic B", got)
	}
	// A message uuid that doesn't exist → "".
	if got := store.TopicForMessage(con, "sess1", "nope"); got != "" {
		t.Errorf("TopicForMessage(nope) = %q, want empty", got)
	}
}

func TestTopicForMessageSingleTopicFallback(t *testing.T) {
	con, _ := openTestDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	addTopicMsg(t, con, "sess1", "user", "m1", "u1")
	addTopicMsg(t, con, "sess1", "assistant", "m2", "u2")
	// One segment covering only u1; u2 is outside the range. With a single session
	// topic, the fallback attaches it anyway.
	if err := store.UpsertTopicSegment(con, "sess1", "u1", "u1", "only topic", "", 1.0); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := store.TopicForMessage(con, "sess1", "u2"); got != "only topic" {
		t.Errorf("TopicForMessage(u2) single-topic fallback = %q, want only topic", got)
	}
}

// TestTopicSurvivesBaseReindex is the reindex-safety guard: topic rows must
// survive a base-table rebuild() exactly like chunk_vec does. We insert a topic
// segment, force the base SchemaVersion path to rebuild (drop+recreate the base
// tables), and confirm topic_segment still holds the row.
func TestTopicSurvivesBaseReindex(t *testing.T) {
	con, _ := openTestDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, "sess1", "u1", "u2", "durable topic", "should survive a reindex", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}

	// Force a base rebuild: stamp a stale schema_version, then EnsureSchema sees the
	// mismatch and runs rebuild() — which drops/recreates ONLY the base tables.
	if _, err := con.Exec("INSERT OR REPLACE INTO meta(key,value) VALUES('schema_version','1')"); err != nil {
		t.Fatalf("stamp stale version: %v", err)
	}
	if err := EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema rebuild: %v", err)
	}

	// The base messages table was dropped+recreated, but topic_segment must persist.
	segs, err := store.TopicsForSession(con, "sess1")
	if err != nil {
		t.Fatalf("TopicsForSession after reindex: %v", err)
	}
	if len(segs) != 1 || segs[0].Topic != "durable topic" {
		t.Fatalf("topic rows did not survive base reindex: %+v", segs)
	}
}
