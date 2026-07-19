//go:build unix

package cli

import (
	"os/exec"
	"syscall"
)

// detach configures cmd to run in its own session (setsid): the child leaves
// the parent's process group, so terminal signals (and any group signal aimed
// at the parent) can never reach it, and it survives the parent exiting. Pair
// with a bare exec.Command — never CommandContext, whose cancel would kill the
// child and defeat the detachment.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
