package cli

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"

	_ "modernc.org/sqlite"
)

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
	if err := index.EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
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

// TestRunTagPrepDumpsCondensed asserts tag-prep prints one line per message in
// the `<uuid8> [<role>] <text>` shape (the dump a tagging subagent reads).
func TestRunTagPrepDumpsCondensed(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-prep-1"
	addMsg(t, con, sid, "user", "how do we blend rankings", "uuuuuuuu-1111-aaaa")
	addMsg(t, con, sid, "assistant", "reciprocal rank fusion explored", "aaaaaaaa-2222-bbbb")

	var b strings.Builder
	if err := runTagPrep(&b, con, sid); err != nil {
		t.Fatalf("runTagPrep: %v", err)
	}
	out := b.String()

	// Each message line is `<uuid8> [<role>] <text>` (uuid8 = first 8 chars).
	wantLines := []string{
		"uuuuuuuu [user] how do we blend rankings",
		"aaaaaaaa [assistant] reciprocal rank fusion explored",
	}
	for _, want := range wantLines {
		if !strings.Contains(out, want) {
			t.Errorf("dump missing line %q; got:\n%s", want, out)
		}
	}
	// Sanity on the line shape: each message line starts with an 8-char uuid8,
	// a space, then `[role]`.
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "#") {
			continue // header lines
		}
		if len(line) < 8 || line[8] != ' ' || line[9] != '[' {
			t.Errorf("malformed message line %q", line)
		}
	}
}

// TestRunTagPrepNoMessages errors cleanly on an empty session.
func TestRunTagPrepNoMessages(t *testing.T) {
	con := newTagTestDB(t)
	var b strings.Builder
	if err := runTagPrep(&b, con, "missing-session"); err == nil {
		t.Fatal("expected an error dumping a session with no messages")
	}
}

// TestRunTagWritePopulatesSegments feeds a JSON array via an io.Reader to
// runTagWrite and asserts the topic_segment rows carry the prefix-resolved
// start/end full-uuids + topic + summary, and that the next-segment end-uuid
// computation is correct.
func TestRunTagWritePopulatesSegments(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-write-1"
	// Four messages with distinct uuid prefixes.
	addMsg(t, con, sid, "user", "how do we blend rankings", "11111111-aaaa")
	addMsg(t, con, sid, "assistant", "reciprocal rank fusion", "22222222-bbbb")
	addMsg(t, con, sid, "user", "do topics survive a reindex", "33333333-cccc")
	addMsg(t, con, sid, "assistant", "sidecar tables persist", "44444444-dddd")

	// Segments keyed by uuid8 PREFIX (not the full uuid).
	jsonIn := `[
		{"start_uuid":"11111111","topic":"ranking fusion","summary":"RRF blending explored"},
		{"start_uuid":"33333333","topic":"schema gating","summary":"sidecar persistence discussed"}
	]`

	n, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 42.0)
	if err != nil {
		t.Fatalf("runTagWrite: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d segments, want 2", n)
	}

	segs, err := index.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("TopicsForSession: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("stored %d segments, want 2", len(segs))
	}
	// Segment 1: start resolves to the FULL uuid; ends at the message just before
	// segment 2's start (the 2nd message, full uuid).
	if segs[0].StartUUID != "11111111-aaaa" || segs[0].EndUUID != "22222222-bbbb" {
		t.Errorf("seg[0] range = %s..%s, want 11111111-aaaa..22222222-bbbb", segs[0].StartUUID, segs[0].EndUUID)
	}
	if segs[0].Topic != "ranking fusion" || segs[0].Summary != "RRF blending explored" {
		t.Errorf("seg[0] = %+v", segs[0])
	}
	// Segment 2: starts at message 3, ends at the session's last message (full uuid).
	if segs[1].StartUUID != "33333333-cccc" || segs[1].EndUUID != "44444444-dddd" {
		t.Errorf("seg[1] range = %s..%s, want 33333333-cccc..44444444-dddd", segs[1].StartUUID, segs[1].EndUUID)
	}
	if segs[1].Topic != "schema gating" {
		t.Errorf("seg[1].Topic = %q, want schema gating", segs[1].Topic)
	}
}

// TestRunTagWriteUnknownStartUUID errors clearly when a start_uuid prefix matches
// no message in the session.
func TestRunTagWriteUnknownStartUUID(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-write-2"
	addMsg(t, con, sid, "user", "first", "aaaa1111")
	addMsg(t, con, sid, "assistant", "second", "bbbb2222")

	jsonIn := `[{"start_uuid":"zzzz9999","topic":"ghost","summary":"never existed"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0); err == nil {
		t.Fatal("expected an error for a start_uuid matching no message")
	} else if !strings.Contains(err.Error(), "no message") {
		t.Errorf("error = %q, want a 'matches no message' message", err)
	}
}

// TestRunTagWriteAmbiguousStartUUID errors clearly when a start_uuid prefix
// matches more than one message.
func TestRunTagWriteAmbiguousStartUUID(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-write-3"
	addMsg(t, con, sid, "user", "first", "dupe-1111")
	addMsg(t, con, sid, "assistant", "second", "dupe-2222")

	// "dupe" is a prefix of both message uuids → ambiguous.
	jsonIn := `[{"start_uuid":"dupe","topic":"t","summary":"s"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0); err == nil {
		t.Fatal("expected an error for an ambiguous start_uuid prefix")
	} else if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want an 'ambiguous' message", err)
	}
}

// TestRunTagWriteEmptyArray errors on an empty segment array.
func TestRunTagWriteEmptyArray(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-write-4"
	addMsg(t, con, sid, "user", "first", "aaaa1111")

	if _, err := runTagWrite(con, sid, strings.NewReader(`[]`), 1.0); err == nil {
		t.Fatal("expected an error for an empty segment array")
	}
}

// TestRunTagWriteMissingTopic errors when a segment lacks a topic label.
func TestRunTagWriteMissingTopic(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-write-5"
	addMsg(t, con, sid, "user", "first", "aaaa1111")

	jsonIn := `[{"start_uuid":"aaaa1111","summary":"no topic here"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0); err == nil {
		t.Fatal("expected an error for a segment missing its topic")
	} else if !strings.Contains(err.Error(), "topic") {
		t.Errorf("error = %q, want a 'missing topic' message", err)
	}
}

// TestRunTagWriteNoMessages errors cleanly on an empty session.
func TestRunTagWriteNoMessages(t *testing.T) {
	con := newTagTestDB(t)
	jsonIn := `[{"start_uuid":"x","topic":"t","summary":"s"}]`
	if _, err := runTagWrite(con, "missing-session", strings.NewReader(jsonIn), 1.0); err == nil {
		t.Fatal("expected an error writing to a session with no messages")
	}
}
