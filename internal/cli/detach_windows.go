//go:build windows

package cli

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detach configures cmd to run detached from the parent's console, so closing
// the invoking terminal (or the parent exiting) never takes the child with it
// — the Windows analogue of setsid.
// https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS,
	}
}
