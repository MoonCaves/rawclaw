package agentproto

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// writeSession writes a single-message transcript file <stem>.jsonl under proj
// with the given uuid + content. The filename stem becomes the session id.
func writeSession(t *testing.T, proj, stem, uuid, content string) {
	t.Helper()
	line := `{"type":"user","uuid":"` + uuid + `","timestamp":"2026-06-01T10:00:00Z",` +
		`"message":{"role":"user","content":"` + content + `"}}`
	path := filepath.Join(proj, stem+".jsonl")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// scopeFor builds a single-project scope rooted at proj, with an isolated cache
// HOME so the index db lands in a temp dir.
func scopeFor(t *testing.T, proj string) []view.Scope {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
}

func TestResolveRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantSID  string
		wantUUID string
		wantErr  string // substring; "" = no error
	}{
		{name: "valid", ref: "a1b2c3d4:9f3e1c20", wantSID: "a1b2c3d4", wantUUID: "9f3e1c20"},
		{name: "uppercase hex folded", ref: "a1b2c3d4:9F3E1C20", wantSID: "a1b2c3d4", wantUUID: "9f3e1c20"},
		{name: "short uuid prefix", ref: "abc:9f3e", wantSID: "abc", wantUUID: "9f3e"},
		{name: "no colon", ref: "a1b2c3d4", wantErr: "expected <session8>"},
		{name: "too many colons", ref: "a:b:c", wantErr: "expected <session8>"},
		{name: "empty uuid", ref: "abc:", wantErr: "expected <session8>"},
		{name: "old numeric ref", ref: "a1b2c3d4:42", wantErr: "old numeric ref"},
		{name: "non-hex uuid", ref: "abc:xyz", wantErr: "must be hex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, uuid, err := resolveRef(tt.ref)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveRef(%q) err = %v, want substring %q", tt.ref, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRef(%q) unexpected err: %v", tt.ref, err)
			}
			if sid != tt.wantSID || uuid != tt.wantUUID {
				t.Fatalf("resolveRef(%q) = (%q, %q), want (%q, %q)", tt.ref, sid, uuid, tt.wantSID, tt.wantUUID)
			}
		})
	}
}

func TestFmtRef(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		uuid      string
		want      string
	}{
		{name: "long both truncated to 8", sessionID: "a1b2c3d4e5f6", uuid: "9f3e1c20aaaa", want: "a1b2c3d4:9f3e1c20"},
		{name: "exactly 8 each", sessionID: "12345678", uuid: "9f3e1c20", want: "12345678:9f3e1c20"},
		{name: "short uuid no pad", sessionID: "abc", uuid: "9f3e", want: "abc:9f3e"},
		{name: "uuid with dashes truncates before dash", sessionID: "deadbeef00", uuid: "9f3e1c20-aaaa", want: "deadbeef:9f3e1c20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmtRef(tt.sessionID, tt.uuid); got != tt.want {
				t.Fatalf("fmtRef(%q, %q) = %q, want %q", tt.sessionID, tt.uuid, got, tt.want)
			}
		})
	}
}

func TestSid8(t *testing.T) {
	if got := sid8("αβγδεζηθικ"); got != "αβγδεζηθ" {
		t.Fatalf("sid8 multibyte: got %q want %q", got, "αβγδεζηθ")
	}
	if got := sid8("ab"); got != "ab" {
		t.Fatalf("sid8 short: got %q want %q", got, "ab")
	}
}

func intp(n int) *int { return &n }

