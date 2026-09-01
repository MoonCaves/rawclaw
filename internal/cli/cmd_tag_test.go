package cli

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/gofrs/flock"
)

// newTagTestDB builds a fresh writable db with the base + topic schema, returning
// the open connection. Uses storetest.NewDB (store.ConnectRW — the same opener
// the command takes), so the test exercises a real on-disk SQLite db.
func newTagTestDB(t *testing.T) *sql.DB {
	t.Helper()
	con, _ := storetest.NewDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	return con
}

// addMsg inserts one message carrying a uuid (topic helpers key off uuid) and
// returns its id.
func addMsg(t *testing.T, con *sql.DB, sid, role, content, uuid string) int {
	t.Helper()
	storetest.InsertSession(t, con, storetest.Session{ID: sid})
	return storetest.InsertMessage(t, con, storetest.Message{SessionID: sid, Role: role, Content: content, UUID: uuid})
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
	// Verify header instructions and schema format
	wantHeaderParts := []string{
		"# condensed session ",
		"# split into contiguous TOPIC segments with brief, inconclusive summaries:",
		"# describe what was explored, raised, or left open—not a verdict.",
		"# RawClaw has no supersession; other memory systems own current truth.",
		"# when finished, immediately run: rawclaw tag-write ",
		`# JSON payload on STDIN: [{"start_uuid":"<uuid8>","topic":"...","summary":"..."}]`,
	}
	for _, part := range wantHeaderParts {
		if !strings.Contains(out, part) {
			t.Errorf("dump missing expected header line %q; got:\n%s", part, out)
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

// TestIncrementalTagging_CloseoutGrowCloseout proves that re-tagging a grown
// session only dumps new untagged messages and preserves first-pass segments.
// (On the old delete-all-then-insert code, this failed by wiping earlier segments).
func TestIncrementalTagging_CloseoutGrowCloseout(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-grow-1"

	// First pass: 4 messages in session
	addMsg(t, con, sid, "user", "alpha", "11111111-aaaa")
	addMsg(t, con, sid, "user", "bravo", "22222222-bbbb")
	addMsg(t, con, sid, "user", "charl", "33333333-cccc")
	addMsg(t, con, sid, "user", "delta", "44444444-dddd")

	pass1JSON := `[
		{"start_uuid":"11111111","topic":"setup","summary":"first half"},
		{"start_uuid":"33333333","topic":"execution","summary":"second half"}
	]`
	if _, err := runTagWrite(con, sid, strings.NewReader(pass1JSON), 10.0, false); err != nil {
		t.Fatalf("pass 1 runTagWrite: %v", err)
	}

	// Grow session with 4 more messages
	addMsg(t, con, sid, "user", "echo", "55555555-eeee")
	addMsg(t, con, sid, "user", "foxtrot", "66666666-ffff")
	addMsg(t, con, sid, "user", "golf", "77777777-gggg")
	addMsg(t, con, sid, "user", "hotel", "88888888-hhhh")

	// Prep pass 2: should dump ONLY the untagged messages (55555555..88888888)
	// and a context comment line naming the previous topic ("execution", ended at 44444444)
	var prepDump strings.Builder
	if err := runTagPrep(&prepDump, con, sid); err != nil {
		t.Fatalf("pass 2 runTagPrep: %v", err)
	}
	prepOut := prepDump.String()

	if strings.Contains(prepOut, "11111111 [user]") || strings.Contains(prepOut, "22222222 [user]") ||
		strings.Contains(prepOut, "33333333 [user]") || strings.Contains(prepOut, "44444444 [user]") {
		t.Errorf("prep pass 2 dumped already-tagged messages 1..4; got:\n%s", prepOut)
	}
	if !strings.Contains(prepOut, "55555555") || !strings.Contains(prepOut, "88888888") {
		t.Errorf("prep pass 2 missing untagged messages 5..8; got:\n%s", prepOut)
	}
	if !strings.Contains(prepOut, `# previous topic: "execution" (ended at 44444444)`) {
		t.Errorf("prep pass 2 missing context comment line for previous segment; got:\n%s", prepOut)
	}

	// Write pass 2
	pass2JSON := `[{"start_uuid":"55555555","topic":"closeout","summary":"final part"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(pass2JSON), 20.0, false); err != nil {
		t.Fatalf("pass 2 runTagWrite: %v", err)
	}

	// Verify all 3 segments survive
	segs, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("TopicsForSession: %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("after growth closeout, stored %d segments, want 3 (pass 1 segments must be preserved)", len(segs))
	}
	segByTopic := make(map[string]store.TopicSegment)
	for _, s := range segs {
		segByTopic[s.Topic] = s
	}
	if s, ok := segByTopic["setup"]; !ok || s.StartUUID != "11111111-aaaa" || s.EndUUID != "22222222-bbbb" {
		t.Errorf("segment setup = %+v, want 1..2", s)
	}
	if s, ok := segByTopic["execution"]; !ok || s.StartUUID != "33333333-cccc" || s.EndUUID != "44444444-dddd" {
		t.Errorf("segment execution = %+v, want 3..4", s)
	}
	if s, ok := segByTopic["closeout"]; !ok || s.StartUUID != "55555555-eeee" || s.EndUUID != "88888888-hhhh" {
		t.Errorf("segment closeout = %+v, want 5..8", s)
	}
}

// TestIncrementalTagging_RerunMarker verifies that the rerun marker appears when
// untagged content exceeds chunkByteCap, and disappears when fully tagged.
func TestIncrementalTagging_RerunMarker(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-rerun-1"
	addMsg(t, con, sid, "user", "alpha", "11111111-aaaa")
	addMsg(t, con, sid, "user", "bravo", "22222222-bbbb")
	addMsg(t, con, sid, "user", "charl", "33333333-cccc")
	addMsg(t, con, sid, "user", "delta", "44444444-dddd")

	line := condensedLine(store.SessionMessage{UUID: "11111111-aaaa", Role: "user", Content: "alpha"})
	lineLen := len(line) + 1

	origCap := chunkByteCap
	// Set chunkByteCap so exactly 2 messages fit in pass 1
	chunkByteCap = 2*lineLen + 2
	defer func() { chunkByteCap = origCap }()

	// Pass 1: untagged 4 messages, cap fits 2 -> rerun marker should appear
	var p1 strings.Builder
	if err := runTagPrep(&p1, con, sid); err != nil {
		t.Fatalf("pass 1 runTagPrep: %v", err)
	}
	out1 := p1.String()
	if !strings.Contains(out1, "rerun 'rawclaw tag-prep sess-rer") {
		t.Errorf("pass 1 missing rerun marker; got:\n%s", out1)
	}
	if !strings.Contains(out1, "11111111") || !strings.Contains(out1, "22222222") {
		t.Errorf("pass 1 missing first 2 messages; got:\n%s", out1)
	}
	if strings.Contains(out1, "33333333") || strings.Contains(out1, "44444444") {
		t.Errorf("pass 1 should have capped before messages 3..4; got:\n%s", out1)
	}

	// Write pass 1
	json1 := `[{"start_uuid":"11111111","topic":"chunk1","summary":"first two"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(json1), 1.0, false); err != nil {
		t.Fatalf("pass 1 runTagWrite: %v", err)
	}

	// Pass 2: untagged 2 messages, cap fits 2 -> rerun marker should NOT appear
	var p2 strings.Builder
	if err := runTagPrep(&p2, con, sid); err != nil {
		t.Fatalf("pass 2 runTagPrep: %v", err)
	}
	out2 := p2.String()
	if strings.Contains(out2, "rerun 'rawclaw tag-prep") {
		t.Errorf("pass 2 should not have rerun marker; got:\n%s", out2)
	}
	if !strings.Contains(out2, "33333333") || !strings.Contains(out2, "44444444") {
		t.Errorf("pass 2 missing messages 3..4; got:\n%s", out2)
	}
	if !strings.Contains(out2, `# previous topic: "chunk1" (ended at 22222222)`) {
		t.Errorf("pass 2 missing context line for chunk1; got:\n%s", out2)
	}

	// Write pass 2
	json2 := `[{"start_uuid":"33333333","topic":"chunk2","summary":"second two"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(json2), 2.0, false); err != nil {
		t.Fatalf("pass 2 runTagWrite: %v", err)
	}

	// Pass 3: fully tagged session -> no-op notice
	var p3 strings.Builder
	if err := runTagPrep(&p3, con, sid); err != nil {
		t.Fatalf("pass 3 runTagPrep: %v", err)
	}
	out3 := p3.String()
	if !strings.Contains(out3, "already fully tagged") {
		t.Errorf("pass 3 missing fully-tagged notice; got:\n%s", out3)
	}
	if strings.Contains(out3, "rerun 'rawclaw tag-prep") {
		t.Errorf("pass 3 should not have rerun marker; got:\n%s", out3)
	}
}

// TestRunTagPrep_FullyTaggedIsNoOp verifies that running tag-prep on a fully tagged
// session outputs a no-op notice and exits with no error.
func TestRunTagPrep_FullyTaggedIsNoOp(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-fully-1"
	addMsg(t, con, sid, "user", "alpha", "11111111-aaaa")
	addMsg(t, con, sid, "user", "bravo", "22222222-bbbb")

	jsonIn := `[{"start_uuid":"11111111","topic":"full","summary":"all"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0, false); err != nil {
		t.Fatalf("runTagWrite: %v", err)
	}

	var out strings.Builder
	if err := runTagPrep(&out, con, sid); err != nil {
		t.Fatalf("runTagPrep on fully tagged session returned error: %v", err)
	}
	if !strings.Contains(out.String(), "already fully tagged") {
		t.Errorf("output missing fully tagged notice: %q", out.String())
	}
	if strings.Contains(out.String(), "[user]") {
		t.Errorf("output should not contain message lines: %q", out.String())
	}
}

// TestRunTagWrite_RejectsSegmentOutsideWindow verifies that tag-write validates
// every incoming segment lies within the declared untagged window [start, end]
// and rejects the write otherwise.
func TestRunTagWrite_RejectsSegmentOutsideWindow(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-window-val-1"
	// 1. Initial 2 messages, tagged in pass 1
	addMsg(t, con, sid, "user", "one", "11111111-aaaa")
	addMsg(t, con, sid, "user", "two", "22222222-bbbb")

	pass1 := `[{"start_uuid":"11111111","topic":"head","summary":"first two"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(pass1), 1.0, false); err != nil {
		t.Fatalf("pass 1 write: %v", err)
	}

	// 2. Grow session with 2 more messages [3..4]
	addMsg(t, con, sid, "user", "three", "33333333-cccc")
	addMsg(t, con, sid, "user", "four", "44444444-dddd")

	// 2. Untagged window is now [3..4] (33333333..44444444).
	// Submitting a segment starting at 11111111 (outside window) must be rejected!
	outOfWindowJSON := `[{"start_uuid":"11111111","topic":"bad","summary":"outside window"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(outOfWindowJSON), 2.0, false); err == nil {
		t.Fatal("expected error when segment start_uuid is outside untagged window")
	} else if !strings.Contains(err.Error(), "outside window") {
		t.Errorf("expected 'outside window' error, got: %v", err)
	}

	// 3. Submitting valid segment for window [3..4] succeeds
	validJSON := `[{"start_uuid":"33333333","topic":"tail","summary":"last two"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(validJSON), 2.0, false); err != nil {
		t.Fatalf("valid window write failed: %v", err)
	}

	// 4. Session is now fully tagged. Writing again without --retag-all must be rejected.
	extraJSON := `[{"start_uuid":"33333333","topic":"extra","summary":"extra"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(extraJSON), 3.0, false); err == nil {
		t.Fatal("expected error writing to already fully tagged session without retagAll")
	} else if !strings.Contains(err.Error(), "already fully tagged") {
		t.Errorf("expected 'already fully tagged' error, got: %v", err)
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

	n, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 42.0, false)
	if err != nil {
		t.Fatalf("runTagWrite: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d segments, want 2", n)
	}

	segs, err := store.TopicsForSession(con, sid)
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

// TestRunTagWriteRejectsOutOfOrderSegments verifies that tag-write rejects
// segment starts that move backwards through the session before writing any
// topic_segment rows.
func TestRunTagWriteRejectsOutOfOrderSegments(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-write-order-1"
	addMsg(t, con, sid, "user", "first", "11111111-aaaa")
	addMsg(t, con, sid, "assistant", "second", "22222222-bbbb")
	addMsg(t, con, sid, "user", "third", "33333333-cccc")

	jsonIn := `[
		{"start_uuid":"33333333","topic":"later","summary":"third message"},
		{"start_uuid":"11111111","topic":"earlier","summary":"first message"}
	]`
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0, false); err == nil {
		t.Fatal("expected out-of-order segment starts to be rejected")
	} else {
		if !strings.Contains(err.Error(), "segment 1") {
			t.Errorf("error = %q, want the offending segment number", err)
		}
		if !strings.Contains(err.Error(), "segment 0") {
			t.Errorf("error = %q, want the preceding segment number", err)
		}
		if !strings.Contains(err.Error(), "not after") {
			t.Errorf("error = %q, want an ordering rejection", err)
		}
	}

	segs, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("TopicsForSession: %v", err)
	}
	if len(segs) != 0 {
		t.Fatalf("stored %d segments after rejected input, want none: %+v", len(segs), segs)
	}
}

// TestRunTagWriteRetagReplaces locks the maintainer-requested behavior: re-tagging a
// session with --retag-all REDOES its tags — it does not stack a second set beside
// the first, and it does not error. A first pass writes two segments; a second pass
// with DIFFERENT boundaries + labels must leave ONLY the second set behind.
func TestRunTagWriteRetagReplaces(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-retag-1"
	addMsg(t, con, sid, "user", "how do we blend rankings", "11111111-aaaa")
	addMsg(t, con, sid, "assistant", "reciprocal rank fusion", "22222222-bbbb")
	addMsg(t, con, sid, "user", "do topics survive a reindex", "33333333-cccc")
	addMsg(t, con, sid, "assistant", "sidecar tables persist", "44444444-dddd")

	// First pass: two segments starting at msg 1 and msg 3.
	first := `[
		{"start_uuid":"11111111","topic":"ranking fusion","summary":"first pass"},
		{"start_uuid":"33333333","topic":"schema gating","summary":"first pass"}
	]`
	if _, err := runTagWrite(con, sid, strings.NewReader(first), 1.0, false); err != nil {
		t.Fatalf("first runTagWrite: %v", err)
	}

	// Second pass: a SINGLE segment with a shifted boundary (start at msg 2) and a
	// new label. With retagAll=true, this replaces the whole set.
	second := `[{"start_uuid":"22222222","topic":"one merged topic","summary":"second pass"}]`
	n, err := runTagWrite(con, sid, strings.NewReader(second), 2.0, true)
	if err != nil {
		t.Fatalf("second runTagWrite: %v", err)
	}
	if n != 1 {
		t.Fatalf("second pass wrote %d segments, want 1", n)
	}

	segs, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("TopicsForSession: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("after re-tag, stored %d segments, want 1 (old set must be gone)", len(segs))
	}
	if segs[0].StartUUID != "22222222-bbbb" || segs[0].Topic != "one merged topic" {
		t.Errorf("surviving segment = %s/%q, want 22222222-bbbb/\"one merged topic\"", segs[0].StartUUID, segs[0].Topic)
	}
	// The local re-tag must keep origin_machine NULL (the "this machine" sentinel).
	if segs[0].OriginMachine != "" {
		t.Errorf("origin_machine = %q, want empty (NULL) for a local re-tag", segs[0].OriginMachine)
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
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0, false); err == nil {
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
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0, false); err == nil {
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

	if _, err := runTagWrite(con, sid, strings.NewReader(`[]`), 1.0, false); err == nil {
		t.Fatal("expected an error for an empty segment array")
	}
}

// TestRunTagWriteMissingTopic errors when a segment lacks a topic label.
func TestRunTagWriteMissingTopic(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-write-5"
	addMsg(t, con, sid, "user", "first", "aaaa1111")

	jsonIn := `[{"start_uuid":"aaaa1111","summary":"no topic here"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0, false); err == nil {
		t.Fatal("expected an error for a segment missing its topic")
	} else if !strings.Contains(err.Error(), "topic") {
		t.Errorf("error = %q, want a 'missing topic' message", err)
	}
}

// TestRunTagWriteNoMessages errors cleanly on an empty session.
func TestRunTagWriteNoMessages(t *testing.T) {
	con := newTagTestDB(t)
	jsonIn := `[{"start_uuid":"x","topic":"t","summary":"s"}]`
	if _, err := runTagWrite(con, "missing-session", strings.NewReader(jsonIn), 1.0, false); err == nil {
		t.Fatal("expected an error writing to a session with no messages")
	}
}

// writeTaggableSession writes a transcript whose messages carry uuids (tag-write
// keys segments by uuid prefix), indexes it, and returns the project dir.
func writeTaggableSession(t *testing.T, root, project, id string, uuids ...string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	var b strings.Builder
	for i, u := range uuids {
		b.WriteString(`{"type":"user","uuid":"` + u + `","timestamp":"2026-06-01T10:00:0` +
			string(rune('0'+i)) + `Z","message":{"role":"user","content":"advancing the retention watermark"}}` + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	if _, _, _, err := index.EnsureIndexed(dir, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	return dir
}

// TestRunTagWriteFoldsIntoTheOneStore verifies immediate authoritative state and
// eventual detached visibility in the consolidated store.
func TestRunTagWriteFoldsIntoTheOneStore(t *testing.T) {
	root := newCfgRoot(t)
	sid := "9f3e1c20-aaaa-bbbb-cccc-0000000abcd1"
	dir := writeTaggableSession(t, root, "proj-tag", sid,
		"11111111-aaaa-bbbb-cccc-000000000001", "22222222-aaaa-bbbb-cccc-000000000002")

	scope := []view.Scope{{Project: "proj-tag", TDir: dir}}
	publishDone := make(chan error, 1)
	oldPublish := spawnTagPublish
	spawnTagPublish = func(dbp, sid string) error {
		go func() { publishDone <- runTagPublishChild(context.Background(), io.Discard, dbp, sid) }()
		return nil
	}
	t.Cleanup(func() { spawnTagPublish = oldPublish })
	jsonIn := `[{"start_uuid":"11111111","topic":"watermark","summary":"how the watermark is advanced"}]`
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(jsonIn), sid[:8], scope, nil, false, "", false); err != nil {
		t.Fatalf("runTagWriteCmd: %v\nout: %s", err, out.String())
	}
	if !strings.Contains(out.String(), "publication queued") {
		t.Fatalf("output = %q, want eventual publication receipt", out.String())
	}
	auth, err := store.ConnectRO(index.RefreshDBPath("claude", sid, filepath.Join(dir, sid+".jsonl")))
	if err != nil {
		t.Fatalf("open authoritative store: %v", err)
	}
	segs, err := store.TopicsForSession(auth, sid)
	_ = auth.Close()
	if err != nil || len(segs) != 1 || segs[0].Topic != "watermark" {
		t.Fatalf("authoritative topics = %#v, err=%v", segs, err)
	}

	select {
	case err := <-publishDone:
		if err != nil {
			t.Fatalf("tag publication: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consolidated publisher did not finish")
	}
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()
	hits, err := store.MatchTopics(con, "watermark", 8, nil)
	if err != nil || len(hits) != 1 || hits[0].Project != "proj-tag" {
		t.Fatalf("consolidated store topic hits = %+v, err=%v", hits, err)
	}
}

// TestRunTagWriteRoutine_MarksRoutineAndFolds verifies that tag-write --routine
// writes a routine verdict with the specified or default source and folds it
// into the consolidated store.
func TestRunTagWriteRoutine_MarksRoutineAndFolds(t *testing.T) {
	root := newCfgRoot(t)
	sid := "8e2d1c10-aaaa-bbbb-cccc-0000000abcd2"
	dir := writeTaggableSession(t, root, "proj-routine", sid,
		"11111111-aaaa-bbbb-cccc-000000000001", "22222222-aaaa-bbbb-cccc-000000000002")

	scope := []view.Scope{{Project: "proj-routine", TDir: dir}}
	publishDone := make(chan error, 1)
	oldPublish := spawnTagPublish
	spawnTagPublish = func(dbp, sid string) error {
		go func() { publishDone <- runTagPublishChild(context.Background(), io.Discard, dbp, sid) }()
		return nil
	}
	t.Cleanup(func() { spawnTagPublish = oldPublish })
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(""), sid[:8], scope, nil, true, store.VerdictSourceAgent, false); err != nil {
		t.Fatalf("runTagWriteCmd --routine: %v\nout: %s", err, out.String())
	}
	if !strings.Contains(out.String(), "marked "+sid[:8]+" as routine (source: agent)") {
		t.Errorf("output missing expected confirmation: %q", out.String())
	}

	// The authoritative refresh DB is synchronous; consolidated visibility is
	// eventual because publication is detached.
	auth, err := store.ConnectRO(index.RefreshDBPath("claude", sid, filepath.Join(dir, sid+".jsonl")))
	if err != nil {
		t.Fatalf("open authoritative store: %v", err)
	}
	v, ok, err := store.VerdictFor(auth, sid)
	_ = auth.Close()
	if err != nil || !ok {
		t.Fatalf("VerdictFor(%s) = (%+v, %v, %v), want ok=true", sid, v, ok, err)
	}
	if v.Verdict != store.VerdictRoutine || v.Source != store.VerdictSourceAgent {
		t.Errorf("verdict = %+v, want verdict=routine, source=agent", v)
	}
	if !strings.Contains(out.String(), "publication queued") {
		t.Fatalf("output = %q, want eventual publication receipt", out.String())
	}

	select {
	case err := <-publishDone:
		if err != nil {
			t.Fatalf("routine publication: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consolidated routine publisher did not finish")
	}
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()
	eff, err := store.IsEffectivelyRoutine(con, sid)
	if err != nil || !eff {
		t.Errorf("IsEffectivelyRoutine = %v, %v, want true, nil", eff, err)
	}
}

// TestRunTagWriteRoutine_ReTagPreservesTopics verifies that tagging real topic
// segments demotes routine, and re-tagging with --routine retains those topics.
func TestRunTagWriteRoutine_ReTagPreservesTopics(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-retag-1"
	addMsg(t, con, sid, "user", "routine test session", "11111111-aaaa")
	addMsg(t, con, sid, "assistant", "done", "22222222-bbbb")

	// 1. Mark routine
	if err := runTagWriteRoutine(con, sid, store.VerdictSourceFloor, 10.0); err != nil {
		t.Fatalf("runTagWriteRoutine: %v", err)
	}
	if eff, _ := store.IsEffectivelyRoutine(con, sid); !eff {
		t.Error("expected session to be effectively routine after runTagWriteRoutine")
	}

	// 2. Tag with real topic segments -> demotes routine
	jsonIn := `[{"start_uuid":"11111111","topic":"real topic","summary":"not routine"}]`
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 20.0, false); err != nil {
		t.Fatalf("runTagWrite with segments: %v", err)
	}
	if eff, _ := store.IsEffectivelyRoutine(con, sid); eff {
		t.Error("expected real topic segments to demote routine (IsEffectivelyRoutine = false)")
	}

	// 3. Re-tag with routine -> re-establishes routine
	if err := runTagWriteRoutine(con, sid, store.VerdictSourceAgent, 30.0); err != nil {
		t.Fatalf("runTagWriteRoutine second time: %v", err)
	}
	if eff, _ := store.IsEffectivelyRoutine(con, sid); eff {
		t.Error("expected retained real topic to keep routine verdict demoted (IsEffectivelyRoutine = false)")
	}
	segs, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("read topics after routine re-tag: %v", err)
	}
	if len(segs) != 1 || segs[0].Topic != "real topic" {
		t.Fatalf("topics after routine re-tag = %+v, want existing real topic preserved", segs)
	}
}

func TestApplyFloorRoutine_RetractsOnGrowth(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-floor-growth"
	addMsg(t, con, sid, "user", "hi", "11111111-aaaa")
	addMsg(t, con, sid, "assistant", "hello", "22222222-bbbb")

	if err := applyFloorRoutine(con, sid, true, 10.0); err != nil {
		t.Fatalf("mark floor routine: %v", err)
	}
	if v, ok, _ := store.VerdictFor(con, sid); !ok || v.Source != store.VerdictSourceFloor {
		t.Fatalf("floor verdict = %+v, %v; want floor verdict", v, ok)
	}

	if err := applyFloorRoutine(con, sid, false, 20.0); err != nil {
		t.Fatalf("retract floor routine: %v", err)
	}
	if _, ok, _ := store.VerdictFor(con, sid); ok {
		t.Fatal("floor verdict survived retraction after session growth")
	}
}

func TestApplyFloorRoutine_PreservesAgentVerdict(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-floor-agent"
	addMsg(t, con, sid, "user", "date?", "11111111-aaaa")
	if err := store.UpsertVerdict(con, store.Verdict{
		SessionID: sid,
		Verdict:   store.VerdictRoutine,
		Source:    store.VerdictSourceAgent,
		TaggedAt:  9,
	}); err != nil {
		t.Fatalf("seed agent verdict: %v", err)
	}

	if err := applyFloorRoutine(con, sid, true, 10.0); err != nil {
		t.Fatalf("apply floor routine: %v", err)
	}
	v, ok, _ := store.VerdictFor(con, sid)
	if !ok || v.Source != store.VerdictSourceAgent || v.TaggedAt != 9 {
		t.Fatalf("verdict after floor = %+v, %v; want original agent verdict", v, ok)
	}
}

func TestRunTagWriteRoutine_FloorPreservesRealTopics(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-floor-topic"
	addMsg(t, con, sid, "user", "real work", "11111111-aaaa")
	if _, err := runTagWrite(con, sid, strings.NewReader(
		`[{"start_uuid":"11111111","topic":"real","summary":"work"}]`), 1.0, false); err != nil {
		t.Fatalf("seed topic: %v", err)
	}

	if err := runTagWriteRoutine(con, sid, store.VerdictSourceFloor, 2.0); err != nil {
		t.Fatalf("apply floor routine: %v", err)
	}
	segs, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("read topics: %v", err)
	}
	if len(segs) != 1 || segs[0].Topic != "real" {
		t.Fatalf("topics after floor = %+v, want existing real topic preserved", segs)
	}
}

func TestRunTagWriteRoutine_AgentPreservesRealTopics(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-agent-topic"
	addMsg(t, con, sid, "user", "real work", "11111111-aaaa")
	if _, err := runTagWrite(con, sid, strings.NewReader(
		`[{"start_uuid":"11111111","topic":"real","summary":"work"}]`), 1.0, false); err != nil {
		t.Fatalf("seed topic: %v", err)
	}

	if err := runTagWriteRoutine(con, sid, store.VerdictSourceAgent, 2.0); err != nil {
		t.Fatalf("apply agent routine: %v", err)
	}
	segs, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("read topics: %v", err)
	}
	if len(segs) != 1 || segs[0].Topic != "real" {
		t.Fatalf("topics after agent routine = %+v, want existing real topic preserved", segs)
	}
	if eff, err := store.IsEffectivelyRoutine(con, sid); err != nil || eff {
		t.Fatalf("IsEffectivelyRoutine = %v, %v, want false because the real topic survives", eff, err)
	}
}

// TestRunTagFloorCmd_WaitsForConsolidatedFence pins the release-blocking fix
// found by the final review: runTagFloorCmd writes directly to the
// consolidated store (no per-project fold to fence it afterward), and used to
// do so unfenced — a concurrent rebuild's snapshot-then-rename could silently
// discard the write. It must now go through the SAME fence a rebuild takes,
// so a held lock blocks it rather than letting it race past.
func TestRunTagFloorCmd_WaitsForConsolidatedFence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	if err := index.EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close setup connection: %v", err)
	}

	holder := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated lock: locked=%t err=%v", locked, err)
	}

	done := make(chan error, 1)
	go func() {
		var b strings.Builder
		done <- runTagFloorCmd(&b, nil)
	}()

	select {
	case err := <-done:
		holder.Unlock()
		t.Fatalf("runTagFloorCmd returned (err=%v) while the fence was held — it wrote to the consolidated store without waiting for the lock", err)
	case <-time.After(200 * time.Millisecond):
		// still blocked, as expected
	}

	if err := holder.Unlock(); err != nil {
		t.Fatalf("release held lock: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runTagFloorCmd after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runTagFloorCmd did not complete after the fence was released")
	}
}
