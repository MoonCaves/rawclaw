package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// assertForeignEntryIntact structurally compares the foreign SessionStart
// entry in the live config file (configFile — either target's settingsPath or
// codexHooksPath) against the entry originally seeded. Whole-file byte
// identity is impossible by design (the merge engine re-marshals the JSON),
// so the spec's "unchanged" guarantee is asserted at the entry level:
// deep-equal structure and values.
func assertForeignEntryIntact(t *testing.T, seeded []byte, configFile string) {
	t.Helper()
	var want map[string]any
	if err := json.Unmarshal(seeded, &want); err != nil {
		t.Fatalf("unmarshal seeded config: %v", err)
	}
	wantEntry := want["hooks"].(map[string]any)["SessionStart"].([]any)[0]

	data, err := readJSONFile(configFile)
	if err != nil {
		t.Fatalf("read config for foreign-entry check: %v", err)
	}
	arr := data["hooks"].(map[string]any)["SessionStart"].([]any)
	for _, e := range arr {
		if !containsRawclaw(e) {
			if !reflect.DeepEqual(e, wantEntry) {
				t.Errorf("foreign entry mutated:\n got %#v\nwant %#v", e, wantEntry)
			}
			return
		}
	}
	t.Errorf("foreign entry missing entirely: %#v", arr)
}

// seedForeignSessionStartHook pre-seeds configFile (either target's
// settingsPath or codexHooksPath) with a single foreign (non-rawclaw)
// SessionStart hook entry, mirroring the sibling-tool fixture the spec's
// testing decisions call for (othertool/agent-monitor/cmux — something already
// hooked into the same event before rawclaw ever runs). Returns the exact
// bytes written, so a later read can assert byte-identity.
func seedForeignSessionStartHook(t *testing.T, configFile string) []byte {
	t.Helper()
	seed := `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {"type": "command", "command": "/opt/foreign-tool/hooks/session.sh"}
        ]
      }
    ]
  }
}
`
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configFile, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed %s: %v", configFile, err)
	}
	return []byte(seed)
}

// TestSetupCmd_Yes_AddsHookAndKeepsForeignEntry drives `rawclaw setup --yes`
// end to end against a sandboxed HOME pre-seeded with a foreign SessionStart
// hook: exactly one rawclaw entry must be added, the hook script must exist on
// disk with the expected content, and the foreign entry must come through
// byte-for-byte (proven by re-reading and comparing raw file bytes for the
// foreign command string, not just a loose "still present" check).
func TestSetupCmd_Yes_AddsHookAndKeepsForeignEntry(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)
	seeded := seedForeignSessionStartHook(t, settingsPath(cfg))

	out, err := runCmd(t, newSetupCmd(), "", "--yes")
	if err != nil {
		t.Fatalf("setup --yes: %v\nout: %s", err, out)
	}

	data, rerr := readJSONFile(settingsPath(cfg))
	if rerr != nil {
		t.Fatalf("read settings after setup: %v", rerr)
	}
	hooks := data["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)

	var rawclawCount, foreignCount int
	for _, e := range arr {
		if containsRawclaw(e) {
			rawclawCount++
		} else {
			foreignCount++
		}
	}
	if rawclawCount != 1 {
		t.Errorf("want exactly 1 rawclaw SessionStart entry, got %d: %#v", rawclawCount, arr)
	}
	if foreignCount != 1 {
		t.Errorf("want the foreign entry preserved, got %d foreign entries: %#v", foreignCount, arr)
	}
	assertForeignEntryIntact(t, seeded, settingsPath(cfg))

	scriptPath := hookScriptPath(cfg)
	b, serr := os.ReadFile(scriptPath)
	if serr != nil {
		t.Fatalf("hook script not on disk at %s: %v", scriptPath, serr)
	}
	if !strings.Contains(string(b), "rawclaw") {
		t.Errorf("script content missing rawclaw banner content: %s", b)
	}
}