func TestApplyBudget(t *testing.T) {
	tests := []struct {
		name          string
		window        []view.ViewMsg
		budget        *int
		wantTexts     []string
		wantTruncated bool
	}{
		{
			name:          "nil budget no cap",
			window:        []view.ViewMsg{{ID: 1, Text: "hello"}, {ID: 2, Text: "world"}},
			budget:        nil,
			wantTexts:     []string{"hello", "world"},
			wantTruncated: false,
		},
		{
			name:          "budget larger than total",
			window:        []view.ViewMsg{{ID: 1, Text: "hello"}, {ID: 2, Text: "world"}},
			budget:        intp(100),
			wantTexts:     []string{"hello", "world"},
			wantTruncated: false,
		},
		{
			name:          "budget cuts second message",
			window:        []view.ViewMsg{{ID: 1, Text: "hello"}, {ID: 2, Text: "world"}},
			budget:        intp(7), // "hello"=5, then available=2 for "world" -> "wo"+marker
			wantTexts:     []string{"hello", "wo" + truncateMarker},
			wantTruncated: true,
		},
		{
			name:          "budget exhausted drops remaining",
			window:        []view.ViewMsg{{ID: 1, Text: "hello"}, {ID: 2, Text: "world"}},
			budget:        intp(5), // "hello" fills it; total>=5 drops "world"
			wantTexts:     []string{"hello"},
			wantTruncated: true,
		},
		{
			name:          "first message truncated",
			window:        []view.ViewMsg{{ID: 1, Text: "abcdefgh"}},
			budget:        intp(3),
			wantTexts:     []string{"abc" + truncateMarker},
			wantTruncated: true,
		},
		{
			name:          "rstrip before marker",
			window:        []view.ViewMsg{{ID: 1, Text: "ab   cdef"}},
			budget:        intp(5), // "ab   " -> rstrip "ab" + marker
			wantTexts:     []string{"ab" + truncateMarker},
			wantTruncated: true,
		},
		{
			name:          "multibyte rune budget",
			window:        []view.ViewMsg{{ID: 1, Text: "αβγδε"}},
			budget:        intp(3), // 3 runes, not bytes
			wantTexts:     []string{"αβγ" + truncateMarker},
			wantTruncated: true,
		},
		{
			name:          "zero budget drops everything",
			window:        []view.ViewMsg{{ID: 1, Text: "x"}},
			budget:        intp(0),
			wantTexts:     []string{},
			wantTruncated: true,
		},
		{
			name:          "exact fit no truncate",
			window:        []view.ViewMsg{{ID: 1, Text: "abc"}},
			budget:        intp(3),
			wantTexts:     []string{"abc"},
			wantTruncated: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			av := &view.AnchoredView{Window: append([]view.ViewMsg(nil), tt.window...)}
			st := applyBudget(av, tt.budget)
			if st.Truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", st.Truncated, tt.wantTruncated)
			}
			gotTexts := make([]string, len(av.Window))
			for i, m := range av.Window {
				gotTexts[i] = m.Text
			}
			if len(gotTexts) != len(tt.wantTexts) {
				t.Fatalf("window texts = %q, want %q", gotTexts, tt.wantTexts)
			}
			for i := range gotTexts {
				if gotTexts[i] != tt.wantTexts[i] {
					t.Errorf("window[%d] = %q, want %q", i, gotTexts[i], tt.wantTexts[i])
				}
			}
		})
	}
}

