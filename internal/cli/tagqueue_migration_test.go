package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetupRemovesLegacyTagQueueHook pins the upgrade path off rawclaw <= v0.5.x.
//
// Those versions installed a SessionEnd hook at hooks/rawclaw/tagqueue.sh that
// shelled out to `rawclaw tag-queue add`. That verb no longer exists — sessions
// self-tag at closeout instead — so an upgraded install that kept the script
// would fire a hook calling a dead command on every session end. Setup has to
// clean it up, not just stop writing it.
func TestSetupRemovesLegacyTagQueueHook(t *testing.T) {
	dir := t.TempDir()
	cf := filepath.Join(dir, "settings.json")

	// Stand up exactly what an old install left behind: the script on disk, and
	// a SessionEnd entry pointing at it alongside a foreign hook that must survive.
	legacy := legacyTagQueueScriptPath(dir)
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := map[string]any{
		"hooks": map[string]any{
			"SessionEnd": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": legacy}}},
			},
			"PreToolUse": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "/opt/someone-else/guard.sh"}}},
			},
		},
	}
	b, _ := json.Marshal(old)
	if err := os.WriteFile(cf, b, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installRawclawHookAt(dir, cf, rawclawPrimeScript); err != nil {
		t.Fatalf("install: %v", err)
	}

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy tagqueue.sh still on disk at %s (err=%v)", legacy, err)
	}

	raw, err := os.ReadFile(cf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "tagqueue.sh") {
		t.Errorf("settings still reference tagqueue.sh:\n%s", raw)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	hooks, _ := got["hooks"].(map[string]any)
	if _, exists := hooks["SessionEnd"]; exists {
		t.Errorf("SessionEnd entry survived the upgrade: %v", hooks["SessionEnd"])
	}
	if _, exists := hooks["SessionStart"]; !exists {
		t.Error("SessionStart discovery hook was not installed")
	}
	// A sibling tool's hook is not ours to touch, on any event.
	if !strings.Contains(string(raw), "/opt/someone-else/guard.sh") {
		t.Errorf("foreign PreToolUse hook was clobbered:\n%s", raw)
	}
}

// TestSetupIsCleanWithoutLegacyHook guards the ordinary path: a fresh machine
// has no tagqueue.sh, and removing something that was never there must not be
// an error.
func TestSetupIsCleanWithoutLegacyHook(t *testing.T) {
	dir := t.TempDir()
	cf := filepath.Join(dir, "settings.json")
	if err := installRawclawHookAt(dir, cf, rawclawPrimeScript); err != nil {
		t.Fatalf("install on a machine with no legacy hook: %v", err)
	}
	raw, err := os.ReadFile(cf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SessionEnd") {
		t.Errorf("a fresh install registered SessionEnd:\n%s", raw)
	}
}

// TestSetupKeepsLegacyHookWhenConfigWriteFails pins the ordering fix: the
// legacy script must survive a failed settings write, because deleting it first
// would leave the still-registered SessionEnd entry pointing at a missing file.
func TestSetupKeepsLegacyHookWhenConfigWriteFails(t *testing.T) {
	dir := t.TempDir()
	cf := filepath.Join(dir, "settings.json")

	legacy := legacyTagQueueScriptPath(dir)
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Off-schema but legal JSON: addRawclawHooks refuses rather than clobbering,
	// so the write never happens.
	if err := os.WriteFile(cf, []byte(`{"hooks": {"SessionStart": "not-an-array"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installRawclawHookAt(dir, cf, rawclawPrimeScript); err == nil {
		t.Fatal("expected install to refuse an off-schema hooks entry")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("legacy script was deleted despite the config write failing: %v", err)
	}
}
