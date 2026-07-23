package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallCodex_WritesEnvelopedScript ties the wiring: the Codex target
// installs the enveloped prime script (hook-JSON), while the Claude target
// installs the plain-text banner. If someone reverts Codex to the shared plain
// script, this fails.
func TestInstallCodex_WritesEnvelopedScript(t *testing.T) {
	codexCfg := t.TempDir()
	if err := installRawclawCodexHook(codexCfg); err != nil {
		t.Fatalf("installRawclawCodexHook: %v", err)
	}
	codexScript, err := os.ReadFile(hookScriptPath(codexCfg))
	if err != nil {
		t.Fatalf("read codex script: %v", err)
	}
	for _, want := range []string{"hookSpecificOutput", "additionalContext", "SessionStart", "python3"} {
		if !strings.Contains(string(codexScript), want) {
			t.Errorf("Codex prime script missing %q (envelope not installed)", want)
		}
	}

	claudeCfg := t.TempDir()
	if err := installRawclawHook(claudeCfg); err != nil {
		t.Fatalf("installRawclawHook: %v", err)
	}
	claudeScript, err := os.ReadFile(hookScriptPath(claudeCfg))
	if err != nil {
		t.Fatalf("read claude script: %v", err)
	}
	if strings.Contains(string(claudeScript), "hookSpecificOutput") {
		t.Error("Claude prime script unexpectedly carries the Codex JSON envelope")
	}
	for name, script := range map[string][]byte{"Claude": claudeScript, "Codex": codexScript} {
		for _, want := range []string{
			"Session closeout: whenever the user signals",
			"background subagent",
			"rawclaw tag-prep <full-session-id>",
			"rawclaw tag-write <full-session-id>",
			"RawClaw has no supersession",
		} {
			if !strings.Contains(string(script), want) {
				t.Errorf("%s SessionStart script missing approved closeout wording %q", name, want)
			}
		}
		for _, forbidden := range []string{"tag-queue", "finished sessions awaiting", "future session"} {
			if strings.Contains(string(script), forbidden) {
				t.Errorf("%s SessionStart script still carries cross-session tagging via %q", name, forbidden)
			}
		}
	}
}

// TestCodexPrimeScript_EmitsValidHookJSON is the Codex-hook regression: run the Codex
// prime script and confirm its stdout is a valid SessionStart hook-JSON object,
// NOT a bare '[rawclaw]…' banner. Codex's output parser (looks_like_json) drops
// hook stdout starting with '[' unless it is valid hook-JSON, so the banner must
// arrive wrapped in hookSpecificOutput/additionalContext.
func TestCodexPrimeScript_EmitsValidHookJSON(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3 (the script itself self-skips without it)")
	}

	// Stub `rawclaw` on PATH so the generated hook's binary-resolution fallback
	// succeeds without depending on this machine's installed binary.
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "rawclaw"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Render the template with an empty baked path so resolution falls through
	// to the PATH stub above — this regression is about the JSON envelope, and
	// exercising the fallback keeps it independent of this machine's own binary.
	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(renderHookScript(rawclawCodexPrimeScript, "''")), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(sh, scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+t.TempDir(),
	)
	cmd.Stdin = strings.NewReader(`{"session_id":"regress-137"}`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run codex prime script: %v (out=%q)", err, out)
	}

	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "{") {
		head := trimmed
		if len(head) > 60 {
			head = head[:60]
		}
		t.Fatalf("Codex hook stdout must start with '{' (valid hook-JSON), else Codex drops it; got: %q", head)
	}

	var env struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v; got %q", err, trimmed)
	}
	if env.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q, want SessionStart", env.HookSpecificOutput.HookEventName)
	}
	// Check distinctive lines spanning the whole banner — first, middle, last —
	// mirroring TestSetupCmd_ScriptContentContainsBanner's multi-line check, so a
	// python3 envelope that dropped or truncated the banner past its first line is
	// caught (a single-line Contains would miss partial loss). Full byte-equality
	// is deliberately NOT asserted: the repo's own banner test rejects duplicating
	// the banner const.
	ctx := env.HookSpecificOutput.AdditionalContext
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
		if !strings.Contains(ctx, want) {
			t.Errorf("additionalContext missing banner line %q; got %q", want, ctx)
		}
	}
	for _, forbidden := range []string{"tag-queue", "finished sessions awaiting", "future session"} {
		if strings.Contains(ctx, forbidden) {
			t.Errorf("additionalContext still carries cross-session tagging via %q; got %q", forbidden, ctx)
		}
	}
}