// TestSetupCmd_Yes_IdempotentSecondRun: running `setup --yes` twice must leave
// exactly one rawclaw entry (no duplicates) and the foreign entry untouched.
func TestSetupCmd_Yes_IdempotentSecondRun(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)
	seeded := seedForeignSessionStartHook(t, settingsPath(cfg))

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("first setup --yes: %v", err)
	}
	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("second setup --yes: %v", err)
	}

	data, rerr := readJSONFile(settingsPath(cfg))
	if rerr != nil {
		t.Fatalf("read settings after two runs: %v", rerr)
	}
	hooks := data["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)

	var rawclawCount, foreignCount int
	for _, e := range arr {
		if containsRawclaw(e) {
			rawclawCount++
		} else {
			foreignCount++
		}
	}
	if rawclawCount != 1 {
		t.Errorf("want exactly 1 rawclaw entry after second run, got %d: %#v", rawclawCount, arr)
	}
	if foreignCount != 1 {
		t.Errorf("want the foreign entry still present after second run, got %d: %#v", foreignCount, arr)
	}
	assertForeignEntryIntact(t, seeded, settingsPath(cfg))
}

// TestSetupCmd_InteractiveYes_AddsHook: without --yes, a 'y' on stdin
// confirms the write.
func TestSetupCmd_InteractiveYes_AddsHook(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)

	out, err := runCmd(t, newSetupCmd(), "y\n")
	if err != nil {
		t.Fatalf("interactive setup: %v\nout: %s", err, out)
	}

	if _, serr := os.Stat(hookScriptPath(cfg)); serr != nil {
		t.Errorf("hook script missing after interactive 'y': %v", serr)
	}
	data, rerr := readJSONFile(settingsPath(cfg))
	if rerr != nil {
		t.Fatalf("read settings: %v", rerr)
	}
	if _, ok := data["hooks"]; !ok {
		t.Errorf("settings.json has no hooks after interactive 'y': %#v", data)
	}
}

// TestSetupCmd_InteractiveNo_WritesNothing: without --yes, a 'n' (or non-tty
// EOF) on stdin must write nothing to disk at all — no settings.json, no
// script, no config dir left half-created.
func TestSetupCmd_InteractiveNo_WritesNothing(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)

	out, err := runCmd(t, newSetupCmd(), "n\n")
	if err != nil {
		t.Fatalf("interactive setup 'n': %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("want an abort message, out: %s", out)
	}

	if _, serr := os.Stat(settingsPath(cfg)); !os.IsNotExist(serr) {
		t.Errorf("settings.json should not exist after declining, stat err=%v", serr)
	}
	if _, serr := os.Stat(hookScriptPath(cfg)); !os.IsNotExist(serr) {
		t.Errorf("hook script should not exist after declining, stat err=%v", serr)
	}
}

// TestSetupCmd_HonorsClaudeConfigDir: the global target resolves against
// CLAUDE_CONFIG_DIR when set, not a hardcoded ~/.claude — proving `setup`
// reuses the same env-honoring resolution as the rest of rawclaw rather than
// hand-rolling its own path.
func TestSetupCmd_HonorsClaudeConfigDir(t *testing.T) {
	realHome := t.TempDir() // must never be touched
	cfg := t.TempDir()      // the CLAUDE_CONFIG_DIR target
	t.Setenv("HOME", realHome)
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("setup --yes: %v", err)
	}

	if _, serr := os.Stat(settingsPath(cfg)); serr != nil {
		t.Errorf("settings.json missing under CLAUDE_CONFIG_DIR: %v", serr)
	}
	if _, serr := os.Stat(filepath.Join(realHome, ".claude")); !os.IsNotExist(serr) {
		t.Errorf("setup wrote under real HOME/.claude despite CLAUDE_CONFIG_DIR override: err=%v", serr)
	}
}

