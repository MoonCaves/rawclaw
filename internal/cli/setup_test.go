package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestReadJSONFileToleratesLineCommentsAndTrailingCommas mirrors the othertool
// fixture exactly: a `//` line comment outside any string, plus trailing
// commas before both a `]` and a `}`, must parse — and a `//`-looking
// substring INSIDE a string value must survive untouched (proving the
// scanner tracks string state rather than blindly stripping `//`).
func TestReadJSONFileToleratesLineCommentsAndTrailingCommas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	fixture := `{ // keep this comment outside strings
  "hooks": {"SessionStart": [{"command": "echo // not a comment"},],},}`
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	data, err := readJSONFile(path)
	if err != nil {
		t.Fatalf("readJSONFile: %v", err)
	}

	hooks, ok := data["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks missing or wrong type: %#v", data["hooks"])
	}
	arr, ok := hooks["SessionStart"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("SessionStart = %#v, want one entry", hooks["SessionStart"])
	}
	entry, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("entry = %#v, want map", arr[0])
	}
	if got := entry["command"]; got != "echo // not a comment" {
		t.Errorf("in-string comment marker was stripped: got %q", got)
	}
}

// TestReadJSONFileMissingReturnsEmptyMap: a fresh machine with no settings.json
// yet must read as an empty map, not an error — nothing to merge into yet is
// the normal first-run state, not a failure.
func TestReadJSONFileMissingReturnsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")

	data, err := readJSONFile(path)
	if err != nil {
		t.Fatalf("readJSONFile on missing file: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("want empty map for missing file, got %#v", data)
	}
}

// TestWriteJSONFileAtomic: a write leaves the final content readable at path
// and cleans up its `.tmp` sibling (rename, not copy-then-leave-behind).
func TestWriteJSONFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "settings.json")
	data := map[string]any{"hello": "world"}

	if err := writeJSONFile(path, data); err != nil {
		t.Fatalf("writeJSONFile: %v", err)
	}

	got, err := readJSONFile(path)
	if err != nil {
		t.Fatalf("readJSONFile after write: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("got %#v, want hello=world", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp sibling should be gone after rename, stat err=%v", err)
	}
}

// TestContainsRawclaw covers the marker match across nested shapes, and the
// negative case (a foreign entry with no rawclaw substring anywhere).
func TestContainsRawclaw(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want bool
	}{
		{"bare string match", "/home/user/.claude/hooks/rawclaw/prime.sh", true},
		{"bare string no match", "/home/user/.claude/hooks/othertool/prime.sh", false},
		{
			"nested in map",
			map[string]any{"command": "/x/hooks/rawclaw/prime.sh"},
			true,
		},
		{
			"nested in array of maps",
			[]any{map[string]any{"type": "command", "command": "/x/hooks/rawclaw/prime.sh"}},
			true,
		},
		{
			"foreign entry, no match",
			map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "/x/hooks/othertool/prime.sh"},
			}},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := containsRawclaw(tc.in); got != tc.want {
				t.Errorf("containsRawclaw(%#v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestAddRawclawHookReplacesExistingRawclawEntry mirrors othertool's
// TestAddClaudeHooksSelectiveReplacesExistingOthertoolHooks: seed SessionStart
// with a stale rawclaw entry (an old script path) plus a foreign custom
// entry, then call addRawclawSessionStartHook — the stale rawclaw row must be
// replaced (not duplicated), and the foreign entry must be untouched
// (identity-equal, not just "looks similar").
func TestAddRawclawHookReplacesExistingRawclawEntry(t *testing.T) {
	foreign := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "/foreign/tool/hook.sh"},
		},
	}
	stale := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "/old/path/hooks/rawclaw/prime.sh"},
		},
	}
	data := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{stale, foreign},
		},
	}

	addRawclawSessionStartHook(data, "/new/path/hooks/rawclaw/prime.sh")

	hooks := data["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)
	if len(arr) != 2 {
		t.Fatalf("SessionStart has %d entries, want 2 (foreign kept + one fresh rawclaw): %#v", len(arr), arr)
	}

	var sawForeign, sawFreshRawclaw bool
	newCount := 0
	for _, e := range arr {
		if containsRawclaw(e) {
			newCount++
			m := e.(map[string]any)
			cmdArr := m["hooks"].([]any)
			cmdEntry := cmdArr[0].(map[string]any)
			if cmdEntry["command"] == "/new/path/hooks/rawclaw/prime.sh" {
				sawFreshRawclaw = true
			}
		} else {
			sawForeign = true
			m := e.(map[string]any)
			cmdArr := m["hooks"].([]any)
			cmdEntry := cmdArr[0].(map[string]any)
			if cmdEntry["command"] != "/foreign/tool/hook.sh" {
				t.Errorf("foreign entry mutated: %#v", e)
			}
		}
	}
	if newCount != 1 {
		t.Errorf("want exactly 1 rawclaw entry after replace, got %d", newCount)
	}
	if !sawFreshRawclaw {
		t.Errorf("want the fresh scriptPath present, arr=%#v", arr)
	}
	if !sawForeign {
		t.Errorf("foreign entry was dropped, arr=%#v", arr)
	}
}

