package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInstallAntigravity_WritesInjectStepsScript verifies that the Antigravity
// target installs the pre-marshaled injectSteps discovery script, while Claude
// installs the plain text banner and Codex installs the hook-JSON envelope.
func TestInstallAntigravity_WritesInjectStepsScript(t *testing.T) {
	agCfg := t.TempDir()
	if err := installRawclawAntigravityHook(agCfg); err != nil {
		t.Fatalf("installRawclawAntigravityHook: %v", err)
	}
	agScript, err := os.ReadFile(hookScriptPath(agCfg))
	if err != nil {
		t.Fatalf("read antigravity script: %v", err)
	}
	agContent := string(agScript)
	for _, want := range []string{"injectSteps", "ephemeralMessage", "invocationNum"} {
		if !strings.Contains(agContent, want) {
			t.Errorf("Antigravity prime script missing %q", want)
		}
	}
	// Verify zero runtime dependencies (no python3 / jq in the script body)
	if strings.Contains(agContent, "python3") {
		t.Error("Antigravity script unexpectedly depends on python3")
	}

	claudeCfg := t.TempDir()
	if err := installRawclawHook(claudeCfg); err != nil {
		t.Fatalf("installRawclawHook: %v", err)
	}
	claudeScript, err := os.ReadFile(hookScriptPath(claudeCfg))
	if err != nil {
		t.Fatalf("read claude script: %v", err)
	}
	if strings.Contains(string(claudeScript), "injectSteps") {
		t.Error("Claude prime script unexpectedly carries the Antigravity injectSteps envelope")
	}

	for _, want := range []string{
		"Session closeout: whenever the user signals",
		"background subagent",
		"rawclaw tag-prep <full-session-id>",
		"rawclaw tag-write <full-session-id>",
		"RawClaw has no supersession",
	} {
		if !strings.Contains(agContent, want) {
			t.Errorf("Antigravity script missing approved closeout wording %q", want)
		}
	}
}

