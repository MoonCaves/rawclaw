package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/source"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("bad fixture json: %v", err)
	}
	return m
}

func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		json     string
		wantRole string
		wantText string
		wantOK   bool
	}{
		{
			name:     "user message",
			json:     `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
			wantRole: "user", wantText: "hello", wantOK: true,
		},
		{
			name:     "developer maps to system",
			json:     `{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"be terse"}]}}`,
			wantRole: "system", wantText: "be terse", wantOK: true,
		},
		{
			name:     "assistant multi-block joins",
			json:     `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a"},{"type":"output_text","text":"b"}]}}`,
			wantRole: "assistant", wantText: "a\nb", wantOK: true,
		},
		{
			name:     "reasoning marks THINKING",
			json:     `{"type":"response_item","payload":{"type":"reasoning","summary":"weighing options"}}`,
			wantRole: "assistant", wantText: "[THINKING] weighing options", wantOK: true,
		},
		{
			name:     "function_call marks TOOL",
			json:     `{"type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"ls\"}"}}`,
			wantRole: "assistant", wantText: `[TOOL:shell] {"cmd":"ls"}`, wantOK: true,
		},
		{
			name:     "function_call_output marks result",
			json:     `{"type":"response_item","payload":{"type":"function_call_output","output":"file listing"}}`,
			wantRole: "tool", wantText: "[TOOL_RESULT] file listing", wantOK: true,
		},
		{
			name:     "web_search_call keeps the query",
			json:     `{"type":"response_item","payload":{"type":"web_search_call","action":{"type":"search","query":"github foo bar"}}}`,
			wantRole: "assistant", wantText: "[TOOL:web_search] github foo bar", wantOK: true,
		},
		{
			name:     "tool_search_call arguments is an object, not a json string",
			json:     `{"type":"response_item","payload":{"type":"tool_search_call","arguments":{"query":"grep app","limit":5}}}`,
			wantRole: "assistant", wantText: "[TOOL:tool_search] grep app", wantOK: true,
		},
		{
			name:     "tool_search_output marks result",
			json:     `{"type":"response_item","payload":{"type":"tool_search_output","output":"3 results"}}`,
			wantRole: "tool", wantText: "[TOOL_RESULT] 3 results", wantOK: true,
		},
		{
			name:     "custom_tool_call keeps name + input",
			json:     `{"type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch"}}`,
			wantRole: "assistant", wantText: "[TOOL:apply_patch] *** Begin Patch", wantOK: true,
		},
		{
			name:     "custom_tool_call_output marks result",
			json:     `{"type":"response_item","payload":{"type":"custom_tool_call_output","output":"patched"}}`,
			wantRole: "tool", wantText: "[TOOL_RESULT] patched", wantOK: true,
		},
		{
			name:     "image_generation_call keeps the prompt",
			json:     `{"type":"response_item","payload":{"type":"image_generation_call","prompt":"a red cube"}}`,
			wantRole: "assistant", wantText: "[TOOL:image_generation] a red cube", wantOK: true,
		},
		{
			name:     "tool_search_call arguments as a json string still reads",
			json:     `{"type":"response_item","payload":{"type":"tool_search_call","arguments":"grep app"}}`,
			wantRole: "assistant", wantText: "[TOOL:tool_search] grep app", wantOK: true,
		},
		{
			name:     "tool_search_call object without query falls back to compact json",
			json:     `{"type":"response_item","payload":{"type":"tool_search_call","arguments":{"limit":5}}}`,
			wantRole: "assistant", wantText: `[TOOL:tool_search] {"limit":5}`, wantOK: true,
		},
		{
			name:     "web_search_call with a missing action yields no trailing space",
			json:     `{"type":"response_item","payload":{"type":"web_search_call"}}`,
			wantRole: "assistant", wantText: "[TOOL:web_search]", wantOK: true,
		},
		{
			name:     "function_call_output object output shape",
			json:     `{"type":"response_item","payload":{"type":"function_call_output","output":{"output":"nested"}}}`,
			wantRole: "tool", wantText: "[TOOL_RESULT] nested", wantOK: true,
		},
		{
			name:     "function_call_output content-block shape",
			json:     `{"type":"response_item","payload":{"type":"function_call_output","output":{"content":[{"type":"output_text","text":"blk"}]}}}`,
			wantRole: "tool", wantText: "[TOOL_RESULT] blk", wantOK: true,
		},
		{
			name:     "reasoning array summary blocks join",
			json:     `{"type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"first"},{"type":"summary_text","text":"second"}]}}`,
			wantRole: "assistant", wantText: "[THINKING] first\nsecond", wantOK: true,
		},
		{
			name:   "reasoning with an empty summary is not indexable",
			json:   `{"type":"response_item","payload":{"type":"reasoning","summary":""}}`,
			wantOK: false,
		},
		{
			name:     "message with empty content is ok but textless (dropped downstream)",
			json:     `{"type":"response_item","payload":{"type":"message","role":"user","content":[]}}`,
			wantRole: "user", wantText: "", wantOK: true,
		},
		{
			name:     "unknown role passes through untouched",
			json:     `{"type":"response_item","payload":{"type":"message","role":"tool","content":[{"type":"output_text","text":"t"}]}}`,
			wantRole: "tool", wantText: "t", wantOK: true,
		},
		{
			name:   "event_msg skipped",
			json:   `{"type":"event_msg","payload":{"type":"token_count"}}`,
			wantOK: false,
		},
		{
			name:   "session_meta skipped",
			json:   `{"type":"session_meta","payload":{"id":"x"}}`,
			wantOK: false,
		},
		{
			name:   "unknown response_item type skipped",
			json:   `{"type":"response_item","payload":{"type":"world_state"}}`,
			wantOK: false,
		},
		{
			name:   "payload not an object is skipped, not a panic",
			json:   `{"type":"response_item","payload":"oops"}`,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			role, text, ok := normalize(decode(t, tt.json))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if role != tt.wantRole || text != tt.wantText {
				t.Errorf("got (%q,%q), want (%q,%q)", role, text, tt.wantRole, tt.wantText)
			}
		})
	}
}

