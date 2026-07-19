package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeLiveSession writes a minimal top-level Claude transcript for live tests.
func writeLiveSession(t *testing.T, root, project, id, cwd, text string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","cwd":"` + cwd + `","timestamp":"` +
		time.Now().UTC().Format(time.RFC3339) + `","uuid":"` + id + `-u0","message":{"role":"user","content":"` + text + `"}}` + "\n"
	p := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLiveServeList: `rawclaw live --serve` is the remote-invoked serving half
// — a JSON session list over stdout.
func TestLiveServeList(t *testing.T) {
	home := newArchiveHome(t)
	root := filepath.Join(home, ".claude", "projects")
	writeLiveSession(t, root, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", "hello from a")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "--serve")
	if err != nil {
		t.Fatalf("live --serve: %v\n%s", err, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("serve list is not JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0]["session_id"] != "aaaa1111-0000-0000-0000-000000000001" {
		t.Errorf("serve rows = %v", rows)
	}
}

// TestLiveServeSession: `rawclaw live --serve <prefix>` renders the live
// transcript for the ssh pipe.
func TestLiveServeSession(t *testing.T) {
	home := newArchiveHome(t)
	root := filepath.Join(home, ".claude", "projects")
	writeLiveSession(t, root, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", "the freshest message")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "--serve", "aaaa1111")
	if err != nil {
		t.Fatalf("live --serve <prefix>: %v\n%s", err, out)
	}
	if !strings.Contains(out, "the freshest message") {
		t.Errorf("serve render missing the message:\n%s", out)
	}
}

// TestLiveServeSession_ToolPosture: the serve render follows the same default
// display posture as read/outline — tool calls stripped unless --include-tools
// asks for them.
func TestLiveServeSession_ToolPosture(t *testing.T) {
	home := newArchiveHome(t)
	root := filepath.Join(home, ".claude", "projects")
	writeLiveSession(t, root, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", `working [TOOL:Bash] \"command\" \"go vet\" [TOOL_RESULT] clean`)

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "--serve", "aaaa1111")
	if err != nil {
		t.Fatalf("live --serve <prefix>: %v\n%s", err, out)
	}
	if strings.Contains(out, "[TOOL") || strings.Contains(out, "go vet") {
		t.Errorf("default serve render leaked tool content:\n%s", out)
	}
	if !strings.Contains(out, "working") {
		t.Errorf("default serve render dropped conversation text:\n%s", out)
	}

	out, err = runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "--serve", "--include-tools", "aaaa1111")
	if err != nil {
		t.Fatalf("live --serve --include-tools <prefix>: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[TOOL:Bash]") || !strings.Contains(out, "go vet") {
		t.Errorf("--include-tools serve render missing tool content:\n%s", out)
	}
}

// TestLiveArgs: client mode needs a machine (1-2 args); serve mode takes at
// most a prefix.
func TestLiveArgs(t *testing.T) {
	newArchiveHome(t)

	if _, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live"); err == nil {
		t.Error("live with no machine should be a usage error")
	}
	if _, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "m", "p", "extra"); err == nil {
		t.Error("live with 3 args should be a usage error")
	}
	if _, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "--serve", "p", "extra"); err == nil {
		t.Error("live --serve with 2 args should be a usage error")
	}
}

// TestLiveCrossModeFlags: a flag on the wrong mode errors loudly instead of
// being silently ignored.
func TestLiveCrossModeFlags(t *testing.T) {
	newArchiveHome(t)

	if _, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "--serve", "--tail", "5"); err == nil {
		t.Error("--tail on a list should be a usage error")
	}
	if _, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "--serve", "somepfx", "--limit", "5"); err == nil {
		t.Error("--limit on a session peek should be a usage error")
	}
	if _, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "--serve", "--include-tools"); err == nil {
		t.Error("--include-tools on a list should be a usage error")
	}
	if _, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "live", "some-machine", "--include-tools"); err == nil {
		t.Error("--include-tools on a client list should be a usage error")
	}
}