// TestSetupCmd_Yes_WiresCodexAndKeepsForeignEntry drives `rawclaw setup --yes`
// against a sandbox with BOTH targets present — a Claude Code config dir and
// an existing Codex config dir (CODEX_HOME), each pre-seeded with a foreign
// SessionStart entry. Both targets must end up with exactly one rawclaw entry
// plus the foreign entry intact, and neither target's config file gets a
// `features` key written — setup never touches Codex feature flags.
func TestSetupCmd_Yes_WiresCodexAndKeepsForeignEntry(t *testing.T) {
	claudeCfg := t.TempDir()
	codexCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeCfg)
	t.Setenv("CODEX_HOME", codexCfg)
	t.Setenv("HOME", claudeCfg)

	claudeSeeded := seedForeignSessionStartHook(t, settingsPath(claudeCfg))
	codexSeeded := seedForeignSessionStartHook(t, codexHooksPath(codexCfg))

	out, err := runCmd(t, newSetupCmd(), "", "--yes")
	if err != nil {
		t.Fatalf("setup --yes: %v\nout: %s", err, out)
	}

	// Claude side: unaffected by Codex also being wired.
	assertForeignEntryIntact(t, claudeSeeded, settingsPath(claudeCfg))

	// Codex side: script on disk, exactly one rawclaw entry, foreign entry intact.
	codexScript := hookScriptPath(codexCfg)
	if _, serr := os.Stat(codexScript); serr != nil {
		t.Fatalf("codex hook script not on disk at %s: %v", codexScript, serr)
	}

	data, rerr := readJSONFile(codexHooksPath(codexCfg))
	if rerr != nil {
		t.Fatalf("read hooks.json after setup: %v", rerr)
	}
	if _, hasFeatures := data["features"]; hasFeatures {
		t.Errorf("setup must not write a Codex [features] flag, got %#v", data["features"])
	}
	hooks := data["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)

	var rawclawCount, foreignCount int
	for _, e := range arr {
		if containsRawclaw(e) {
			rawclawCount++
		} else {
			foreignCount++
		}
	}
	if rawclawCount != 1 {
		t.Errorf("want exactly 1 rawclaw SessionStart entry in hooks.json, got %d: %#v", rawclawCount, arr)
	}
	if foreignCount != 1 {
		t.Errorf("want the foreign hooks.json entry preserved, got %d: %#v", foreignCount, arr)
	}
	assertForeignEntryIntact(t, codexSeeded, codexHooksPath(codexCfg))
}

// TestSetupCmd_Yes_CodexIdempotentSecondRun: running `setup --yes` twice with
// Codex detected must leave exactly one rawclaw entry in hooks.json (no
// duplicates) and the foreign entry untouched — same idempotency guarantee as
// the Claude Code target.
func TestSetupCmd_Yes_CodexIdempotentSecondRun(t *testing.T) {
	claudeCfg := t.TempDir()
	codexCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeCfg)
	t.Setenv("CODEX_HOME", codexCfg)
	t.Setenv("HOME", claudeCfg)

	codexSeeded := seedForeignSessionStartHook(t, codexHooksPath(codexCfg))

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("first setup --yes: %v", err)
	}
	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("second setup --yes: %v", err)
	}

	data, rerr := readJSONFile(codexHooksPath(codexCfg))
	if rerr != nil {
		t.Fatalf("read hooks.json after two runs: %v", rerr)
	}
	hooks := data["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)

	var rawclawCount, foreignCount int
	for _, e := range arr {
		if containsRawclaw(e) {
			rawclawCount++
		} else {
			foreignCount++
		}
	}
	if rawclawCount != 1 {
		t.Errorf("want exactly 1 rawclaw entry in hooks.json after second run, got %d: %#v", rawclawCount, arr)
	}
	if foreignCount != 1 {
		t.Errorf("want the foreign hooks.json entry still present after second run, got %d: %#v", foreignCount, arr)
	}
	assertForeignEntryIntact(t, codexSeeded, codexHooksPath(codexCfg))
}

