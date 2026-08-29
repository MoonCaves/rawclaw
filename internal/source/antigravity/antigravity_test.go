package antigravity

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
			role, text, ok := NormalizeRecord(tt.rec)
			if ok != tt.wantOK {
				t.Fatalf("NormalizeRecord() ok = %v, want %v", ok, tt.wantOK)
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

func TestLookupRejectsChildNamedByParentInvoke(t *testing.T) {
	t.Setenv("ANTIGRAVITY_HOME", t.TempDir())
	root := os.Getenv("ANTIGRAVITY_HOME")
	t.Setenv("HOME", root)
	writeJSONL(t, filepath.Join(root, "history.jsonl"),
		`{"conversationId":"parent","workspace":"/workspace/main"}`,
		`{"conversationId":"child","workspace":"/workspace/main"}`,
	)
	writeJSONL(t, filepath.Join(root, "brain", "parent", ".system_generated", "logs", "transcript.jsonl"),
		`{"type":"INVOKE_SUBAGENT","content":"Created subagent: {\"conversationId\": \"child\"}"}`,
	)
	writeJSONL(t, filepath.Join(root, "brain", "child", ".system_generated", "logs", "transcript.jsonl"),
		`{"type":"USER_INPUT","content":"child work"}`,
	)

	if got, err := lookup("child"); err != nil {
		t.Fatalf("lookup child: %v", err)
	} else if len(got) != 0 {
		t.Fatalf("lookup returned parent-invoked child: %+v", got)
	}
	got, err := lookup("parent")
	if err != nil {
		t.Fatalf("lookup parent: %v", err)
	}
	if len(got) != 1 || got[0].CWD != "/workspace/main" || got[0].ParentID != "" {
		t.Fatalf("parent lookup metadata mismatch: %+v", got)
	}
}

// TestScanSpawnedSubagentsLargeLine verifies Bug 2 fix: a transcript with a JSON line
// larger than the default bufio.Scanner 64KB buffer limit (e.g. 80KB) is not dropped.
func TestScanSpawnedSubagentsLargeLine(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	largePadding := strings.Repeat("x", 80*1024) // 80KB of padding in tool args/content
	parentID := "parent-large-line"
	childID := "sub-child-80k"

	// Parent transcript with >64KB INVOKE_SUBAGENT line
	tParent := filepath.Join(tmp, "brain", parentID, ".system_generated", "logs", "transcript.jsonl")
	invokeLine := fmt.Sprintf(`{"step_index":1,"source":"MODEL","type":"INVOKE_SUBAGENT","created_at":"2026-08-14T10:00:02Z","content":"Created subagent:\n{\n  \"conversationId\": \"%s\"\n}","padding":"%s"}`, childID, largePadding)
	writeJSONL(t, tParent,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:00Z","content":"<USER_REQUEST>large line test</USER_REQUEST>"}`,
		invokeLine,
	)

	// Subagent transcript
	tChild := filepath.Join(tmp, "brain", childID, ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(t, tChild,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:05Z","content":"subagent task"}`,
	)

	// 1. Direct scanSpawnedSubagents test
	spawned := scanSpawnedSubagents(tParent)
	if len(spawned) != 1 || spawned[0] != childID {
		t.Fatalf("scanSpawnedSubagents() = %v, want [%s] (failed to scan >64KB line)", spawned, childID)
	}

	// 2. Full Discover test
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

	child, ok := byID[childID]
	if !ok {
		t.Fatalf("%s not found", childID)
	}
	if !child.IsSubagent {
		t.Errorf("%s IsSubagent = false, want true", childID)
	}
	if child.ParentID != parentID {
		t.Errorf("%s ParentID = %q, want %q", childID, child.ParentID, parentID)
	}
}

// TestExtractCWDFromTranscript verifies Bug 3 fix:
// 1. Decodes JSON content so that escaped newlines within <user_information> blocks parse correctly.
// 2. Scans up to 50 records into the transcript so that CWD appearing beyond the 10th line is discovered.
// 3. Handles tool call Cwd arguments at depth.
func TestExtractCWDFromTranscript(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Session 1: CWD is in <user_information> on record 25 (beyond line 10), with escaped newlines and mapping format.
	sess1Lines := make([]string, 0, 30)
	for i := 0; i < 24; i++ {
		sess1Lines = append(sess1Lines, fmt.Sprintf(`{"step_index":%d,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-14T10:00:00Z","content":"step %d"}`, i, i))
	}
	// Record 24 (the 25th record) contains user_information with escaped newlines
	sess1Lines = append(sess1Lines, `{"step_index":24,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:01:00Z","content":"<user_information>\n/Users/test/workspace/deep-repo -> my-repo\n</user_information>\n<USER_REQUEST>fix issue</USER_REQUEST>"}`)

	t1 := filepath.Join(tmp, "brain", "sess-deep-cwd", ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(t, t1, sess1Lines...)

	// Session 2: CWD is in tool_calls args Cwd on record 20 (beyond line 10).
	sess2Lines := make([]string, 0, 25)
	for i := 0; i < 19; i++ {
		sess2Lines = append(sess2Lines, fmt.Sprintf(`{"step_index":%d,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-14T10:00:00Z","content":"step %d"}`, i, i))
	}
	sess2Lines = append(sess2Lines, `{"step_index":19,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-14T10:01:00Z","content":"running cmd","tool_calls":[{"name":"run_command","args":{"Cwd":"/Users/test/workspace/tool-repo","CommandLine":"git status"}}]}`)

	t2 := filepath.Join(tmp, "brain", "sess-tool-cwd", ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(t, t2, sess2Lines...)

	// Neither session is in history.jsonl
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

	c1, ok := byID["sess-deep-cwd"]
	if !ok {
		t.Fatal("sess-deep-cwd not found")
	}
	if c1.CWD != "/Users/test/workspace/deep-repo" {
		t.Errorf("sess-deep-cwd CWD = %q, want /Users/test/workspace/deep-repo", c1.CWD)
	}

	c2, ok := byID["sess-tool-cwd"]
	if !ok {
		t.Fatal("sess-tool-cwd not found")
	}
	if c2.CWD != "/Users/test/workspace/tool-repo" {
		t.Errorf("sess-tool-cwd CWD = %q, want /Users/test/workspace/tool-repo", c2.CWD)
	}
}

// TestInspectSessionHeaderCWDPrecedence verifies CWD extraction precedence:
//  1. Within a single record: tool_calls Cwd takes precedence over <user_information>.
//  2. Across separate records: the first valid CWD encountered in sequential scan order wins,
//     matching the reference implementation (commit 5885051).
func TestInspectSessionHeaderCWDPrecedence(t *testing.T) {
	t.Parallel()

	t.Run("same record tool_calls takes precedence over user_information", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()

		sessID := "sess-precedence-same"
		tPath := filepath.Join(tmp, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")

		// Single record containing BOTH tool_calls Cwd and <user_information> block with different paths.
		dualRecord := `{"step_index":0,"source":"PLANNER_RESPONSE","type":"PLANNER_RESPONSE","created_at":"2026-08-14T10:00:00Z","content":"<user_information>\n/Users/test/workspace/user-info-repo -> repo\n</user_information>","tool_calls":[{"name":"run_command","args":{"Cwd":"/Users/test/workspace/tool-call-repo","CommandLine":"git status"}}]}`
		writeJSONL(t, tPath, dualRecord)

		hdr, _ := inspectSessionHeaderAndSubagents(tPath)
		if hdr.cwd != "/Users/test/workspace/tool-call-repo" {
			t.Errorf("hdr.cwd = %q, want /Users/test/workspace/tool-call-repo (tool_calls Cwd must take precedence over user_information in same record)", hdr.cwd)
		}

		ad := NewRoot(tmp)
		containers, err := ad.Discover()
		if err != nil {
			t.Fatalf("Discover() error: %v", err)
		}
		if len(containers) != 1 {
			t.Fatalf("Discover() returned %d containers, want 1", len(containers))
		}
		if containers[0].CWD != "/Users/test/workspace/tool-call-repo" {
			t.Errorf("containers[0].CWD = %q, want /Users/test/workspace/tool-call-repo", containers[0].CWD)
		}
	})

	t.Run("separate records user_information first wins over subsequent tool_calls", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()

		sessID := "sess-precedence-user-first"
		tPath := filepath.Join(tmp, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")

		// Record 1: user_information with path A
		rec1 := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:00Z","content":"<user_information>\n/Users/test/workspace/user-info-repo -> repo\n</user_information>\n<USER_REQUEST>run task</USER_REQUEST>"}`
		// Record 2: tool_calls Cwd with path B
		rec2 := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-14T10:00:01Z","content":"running cmd","tool_calls":[{"name":"run_command","args":{"Cwd":"/Users/test/workspace/tool-call-repo","CommandLine":"git status"}}]}`
		writeJSONL(t, tPath, rec1, rec2)

		hdr, _ := inspectSessionHeaderAndSubagents(tPath)
		if hdr.cwd != "/Users/test/workspace/user-info-repo" {
			t.Errorf("hdr.cwd = %q, want /Users/test/workspace/user-info-repo (first valid CWD found across records must win)", hdr.cwd)
		}

		ad := NewRoot(tmp)
		containers, err := ad.Discover()
		if err != nil {
			t.Fatalf("Discover() error: %v", err)
		}
		if len(containers) != 1 {
			t.Fatalf("Discover() returned %d containers, want 1", len(containers))
		}
		if containers[0].CWD != "/Users/test/workspace/user-info-repo" {
			t.Errorf("containers[0].CWD = %q, want /Users/test/workspace/user-info-repo", containers[0].CWD)
		}
	})

	t.Run("separate records tool_calls first wins over subsequent user_information", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()

		sessID := "sess-precedence-tool-first"
		tPath := filepath.Join(tmp, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")

		// Record 1: tool_calls Cwd with path B
		rec1 := `{"step_index":0,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-14T10:00:00Z","content":"running cmd","tool_calls":[{"name":"run_command","args":{"Cwd":"/Users/test/workspace/tool-call-repo","CommandLine":"git status"}}]}`
		// Record 2: user_information with path A
		rec2 := `{"step_index":1,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:01Z","content":"<user_information>\n/Users/test/workspace/user-info-repo -> repo\n</user_information>\n<USER_REQUEST>run task</USER_REQUEST>"}`
		writeJSONL(t, tPath, rec1, rec2)

		hdr, _ := inspectSessionHeaderAndSubagents(tPath)
		if hdr.cwd != "/Users/test/workspace/tool-call-repo" {
			t.Errorf("hdr.cwd = %q, want /Users/test/workspace/tool-call-repo (first valid CWD found across records must win)", hdr.cwd)
		}

		ad := NewRoot(tmp)
		containers, err := ad.Discover()
		if err != nil {
			t.Fatalf("Discover() error: %v", err)
		}
		if len(containers) != 1 {
			t.Fatalf("Discover() returned %d containers, want 1", len(containers))
		}
		if containers[0].CWD != "/Users/test/workspace/tool-call-repo" {
			t.Errorf("containers[0].CWD = %q, want /Users/test/workspace/tool-call-repo", containers[0].CWD)
		}
	})
}

// TestScanTranscriptScannerError verifies that when a transcript contains a line
// exceeding the scanner buffer limit (1MB), scanner.Err() is checked and a structured
// warning is logged with the file path and error details.
func TestScanTranscriptScannerError(t *testing.T) {
	tmp := t.TempDir()

	sessID := "sess-oversized-line"
	tPath := filepath.Join(tmp, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")

	// Create a single line exceeding the 1MB (1024*1024) scanner buffer
	oversizedLine := fmt.Sprintf(`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-14T10:00:00Z","content":"%s"}`, strings.Repeat("x", 1024*1024+1024))
	writeJSONL(t, tPath, oversizedLine)

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(orig)

	inspectSessionHeaderAndSubagents(tPath)

	out := buf.String()
	if !strings.Contains(out, "antigravity: scan transcript error") {
		t.Fatalf("expected warning log missing from output: %s", out)
	}
	if !strings.Contains(out, tPath) {
		t.Errorf("expected warning to contain path %q, got: %s", tPath, out)
	}
	if !strings.Contains(out, "token too long") {
		t.Errorf("expected warning to contain error token too long, got: %s", out)
	}
}
