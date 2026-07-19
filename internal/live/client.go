package live

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// sshConnectTimeoutSecs bounds the ssh connect phase so a dead machine fails
// in seconds instead of stalling on the OS default.
const sshConnectTimeoutSecs = 10

// runSSHFunc is the ssh seam: one real adapter (the system ssh binary) and
// fakes in unit tests. remoteArgs is the un-quoted remote argv; the adapter
// owns quoting for the POSIX shell ssh hands it to. Remote stdout streams to
// stdout; the returned string is the captured stderr (for classification).
type runSSHFunc func(ctx context.Context, dest string, remoteArgs []string, stdout io.Writer) (string, error)

// Client peeks one remote machine over SSH. Machine is the user-facing name
// (for error stories); Dest is the ssh destination it resolved to.
type Client struct {
	Machine string
	Dest    string
	run     runSSHFunc
}

// NewClient returns a client dialing dest (as resolved from machine) with the
// real ssh adapter.
func NewClient(machine, dest string) *Client {
	return &Client{Machine: machine, Dest: dest, run: runSSH}
}

// listResponseLimit caps how much of a remote session list List buffers
// before parsing. A real list is a few kilobytes; only a broken or hostile
// remote streams more, and the cap stops it from filling local memory.
// (Session peeks stream through unbuffered and need no cap.)
const listResponseLimit = 8 << 20 // 8 MiB

// boundedBuffer collects at most max bytes; the write that would exceed the
// cap is refused, which stops the ssh copy and records the overflow.
type boundedBuffer struct {
	buf        bytes.Buffer
	max        int
	overflowed bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > b.max {
		b.overflowed = true
		return 0, fmt.Errorf("response larger than %d bytes", b.max)
	}
	return b.buf.Write(p)
}

// List invokes the remote rawclaw's serving half and renders its session list:
// a human table by default (remote-computed ages, so clock skew never lies),
// or the raw JSON bytes under jsonOut. limit<=0 uses the serve default.
func (c *Client) List(ctx context.Context, w io.Writer, limit int, jsonOut bool) error {
	args := serveArgs()
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}

	bounded := &boundedBuffer{max: listResponseLimit}
	stderr, err := c.run(ctx, c.Dest, args, bounded)
	// The overflow is the real story: cutting off the stream also makes the
	// underlying ssh run fail, so check the cap before classifying err.
	if bounded.overflowed {
		return fmt.Errorf(
			"machine %q streamed over %d MiB for a session list — refusing to buffer it (broken or untrustworthy remote?)",
			c.Machine, listResponseLimit>>20)
	}
	if err != nil {
		return c.classify(stderr, err)
	}
	buf := &bounded.buf
	if jsonOut {
		if _, werr := w.Write(buf.Bytes()); werr != nil {
			return fmt.Errorf("write session list: %w", werr)
		}
		return nil
	}

	var rows []Session
	if uerr := json.Unmarshal(buf.Bytes(), &rows); uerr != nil {
		return fmt.Errorf("machine %q returned an unreadable session list (mismatched rawclaw versions? run `rawclaw upgrade` on both ends): %w", c.Machine, uerr)
	}
	if len(rows) == 0 {
		fmt.Fprintf(w, "No recent sessions on %s.\n", c.Machine)
		return nil
	}
	fmt.Fprintf(w, "Recent sessions on %s (most recent first):\n\n", c.Machine)
	for _, r := range rows {
		fmt.Fprintf(w, "  %8s  %-6s  %-24s  %s\n",
			FormatAge(r.AgeSeconds), r.Source, orUnknown(r.Project), shortID(r.SessionID))
	}
	fmt.Fprintf(w, "\nPeek one: rawclaw live %s <session-prefix>\n", c.Machine)
	return nil
}

// Session streams the remote rendering of one in-progress session — a one-hop
// read of the live transcript, so messages written seconds ago are included.
// The remote bytes pass through verbatim (text or, under jsonOut, JSON).
// includeTools crosses the pipe only when set: the stripped render is the
// remote's own default, so the default peek stays compatible with remotes
// that predate the flag.
func (c *Client) Session(ctx context.Context, w io.Writer, prefix string, tail int, includeTools, jsonOut bool) error {
	args := serveArgs()
	if tail > 0 {
		args = append(args, "--tail", strconv.Itoa(tail))
	}
	if includeTools {
		args = append(args, "--include-tools")
	}
	if jsonOut {
		args = append(args, "--json")
	}
	// The prefix is user input: it crosses as a POSITIONAL behind a literal
	// "--" so a flag-shaped prefix can never flip the remote's mode.
	args = append(args, "--", prefix)
	stderr, err := c.run(ctx, c.Dest, args, w)
	if err != nil {
		return c.classify(stderr, err)
	}
	return nil
}

// serveArgs is the remote argv every peek starts from. The remote watchdog is
// disarmed (--timeout 0): the run is already bounded on THIS side (the local
// watchdog + ssh), and the remote's own 30s default would kill a long render
// mid-stream with advice — raise --timeout — that no local flag could satisfy.
// A serving process can't outlive its pipe: when the client dies, the next
// write fails and the remote exits.
func serveArgs() []string {
	return []string{"rawclaw", "live", "--serve", "--timeout", "0"}
}

