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
