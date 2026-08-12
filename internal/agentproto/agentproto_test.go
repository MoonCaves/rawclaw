package agentproto

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"

	_ "modernc.org/sqlite"
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
		// Search output prints refs as `read ref=<session8>:<uuid8>` — agents
		// paste that token verbatim, so the parser must accept it.
		{name: "ref= prefix as printed", ref: "ref=a1b2c3d4:9f3e1c20", wantSID: "a1b2c3d4", wantUUID: "9f3e1c20"},
		{name: "bare ref= still bad", ref: "ref=", wantErr: "expected <session8>"},
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

// TestSearchPrintedRefRoundTrips: the EXACT token search output prints after
// "read " must be accepted by the read verb's ref parser — an agent copies the
// printed `ref=<session8>:<uuid8>` verbatim.
func TestSearchPrintedRefRoundTrips(t *testing.T) {
	env := SearchEnvelope{
		Results: []SearchRef{{
			Project:   "p",
			SessionID: "a1b2c3d4eeee",
			ISO:       "2026-06-01T10:00:00Z",
			Snippet:   "snippet",
			ReadRef:   "a1b2c3d4:9f3e1c20",
		}},
		Complete: true,
		Count:    1,
	}
	var buf bytes.Buffer
	renderSearch(&buf, env, "q", "")

	token := ""
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "read "); ok {
			token = rest
			break
		}
	}
	if token == "" {
		t.Fatalf("no `read <token>` line in search output:\n%s", buf.String())
	}
	sid, uuid, err := resolveRef(token)
	if err != nil {
		t.Fatalf("resolveRef(%q) — the token search itself printed — err: %v", token, err)
	}
	if sid != "a1b2c3d4" || uuid != "9f3e1c20" {
		t.Fatalf("resolveRef(%q) = (%q, %q), want (a1b2c3d4, 9f3e1c20)", token, sid, uuid)
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
	t.Run("a topic rides the header, never its own line", func(t *testing.T) {
		// This subtest used to assert that topics never appear in search output at
		// all. That rule was superseded: the label now rides the header as context
		// on the hit (see TestSearchCarriesTopicLabel). What survives of it is the
		// narrower rule — the label costs no VERTICAL space, because a fourth line
		// per hit is the whole reason it was kept out of search in the first place.
		//
		// Asserting the header placement too is what stops this passing vacuously:
		// the old form only checked that an "in: " line was absent, which would
		// stay true even if the label were dropped entirely.
		var buf bytes.Buffer
		renderSearch(&buf, SearchEnvelope{Complete: true, Results: []SearchRef{
			{Project: "proj", SessionID: "aaaa", ISO: "2026", Snippet: "s", ReadRef: "aaaa:9f", Topic: "deployment rollback"},
		}}, "kw", "x")
		out := buf.String()
		if strings.Contains(out, "     in: ") {
			t.Errorf("the topic took a line of its own: %q", out)
		}
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "deployment rollback") && !strings.Contains(line, "━━") {
				t.Errorf("the topic rendered off the header line: %q", line)
			}
		}
		if !strings.Contains(out, "deployment rollback") {
			t.Errorf("the topic was dropped from the output entirely: %q", out)
		}
	})
	t.Run("incomplete scope footer", func(t *testing.T) {
		// The footer is now DERIVED once, into the envelope's warnings, and the
		// renderer only prints what it is given — so this drives the real path
		// (buildWarnings → renderSearch) rather than a hand-written envelope that
		// could disagree with what a live search would produce.
		reports := []ScopeReport{
			{Project: "p", Status: ScopeSearched},
			{Project: "q", Status: ScopeSkippedError, Detail: "boom"},
			{Project: "r", Status: ScopeStaleFallback},
		}
		results := []SearchRef{{Project: "p", SessionID: "aaaa", ISO: "2026", Snippet: "s", ReadRef: "aaaa:9f"}}
		warns := buildWarnings(warningInputs{results: results, reports: reports})

		var buf bytes.Buffer
		renderSearch(&buf, SearchEnvelope{
			Complete: false,
			Results:  results,
			Scopes:   reports,
			Warnings: warns,
		}, "kw", "across all projects")
		if out := buf.String(); !strings.Contains(out, "note: 2 of 3 projects incomplete (1 error, 1 stale)") {
			t.Errorf("missing incompleteness footer: %q", out)
		}
		// ...and the same fact is available as data, not only as that sentence.
		w := findWarning(warns, WarnScopeIncomplete)
		if w == nil {
			t.Fatalf("no %s warning built from %d reports", WarnScopeIncomplete, len(reports))
		}
		for key, want := range map[string]any{"scopes": 3, "incomplete": 2, "errored": 1, "stale": 1} {
			if got := w.Facts[key]; got != want {
				t.Errorf("facts[%q] = %v, want %v", key, got, want)
			}
		}
	})
}

