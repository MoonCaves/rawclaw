package live

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// fakeRun builds a runSSHFunc that records the invocation and plays back a
// canned stdout/stderr/err.
func fakeRun(calls *[][]string, stdout, stderr string, err error) runSSHFunc {
	return func(_ context.Context, dest string, remoteArgs []string, w io.Writer) (string, error) {
		*calls = append(*calls, append([]string{dest}, remoteArgs...))
		if stdout != "" {
			if _, werr := io.WriteString(w, stdout); werr != nil {
				return stderr, werr
			}
		}
		return stderr, err
	}
}

const listJSON = `[
  {"session_id":"aaaa1111-0000-0000-0000-000000000001","source":"claude","project":"proj-a","cwd":"/home/u/proj-a","last_activity":"2026-07-19T10:00:00Z","age_seconds":30,"size_bytes":2048},
  {"session_id":"cccc3333-0000-0000-0000-000000000003","source":"codex","project":"proj-c","last_activity":"2026-07-19T09:55:00Z","age_seconds":300,"size_bytes":512}
]`

// TestClientList_RendersHuman: the client invokes the remote serving half and
// renders the JSON rows as a human list — remote-computed ages, source,
// project, and the 8-char prefix to peek with.
func TestClientList_RendersHuman(t *testing.T) {
	t.Parallel()
	var calls [][]string
	c := &Client{Machine: "box-a", Dest: "user@10.0.0.5", run: fakeRun(&calls, listJSON, "", nil)}

	var buf bytes.Buffer
	if err := c.List(context.Background(), &buf, 0, false); err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("remote invocations = %d, want 1", len(calls))
	}
	got := strings.Join(calls[0], " ")
	for _, want := range []string{"user@10.0.0.5", "rawclaw", "live", "--serve", "--timeout 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("remote invocation %q missing %q", got, want)
		}
	}

	out := buf.String()
	for _, want := range []string{"box-a", "30s ago", "5m ago", "claude", "codex", "proj-a", "aaaa1111", "cccc3333"} {
		if !strings.Contains(out, want) {
			t.Errorf("list render missing %q:\n%s", want, out)
		}
	}
	// The rows must keep the remote order (most recent first).
	if strings.Index(out, "aaaa1111") > strings.Index(out, "cccc3333") {
		t.Errorf("rows out of order:\n%s", out)
	}
}

// TestClientList_JSONPassthrough: --json hands the remote's structured bytes
// through untouched — the pipe format IS the contract.
func TestClientList_JSONPassthrough(t *testing.T) {
	t.Parallel()
	var calls [][]string
	c := &Client{Machine: "box-a", Dest: "box-a", run: fakeRun(&calls, listJSON, "", nil)}

	var buf bytes.Buffer
	if err := c.List(context.Background(), &buf, 0, true); err != nil {
		t.Fatalf("List --json: %v", err)
	}
	if buf.String() != listJSON {
		t.Errorf("json passthrough altered the bytes:\ngot:  %s\nwant: %s", buf.String(), listJSON)
	}
}

// TestClientList_Empty: an empty remote list renders the "nothing recent"
// story, not silence.
func TestClientList_Empty(t *testing.T) {
	t.Parallel()
	var calls [][]string
	c := &Client{Machine: "box-a", Dest: "box-a", run: fakeRun(&calls, "[]\n", "", nil)}

	var buf bytes.Buffer
	if err := c.List(context.Background(), &buf, 0, false); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(buf.String(), "No recent sessions") {
		t.Errorf("empty list render = %q, want a 'No recent sessions' line", buf.String())
	}
}

