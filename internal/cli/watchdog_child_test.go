//go:build unix

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestWatchdog_ChildDoesNotOutliveDeadline: the watchdog's exit(124) is the
// never-hang guarantee for the rawclaw process itself — but a child process
// started for the run (the ssh a live peek dials) must die with it, not
// survive as an orphan holding the remote session open. The helper process
// below wires a child exactly the way production does and lets the watchdog
// fire; the child must be gone once the helper has exited.
//
// Deliberately not parallel: the helper's deadline is wall-clock, and a loaded
// runner racing sibling tests could delay the child's Start past it.
func TestWatchdog_ChildDoesNotOutliveDeadline(t *testing.T) {
	if os.Getenv("RAWCLAW_TEST_WATCHDOG_CHILD") == "1" {
		watchdogChildHelper()
		return
	}

	helper := exec.Command(os.Args[0], "-test.run=TestWatchdog_ChildDoesNotOutliveDeadline")
	helper.Env = append(os.Environ(), "RAWCLAW_TEST_WATCHDOG_CHILD=1")
	var out, errb bytes.Buffer
	helper.Stdout = &out
	helper.Stderr = &errb

	err := helper.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 124 {
		t.Fatalf("helper exited %v (stderr %q), want the watchdog's 124", err, errb.String())
	}

	pidStr := strings.TrimSpace(out.String())
	pid, perr := strconv.Atoi(pidStr)
	if perr != nil {
		t.Fatalf("helper printed %q, want the child pid (stderr %q)", pidStr, errb.String())
	}

	// The child should already be dead (or a moment from reaped). Poll briefly:
	// signal 0 probes liveness without touching the process.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return // gone — the deadline took the child with it
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL) // don't leave the orphan behind
	t.Fatalf("child %d outlived the watchdog deadline (orphaned)", pid)
}

// watchdogChildHelper mirrors the production run shape: watchdog armed for the
// invocation, a long-running child started under the run's context with its
// stdout captured through a pipe (as the live client does). It prints the
// child pid for the parent test, then blocks until the watchdog fires.
func watchdogChildHelper() {
	// Shaped like Execute: the run returns through the deferred stop(), which on
	// a fired deadline blocks until the watchdog's exit(124) takes the process.
	code := func() int {
		ctx, stop := startWatchdog(time.Second, os.Stderr, os.Exit)
		defer stop()

		// ctx is what cmd.Context() resolves to in production (Execute threads
		// the watchdog's context through ExecuteContext).
		child := exec.CommandContext(ctx, "sleep", "30")
		var buf bytes.Buffer
		child.Stdout = &buf
		if err := child.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(child.Process.Pid)
		_ = child.Wait()
		return 0
	}()
	os.Exit(code)
}
