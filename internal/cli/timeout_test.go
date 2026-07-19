package cli

import (
	"bytes"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestResolveTimeout covers the precedence: explicit flag > env > default, with a
// malformed env ignored and a non-positive flag disabling the watchdog.
func TestResolveTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flagSet bool
		flagVal time.Duration
		env     string
		want    time.Duration
	}{
		{"default", false, 0, "", defaultTimeout},
		{"env wins over default", false, 0, "45s", 45 * time.Second},
		{"flag wins over env", true, 5 * time.Second, "45s", 5 * time.Second},
		{"flag zero disables", true, 0, "45s", 0},
		{"malformed env falls back", false, 0, "not-a-duration", defaultTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveTimeout(tc.flagSet, tc.flagVal, tc.env); got != tc.want {
				t.Errorf("resolveTimeout(%v,%v,%q) = %v, want %v", tc.flagSet, tc.flagVal, tc.env, got, tc.want)
			}
		})
	}
}

// TestResolveTimeoutFromArgsUpgradeFloor proves the watchdog-vs-download fix: a
// bare `upgrade` (no --timeout, no env) must NOT inherit the 30s default that would
// kill a download mid-flight — its floor is raised to upgradeWatchdog. An explicit
// --timeout / RAWCLAW_TIMEOUT always wins, and non-upgrade commands are unaffected.
func TestResolveTimeoutFromArgsUpgradeFloor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		env  string
		want time.Duration
	}{
		{"bare upgrade gets the floor", []string{"upgrade"}, "", upgradeWatchdog},
		{"update alias gets the floor", []string{"update"}, "", upgradeWatchdog},
		{"upgrade --check gets the floor", []string{"upgrade", "--check"}, "", upgradeWatchdog},
		{"explicit --timeout wins over floor", []string{"upgrade", "--timeout", "90s"}, "", 90 * time.Second},
		{"explicit --timeout=0 disables even for upgrade", []string{"upgrade", "--timeout", "0"}, "", 0},
		{"RAWCLAW_TIMEOUT wins over floor", []string{"upgrade"}, "45s", 45 * time.Second},
		{"timeout flag before subcommand still wins", []string{"--timeout", "10s", "upgrade"}, "", 10 * time.Second},
		{"non-upgrade command keeps default", []string{"some", "query"}, "", defaultTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveTimeoutFromArgs(tc.args, tc.env); got != tc.want {
				t.Errorf("resolveTimeoutFromArgs(%q, %q) = %v, want %v", tc.args, tc.env, got, tc.want)
			}
		})
	}
}

func TestIsUpgradeInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"upgrade"}, true},
		{[]string{"update"}, true},
		{[]string{"upgrade", "--check"}, true},
		{[]string{"--timeout", "5s", "upgrade"}, true},
		{[]string{"version"}, false},
		{[]string{"some", "search", "terms"}, false},
		{nil, false},
		{[]string{"--json"}, false},
		// "upgrade" as a flag VALUE, or after `--` (a root positional to
		// cobra, never a subcommand dispatch), is not the upgrade command.
		{[]string{"--dir", "upgrade"}, false},
		{[]string{"--", "upgrade"}, false},
	}
	for _, tc := range tests {
		if got := isUpgradeInvocation(tc.args); got != tc.want {
			t.Errorf("isUpgradeInvocation(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

// TestWatchdogFiresOnDeadline is the HARD-GUARANTEE test: a deliberately-slow
// path (a goroutine parked far longer than the deadline, standing in for a blocked
// DB call) must NOT outlast the watchdog. With a tiny timeout and an injected exit
// hook, the watchdog must call exit(124) and write a clear stderr message within a
// small multiple of the deadline — never hanging for the full slow-path duration.
func TestWatchdogFiresOnDeadline(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		gotCode  = -1
		gotCalls int
	)
	exited := make(chan struct{})
	exit := func(code int) {
		mu.Lock()
		gotCode = code
		gotCalls++
		first := gotCalls == 1
		mu.Unlock()
		if first {
			close(exited)
		}
		// Real os.Exit never returns control to the caller. runtime.Goexit models
		// that (the watchdog goroutine stops here, never falling through to do more
		// work) while still running deferred cleanup — so the goroutine ends and
		// goleak stays green instead of the fake exit blocking forever.
		runtime.Goexit()
	}

	var stderr bytes.Buffer
	// No defer stop(): a fired watchdog's goroutine has already exited via Goexit,
	// so there is nothing to disarm; calling stop() would block on a goroutine that
	// is gone. The successful-disarm path is covered by TestWatchdogStopDisarmsCleanly.
	startWatchdog(20*time.Millisecond, &stderr, exit)

	// Stand-in for a blocked DB call that would otherwise hang far past the
	// deadline. release lets the test reap this goroutine once the watchdog has
	// fired, so it doesn't outlive the test (goleak would flag a parked sleeper).
	slowDone := make(chan struct{})
	release := make(chan struct{})
	go func() {
		defer close(slowDone)
		select {
		case <-time.After(10 * time.Second): // the "hang" we must never reach
		case <-release:
		}
	}()
	defer func() {
		close(release)
		<-slowDone
	}()

	select {
	case <-exited:
		// Good: the watchdog fired before the slow path finished.
	case <-slowDone:
		t.Fatal("slow path finished before the watchdog fired — rawclaw would have hung")
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog never fired within 2s of a 20ms deadline")
	}

	mu.Lock()
	code := gotCode
	mu.Unlock()
	if code != 124 {
		t.Errorf("watchdog exit code = %d, want 124 (timeout convention)", code)
	}
	if !strings.Contains(stderr.String(), "timed out") {
		t.Errorf("watchdog stderr missing timeout message, got %q", stderr.String())
	}
}

// TestWatchdogDisabledIsNoop: a non-positive timeout arms nothing and its stop()
// is a safe no-op (and leaves goleak nothing to find).
func TestWatchdogDisabledIsNoop(t *testing.T) {
	t.Parallel()

	called := false
	_, stop := startWatchdog(0, &bytes.Buffer{}, func(int) { called = true })
	stop()
	if called {
		t.Error("disabled watchdog must never call exit")
	}
}

// TestWatchdogStopDisarmsCleanly: arming then stopping before the deadline must not
// call exit and must not leak the watchdog goroutine (goleak's TestMain enforces
// the no-leak half).
func TestWatchdogStopDisarmsCleanly(t *testing.T) {
	t.Parallel()

	called := false
	_, stop := startWatchdog(time.Hour, &bytes.Buffer{}, func(int) { called = true })
	stop() // disarm well before the hour-long deadline
	if called {
		t.Error("exit called despite disarming before the deadline")
	}
}

// TestExecuteRunsUnderWatchdogAndDisarms drives the real Execute wrapper end to
// end with a generous --timeout: the command runs, returns, and the watchdog is
// disarmed (no leaked goroutine — goleak's TestMain is the assertion).
func TestExecuteRunsUnderWatchdog(t *testing.T) {
	t.Parallel()

	root := NewRootCmd(BuildInfo{})
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	// `version` is a fast, corpus-free path; --timeout keeps the watchdog armed
	// during the run, exercising the arm→disarm lifecycle inside Execute.
	if err := Execute(root, []string{"--timeout", "30s", "version"}); err != nil {
		t.Fatalf("Execute under watchdog: %v", err)
	}
	if !strings.Contains(out.String(), "rawclaw") {
		t.Errorf("version output missing under Execute, got %q", out.String())
	}
}