func TestMintUUIDStableAndDistinct(t *testing.T) {
	t.Parallel()
	a := mintUUID("sess", 0)
	if a != mintUUID("sess", 0) {
		t.Error("mintUUID must be stable for the same (session, ordinal)")
	}
	if a == mintUUID("sess", 1) {
		t.Error("mintUUID must differ across ordinals")
	}
	if len(a) != 16 {
		t.Errorf("uuid length = %d, want 16", len(a))
	}
}

func TestMessagesOrdinalUUIDs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "rollout-x.jsonl")
	writeJSONL(t, f,
		`{"type":"session_meta","timestamp":"2026-07-15T10:00:00Z","payload":{"id":"s1","cwd":"/repo"}}`,
		`{"type":"response_item","timestamp":"2026-07-15T10:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"q1"}]}}`,
		`{"type":"event_msg","timestamp":"2026-07-15T10:00:02Z","payload":{"type":"token_count"}}`,
		`{not valid`, // malformed -> skipped
		`{"type":"response_item","timestamp":"2026-07-15T10:00:03Z","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a1"}]}}`,
	)
	got, err := New().Messages(source.Container{ID: "s1", Path: f})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 indexable messages, got %d: %+v", len(got), got)
	}
	if got[0].UUID != mintUUID("s1", 0) || got[1].UUID != mintUUID("s1", 1) {
		t.Errorf("uuids not ordinal: %q,%q", got[0].UUID, got[1].UUID)
	}
	if got[0].Text != "q1" || got[1].Text != "a1" {
		t.Errorf("text wrong: %+v", got)
	}
	if got[0].TS == 0 || got[0].TSISO == "" {
		t.Errorf("timestamp not parsed: %+v", got[0])
	}
}

func TestReadMetaLineage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	root := filepath.Join(dir, "rollout-root.jsonl")
	writeJSONL(t, root,
		`{"type":"session_meta","payload":{"id":"root1","cwd":"/repo","thread_source":"user","parent_thread_id":null,"forked_from_id":null}}`,
	)
	child := filepath.Join(dir, "rollout-child.jsonl")
	writeJSONL(t, child,
		`{"type":"session_meta","payload":{"id":"child1","cwd":"/repo","thread_source":"subagent","parent_thread_id":"root1","forked_from_id":"root1"}}`,
		`{"type":"session_meta","payload":{"id":"root1","thread_source":"user"}}`, // replayed parent header, must be ignored
	)

	rm, ok := readMeta(root)
	if !ok || rm.id != "root1" || rm.isChild() {
		t.Errorf("root meta wrong: %+v ok=%v isChild=%v", rm, ok, rm.isChild())
	}
	cm, ok := readMeta(child)
	if !ok || cm.id != "child1" || !cm.isChild() || cm.parent() != "root1" {
		t.Errorf("child meta wrong: %+v ok=%v isChild=%v parent=%q", cm, ok, cm.isChild(), cm.parent())
	}
}

