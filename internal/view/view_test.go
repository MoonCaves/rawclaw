package view

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// seedMsg is one row to insert into the messages table for a test fixture.
// id is the EXPECTED rowid: rows are inserted in slice order into a fresh
// production-schema db, so ids must be contiguous from 1 — the helper fails
// fast if an insert lands on a different rowid.
type seedMsg struct {
	id      int
	role    string
	content string
}

// newTestDB opens a fresh production-schema db (via storetest) and inserts the
// session row plus the given messages. The fixture ids in the test tables are
// pinned by asserting each insert's rowid matches the seedMsg id.
func newTestDB(t *testing.T, sessionID string, msgs []seedMsg) *sql.DB {
	t.Helper()
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: sessionID})
	for _, m := range msgs {
		got := storetest.InsertMessage(t, con, storetest.Message{
			SessionID: sessionID, Role: m.role, Content: m.content})
		if got != m.id {
			t.Fatalf("fixture rowid = %d, want %d (seedMsg ids must be contiguous from 1)", got, m.id)
		}
	}
	return con
}

func ids(ms []ViewMsg) []int {
	out := make([]int, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildAnchoredView(t *testing.T) {
	const sid = "sess-1"
	// A 12-message conversation, ids 1..12, alternating user/assistant,
	// with one tool row (id=6) and one empty row (id=8) to exercise filtering.
	base := []seedMsg{
		{1, "user", "first user message"},
		{2, "assistant", "first assistant reply"},
		{3, "user", "second user"},
		{4, "assistant", "second reply"},
		{5, "user", "anchor-neighbor before"},
		{6, "tool", "tool output blob"},
		{7, "assistant", "anchor message here"}, // anchor
		{8, "user", ""},                         // empty content
		{9, "assistant", "after one"},
		{10, "user", "after two"},
		{11, "assistant", "near end"},
		{12, "user", "last user message"},
	}

	tests := []struct {
		name             string
		anchor           int
		opts             AnchoredViewOpts
		wantNil          bool
		wantWindow       []int
		wantAnchor       int // expected anchor id in window (0 = none)
		wantBookendStart []int
		wantBookendEnd   []int
		wantBefore       int
		wantAfter        int
	}{
		{
			name:   "default window keyword (tools excluded, empty skipped)",
			anchor: 7,
			opts:   AnchoredViewOpts{Window: 5, Bookend: 3, IncludeTools: false},
			// before = id<=7 DESC LIMIT 6 -> ids 7,6,5,4,3,2 ; reversed -> 2,3,4,5,6,7
			// after  = id>7 ASC LIMIT 5   -> 8,9,10,11,12
			// win ids = 2,3,4,5,6,7,8,9,10,11,12
			// filter: drop tool id=6 (not anchor), drop empty id=8 (not anchor)
			wantWindow: []int{2, 3, 4, 5, 7, 9, 10, 11, 12},
			wantAnchor: 7,
			// bookend_start: win_min=2 -> id<2 user/assistant len>0 ASC LIMIT 3 -> id 1
			wantBookendStart: []int{1},
			// win_max=12 -> id>12 none
			wantBookendEnd: nil,
			// before list had 6 rows -> messages_before = 5
			wantBefore: 5,
			wantAfter:  5,
		},
		{
			name:   "small window surfaces bookends",
			anchor: 7,
			opts:   AnchoredViewOpts{Window: 1, Bookend: 2, IncludeTools: false},
			// before = id<=7 DESC LIMIT 2 -> 7,6 ; reversed -> 6,7
			// after  = id>7 ASC LIMIT 1   -> 8
			// win ids = 6,7,8 ; win_min=6 win_max=8
			// filter: id=6 tool dropped, id=7 anchor kept, id=8 empty dropped
			wantWindow: []int{7},
			wantAnchor: 7,
			// bookend_start: id<6 user/assistant len>0 ASC LIMIT 2 -> 1,2
			wantBookendStart: []int{1, 2},
			// bookend_end: id>8 user/assistant len>0 DESC LIMIT 2 -> 12,11 ; reversed -> 11,12
			wantBookendEnd: []int{11, 12},
			wantBefore:     1, // before had 2 rows -> 2-1
			wantAfter:      1,
		},
		{
			name:   "include tools keeps tool + empty-but-anchor logic",
			anchor: 7,
			opts:   AnchoredViewOpts{Window: 2, Bookend: 0, IncludeTools: true},
			// before id<=7 DESC LIMIT 3 -> 7,6,5 ; reversed 5,6,7
			// after  id>7 ASC LIMIT 2   -> 8,9
			// win = 5,6,7,8,9 ; include_tools so tool id=6 kept; id=8 empty (not anchor) dropped
			wantWindow: []int{5, 6, 7, 9},
			wantAnchor: 7,
			wantBefore: 2,
			wantAfter:  2,
		},
		{
			name:   "anchor at very start",
			anchor: 1,
			opts:   AnchoredViewOpts{Window: 2, Bookend: 2, IncludeTools: false},
			// before id<=1 DESC LIMIT 3 -> 1 ; reversed -> 1
			// after  id>1 ASC LIMIT 2   -> 2,3
			// win = 1,2,3
			wantWindow:       []int{1, 2, 3},
			wantAnchor:       1,
			wantBookendStart: nil, // id<1 none
			// win_max=3 -> bookend_end id>3 user/assistant len>0 DESC LIMIT 2 -> 12,11 reversed -> 11,12
			wantBookendEnd: []int{11, 12},
			wantBefore:     0, // before had 1 row -> max(0, 0)
			wantAfter:      2,
		},
		{
			name:    "empty session yields empty window -> nil",
			anchor:  5,
			opts:    AnchoredViewOpts{Window: 5, Bookend: 3, IncludeTools: false},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := sid
			if tt.wantNil {
				// query a session id that has no rows -> empty window -> nil
				session = "empty-sess"
			}
			con := newTestDB(t, sid, base)
			av := BuildAnchoredView(con, session, tt.anchor, tt.opts)
			if tt.wantNil {
				if av != nil {
					t.Fatalf("want nil view, got %+v", av)
				}
				return
			}
			if av == nil {
				t.Fatal("got nil view, want non-nil")
			}
			if got := ids(av.Window); !eqInts(got, tt.wantWindow) {
				t.Errorf("window ids = %v, want %v", got, tt.wantWindow)
			}
			// verify exactly one anchor flag, on the right id
			anchorCount, anchorID := 0, 0
			for _, m := range av.Window {
				if m.Anchor {
					anchorCount++
					anchorID = m.ID
				}
			}
			if anchorCount != 1 || anchorID != tt.wantAnchor {
				t.Errorf("anchor flag = (count=%d id=%d), want id %d once", anchorCount, anchorID, tt.wantAnchor)
			}
			if got := ids(av.BookendStart); !eqInts(got, tt.wantBookendStart) {
				t.Errorf("bookend_start = %v, want %v", got, tt.wantBookendStart)
			}
			if got := ids(av.BookendEnd); !eqInts(got, tt.wantBookendEnd) {
				t.Errorf("bookend_end = %v, want %v", got, tt.wantBookendEnd)
			}
			if av.MessagesBefore != tt.wantBefore {
				t.Errorf("messages_before = %d, want %d", av.MessagesBefore, tt.wantBefore)
			}
			if av.MessagesAfter != tt.wantAfter {
				t.Errorf("messages_after = %d, want %d", av.MessagesAfter, tt.wantAfter)
			}
		})
	}
}

// TestBuildAnchoredViewAnchorAlwaysKept verifies the anchor survives even when
// it is a tool row or has empty display text (the "always keep the anchor" rule).
func TestBuildAnchoredViewAnchorAlwaysKept(t *testing.T) {
	const sid = "s"
	msgs := []seedMsg{
		{1, "user", "u"},
		{2, "tool", ""}, // anchor: tool role AND empty content
		{3, "assistant", "a"},
	}
	con := newTestDB(t, sid, msgs)
	av := BuildAnchoredView(con, sid, 2, AnchoredViewOpts{Window: 2, Bookend: 0, IncludeTools: false})
	if av == nil {
		t.Fatal("nil view")
	}
	if got := ids(av.Window); !eqInts(got, []int{1, 2, 3}) {
		t.Fatalf("window = %v, want [1 2 3] (anchor kept despite tool+empty)", got)
	}
	if !av.Window[1].Anchor || av.Window[1].ID != 2 {
		t.Errorf("anchor flag not on id=2: %+v", av.Window)
	}
}

// newPreviewDB builds a production-schema db seeding one session's worth of
// user messages for sessionPreview.
func newPreviewDB(t *testing.T, sessionID string, msgs []seedMsg) *sql.DB {
	t.Helper()
	con := newTestDB(t, sessionID, msgs)
	return con
}

func TestSessionPreview(t *testing.T) {
	const sid = "sess-prev"
	tests := []struct {
		name string
		msgs []seedMsg
		want string
	}{
		{
			name: "hi opener skipped, preview from later substantive turn",
			msgs: []seedMsg{
				{1, "user", "hi"},
				{2, "assistant", "hello there"},
				{3, "user", "explain the bookend window logic"},
			},
			want: "explain the bookend window logic",
		},
		{
			name: "slash-clear-only opener skipped in preview",
			msgs: []seedMsg{
				{1, "user", "/clear"},
				{2, "user", "now build the index"},
			},
			want: "now build the index",
		},
		{
			name: "first message already substantive",
			msgs: []seedMsg{
				{1, "user", "fix the parser"},
				{2, "user", "and add tests"},
			},
			want: "fix the parser",
		},
		{
			name: "all low-signal falls back to first non-empty user message (session kept)",
			msgs: []seedMsg{
				{1, "user", "hi"},
				{2, "user", "/clear"},
			},
			// no substantive turn -> fallback to first non-empty so the row still previews
			want: "hi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			con := newPreviewDB(t, sid, tt.msgs)
			if got := SessionPreview(con, sid, browsePreviewCap); got != tt.want {
				t.Errorf("SessionPreview() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSessionPreviewKeepsSessionWithGreetingOpener is the headline case:
// a 'hi'-opening session must still surface (a non-empty preview), with the
// preview text coming from its first substantive turn — the session is never
// dropped on the low-signal predicate.
func TestSessionPreviewKeepsSessionWithGreetingOpener(t *testing.T) {
	const sid = "greet"
	con := newPreviewDB(t, sid, []seedMsg{
		{1, "user", "hey"},
		{2, "assistant", "hi!"},
		{3, "user", "wire up the discovery dedup"},
	})
	got := SessionPreview(con, sid, browsePreviewCap)
	if got == "" {
		t.Fatal("session with greeting opener produced empty preview (session effectively dropped)")
	}
	if got != "wire up the discovery dedup" {
		t.Errorf("preview = %q, want the first substantive turn", got)
	}
}

// writeScrollSession writes a single-message transcript <stem>.jsonl under proj.
func writeScrollSession(t *testing.T, proj, stem, content string) {
	t.Helper()
	line := `{"type":"user","uuid":"u-` + stem + `","timestamp":"2026-06-01T10:00:00Z",` +
		`"message":{"role":"user","content":"` + content + `"}}`
	path := filepath.Join(proj, stem+".jsonl")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestBrowseReturnsPreviewsWithoutDeadlock pins the browse deadlock fix. Browse
// runs a per-session preview query for each row it returns, on the same
// single-connection RO pool (store.ConnectRO sets SetMaxOpenConns(1)). The bug
// shipped in v0.1.0 was issuing that preview query from inside the still-open
// session-rows cursor, which blocked forever waiting for a second connection.
// The fix drains and closes the rows before previewing. This test exercises that
// composition against a real index and, via a timeout, turns a reintroduced
// deadlock into a loud failure instead of a hung CI run.
func TestBrowseReturnsPreviewsWithoutDeadlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	writeScrollSession(t, proj, "aaaa00000001", "investigating the browse deadlock on the single conn pool")
	writeScrollSession(t, proj, "bbbb00000002", "tracing the scroll resolver and subagent ids")

	done := make(chan []BrowseRow, 1)
	go func() { done <- Browse(proj, 10, "", "") }()

	select {
	case rows := <-done:
		if len(rows) < 2 {
			t.Fatalf("Browse returned %d rows, want >= 2", len(rows))
		}
		for _, r := range rows {
			if r.Preview == "" {
				t.Errorf("session %s has empty Preview — drain-then-preview did not run", r.SessionID)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Browse() did not return within 10s — single-conn pool deadlock reintroduced")
	}
}

// TestSessionLastActivityShowsNewestRealMessage is the "what is this desk doing
// right now" contract: the row's Last must be the newest message that is real
// conversation, not the session's opening and not the machinery that usually
// sits at the tail of a busy session.
func TestSessionLastActivityShowsNewestRealMessage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	lines := []string{
		`{"type":"user","uuid":"11111111-0000-0000-0000-000000000001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"set up the deployment pipeline"}}`,
		`{"type":"assistant","uuid":"11111111-0000-0000-0000-000000000002","timestamp":"2026-06-01T10:05:00Z","message":{"role":"assistant","content":"the nightly job now repins every image"}}`,
		`{"type":"user","uuid":"11111111-0000-0000-0000-000000000003","timestamp":"2026-06-01T10:06:00Z","message":{"role":"user","content":"[TOOL_RESULT] exit 0"}}`,
		`{"type":"user","uuid":"11111111-0000-0000-0000-000000000004","timestamp":"2026-06-01T10:07:00Z","message":{"role":"user","content":"<task-notification>agent done</task-notification>"}}`,
		`{"type":"user","uuid":"11111111-0000-0000-0000-000000000005","timestamp":"2026-06-01T10:08:00Z","message":{"role":"user","content":"[Request interrupted by user for tool use]"}}`,
	}
	if err := os.WriteFile(filepath.Join(dir, "sessnow.jsonl"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rows := Browse(dir, 10, "", "")
	if len(rows) != 1 {
		t.Fatalf("expected 1 session, got %d", len(rows))
	}
	r := rows[0]
	// The opening stays the identity line.
	if !strings.Contains(r.Preview, "set up the deployment pipeline") {
		t.Errorf("Preview = %q, want the session's opening", r.Preview)
	}
	// Last must skip the tool result, the envelope AND the interruption marker,
	// landing on the newest real message.
	if !strings.Contains(r.Last, "nightly job now repins") {
		t.Errorf("Last = %q, want the newest real message", r.Last)
	}
	for _, banned := range []string{"TOOL_RESULT", "task-notification", "interrupted by user"} {
		if strings.Contains(r.Last, banned) {
			t.Errorf("Last = %q, leaked machinery %q", r.Last, banned)
		}
	}
}

// TestSessionLastActivityEmptyWhenTailIsAllMachinery: honest silence. A session
// whose entire tail is generated material gets no "now" line rather than a row
// captioned with a tool result.
func TestSessionLastActivityEmptyWhenTailIsAllMachinery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	lines := []string{
		`{"type":"user","uuid":"22222222-0000-0000-0000-000000000001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"[TOOL_RESULT] only machinery here"}}`,
		`{"type":"user","uuid":"22222222-0000-0000-0000-000000000002","timestamp":"2026-06-01T10:01:00Z","message":{"role":"user","content":"<system-reminder>nudge</system-reminder>"}}`,
	}
	if err := os.WriteFile(filepath.Join(dir, "sessnoise.jsonl"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows := Browse(dir, 10, "", "")
	if len(rows) != 1 {
		t.Fatalf("expected 1 session, got %d", len(rows))
	}
	if rows[0].Last != "" {
		t.Errorf("Last = %q, want empty (tail is all machinery)", rows[0].Last)
	}
}

// TestSessionLastActivitySkipsRedactedThinking: a model that withholds its
// reasoning still emits a thinking block, and the parser writes the label with
// an empty body. That row survives the machinery strip — it is model output,
// not a tool run or an injected envelope — so it used to caption a session
// "now → [THINKING]", which says nothing.
//
// This is the common shape, not an edge case: about 99.5% of thinking records
// in the live stores are bare. The fix filters on emptiness rather than on
// block type, so thinking that carries text still captions the row — asserted
// in the second half of this test, which is what keeps the fix from
// overreaching into "hide all reasoning".
func TestSessionLastActivitySkipsRedactedThinking(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	lines := []string{
		`{"type":"user","uuid":"33333333-0000-0000-0000-000000000001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"reconcile the spec"}}`,
		`{"type":"assistant","uuid":"33333333-0000-0000-0000-000000000002","timestamp":"2026-06-01T10:05:00Z","message":{"role":"assistant","content":"the eight commits are on the branch"}}`,
		`{"type":"assistant","uuid":"33333333-0000-0000-0000-000000000003","timestamp":"2026-06-01T10:06:00Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":""}]}}`,
	}
	if err := os.WriteFile(filepath.Join(dir, "sessredact.jsonl"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows := Browse(dir, 10, "", "")
	if len(rows) != 1 {
		t.Fatalf("expected 1 session, got %d", len(rows))
	}
	if strings.Contains(rows[0].Last, "[THINKING]") {
		t.Errorf("Last = %q, captioned the session with a bare block label", rows[0].Last)
	}
	if !strings.Contains(rows[0].Last, "eight commits are on the branch") {
		t.Errorf("Last = %q, want the newest message that actually says something", rows[0].Last)
	}

	// The other half of the contract: reasoning WITH text is real activity and
	// must still caption the row. Without this, the fix above would pass just as
	// well if it hid every thinking block.
	dir2 := t.TempDir()
	lines2 := []string{
		`{"type":"user","uuid":"44444444-0000-0000-0000-000000000001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"reconcile the spec"}}`,
		`{"type":"assistant","uuid":"44444444-0000-0000-0000-000000000002","timestamp":"2026-06-01T10:05:00Z","message":{"role":"assistant","content":"an older reply"}}`,
		`{"type":"assistant","uuid":"44444444-0000-0000-0000-000000000003","timestamp":"2026-06-01T10:06:00Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"the cross-database merge is unsound"}]}}`,
	}
	if err := os.WriteFile(filepath.Join(dir2, "sessthink.jsonl"),
		[]byte(strings.Join(lines2, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows2 := Browse(dir2, 10, "", "")
	if len(rows2) != 1 {
		t.Fatalf("expected 1 session, got %d", len(rows2))
	}
	if !strings.Contains(rows2[0].Last, "cross-database merge is unsound") {
		t.Errorf("Last = %q, want the populated thinking block", rows2[0].Last)
	}
}

// TestBookendsAreConversationOnly pins the rule that closed the gap between
// search and display: a session's opening and closing bookends show what a
// person or a model said, never the runtime's injected machinery.
//
// The fixture is the shape the live corpus actually produces at a session's
// edges — an injected handbook and a slash-command echo at the head, bare
// [THINKING] markers (~99.5% of stored thinking rows) at the tail — which is
// why reading only Bookend rows returned a bookend of leftovers before this.
func TestBookendsAreConversationOnly(t *testing.T) {
	const sid = "sess-bookend"
	msgs := []seedMsg{
		{1, "user", "<system-reminder>handbook preamble injected here</system-reminder>"},
		{2, "user", "<local-command-stdout>See ya!</local-command-stdout>"},
		{3, "assistant", "[THINKING]"},
		{4, "user", "real opening request"},
		{5, "assistant", "real opening answer"},
		{6, "user", "middle one"},
		{7, "assistant", "anchor message"},
		{8, "user", "middle two"},
		{9, "assistant", "real closing answer"},
		{10, "user", "real closing word"},
		{11, "assistant", "[THINKING]"},
		{12, "user", "<task-notification>agent finished</task-notification>"},
	}
	con := newTestDB(t, sid, msgs)
	defer con.Close()

	av := BuildAnchoredView(con, sid, 7, AnchoredViewOpts{Window: 0, Bookend: 2})
	if av == nil {
		t.Fatal("BuildAnchoredView = nil, want a view")
	}
	if got, want := ids(av.BookendStart), []int{4, 5}; !eqInts(got, want) {
		t.Errorf("bookend_start = %v, want %v (injected rows must not open a session)", got, want)
	}
	if got, want := ids(av.BookendEnd), []int{9, 10}; !eqInts(got, want) {
		t.Errorf("bookend_end = %v, want %v (bare markers and notifications must not close a session)", got, want)
	}

	// Nothing is hidden from the store: --include-tools puts every row back,
	// exactly as it does for search.
	raw := BuildAnchoredView(con, sid, 7, AnchoredViewOpts{Window: 0, Bookend: 2, IncludeTools: true})
	if got, want := ids(raw.BookendStart), []int{1, 2}; !eqInts(got, want) {
		t.Errorf("bookend_start with tools = %v, want %v (suppression is display, not deletion)", got, want)
	}
}

// TestAnchorSurvivesEvenWhenGenerated: a caller who names a record by ref gets
// that record. Refusing to render the row someone asked for by id would be a
// worse failure than showing them machinery they explicitly requested.
func TestAnchorSurvivesEvenWhenGenerated(t *testing.T) {
	const sid = "sess-anchor"
	msgs := []seedMsg{
		{1, "user", "opening"},
		{2, "user", "<task-notification>the anchor is pure machinery</task-notification>"},
		{3, "user", "closing"},
	}
	con := newTestDB(t, sid, msgs)
	defer con.Close()

	av := BuildAnchoredView(con, sid, 2, AnchoredViewOpts{Window: 1, Bookend: 0})
	if av == nil {
		t.Fatal("BuildAnchoredView = nil, want a view")
	}
	var anchor *ViewMsg
	for i := range av.Window {
		if av.Window[i].Anchor {
			anchor = &av.Window[i]
		}
	}
	if anchor == nil {
		t.Fatal("anchor absent from window; a named ref must always render")
	}
	if anchor.Text == "" {
		t.Error("anchor rendered empty; want the raw record when stripping empties it")
	}
}

func TestIsDisplayable(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"plain conversation", "what did we decide about the store?", true},
		{"injected envelope only", "<system-reminder>be helpful</system-reminder>", false},
		{"captured command output", "<local-command-stdout>See ya!</local-command-stdout>", false},
		{"bare thinking marker", "[THINKING]", false},
		{"thinking with a body", "[THINKING] the bm25 scores are not comparable", true},
		{"empty", "", false},
		{"envelope wrapping real text", "<system-reminder>noise</system-reminder> the real question", true},
	}
	for _, tt := range tests {
		if got := IsDisplayable(tt.content); got != tt.want {
			t.Errorf("IsDisplayable(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}