// TestAddRawclawHookIdempotentSecondCall: calling addRawclawSessionStartHook
// twice with the same scriptPath must still leave exactly one rawclaw entry —
// the re-run story the CLI-level idempotency test also covers, isolated here
// at the merge-engine seam.
func TestAddRawclawHookIdempotentSecondCall(t *testing.T) {
	data := map[string]any{}
	addRawclawSessionStartHook(data, "/x/hooks/rawclaw/prime.sh")
	addRawclawSessionStartHook(data, "/x/hooks/rawclaw/prime.sh")

	hooks := data["hooks"].(map[string]any)
	arr := hooks["SessionStart"].([]any)
	if len(arr) != 1 {
		t.Fatalf("want exactly 1 entry after two calls, got %d: %#v", len(arr), arr)
	}
}

// TestInstallRawclawCodexHookWritesHooksJSON verifies the Codex target reuses
// the same merge engine and script as Claude Code — same
// installRawclawHookAt seam, same SessionStart shape — writing to hooks.json
// instead of settings.json. Also locks the rule that install must never write a
// Codex `[features]` flag (no feature-flag writes, no auto-trust).
func TestInstallRawclawCodexHookWritesHooksJSON(t *testing.T) {
	dir := t.TempDir()
	if err := installRawclawCodexHook(dir); err != nil {
		t.Fatalf("installRawclawCodexHook: %v", err)
	}

	scriptPath := hookScriptPath(dir)
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("hook script missing: %v", err)
	}

	data, err := readJSONFile(codexHooksPath(dir))
	if err != nil {
		t.Fatalf("readJSONFile(hooks.json): %v", err)
	}
	if _, hasFeatures := data["features"]; hasFeatures {
		t.Errorf("installRawclawCodexHook must not write a [features] flag, got %#v", data["features"])
	}
	hooks, ok := data["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks missing or wrong type: %#v", data["hooks"])
	}
	arr, ok := hooks["SessionStart"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("SessionStart = %#v, want one entry", hooks["SessionStart"])
	}
	if !containsRawclaw(arr[0]) {
		t.Errorf("entry does not carry the rawclaw marker: %#v", arr[0])
	}
}

// TestRemoveRawclawHooksDropsEmptyEventAndHooksKey: when a rawclaw entry is
// the ONLY thing in an event array, removing it must drop that event key
// entirely; when it's the only event, the whole "hooks" key is dropped too —
// mirroring othertool's RemoveClaudeHooks cleanup, and the shape a later --eject
// slice depends on.
func TestRemoveRawclawHooksDropsEmptyEventAndHooksKey(t *testing.T) {
	data := map[string]any{
		"other_setting": true,
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{"hooks": []any{
					map[string]any{"type": "command", "command": "/x/hooks/rawclaw/prime.sh"},
				}},
			},
		},
	}

	removeRawclawHooks(data)

	if _, ok := data["hooks"]; ok {
		t.Errorf("hooks key should be fully removed, got %#v", data["hooks"])
	}
	if data["other_setting"] != true {
		t.Errorf("unrelated top-level key was disturbed: %#v", data)
	}
}

// TestScopeConfigDirGlobalDefault: with project=false, scopeConfigDir must
// return globalDir verbatim, never touching cwd — the global default.
func TestScopeConfigDirGlobalDefault(t *testing.T) {
	got, err := scopeConfigDir(false, "/global/.claude", ".claude")
	if err != nil {
		t.Fatalf("scopeConfigDir: %v", err)
	}
	if got != "/global/.claude" {
		t.Errorf("scopeConfigDir(false, ...) = %q, want the global dir untouched", got)
	}
}

// TestScopeConfigDirProjectOptIn: with project=true, scopeConfigDir must
// resolve to cwd joined with base, ignoring globalDir entirely — the
// --project narrowing opt-in.
func TestScopeConfigDirProjectOptIn(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	got, err := scopeConfigDir(true, "/global/.claude", ".claude")
	if err != nil {
		t.Fatalf("scopeConfigDir: %v", err)
	}
	want := filepath.Join(dir, ".claude")
	if got != want {
		t.Errorf("scopeConfigDir(true, ...) = %q, want %q", got, want)
	}
}

