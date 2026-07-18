package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// rawclawMarker identifies a rawclaw-owned entry by its installed script's
// path suffix — NOT the bare word "rawclaw", which would also match a sibling
// tool whose command merely lives under a rawclaw-named directory (e.g.
// /home/me/rawclaw-notes/hooks/other-tool.sh) and delete it. Matching the full
// hooks/rawclaw/prime.sh suffix keeps installs idempotent (re-running finds
// and replaces rawclaw's own rows) while a sibling entry (othertool,
// agent-monitor, cmux, or anything else sharing SessionStart) is never
// touched. Path separators are normalized before matching so the identity
// check holds on Windows too.
const rawclawMarker = "hooks/rawclaw/prime.sh"

// rawclawPrimeScript is installed at <configDir>/hooks/rawclaw/prime.sh and
// registered as a Claude Code SessionStart hook. POSIX sh only — a SessionStart
// hook runs with no guaranteed bash. It self-gates on the rawclaw binary being
// on PATH (a machine with the hook wired but the binary since removed degrades
// to a silent no-op, never a hook error) and prints the discovery banner at most
// once per session, keyed on the session_id Claude Code passes on the hook's
// stdin (undocumented exact schema, so the id is pulled with a tolerant sed
// scan rather than a full JSON parse — no jq/python dependency assumed).
const rawclawPrimeScript = `#!/bin/sh
# Installed by ` + "`rawclaw setup`" + `; removed by ` + "`rawclaw setup --eject`" + ` along with
# its settings.json entry. Prints a one-time discovery banner on Claude Code
# SessionStart so the agent knows rawclaw exists.
set -eu

# No rawclaw on PATH (uninstalled since this hook was wired, or never
# installed) — silent no-op rather than a hook error.
command -v rawclaw >/dev/null 2>&1 || exit 0

input=$(cat)
session_id=$(printf '%s' "$input" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)

# Once-per-session marker: a session_id we can't extract just means the banner
# prints every time for that call (harmless), rather than failing the hook.
if [ -n "$session_id" ]; then
	marker_dir="${TMPDIR:-/tmp}/rawclaw-prime"
	mkdir -p "$marker_dir" 2>/dev/null || true
	marker="$marker_dir/$session_id"
	if [ -f "$marker" ]; then
		exit 0
	fi
	: > "$marker" 2>/dev/null || true
fi

cat <<'BANNER'
[rawclaw] Raw transcript history for context — the receipts + thought process behind past
sessions, across every project on this machine (not just this one's native session folder).
Fast FTS5/BM25 search: cheaper than grepping your own agent's session folders (Claude Code
projects/, Codex sessions/) — use rawclaw instead and save tokens + greps. Memory providers
hold the superseding current truth; rawclaw is the dated raw record underneath it.
  rawclaw "query"              search every session  (--this-project / --include-path <re> to scope; --sort newest)
  rawclaw read <ref>           the matched message whole, with context  (--more / --around to expand)
  rawclaw outline <sess8>      a session's goal -> resolution arc
--json for structured output; --help for the rest.
If the user seems to want to pick up a past session, offering to resume/fork it can help.
BANNER
`

// setupTarget names the agent whose config `rawclaw setup` is wiring into.
// Only targetClaudeCode is implemented this slice; targetCodex is declared
// now so a Codex target (whose project-trust
// warning) has a stable value to switch on with no rename once it merges.
type setupTarget string

const (
	targetClaudeCode setupTarget = "claude-code"
	targetCodex      setupTarget = "codex"
)

// projectTrustWarning is the Codex-only project-scope caveat:
// untrusted Codex projects silently skip project-local
// `.codex/` config layers — including hooks.json — entirely, so a
// project-local rawclaw hook may never fire until the project is
// Codex-trusted; the default global install has no such gate. The survey
// found no equivalent gate on Claude Code's project-local
// .claude/settings.json, so this text is Codex-only.
const projectTrustWarning = "Warning: this project is not yet Codex-trusted. A project-local hook " +
	"silently won't fire until the project passes Codex's own trust review (see Codex's `/hooks`); " +
	"the default global install has no such gate."

// maybePrintProjectTrustWarning prints projectTrustWarning when target/scope
// requires it (currently: target == targetCodex and scope is project-local),
// both for interactive and --yes runs — call it once, right after scope
// resolution and before describing what setup will write. Every other
// target/scope combination (including every call this slice makes, since only
// targetClaudeCode exists here) is a deliberate no-op: the seam a later Codex
// slice switches on by passing targetCodex, with no change to this function.
func maybePrintProjectTrustWarning(out io.Writer, target setupTarget, project bool) {
	if target != targetCodex || !project {
		return
	}
	fmt.Fprintln(out, projectTrustWarning)
	fmt.Fprintln(out)
}

