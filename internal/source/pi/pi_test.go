package pi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/Users/jay-m4/.pi/agent/sessions/--Users-jay-m4-code-rawclaw--/2026-08-28.jsonl", true},
		{"/custom/pi/agent/sessions/test/session.jsonl", true},
		{"/tmp/sessions/--test--/session.jsonl", true},
		{"/Users/jay-m4/.claude/projects/test.jsonl", false},
		{"/Users/jay-m4/.codex/sessions/rollout.jsonl", false},
	} {
		if got := detect(tc.path); got != tc.want {
			t.Errorf("detect(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestDiscoverAndMessages(t *testing.T) {
	tmp := t.TempDir()
	projDir := filepath.Join(tmp, "--Users-tester-myproj--")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(projDir, "2026-08-29T10-00-00-000Z_01a04655-b6eb-7d4d-9d6c-3abcd0d845be.jsonl")
	content := `{"type":"session","version":3,"id":"01a04655-b6eb-7d4d-9d6c-3abcd0d845be","timestamp":"2026-08-29T10:00:00.000Z","cwd":"/Users/tester/myproj"}
{"type":"model_change","id":"m1","timestamp":"2026-08-29T10:00:01.000Z","provider":"claude-max","modelId":"claude-subscription"}
{"type":"message","id":"msg-1","timestamp":"2026-08-29T10:00:02.000Z","message":{"role":"user","content":[{"type":"text","text":"hello pi agent"}],"timestamp":1787886431281}}
{"type":"custom_message","customType":"mnemon","content":"[mnemon] Memory active","id":"cust-1","timestamp":"2026-08-29T10:00:03.000Z"}
{"type":"message","id":"msg-2","timestamp":"2026-08-29T10:00:04.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"pondering answer"},{"type":"text","text":"hello human!"},{"type":"toolCall","id":"t1","name":"bash","arguments":{"command":"ls"}}],"timestamp":1787886435000}}
{"type":"message","id":"msg-3","timestamp":"2026-08-29T10:00:06.000Z","message":{"role":"toolResult","toolCallId":"t1","toolName":"bash","content":[{"type":"text","text":"file1.txt\nfile2.txt"}]}}
{"type":"compaction","id":"cmp-1","summary":"Summarized previous discussion","timestamp":"2026-08-29T10:00:10.000Z"}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	adapter := NewRoot(tmp)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("Discover() returned %d containers, want 1", len(containers))
	}

	c := containers[0]
	if c.ID != "01a04655-b6eb-7d4d-9d6c-3abcd0d845be" {
		t.Errorf("Container ID = %q, want %q", c.ID, "01a04655-b6eb-7d4d-9d6c-3abcd0d845be")
	}
	if c.CWD != "/Users/tester/myproj" {
		t.Errorf("Container CWD = %q, want %q", c.CWD, "/Users/tester/myproj")
	}

	msgs, err := adapter.Messages(c)
	if err != nil {
		t.Fatalf("Messages() error: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("Messages() returned %d messages, want 5", len(msgs))
	}

	// Message 0: User
	if msgs[0].Role != "user" || msgs[0].Text != "hello pi agent" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	// Message 1: Custom message
	if msgs[1].Role != "system" || msgs[1].Text != "[MNEMON] [mnemon] Memory active" {
		t.Errorf("msgs[1] = %+v", msgs[1])
	}
	// Message 2: Assistant with text, thinking, toolCall
	if msgs[2].Role != "assistant" {
		t.Errorf("msgs[2].Role = %q, want assistant", msgs[2].Role)
	}
	// Message 3: ToolResult
	if msgs[3].Role != "assistant" {
		t.Errorf("msgs[3].Role = %q, want assistant", msgs[3].Role)
	}
	// Message 4: Compaction summary
	if msgs[4].Role != "summary" || msgs[4].Text != "[SUMMARY] Summarized previous discussion" {
		t.Errorf("msgs[4] = %+v", msgs[4])
	}
}

func TestLookup(t *testing.T) {
	origRoots := SessionsRoots()
	_ = origRoots

	tmp := t.TempDir()
	sessionFile := filepath.Join(tmp, "2026-08-29_test-pi-lookup.jsonl")
	content := `{"type":"session","version":3,"id":"test-pi-lookup","cwd":"/tmp/lookuptest"}
{"type":"message","id":"msg-1","message":{"role":"user","content":"lookup content"}}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	a := NewRoot(tmp)
	containers, err := a.Discover()
	if err != nil || len(containers) != 1 {
		t.Fatalf("Discover() error: %v, containers: %v", err, containers)
	}

	if containers[0].ResumeArgv[0] != "pi" || containers[0].ResumeArgv[2] != "test-pi-lookup" {
		t.Errorf("ResumeArgv = %v, want [pi --session test-pi-lookup]", containers[0].ResumeArgv)
	}
}