// TestProjectConfigDirUsesBase proves projectConfigDir is parameterized on
// base rather than hardcoded to ".claude" — the seam a future Codex slice
// reuses with ".codex" instead of forking its own resolver.
func TestProjectConfigDirUsesBase(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	got, err := projectConfigDir(".codex")
	if err != nil {
		t.Fatalf("projectConfigDir: %v", err)
	}
	want := filepath.Join(dir, ".codex")
	if got != want {
		t.Errorf("projectConfigDir(\".codex\") = %q, want %q", got, want)
	}
}

// TestMaybePrintProjectTrustWarning covers every target/scope combination:
// the warning is Codex-only AND project-scope-only — the
// Claude Code target (the only one this slice wires up) must never print it,
// proving the hook is a safe no-op until the Codex slice switches it on.
func TestMaybePrintProjectTrustWarning(t *testing.T) {
	tests := []struct {
		name    string
		target  setupTarget
		project bool
		want    bool
	}{
		{"claude-code, global", targetClaudeCode, false, false},
		{"claude-code, project", targetClaudeCode, true, false},
		{"codex, global", targetCodex, false, false},
		{"codex, project", targetCodex, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			maybePrintProjectTrustWarning(&buf, tc.target, tc.project)
			got := buf.Len() > 0
			if got != tc.want {
				t.Errorf("maybePrintProjectTrustWarning(target=%s, project=%v) printed=%v, want %v; output=%q",
					tc.target, tc.project, got, tc.want, buf.String())
			}
		})
	}
}

// TestRemoveRawclawHooksSparesForeignRawclawNamedPath: a sibling tool whose
// command merely lives under a rawclaw-named directory is NOT ours — the
// identity marker must match the installed script's path suffix, not the bare
// word. Repro'd live before the fix: the loose marker deleted this entry.
func TestRemoveRawclawHooksSparesForeignRawclawNamedPath(t *testing.T) {
	data := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{"type": "command", "command": "/home/me/rawclaw-notes/hooks/other-tool.sh"},
					},
				},
			},
		},
	}
	if err := addRawclawSessionStartHook(data, "/cfg/hooks/rawclaw/prime.sh"); err != nil {
		t.Fatalf("add: %v", err)
	}
	arr := data["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(arr) != 2 {
		t.Fatalf("want foreign entry + rawclaw entry (2), got %d: %#v", len(arr), arr)
	}
	var foreign int
	for _, e := range arr {
		if !containsRawclaw(e) {
			foreign++
		}
	}
	if foreign != 1 {
		t.Errorf("foreign rawclaw-named-path entry was treated as ours (foreign=%d)", foreign)
	}
}

// TestAddRawclawHookRefusesOffShapeSessionStart: legal JSON with SessionStart
// holding a non-array must be refused, never silently clobbered.
func TestAddRawclawHookRefusesOffShapeSessionStart(t *testing.T) {
	data := map[string]any{
		"hooks": map[string]any{
			"SessionStart": map[string]any{"weird": true},
		},
	}
	if err := addRawclawSessionStartHook(data, "/cfg/hooks/rawclaw/prime.sh"); err == nil {
		t.Fatal("want an error on off-shape SessionStart, got nil (silent clobber)")
	}
	if _, ok := data["hooks"].(map[string]any)["SessionStart"].(map[string]any); !ok {
		t.Errorf("off-shape value was mutated despite the refusal")
	}
}

// TestStripJSON5TrailingCommaBeforeComment: a trailing comma separated from
// its closing bracket by a `//` comment must still be stripped.
func TestStripJSON5TrailingCommaBeforeComment(t *testing.T) {
	in := "{\n  \"a\": [1, // note\n  ],\n}\n"
	var out map[string]any
	if err := json.Unmarshal([]byte(stripJSON5(in)), &out); err != nil {
		t.Fatalf("stripJSON5 left invalid JSON: %v\nstripped: %q", err, stripJSON5(in))
	}
}

// TestRemoveIfEmptyDeletesOnlyWhenEmpty: an empty dir is removed; a dir
// holding a sibling file is left standing; a missing dir is a silent no-op —
// the whole safety story eject's directory cascade depends on.
func TestRemoveIfEmptyDeletesOnlyWhenEmpty(t *testing.T) {
	root := t.TempDir()

	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatalf("mkdir empty: %v", err)
	}
	removeIfEmpty(empty)
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Errorf("empty dir should be gone, stat err=%v", err)
	}

	nonEmpty := filepath.Join(root, "non-empty")
	if err := os.Mkdir(nonEmpty, 0o755); err != nil {
		t.Fatalf("mkdir non-empty: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmpty, "sibling.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write sibling file: %v", err)
	}
	removeIfEmpty(nonEmpty)
	if _, err := os.Stat(nonEmpty); err != nil {
		t.Errorf("non-empty dir should survive, stat err=%v", err)
	}

	// Missing dir: must not panic or error.
	removeIfEmpty(filepath.Join(root, "does-not-exist"))
}

