package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeQueueHome pins HOME to a temp dir so tagQueuePath lands in an isolated
// state dir — the queue file must never touch the developer's real cache.
func fakeQueueHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// TestTagQueueAddListRemoveRoundTrip: add two ids, list them oldest-first as
// 8-char prefixes, remove one by prefix, and see the survivor alone.
func TestTagQueueAddListRemoveRoundTrip(t *testing.T) {
	fakeQueueHome(t)

	a := "aaaabbbb-1111-2222-3333-444455556666"
	b := "ccccdddd-7777-8888-9999-000011112222"
	for _, id := range []string{a, b} {
		if err := tagQueueAdd(id); err != nil {
			t.Fatalf("add %s: %v", id, err)
		}
	}

	ids, err := readTagQueue()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != a || ids[1] != b {
		t.Fatalf("queue = %v, want [%s %s] oldest first", ids, a, b)
	}

	removed, err := tagQueueRemove("aaaabbbb")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("prefix remove reported nothing removed")
	}
	ids, err = readTagQueue()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != b {
		t.Fatalf("queue after remove = %v, want [%s]", ids, b)
	}
}

// TestTagQueueAddIsIdempotent: re-adding a queued id must not duplicate it.
func TestTagQueueAddIsIdempotent(t *testing.T) {
	fakeQueueHome(t)

	id := "aaaabbbb-1111-2222-3333-444455556666"
	for range 3 {
		if err := tagQueueAdd(id); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := readTagQueue()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("queue = %v, want exactly one entry", ids)
	}
}

// TestTagQueueAddRejectsGarbage: the hook parses stdin with a tolerant sed
// scan, so the add path is the gate against a malformed payload smuggling
// arbitrary bytes into the queue file.
func TestTagQueueAddRejectsGarbage(t *testing.T) {
	fakeQueueHome(t)

	for _, bad := range []string{"", "a b", "x\ny", "id;rm -rf /", "päth"} {
		if err := tagQueueAdd(bad); err == nil {
			t.Errorf("add(%q) accepted, want a rejection", bad)
		}
	}
	if _, err := os.Stat(tagQueuePath()); !os.IsNotExist(err) {
		t.Errorf("queue file created despite only invalid adds (err=%v)", err)
	}
}

// TestTagQueueEmptiedFileIsDeleted: removing the last entry deletes the queue
// file instead of leaving a zero-byte husk (the SessionStart hook keys its
// pending note on output, but a lingering empty file is still clutter).
func TestTagQueueEmptiedFileIsDeleted(t *testing.T) {
	fakeQueueHome(t)

	id := "aaaabbbb-1111-2222-3333-444455556666"
	if err := tagQueueAdd(id); err != nil {
		t.Fatal(err)
	}
	if _, err := tagQueueRemove(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tagQueuePath()); !os.IsNotExist(err) {
		t.Errorf("queue file still present after last remove (err=%v)", err)
	}
}

// TestTagQueueRemoveNoMatch: removing an unknown id reports no removal and
// leaves the queue intact.
func TestTagQueueRemoveNoMatch(t *testing.T) {
	fakeQueueHome(t)

	if err := tagQueueAdd("aaaabbbb-1111-2222-3333-444455556666"); err != nil {
		t.Fatal(err)
	}
	removed, err := tagQueueRemove("zzzzzzzz")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("remove of an unknown id reported a removal")
	}
	ids, _ := readTagQueue()
	if len(ids) != 1 {
		t.Fatalf("queue mutated by a no-match remove: %v", ids)
	}
}