// projectConfigDir resolves cwd's own project-local config dir named base
// (e.g. "`.claude`"; a future Codex slice would pass its own, e.g.
// "`.codex`") — the --project narrowing opt-in's target, matching how Claude
// Code itself layers project settings inside the project directory.
func projectConfigDir(base string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd for --project: %w", err)
	}
	return filepath.Join(cwd, base), nil
}

// scopeConfigDir resolves the config dir `rawclaw setup` writes to: globalDir
// (global unless narrowed — rawclaw searches every project, so a global hook
// is the honest default) unless project
// is set, in which case it resolves to cwd's own base-named dir instead. One
// function every target's scope routing shares — a later Codex slice reuses
// it with its own globalDir/base rather than forking a second copy.
func scopeConfigDir(project bool, globalDir, base string) (string, error) {
	if !project {
		return globalDir, nil
	}
	return projectConfigDir(base)
}

// hookScriptPath is the fixed location `rawclaw setup` installs the discovery
// hook to, under a target Claude Code config dir.
func hookScriptPath(configDir string) string {
	return filepath.Join(configDir, "hooks", "rawclaw", "prime.sh")
}

// settingsPath is the Claude Code settings file a target config dir owns.
func settingsPath(configDir string) string {
	return filepath.Join(configDir, "settings.json")
}

// codexHooksPath is the Codex hooks file a target config dir owns. Codex's
// hooks.json shares the identical {"hooks": {"<Event>": [...]}} shape Claude
// Code's settings.json uses at the hooks level, so every merge-engine
// primitive below (read/write/add/remove) is reused verbatim across both
// targets — only the file path (and which top-level file we merge into)
// differs.
func codexHooksPath(configDir string) string {
	return filepath.Join(configDir, "hooks.json")
}

// writeHookScript (re)writes the discovery-hook script to path, creating its
// parent dir as needed. Executable (0o755): Claude Code invokes it directly.
func writeHookScript(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(rawclawPrimeScript), 0o755); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// readJSONFile reads a JSON5-tolerant settings file — Claude Code's own
// settings.json tolerates `//` line comments and trailing commas before a
// closing `]`/`}`, so a hand-edited file with either must still parse cleanly
// rather than erroring the merge. A missing file reads as an empty map: a
// fresh machine has nothing to merge into yet, not a failure.
func readJSONFile(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	cleaned := stripJSON5(string(b))
	data := map[string]any{}
	if err := json.Unmarshal([]byte(cleaned), &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return data, nil
}

// stripJSON5 strips `//` line comments and trailing commas (before a closing
// `]`/`}`) from JSON5-ish input, leaving valid JSON behind. A single
// character-by-character scan tracks whether it is inside a quoted string (and
// whether the next char is escaped) so a comment marker or trailing comma
// INSIDE a string value is never touched — only structural/comment bytes
// outside strings are stripped.
func stripJSON5(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false
	i := 0
	for i < len(s) {
		ch := s[i]
		if escaped {
			b.WriteByte(ch)
			escaped = false
			i++
			continue
		}
		if inString {
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			b.WriteByte(ch)
			i++
			continue
		}
		if ch == '"' {
			inString = true
			b.WriteByte(ch)
			i++
			continue
		}
		if ch == '/' && i+1 < len(s) && s[i+1] == '/' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if ch == ',' {
			// Look ahead across whitespace AND `//` line comments: a comma is
			// trailing when the next structural byte is a closing bracket, even
			// with a comment between them (`[1, // note\n]`).
			j := i + 1
			for j < len(s) {
				if s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r' {
					j++
					continue
				}
				if s[j] == '/' && j+1 < len(s) && s[j+1] == '/' {
					for j < len(s) && s[j] != '\n' {
						j++
					}
					continue
				}
				break
			}
			if j < len(s) && (s[j] == ']' || s[j] == '}') {
				i++ // drop the trailing comma; the closing bracket/brace follows as-is
				continue
			}
		}
		b.WriteByte(ch)
		i++
	}
	return b.String()
}

// writeJSONFile writes data as indented JSON, atomically: write a sibling
// `.tmp` file then rename it over the target, so a crash mid-write never
// leaves a half-written settings.json behind (the target is either the old
// content or the new content, never a truncated mix).
func writeJSONFile(path string, data map[string]any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	b = append(b, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmp, path, err)
	}
	return nil
}

