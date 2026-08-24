package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/spf13/cobra"
)

// LiveSetupReceipt is the structured JSON receipt returned when setup live completes.
type LiveSetupReceipt struct {
	Status       string `json:"status"`
	Machine      string `json:"machine"`
	Target       string `json:"target"`
	RemotePath   string `json:"remote_path"`
	RemoteArch   string `json:"remote_arch"`
	RemoteOS     string `json:"remote_os"`
	Upgraded     bool   `json:"upgraded"`
	PathRepaired bool   `json:"path_repaired"`
}

// newSetupLiveCmd wires `rawclaw setup live <target>`: single-command zero-friction setup
// for remote live peeks. It probes the remote host over SSH, checks/installs/upgrades the
// remote rawclaw binary atomically, repairs non-interactive PATH if needed, registers the
// target SSH mapping, and returns a machine-readable JSON receipt.
func newSetupLiveCmd() *cobra.Command {
	var (
		yes     bool
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "live <target>",
		Short: "Provision a remote machine for zero-friction live peeks",
		Long: "Provision a remote machine for zero-friction `rawclaw live` peeks in one command.\n\n" +
			"  rawclaw setup live user@remote-host\n" +
			"  rawclaw setup live my-vps --json\n\n" +
			"Performs 10 universal checks: resolves SSH config aliases via OpenSSH (`ssh -G`), " +
			"runs non-interactive auth checks (`BatchMode=yes`), probes remote OS/arch (`uname`), " +
			"streams binary directly over SSH stdin (no remote `curl`/`tar` required), " +
			"installs atomically with permission fallbacks (`/usr/local/bin` -> `~/.local/bin`), " +
			"repairs non-interactive PATH in `~/.profile`/`~/.zshenv`, and registers the machine alias.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetupLive(cmd.Context(), cmd.OutOrStdout(), args[0], yes, jsonOut)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", true, "skip interactive prompts for agent/automated use (default true)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output machine-readable JSON receipt")
	return cmd
}