// TestTagQueueCmdListsSession8AndJSON: the bare verb prints one 8-char id per
// line (nothing at all for an empty queue); --json carries both forms.
func TestTagQueueCmdListsSession8AndJSON(t *testing.T) {
	fakeQueueHome(t)

	out, err := runCmd(t, newTagQueueCmd(), "")
	if err != nil {
		t.Fatalf("tag-queue: %v", err)
	}
	if out != "" {
		t.Fatalf("empty queue printed %q, want no output", out)
	}

	id := "aaaabbbb-1111-2222-3333-444455556666"
	if err := tagQueueAdd(id); err != nil {
		t.Fatal(err)
	}

	out, err = runCmd(t, newTagQueueCmd(), "")
	if err != nil {
		t.Fatalf("tag-queue: %v", err)
	}
	if strings.TrimSpace(out) != "aaaabbbb" {
		t.Fatalf("list printed %q, want the 8-char prefix alone", out)
	}

	out, err = runCmd(t, newTagQueueCmd(), "", "--json")
	if err != nil {
		t.Fatalf("tag-queue --json: %v", err)
	}
	var payload struct {
		Pending []struct {
			SessionID string `json:"session_id"`
			Session8  string `json:"session8"`
		} `json:"pending"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad JSON %q: %v", out, err)
	}
	if len(payload.Pending) != 1 || payload.Pending[0].SessionID != id || payload.Pending[0].Session8 != "aaaabbbb" {
		t.Fatalf("json payload = %+v", payload)
	}
}

// TestSetupInstallsSessionEndHook: the Claude Code target gets BOTH scripts
// and BOTH event entries; eject removes both and leaves nothing rawclaw-owned.
func TestSetupInstallsSessionEndHook(t *testing.T) {
	dir := t.TempDir()
	cf := settingsPath(dir)

	if err := installRawclawHookAt(dir, cf, true, rawclawPrimeScript); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, p := range []string{hookScriptPath(dir), tagQueueScriptPath(dir)} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("script missing after install: %v", err)
		}
	}
	b, err := os.ReadFile(tagQueueScriptPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "tag-queue add") {
		t.Errorf("tagqueue.sh does not call `rawclaw tag-queue add`:\n%s", b)
	}
	// The transcript-existence gate: an ephemeral session (SessionEnd fired but
	// no transcript file ever written) must not reach the queue.
	if !strings.Contains(string(b), `"transcript_path"`) || !strings.Contains(string(b), `-f "$transcript_path"`) {
		t.Errorf("tagqueue.sh missing the transcript-file existence gate:\n%s", b)
	}

	data, err := readJSONFile(cf)
	if err != nil {
		t.Fatal(err)
	}
	hooks := data["hooks"].(map[string]any)
	for _, event := range []string{"SessionStart", "SessionEnd"} {
		arr, ok := hooks[event].([]any)
		if !ok || len(arr) != 1 || !containsRawclaw(arr[0]) {
			t.Errorf("%s entry wrong after install: %#v", event, hooks[event])
		}
	}

	out, err := ejectRawclawHookAt(dir, cf)
	if err != nil {
		t.Fatalf("eject: %v", err)
	}
	if !out.scriptRemoved || !out.tagScriptRemoved {
		t.Errorf("eject outcome = %+v, want both scripts removed", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks")); !os.IsNotExist(err) {
		t.Errorf("hooks dir survived a full eject (err=%v)", err)
	}
}

// TestSetupCodexGetsNoSessionEndHook: the Codex target wires SessionStart
// only — SessionEnd is unverified on Codex's event surface.
func TestSetupCodexGetsNoSessionEndHook(t *testing.T) {
	dir := t.TempDir()
	cf := codexHooksPath(dir)

	if err := installRawclawHookAt(dir, cf, false, rawclawPrimeScript); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(tagQueueScriptPath(dir)); !os.IsNotExist(err) {
		t.Errorf("tagqueue.sh written for the Codex target (err=%v)", err)
	}
	data, err := readJSONFile(cf)
	if err != nil {
		t.Fatal(err)
	}
	hooks := data["hooks"].(map[string]any)
	if _, has := hooks["SessionEnd"]; has {
		t.Errorf("SessionEnd registered for Codex: %#v", hooks)
	}
	if _, has := hooks["SessionStart"]; !has {
		t.Errorf("SessionStart missing for Codex: %#v", hooks)
	}
}
