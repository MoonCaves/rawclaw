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

// runGit is the real adapter: the system git binary via exec. Terminal
// credential prompts are disabled — a push against a remote that wants
// interactive auth must fail fast, never hang an agent's tool call. LC_ALL=C
// pins git's message locale so output classification (rejected pushes,
// missing remote refs) never breaks on a translated message.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
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