func TestDiscover(t *testing.T) {
	// Not parallel: sets CODEX_HOME via t.Setenv.
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	day := filepath.Join(home, "sessions", "2026", "07", "15")
	writeJSONL(t, filepath.Join(day, "rollout-root.jsonl"),
		`{"type":"session_meta","payload":{"id":"root1","cwd":"/repo","thread_source":"user"}}`,
	)
	writeJSONL(t, filepath.Join(day, "rollout-child.jsonl"),
		`{"type":"session_meta","payload":{"id":"child1","cwd":"/repo","thread_source":"subagent","parent_thread_id":"root1"}}`,
	)

	got, err := New().Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 containers, got %d: %+v", len(got), got)
	}
	byID := map[string]source.Container{}
	for _, c := range got {
		byID[c.ID] = c
	}
	if r := byID["root1"]; r.IsSubagent || r.CWD != "/repo" || len(r.ResumeArgv) != 3 || r.ResumeArgv[0] != "codex" {
		t.Errorf("root container wrong: %+v", r)
	}
	if c := byID["child1"]; !c.IsSubagent || c.ParentID != "root1" {
		t.Errorf("child container wrong: %+v", c)
	}
}

func TestDiscoverAbsentTreeIsNotError(t *testing.T) {
	home := t.TempDir() // no sessions/ subdir
	t.Setenv("CODEX_HOME", home)
	got, err := New().Discover()
	if err != nil {
		t.Fatalf("absent tree must not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 containers for an empty tree, got %d", len(got))
	}
}

// A rollout whose header is unreadable must be skipped without failing the walk:
// its siblings still get discovered. Regression guard for a single corrupt file
// silently blanking a whole day's corpus.
func TestDiscoverSkipsUnreadableHeader(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	day := filepath.Join(home, "sessions", "2026", "07", "15")
	writeJSONL(t, filepath.Join(day, "rollout-good.jsonl"),
		`{"type":"session_meta","payload":{"id":"good1","cwd":"/repo","thread_source":"user"}}`,
	)
	// First line is not a decodable session_meta -> readMeta fails -> skipped.
	writeJSONL(t, filepath.Join(day, "rollout-bad.jsonl"), `{not json at all`)
	// A non-rollout file in the tree must be ignored entirely.
	writeJSONL(t, filepath.Join(day, "notes.txt"), `ignore me`)

	got, err := New().Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].ID != "good1" {
		t.Fatalf("want only the good rollout, got %d: %+v", len(got), got)
	}
}

// The minted-uuid ordinal counts only records normalize() indexes, so a record
// it accepts-but-drops (empty text) must NOT consume an ordinal — otherwise every
// later ref shifts. This locks the FOOTGUN documented on mintUUID.
func TestMessagesEmptyContentDoesNotShiftOrdinals(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "rollout-x.jsonl")
	writeJSONL(t, f,
		`{"type":"response_item","timestamp":"2026-07-15T10:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"q1"}]}}`,
		`{"type":"response_item","timestamp":"2026-07-15T10:00:02Z","payload":{"type":"message","role":"assistant","content":[]}}`, // empty -> dropped, no ordinal
		`{"type":"response_item","timestamp":"2026-07-15T10:00:03Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"q2"}]}}`,
	)
	got, err := New().Messages(source.Container{ID: "s1", Path: f})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 messages, got %d: %+v", len(got), got)
	}
	if got[1].Text != "q2" || got[1].UUID != mintUUID("s1", 1) {
		t.Errorf("q2 should hold ordinal 1, got text=%q uuid=%q (want uuid %q)", got[1].Text, got[1].UUID, mintUUID("s1", 1))
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		codexEnv string
		want     bool
	}{
		{"default sessions path", "/home/u/.codex/sessions/2026/07/rollout-x.jsonl", "", true},
		{"unrelated path", "/home/u/.claude/projects/foo/bar.jsonl", "", false},
		{"custom CODEX_HOME sessions", "/data/codex/sessions/rollout-y.jsonl", "/data/codex", true},
		{"custom home unset ignores sessions substring", "/data/codex/sessions/rollout-y.jsonl", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CODEX_HOME", tc.codexEnv)
			if got := detect(tc.path); got != tc.want {
				t.Errorf("detect(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
