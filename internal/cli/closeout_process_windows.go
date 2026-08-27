//go:build windows

package cli

import (
	"fmt"
	"os/exec"

	"golang.org/x/sys/windows"
)

func configureCloseoutProcess(cmd *exec.Cmd) {}

func terminateCloseoutProcess(cmd *exec.Cmd) error {
	if cmd.Process != nil {
		if err := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(cmd.Process.Pid)).Run(); err != nil {
			return err
		}
	}
	return nil
}

func closeoutProcessAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(h)
	return true
}