// findWarning returns the warning carrying code, or nil. Tests assert on the
// code they care about rather than on slice position, so adding a warning
// elsewhere in the order does not break unrelated tests.
func findWarning(ws []Warning, code string) *Warning {
	for i := range ws {
		if ws[i].Code == code {
			return &ws[i]
		}
	}
	return nil
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
		NextCommand:  "rawclaw read a1b2c3d4:9f3e1c20 --more",
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
		"  [+1.8k chars · 2 msgs hidden — rawclaw read a1b2c3d4:9f3e1c20 --more]\n",
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

// TestNormalizeSessionArg: the session-taking verbs (outline, tag) accept a
// pasted read-ref token — leading "ref=" dropped, and a full
// <session8>:<uuid8> paste keeps only the session half.
func TestNormalizeSessionArg(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a1b2c3d4", "a1b2c3d4"},              // plain sid8 untouched
		{"ref=a1b2c3d4", "a1b2c3d4"},          // pasted with the printed prefix
		{"ref=a1b2c3d4:9f3e1c20", "a1b2c3d4"}, // full printed read-ref token
		{"a1b2c3d4:9f3e1c20", "a1b2c3d4"},     // read-ref without the prefix
		{"ref=", "ref="},                      // degenerate paste stays verbatim for an honest not-found
	}
	for _, tt := range tests {
		if got := normalizeSessionArg(tt.in); got != tt.want {
			t.Errorf("normalizeSessionArg(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestLocateSessionAcceptsPastedRef: the exported LocateSession (tag verb) and
// Outline path resolve a session from the full pasted `ref=...` token.
func TestLocateSessionAcceptsPastedRef(t *testing.T) {
	proj := t.TempDir()
	writeSession(t, proj, "a1b2c3d4eeee", "uuid-a-1", "first session")
	scope := scopeFor(t, proj)

	_, fullSID, err := LocateSession("ref=a1b2c3d4:9f3e1c20", scope)
	if err != nil {
		t.Fatalf("LocateSession with pasted ref token: %v", err)
	}
	if fullSID != "a1b2c3d4eeee" {
		t.Fatalf("fullSID = %q, want a1b2c3d4eeee", fullSID)
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

// openCacheRW opens the cache db EnsureIndexed built, read-write, so a test can
// insert topic rows after the keyword index exists.
func openCacheRW(t *testing.T, dbp string) *sql.DB {
	t.Helper()
	con, err := sql.Open("sqlite", "file:"+dbp+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open cache rw: %v", err)
	}
	con.SetMaxOpenConns(1)
	t.Cleanup(func() { con.Close() })
	return con
}

// TestSearchCarriesTopicLabel supersedes the former TestSearchNeverCarriesTopic,
// which asserted the opposite: that a search hit must carry an EMPTY topic.
//
// That earlier rule came from a concern about RANKING — that a label sharing a
// word with the query could mis-route which conversation won. The label is now
// attached after ranking has finished (see attachTopics), so it cannot do that,
// and the label is worth showing: it is the one field that says what a session
// was about without opening it. The ranking property the old rule was really
// protecting is held directly by TestTopicLabelDoesNotAffectOrdering below.
func TestSearchCarriesTopicLabel(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeSession(t, proj, "sessone", "11111111-aaaa-bbbb-cccc-000000000001", "discussing the deployment rollback procedure")

	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	con := openCacheRW(t, dbp)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, "sessone",
		"11111111-aaaa-bbbb-cccc-000000000001", "", "deployment rollback", "how we rolled back", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
	con.Close()

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	env := Search("deployment", scope, SearchOpts{}, nil)
	if len(env.Results) == 0 {
		t.Fatalf("expected a match for 'deployment', got none")
	}
	if env.Results[0].Topic != "deployment rollback" {
		t.Fatalf("result Topic = %q, want %q", env.Results[0].Topic, "deployment rollback")
	}
}

// TestSearchUntaggedSessionHasEmptyTopic keeps the untagged corpus honest: with
// no topic rows at all, every hit's label is empty and the JSON stays identical
// to a pre-topic corpus (the field is omitempty).
func TestSearchUntaggedSessionHasEmptyTopic(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeSession(t, proj, "sessone", "11111111-aaaa-bbbb-cccc-000000000001", "discussing the deployment rollback procedure")

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	env := Search("deployment", scope, SearchOpts{}, nil)
	if len(env.Results) == 0 {
		t.Fatalf("expected a match for 'deployment', got none")
	}
	if env.Results[0].Topic != "" {
		t.Fatalf("untagged result Topic = %q, want empty", env.Results[0].Topic)
	}
}

// TestTopicLabelDoesNotAffectOrdering is the property the old never-carry rule
// was actually defending, asserted directly.
//
// The corpus is built so that a topic label is the WORST possible influence: the
// query word appears in the label of a session whose message text matches the
// query only weakly, while a different session matches strongly in its text. If
// the label were reaching the ranking, tagging would reorder the results. The
// test records the order with no topic rows present, inserts the adversarial
// labels, searches again, and requires the identical sequence of read-refs.
func TestTopicLabelDoesNotAffectOrdering(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	// Strong textual match for "deployment": the word appears repeatedly.
	writeSession(t, proj, "sessstrong", "22222222-aaaa-bbbb-cccc-000000000001",
		"deployment deployment deployment notes about the deployment")
	// Weak textual match: the word appears once, amid unrelated text.
	writeSession(t, proj, "sessweak", "33333333-aaaa-bbbb-cccc-000000000001",
		"mostly unrelated chatter that mentions deployment once in passing")

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}

	before := Search("deployment", scope, SearchOpts{}, nil)
	if len(before.Results) < 2 {
		t.Fatalf("expected both sessions to match, got %d", len(before.Results))
	}
	beforeOrder := make([]string, len(before.Results))
	for i, r := range before.Results {
		beforeOrder[i] = r.ReadRef
		if r.Topic != "" {
			t.Fatalf("pre-tagging result %d already carries a topic %q", i, r.Topic)
		}
	}

	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	con := openCacheRW(t, dbp)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	// Load the query word into the WEAK session's label, and keep it out of the
	// strong one's — the arrangement most likely to flip the order if labels leak.
	if err := store.UpsertTopicSegment(con, "sessweak",
		"33333333-aaaa-bbbb-cccc-000000000001", "", "deployment deployment deployment", "", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment weak: %v", err)
	}
	if err := store.UpsertTopicSegment(con, "sessstrong",
		"22222222-aaaa-bbbb-cccc-000000000001", "", "unrelated label", "", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment strong: %v", err)
	}
	con.Close()

	after := Search("deployment", scope, SearchOpts{}, nil)
	afterOrder := make([]string, len(after.Results))
	for i, r := range after.Results {
		afterOrder[i] = r.ReadRef
	}
	if len(afterOrder) != len(beforeOrder) {
		t.Fatalf("result count changed after tagging: %d → %d", len(beforeOrder), len(afterOrder))
	}
	for i := range beforeOrder {
		if beforeOrder[i] != afterOrder[i] {
			t.Fatalf("tagging reordered results at %d: %q → %q (full: %v → %v)",
				i, beforeOrder[i], afterOrder[i], beforeOrder, afterOrder)
		}
	}
	// And the labels really did land — otherwise this test would pass vacuously.
	labelled := false
	for _, r := range after.Results {
		if r.Topic != "" {
			labelled = true
		}
	}
	if !labelled {
		t.Fatal("no result carried a topic label after tagging — test would be vacuous")
	}
}

// TestTopicsCommand drives the on-demand topic finder: a tagged session, queried
// by a topic word, returns the matching topic with a well-formed read-ref
// (<sess8>:<uuid8>) pointing at the segment's start message.
func TestTopicsCommand(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	uuid := "9f3e1c20-aaaa-bbbb-cccc-000000000001" // hex prefix with letters → resolveRef-valid
	writeSession(t, proj, "sessone", uuid, "discussing the deployment rollback procedure")

	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	con := openCacheRW(t, dbp)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, "sessone",
		uuid, "", "deployment rollback", "how we rolled back", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
	con.Close()

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	res, err := Topics("rollback", scope, TopicsOpts{Limit: 8})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("Topics hits = %d, want 1: %+v", len(res.Hits), res.Hits)
	}
	h := res.Hits[0]
	if h.Topic != "deployment rollback" {
		t.Errorf("hit Topic = %q, want deployment rollback", h.Topic)
	}
	// The read-ref must be a well-formed <sess8>:<uuid8> that resolveRef accepts and
	// whose uuid8 half matches the tagged segment's start message.
	wantRef := fmtRef("sessone", uuid)
	if h.ReadRef != wantRef {
		t.Errorf("hit ReadRef = %q, want %q", h.ReadRef, wantRef)
	}
	if _, _, err := resolveRef(h.ReadRef); err != nil {
		t.Errorf("ReadRef %q is not resolvable: %v", h.ReadRef, err)
	}
	if res.Note != "" {
		t.Errorf("note should be empty when topics exist, got %q", res.Note)
	}

	// A query that matches nothing (but topics ARE tagged) → no hits, no empty-note.
	none, err := Topics("kubernetes", scope, TopicsOpts{Limit: 8})
	if err != nil {
		t.Fatalf("Topics(kubernetes): %v", err)
	}
	if len(none.Hits) != 0 || none.Note != "" {
		t.Errorf("Topics(kubernetes) = hits %d note %q, want 0 hits and no note", len(none.Hits), none.Note)
	}

	// Render smoke: the matching hit line carries topic, project, and ref.
	var buf bytes.Buffer
	renderTopics(&buf, res)
	out := buf.String()
	if !strings.Contains(out, "deployment rollback") || !strings.Contains(out, "read ref="+wantRef) {
		t.Errorf("renderTopics missing topic/ref line:\n%s", out)
	}
}

// tagInto writes one topic segment into a project's index db and folds it into
// the consolidated store, mirroring what `rawclaw tag-write` does end to end.
// It returns the project label the store stamped on the session.
func tagInto(t *testing.T, proj, sid, uuid, topic, summary string) {
	t.Helper()
	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(%s): %v", proj, err)
	}
	con := openCacheRW(t, dbp)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, sid, uuid, "", topic, summary, 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
	con.Close()
	if err := index.SyncConsolidatedFrom(dbp); err != nil {
		t.Fatalf("SyncConsolidatedFrom(%s): %v", proj, err)
	}
}

// TestTopicsRanksAcrossProjectsFromTheStore is the end-to-end shape of the
// change: two projects, both folded into one store, one query, and a single
// ranked list in which a topic LABEL beats a passing mention regardless of which
// project is larger. The old fan-out could only return the two projects'
// separate lists back to back.
func TestTopicsRanksAcrossProjectsFromTheStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	big := t.TempDir()
	writeSession(t, big, "sessbig", "9f3e1c20-aaaa-bbbb-cccc-000000000001", "advancing the retention watermark")
	small := t.TempDir()
	writeSession(t, small, "sesssml", "9f3e1c20-aaaa-bbbb-cccc-000000000002", "the weekly planning recap")

	tagInto(t, big, "sessbig", "9f3e1c20-aaaa-bbbb-cccc-000000000001",
		"watermark", "how the watermark is advanced")
	tagInto(t, small, "sesssml", "9f3e1c20-aaaa-bbbb-cccc-000000000002",
		"weekly planning",
		"a long recap covering staffing, the release calendar, pricing questions, "+
			"a tangent about the watermark, the review backlog, and what we deferred")

	// Scope is deliberately EMPTY: if the store did not answer, the fan-out would
	// have no project to look in and the result would be empty, so a passing
	// assertion here can only have come from the one store.
	res, err := Topics("watermark", []view.Scope{}, TopicsOpts{Limit: 8})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("Topics hits = %d, want both projects' hits in one list: %+v", len(res.Hits), res.Hits)
	}
	if res.Hits[0].Topic != "watermark" {
		t.Errorf("top hit = %q from %q, want the label hit", res.Hits[0].Topic, res.Hits[0].Project)
	}
	if res.Hits[0].Project == "" || res.Hits[0].Project == res.Hits[1].Project {
		t.Errorf("hits should carry distinct project labels, got %q and %q",
			res.Hits[0].Project, res.Hits[1].Project)
	}
	if res.Note != "" {
		t.Errorf("note should be empty when topics exist, got %q", res.Note)
	}

	// Narrowing to the small project drops the other project's hit entirely.
	only, err := Topics("watermark", []view.Scope{}, TopicsOpts{Limit: 8, Project: res.Hits[1].Project})
	if err != nil {
		t.Fatalf("Topics(narrowed): %v", err)
	}
	if len(only.Hits) != 1 || only.Hits[0].Project != res.Hits[1].Project {
		t.Errorf("narrowed Topics = %+v, want only the %q hit", only.Hits, res.Hits[1].Project)
	}

	// A global --limit caps the combined list, not each project's share of it.
	capped, err := Topics("watermark", []view.Scope{}, TopicsOpts{Limit: 1})
	if err != nil {
		t.Fatalf("Topics(limit=1): %v", err)
	}
	if len(capped.Hits) != 1 {
		t.Errorf("Topics(limit=1) = %d hits, want 1 across all projects", len(capped.Hits))
	}
}

// TestTopicsFallsBackWhenTheStoreHasNotHeardOfTheProject covers the honest
// degradation. The store carries topics, but not this project's — it was
// rebuilt from a narrower set of sources — so a request narrowed to that
// project must go to the project db rather than come back empty from a store
// that never had the answer.
func TestTopicsFallsBackWhenTheStoreHasNotHeardOfTheProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	kept := t.TempDir()
	writeSession(t, kept, "sesskpt", "9f3e1c20-aaaa-bbbb-cccc-000000000001", "advancing the retention watermark")
	tagInto(t, kept, "sesskpt", "9f3e1c20-aaaa-bbbb-cccc-000000000001",
		"watermark", "how the watermark is advanced")
	keptDB, _, _, err := index.EnsureIndexed(kept, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(kept): %v", err)
	}

	dropped := t.TempDir()
	writeSession(t, dropped, "sessdrp", "9f3e1c20-aaaa-bbbb-cccc-000000000002", "the shard rebalance run")
	tagInto(t, dropped, "sessdrp", "9f3e1c20-aaaa-bbbb-cccc-000000000002",
		"shard rebalance", "how the shards were rebalanced")

	// Rebuild the store from the kept project ALONE, so the other project is
	// genuinely absent from it while its own db still carries the topic.
	if _, err := index.ConsolidateFrom([]string{keptDB}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	label := paths.ProjectLabel(dropped)
	scope := []view.Scope{{Project: label, TDir: dropped}}
	res, err := Topics("rebalance", scope, TopicsOpts{Limit: 8, Project: label})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(res.Hits) != 1 || res.Hits[0].Topic != "shard rebalance" {
		t.Fatalf("Topics = %+v, want the dropped project's hit via the fan-out", res.Hits)
	}
}

// TestTopicsEmptyState confirms the helpful empty-state note when NO topics are
// tagged anywhere in scope (distinct from a query that simply matched nothing).
func TestTopicsEmptyState(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeSession(t, proj, "sessone", "11111111-aaaa-bbbb-cccc-000000000001", "a session with no topics tagged")
	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	res, err := Topics("anything", scope, TopicsOpts{Limit: 8})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("expected 0 hits, got %d", len(res.Hits))
	}
	if res.Note != topicsEmptyNote {
		t.Errorf("note = %q, want %q", res.Note, topicsEmptyNote)
	}
	var buf bytes.Buffer
	renderTopics(&buf, res)
	if !strings.Contains(buf.String(), "no topics tagged yet") {
		t.Errorf("renderTopics missing empty-state note:\n%s", buf.String())
	}
}

