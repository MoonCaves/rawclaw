package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
	"github.com/MoonCaves/rawclaw/internal/view"
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

// setBookendCap sizes dumpByteCap so a 4-uniform-length-message fixture (see
// addBookendFixture) bookends to exactly message 1 (head) and message 4
// (tail), dropping messages 2 and 3 as the gap. Returns a restore func.
func setBookendCap(t *testing.T) func() {
	t.Helper()
	line := condensedLine(store.SessionMessage{UUID: "11111111-aaaa", Role: "user", Content: "alpha"})
	l := len(line)
	orig := dumpByteCap
	dumpByteCap = 2*(l+1) + 2 // half = l+2: fits 1 line, not 2, on each side
	return func() { dumpByteCap = orig }
}

// addBookendFixture inserts 4 messages with uniform-length condensed lines
// (same role, same content length) so head/tail budget math is exact.
func addBookendFixture(t *testing.T, con *sql.DB, sid string) {
	t.Helper()
	addMsg(t, con, sid, "user", "alpha", "11111111-aaaa")
	addMsg(t, con, sid, "user", "bravo", "22222222-bbbb")
	addMsg(t, con, sid, "user", "charl", "33333333-cccc")
	addMsg(t, con, sid, "user", "delta", "44444444-dddd")
}

// TestRunTagPrepBookendsLargeSessions asserts tag-prep keeps a head prefix AND
// a tail suffix (not just the head) when a dump exceeds dumpByteCap, dropping
// the middle with a clear gap note — a session's setup and its conclusion both
// matter for topic segmentation.
func TestRunTagPrepBookendsLargeSessions(t *testing.T) {
	con := newTagTestDB(t)
	sid := "sess-bookend-1"
	addBookendFixture(t, con, sid)
	defer setBookendCap(t)()

	var dump strings.Builder
	if err := runTagPrep(&dump, con, sid); err != nil {
		t.Fatalf("runTagPrep: %v", err)
	}
	out := dump.String()

	if !strings.Contains(out, "11111111") {
		t.Errorf("dump missing the head message (11111111):\n%s", out)
	}
	if !strings.Contains(out, "44444444") {
		t.Errorf("dump missing the tail message (44444444):\n%s", out)
	}
	if strings.Contains(out, "22222222") || strings.Contains(out, "33333333") {
		t.Errorf("dump should drop the middle messages (22222222, 33333333):\n%s", out)
	}
	if !strings.Contains(out, "may NOT span this gap") {
		t.Errorf("dump missing the gap note:\n%s", out)
	}
}

// TestRunTagWriteRespectsBookendGap locks the write-side half of the same fix:
// a segment can never silently claim the dropped middle of a bookended dump,
// whether the tagging subagent splits at the gap (well-behaved) or not (lazy).
func TestRunTagWriteRespectsBookendGap(t *testing.T) {
	t.Run("well-behaved: subagent splits at the gap", func(t *testing.T) {
		con := newTagTestDB(t)
		sid := "sess-bookend-2"
		addBookendFixture(t, con, sid)
		defer setBookendCap(t)()

		jsonIn := `[
			{"start_uuid":"11111111","topic":"setup","summary":"head"},
			{"start_uuid":"44444444","topic":"conclusion","summary":"tail"}
		]`
		if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0); err != nil {
			t.Fatalf("runTagWrite: %v", err)
		}
		segs, err := store.TopicsForSession(con, sid)
		if err != nil {
			t.Fatalf("TopicsForSession: %v", err)
		}
		if len(segs) != 2 {
			t.Fatalf("stored %d segments, want 2", len(segs))
		}
		if segs[0].EndUUID != "11111111-aaaa" {
			t.Errorf("segment 1 end_uuid = %q, want 11111111-aaaa (its own message, not into the gap)", segs[0].EndUUID)
		}
		if segs[1].EndUUID != "44444444-dddd" {
			t.Errorf("segment 2 end_uuid = %q, want 44444444-dddd (the last message shown)", segs[1].EndUUID)
		}
	})

	t.Run("lazy: subagent submits one segment for the whole dump", func(t *testing.T) {
		con := newTagTestDB(t)
		sid := "sess-bookend-3"
		addBookendFixture(t, con, sid)
		defer setBookendCap(t)()

		// A single segment covering only the head, with nothing for the tail, must
		// be REJECTED outright — not accepted-but-clamped. A session never gets
		// left half-labeled: tag-write errors, and the subagent has to retry with
		// segments on both sides of the gap.
		jsonIn := `[{"start_uuid":"11111111","topic":"whole session","summary":"ignored the gap note"}]`
		_, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0)
		if err == nil {
			t.Fatal("expected an error: a bookended dump requires segments on both sides of the gap")
		}
		if !strings.Contains(err.Error(), "BOTH sides") {
			t.Errorf("error = %q, want a message about covering both sides of the gap", err)
		}
		if segs, _ := store.TopicsForSession(con, sid); len(segs) != 0 {
			t.Errorf("rejected write left %d segments stored, want 0 (nothing half-labeled)", len(segs))
		}
	})

	t.Run("well-behaved: the dropped middle reads back as genuinely untagged", func(t *testing.T) {
		con := newTagTestDB(t)
		sid := "sess-bookend-4"
		addBookendFixture(t, con, sid)
		defer setBookendCap(t)()

		jsonIn := `[
			{"start_uuid":"11111111","topic":"setup","summary":"head"},
			{"start_uuid":"44444444","topic":"conclusion","summary":"tail"}
		]`
		if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 1.0); err != nil {
			t.Fatalf("runTagWrite: %v", err)
		}
		// Message 2 was in the dropped middle — never shown to the subagent, never
		// covered by either stored segment. It must come back as genuinely
		// untagged, not silently inherit "setup" or "conclusion" via a
		// single-segment fallback meant for ordinary, non-bookended sessions.
		if topic := store.TopicForMessage(con, sid, "22222222-bbbb"); topic != "" {
			t.Errorf("TopicForMessage(dropped-middle message) = %q, want \"\" (untagged)", topic)
		}
	})
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

