package antigravity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/source"
)

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
		rec      map[string]any
		wantRole string
		wantText string
		wantOK   bool
	}{
		{
			name: "user input with USER_REQUEST tag",
			rec: map[string]any{
				"type":    "USER_INPUT",
				"source":  "USER_EXPLICIT",
				"content": "<USER_REQUEST>\nfix the bug\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\ntime\n</ADDITIONAL_METADATA>",
			},
			wantRole: "user",
			wantText: "fix the bug",
			wantOK:   true,
		},
		{
			name: "user input raw string",
			rec: map[string]any{
				"type":    "USER_INPUT",
				"source":  "USER_EXPLICIT",
				"content": "hello world",
			},
			wantRole: "user",
			wantText: "hello world",
			wantOK:   true,
		},
		{
			name: "planner response with thinking and tool calls",
			rec: map[string]any{
				"type":     "PLANNER_RESPONSE",
				"source":   "MODEL",
				"thinking": "thinking about solution",
				"content":  "I will run the command",
				"tool_calls": []any{
					map[string]any{
						"name": "run_command",
						"args": map[string]any{
							"CommandLine": "ls -la",
						},
					},
				},
			},
			wantRole: "assistant",
			wantText: "[THINKING] thinking about solution\nI will run the command\n[TOOL:run_command] ls -la",
			wantOK:   true,
		},
		{
			name: "tool result step",
			rec: map[string]any{
				"type":    "RUN_COMMAND",
				"source":  "MODEL",
				"content": "file1.txt\nfile2.txt",
			},
			wantRole: "tool",
			wantText: "[TOOL_RESULT] file1.txt\nfile2.txt",
			wantOK:   true,
		},
		{
			name: "system message",
			rec: map[string]any{
				"type":    "SYSTEM_MESSAGE",
				"source":  "SYSTEM",
				"content": "Task completed successfully.",
			},
			wantRole: "system",
			wantText: "Task completed successfully.",
			wantOK:   true,
		},
		{
			name: "conversation history skipped",
			rec: map[string]any{
				"type":   "CONVERSATION_HISTORY",
				"source": "SYSTEM",
			},
			wantOK: false,
		},
		{
			name: "checkpoint skipped",
			rec: map[string]any{
				"type":   "CHECKPOINT",
				"source": "SYSTEM",
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, text, ok := normalize(tt.rec)
			if ok != tt.wantOK {
				t.Fatalf("normalize() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if role != tt.wantRole {
				t.Errorf("role = %q, want %q", role, tt.wantRole)
			}
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestDiscoverAndMessages(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Seed history.jsonl
	writeJSONL(t, filepath.Join(tmp, "history.jsonl"),
		`{"conversationId":"sess-1","workspace":"/tmp/project-a"}`,
		`{"conversationId":"sess-2","workspace":"/tmp/project-b"}`,
	)

	// Seed sess-1
	t1 := filepath.Join(tmp, "brain", "sess-1", ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(t, t1,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:00Z","content":"<USER_REQUEST>test prompt</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-14T10:00:01Z","thinking":"planning","content":"doing it"}`,
		`{"step_index":2,"source":"MODEL","type":"INVOKE_SUBAGENT","created_at":"2026-08-14T10:00:02Z","content":"Created the following subagents:\n{\n  \"conversationId\": \"sub-1\",\n  \"workspaceUris\": [\"/tmp/project-a\"]\n}"}`,
	)

	// Seed sub-1 (spawned by sess-1)
	tSub := filepath.Join(tmp, "brain", "sub-1", ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(t, tSub,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:05Z","content":"subagent task"}`,
	)

	ad := NewRoot(tmp)
	containers, err := ad.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("Discover() returned %d containers, want 2", len(containers))
	}

	byID := map[string]source.Container{}
	for _, c := range containers {
		byID[c.ID] = c
	}

	c1, ok := byID["sess-1"]
	if !ok {
		t.Fatal("sess-1 not found")
	}
	if c1.CWD != "/tmp/project-a" {
		t.Errorf("sess-1 CWD = %q, want /tmp/project-a", c1.CWD)
	}
	if c1.IsSubagent {
		t.Error("sess-1 marked as subagent, want false")
	}

	cSub, ok := byID["sub-1"]
	if !ok {
		t.Fatal("sub-1 not found")
	}
	if !cSub.IsSubagent {
		t.Error("sub-1 IsSubagent = false, want true")
	}
	if cSub.ParentID != "sess-1" {
		t.Errorf("sub-1 ParentID = %q, want sess-1", cSub.ParentID)
	}

	// Test Messages
	msgs, err := ad.Messages(c1)
	if err != nil {
		t.Fatalf("Messages() error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("Messages() returned %d messages, want 3", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Text != "test prompt" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("msgs[1].Role = %q, want assistant", msgs[1].Role)
	}
}

func TestDetect(t *testing.T) {
	if !detect("/Users/test/.gemini/antigravity-cli/brain/abc/.system_generated/logs/transcript.jsonl") {
		t.Error("detect() = false for ~/.gemini/antigravity-cli path")
	}
	if detect("/Users/test/.claude/projects/proj/session.jsonl") {
		t.Error("detect() = true for claude path")
	}
}

// TestDiscoverSubagentInHistory verifies Bug 1 fix: a session present in history.jsonl
// must still have its transcript scanned for lineage, rather than being hardcoded
// as IsSubagent:false with no ParentID via history membership or an early return fast path.
func TestDiscoverSubagentInHistory(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Every session is mapped in history.jsonl (would have triggered the old fast path)
	writeJSONL(t, filepath.Join(tmp, "history.jsonl"),
		`{"conversationId":"parent-sess","workspace":"/workspace/main"}`,
		`{"conversationId":"sub-from-invoke","workspace":"/workspace/main"}`,
		`{"conversationId":"sub-from-reminder","workspace":"/workspace/main"}`,
	)

	// Parent session spawning sub-from-invoke
	tParent := filepath.Join(tmp, "brain", "parent-sess", ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(t, tParent,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:00Z","content":"<USER_REQUEST>parent task</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-14T10:00:01Z","thinking":"spawning subagent","content":"spawning subagent"}`,
		`{"step_index":2,"source":"MODEL","type":"INVOKE_SUBAGENT","created_at":"2026-08-14T10:00:02Z","content":"Created subagent:\n{\n  \"conversationId\": \"sub-from-invoke\"\n}"}`,
	)

	// Subagent 1 (spawned by parent via INVOKE_SUBAGENT)
	tSub1 := filepath.Join(tmp, "brain", "sub-from-invoke", ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(t, tSub1,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:05Z","content":"subagent 1 task"}`,
	)

	// Subagent 2 (identifies parent via subagent_reminder in prompt)
	tSub2 := filepath.Join(tmp, "brain", "sub-from-reminder", ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(t, tSub2,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:06Z","content":"<subagent_reminder>\ncaller agent id: parent-sess\n</subagent_reminder>\nsubagent 2 task"}`,
	)

	ad := NewRoot(tmp)
	containers, err := ad.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(containers) != 3 {
		t.Fatalf("Discover() returned %d containers, want 3", len(containers))
	}

	byID := map[string]source.Container{}
	for _, c := range containers {
		byID[c.ID] = c
	}

	parent, ok := byID["parent-sess"]
	if !ok {
		t.Fatal("parent-sess not found")
	}
	if parent.IsSubagent {
		t.Errorf("parent-sess IsSubagent = true, want false")
	}
	if parent.ParentID != "" {
		t.Errorf("parent-sess ParentID = %q, want empty", parent.ParentID)
	}
	if parent.CWD != "/workspace/main" {
		t.Errorf("parent-sess CWD = %q, want /workspace/main", parent.CWD)
	}

	sub1, ok := byID["sub-from-invoke"]
	if !ok {
		t.Fatal("sub-from-invoke not found")
	}
	if !sub1.IsSubagent {
		t.Errorf("sub-from-invoke IsSubagent = false, want true (history.jsonl membership must not clear subagent status)")
	}
	if sub1.ParentID != "parent-sess" {
		t.Errorf("sub-from-invoke ParentID = %q, want parent-sess", sub1.ParentID)
	}
	if sub1.CWD != "/workspace/main" {
		t.Errorf("sub-from-invoke CWD = %q, want /workspace/main", sub1.CWD)
	}

	sub2, ok := byID["sub-from-reminder"]
	if !ok {
		t.Fatal("sub-from-reminder not found")
	}
	if !sub2.IsSubagent {
		t.Errorf("sub-from-reminder IsSubagent = false, want true (history.jsonl membership must not clear subagent status)")
	}
	if sub2.ParentID != "parent-sess" {
		t.Errorf("sub-from-reminder ParentID = %q, want parent-sess", sub2.ParentID)
	}
	if sub2.CWD != "/workspace/main" {
		t.Errorf("sub-from-reminder CWD = %q, want /workspace/main", sub2.CWD)
	}
}