// TestOutlineListsTopics confirms Outline surfaces the session's topic segments
// as a "topics:" line.
func TestOutlineListsTopics(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeSession(t, proj, "sesstwo", "22222222-aaaa-bbbb-cccc-000000000002", "an opening message for the outline")

	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	// Author the tag in the db the READ path resolves to — the same one tag-write
	// opens. Writing it anywhere else would be testing a tag no reader can see.
	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	dbp, _, _, err := locateSession(scope, "sesstwo")
	if err != nil {
		t.Fatalf("locateSession: %v", err)
	}
	con := openCacheRW(t, dbp)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, "sesstwo",
		"22222222-aaaa-bbbb-cccc-000000000002", "", "first topic", "", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
	con.Close()

	res, err := Outline("sesstwo", scope, false)
	if err != nil {
		t.Fatalf("Outline: %v", err)
	}
	if len(res.Topics) != 1 || res.Topics[0] != "first topic" {
		t.Fatalf("Outline Topics = %v, want [first topic]", res.Topics)
	}
	var buf bytes.Buffer
	renderOutline(&buf, res)
	if !strings.Contains(buf.String(), "  topics: first topic\n") {
		t.Errorf("renderOutline missing topics line:\n%s", buf.String())
	}
}

// TestSearchFlagsRetainedMissing is the D7 surface check: once a session's
// backing file is purged, it is retained (durable retention) but the search
// envelope marks the ref Missing so an agent doesn't read it as current.
func TestSearchFlagsRetainedMissing(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj) // isolates HOME → db + machine-id land in a temp dir
	writeSession(t, proj, "gone", "uuid-miss-0001", "retainedmissingbeacon token")

	// First search indexes it while the file is present → not missing.
	env := Search("retainedmissingbeacon", scope, SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("pass 1 results = %d, want 1", len(env.Results))
	}
	if env.Results[0].Missing {
		t.Fatalf("present session wrongly flagged Missing")
	}

	// Purge the transcript, search again: still returned, now flagged Missing.
	if err := os.Remove(filepath.Join(proj, "gone.jsonl")); err != nil {
		t.Fatal(err)
	}
	env = Search("retainedmissingbeacon", scope, SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("pass 2 results = %d, want 1 (retained after purge)", len(env.Results))
	}
	if !env.Results[0].Missing {
		t.Errorf("retained-but-missing result not flagged Missing in the envelope")
	}
	// The text renderer surfaces it too.
	var buf bytes.Buffer
	renderSearch(&buf, env, "retainedmissingbeacon", "across all projects")
	if !strings.Contains(buf.String(), "source file gone") {
		t.Errorf("renderSearch missing the retained-history marker:\n%s", buf.String())
	}
}

