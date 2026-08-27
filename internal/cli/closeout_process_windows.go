//go:build windows

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func configureCloseoutProcess(cmd *exec.Cmd) {}

func terminateCloseoutProcess(cmd *exec.Cmd) error {
	var treeErr error
	if cmd.Process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		treeErr = exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
		cancel()
		killErr := cmd.Process.Kill()
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return killErr
		}
	}
	return treeErr
}
