package live

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loopbackSSHOpts pins the host-key state to a per-test file: the probe and
// every dial share it, and a test run never touches ~/.ssh/known_hosts.
func loopbackSSHOpts(t *testing.T) []string {
	t.Helper()
	return []string{
		"-o", "UserKnownHostsFile=" + filepath.Join(t.TempDir(), "known_hosts"),
		"-o", "StrictHostKeyChecking=accept-new",
	}
}

// sshToLocalhostOK probes whether this machine can ssh itself non-interactively
// — the loopback "remote" the end-to-end test drives. BatchMode forbids
// prompts, so a machine without key-auth'd sshd skips rather than hangs.
func sshToLocalhostOK(t *testing.T, sshOpts []string) bool {
	t.Helper()
	if _, err := exec.LookPath("ssh"); err != nil {
		return false
	}
	args := append([]string{}, sshOpts...)
	args = append(args,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=3",
		"localhost", "true")
	return exec.Command("ssh", args...).Run() == nil
}

// buildRawclaw compiles the real binary the loopback "remote" runs.
func buildRawclaw(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "rawclaw")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/MoonCaves/rawclaw/cmd/rawclaw")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// repoRoot resolves the module root via the go tool — robust against this
// package ever moving depth.
func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// TestLoopbackLivePeek drives the WHOLE live path against a loopback remote:
// real ssh to localhost, a real rawclaw binary serving, real quoting and
// stream plumbing. The remote env is pinned by a wrapper script so the fixture
// corpus — not this machine's — is what gets served. Skips (with the reason)
// when localhost isn't ssh-able non-interactively; release verification
// against a genuinely separate machine happens outside the unit suite.
func TestLoopbackLivePeek(t *testing.T) {
	if testing.Short() {
		t.Skip("loopback ssh test skipped in -short mode")
	}
	sshOpts := loopbackSSHOpts(t)
	if !sshToLocalhostOK(t, sshOpts) {
		t.Skip("skipping: ssh to localhost is not available non-interactively (BatchMode probe failed)")
	}
	bin := buildRawclaw(t)

	// Fixture corpus for the "remote" — NOT this process's env: the remote
	// process only sees what the wrapper script exports.
	remoteHome := t.TempDir()
	claudeRoot := filepath.Join(remoteHome, ".claude", "projects")
	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionFile := writeLoopbackSession(t, claudeRoot, "-proj-live",
		"abcd1234-0000-0000-0000-00000000cafe", "/home/u/proj-live", "opening message")

	wrapper := filepath.Join(t.TempDir(), "rawclaw-remote.sh")
	script := "#!/bin/sh\n" +
		"HOME=" + quoteRemoteArg(remoteHome) + " " +
		"CLAUDE_CONFIG_DIR=" + quoteRemoteArg(filepath.Join(remoteHome, ".claude")) + " " +
		"CODEX_HOME=" + quoteRemoteArg(filepath.Join(remoteHome, ".codex")) + " " +
		"exec " + quoteRemoteArg(bin) + " \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	c := NewClient("loopback", "localhost")
	// Fixture seam: same real ssh adapter, argv[0] swapped from the PATH-found
	// "rawclaw" to the pinned wrapper, host-key state pinned to the temp file.
	c.run = func(ctx context.Context, dest string, remoteArgs []string, w io.Writer) (string, error) {
		remoteArgs = append([]string{wrapper}, remoteArgs[1:]...)
		return runSSHOpts(ctx, dest, sshOpts, remoteArgs, w)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// List: the fixture session shows up, newest first, via real ssh.
	var list bytes.Buffer
	if err := c.List(ctx, &list, 0, false); err != nil {
		t.Fatalf("loopback List: %v", err)
	}
	if !strings.Contains(list.String(), "abcd1234") || !strings.Contains(list.String(), "proj-live") {
		t.Errorf("loopback list missing the fixture session:\n%s", list.String())
	}

	// JSON passthrough over the real pipe.
	var listJSONBuf bytes.Buffer
	if err := c.List(ctx, &listJSONBuf, 0, true); err != nil {
		t.Fatalf("loopback List --json: %v", err)
	}
	var rows []Session
	if err := json.Unmarshal(listJSONBuf.Bytes(), &rows); err != nil {
		t.Fatalf("loopback json not parseable: %v\n%s", err, listJSONBuf.String())
	}

	// Freshness: append a message NOW, then peek — the seconds-old write must
	// be in the rendered transcript (the whole point of the direct path).
	appendLoopbackMessage(t, sessionFile, "assistant", "written seconds ago over loopback")
	var sess bytes.Buffer
	if err := c.Session(ctx, &sess, "abcd1234", 0, false, false); err != nil {
		t.Fatalf("loopback Session: %v", err)
	}
	for _, want := range []string{"opening message", "written seconds ago over loopback"} {
		if !strings.Contains(sess.String(), want) {
			t.Errorf("loopback session render missing %q:\n%s", want, sess.String())
		}
	}

	// A bad prefix travels back as the serving half's distinct error.
	err := c.Session(ctx, io.Discard, "ffffffff", 0, false, false)
	if err == nil || !strings.Contains(err.Error(), "ffffffff") {
		t.Errorf("loopback bad-prefix error = %v, want the remote no-match story", err)
	}
}

// writeLoopbackSession writes one-line Claude transcript fixtures without
// t.Setenv (the remote process reads them via the wrapper env, not ours).
func writeLoopbackSession(t *testing.T, root, project, id, cwd, text string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+".jsonl")
	writeLoopbackLine(t, p, "user", cwd, text, false)
	return p
}

// appendLoopbackMessage appends one message line to an existing transcript.
func appendLoopbackMessage(t *testing.T, path, role, text string) {
	t.Helper()
	writeLoopbackLine(t, path, role, "", text, true)
}

func writeLoopbackLine(t *testing.T, path, role, cwd, text string, appendTo bool) {
	t.Helper()
	rec := map[string]any{
		"type":      role,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"uuid":      role + "-" + text[:4],
		"message":   map[string]any{"role": role, "content": text},
	}
	if cwd != "" {
		rec["cwd"] = cwd
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	flags := os.O_CREATE | os.O_WRONLY
	if appendTo {
		flags |= os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}
