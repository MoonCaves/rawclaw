package archive

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// runGitFunc is the git seam: one real adapter (system git via exec) and fakes
// in unit tests. dir is the working directory; the combined output is returned
// even on error so callers can classify failures (e.g. rejected pushes).
type runGitFunc func(ctx context.Context, dir string, args ...string) (string, error)

// Stall detection on transfers — the industry-standard posture (rsync
// --timeout, curl --speed-limit/--speed-time): no wall-clock cap fits both a
// hung transfer and a legitimate slow multi-GB first push, so a transfer dies
// when it stops MOVING, and runs as long as it keeps moving. The archive verbs
// run with the CLI's wall-clock watchdog disabled and rely on these bounds.
const (
	// HTTP(S) remotes: git aborts a transfer under lowSpeedLimit bytes/sec
	// sustained for lowSpeedTime seconds.
	gitLowSpeedLimit = "http.lowSpeedLimit=1000"
	gitLowSpeedTime  = "http.lowSpeedTime=30"

	// SSH remotes: keepalive probes every 15s, dead after 4 misses (~60s) —
	// git itself has no stall detection over ssh, the transport supplies it.
	sshKeepalives = "-o ServerAliveInterval=15 -o ServerAliveCountMax=4"
)

// gitSSHCommand builds the GIT_SSH_COMMAND carrying the keepalive options. A
// user's own GIT_SSH_COMMAND is respected: the keepalives are appended to it,
// and ssh's first-obtained-value-wins rule keeps any option the user already
// set authoritative over ours. Assumes an OpenSSH-compatible CLI (-o syntax) —
// git's own default; an exotic transport wrapper may need its own stall story.
func gitSSHCommand(existing string) string {
	base := strings.TrimSpace(existing)
	if base == "" {
		base = "ssh"
	}
	return base + " " + sshKeepalives
}

// gitCommand builds the exec.Cmd runGit runs: the stall-detection -c configs
// prepended to args, and the keepalive GIT_SSH_COMMAND layered over the
// environment (os/exec keeps the LAST duplicate env entry, so appending
// overrides an inherited value while gitSSHCommand preserves its content).
func gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	full := append([]string{"-c", gitLowSpeedLimit, "-c", gitLowSpeedTime}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
		"GIT_SSH_COMMAND="+gitSSHCommand(os.Getenv("GIT_SSH_COMMAND")),
	)
	return cmd
}

// runGit is the real adapter: the system git binary via exec. Terminal
// credential prompts are disabled — a push against a remote that wants
// interactive auth must fail fast, never hang an agent's tool call. LC_ALL=C
// pins git's message locale so output classification (rejected pushes,
// missing remote refs) never breaks on a translated message.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := gitCommand(ctx, dir, args...)
	// Context cancellation delivers SIGTERM, not the default SIGKILL: git's
	// signal handler removes its lock files (.git/index.lock) on the way out,
	// so a cancelled run cannot strand a lock that wedges every later one.
	// WaitDelay is the backstop for a git that ignores the signal (and for
	// platforms where Signal is unsupported).
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(out))
	}
	return out, nil
}