// TestSearchDiscoversOrphanedProject is the D8 end-to-end guard, driven through
// the REAL scope discovery (scopes.All), not an injected scope: after a project's
// transcripts are purged (dir emptied → AllProjectDirs drops it), search and read
// must STILL reach the retained session via the orphaned index db. The earlier
// injected-scope tests passed while this exact path was broken end-to-end, so
// this one goes through discovery on a real projects root.
func TestSearchDiscoversOrphanedProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))

	projDir := filepath.Join(home, ".claude", "projects", "-tmp-orphanproj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessFile := filepath.Join(projDir, "77777777-2222-3333-4444-555555555555.jsonl")
	line := `{"type":"user","uuid":"aaaa1111-bbbb-cccc-dddd-eeeeeeeeeeee","timestamp":"2026-06-01T10:00:00Z","cwd":"/tmp/orphanproj","message":{"role":"user","content":"orphandiscoverybeacon token"}}`
	if err := os.WriteFile(sessFile, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Discover from the real projects root; the first search builds the index db.
	env := Search("orphandiscoverybeacon", scopes.All(t.Context(), "", false), SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("pre-purge results = %d, want 1", len(env.Results))
	}
	ref := env.Results[0].ReadRef

	// Purge: remove the transcript → the project dir is empty, so AllProjectDirs
	// no longer yields it. Discovery must now come from the orphaned index db.
	if err := os.Remove(sessFile); err != nil {
		t.Fatal(err)
	}

	env = Search("orphandiscoverybeacon", scopes.All(t.Context(), "", false), SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("post-purge results = %d, want 1 — orphaned-source db not discovered/searchable", len(env.Results))
	}
	if !env.Results[0].Missing {
		t.Errorf("discovered orphan hit not flagged Missing")
	}

	// read resolves the retained content through the discovered scope.
	rr, err := Read(ref, scopes.All(t.Context(), "", false), ReadOpts{})
	if err != nil {
		t.Fatalf("read via discovered orphan scope: %v", err)
	}
	if rr == nil || rr.AnchoredView == nil {
		t.Fatal("read returned no content for the retained orphan session")
	}
}

// TestTopicsCollapsesRepeatedLabel covers the duplicate-row defect: a tagger
// often cuts one long conversation into several segments carrying the SAME
// label, and every one of them was a separate row. Observed live, one session
// filled four of the result slots with four identical labels, differing only in
// which message each read-ref pointed at.
//
// The session below has three segments labelled "shared label" and one labelled
// "distinct label". The repeats must collapse to a single row while the distinct
// label survives as its own row — collapsing must not hide real coverage.
func TestTopicsCollapsesRepeatedLabel(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	// One session, four messages, so segments can start at distinct anchors.
	uuids := []string{
		"aaaaaaaa-1111-2222-3333-000000000001",
		"aaaaaaaa-1111-2222-3333-000000000002",
		"aaaaaaaa-1111-2222-3333-000000000003",
		"aaaaaaaa-1111-2222-3333-000000000004",
	}
	var lines []string
	for i, u := range uuids {
		lines = append(lines, `{"type":"user","uuid":"`+u+`","timestamp":"2026-06-01T10:0`+
			string(rune('0'+i))+`:00Z","message":{"role":"user","content":"message `+string(rune('a'+i))+`"}}`)
	}
	if err := os.WriteFile(filepath.Join(proj, "sessdup.jsonl"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	con := openCacheRW(t, dbp)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	labels := []string{"shared label", "shared label", "shared label", "distinct label"}
	for i, lbl := range labels {
		if err := store.UpsertTopicSegment(con, "sessdup", uuids[i], "", lbl, "", 1.0); err != nil {
			t.Fatalf("UpsertTopicSegment %d: %v", i, err)
		}
	}
	con.Close()

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	res, err := Topics("label", scope, TopicsOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	counts := map[string]int{}
	for _, h := range res.Hits {
		counts[h.Topic]++
	}
	if counts["shared label"] != 1 {
		t.Errorf("repeated label appeared %d times, want 1 (hits: %+v)", counts["shared label"], res.Hits)
	}
	if counts["distinct label"] != 1 {
		t.Errorf("distinct label appeared %d times, want 1 (hits: %+v)", counts["distinct label"], res.Hits)
	}
}
