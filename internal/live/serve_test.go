package live

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newServeHome isolates both transcript roots under a temp HOME and returns
// (claude projects root, codex sessions root).
func newServeHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeRoot := filepath.Join(home, ".claude", "projects")
	codexRoot := filepath.Join(home, ".codex", "sessions")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(codexRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return claudeRoot, codexRoot
}

// writeClaudeSession writes a top-level Claude transcript with the given cwd
// and messages, then stamps mtime.
func writeClaudeSession(t *testing.T, root, project, id, cwd string, mtime time.Time, texts ...string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i, txt := range texts {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		line, err := json.Marshal(map[string]any{
			"type":      role,
			"cwd":       cwd,
			"timestamp": mtime.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339),
			"uuid":      id + "-u" + string(rune('0'+i)),
			"message":   map[string]any{"role": role, "content": txt},
		})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	p := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, time.Time{}, mtime); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeCodexSession writes a codex rollout with a session_meta header + one
// message, then stamps mtime.
func writeCodexSession(t *testing.T, root, id, cwd string, mtime time.Time) string {
	t.Helper()
	dir := filepath.Join(root, "2026", "07", "19")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `","thread_source":"user"}}` + "\n" +
		`{"type":"response_item","timestamp":"2026-07-19T10:00:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"codex hello"}]}}` + "\n"
	p := filepath.Join(dir, "rollout-"+id+".jsonl")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, time.Time{}, mtime); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestServeList: sessions from both runtimes, ordered by last activity
// descending, subagents excluded, limit honored, JSON over the pipe.
func TestServeList(t *testing.T) {
	claudeRoot, codexRoot := newServeHome(t)
	now := time.Now()

	writeClaudeSession(t, claudeRoot, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", now.Add(-30*time.Second), "newest claude")
	writeClaudeSession(t, claudeRoot, "-proj-b", "bbbb2222-0000-0000-0000-000000000002",
		"/home/u/proj-b", now.Add(-2*time.Hour), "older claude")
	writeCodexSession(t, codexRoot, "cccc3333-0000-0000-0000-000000000003",
		"/home/u/proj-c", now.Add(-5*time.Minute))

	// A subagent thread must not appear in the list.
	subDir := filepath.Join(claudeRoot, "-proj-a", "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "agent-x.jsonl"),
		[]byte(`{"type":"user","message":{"role":"user","content":"sub"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := ServeList(&buf, 10); err != nil {
		t.Fatalf("ServeList: %v", err)
	}

	var got []Session
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("ServeList output is not a JSON session array: %v\n%s", err, buf.String())
	}
	if len(got) != 3 {
		t.Fatalf("ServeList returned %d sessions, want 3 (subagent excluded):\n%s", len(got), buf.String())
	}
	wantOrder := []string{
		"aaaa1111-0000-0000-0000-000000000001", // -30s
		"cccc3333-0000-0000-0000-000000000003", // -5m
		"bbbb2222-0000-0000-0000-000000000002", // -2h
	}
	for i, want := range wantOrder {
		if got[i].SessionID != want {
			t.Errorf("row %d = %s, want %s (order = most recent first)", i, got[i].SessionID, want)
		}
	}
	if got[0].Source != "claude" || got[1].Source != "codex" {
		t.Errorf("sources = %s,%s want claude,codex", got[0].Source, got[1].Source)
	}
	if got[0].Project != "proj-a" {
		t.Errorf("project = %q, want proj-a (basename of cwd)", got[0].Project)
	}
	if got[0].AgeSeconds < 0 || got[0].AgeSeconds > 300 {
		t.Errorf("age_seconds = %d, want ~30", got[0].AgeSeconds)
	}
	if got[0].LastActivity == "" {
		t.Error("last_activity empty")
	}
	if got[0].SizeBytes <= 0 {
		t.Error("size_bytes not set")
	}
}

// TestServeList_Limit: the limit caps the rows, keeping the most recent.
func TestServeList_Limit(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	now := time.Now()
	writeClaudeSession(t, claudeRoot, "-p", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/p", now.Add(-time.Minute), "m1")
	writeClaudeSession(t, claudeRoot, "-p", "bbbb2222-0000-0000-0000-000000000002",
		"/home/u/p", now.Add(-time.Hour), "m2")

	var buf bytes.Buffer
	if err := ServeList(&buf, 1); err != nil {
		t.Fatalf("ServeList: %v", err)
	}
	var got []Session
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != "aaaa1111-0000-0000-0000-000000000001" {
		t.Errorf("limit=1 rows = %+v, want just the most recent", got)
	}
}

// TestServeList_Empty: an empty corpus emits an empty JSON array, not an error
// — the client renders the "nothing running" story.
func TestServeList_Empty(t *testing.T) {
	newServeHome(t)
	var buf bytes.Buffer
	if err := ServeList(&buf, 10); err != nil {
		t.Fatalf("ServeList on empty corpus: %v", err)
	}
	var got []Session
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("rows = %d, want 0", len(got))
	}
}
