package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
)

func writeRichReadSession(t *testing.T, root, project, id string) {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"user","uuid":"9f3e1c20-0000-0000-0000-000000000001","timestamp":"2026-06-01T10:00:00Z","cwd":"/home/user/proj","message":{"role":"user","content":"How do we optimize DB queries?"}}`,
		`{"type":"assistant","uuid":"a2b3c4d5-0000-0000-0000-000000000002","timestamp":"2026-06-01T10:00:05Z","cwd":"/home/user/proj","message":{"role":"assistant","content":"[THINKING] Let's consider indexing and query plans\nI recommend adding compound indexes."}}`,
		`{"type":"assistant","uuid":"b3c4d5e6-0000-0000-0000-000000000003","timestamp":"2026-06-01T10:00:10Z","cwd":"/home/user/proj","message":{"role":"assistant","content":"[TOOL:Bash] {\"command\": \"SELECT * FROM users\"}\n[TOOL_RESULT] query output returned\nExecution complete."}}`,
		`{"type":"user","uuid":"c4d5e6f7-0000-0000-0000-000000000004","timestamp":"2026-06-01T10:01:00Z","cwd":"/home/user/proj","message":{"role":"user","content":"Great, apply the indexes now."}}`,
	}
	p := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReadWithThinking verifies that --with thinking reveals thinking chunks in read output.
func TestReadWithThinking(t *testing.T) {
	root := newCfgRoot(t)
	sessID := "aaaa1111-0000-0000-0000-000000000001"
	writeRichReadSession(t, root, "-home-u-proj", sessID)

	ref := "aaaa1111:a2b3c4d5"

	// Default read: thinking stripped
	outDefault, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref, "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("read: %v\n%s", err, outDefault)
	}
	if strings.Contains(outDefault, "THINKING") || strings.Contains(outDefault, "indexing and query plans") {
		t.Errorf("default read should not contain THINKING, got:\n%s", outDefault)
	}
	if !strings.Contains(outDefault, "I recommend adding compound indexes") {
		t.Errorf("default read missing main prose, got:\n%s", outDefault)
	}

	// Read with thinking: thinking included
	outThinking, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref, "--with", "thinking", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("read --with thinking: %v\n%s", err, outThinking)
	}
	if !strings.Contains(outThinking, "THINKING") || !strings.Contains(outThinking, "indexing and query plans") {
		t.Errorf("read --with thinking should contain thinking, got:\n%s", outThinking)
	}
}

// TestReadWithTools verifies that --with tools and legacy --include-tools include tool calls.
func TestReadWithTools(t *testing.T) {
	root := newCfgRoot(t)
	sessID := "aaaa1111-0000-0000-0000-000000000001"
	writeRichReadSession(t, root, "-home-u-proj", sessID)

	// Anchor on user message #1, so message #3 is a context neighbor
	ref := "aaaa1111:9f3e1c20"

	// Default read: tool runs stripped from neighbor message
	outDefault, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref, "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("read: %v\n%s", err, outDefault)
	}
	if strings.Contains(outDefault, "SELECT * FROM users") {
		t.Errorf("default read should not contain tool runs in context, got:\n%s", outDefault)
	}

	// Read with --with tools
	outWithTools, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref, "--with", "tools", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("read --with tools: %v\n%s", err, outWithTools)
	}
	if !strings.Contains(outWithTools, "SELECT * FROM users") {
		t.Errorf("read --with tools should contain tool run, got:\n%s", outWithTools)
	}

	// Legacy --include-tools flag
	outLegacy, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref, "--include-tools", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("read --include-tools: %v\n%s", err, outLegacy)
	}
	if !strings.Contains(outLegacy, "SELECT * FROM users") {
		t.Errorf("read --include-tools should contain tool run, got:\n%s", outLegacy)
	}
}

// TestOutlineWithThinkingAndTools verifies outline with --with thinking,tools.
func TestOutlineWithThinkingAndTools(t *testing.T) {
	root := newCfgRoot(t)
	sessID := "aaaa1111-0000-0000-0000-000000000001"
	writeRichReadSession(t, root, "-home-u-proj", sessID)

	// Outline with --with thinking,tools
	outWith, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", "aaaa1111", "--with", "thinking,tools", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("outline --with thinking,tools: %v\n%s", err, outWith)
	}
	if !strings.Contains(outWith, "GOAL") {
		t.Errorf("outline missing GOAL, got:\n%s", outWith)
	}

	// JSON outline
	outJSON, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", "aaaa1111", "--with", "thinking", "--json", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("outline --json: %v\n%s", err, outJSON)
	}
	var res agentproto.OutlineResult
	if err := json.Unmarshal([]byte(outJSON), &res); err != nil {
		t.Fatalf("json.Unmarshal OutlineResult: %v\n%s", err, outJSON)
	}
	if res.SessionID != sessID {
		t.Errorf("res.SessionID = %q, want %q", res.SessionID, sessID)
	}
}

// TestWithValidationErrors verifies invalid --with options return error.
func TestWithValidationErrors(t *testing.T) {
	newCfgRoot(t)
	_, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", "aaaa1111:a2b3c4d5", "--with", "invalid")
	if err == nil {
		t.Fatalf("expected error for --with invalid, got nil")
	}
	if !strings.Contains(err.Error(), "invalid --with choice") {
		t.Errorf("err = %q, want 'invalid --with choice'", err.Error())
	}
}