func TestFocusHighlight(t *testing.T) {
	tests := []struct {
		name   string
		window []view.ViewMsg
		focus  string
		want   string
	}{
		{
			name:   "empty focus returns empty",
			window: []view.ViewMsg{{ID: 1, Role: "user", Text: "the api key is secret"}},
			focus:  "",
			want:   "",
		},
		{
			name:   "no match returns empty",
			window: []view.ViewMsg{{ID: 1, Role: "user", Text: "hello world"}},
			focus:  "zzz",
			want:   "",
		},
		{
			name:   "case-insensitive match wraps",
			window: []view.ViewMsg{{ID: 3, Role: "assistant", Text: "the API key"}},
			focus:  "api",
			want:   "[#3 assistant] the >>>API<<< key",
		},
		{
			name: "first matching message wins",
			window: []view.ViewMsg{
				{ID: 1, Role: "user", Text: "no hit here"},
				{ID: 2, Role: "assistant", Text: "found target word"},
			},
			focus: "target",
			want:  "[#2 assistant] found >>>target<<< word",
		},
		{
			name:   "regex metachars in focus are literal",
			window: []view.ViewMsg{{ID: 9, Role: "user", Text: "value a.b matched"}},
			focus:  "a.b",
			want:   "[#9 user] value >>>a.b<<< matched",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := focusHighlight(tt.window, tt.focus); got != tt.want {
				t.Fatalf("focusHighlight = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFocusHighlightWindowClamp(t *testing.T) {
	// A long prefix: ensure the window is clamped to 60 runes before the match
	// and 120 after the match start.
	prefix := strings.Repeat("x", 200)
	text := prefix + "NEEDLE" + strings.Repeat("y", 200)
	got := focusHighlight([]view.ViewMsg{{ID: 1, Role: "user", Text: text}}, "needle")
	if !strings.Contains(got, ">>>NEEDLE<<<") {
		t.Fatalf("highlight missing: %q", got)
	}
	// 60 x's before, then >>>NEEDLE<<<, then 120-6=114 y's after the match start.
	wantPrefix := "[#1 user] " + strings.Repeat("x", 60) + ">>>NEEDLE<<<"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("got %q, want prefix %q", got, wantPrefix)
	}
	if strings.Count(got, "y") != 114 {
		t.Fatalf("got %d trailing y's, want 114 (idx+120 window): %q", strings.Count(got, "y"), got)
	}
}

func TestSortCandidates(t *testing.T) {
	t.Run("newest by iso desc", func(t *testing.T) {
		cands := anchors([]anchorLite{
			{sid: "a", iso: "2026-01-01"},
			{sid: "b", iso: "2026-03-01"},
			{sid: "c", iso: "2026-02-01"},
		})
		sortCandidates(cands, "newest")
		got := sidsOf(cands)
		want := []string{"b", "c", "a"}
		assertOrder(t, got, want)
	})
	t.Run("oldest by iso asc", func(t *testing.T) {
		cands := anchors([]anchorLite{
			{sid: "a", iso: "2026-01-01"},
			{sid: "b", iso: "2026-03-01"},
			{sid: "c", iso: "2026-02-01"},
		})
		sortCandidates(cands, "oldest")
		assertOrder(t, sidsOf(cands), []string{"a", "c", "b"})
	})
	t.Run("relevance by cov desc then rank", func(t *testing.T) {
		cands := anchors([]anchorLite{
			{sid: "a", cov: 1, rank: 0},
			{sid: "b", cov: 3, rank: 1},
			{sid: "c", cov: 3, rank: 2},
		})
		sortCandidates(cands, "")
		// cov 3 first (b before c by rank tiebreak), then cov 1.
		assertOrder(t, sidsOf(cands), []string{"b", "c", "a"})
	})
	t.Run("relevance stable on equal keys", func(t *testing.T) {
		cands := anchors([]anchorLite{
			{sid: "a", cov: 2, rank: 0},
			{sid: "b", cov: 2, rank: 0},
		})
		sortCandidates(cands, "")
		assertOrder(t, sidsOf(cands), []string{"a", "b"})
	})
}

func TestPopBool(t *testing.T) {
	a := []string{"x", "--json", "y"}
	if !popBool(&a, "--json") {
		t.Fatal("popBool should find --json")
	}
	if strings.Join(a, " ") != "x y" {
		t.Fatalf("after pop: %v", a)
	}
	if popBool(&a, "--missing") {
		t.Fatal("popBool should not find --missing")
	}
}

func TestPopVal(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flag     string
		wantVal  string
		wantRest string
	}{
		{name: "value present", args: []string{"--limit", "10", "rest"}, flag: "--limit", wantVal: "10", wantRest: "rest"},
		{name: "flag absent", args: []string{"a", "b"}, flag: "--limit", wantVal: "", wantRest: "a b"},
		// A trailing valueless flag is left in place (no following arg to consume).
		{name: "flag at end no value", args: []string{"a", "--budget"}, flag: "--budget", wantVal: "", wantRest: "a --budget"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := append([]string(nil), tt.args...)
			got := popVal(&a, tt.flag)
			if got != tt.wantVal {
				t.Errorf("popVal val = %q, want %q", got, tt.wantVal)
			}
			if strings.Join(a, " ") != tt.wantRest {
				t.Errorf("popVal rest = %q, want %q", strings.Join(a, " "), tt.wantRest)
			}
		})
	}
}

func TestEmitJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := emit(&buf, []SearchRef{{Project: "p", SessionID: "s", ISO: "2026", Snippet: "café <b>", ReadRef: "s:1"}}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	out := buf.String()
	// ensure_ascii=False: non-ASCII stays literal.
	if !strings.Contains(out, "café") {
		t.Errorf("expected literal unicode, got: %s", out)
	}
	// SetEscapeHTML(false): '<' must not become <.
	if !strings.Contains(out, "<b>") {
		t.Errorf("expected unescaped HTML chars, got: %s", out)
	}
	// indent=2: nested keys are two-space indented.
	if !strings.Contains(out, "\n  {\n    \"project\": \"p\"") {
		t.Errorf("expected two-space indent, got: %s", out)
	}
}

func TestRenderSearch(t *testing.T) {
	t.Run("no matches", func(t *testing.T) {
		var buf bytes.Buffer
		renderSearch(&buf, SearchEnvelope{Results: []SearchRef{}, Complete: true}, "q", "across all projects")
		want := "No matches. Lead with a single distinctive term that appears in the text (a filename, flag, or error string), not a topic word — or rephrase.\n"
		if buf.String() != want {
			t.Fatalf("got %q, want %q", buf.String(), want)
		}
	})
	t.Run("with results", func(t *testing.T) {
		var buf bytes.Buffer
		renderSearch(&buf, SearchEnvelope{Complete: true, Results: []SearchRef{
			{Project: "proj", SessionID: "a1b2c3d4e5", ISO: "2026-06-18", Snippet: "snip", ReadRef: "a1b2c3d4:9f3e1c20"},
			{Project: "proj2", SessionID: "ffff", ISO: "", Snippet: "s2", ReadRef: "ffff:1a2b"},
		}}, "kw", "on this project")
		out := buf.String()
		if !strings.HasPrefix(out, "2 conversation(s) matching 'kw' on this project:\n\n") {
			t.Fatalf("header wrong: %q", out)
		}
		if !strings.Contains(out, "  ━━ 2026-06-18 · a1b2c3d4 · proj\n") {
			t.Errorf("missing first ref line: %q", out)
		}
		if !strings.Contains(out, "     …snip…\n") {
			t.Errorf("missing snippet: %q", out)
		}
		if !strings.Contains(out, "     read ref=a1b2c3d4:9f3e1c20\n\n") {
			t.Errorf("missing read ref: %q", out)
		}
		// Empty ISO renders as "?".
		if !strings.Contains(out, "  ━━ ? · ffff · proj2\n") {
			t.Errorf("empty iso should render ?: %q", out)
		}
	})
	t.Run("incomplete scope footer", func(t *testing.T) {
		var buf bytes.Buffer
		renderSearch(&buf, SearchEnvelope{
			Complete: false,
			Results:  []SearchRef{{Project: "p", SessionID: "aaaa", ISO: "2026", Snippet: "s", ReadRef: "aaaa:9f"}},
			Scopes: []ScopeReport{
				{Project: "p", Status: ScopeSearched},
				{Project: "q", Status: ScopeSkippedError, Detail: "boom"},
				{Project: "r", Status: ScopeStaleFallback},
			},
		}, "kw", "across all projects")
		out := buf.String()
		if !strings.Contains(out, "note: 2 of 3 projects incomplete (1 error, 1 stale)") {
			t.Errorf("missing incompleteness footer: %q", out)
		}
	})
}

func TestRenderRead(t *testing.T) {
	r := &ReadResult{
		Project:      "proj",
		SessionID:    "a1b2c3d4e5",
		AnchorID:     7,
		FocusSnippet: "[#7 user] >>>hit<<<",
		Truncated:    true,
		TrimmedChars: 1800,
		TrimmedMsgs:  2,
		NextCommand:  "rawclaw agent read a1b2c3d4:9f3e1c20 --more",
		AnchoredView: &view.AnchoredView{
			BookendStart:   []view.ViewMsg{{ID: 1, Role: "user", Text: "start"}},
			Window:         []view.ViewMsg{{ID: 7, Role: "user", Text: "anchored", Anchor: true}, {ID: 8, Role: "assistant", Text: "after"}},
			BookendEnd:     []view.ViewMsg{{ID: 99, Role: "assistant", Text: "end"}},
			MessagesBefore: 6,
			MessagesAfter:  4,
		},
	}
	var buf bytes.Buffer
	renderRead(&buf, r)
	out := buf.String()

	checks := []string{
		"━━ a1b2c3d4 · proj · anchor #7 (6 before / 4 after) ━━\n",
		"  focus match: [#7 user] >>>hit<<<\n",
		"  ─ session start ─\n",
		"       [user #1] start\n", // non-anchor: 5 spaces + " " + space
		"     ▶ [user #7] anchored\n",
		"       [assistant #8] after\n",
		"  ─ session end ─\n",
		"       [assistant #99] end\n",
		// Never-silent trim: the note carries the omitted counts AND the literal
		// recovery command (no bare "…[truncated]", no dead --no-budget hint).
		"  [+1.8k chars · 2 msgs hidden — rawclaw agent read a1b2c3d4:9f3e1c20 --more]\n",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("render output missing %q\n--- full ---\n%s", c, out)
		}
	}
	// When truncated, the generic scroll hint is replaced by the recovery note.
	if strings.Contains(out, "scroll more:") {
		t.Errorf("truncated render should not also print the generic scroll hint\n--- full ---\n%s", out)
	}
	if strings.Contains(out, "--no-budget") {
		t.Errorf("dead --no-budget hint must be gone\n--- full ---\n%s", out)
	}
}

