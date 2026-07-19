package archive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pristineTransportEnv strips every user transport override the stall-config
// path consults, so tests exercise the default shape hermetically (a
// contributor's own core.sshCommand or GIT_SSH must not steer assertions).
func pristineTransportEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig-empty"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(t.TempDir(), "gitconfig-empty"))
}

// TestGitCommand_StallDetectionConfig: every git invocation carries the
// HTTP(S) stall bounds (-c http.lowSpeedLimit/Time, before the subcommand so
// they apply globally) and a GIT_SSH_COMMAND with keepalives — the bounds
// that replace the wall-clock watchdog on the syncing archive verbs.
func TestGitCommand_StallDetectionConfig(t *testing.T) {
	pristineTransportEnv(t)

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
	pristineTransportEnv(t)
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

// TestGitCommand_RespectsLegacyGITSSH: GIT_SSH_COMMAND outranks the legacy
// GIT_SSH var — so when the user relies on GIT_SSH, rawclaw must NOT set
// GIT_SSH_COMMAND at all (it would silently replace their transport).
func TestGitCommand_RespectsLegacyGITSSH(t *testing.T) {
	pristineTransportEnv(t)
	t.Setenv("GIT_SSH", "/usr/local/bin/customssh")

	cmd := gitCommand(context.Background(), t.TempDir(), "fetch")
	if got := lastEnv(cmd.Env, "GIT_SSH_COMMAND"); got != "" {
		t.Errorf("GIT_SSH_COMMAND forced to %q despite GIT_SSH being set", got)
	}
}

// TestGitCommand_RespectsCoreSSHCommand: same rule for core.sshCommand in git
// config — GIT_SSH_COMMAND outranks it, so rawclaw leaves the env untouched
// when the config carries a transport.
func TestGitCommand_RespectsCoreSSHCommand(t *testing.T) {
	pristineTransportEnv(t)
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte("[core]\n\tsshCommand = ssh -i /keys/special\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	cmd := gitCommand(context.Background(), t.TempDir(), "push")
	if got := lastEnv(cmd.Env, "GIT_SSH_COMMAND"); got != "" {
		t.Errorf("GIT_SSH_COMMAND forced to %q despite core.sshCommand being set", got)
	}
}

// lastEnv returns the LAST value of key in env (the one os/exec uses).
func lastEnv(env []string, key string) string {
	v := ""
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			v = strings.TrimPrefix(e, key+"=")
		}
	}
	return v
}

// TestTransferOp: the transfer verbs (remote-talking, stall-bounded, never
// wall-clock-capped) vs local ops (wall-clock-bounded when the ctx has no
// deadline, so a wedged local git can't hold the sync flock forever).
func TestTransferOp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"push", "origin", "HEAD"}, true},
		{[]string{"pull", "--rebase"}, true},
		{[]string{"clone", "--no-checkout", "url", "dir"}, true},
		{[]string{"fetch", "origin"}, true},
		{[]string{"ls-remote", "origin"}, true},
		{[]string{"add", "-A"}, false},
		{[]string{"commit", "-m", "x"}, false},
		{[]string{"status", "--porcelain"}, false},
		{[]string{"rebase", "--abort"}, false},
		{nil, false},
	}
	for _, tc := range tests {
		if got := transferOp(tc.args); got != tc.want {
			t.Errorf("transferOp(%q) = %v, want %v", tc.args, got, tc.want)
		}
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
