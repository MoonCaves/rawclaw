package claude

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

// TestMessages proves the lift is faithful: the adapter yields exactly the
// indexable, non-empty records the former index.parseTranscript produced —
// skipping blank lines, malformed JSON, non-indexable types, and empty text.
func TestMessages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	f := filepath.Join(dir, "sess1.jsonl")
	writeJSONL(t, f,
		`{"type":"user","uuid":"u1","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"first question about deploy"}}`,
		`{"type":"assistant","uuid":"u2","timestamp":"2026-06-01T10:01:00Z","message":{"role":"assistant","content":[{"type":"text","text":"the answer is here"}]}}`,
		``, // blank -> skipped
		`{"type":"summary","summary":"a short recap"}`,
		`{"type":"user","uuid":"u4","timestamp":"2026-06-01T10:02:00Z","message":{"role":"user","content":""}}`, // empty text -> skipped
		`{not valid json`, // malformed -> skipped (counted+logged)
	)

	a := New()
	got, err := a.Messages(source.Container{Path: f})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 indexable messages, got %d: %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].UUID != "u1" || got[0].Text == "" {
		t.Errorf("msg0 unexpected: %+v", got[0])
	}
	if got[1].Role != "assistant" || got[1].UUID != "u2" {
		t.Errorf("msg1 unexpected: %+v", got[1])
	}
	if got[2].Role != "summary" {
		t.Errorf("msg2 (summary) unexpected: %+v", got[2])
	}
	if got[0].TS == 0 || got[0].TSISO == "" {
		t.Errorf("msg0 timestamp not parsed: ts=%v iso=%q", got[0].TS, got[0].TSISO)
	}
}

// TestMessagesMissingFile returns a wrapped error, never a panic.
func TestMessagesMissingFile(t *testing.T) {
	t.Parallel()
	a := New()
	if _, err := a.Messages(source.Container{Path: filepath.Join(t.TempDir(), "nope.jsonl")}); err == nil {
		t.Fatal("want error for a missing file, got nil")
	}
}

// TestDiscover tags top-level vs subagent sessions with the right lineage.
func TestDiscover(t *testing.T) {
	// Not parallel: mutates CLAUDE_CONFIG_DIR via t.Setenv.
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	proj := filepath.Join(root, "projects", "-Users-octocat-demo")
	writeJSONL(t, filepath.Join(proj, "top.jsonl"),
		`{"type":"user","uuid":"a","timestamp":"2026-06-01T10:00:00Z","cwd":"/Users/octocat/demo","message":{"role":"user","content":"hi"}}`,
	)
	writeJSONL(t, filepath.Join(proj, "top", "subagents", "sub.jsonl"),
		`{"type":"user","uuid":"b","timestamp":"2026-06-01T10:05:00Z","message":{"role":"user","content":"sub work"}}`,
	)

	got, err := New().Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	var top, sub *source.Container
	for i := range got {
		switch {
		case got[i].IsSubagent:
			sub = &got[i]
		default:
			top = &got[i]
		}
	}
	if top == nil || sub == nil {
		t.Fatalf("want one top-level + one subagent container, got %+v", got)
	}
	if top.ID != "top" || top.IsSubagent {
		t.Errorf("top container wrong: %+v", *top)
	}
	if sub.ID != "top/sub" || sub.ParentID != "top" {
		t.Errorf("subagent container wrong: %+v", *sub)
	}
	if len(top.ResumeArgv) != 3 || top.ResumeArgv[0] != "claude" || top.ResumeArgv[2] != "top" {
		t.Errorf("top resume argv wrong: %v", top.ResumeArgv)
	}
}