func TestAntigravityStopScript_DispatchesPrewarmDetached(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	root := t.TempDir()
	marker := filepath.Join(root, "args")
	stubDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\nprintf '%s %s' \"$1\" \"$2\" > \"" + marker + "\"\n"
	if err := os.WriteFile(filepath.Join(stubDir, "rawclaw"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	rendered, err := renderAntigravityPrimeScript("''")
	if err != nil {
		t.Fatalf("renderAntigravityPrimeScript: %v", err)
	}
	scriptPath := filepath.Join(root, "stop.sh")
	if err := os.WriteFile(scriptPath, []byte(rendered), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(sh, scriptPath)
	cmd.Env = append(os.Environ(), "PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = strings.NewReader(`{"conversationId":"test-conversation","terminationReason":"model_stop"}`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run stop hook: %v (out=%q)", err, out)
	}
	// A 1s ceiling was too tight under -race plus full-suite concurrent package
	// load: clean 10/10 in isolation, but flaky when many other packages'
	// tests compete for CPU. This only waits for a detached background
	// process to appear, not asserting a specific latency, so a longer
	// ceiling costs nothing in the common (fast) case.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := os.ReadFile(marker); err == nil {
			if string(got) != "prewarm test-conversation" {
				t.Fatalf("rawclaw args = %q, want prewarm test-conversation", got)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("detached prewarm command did not run")
}

// TestAntigravityPrimeScript_InvocationNum0 verifies that when invocationNum is 0,
// the hook script outputs valid injectSteps JSON with the discovery banner.
func TestAntigravityPrimeScript_InvocationNum0(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}

	// Stub `rawclaw` on PATH so fallback succeeds
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "rawclaw"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	rendered, err := renderAntigravityPrimeScript("''")
	if err != nil {
		t.Fatalf("renderAntigravityPrimeScript: %v", err)
	}
	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(rendered), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(sh, scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+t.TempDir(),
	)
	cmd.Stdin = strings.NewReader(`{"conversationId":"test-conv-001","invocationNum":0,"initialNumSteps":1}`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run antigravity prime script: %v (out=%q)", err, out)
	}

	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "{") {
		t.Fatalf("stdout must start with '{' (valid JSON); got: %q", trimmed)
	}

	var env struct {
		InjectSteps []struct {
			EphemeralMessage string `json:"ephemeralMessage"`
		} `json:"injectSteps"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("stdout is not valid injectSteps JSON: %v; got: %q", err, trimmed)
	}
	if len(env.InjectSteps) != 1 {
		t.Fatalf("len(InjectSteps) = %d, want 1", len(env.InjectSteps))
	}

	msg := env.InjectSteps[0].EphemeralMessage
	for _, want := range []string{
		"[rawclaw] Raw transcript history",
		"Fast FTS5/BM25 search",
		`rawclaw "query"`,
		"offering to resume/fork it can help",
		"Session closeout: whenever the user signals",
		"background subagent",
		"rawclaw tag-prep <full-session-id>",
		"rawclaw tag-write <full-session-id>",
		"RawClaw has no supersession",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("ephemeralMessage missing banner line %q; got: %q", want, msg)
		}
	}
}

// TestAntigravityPrimeScript_InvocationNum1_ExitsQuietly verifies that subsequent
// turns (invocationNum > 0) exit 0 with empty stdout (no repeated banner injection).
func TestAntigravityPrimeScript_InvocationNum1_ExitsQuietly(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}

	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "rawclaw"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	rendered, err := renderAntigravityPrimeScript("''")
	if err != nil {
		t.Fatalf("renderAntigravityPrimeScript: %v", err)
	}
	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(rendered), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, invNum := range []int{1, 2, 5} {
		cmd := exec.Command(sh, scriptPath)
		cmd.Env = append(os.Environ(),
			"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
			"TMPDIR="+t.TempDir(),
		)
		cmd.Stdin = strings.NewReader(`{"conversationId":"test-conv-001","invocationNum":` + string(rune('0'+invNum)) + `}`)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("invocationNum=%d returned error: %v (out=%q)", invNum, err, out)
		}
		if len(strings.TrimSpace(string(out))) != 0 {
			t.Errorf("invocationNum=%d produced non-empty stdout: %q, want empty", invNum, out)
		}
	}
}

// TestAntigravityPrimeScript_Contract_RecordedPayload verifies the exact stdin/stdout contract:
// 1. First invocation (invocationNum 0) with recorded AGY payload emits byte-valid injectSteps JSON.
// 2. Second invocation (invocationNum 1) with recorded AGY payload emits empty stdout (exit 0).
// 3. Garbage / unparseable payload degrades gracefully to emitting the banner (exit 0, never fails).
func TestAntigravityPrimeScript_Contract_RecordedPayload(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}

	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "rawclaw"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	rendered, err := renderAntigravityPrimeScript("''")
	if err != nil {
		t.Fatalf("renderAntigravityPrimeScript: %v", err)
	}
	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(rendered), 0o755); err != nil {
		t.Fatal(err)
	}

	runScriptWithStdin := func(stdinContent string) (string, error) {
		cmd := exec.Command(sh, scriptPath)
		cmd.Env = append(os.Environ(),
			"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
			"TMPDIR="+t.TempDir(),
		)
		cmd.Stdin = strings.NewReader(stdinContent)
		out, runErr := cmd.Output()
		return string(out), runErr
	}

	// 1. Recorded Turn 1 payload (live probe receipt from subagent 11111111-1111-1111-1111-111111111111)
	recordedTurn1 := `{
  "artifactDirectoryPath": "/home/user/.gemini/antigravity-cli/brain/11111111-1111-1111-1111-111111111111",
  "conversationId": "11111111-1111-1111-1111-111111111111",
  "initialNumSteps": 1,
  "invocationNum": 0,
  "modelName": "gemini-3.7-flash-tiered",
  "transcriptPath": "/home/user/.gemini/antigravity-cli/brain/11111111-1111-1111-1111-111111111111/.system_generated/logs/transcript_full.jsonl",
  "workspacePaths": ["/home/user/project"]
}`
	out1, err := runScriptWithStdin(recordedTurn1)
	if err != nil {
		t.Fatalf("turn 1 with recorded payload failed: %v", err)
	}
	trimmed1 := strings.TrimSpace(out1)
	if !strings.HasPrefix(trimmed1, "{") {
		t.Fatalf("turn 1 stdout must start with '{', got: %q", trimmed1)
	}

	var env struct {
		InjectSteps []struct {
			EphemeralMessage string `json:"ephemeralMessage"`
		} `json:"injectSteps"`
	}
	if err := json.Unmarshal([]byte(trimmed1), &env); err != nil {
		t.Fatalf("turn 1 output is not valid JSON: %v (raw=%q)", err, trimmed1)
	}
	if len(env.InjectSteps) != 1 {
		t.Fatalf("len(InjectSteps) = %d, want 1", len(env.InjectSteps))
	}
	if !strings.Contains(env.InjectSteps[0].EphemeralMessage, "[rawclaw] Raw transcript history") {
		t.Errorf("ephemeralMessage missing rawclaw banner header: %q", env.InjectSteps[0].EphemeralMessage)
	}

	// 2. Recorded Turn 2 payload (invocationNum 1)
	recordedTurn2 := `{
  "artifactDirectoryPath": "/home/user/.gemini/antigravity-cli/brain/11111111-1111-1111-1111-111111111111",
  "conversationId": "11111111-1111-1111-1111-111111111111",
  "initialNumSteps": 4,
  "invocationNum": 1,
  "modelName": "gemini-3.7-flash-tiered",
  "transcriptPath": "/home/user/.gemini/antigravity-cli/brain/11111111-1111-1111-1111-111111111111/.system_generated/logs/transcript_full.jsonl",
  "workspacePaths": ["/home/user/project"]
}`
	out2, err := runScriptWithStdin(recordedTurn2)
	if err != nil {
		t.Fatalf("turn 2 with recorded payload failed: %v", err)
	}
	if len(strings.TrimSpace(out2)) != 0 {
		t.Errorf("turn 2 produced stdout %q, want empty (once-per-conversation)", out2)
	}

	// 3. Graceful fallback on garbage payloads (never fail hook, exit 0, emit banner)
	garbageCases := []struct {
		name  string
		stdin string
	}{
		{"empty stdin", ""},
		{"malformed json", `{"invocationNum":`},
		{"non-json plain text", "not a json object"},
		{"missing invocationNum", `{"conversationId": "xyz", "steps": 10}`},
		{"string invocationNum", `{"invocationNum": "zero"}`},
	}
	for _, tc := range garbageCases {
		t.Run(tc.name, func(t *testing.T) {
			gOut, gErr := runScriptWithStdin(tc.stdin)
			if gErr != nil {
				t.Fatalf("garbage case %q failed with error: %v", tc.name, gErr)
			}
			gTrimmed := strings.TrimSpace(gOut)
			if !strings.HasPrefix(gTrimmed, "{") {
				t.Fatalf("garbage case %q should fall back to emitting banner, got: %q", tc.name, gTrimmed)
			}
		})
	}
}
