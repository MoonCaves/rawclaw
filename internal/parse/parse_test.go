package parse

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// decode is a test helper: parse a JSON object literal into the map[string]any
// shape ExtractText / MsgRole consume, matching how real transcript records arrive.
func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("decode(%q): %v", s, err)
	}
	return m
}

func TestExtractText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "summary record",
			json: `{"type":"summary","summary":"hi there"}`,
			want: "[SUMMARY] hi there",
		},
		{
			name: "summary with non-string falls through to empty",
			json: `{"type":"summary","summary":123}`,
			want: "",
		},
		{
			name: "system record",
			json: `{"type":"system","content":"syslog"}`,
			want: "[SYSTEM] syslog",
		},
		{
			name: "message string content",
			json: `{"type":"user","message":{"content":"plain"}}`,
			want: "plain",
		},
		{
			name: "no message, top-level string content tagged by type",
			json: `{"type":"foo","content":"raw"}`,
			want: "[FOO] raw",
		},
		{
			name: "no message, missing type defaults to MISC",
			json: `{"content":"raw"}`,
			want: "[MISC] raw",
		},
		{
			name: "no message, no content yields empty",
			json: `{"type":"foo"}`,
			want: "",
		},
		{
			name: "all block types flattened and tagged",
			json: `{"type":"assistant","message":{"content":[` +
				`{"type":"text","text":"T"},` +
				`{"type":"thinking","thinking":"TH"},` +
				`{"type":"tool_use","name":"Bash","input":{"a":1}},` +
				`{"type":"tool_result","content":"res"},` +
				`{"type":"tool_result","content":[{"text":"rx"},{"nope":1}]}` +
				`]}}`,
			want: `T [THINKING] TH [TOOL:Bash] {"a": 1} [TOOL_RESULT] res [TOOL_RESULT] rx`,
		},
		{
			name: "tool_use without name uses ? and empty input",
			json: `{"message":{"content":[{"type":"tool_use","input":{}}]}}`,
			want: "[TOOL:?] {}",
		},
		{
			name: "unknown block type yields empty",
			json: `{"message":{"content":[{"type":"other"}]}}`,
			want: "",
		},
		{
			name: "message content non-list non-string yields empty",
			json: `{"message":{"content":42}}`,
			want: "",
		},
		{
			name: "tool_use with missing input defaults to empty object",
			json: `{"message":{"content":[{"type":"tool_use","name":"X"}]}}`,
			want: "[TOOL:X] {}",
		},
		{
			name: "non-dict block in content list is skipped",
			json: `{"message":{"content":["bare",{"type":"text","text":"keep"}]}}`,
			want: "keep",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractText(decode(t, tt.json))
			if got != tt.want {
				t.Errorf("ExtractText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractText_BlockCapTruncation(t *testing.T) {
	t.Parallel()
	long := make([]byte, BlockCap+100)
	for i := range long {
		long[i] = 'x'
	}
	obj := map[string]any{
		"type":    "user",
		"message": map[string]any{"content": string(long)},
	}
	got := ExtractText(obj)
	if len(got) != BlockCap {
		t.Errorf("ExtractText() truncated len = %d, want %d", len(got), BlockCap)
	}
}

func TestStripTools(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "tool run removed, trailing text kept",
			in:   `hello [TOOL:Bash] {"cmd":"ls"} world`,
			want: "hello",
		},
		{
			name: "tool_result removed, thinking marker preserved",
			in:   "pre [TOOL_RESULT] some output [THINKING] keep this",
			want: "pre [THINKING] keep this",
		},
		{
			name: "adjacent tool runs consume through end",
			in:   "a [TOOL:X] foo [TOOL_RESULT] bar end",
			want: "a",
		},
		{
			name: "only a tool run yields empty",
			in:   "[TOOL:X] only tool",
			want: "",
		},
		{
			name: "no tool markers passes through",
			in:   "no tools here",
			want: "no tools here",
		},
		{
			name: "system marker is a boundary and preserved",
			in:   "text [TOOL:Read] {} [SYSTEM] sys",
			want: "text [SYSTEM] sys",
		},
		{
			name: "no whitespace before next bracket: not a boundary",
			in:   "x [TOOL:X]foo[SYSTEM]bar",
			want: "x",
		},
		{
			name: "multiple spaces before SYSTEM boundary",
			in:   "x [TOOL:X] foo  [SYSTEM] bar",
			want: "x [SYSTEM] bar",
		},
		{
			name: "leading tool_result then tool",
			in:   "[TOOL_RESULT]a [TOOL:b] c",
			want: "",
		},
		{
			name: "whitespace collapse with no tools",
			in:   "lead   trail   spaces",
			want: "lead trail spaces",
		},
		{
			name: "lowercase tag is not a tool marker",
			in:   "[tool:lower] not matched",
			want: "[tool:lower] not matched",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripTools(tt.in)
			if got != tt.want {
				t.Errorf("StripTools(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMsgRole(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "message role wins over type",
			json: `{"message":{"role":"assistant"},"type":"x"}`,
			want: "assistant",
		},
		{
			name: "no message falls to type",
			json: `{"type":"summary"}`,
			want: "summary",
		},
		{
			name: "empty message dict, no type",
			json: `{"message":{}}`,
			want: "",
		},
		{
			name: "null message falls to type",
			json: `{"message":null,"type":"user"}`,
			want: "user",
		},
		{
			name: "empty object yields empty",
			json: `{}`,
			want: "",
		},
		{
			name: "empty role string falls through to type",
			json: `{"message":{"role":""},"type":"user"}`,
			want: "user",
		},
		{
			name: "empty message dict falls through to type",
			json: `{"message":{},"type":"user"}`,
			want: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MsgRole(decode(t, tt.json))
			if got != tt.want {
				t.Errorf("MsgRole() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMsgUUID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "top-level uuid wins",
			json: `{"type":"user","uuid":"9f3e1c20-aaaa","leafUuid":"zzzz"}`,
			want: "9f3e1c20-aaaa",
		},
		{
			name: "summary falls back to leafUuid",
			json: `{"type":"summary","summary":"recap","leafUuid":"abcd-1234"}`,
			want: "abcd-1234",
		},
		{
			name: "empty uuid falls through to leafUuid",
			json: `{"type":"user","uuid":"","leafUuid":"abcd-1234"}`,
			want: "abcd-1234",
		},
		{
			name: "no uuid and no leafUuid yields empty",
			json: `{"type":"summary","summary":"recap"}`,
			want: "",
		},
		{
			name: "empty object yields empty",
			json: `{}`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MsgUUID(decode(t, tt.json)); got != tt.want {
				t.Errorf("MsgUUID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsLowSignal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: true},
		{name: "whitespace only", in: "   \n\t ", want: true},
		{name: "bare greeting hi", in: "hi", want: true},
		{name: "bare greeting uppercase", in: "HEY", want: true},
		{name: "bare greeting with surrounding space", in: "  yo  ", want: true},
		{name: "bare slash clear", in: "/clear", want: true},
		{name: "bare slash compact", in: "/compact", want: true},
		{name: "slash with trailing arg still control verb", in: "/resume abc123", want: true},
		{name: "command-name envelope", in: "<command-name>foo</command-name>", want: true},
		{name: "command-message envelope", in: "<command-message>run</command-message>", want: true},
		{name: "local-command-stdout envelope", in: "<local-command-stdout>x</local-command-stdout>", want: true},
		{name: "json opener", in: `{"a":1}`, want: true},
		{name: "array opener", in: "[1, 2, 3]", want: true},
		{name: "xml opener", in: "<thing/>", want: true},
		{name: "substantive question is not low-signal", in: "how do I parse this transcript?", want: false},
		{name: "greeting inside a real sentence is not low-signal", in: "hi, can you fix the bug in parse.go?", want: false},
		{name: "unknown slash command is not low-signal", in: "/deploy the staging box", want: false},
		{name: "message merely containing brace mid-text is not low-signal", in: "the value should be {x}", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsLowSignal(tt.in); got != tt.want {
				t.Errorf("IsLowSignal(%q) = %v, want %v", tt.in, got, tt.want)
			}
			if got := IsSubstantive(tt.in); got != !tt.want {
				t.Errorf("IsSubstantive(%q) = %v, want %v", tt.in, got, !tt.want)
			}
		})
	}
}

// decodeRecords decodes a slice of JSON object literals into the []map[string]any
// shape DeriveTitle consumes (transcript order preserved).
func decodeRecords(t *testing.T, lines []string) []map[string]any {
	t.Helper()
	out := make([]map[string]any, 0, len(lines))
	for _, s := range lines {
		out = append(out, decode(t, s))
	}
	return out
}

func TestDeriveTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		lines     []string
		wantTitle string
		wantFound bool
	}{
		{
			name: "hi opener yields title from later substantive turn",
			lines: []string{
				`{"type":"user","message":{"role":"user","content":"hi"}}`,
				`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello!"}]}}`,
				`{"type":"user","message":{"role":"user","content":"fix the FTS5 ranking in retrieve.go"}}`,
			},
			wantTitle: "fix the FTS5 ranking in retrieve.go",
			wantFound: true,
		},
		{
			name: "native ai_title preferred over derivation",
			lines: []string{
				`{"type":"user","ai_title":"Refactor the indexer","message":{"role":"user","content":"actually start with the schema"}}`,
			},
			wantTitle: "Refactor the indexer",
			wantFound: true,
		},
		{
			name: "native ai_title on a later record still wins over earlier substantive user text",
			lines: []string{
				`{"type":"user","message":{"role":"user","content":"first substantive question here"}}`,
				`{"type":"summary","summary":"Session: build the parser","leafUuid":"x"}`,
			},
			// summary key is in titleFields; the native field wins over pass-2.
			wantTitle: "Session: build the parser",
			wantFound: true,
		},
		{
			name: "slash-clear opener skipped, next user turn used",
			lines: []string{
				`{"type":"user","message":{"role":"user","content":"/clear"}}`,
				`{"type":"user","message":{"role":"user","content":"now explain the bookend logic"}}`,
			},
			wantTitle: "now explain the bookend logic",
			wantFound: true,
		},
		{
			name: "assistant turns ignored for derivation; only user counts",
			lines: []string{
				`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I will help"}]}}`,
				`{"type":"user","message":{"role":"user","content":"add a dry-run flag"}}`,
			},
			wantTitle: "add a dry-run flag",
			wantFound: true,
		},
		{
			name: "multiline user text collapsed to single line",
			lines: []string{
				`{"type":"user","message":{"role":"user","content":"line one\n\nline two    spaced"}}`,
			},
			wantTitle: "line one line two spaced",
			wantFound: true,
		},
		{
			name: "all low-signal yields no title",
			lines: []string{
				`{"type":"user","message":{"role":"user","content":"hi"}}`,
				`{"type":"user","message":{"role":"user","content":"/clear"}}`,
				`{"type":"user","message":{"role":"user","content":"   "}}`,
			},
			wantTitle: "",
			wantFound: false,
		},
		{
			name:      "no records yields no title",
			lines:     nil,
			wantTitle: "",
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, found := DeriveTitle(decodeRecords(t, tt.lines))
			if got != tt.wantTitle || found != tt.wantFound {
				t.Errorf("DeriveTitle() = (%q, %v), want (%q, %v)", got, found, tt.wantTitle, tt.wantFound)
			}
		})
	}
}

func TestDeriveTitle_CapAndNilRecords(t *testing.T) {
	t.Parallel()
	// A nil record in the slice must be skipped, not panic.
	long := strings.Repeat("x", TitleCap+50)
	recs := []map[string]any{
		nil,
		{"type": "user", "message": map[string]any{"role": "user", "content": long}},
	}
	got, found := DeriveTitle(recs)
	if !found {
		t.Fatal("want found=true")
	}
	if len([]rune(got)) != TitleCap {
		t.Errorf("title rune len = %d, want %d", len([]rune(got)), TitleCap)
	}
}

func TestISOToEpoch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{name: "Z suffix UTC", in: "2026-06-18T12:00:00Z", want: 1781784000.0},
		{name: "explicit +00:00 offset", in: "2026-06-18T12:00:00+00:00", want: 1781784000.0},
		{name: "tz-naive treated as UTC", in: "2026-06-18T12:00:00", want: 1781784000.0},
		{name: "empty string", in: "", want: 0.0},
		{name: "date only treated as UTC midnight", in: "2026-06-18", want: 1781740800.0},
		{name: "garbage", in: "garbage", want: 0.0},
		{name: "fractional seconds with Z", in: "2026-06-18T12:00:00.123456Z", want: 1781784000.123456},
		{name: "space separator naive", in: "2026-06-18 12:00:00", want: 1781784000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ISOToEpoch(tt.in)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("ISOToEpoch(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestISOToEpoch_OffsetApplied(t *testing.T) {
	t.Parallel()
	// 12:00 at +02:00 is 10:00 UTC = 1781784000 - 7200.
	got := ISOToEpoch("2026-06-18T12:00:00+02:00")
	want := 1781784000.0 - 7200.0
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("ISOToEpoch(+02:00) = %v, want %v", got, want)
	}
}

func TestDisp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		content      string
		includeTools bool
		cap          int
		want         string
	}{
		{
			name:         "strip tools and collapse",
			content:      "a [TOOL:x] b   c",
			includeTools: false,
			cap:          200,
			want:         "a",
		},
		{
			name:         "include tools keeps marker",
			content:      "a [TOOL:x] b",
			includeTools: true,
			cap:          200,
			want:         "a [TOOL:x] b",
		},
		{
			name:         "cap truncates",
			content:      "abcdefghij",
			includeTools: false,
			cap:          4,
			want:         "abcd",
		},
		{
			name:         "empty content",
			content:      "",
			includeTools: false,
			cap:          200,
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Disp(tt.content, tt.includeTools, tt.cap)
			if got != tt.want {
				t.Errorf("Disp(%q, %v, %d) = %q, want %q",
					tt.content, tt.includeTools, tt.cap, got, tt.want)
			}
		})
	}
}

func TestCapRunes_MultibyteBoundary(t *testing.T) {
	t.Parallel()
	// Three 2-byte runes; cap at 2 runes must yield 4 bytes, not split a rune.
	got := capRunes("éé é", 2)
	want := "éé"
	if got != want {
		t.Errorf("capRunes = %q, want %q", got, want)
	}
}

func TestModuleConstants(t *testing.T) {
	t.Parallel()
	if BlockCap != 8000 || ToolInCap != 500 || ToolResCap != 1000 {
		t.Errorf("caps drifted: BlockCap=%d ToolInCap=%d ToolResCap=%d", BlockCap, ToolInCap, ToolResCap)
	}
	want := []string{"user", "assistant", "summary", "system"}
	if len(IndexableTypes) != len(want) {
		t.Fatalf("IndexableTypes = %v, want %v", IndexableTypes, want)
	}
	for i, v := range want {
		if IndexableTypes[i] != v {
			t.Errorf("IndexableTypes[%d] = %q, want %q", i, IndexableTypes[i], v)
		}
	}
}