// containsRawclaw recursively reports whether any string value nested inside v
// contains the rawclaw marker — the entry-identity check the merge engine uses
// to find its own rows regardless of how they're nested.
func containsRawclaw(v any) bool {
	switch val := v.(type) {
	case string:
		return strings.Contains(strings.ReplaceAll(val, "\\", "/"), rawclawMarker)
	case map[string]any:
		for _, vv := range val {
			if containsRawclaw(vv) {
				return true
			}
		}
	case []any:
		for _, vv := range val {
			if containsRawclaw(vv) {
				return true
			}
		}
	}
	return false
}

// ensureHooksMap returns data["hooks"] as a map, creating it when absent. A
// PRESENT value of any other type is legal JSON but off-schema — refuse it
// rather than silently discarding whatever the user stored there.
func ensureHooksMap(data map[string]any) (map[string]any, error) {
	if raw, exists := data["hooks"]; exists {
		if hooks, ok := raw.(map[string]any); ok {
			return hooks, nil
		}
		return nil, fmt.Errorf("unexpected hooks shape %T (want an object); refusing to overwrite it", raw)
	}
	hooks := make(map[string]any)
	data["hooks"] = hooks
	return hooks, nil
}

// filterHookArray drops every entry in a hook-event array that references
// rawclaw, keeping every sibling entry (foreign or otherwise) untouched.
func filterHookArray(arr []any) []any {
	filtered := make([]any, 0, len(arr))
	for _, entry := range arr {
		if !containsRawclaw(entry) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// removeRawclawHooks strips every rawclaw-owned entry out of data's Claude
// Code hooks, across every event key present (not just SessionStart, which is
// the only one rawclaw currently installs — a stale entry from a prior rawclaw
// version under a different event key must still be reachable by --eject).
// Empty event arrays are dropped, and the hooks map itself is removed if
// nothing is left in it — this is the "remove-own-entries" half of the
// remove-then-readd merge that makes install idempotent and eject symmetric.
func removeRawclawHooks(data map[string]any) {
	hooks, ok := data["hooks"].(map[string]any)
	if !ok {
		return
	}
	for key, v := range hooks {
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		filtered := filterHookArray(arr)
		if len(filtered) == 0 {
			delete(hooks, key)
		} else {
			hooks[key] = filtered
		}
	}
	if len(hooks) == 0 {
		delete(data, "hooks")
	}
}

// addRawclawSessionStartHook idempotently registers the discovery hook as a
// SessionStart entry: it first removes any existing rawclaw entry (so a
// re-run, or an upgrade from a previous scriptPath, never leaves a duplicate),
// then appends exactly one fresh entry pointing at scriptPath. Every sibling
// hook — on SessionStart or any other event — is left exactly as found.
func addRawclawSessionStartHook(data map[string]any, scriptPath string) error {
	removeRawclawHooks(data)
	hooks, err := ensureHooksMap(data)
	if err != nil {
		return err
	}

	entry := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": scriptPath,
			},
		},
	}
	// Off-schema but legal JSON (SessionStart holding a non-array) must never
	// be silently clobbered — refuse and leave the user's file untouched.
	if raw, exists := hooks["SessionStart"]; exists {
		if _, ok := raw.([]any); !ok {
			return fmt.Errorf("unexpected SessionStart shape %T (want an array); refusing to overwrite it", raw)
		}
	}
	arr, _ := hooks["SessionStart"].([]any)
	hooks["SessionStart"] = append(arr, entry)
	return nil
}

// installRawclawHook writes the hook script and registers it in
// <configDir>/settings.json — the Claude Code target.
func installRawclawHook(configDir string) error {
	return installRawclawHookAt(configDir, settingsPath(configDir))
}

// installRawclawCodexHook writes the (shared) hook script and registers it in
// <configDir>/hooks.json — the Codex target. Same script, same merge engine,
// same SessionStart shape as Claude Code; only the config file differs, since
// Codex's hooks.json and Claude's settings.json agree on the hooks{} shape.
func installRawclawCodexHook(configDir string) error {
	return installRawclawHookAt(configDir, codexHooksPath(configDir))
}

