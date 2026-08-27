//go:build unix

package cli

import (
	"os/exec"
	"syscall"
)

func configureCloseoutProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateCloseoutProcess(cmd *exec.Cmd) error {
	if cmd.Process != nil {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return nil
}

func closeoutProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