// TestSetupCmd_Yes_CodexNotDetected_SkipsWithoutCreatingTree: when
// CODEX_HOME points at a directory that does not exist, `setup --yes` must
// skip the Codex target cleanly — print a note, write nothing under it, and
// never create the directory — while still wiring Claude Code normally.
// Mirrors othertool's detect-then-offer shape as detect-then-skip for --yes.
func TestSetupCmd_Yes_CodexNotDetected_SkipsWithoutCreatingTree(t *testing.T) {
	claudeCfg := t.TempDir()
	missingCodexDir := filepath.Join(t.TempDir(), "nonexistent-codex-home")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeCfg)
	t.Setenv("CODEX_HOME", missingCodexDir)
	t.Setenv("HOME", claudeCfg)

	out, err := runCmd(t, newSetupCmd(), "", "--yes")
	if err != nil {
		t.Fatalf("setup --yes: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "not detected") {
		t.Errorf("want a 'not detected' note for the missing Codex target, out: %s", out)
	}

	if _, serr := os.Stat(missingCodexDir); !os.IsNotExist(serr) {
		t.Errorf("setup must not create a Codex tree for a user who has none, stat err=%v", serr)
	}

	// Claude Code side must still be wired normally despite the Codex skip.
	if _, serr := os.Stat(settingsPath(claudeCfg)); serr != nil {
		t.Errorf("Claude settings.json missing despite Codex skip: %v", serr)
	}
	if _, serr := os.Stat(hookScriptPath(claudeCfg)); serr != nil {
		t.Errorf("Claude hook script missing despite Codex skip: %v", serr)
	}
}

// TestSetupCmd_ScriptContentContainsBanner: the installed script carries the
// spec's finalized banner text verbatim (a subset of distinctive lines, since
// asserting the byte-for-byte totality here would just duplicate the
// rawclawPrimeScript constant definition itself).
func TestSetupCmd_ScriptContentContainsBanner(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("setup --yes: %v", err)
	}

	b, err := os.ReadFile(hookScriptPath(cfg))
	if err != nil {
		t.Fatalf("read installed script: %v", err)
	}
	content := string(b)

	wantLines := []string{
		"#!/bin/sh",
		`command -v rawclaw >/dev/null 2>&1 || exit 0`,
		"Raw transcript history for context",
		"Fast FTS5/BM25 search",
		"Memory providers",
		`rawclaw "query"`,
		"rawclaw read <ref>",
		"rawclaw outline <sess8>",
		"--json for structured output",
		"offering to resume/fork it can help",
	}
	for _, want := range wantLines {
		if !strings.Contains(content, want) {
			t.Errorf("installed script missing expected line %q", want)
		}
	}
}