// installRawclawHookAt writes the hook script under configDir and registers it
// as a SessionStart entry in configFile, in that order (an entry pointing at a
// script that doesn't exist yet is the wrong intermediate state to risk if the
// second step fails). Shared by both the Claude Code and Codex targets — they
// differ only in which JSON file the SessionStart entry is merged into.
func installRawclawHookAt(configDir, configFile string) error {
	scriptPath := hookScriptPath(configDir)
	if err := writeHookScript(scriptPath); err != nil {
		return fmt.Errorf("install hook script: %w", err)
	}

	data, err := readJSONFile(configFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", configFile, err)
	}
	if err := addRawclawSessionStartHook(data, scriptPath); err != nil {
		return fmt.Errorf("register hook in %s: %w", configFile, err)
	}
	if err := writeJSONFile(configFile, data); err != nil {
		return fmt.Errorf("write %s: %w", configFile, err)
	}
	return nil
}

// removeIfEmpty removes dir only when it exists and holds nothing at all. A
// directory that still has something in it — a sibling tool's own hook script,
// a settings backup, an unrelated file — is left standing untouched; a missing
// dir is a silent no-op. This is the whole safety story for eject's directory
// cascade: it never needs a special case for "don't delete ~/.claude" because
// ~/.claude (or ~/.codex) holding anything else at all — and on a real machine
// it will, sessions/projects/other settings — already makes it non-empty.
func removeIfEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		_ = os.Remove(dir) // best-effort: a failure just leaves an empty dir behind
	}
}

// writeOrRemoveConfigFile writes data to path, or removes path entirely once
// data has nothing left in it — the config-file half of eject's "delete only
// when nothing else remains" rule. A path that was already missing is not an
// error either way (a second eject, or ejecting a target that was never
// installed, must stay a clean no-op).
func writeOrRemoveConfigFile(path string, data map[string]any) error {
	if len(data) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return writeJSONFile(path, data)
}

// ejectOutcome reports what one target's eject actually touched, so the
// caller can print an accurate line (or a clean "nothing to remove" note)
// instead of always claiming success at full volume.
type ejectOutcome struct {
	scriptPath    string
	configFile    string
	scriptRemoved bool
	entryRemoved  bool
	fileDeleted   bool
}

// didAnything reports whether this target had anything rawclaw-owned to
// remove at all — the signal a fully clean, nothing-installed machine uses to
// print a single no-op message instead of a line per target.
func (o ejectOutcome) didAnything() bool {
	return o.scriptRemoved || o.entryRemoved || o.fileDeleted
}

// ejectRawclawHookAt reverses installRawclawHookAt: remove the hook script
// (and its now-possibly-empty parent dirs), strip rawclaw's own entries out of
// configFile, and delete configFile entirely once nothing else is left in it.
// Every step tolerates the thing already being gone — ejecting a target that
// was never installed (or ejecting twice) is a clean no-op, never an error.
func ejectRawclawHookAt(configDir, configFile string) (ejectOutcome, error) {
	scriptPath := hookScriptPath(configDir)
	scriptDir := filepath.Dir(scriptPath)     // configDir/hooks/rawclaw — ours alone
	hooksParentDir := filepath.Dir(scriptDir) // configDir/hooks — may hold siblings

	out := ejectOutcome{scriptPath: scriptPath, configFile: configFile}

	// Remove exactly the file setup installed — never the whole directory. A
	// user may have parked their own files under hooks/rawclaw; they are not
	// ours to delete, and the dir cascade below only fires when truly empty.
	if _, err := os.Stat(scriptPath); err == nil {
		if err := os.Remove(scriptPath); err != nil {
			return out, fmt.Errorf("remove %s: %w", scriptPath, err)
		}
		out.scriptRemoved = true
	}
	removeIfEmpty(scriptDir)
	removeIfEmpty(hooksParentDir)

	data, err := readJSONFile(configFile)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", configFile, err)
	}
	// Only a config that actually holds a rawclaw entry is rewritten (and only
	// then may it be deleted, once nothing else remains). A file with no
	// rawclaw entry — including a user's pre-existing empty {} — is not ours to
	// rewrite or remove: it stays byte-untouched.
	if containsRawclaw(data["hooks"]) {
		out.entryRemoved = true
		removeRawclawHooks(data)
		if err := writeOrRemoveConfigFile(configFile, data); err != nil {
			return out, err
		}
		out.fileDeleted = len(data) == 0
	}

	removeIfEmpty(configDir)
	return out, nil
}

// ejectRawclawHook reverses installRawclawHook — the Claude Code target.
func ejectRawclawHook(configDir string) (ejectOutcome, error) {
	return ejectRawclawHookAt(configDir, settingsPath(configDir))
}

// ejectRawclawCodexHook reverses installRawclawCodexHook — the Codex target.
func ejectRawclawCodexHook(configDir string) (ejectOutcome, error) {
	return ejectRawclawHookAt(configDir, codexHooksPath(configDir))
}
