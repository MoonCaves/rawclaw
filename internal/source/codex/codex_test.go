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