func TestRenderReadNoFocusNoTrunc(t *testing.T) {
	r := &ReadResult{
		Project:   "p",
		SessionID: "sid12345",
		AnchorID:  2,
		AnchoredView: &view.AnchoredView{
			Window:         []view.ViewMsg{{ID: 2, Role: "user", Text: "only", Anchor: true}},
			MessagesBefore: 0,
			MessagesAfter:  0,
		},
	}
	var buf bytes.Buffer
	renderRead(&buf, r)
	out := buf.String()
	if strings.Contains(out, "focus match") {
		t.Errorf("should not print focus line: %q", out)
	}
	if strings.Contains(out, "budget reached") {
		t.Errorf("should not print budget note: %q", out)
	}
}

func TestRenderOutline(t *testing.T) {
	tests := []struct {
		name    string
		res     *OutlineResult
		want    []string
		notWant []string
	}{
		{
			name: "full arc with mid",
			res: &OutlineResult{
				Project: "proj", SessionID: "a1b2c3d4xx", ISO: "2026-06-18", MessageCount: 50,
				Start:    []view.ViewMsg{{ID: 1, Role: "user", Text: "goal"}},
				End:      []view.ViewMsg{{ID: 49, Role: "assistant", Text: "done"}},
				MidCount: 47,
			},
			want: []string{
				"━━ 2026-06-18 · a1b2c3d4 · proj · 50 messages ━━\n\n",
				"  ── GOAL (session opening) ──\n",
				"     [user #1] goal\n",
				"\n  … 47 messages in between …\n\n",
				"  ── RESOLUTION (session close) ──\n",
				"     [assistant #49] done\n",
			},
		},
		{
			name: "no mid no end, empty iso",
			res: &OutlineResult{
				Project: "p", SessionID: "shortid", ISO: "", MessageCount: 2,
				Start:    []view.ViewMsg{{ID: 1, Role: "user", Text: "hi"}},
				End:      []view.ViewMsg{},
				MidCount: 0,
			},
			want:    []string{"━━ ? · shortid · p · 2 messages ━━\n\n"},
			notWant: []string{"messages in between", "RESOLUTION"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderOutline(&buf, tt.res)
			out := buf.String()
			for _, c := range tt.want {
				if !strings.Contains(out, c) {
					t.Errorf("missing %q\n--- full ---\n%s", c, out)
				}
			}
			for _, c := range tt.notWant {
				if strings.Contains(out, c) {
					t.Errorf("should not contain %q\n--- full ---\n%s", c, out)
				}
			}
		})
	}
}

