package view

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/retrieve"

	_ "modernc.org/sqlite"
)

// seedMsg is one row to insert into the messages table for a test fixture.
type seedMsg struct {
	id      int
	role    string
	content string
}

// newTestDB opens an in-memory SQLite db with a messages table and inserts
// the given rows at explicit ids. The table carries the columns view reads:
// id, session_id, role, content.
func newTestDB(t *testing.T, sessionID string, msgs []seedMsg) *sql.DB {
	t.Helper()
	con, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	con.SetMaxOpenConns(1) // :memory: lives on a single conn
	t.Cleanup(func() { con.Close() })

	_, err = con.Exec(`CREATE TABLE messages (
		id INTEGER PRIMARY KEY, session_id TEXT NOT NULL,
		role TEXT, content TEXT, ts REAL, ts_iso TEXT)`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, m := range msgs {
		if _, err := con.Exec(
			`INSERT INTO messages (id, session_id, role, content) VALUES (?,?,?,?)`,
			m.id, sessionID, m.role, m.content); err != nil {
			t.Fatalf("insert id=%d: %v", m.id, err)
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

// newPreviewDB builds an in-memory db with the sessions + messages tables that
// sessionPreview reads, seeding one session's worth of user messages.
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
			if got := sessionPreview(con, sid); got != tt.want {
				t.Errorf("sessionPreview() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSessionPreviewKeepsSessionWithGreetingOpener is the spec's headline case:
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
	got := sessionPreview(con, sid)
	if got == "" {
		t.Fatal("session with greeting opener produced empty preview (session effectively dropped)")
	}
	if got != "wire up the discovery dedup" {
		t.Errorf("preview = %q, want the first substantive turn", got)
	}
}

func TestSortCandidates(t *testing.T) {
	mk := func(rank int, iso string, fused float64, cov int) retrieve.Anchor {
		return retrieve.Anchor{Rank: rank, ISO: iso, Fused: fused, Cov: cov}
	}

	tests := []struct {
		name     string
		mode     string
		in       []retrieve.Anchor
		wantRank []int // expected order by original Rank
	}{
		{
			name:     "relevance: fused desc, cov desc, rank asc",
			mode:     "",
			in:       []retrieve.Anchor{mk(0, "", 0.1, 2), mk(1, "", 0.3, 1), mk(2, "", 0.3, 5)},
			wantRank: []int{2, 1, 0}, // fused .3 group first; within it cov 5>1; then .1
		},
		{
			name:     "relevance tiebreak by rank when fused+cov equal",
			mode:     "",
			in:       []retrieve.Anchor{mk(3, "", 0.0, 0), mk(1, "", 0.0, 0), mk(2, "", 0.0, 0)},
			wantRank: []int{1, 2, 3},
		},
		{
			name:     "newest: iso desc, empty sinks",
			mode:     "newest",
			in:       []retrieve.Anchor{mk(0, "2026-01-01", 0, 0), mk(1, "", 0, 0), mk(2, "2026-06-01", 0, 0)},
			wantRank: []int{2, 0, 1},
		},
		{
			name:     "oldest: iso asc, empty floats",
			mode:     "oldest",
			in:       []retrieve.Anchor{mk(0, "2026-01-01", 0, 0), mk(1, "", 0, 0), mk(2, "2026-06-01", 0, 0)},
			wantRank: []int{1, 0, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := append([]retrieve.Anchor(nil), tt.in...)
			sortCandidates(cs, tt.mode)
			got := make([]int, len(cs))
			for i, c := range cs {
				got[i] = c.Rank
			}
			if !eqInts(got, tt.wantRank) {
				t.Errorf("order by rank = %v, want %v", got, tt.wantRank)
			}
		})
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

// TestScrollAmbiguous mirrors the agentproto ambiguity guard: a session8 prefix
// matching ≥2 sessions returns *ErrAmbiguousScroll and resolves none.
func TestScrollAmbiguous(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	writeScrollSession(t, proj, "a1b2c3d4aaaa", "alpha")
	writeScrollSession(t, proj, "a1b2c3d4bbbb", "beta")
	scope := []Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}

	res, err := Scroll(scope, "a1b2c3d4", 1, 5)
	if res != nil {
		t.Errorf("ambiguous Scroll should resolve none, got %+v", res)
	}
	var amb *ErrAmbiguousScroll
	if !errors.As(err, &amb) {
		t.Fatalf("want *ErrAmbiguousScroll, got %T: %v", err, err)
	}
	if len(amb.Candidates) != 2 {
		t.Errorf("ambiguous candidates = %d, want 2", len(amb.Candidates))
	}
}

// TestScrollUnique: a non-colliding prefix resolves to exactly that session.
func TestScrollUnique(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	writeScrollSession(t, proj, "a1b2c3d4aaaa", "alpha")
	writeScrollSession(t, proj, "ffff0000bbbb", "beta")
	scope := []Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}

	res, err := Scroll(scope, "a1b2c3d4", 1, 5)
	if err != nil {
		t.Fatalf("unique Scroll: unexpected err %v", err)
	}
	if res == nil || res.SessionID != "a1b2c3d4aaaa" {
		t.Fatalf("unique Scroll resolved wrong session: %+v", res)
	}
}

// writeSubagentSession writes a subagent transcript under
// <proj>/<parent>/subagents/<stem>.jsonl, which the indexer flags is_subagent=1
// with parent=<parent> (see index.SessionIDFor).
func writeSubagentSession(t *testing.T, proj, parent, stem, content string) {
	t.Helper()
	dir := filepath.Join(proj, parent, "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	line := `{"type":"user","uuid":"u-` + stem + `","timestamp":"2026-06-01T10:00:00Z",` +
		`"message":{"role":"user","content":"` + content + `"}}`
	path := filepath.Join(dir, stem+".jsonl")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestScrollIgnoresSubagentSibling: a bare session UUID that also prefixes the
// session's OWN subagent transcript must resolve to the parent, not false-trip
// the ambiguity guard against its child. The two share the UUID prefix but are
// one logical session. Regression for the Scroll is_subagent filter.
func TestScrollIgnoresSubagentSibling(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	writeScrollSession(t, proj, "a1b2c3d4aaaa", "alpha")
	writeSubagentSession(t, proj, "a1b2c3d4aaaa", "agentchild", "child")
	scope := []Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}

	res, err := Scroll(scope, "a1b2c3d4", 1, 5)
	if err != nil {
		t.Fatalf("scroll past subagent sibling: unexpected err %v", err)
	}
	if res == nil || res.SessionID != "a1b2c3d4aaaa" {
		t.Fatalf("want parent session a1b2c3d4aaaa, got %+v", res)
	}
}