// TestRunTagWriteRetagReplaces locks the maintainer-requested behavior: re-tagging a
// session REDOES its tags — it does not stack a second set beside the first, and
// it does not error. A first pass writes two segments; a second pass with DIFFERENT
// boundaries + labels must leave ONLY the second set behind.
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
	if _, err := runTagWrite(con, sid, strings.NewReader(first), 1.0); err != nil {
		t.Fatalf("first runTagWrite: %v", err)
	}

	// Second pass: a SINGLE segment with a shifted boundary (start at msg 2) and a
	// new label. Under the old per-(session,start_uuid) upsert this would have left
	// both first-pass rows behind (different start_uuids) — the stacking bug.
	second := `[{"start_uuid":"22222222","topic":"one merged topic","summary":"second pass"}]`
	n, err := runTagWrite(con, sid, strings.NewReader(second), 2.0)
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

// TestRunTagWriteFoldsIntoTheOneStore locks the write-through: a topic tagged
// today must be visible to the readers that query the consolidated store, not
// only to the project db it was written into. Without the fold, `topics` would
// keep missing every tag until the next full consolidate run.
func TestRunTagWriteFoldsIntoTheOneStore(t *testing.T) {
	root := newCfgRoot(t)
	sid := "9f3e1c20-aaaa-bbbb-cccc-0000000abcd1"
	dir := writeTaggableSession(t, root, "proj-tag", sid,
		"11111111-aaaa-bbbb-cccc-000000000001", "22222222-aaaa-bbbb-cccc-000000000002")

	scope := []view.Scope{{Project: "proj-tag", TDir: dir}}
	jsonIn := `[{"start_uuid":"11111111","topic":"watermark","summary":"how the watermark is advanced"}]`
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(jsonIn), sid[:8], scope, nil, false, ""); err != nil {
		t.Fatalf("runTagWriteCmd: %v\nout: %s", err, out.String())
	}

	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()
	hits, err := store.MatchTopics(con, "watermark", 8, nil)
	if err != nil {
		t.Fatalf("MatchTopics on the consolidated store: %v", err)
	}
	if len(hits) != 1 || hits[0].Topic != "watermark" {
		t.Fatalf("consolidated store topic hits = %+v, want the freshly written watermark tag", hits)
	}
	if hits[0].Project != "proj-tag" {
		t.Errorf("hit project = %q, want proj-tag", hits[0].Project)
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
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(""), sid[:8], scope, nil, true, store.VerdictSourceAgent); err != nil {
		t.Fatalf("runTagWriteCmd --routine: %v\nout: %s", err, out.String())
	}
	if !strings.Contains(out.String(), "marked "+sid[:8]+" as routine (source: agent)") {
		t.Errorf("output missing expected confirmation: %q", out.String())
	}

	// Verify in the consolidated store
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()

	v, ok, err := store.VerdictFor(con, sid)
	if err != nil || !ok {
		t.Fatalf("VerdictFor(%s) = (%+v, %v, %v), want ok=true", sid, v, ok, err)
	}
	if v.Verdict != store.VerdictRoutine || v.Source != store.VerdictSourceAgent {
		t.Errorf("verdict = %+v, want verdict=routine, source=agent", v)
	}

	eff, err := store.IsEffectivelyRoutine(con, sid)
	if err != nil || !eff {
		t.Errorf("IsEffectivelyRoutine = %v, %v, want true, nil", eff, err)
	}
}

// TestRunTagWriteRoutine_ReTagReverses verifies that tagging real topic segments
// demotes routine, and re-tagging with --routine re-establishes routine.
func TestRunTagWriteRoutine_ReTagReverses(t *testing.T) {
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
	if _, err := runTagWrite(con, sid, strings.NewReader(jsonIn), 20.0); err != nil {
		t.Fatalf("runTagWrite with segments: %v", err)
	}
	if eff, _ := store.IsEffectivelyRoutine(con, sid); eff {
		t.Error("expected real topic segments to demote routine (IsEffectivelyRoutine = false)")
	}

	// 3. Re-tag with routine -> re-establishes routine
	if err := runTagWriteRoutine(con, sid, store.VerdictSourceAgent, 30.0); err != nil {
		t.Fatalf("runTagWriteRoutine second time: %v", err)
	}
	if eff, _ := store.IsEffectivelyRoutine(con, sid); !eff {
		t.Error("expected re-tagging with routine to re-establish routine (IsEffectivelyRoutine = true)")
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
		`[{"start_uuid":"11111111","topic":"real","summary":"work"}]`), 1.0); err != nil {
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
