package cli

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/tagger"

	_ "modernc.org/sqlite"
)

// mockTagger returns canned segments and records the condensed view it saw, so a
// test can drive the populate path end-to-end without a live LLM.
type mockTagger struct {
	segs []tagger.Segment
	err  error
	seen []string // one entry per TagSession call (per window)
}

func (m *mockTagger) TagSession(condensed string) ([]tagger.Segment, error) {
	m.seen = append(m.seen, condensed)
	if m.err != nil {
		return nil, m.err
	}
	return m.segs, nil
}

// newTagTestDB builds a fresh writable db with the base + topic schema, returning
// the open connection. Uses the cli package's openRW (the same path the command
// takes), so the test exercises a real on-disk SQLite db.
func newTagTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbp := filepath.Join(t.TempDir(), "tag.db")
	con, err := openRW(dbp)
	if err != nil {
		t.Fatalf("openRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })
	if err := index.EnsureSchema(con); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return con
}

// addMsg inserts one message carrying a uuid (topic helpers key off uuid) and
// returns its id.
func addMsg(t *testing.T, con *sql.DB, sid, role, content, uuid string) int {
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
	id, _ := res.LastInsertId()
	return int(id)
}

// TestRunTagPopulatesSegments drives the populate core end-to-end on a small
// indexed temp session and asserts the topic_segment rows carry the right
// start/end uuids + topic + summary.
func TestRunTagPopulatesSegments(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-tag-1"
	// Four messages; the mock returns two segments starting at id 1 and id 3.
	id1 := addMsg(t, con, sid, "user", "how do we blend rankings", "u1")
	addMsg(t, con, sid, "assistant", "reciprocal rank fusion", "u2")
	id3 := addMsg(t, con, sid, "user", "do topics survive a reindex", "u3")
	addMsg(t, con, sid, "assistant", "sidecar tables persist", "u4")

	mt := &mockTagger{segs: []tagger.Segment{
		{StartID: id1, Topic: "ranking fusion", Summary: "RRF blending explored"},
		{StartID: id3, Topic: "schema gating", Summary: "sidecar persistence discussed"},
	}}

	n, err := runTag(con, sid, mt, 42.0)
	if err != nil {
		t.Fatalf("runTag: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d segments, want 2", n)
	}

	// The mock saw the condensed view with #id/role lines.
	if len(mt.seen) != 1 {
		t.Fatalf("TagSession called %d times, want 1 (one window)", len(mt.seen))
	}
	if want := "[#" + strconv.Itoa(id1) + " user] how do we blend rankings"; !strings.Contains(mt.seen[0], want) {
		t.Errorf("condensed view missing %q; got:\n%s", want, mt.seen[0])
	}

	segs, err := index.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("TopicsForSession: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("stored %d segments, want 2", len(segs))
	}
	// Segment 1: starts at u1, ends at u2 (the message just before segment 2's start u3).
	if segs[0].StartUUID != "u1" || segs[0].EndUUID != "u2" {
		t.Errorf("seg[0] range = %s..%s, want u1..u2", segs[0].StartUUID, segs[0].EndUUID)
	}
	if segs[0].Topic != "ranking fusion" || segs[0].Summary != "RRF blending explored" {
		t.Errorf("seg[0] = %+v", segs[0])
	}
	// Segment 2: starts at u3, ends at the session's last message u4.
	if segs[1].StartUUID != "u3" || segs[1].EndUUID != "u4" {
		t.Errorf("seg[1] range = %s..%s, want u3..u4", segs[1].StartUUID, segs[1].EndUUID)
	}
	if segs[1].Topic != "schema gating" {
		t.Errorf("seg[1].Topic = %q, want schema gating", segs[1].Topic)
	}
}

// TestRunTagSkipsInventedStartID confirms a segment whose start_id matches no
// loaded message is skipped (the model occasionally invents an id), and the
// surrounding boundaries still resolve.
func TestRunTagSkipsInventedStartID(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-tag-2"
	id1 := addMsg(t, con, sid, "user", "first", "a1")
	addMsg(t, con, sid, "assistant", "second", "a2")

	mt := &mockTagger{segs: []tagger.Segment{
		{StartID: id1, Topic: "real", Summary: "discussed"},
		{StartID: 99999, Topic: "ghost", Summary: "never existed"},
	}}

	n, err := runTag(con, sid, mt, 1.0)
	if err != nil {
		t.Fatalf("runTag: %v", err)
	}
	if n != 1 {
		t.Fatalf("wrote %d segments, want 1 (the invented id is skipped)", n)
	}
	segs, _ := index.TopicsForSession(con, sid)
	if len(segs) != 1 || segs[0].Topic != "real" {
		t.Fatalf("segments = %+v, want one 'real'", segs)
	}
	// The real segment is the only/last → end_uuid is the session's last message.
	if segs[0].EndUUID != "a2" {
		t.Errorf("seg[0].EndUUID = %q, want a2", segs[0].EndUUID)
	}
}

// TestRunTagNoMessages errors cleanly on an empty session.
func TestRunTagNoMessages(t *testing.T) {
	con := newTagTestDB(t)
	mt := &mockTagger{}
	if _, err := runTag(con, "missing-session", mt, 1.0); err == nil {
		t.Fatal("expected an error tagging a session with no messages")
	}
}
