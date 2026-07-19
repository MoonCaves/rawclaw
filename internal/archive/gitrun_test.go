package archive

import (
	"context"
	"strings"
	"testing"
)

// TestGitCommand_StallDetectionConfig: every git invocation carries the
// HTTP(S) stall bounds (-c http.lowSpeedLimit/Time, before the subcommand so
// they apply globally) and a GIT_SSH_COMMAND with keepalives — the bounds
// that replace the wall-clock watchdog on the syncing archive verbs.
func TestGitCommand_StallDetectionConfig(t *testing.T) {
	t.Setenv("GIT_SSH_COMMAND", "") // pristine environment for the default shape

	cmd := gitCommand(context.Background(), t.TempDir(), "push", "origin", "HEAD")

	got := strings.Join(cmd.Args, " ")
	want := "git -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=30 push origin HEAD"
	if got != want {
		t.Errorf("git argv = %q, want %q", got, want)
	}

	// os/exec keeps the LAST duplicate env entry, so ours must be last.
	var ssh string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			ssh = strings.TrimPrefix(e, "GIT_SSH_COMMAND=")
		}
	}
	want = "ssh -o ServerAliveInterval=15 -o ServerAliveCountMax=4"
	if ssh != want {
		t.Errorf("GIT_SSH_COMMAND = %q, want %q", ssh, want)
	}
}

// TestGitCommand_RespectsUserSSHCommand: a user's own GIT_SSH_COMMAND is kept
// as the base — the keepalives are appended, and ssh's first-value-wins rule
// keeps any option the user already set authoritative.
func TestGitCommand_RespectsUserSSHCommand(t *testing.T) {
	t.Setenv("GIT_SSH_COMMAND", "ssh -i /keys/archive -o ServerAliveInterval=5")

	cmd := gitCommand(context.Background(), t.TempDir(), "fetch")

	var ssh string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			ssh = strings.TrimPrefix(e, "GIT_SSH_COMMAND=")
		}
	}
	want := "ssh -i /keys/archive -o ServerAliveInterval=5 -o ServerAliveInterval=15 -o ServerAliveCountMax=4"
	if ssh != want {
		t.Errorf("GIT_SSH_COMMAND = %q, want %q", ssh, want)
	}
}

// TestGitSSHCommand: the builder itself — empty/whitespace falls back to bare
// ssh; a user command is preserved verbatim as the base.
func TestGitSSHCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		existing string
		want     string
	}{
		{"empty falls back to ssh", "", "ssh " + sshKeepalives},
		{"whitespace falls back to ssh", "  ", "ssh " + sshKeepalives},
		{"user command kept as base", "ssh -p 2222", "ssh -p 2222 " + sshKeepalives},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := gitSSHCommand(tc.existing); got != tc.want {
				t.Errorf("gitSSHCommand(%q) = %q, want %q", tc.existing, got, tc.want)
			}
		})
	}
}