// TestClientSession_StreamsAndPassesArgs: the session peek passes the prefix,
// tail, and json flags to the remote serving half and streams its stdout
// verbatim.
func TestClientSession_StreamsAndPassesArgs(t *testing.T) {
	t.Parallel()
	var calls [][]string
	c := &Client{Machine: "box-a", Dest: "box-a", run: fakeRun(&calls, "rendered transcript\n", "", nil)}

	var buf bytes.Buffer
	if err := c.Session(context.Background(), &buf, "aaaa1111", 7, false); err != nil {
		t.Fatalf("Session: %v", err)
	}
	if buf.String() != "rendered transcript\n" {
		t.Errorf("stream = %q, want the remote bytes verbatim", buf.String())
	}
	got := strings.Join(calls[0], " ")
	for _, want := range []string{"--serve", "--tail 7", "--timeout 0", "-- aaaa1111"} {
		if !strings.Contains(got, want) {
			t.Errorf("remote invocation %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "--json") {
		t.Errorf("remote invocation %q should not carry --json", got)
	}
}

// TestClientSession_FlagShapedPrefix: a prefix that looks like a flag crosses
// the pipe as a POSITIONAL (behind `--`) — it must never flip the remote into
// a different mode.
func TestClientSession_FlagShapedPrefix(t *testing.T) {
	t.Parallel()
	var calls [][]string
	c := &Client{Machine: "box-a", Dest: "box-a", run: fakeRun(&calls, "", "", nil)}
	if err := c.Session(context.Background(), io.Discard, "--json", 0, false); err != nil {
		t.Fatalf("Session: %v", err)
	}
	got := calls[0]
	sep := -1
	for i, a := range got {
		if a == "--" {
			sep = i
		}
	}
	if sep < 0 || sep+1 >= len(got) || got[sep+1] != "--json" {
		t.Errorf("remote invocation %v must carry the prefix after a literal --", got)
	}
}

// exitErr manufactures a real *exec.ExitError with the given code — the only
// way to build one is to actually exit with it.
func exitErr(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	if err == nil {
		t.Fatal("expected non-nil exit error")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("not an ExitError: %v", err)
	}
	return err
}

// TestClientErrors: each degradation is a DISTINCT, actionable error — unknown
// machine name, unreachable ssh, missing remote binary, too-old remote.
func TestClientErrors(t *testing.T) {
	tests := []struct {
		name      string
		exitCode  int
		stderr    string
		wantParts []string
		notWanted string
	}{
		{
			name:      "unknown machine (hostname does not resolve)",
			exitCode:  255,
			stderr:    "ssh: Could not resolve hostname box-x: nodename nor servname provided, or not known",
			wantParts: []string{"box-x", "resolve", "archive config", ".ssh/config"},
			notWanted: "go install",
		},
		{
			name:      "unreachable ssh",
			exitCode:  255,
			stderr:    "ssh: connect to host 10.0.0.5 port 22: Connection refused",
			wantParts: []string{"box-x", "unreachable", "Connection refused"},
			notWanted: "go install",
		},
		{
			name:      "missing remote rawclaw",
			exitCode:  127,
			stderr:    "sh: rawclaw: command not found",
			wantParts: []string{"box-x", "rawclaw", "go install github.com/MoonCaves/rawclaw/cmd/rawclaw@latest", "PATH"},
			notWanted: "unreachable",
		},
		{
			// The REAL pre-live shape, captured from a v0.4.0 build: bare
			// "unknown flag" (this CLI prints errors without a prefix).
			name:      "remote rawclaw too old for live",
			exitCode:  1,
			stderr:    "unknown flag: --serve",
			wantParts: []string{"box-x", "too old", "upgrade"},
			notWanted: "go install",
		},
		{
			name:      "remote too old, cobra-prefixed form",
			exitCode:  1,
			stderr:    `Error: unknown command "live" for "rawclaw"`,
			wantParts: []string{"box-x", "too old", "upgrade"},
			notWanted: "go install",
		},
		{
			name:     "remote error echoing a marker-shaped prefix is NOT too-old",
			exitCode: 1,
			stderr: `no session on this machine matches "unknown flag: --serve" ` +
				`— drop the prefix to list recent sessions`,
			wantParts: []string{"box-x", "no session on this machine"},
			notWanted: "too old",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls [][]string
			c := &Client{
				Machine: "box-x", Dest: "box-x",
				run: fakeRun(&calls, "", tt.stderr, exitErr(t, tt.exitCode)),
			}
			err := c.List(context.Background(), io.Discard, 0, false)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			for _, want := range tt.wantParts {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err.Error(), want)
				}
			}
			if tt.notWanted != "" && strings.Contains(err.Error(), tt.notWanted) {
				t.Errorf("error %q should not contain %q (errors must stay distinct)", err.Error(), tt.notWanted)
			}
		})
	}
}

// TestQuoteRemoteArg: the remote command line crosses a POSIX shell — every
// arg is single-quoted, embedded quotes escaped, so a hostile prefix cannot
// inject shell.
func TestQuoteRemoteArg(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"plain", "'plain'"},
		{"with space", "'with space'"},
		{"a'b", `'a'\''b'`},
		{"$(rm -rf /)", `'$(rm -rf /)'`},
	}
	for _, tt := range tests {
		if got := quoteRemoteArg(tt.in); got != tt.want {
			t.Errorf("quoteRemoteArg(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
