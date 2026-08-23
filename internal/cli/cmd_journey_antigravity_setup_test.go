package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestJourney_AntigravitySetupHermeticEndToEnd tests the complete black-box lifecycle
// of `rawclaw setup` and `rawclaw setup --eject` for Antigravity in a strictly
// isolated environment where HOME and ANTIGRAVITY_HOME point to a temporary directory.
// It verifies:
// 1. Live ~/.gemini is unreachable and untouched.
// 2. Pre-existing sibling hooks (other-tool) survive install untouched.
// 3. The installed hook script is executable (0755) and emits valid injectSteps JSON on invocationNum 0.
// 4. Subsequent turns (invocationNum > 0) are suppressed (silent exit 0).
// 5. Eject removes the hook script and rawclaw config while preserving sibling entries.
func TestJourney_AntigravitySetupHermeticEndToEnd(t *testing.T) {
	sandbox := t.TempDir()
	claudeCfg := filepath.Join(sandbox, ".claude")
	agHome := filepath.Join(sandbox, ".gemini", "antigravity-cli")
	agConfigDir := filepath.Join(sandbox, ".gemini", "config")
	agHooksFile := filepath.Join(agConfigDir, "hooks.json")

	t.Setenv("HOME", sandbox)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeCfg)
	t.Setenv("ANTIGRAVITY_HOME", agHome)
	t.Setenv("XDG_DATA_HOME", filepath.Join(sandbox, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(sandbox, ".cache"))

	// Create Antigravity home directory so detection recognizes it
	if err := os.MkdirAll(agHome, 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed pre-existing sibling hook (other-tool) in hooks.json
	if err := os.MkdirAll(agConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seededSibling := `{
  "other-tool": {
    "PreInvocation": [
      {"type": "command", "command": "/opt/other-tool/hooks/status.sh", "timeout": 10}
    ],
    "Stop": [
      {"type": "command", "command": "/opt/other-tool/hooks/stop.sh", "timeout": 10}
    ]
  }
}`
	if err := os.WriteFile(agHooksFile, []byte(seededSibling), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Run `rawclaw setup --yes`
	setupOut, err := runCmd(t, newSetupCmd(), "", "--yes")
	if err != nil {
		t.Fatalf("setup --yes failed: %v\nout:\n%s", err, setupOut)
	}

	// Verify setup output announces Antigravity installation
	if !strings.Contains(setupOut, "Registered PreInvocation hook in") {
		t.Errorf("setup output missing Antigravity registration note; got:\n%s", setupOut)
	}

	// 2. Verify installed hook script
	scriptPath := hookScriptPath(agConfigDir)
	info, serr := os.Stat(scriptPath)
	if serr != nil {
		t.Fatalf("installed hook script not found at %s: %v", scriptPath, serr)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("installed hook script %s is not executable, mode=%v", scriptPath, info.Mode())
	}

	// 3. Verify hooks.json content
	agData, rerr := readJSONFile(agHooksFile)
	if rerr != nil {
		t.Fatalf("read hooks.json: %v", rerr)
	}
	if _, ok := agData["other-tool"]; !ok {
		t.Errorf("other-tool was corrupted or removed from hooks.json: %#v", agData)
	}
	rawclawGroup, ok := agData["rawclaw"].(map[string]any)
	if !ok {
		t.Fatalf("rawclaw group missing from hooks.json: %#v", agData)
	}
	preInvocations, ok := rawclawGroup["PreInvocation"].([]any)
	if !ok || len(preInvocations) != 1 {
		t.Fatalf("rawclaw.PreInvocation = %#v, want 1 entry", rawclawGroup["PreInvocation"])
	}
	cmdEntry, ok := preInvocations[0].(map[string]any)
	if !ok || cmdEntry["command"] != scriptPath {
		t.Errorf("rawclaw command = %v, want %s", cmdEntry["command"], scriptPath)
	}

	// 4. Execute the installed script on invocationNum 0 -> assert valid injectSteps JSON
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available for execution test")
	}

	cmdTurn1 := exec.Command(sh, scriptPath)
	cmdTurn1.Env = append(os.Environ(), "TMPDIR="+sandbox)
	cmdTurn1.Stdin = strings.NewReader(`{
		"conversationId": "journey-test-conv-001",
		"invocationNum": 0,
		"initialNumSteps": 1,
		"modelName": "gemini-3.7-flash",
		"workspacePaths": ["` + sandbox + `"]
	}`)
	outTurn1, err := cmdTurn1.Output()
	if err != nil {
		t.Fatalf("execute hook script on turn 1 failed: %v (out=%q)", err, outTurn1)
	}

	var injectEnvelope struct {
		InjectSteps []struct {
			EphemeralMessage string `json:"ephemeralMessage"`
		} `json:"injectSteps"`
	}
	if err := json.Unmarshal(outTurn1, &injectEnvelope); err != nil {
		t.Fatalf("turn 1 stdout is not valid injectSteps JSON: %v; got: %q", err, string(outTurn1))
	}
	if len(injectEnvelope.InjectSteps) != 1 {
		t.Fatalf("len(injectSteps) = %d, want 1", len(injectEnvelope.InjectSteps))
	}
	banner := injectEnvelope.InjectSteps[0].EphemeralMessage
	if !strings.Contains(banner, "[rawclaw] Raw transcript history") {
		t.Errorf("injected ephemeralMessage missing header banner: %q", banner)
	}
	if !strings.Contains(banner, "Session closeout:") {
		t.Errorf("injected ephemeralMessage missing session closeout instructions: %q", banner)
	}

	// 5. Execute on invocationNum 1 -> assert silent exit 0 (empty stdout)
	cmdTurn2 := exec.Command(sh, scriptPath)
	cmdTurn2.Env = append(os.Environ(), "TMPDIR="+sandbox)
	cmdTurn2.Stdin = strings.NewReader(`{
		"conversationId": "journey-test-conv-001",
		"invocationNum": 1,
		"initialNumSteps": 4
	}`)
	outTurn2, err := cmdTurn2.Output()
	if err != nil {
		t.Fatalf("execute hook script on turn 2 failed: %v (out=%q)", err, outTurn2)
	}
	if len(strings.TrimSpace(string(outTurn2))) != 0 {
		t.Errorf("turn 2 produced stdout %q, want empty (once-per-conversation)", string(outTurn2))
	}

	// 6. Run `rawclaw setup --eject --yes`
	ejectOut, err := runCmd(t, newSetupCmd(), "", "--eject", "--yes")
	if err != nil {
		t.Fatalf("setup --eject --yes failed: %v\nout:\n%s", err, ejectOut)
	}

	// 7. Verify script removed, rawclaw entry removed, other-tool survived
	if _, serr := os.Stat(scriptPath); !os.IsNotExist(serr) {
		t.Errorf("hook script %s should be removed after eject, stat err=%v", scriptPath, serr)
	}
	agDataAfterEject, err := readJSONFile(agHooksFile)
	if err != nil {
		t.Fatalf("read hooks.json after eject: %v", err)
	}
	if _, ok := agDataAfterEject["rawclaw"]; ok {
		t.Errorf("rawclaw group should be removed after eject: %#v", agDataAfterEject)
	}
	if _, ok := agDataAfterEject["other-tool"]; !ok {
		t.Errorf("other-tool was lost after eject: %#v", agDataAfterEject)
	}
}