func TestRunNoArgs(t *testing.T) {
	var out, errw bytes.Buffer
	code := Run([]string{}, &out, &errw)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errw.String(), "usage: rawclaw agent") {
		t.Fatalf("expected usage on stderr, got %q", errw.String())
	}
}

func TestRunUnknownVerb(t *testing.T) {
	var out, errw bytes.Buffer
	code := Run([]string{"frobnicate"}, &out, &errw)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errw.String(), "unknown verb 'frobnicate'") {
		t.Fatalf("expected unknown-verb error, got %q", errw.String())
	}
}

func TestRunVerbMissingPositional(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "search no query", args: []string{"search", "--json"}, want: "usage: rawclaw agent search <query>"},
		{name: "read no ref", args: []string{"read"}, want: "usage: rawclaw agent read <session8:uuid8>"},
		{name: "outline no id", args: []string{"outline"}, want: "usage: rawclaw agent outline <session8>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errw bytes.Buffer
			code := Run(tt.args, &out, &errw)
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !strings.Contains(errw.String(), tt.want) {
				t.Errorf("stderr = %q, want substring %q", errw.String(), tt.want)
			}
		})
	}
}

func TestRunReadBadRef(t *testing.T) {
	// A malformed ref fails in resolveRef before any DB access — exercises the
	// read error path end-to-end through Run.
	var out, errw bytes.Buffer
	code := Run([]string{"read", "not-a-ref"}, &out, &errw)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errw.String(), "bad ref") {
		t.Fatalf("stderr = %q, want bad ref error", errw.String())
	}
}

func TestLocateSessionUnique(t *testing.T) {
	proj := t.TempDir()
	// Two sessions with DIFFERENT 8-char prefixes — a session8 lookup is unique.
	writeSession(t, proj, "a1b2c3d4eeee", "uuid-a-1", "first session")
	writeSession(t, proj, "ffff0000zzzz", "uuid-b-1", "second session")
	scope := scopeFor(t, proj)

	dbp, fullSID, _, err := locateSession(scope, "a1b2c3d4")
	if err != nil {
		t.Fatalf("locateSession unique: unexpected err %v", err)
	}
	if dbp == "" || fullSID != "a1b2c3d4eeee" {
		t.Fatalf("locateSession = (%q, %q), want full sid a1b2c3d4eeee", dbp, fullSID)
	}
}