// TestSetupCmd_Project_WritesToProjectLocalPathAndKeepsForeignEntry drives
// `rawclaw setup --project --yes` from a sandboxed cwd pre-seeded with a
// foreign SessionStart hook in the project-local settings file: the rawclaw
// entry must land at ./.claude/settings.json (not the global
// CLAUDE_CONFIG_DIR path), with the same one-entry idempotency and
// sibling-survival guarantees as the global default.
func TestSetupCmd_Project_WritesToProjectLocalPathAndKeepsForeignEntry(t *testing.T) {
	globalCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", globalCfg)
	t.Setenv("HOME", globalCfg)

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	projectClaudeDir := filepath.Join(projectDir, ".claude")
	seeded := seedForeignSessionStartHook(t, settingsPath(projectClaudeDir))

	out, err := runCmd(t, newSetupCmd(), "", "--project", "--yes")
	if err != nil {
		t.Fatalf("setup --project --yes: %v\nout: %s", err, out)
	}

	data, rerr := readJSONFile(settingsPath(projectClaudeDir))
	if rerr != nil {
		t.Fatalf("read project-local settings after setup: %v", rerr)
	}
	hooks := data["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)

	var rawclawCount, foreignCount int
	for _, e := range arr {
		if containsRawclaw(e) {
			rawclawCount++
		} else {
			foreignCount++
		}
	}
	if rawclawCount != 1 {
		t.Errorf("want exactly 1 rawclaw SessionStart entry, got %d: %#v", rawclawCount, arr)
	}
	if foreignCount != 1 {
		t.Errorf("want the foreign entry preserved, got %d foreign entries: %#v", foreignCount, arr)
	}
	assertForeignEntryIntact(t, seeded, settingsPath(projectClaudeDir))

	scriptPath := hookScriptPath(projectClaudeDir)
	if _, serr := os.Stat(scriptPath); serr != nil {
		t.Errorf("hook script missing at project-local path %s: %v", scriptPath, serr)
	}

	// The global (default-scope) config must never be touched by a --project run.
	if _, serr := os.Stat(settingsPath(globalCfg)); !os.IsNotExist(serr) {
		t.Errorf("--project run wrote to the global config dir too: stat err=%v", serr)
	}
	if _, serr := os.Stat(hookScriptPath(globalCfg)); !os.IsNotExist(serr) {
		t.Errorf("--project run installed the hook script under the global config dir too: stat err=%v", serr)
	}
}

// TestSetupCmd_Project_IdempotentSecondRun: running `setup --project --yes`
// twice must leave exactly one rawclaw entry in the project-local settings
// file (no duplicates) and the foreign entry untouched — the same re-run
// story the global-scope test covers, isolated to the project-local path.
func TestSetupCmd_Project_IdempotentSecondRun(t *testing.T) {
	globalCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", globalCfg)
	t.Setenv("HOME", globalCfg)

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	projectClaudeDir := filepath.Join(projectDir, ".claude")
	seeded := seedForeignSessionStartHook(t, settingsPath(projectClaudeDir))

	if _, err := runCmd(t, newSetupCmd(), "", "--project", "--yes"); err != nil {
		t.Fatalf("first setup --project --yes: %v", err)
	}
	if _, err := runCmd(t, newSetupCmd(), "", "--project", "--yes"); err != nil {
		t.Fatalf("second setup --project --yes: %v", err)
	}

	data, rerr := readJSONFile(settingsPath(projectClaudeDir))
	if rerr != nil {
		t.Fatalf("read project-local settings after two runs: %v", rerr)
	}
	hooks := data["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)

	var rawclawCount, foreignCount int
	for _, e := range arr {
		if containsRawclaw(e) {
			rawclawCount++
		} else {
			foreignCount++
		}
	}
	if rawclawCount != 1 {
		t.Errorf("want exactly 1 rawclaw entry after second run, got %d: %#v", rawclawCount, arr)
	}
	if foreignCount != 1 {
		t.Errorf("want the foreign entry still present after second run, got %d: %#v", foreignCount, arr)
	}
	assertForeignEntryIntact(t, seeded, settingsPath(projectClaudeDir))
}

// TestSetupCmd_NoFlag_StaysGlobalEvenInsideAProject: without --project, setup
// must still write to the global scope even when run from inside a project
// directory — the default is never accidentally narrowed just by cwd.
func TestSetupCmd_NoFlag_StaysGlobalEvenInsideAProject(t *testing.T) {
	globalCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", globalCfg)
	t.Setenv("HOME", globalCfg)

	projectDir := t.TempDir()
	t.Chdir(projectDir)

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("setup --yes (no --project): %v", err)
	}

	if _, serr := os.Stat(settingsPath(globalCfg)); serr != nil {
		t.Errorf("global settings.json missing after default-scope setup: %v", serr)
	}
	if _, serr := os.Stat(filepath.Join(projectDir, ".claude")); !os.IsNotExist(serr) {
		t.Errorf("default-scope setup wrote a project-local .claude/ despite no --project flag: err=%v", serr)
	}
}

// TestSetupCmd_Eject_Global_RoundTripsSiblingIntactScriptGone drives install
// then eject at global scope against a sandbox pre-seeded with a foreign
// SessionStart entry: after eject the script and rawclaw's own entry must be
// gone, the foreign entry must survive byte/structurally intact, and the
// hooks/rawclaw script dir must be cascaded away (its parent hooks dir
// survives — the config file living directly under configDir keeps configDir
// itself non-empty).
func TestSetupCmd_Eject_Global_RoundTripsSiblingIntactScriptGone(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)
	seeded := seedForeignSessionStartHook(t, settingsPath(cfg))

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("setup --yes: %v", err)
	}

	out, err := runCmd(t, newSetupCmd(), "", "--eject", "--yes")
	if err != nil {
		t.Fatalf("setup --eject --yes: %v\nout: %s", err, out)
	}

	if _, serr := os.Stat(hookScriptPath(cfg)); !os.IsNotExist(serr) {
		t.Errorf("hook script should be gone after eject, stat err=%v", serr)
	}
	if _, serr := os.Stat(filepath.Join(cfg, "hooks", "rawclaw")); !os.IsNotExist(serr) {
		t.Errorf("hooks/rawclaw dir should be cascaded away, stat err=%v", serr)
	}

	data, rerr := readJSONFile(settingsPath(cfg))
	if rerr != nil {
		t.Fatalf("read settings after eject: %v", rerr)
	}
	if containsRawclaw(data["hooks"]) {
		t.Errorf("rawclaw entry should be gone after eject, got %#v", data["hooks"])
	}
	assertForeignEntryIntact(t, seeded, settingsPath(cfg))
}