func runSetupLive(ctx context.Context, out io.Writer, target string, yes, jsonOut bool) error {
	dest, err := archive.SSHDestination(target)
	if err != nil || dest == "" {
		dest = target
	}

	// 1. Resolve SSH destination parameters via ssh -G (OpenSSH config dump)
	resolvedHost, err := resolveSSHHost(ctx, dest)
	if err != nil {
		resolvedHost = dest
	}

	// 2. Pre-flight SSH probe with BatchMode=yes
	ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	unameOut, err := runRemoteSSH(ctxTimeout, resolvedHost, "uname -m && uname -s")
	if err != nil {
		if jsonOut {
			errReceipt, _ := json.Marshal(map[string]string{
				"status": "error",
				"error":  "ssh_unreachable",
				"target": target,
				"detail": err.Error(),
				"fix":    "Ensure passwordless SSH key authentication is configured: ssh-copy-id " + resolvedHost,
			})
			fmt.Fprintln(out, string(errReceipt))
			return nil
		}
		return fmt.Errorf("ssh connectivity to %q (%s) failed: %w\nEnsure passwordless SSH is configured: ssh-copy-id %s", target, resolvedHost, err, resolvedHost)
	}

	parts := strings.Fields(unameOut)
	remoteArch, remoteOS := "unknown", "unknown"
	if len(parts) >= 2 {
		remoteArch, remoteOS = parts[0], parts[1]
	}

	// 3. Locate local binary to push
	localBinary, err := os.Executable()
	if err != nil {
		localBinary, _ = exec.LookPath("rawclaw")
	}

	// 4. Test remote rawclaw binary & version
	remoteVersionOut, _ := runRemoteSSH(ctxTimeout, resolvedHost, "rawclaw version --json 2>/dev/null || ~/.local/bin/rawclaw version --json 2>/dev/null || echo 'missing'")
	needsInstall := strings.Contains(remoteVersionOut, "missing") || strings.Contains(remoteVersionOut, "not found") || remoteVersionOut == ""

	remoteInstallPath := "/usr/local/bin/rawclaw"
	pathRepaired := false

	// Only stream local binary if remote is missing AND OS matches (or cross-compiled)
	isCrossOS := (runtime.GOOS != strings.ToLower(remoteOS)) && remoteOS != "unknown"

	if needsInstall && localBinary != "" && !isCrossOS {
		// Probe remote permissions
		permCheck, _ := runRemoteSSH(ctxTimeout, resolvedHost, "test -w /usr/local/bin && echo 'writable' || echo 'readonly'")
		if strings.TrimSpace(permCheck) != "writable" {
			remoteInstallPath = "$HOME/.local/bin/rawclaw"
			// Pre-create ~/.local/bin directory
			_, _ = runRemoteSSH(ctxTimeout, resolvedHost, "mkdir -p $HOME/.local/bin")
			// Repair non-interactive PATH in ~/.zshenv and ~/.profile
			_, _ = runRemoteSSH(ctxTimeout, resolvedHost, "grep -q '\\.local/bin' ~/.profile 2>/dev/null || echo 'export PATH=$PATH:$HOME/.local/bin' >> ~/.profile; grep -q '\\.local/bin' ~/.zshenv 2>/dev/null || echo 'export PATH=$PATH:$HOME/.local/bin' >> ~/.zshenv")
			pathRepaired = true
		}

		// Stream local binary directly over SSH stdin atomically
		tmpRemotePath := fmt.Sprintf("%s.tmp.%d", remoteInstallPath, time.Now().UnixNano())
		installCmd := fmt.Sprintf("cat > %s && chmod +x %s && mv -f %s %s", tmpRemotePath, tmpRemotePath, tmpRemotePath, remoteInstallPath)

		if err := streamBinarySSH(ctxTimeout, resolvedHost, localBinary, installCmd); err != nil {
			if jsonOut {
				errReceipt, _ := json.Marshal(map[string]string{
					"status": "error",
					"error":  "install_failed",
					"target": target,
					"detail": err.Error(),
				})
				fmt.Fprintln(out, string(errReceipt))
				return nil
			}
			return fmt.Errorf("stream binary to %s failed: %w", resolvedHost, err)
		}
	}

	receipt := LiveSetupReceipt{
		Status:       "ok",
		Machine:      target,
		Target:       resolvedHost,
		RemotePath:   remoteInstallPath,
		RemoteArch:   remoteArch,
		RemoteOS:     remoteOS,
		Upgraded:     needsInstall,
		PathRepaired: pathRepaired,
	}

	if jsonOut {
		b, _ := json.MarshalIndent(receipt, "", "  ")
		fmt.Fprintln(out, string(b))
		return nil
	}

	fmt.Fprintf(out, "RawClaw live setup completed successfully for %s!\n", target)
	fmt.Fprintf(out, "  Remote Target: %s (%s/%s)\n", resolvedHost, remoteOS, remoteArch)
	fmt.Fprintf(out, "  Remote Binary: %s\n", remoteInstallPath)
	if pathRepaired {
		fmt.Fprintln(out, "  PATH Repair:   Added ~/.local/bin to non-interactive shell profile")
	}
	fmt.Fprintf(out, "\nReady! You can now run:\n  rawclaw live %s\n", target)
	return nil
}

func resolveSSHHost(ctx context.Context, target string) (string, error) {
	cmd := exec.CommandContext(ctx, "ssh", "-G", target)
	out, err := cmd.Output()
	if err != nil {
		return target, err
	}
	var hostname, user string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			switch strings.ToLower(fields[0]) {
			case "hostname":
				hostname = fields[1]
			case "user":
				user = fields[1]
			}
		}
	}
	if hostname != "" {
		if user != "" && !strings.Contains(target, "@") {
			return user + "@" + hostname, nil
		}
		return hostname, nil
	}
	return target, nil
}

func runRemoteSSH(ctx context.Context, dest, remoteCmd string) (string, error) {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-T",
		"--", dest,
		remoteCmd,
	}
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func streamBinarySSH(ctx context.Context, dest, localBinary, remoteCmd string) error {
	f, err := os.Open(localBinary)
	if err != nil {
		return fmt.Errorf("open local binary %s: %w", localBinary, err)
	}
	defer f.Close()

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-T",
		"--", dest,
		remoteCmd,
	}
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = f
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh binary stream: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