func TestLocateSessionAmbiguous(t *testing.T) {
	proj := t.TempDir()
	// Two sessions SHARING the first 8 chars — the prefix is ambiguous.
	writeSession(t, proj, "a1b2c3d4aaaa", "uuid-a-1", "alpha session")
	writeSession(t, proj, "a1b2c3d4bbbb", "uuid-b-1", "beta session")
	scope := scopeFor(t, proj)

	_, _, _, err := locateSession(scope, "a1b2c3d4")
	if err == nil {
		t.Fatal("locateSession should reject an ambiguous prefix, got nil err")
	}
	var amb *ErrAmbiguousSession
	if !errors.As(err, &amb) {
		t.Fatalf("want *ErrAmbiguousSession, got %T: %v", err, err)
	}
	if len(amb.Candidates) != 2 {
		t.Errorf("ambiguous candidates = %d, want 2", len(amb.Candidates))
	}
	// Resolves none, lists both — git-style.
	msg := amb.Error()
	if !strings.Contains(msg, "ambiguous session prefix") || !strings.Contains(msg, "longer prefix") {
		t.Errorf("error message not git-style: %q", msg)
	}
}

func TestLocateSessionNotFound(t *testing.T) {
	proj := t.TempDir()
	writeSession(t, proj, "a1b2c3d4eeee", "uuid-a-1", "only session")
	scope := scopeFor(t, proj)

	_, _, _, err := locateSession(scope, "zzzzzzzz")
	var nf *ErrSessionNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("want *ErrSessionNotFound, got %T: %v", err, err)
	}
}

