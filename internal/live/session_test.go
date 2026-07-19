package live

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestServeSession_RendersTranscript: a prefix resolves to the session and the
// rendered output carries its messages — including the freshest one — plus the
// session header. This is the raw one-shot render the client streams through.
func TestServeSession_RendersTranscript(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	writeClaudeSession(t, claudeRoot, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", time.Now().Add(-10*time.Second),
		"first question", "working on it", "written seconds ago")

	var buf bytes.Buffer
	if err := ServeSession(&buf, "aaaa1111", 0, false); err != nil {
		t.Fatalf("ServeSession: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"aaaa1111-0000-0000-0000-000000000001", // header names the session
		"proj-a",                               // and its project
		"first question",
		"working on it",
		"written seconds ago", // the freshest message is present
		"user", "assistant",   // roles rendered
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
}

// TestServeSession_TailCapsMessages: tail=N keeps only the last N messages and
// says how many were omitted — a snapshot of "now", not a full replay.
func TestServeSession_TailCapsMessages(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	writeClaudeSession(t, claudeRoot, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", time.Now(),
		"msg-one", "msg-two", "msg-three", "msg-four")

	var buf bytes.Buffer
	if err := ServeSession(&buf, "aaaa1111", 2, false); err != nil {
		t.Fatalf("ServeSession: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "msg-one") || strings.Contains(out, "msg-two") {
		t.Errorf("tail=2 leaked earlier messages:\n%s", out)
	}
	if !strings.Contains(out, "msg-three") || !strings.Contains(out, "msg-four") {
		t.Errorf("tail=2 dropped the last messages:\n%s", out)
	}
	if !strings.Contains(out, "2 of 4") {
		t.Errorf("tail render should say how much was omitted (want \"2 of 4\"):\n%s", out)
	}
}

// TestServeSession_JSON: --json emits the session + message tail as one JSON
// object for machine consumers.
func TestServeSession_JSON(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	writeClaudeSession(t, claudeRoot, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", time.Now(), "hello", "hi there")

	var buf bytes.Buffer
	if err := ServeSession(&buf, "aaaa1111", 0, true); err != nil {
		t.Fatalf("ServeSession --json: %v", err)
	}
	var got struct {
		SessionID string `json:"session_id"`
		Source    string `json:"source"`
		Project   string `json:"project"`
		Total     int    `json:"total_messages"`
		Messages  []struct {
			Role string `json:"role"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	if got.SessionID != "aaaa1111-0000-0000-0000-000000000001" || got.Source != "claude" {
		t.Errorf("json header = %+v", got)
	}
	if got.Total != 2 || len(got.Messages) != 2 {
		t.Errorf("messages = %d/%d, want 2/2", len(got.Messages), got.Total)
	}
	if got.Messages[1].Role != "assistant" || got.Messages[1].Text != "hi there" {
		t.Errorf("last message = %+v", got.Messages[1])
	}
}

// TestServeSession_NoMatch: an unmatched prefix is a distinct, actionable
// error naming the prefix.
func TestServeSession_NoMatch(t *testing.T) {
	newServeHome(t)
	var buf bytes.Buffer
	err := ServeSession(&buf, "deadbeef", 0, false)
	if err == nil {
		t.Fatal("ServeSession on unknown prefix: want error, got nil")
	}
	if !strings.Contains(err.Error(), "deadbeef") {
		t.Errorf("error should name the prefix: %v", err)
	}
}

// TestServeSession_EmptyPrefix: an empty prefix would match every session —
// reject it instead of "rendering" an arbitrary one.
func TestServeSession_EmptyPrefix(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	writeClaudeSession(t, claudeRoot, "-p", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/p", time.Now(), "m")

	var buf bytes.Buffer
	if err := ServeSession(&buf, "", 0, false); err == nil {
		t.Fatal("empty prefix: want error, got nil")
	}
}

// TestServeSession_AmbiguousCapped: a floods-everything prefix lists at most a
// handful of candidates plus a count — an error message, not a dump.
func TestServeSession_AmbiguousCapped(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	for i := 0; i < 15; i++ {
		writeClaudeSession(t, claudeRoot, "-p",
			fmt.Sprintf("aaaa1111-%04d-0000-0000-000000000000", i),
			"/home/u/p", time.Now(), "m")
	}
	var buf bytes.Buffer
	err := ServeSession(&buf, "aaaa1111", 0, false)
	if err == nil {
		t.Fatal("ambiguous prefix: want error, got nil")
	}
	if n := strings.Count(err.Error(), "aaaa1111-"); n > 10 {
		t.Errorf("ambiguous error lists %d candidates, want <= 10", n)
	}
	if !strings.Contains(err.Error(), "15 sessions match") {
		t.Errorf("ambiguous error should carry the full count: %v", err)
	}
}

// TestServeSession_Ambiguous: two sessions sharing the prefix produce an error
// listing both ids so the caller can narrow.
func TestServeSession_Ambiguous(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	writeClaudeSession(t, claudeRoot, "-p", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/p", time.Now(), "m")
	writeClaudeSession(t, claudeRoot, "-p", "aaaa1111-ffff-0000-0000-000000000002",
		"/home/u/p", time.Now(), "m")

	var buf bytes.Buffer
	err := ServeSession(&buf, "aaaa1111", 0, false)
	if err == nil {
		t.Fatal("ambiguous prefix: want error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "aaaa1111-0000-0000-0000-000000000001") ||
		!strings.Contains(msg, "aaaa1111-ffff-0000-0000-000000000002") {
		t.Errorf("ambiguous error should list the candidates: %v", err)
	}
}

// TestServeSession_Codex: the codex adapter path renders too — the serving
// half is source-agnostic.
func TestServeSession_Codex(t *testing.T) {
	_, codexRoot := newServeHome(t)
	writeCodexSession(t, codexRoot, "cccc3333-0000-0000-0000-000000000003",
		"/home/u/proj-c", time.Now())

	var buf bytes.Buffer
	if err := ServeSession(&buf, "cccc3333", 0, false); err != nil {
		t.Fatalf("ServeSession codex: %v", err)
	}
	if !strings.Contains(buf.String(), "codex hello") {
		t.Errorf("codex render missing message:\n%s", buf.String())
	}
}