// classify turns an ssh/remote failure into ONE distinct, actionable error:
// unknown machine (the name resolves nowhere), unreachable ssh, missing
// remote rawclaw, too-old remote rawclaw — or the remote's own message.
func (c *Client) classify(stderr string, err error) error {
	stderr = strings.TrimSpace(stderr)
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New("ssh not found on this machine's PATH — live peek dials machines with the system ssh binary")
	}

	var ee *exec.ExitError
	if errors.As(err, &ee) {
		switch ee.ExitCode() {
		case 255: // ssh's own failure, not the remote command's
			if isResolveFailure(stderr) {
				return fmt.Errorf(
					"cannot resolve machine %q (ssh destination %q) — not a known host.\n"+
						"Map it in the archive config (\"ssh\": {%q: \"user@host\"}) or add a Host alias in ~/.ssh/config.\nssh: %s",
					c.Machine, c.Dest, c.Machine, stderr)
			}
			return fmt.Errorf("machine %q (ssh destination %q) is unreachable: %s", c.Machine, c.Dest, stderr)
		case 127: // the remote shell had no rawclaw to run
			return fmt.Errorf(
				"machine %q is reachable but has no rawclaw on its non-interactive PATH.\n"+
					"Install it there:  go install github.com/MoonCaves/rawclaw/cmd/rawclaw@latest\n"+
					"(or drop a release binary on the PATH sshd gives non-interactive commands)\nremote: %s",
				c.Machine, stderr)
		}
		if isTooOldRemote(stderr) {
			return fmt.Errorf(
				"rawclaw on machine %q is too old for live peek — run `rawclaw upgrade` there.\nremote: %s",
				c.Machine, stderr)
		}
	}
	if stderr != "" {
		return fmt.Errorf("live peek of %q failed: %s", c.Machine, stderr)
	}
	return fmt.Errorf("live peek of %q failed: %w", c.Machine, err)
}

// isResolveFailure matches the ssh stderr shapes for "this name is not a
// host": OpenSSH's resolver message plus the glibc/macOS getaddrinfo strings.
func isResolveFailure(stderr string) bool {
	for _, marker := range []string{
		"Could not resolve hostname",
		"Name or service not known",
		"nodename nor servname provided",
	} {
		if strings.Contains(stderr, marker) {
			return true
		}
	}
	return false
}

// isTooOldRemote matches a remote rawclaw that predates the live verb or its
// serve flags. A pre-live rawclaw treats "live" as a search term and trips on
// the flag — verified against a real v0.4.0 build, whose stderr is exactly
// "unknown flag: --serve" (bare; this CLI prints errors without a prefix).
// A remote that knows --serve but predates the tool opt-in fails the same
// way on "unknown flag: --include-tools" (captured from a real pre-flag
// build). The "Error: "-prefixed cobra form is accepted too. Matches are
// anchored to the start of a stderr line so a remote error that merely
// ECHOES marker-shaped user input (a no-match error quoting the prefix) is
// never mistaken for an old binary.
func isTooOldRemote(stderr string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "Error: ")
		if strings.HasPrefix(line, "unknown flag: --serve") ||
			strings.HasPrefix(line, "unknown flag: --include-tools") ||
			strings.HasPrefix(line, `unknown command "live"`) {
			return true
		}
	}
	return false
}

// runSSH is the real adapter: the system ssh binary. BatchMode forbids
// interactive prompts (an agent's tool call must fail fast, never hang on a
// password), ConnectTimeout bounds the dial, -T skips pty allocation, and
// "--" ends option parsing so a hostile destination cannot inject ssh flags.
// The remote argv is single-quoted for the POSIX shell sshd hands it to.
func runSSH(ctx context.Context, dest string, remoteArgs []string, stdout io.Writer) (string, error) {
	return runSSHOpts(ctx, dest, nil, remoteArgs, stdout)
}

// runSSHOpts is runSSH with extra ssh options prepended — the loopback test
// threads an isolated UserKnownHostsFile through here so a test run never
// touches the user's real ssh state.
func runSSHOpts(ctx context.Context, dest string, sshOpts, remoteArgs []string, stdout io.Writer) (string, error) {
	quoted := make([]string, 0, len(remoteArgs))
	for _, a := range remoteArgs {
		quoted = append(quoted, quoteRemoteArg(a))
	}
	args := append([]string{}, sshOpts...)
	args = append(args,
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", sshConnectTimeoutSecs),
		"-T",
		"--", dest,
		strings.Join(quoted, " "),
	)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var errBuf bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return errBuf.String(), err
}

// quoteRemoteArg single-quotes one arg for a POSIX shell, escaping embedded
// single quotes — user input (a session prefix) can never become shell.
func quoteRemoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shortID returns the first 8 runes of the id's final path segment — the
// prefix a follow-up `live <machine> <prefix>` takes.
func shortID(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	r := []rune(id)
	if len(r) <= 8 {
		return id
	}
	return string(r[:8])
}