// TestSetupCmd_Eject_Project_RoundTrips: same round-trip guarantee as the
// global test, but under --project both for install and eject.
func TestSetupCmd_Eject_Project_RoundTrips(t *testing.T) {
	globalCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", globalCfg)
	t.Setenv("HOME", globalCfg)

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	projectClaudeDir := filepath.Join(projectDir, ".claude")
	seeded := seedForeignSessionStartHook(t, settingsPath(projectClaudeDir))

	if _, err := runCmd(t, newSetupCmd(), "", "--project", "--yes"); err != nil {
		t.Fatalf("setup --project --yes: %v", err)
	}
	if _, err := runCmd(t, newSetupCmd(), "", "--project", "--eject", "--yes"); err != nil {
		t.Fatalf("setup --project --eject --yes: %v", err)
	}

	if _, serr := os.Stat(hookScriptPath(projectClaudeDir)); !os.IsNotExist(serr) {
		t.Errorf("project-local hook script should be gone after eject, stat err=%v", serr)
	}
	data, rerr := readJSONFile(settingsPath(projectClaudeDir))
	if rerr != nil {
		t.Fatalf("read project-local settings after eject: %v", rerr)
	}
	if containsRawclaw(data["hooks"]) {
		t.Errorf("rawclaw entry should be gone after eject, got %#v", data["hooks"])
	}
	assertForeignEntryIntact(t, seeded, settingsPath(projectClaudeDir))

	// The global (default-scope) config must never have been touched.
	if _, serr := os.Stat(settingsPath(globalCfg)); !os.IsNotExist(serr) {
		t.Errorf("--project eject touched the global config dir too: stat err=%v", serr)
	}
}

// TestSetupCmd_Eject_RawclawOnlyFile_DeletesConfigAndCascadesDirs: install
// with NO sibling ever seeded, then eject — since the config file held
// nothing but rawclaw's own entry, it must be deleted entirely, and every
// directory it lived under (hooks/rawclaw, hooks, and configDir itself) must
// cascade away since nothing else lives there.
func TestSetupCmd_Eject_RawclawOnlyFile_DeletesConfigAndCascadesDirs(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "claude-cfg")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", filepath.Dir(cfg))

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("setup --yes: %v", err)
	}
	if _, err := runCmd(t, newSetupCmd(), "", "--eject", "--yes"); err != nil {
		t.Fatalf("setup --eject --yes: %v", err)
	}

	if _, serr := os.Stat(settingsPath(cfg)); !os.IsNotExist(serr) {
		t.Errorf("rawclaw-only settings.json should be deleted, stat err=%v", serr)
	}
	if _, serr := os.Stat(cfg); !os.IsNotExist(serr) {
		t.Errorf("configDir should cascade away once fully empty, stat err=%v", serr)
	}
}