// TestWriteOrRemoveConfigFileDeletesWhenEmpty: an empty data map removes the
// file (missing is fine too); a non-empty map is written normally.
func TestWriteOrRemoveConfigFileDeletesWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := writeJSONFile(path, map[string]any{"other_setting": true}); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := writeOrRemoveConfigFile(path, map[string]any{}); err != nil {
		t.Fatalf("writeOrRemoveConfigFile(empty): %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should be removed when data is empty, stat err=%v", err)
	}

	// Removing an already-missing file must not error.
	if err := writeOrRemoveConfigFile(path, map[string]any{}); err != nil {
		t.Errorf("writeOrRemoveConfigFile on missing file: %v", err)
	}

	if err := writeOrRemoveConfigFile(path, map[string]any{"other_setting": true}); err != nil {
		t.Fatalf("writeOrRemoveConfigFile(non-empty): %v", err)
	}
	got, err := readJSONFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got["other_setting"] != true {
		t.Errorf("non-empty data should be written, got %#v", got)
	}
}

// TestEjectRawclawHookAtRawclawOnlyFileDeletesFileAndCascadesDirs: a config
// dir holding NOTHING but rawclaw's own script and entry must end up with the
// script gone, the config file deleted (nothing else was in it), and every
// directory cascaded away including configDir itself — no sibling content
// anywhere means nothing survives to keep any of it around.
func TestEjectRawclawHookAtRawclawOnlyFileDeletesFileAndCascadesDirs(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "cfg")
	if err := installRawclawHook(configDir); err != nil {
		t.Fatalf("installRawclawHook: %v", err)
	}

	out, err := ejectRawclawHook(configDir)
	if err != nil {
		t.Fatalf("ejectRawclawHook: %v", err)
	}
	if !out.scriptRemoved {
		t.Errorf("want scriptRemoved=true, got %#v", out)
	}
	if !out.entryRemoved {
		t.Errorf("want entryRemoved=true, got %#v", out)
	}
	if !out.fileDeleted {
		t.Errorf("want fileDeleted=true (rawclaw-only file), got %#v", out)
	}

	if _, serr := os.Stat(hookScriptPath(configDir)); !os.IsNotExist(serr) {
		t.Errorf("script should be gone, stat err=%v", serr)
	}
	if _, serr := os.Stat(settingsPath(configDir)); !os.IsNotExist(serr) {
		t.Errorf("settings.json should be deleted, stat err=%v", serr)
	}
	if _, serr := os.Stat(filepath.Join(configDir, "hooks")); !os.IsNotExist(serr) {
		t.Errorf("hooks dir should be cascaded away, stat err=%v", serr)
	}
	if _, serr := os.Stat(configDir); !os.IsNotExist(serr) {
		t.Errorf("configDir itself should be cascaded away once fully empty, stat err=%v", serr)
	}
}

// TestEjectRawclawHookAtKeepsSiblingOnlyFile: a config file holding a foreign
// entry alongside rawclaw's own must survive eject with the foreign entry
// intact and the file NOT deleted, even though rawclaw's own entry and script
// are both gone.
func TestEjectRawclawHookAtKeepsSiblingOnlyFile(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "cfg")
	seeded := seedForeignSessionStartHook(t, settingsPath(configDir))
	if err := installRawclawHook(configDir); err != nil {
		t.Fatalf("installRawclawHook: %v", err)
	}

	out, err := ejectRawclawHook(configDir)
	if err != nil {
		t.Fatalf("ejectRawclawHook: %v", err)
	}
	if !out.scriptRemoved || !out.entryRemoved {
		t.Errorf("want script and entry both removed, got %#v", out)
	}
	if out.fileDeleted {
		t.Errorf("sibling-holding file must not be deleted, got %#v", out)
	}

	if _, serr := os.Stat(settingsPath(configDir)); serr != nil {
		t.Errorf("settings.json should survive (sibling entry remains): %v", serr)
	}
	assertForeignEntryIntact(t, seeded, settingsPath(configDir))

	data, rerr := readJSONFile(settingsPath(configDir))
	if rerr != nil {
		t.Fatalf("read settings after eject: %v", rerr)
	}
	if containsRawclaw(data["hooks"]) {
		t.Errorf("rawclaw entry should be gone, got %#v", data["hooks"])
	}
	if _, serr := os.Stat(hookScriptPath(configDir)); !os.IsNotExist(serr) {
		t.Errorf("script should be gone, stat err=%v", serr)
	}
}

