//go:build windows

package cli

import "os/exec"

func configureCloseoutProcess(cmd *exec.Cmd) {}

func terminateCloseoutProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