// TestSetupCmd_Eject_NothingInstalled_CleanNoOp: running `setup --eject --yes`
// on a machine with nothing ever installed must succeed with a clean no-op
// message, creating nothing on disk.
func TestSetupCmd_Eject_NothingInstalled_CleanNoOp(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "claude-cfg")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", filepath.Dir(cfg))

	out, err := runCmd(t, newSetupCmd(), "", "--eject", "--yes")
	if err != nil {
		t.Fatalf("setup --eject --yes on virgin machine: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "no-op") && !strings.Contains(out, "nothing to remove") {
		t.Errorf("want a clean no-op message, out: %s", out)
	}
	if _, serr := os.Stat(cfg); !os.IsNotExist(serr) {
		t.Errorf("eject must not create anything for a target that was never installed, stat err=%v", serr)
	}
}

// TestSetupCmd_Eject_InteractiveNo_LeavesInstallIntact: without --yes, a 'n'
// on stdin must abort eject and leave every previously-installed file
// exactly as it was.
func TestSetupCmd_Eject_InteractiveNo_LeavesInstallIntact(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("setup --yes: %v", err)
	}

	out, err := runCmd(t, newSetupCmd(), "n\n", "--eject")
	if err != nil {
		t.Fatalf("interactive eject 'n': %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("want an abort message, out: %s", out)
	}

	if _, serr := os.Stat(hookScriptPath(cfg)); serr != nil {
		t.Errorf("hook script should survive a declined eject: %v", serr)
	}
	if _, serr := os.Stat(settingsPath(cfg)); serr != nil {
		t.Errorf("settings.json should survive a declined eject: %v", serr)
	}
}

// TestSetupCmd_Eject_WiresBothTargets: install with both Claude Code and
// Codex present (each pre-seeded with its own foreign entry), then eject —
// both targets must come out clean: rawclaw gone, script gone, foreign entry
// intact in each.
func TestSetupCmd_Eject_WiresBothTargets(t *testing.T) {
	claudeCfg := t.TempDir()
	codexCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeCfg)
	t.Setenv("CODEX_HOME", codexCfg)
	t.Setenv("HOME", claudeCfg)

	claudeSeeded := seedForeignSessionStartHook(t, settingsPath(claudeCfg))
	codexSeeded := seedForeignSessionStartHook(t, codexHooksPath(codexCfg))

	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("setup --yes: %v", err)
	}
	out, err := runCmd(t, newSetupCmd(), "", "--eject", "--yes")
	if err != nil {
		t.Fatalf("setup --eject --yes: %v\nout: %s", err, out)
	}

	if _, serr := os.Stat(hookScriptPath(claudeCfg)); !os.IsNotExist(serr) {
		t.Errorf("claude hook script should be gone, stat err=%v", serr)
	}
	if _, serr := os.Stat(hookScriptPath(codexCfg)); !os.IsNotExist(serr) {
		t.Errorf("codex hook script should be gone, stat err=%v", serr)
	}

	assertForeignEntryIntact(t, claudeSeeded, settingsPath(claudeCfg))
	assertForeignEntryIntact(t, codexSeeded, codexHooksPath(codexCfg))

	claudeData, err := readJSONFile(settingsPath(claudeCfg))
	if err != nil {
		t.Fatalf("read claude settings after eject: %v", err)
	}
	if containsRawclaw(claudeData["hooks"]) {
		t.Errorf("claude rawclaw entry should be gone, got %#v", claudeData["hooks"])
	}
	codexData, err := readJSONFile(codexHooksPath(codexCfg))
	if err != nil {
		t.Fatalf("read codex hooks after eject: %v", err)
	}
	if containsRawclaw(codexData["hooks"]) {
		t.Errorf("codex rawclaw entry should be gone, got %#v", codexData["hooks"])
	}
}

// TestSetupCmd_HelpMentionsEjectAndCodexLimitation: the Long help text must
// document --eject and the known Codex stale trust-state-row limitation, so a
// user reading --help learns about it without digging through source.
func TestSetupCmd_HelpMentionsEjectAndCodexLimitation(t *testing.T) {
	long := newSetupCmd().Long
	if !strings.Contains(long, "--eject") {
		t.Errorf("help text should mention --eject, got: %s", long)
	}
	if !strings.Contains(strings.ToLower(long), "trust-state") {
		t.Errorf("help text should mention the Codex stale trust-state limitation, got: %s", long)
	}
}
