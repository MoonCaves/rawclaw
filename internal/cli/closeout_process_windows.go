//go:build windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
)

func configureCloseoutProcess(cmd *exec.Cmd) {}

func terminateCloseoutProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
		_ = cmd.Process.Kill()
	}
}

func closeoutProcessAlive(pid int) bool {
	return pid == os.Getpid()
}