// TestEjectRawclawHookAtNothingInstalledIsCleanNoOp: ejecting a configDir that
// was never installed into must return a nothing-to-report outcome, create
// nothing, and error on nothing.
func TestEjectRawclawHookAtNothingInstalledIsCleanNoOp(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "cfg")

	out, err := ejectRawclawHook(configDir)
	if err != nil {
		t.Fatalf("ejectRawclawHook on virgin dir: %v", err)
	}
	if out.didAnything() {
		t.Errorf("want a clean no-op, got %#v", out)
	}
	if _, serr := os.Stat(configDir); !os.IsNotExist(serr) {
		t.Errorf("eject must not create configDir for a target that was never installed, stat err=%v", serr)
	}
}

// TestEjectKeepsConfigFileWithNonHookKeys: a config whose only other content
// is non-hook top-level keys (e.g. a model preference) must survive eject with
// those keys intact — eject deletes the file only when truly nothing remains.
func TestEjectKeepsConfigFileWithNonHookKeys(t *testing.T) {
	dir := t.TempDir()
	cf := settingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(cf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cf, []byte(`{"model": "opus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installRawclawHookAt(dir, cf); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := ejectRawclawHookAt(dir, cf); err != nil {
		t.Fatalf("eject: %v", err)
	}
	data, err := readJSONFile(cf)
	if err != nil {
		t.Fatalf("config file deleted despite non-hook keys: %v", err)
	}
	if data["model"] != "opus" {
		t.Errorf("non-hook key lost across install/eject round-trip: %#v", data)
	}
	if _, hasHooks := data["hooks"]; hasHooks {
		t.Errorf("empty hooks key left behind after eject: %#v", data)
	}
}

// TestEjectSparesUserFilesInScriptDir (third-party review, critical): a user
// file parked inside hooks/rawclaw must survive eject — only the installed
// script is ours to remove, and the dir cascade fires only on truly empty.
func TestEjectSparesUserFilesInScriptDir(t *testing.T) {
	dir := t.TempDir()
	cf := settingsPath(dir)
	if err := installRawclawHookAt(dir, cf); err != nil {
		t.Fatalf("install: %v", err)
	}
	userFile := filepath.Join(filepath.Dir(hookScriptPath(dir)), "notes.txt")
	if err := os.WriteFile(userFile, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ejectRawclawHookAt(dir, cf); err != nil {
		t.Fatalf("eject: %v", err)
	}
	if _, err := os.Stat(hookScriptPath(dir)); !os.IsNotExist(err) {
		t.Errorf("installed script should be gone, stat err=%v", err)
	}
	if b, err := os.ReadFile(userFile); err != nil || string(b) != "mine" {
		t.Errorf("user file in hooks/rawclaw was destroyed by eject: %v %q", err, b)
	}
}

// TestEjectLeavesForeignEmptyConfigUntouched (third-party review, high): a
// pre-existing config with NO rawclaw entry — even an empty {} — is not ours
// to rewrite or delete; a no-op eject leaves it byte-identical.
func TestEjectLeavesForeignEmptyConfigUntouched(t *testing.T) {
	dir := t.TempDir()
	cf := settingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(cf), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := []byte("{}\n")
	if err := os.WriteFile(cf, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ejectRawclawHookAt(dir, cf); err != nil {
		t.Fatalf("eject: %v", err)
	}
	b, err := os.ReadFile(cf)
	if err != nil {
		t.Fatalf("user's empty config was deleted by a no-op eject: %v", err)
	}
	if string(b) != string(orig) {
		t.Errorf("no-op eject rewrote the file: %q -> %q", orig, b)
	}
}

// TestInstallRefusesNonObjectHooksValue (third-party review, high): a config
// where top-level "hooks" holds a non-object is refused, never overwritten.
func TestInstallRefusesNonObjectHooksValue(t *testing.T) {
	dir := t.TempDir()
	cf := settingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(cf), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := []byte(`{"hooks": "user-defined-value", "model": "opus"}`)
	if err := os.WriteFile(cf, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installRawclawHookAt(dir, cf); err == nil {
		t.Fatal("want an error on non-object hooks value, got nil (silent clobber)")
	}
	b, err := os.ReadFile(cf)
	if err != nil || string(b) != string(orig) {
		t.Errorf("config mutated despite the refusal: %v %q", err, b)
	}
}