// writeMultiSession writes a multi-message transcript file <stem>.jsonl. Each
// (uuid, content) pair becomes one user message in order.
func writeMultiSession(t *testing.T, proj, stem string, msgs [][2]string) {
	t.Helper()
	var b strings.Builder
	for i, m := range msgs {
		ts := fmt.Sprintf("2026-06-01T10:%02d:00Z", i%60)
		b.WriteString(`{"type":"user","uuid":"` + m[0] + `","timestamp":"` + ts + `",` +
			`"message":{"role":"user","content":"` + m[1] + `"}}` + "\n")
	}
	path := filepath.Join(proj, stem+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReadSingleMessageWholeByDefault(t *testing.T) {
	proj := t.TempDir()
	long := strings.Repeat("x", 9000) // exceeds the old DefaultReadBudget of 4000
	writeMultiSession(t, proj, "a1b2c3d4eeee", [][2]string{{"aaaa11110000", long}})
	scope := scopeFor(t, proj)

	// No --budget → whole, no truncation, even though the message is large.
	res, err := Read("a1b2c3d4:aaaa1111", scope, ReadOpts{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if res.Truncated {
		t.Errorf("default read must NOT truncate (budget flip #3)")
	}
	if res.CharBudget != nil {
		t.Errorf("default CharBudget should be nil (no cap requested), got %v", *res.CharBudget)
	}
	// The anchor message text is present whole (capped only by the per-message
	// display cap, which is independent of budget).
	var anchorLen int
	for _, m := range res.Window {
		if m.Anchor {
			anchorLen = len([]rune(m.Text))
		}
	}
	if anchorLen == 0 {
		t.Fatal("anchor message missing from window")
	}
}

func TestReadAmbiguousUUID(t *testing.T) {
	proj := t.TempDir()
	// Two messages in ONE session whose uuids share the first 8 hex chars — the
	// uuid8 prefix is ambiguous and must NOT silently resolve to one (the C2
	// guard, extended to the uuid half).
	writeMultiSession(t, proj, "a1b2c3d4eeee", [][2]string{
		{"deadbeef0001", "first message"},
		{"deadbeef0002", "second message"},
	})
	scope := scopeFor(t, proj)

	_, err := Read("a1b2c3d4:deadbeef", scope, ReadOpts{})
	if err == nil {
		t.Fatal("Read must reject an ambiguous uuid8 prefix, got nil err")
	}
	var amb *ErrAmbiguousUUID
	if !errors.As(err, &amb) {
		t.Fatalf("want *ErrAmbiguousUUID, got %T: %v", err, err)
	}
	if !strings.Contains(amb.Error(), "longer uuid prefix") {
		t.Errorf("error not git-style: %q", amb.Error())
	}
}

func TestReadBudgetIsCeilingOnly(t *testing.T) {
	proj := t.TempDir()
	// Three messages, each ~100 chars, so a small --budget trims the window.
	a := strings.Repeat("a", 100)
	b := strings.Repeat("b", 100)
	c := strings.Repeat("c", 100)
	writeMultiSession(t, proj, "a1b2c3d4eeee", [][2]string{
		{"aaaa11110000", a}, {"bbbb22220000", b}, {"cccc33330000", c},
	})
	scope := scopeFor(t, proj)

	// Absent budget → whole, no truncation.
	res, err := Read("a1b2c3d4:aaaa1111", scope, ReadOpts{})
	if err != nil {
		t.Fatalf("Read (no budget): %v", err)
	}
	if res.Truncated {
		t.Errorf("no --budget must not truncate")
	}

	// Explicit small budget → truncates the multi-message window.
	cap := 50
	res2, err := Read("a1b2c3d4:aaaa1111", scope, ReadOpts{Budget: &cap})
	if err != nil {
		t.Fatalf("Read (budget 50): %v", err)
	}
	if !res2.Truncated {
		t.Errorf("--budget 50 over a multi-message window must truncate")
	}
	if res2.CharBudget == nil || *res2.CharBudget != 50 {
		t.Errorf("CharBudget = %v, want 50", res2.CharBudget)
	}
}

// hexUUID builds a deterministic 12-hex-char uuid for message i. The first 8
// chars are unique per i (a leading 'f' guarantees a non-numeric prefix so the
// ref never looks like an old numeric ref, plus i as 7-wide hex) — so uuid8
// prefixes never collide within a fixture session.
func hexUUID(i int) string {
	return fmt.Sprintf("f%07x0000", i)
}

// writeNSession writes a session of n user messages with hex uuids, returning
// the uuid8 prefix of the anchor message at index `anchor`.
func writeNSession(t *testing.T, proj, stem string, n, anchor int) string {
	t.Helper()
	msgs := make([][2]string, 0, n)
	for i := 0; i < n; i++ {
		msgs = append(msgs, [2]string{hexUUID(i), "message number " + strings.Repeat("z", i+1)})
	}
	writeMultiSession(t, proj, stem, msgs)
	return uuid8(hexUUID(anchor))
}

func TestReadMoreWidensWindow(t *testing.T) {
	proj := t.TempDir()
	// 40 messages, anchor in the middle, so both ReadWindow and a wider --more
	// level have room to grow on each side.
	anchorUUID8 := writeNSession(t, proj, "a1b2c3d4eeee", 40, 20)
	scope := scopeFor(t, proj)
	ref := "a1b2c3d4:" + anchorUUID8

	base, err := Read(ref, scope, ReadOpts{})
	if err != nil {
		t.Fatalf("Read base: %v", err)
	}
	more, err := Read(ref, scope, ReadOpts{Window: moreWindow(1)})
	if err != nil {
		t.Fatalf("Read --more: %v", err)
	}

	if len(more.Window) <= len(base.Window) {
		t.Errorf("--more should widen: base=%d more=%d", len(base.Window), len(more.Window))
	}
	// Same stable anchor across rungs — expand-in-place, not a new query.
	if more.AnchorID != base.AnchorID {
		t.Errorf("anchor id drifted across --more: %d vs %d", base.AnchorID, more.AnchorID)
	}
	if more.SessionID != base.SessionID {
		t.Errorf("session drifted across --more: %q vs %q", base.SessionID, more.SessionID)
	}
}

func TestReadAroundRadius(t *testing.T) {
	proj := t.TempDir()
	anchorUUID8 := writeNSession(t, proj, "a1b2c3d4eeee", 40, 20)
	scope := scopeFor(t, proj)
	ref := "a1b2c3d4:" + anchorUUID8

	base, err := Read(ref, scope, ReadOpts{})
	if err != nil {
		t.Fatalf("Read base: %v", err)
	}
	// --around shifts the window center forward; the anchor flag should now land
	// on a later message id than the base anchor, but the ref's AnchorID (the
	// resolved ref identity) stays put.
	shifted, err := Read(ref, scope, ReadOpts{Around: 10})
	if err != nil {
		t.Fatalf("Read --around: %v", err)
	}
	if shifted.AnchorID != base.AnchorID {
		t.Errorf("--around must not change the ref's AnchorID: %d vs %d", shifted.AnchorID, base.AnchorID)
	}
	// The re-centered window contains a different id range than the base.
	baseMax := base.Window[len(base.Window)-1].ID
	shiftedMax := shifted.Window[len(shifted.Window)-1].ID
	if shiftedMax <= baseMax {
		t.Errorf("--around 10 should reach later messages: base max %d, shifted max %d", baseMax, shiftedMax)
	}
}

// TestReadMoreIssuesNoSearch proves expand-in-place is a follow-up on the
// resolved ref, NOT a re-search: the message content matches no query term, yet
// --more still returns the widened window. A re-search path would return
// nothing for content that no FTS query was ever issued against.
func TestReadMoreIssuesNoSearch(t *testing.T) {
	proj := t.TempDir()
	anchorUUID8 := writeNSession(t, proj, "a1b2c3d4eeee", 40, 20)
	scope := scopeFor(t, proj)
	ref := "a1b2c3d4:" + anchorUUID8

	// No query string is passed to Read at all — it resolves purely by ref.
	res, err := Read(ref, scope, ReadOpts{Window: moreWindow(2)})
	if err != nil {
		t.Fatalf("Read --more=2: %v", err)
	}
	if len(res.Window) == 0 {
		t.Fatal("expand-in-place returned an empty window — did it re-search?")
	}
}

func TestTrimNoteEmitsNextCommand(t *testing.T) {
	proj := t.TempDir()
	a := strings.Repeat("a", 100)
	b := strings.Repeat("b", 100)
	c := strings.Repeat("c", 100)
	writeMultiSession(t, proj, "a1b2c3d4eeee", [][2]string{
		{"aaaa11110000", a}, {"bbbb22220000", b}, {"cccc33330000", c},
	})
	scope := scopeFor(t, proj)

	cap := 50
	res, err := Read("a1b2c3d4:aaaa1111", scope, ReadOpts{Budget: &cap})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !res.Truncated {
		t.Fatal("expected truncation with budget 50 over a multi-message window")
	}
	// The machine flag carries the literal next command on the SAME ref + --more.
	wantCmd := "rawclaw agent read a1b2c3d4:aaaa1111 --more"
	if res.NextCommand != wantCmd {
		t.Errorf("NextCommand = %q, want %q", res.NextCommand, wantCmd)
	}
	if res.TrimmedChars <= 0 {
		t.Errorf("TrimmedChars should be > 0 when truncated, got %d", res.TrimmedChars)
	}

	// JSON output carries both truncated:true and next_command verbatim.
	var buf bytes.Buffer
	if err := emit(&buf, res); err != nil {
		t.Fatalf("emit: %v", err)
	}
	js := buf.String()
	if !strings.Contains(js, `"truncated": true`) {
		t.Errorf("JSON missing truncated:true\n%s", js)
	}
	if !strings.Contains(js, `"next_command": "`+wantCmd+`"`) {
		t.Errorf("JSON missing next_command\n%s", js)
	}
}

func TestScopeReportAllSearched(t *testing.T) {
	proj := t.TempDir()
	writeSession(t, proj, "a1b2c3d4eeee", "f0000000aaaa", "searchable deploy content")
	scope := scopeFor(t, proj)

	env := Search("deploy", scope, SearchOpts{}, nil)
	if !env.Complete {
		t.Errorf("clean run must report Complete=true; scopes=%+v", env.Scopes)
	}
	if len(env.Scopes) != 1 {
		t.Fatalf("expected 1 scope report, got %d", len(env.Scopes))
	}
	if env.Scopes[0].Status != ScopeSearched {
		t.Errorf("scope status = %q, want %q", env.Scopes[0].Status, ScopeSearched)
	}
}

// TestScopeReportSkipsLocked: when a project's index db is busy/locked (here
// simulated by a pre-existing corrupt db file at the cache path that openRW
// can't use), EnsureIndexed falls back to the stale cached index, and Search
// reports that scope as stale_fallback with Complete=false — the agent reads an
// incomplete result AS incomplete rather than as "no matches".
func TestScopeReportSkipsLocked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	proj := t.TempDir()
	writeSession(t, proj, "a1b2c3d4eeee", "f0000000aaaa", "searchable content")

	// Plant a corrupt db at the exact cache path EnsureIndexed will use, so the
	// read-write open Ping fails → stale fallback (the locked-scope signal).
	dbp := index.DBPath(proj)
	if err := os.MkdirAll(filepath.Dir(dbp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbp, []byte("not a sqlite database at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	env := Search("searchable", scope, SearchOpts{}, nil)

	if env.Complete {
		t.Errorf("a stale/locked scope must report Complete=false; scopes=%+v", env.Scopes)
	}
	if len(env.Scopes) != 1 {
		t.Fatalf("expected 1 scope report, got %d", len(env.Scopes))
	}
	st := env.Scopes[0].Status
	if st != ScopeStaleFallback && st != ScopeSkippedError {
		t.Errorf("locked-scope status = %q, want stale_fallback or skipped_error", st)
	}
}

func TestRunThisProjectNoHistory(t *testing.T) {
	// In a temp cwd with no transcript history, --this-project should print the
	// hint and exit 1 before touching any verb.
	dir := t.TempDir()
	cwd := chdir(t, dir)
	defer cwd()

	var out, errw bytes.Buffer
	code := Run([]string{"--this-project", "search", "q"}, &out, &errw)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errw.String(), "No transcript history for this directory") {
		t.Fatalf("stderr = %q, want no-history hint", errw.String())
	}
}
