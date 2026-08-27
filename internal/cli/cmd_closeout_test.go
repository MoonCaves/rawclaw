package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
)

func writeCloseoutConfig(t *testing.T, argv []string) {
	t.Helper()
	path := filepath.Join(os.Getenv("HOME"), ".cache", "session-search", "tagger-config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(closeoutTaggerConfig{Argv: argv})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCloseout_MissingConfigPrintsManualRecovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "11111111-2222-3333-4444-555555555555"

	var out bytes.Buffer
	if err := runCloseout(&out, sid); err != nil {
		t.Fatalf("runCloseout: %v", err)
	}
	if got, want := out.String(), "rawclaw tag-prep "+sid+"\n"; got != want {
		t.Fatalf("recovery = %q, want %q", got, want)
	}
}

func TestRunCloseout_LaunchesOncePerSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCloseoutConfig(t, []string{"/absolute/tagger"})
	sid := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	oldSpawn := spawnCloseout
	oldNow := closeoutNow
	t.Cleanup(func() {
		spawnCloseout = oldSpawn
		closeoutNow = oldNow
	})
	var launches []string
	var launchToken string
	spawnCloseout = func(sessionID, token string) error {
		launches = append(launches, sessionID)
		launchToken = token
		return nil
	}
	closeoutNow = func() time.Time { return time.Unix(100, 0) }

	var first, second bytes.Buffer
	if err := runCloseout(&first, sid); err != nil {
		t.Fatalf("first runCloseout: %v", err)
	}
	if err := runCloseout(&second, sid); err != nil {
		t.Fatalf("second runCloseout: %v", err)
	}
	if len(launches) != 1 || launches[0] != sid {
		t.Fatalf("launches = %v, want one launch for %s", launches, sid)
	}
	if !strings.Contains(second.String(), "already queued") {
		t.Fatalf("duplicate output = %q", second.String())
	}
	releaseCloseoutToken(sid, launchToken)
}

func TestRunCloseout_ConcurrentParentsLaunchOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCloseoutConfig(t, []string{"/absolute/tagger"})
	sid := "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	oldSpawn := spawnCloseout
	t.Cleanup(func() { spawnCloseout = oldSpawn })
	var mu sync.Mutex
	launches := 0
	var launchToken string
	spawnCloseout = func(_, token string) error {
		mu.Lock()
		launches++
		launchToken = token
		mu.Unlock()
		return nil
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var out bytes.Buffer
			if err := runCloseout(&out, sid); err != nil {
				t.Errorf("runCloseout: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if launches != 1 {
		t.Fatalf("launches = %d, want exactly one", launches)
	}
	releaseCloseoutToken(sid, launchToken)
}

func TestCloseoutToken_FailsClosedWhenDirectoryCannotBeCreated(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home-file")
	if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if _, ok := acquireCloseoutToken("failed-dir"); ok {
		t.Fatal("closeout token acquisition succeeded without a token directory")
	}
	if acquireIngestSpawnToken("failed-dir", time.Now()) {
		t.Fatal("ingest token acquisition succeeded without a token directory")
	}
}

func TestRunCloseout_SpawnFailureReleasesToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCloseoutConfig(t, []string{"/absolute/tagger"})
	sid := "eeeeeeee-ffff-0000-1111-222222222222"
	oldSpawn := spawnCloseout
	spawnCloseout = func(string, string) error { return errors.New("startup failed") }
	t.Cleanup(func() { spawnCloseout = oldSpawn })
	if err := runCloseout(new(bytes.Buffer), sid); err == nil {
		t.Fatal("runCloseout unexpectedly succeeded")
	}
	token, ok := acquireCloseoutToken(sid)
	if !ok {
		t.Fatal("token remained held after spawn failure")
	}
	releaseCloseoutToken(sid, token)
}

func TestCloseoutToken_ReclaimsDeadLeaseButNotLiveLease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "ffffffff-0000-1111-2222-333333333333"
	dir := filepath.Join(store.CacheDir(), "ingest-spawns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := closeoutTokenPath(dir, sid)
	old := time.Now().Add(-2 * closeoutTokenTTL)
	writeLease := func(value string, when time.Time) {
		t.Helper()
		_ = os.RemoveAll(path)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, value), nil, 0o400); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	writeLease("live-token", time.Now())
	if _, ok := acquireCloseoutToken(sid); ok {
		t.Fatal("reclaimed a live closeout lease")
	}
	writeLease("stale-token", old)
	if _, ok := acquireCloseoutToken(sid); !ok {
		t.Fatal("did not reclaim a dead closeout lease")
	}
	// The newly acquired token is intentionally released by its owner only.
}

func TestCloseoutToken_ConcurrentStaleTakeoverHasOneWinner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "12121212-3434-5656-7878-909090909090"
	dir := filepath.Join(store.CacheDir(), "ingest-spawns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := closeoutTokenPath(dir, sid)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "stale-token"), nil, 0o400); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * closeoutTokenTTL)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var tokens []string
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if token, ok := acquireCloseoutToken(sid); ok {
				mu.Lock()
				tokens = append(tokens, token)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if len(tokens) != 1 {
		t.Fatalf("stale takeover winners = %d, want exactly one", len(tokens))
	}
	releaseCloseoutToken(sid, tokens[0])
}

func TestCloseoutToken_ReclaimsExitedOwnerImmediately(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "abababab-cdcd-efef-0101-232323232323"
	dir := filepath.Join(store.CacheDir(), "ingest-spawns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := closeoutTokenPath(dir, sid)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "stale-token"), nil, 0o400); err != nil {
		t.Fatal(err)
	}
	owner, err := json.Marshal(closeoutLease{PID: 999999})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".owner"), owner, 0o400); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	token, ok := acquireCloseoutToken(sid)
	if !ok {
		t.Fatal("did not reclaim exited owner lease")
	}
	defer releaseCloseoutToken(sid, token)
	if elapsed := time.Since(started); elapsed >= closeoutTokenTTL {
		t.Fatalf("reclaim took %s; want immediate takeover", elapsed)
	}
}

func TestCloseoutTokenHeldUntilExplicitRelease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "cccccccc-dddd-eeee-ffff-000000000000"
	token, ok := acquireCloseoutToken(sid)
	if !ok {
		t.Fatal("first closeout token acquisition failed")
	}
	defer releaseCloseoutToken(sid, token)
	if _, ok := acquireCloseoutToken(sid); ok {
		t.Fatal("active closeout token was reacquired")
	}
	if _, ok := acquireCloseoutToken(sid); ok {
		t.Fatal("active closeout token was reacquired after elapsed time")
	}
	releaseCloseoutToken(sid, token)
	next, ok := acquireCloseoutToken(sid)
	if !ok {
		t.Fatal("closeout token was not recoverable after release")
	}
	releaseCloseoutToken(sid, next)
}

func TestCloseoutToken_IndependentProcessesSingleWinner(t *testing.T) {
	if os.Getenv("RAWCLAW_CLOSEOUT_HELPER") == "stale-taker" {
		ready := os.Getenv("RAWCLAW_CLOSEOUT_READY")
		_ = os.WriteFile(filepath.Join(ready, strconv.Itoa(os.Getpid())), nil, 0o600)
		for {
			if _, err := os.Stat(os.Getenv("RAWCLAW_CLOSEOUT_START")); err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if _, ok := acquireCloseoutToken(os.Getenv("RAWCLAW_CLOSEOUT_SESSION")); ok {
			fmt.Fprintln(os.Stdout, "acquired")
		}
		_ = os.WriteFile(filepath.Join(os.Getenv("RAWCLAW_CLOSEOUT_ATTEMPTED"), strconv.Itoa(os.Getpid())), nil, 0o600)
		for {
			if _, err := os.Stat(os.Getenv("RAWCLAW_CLOSEOUT_RELEASE")); err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("independent process lease race uses POSIX mtime controls")
	}
	t.Setenv("HOME", t.TempDir())
	sid := "56565656-7878-9090-abab-cdcdcdcdcdcd"
	dir := filepath.Join(store.CacheDir(), "ingest-spawns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := closeoutTokenPath(dir, sid)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "stale-token"), nil, 0o400); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * closeoutTokenTTL)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	readyDir := filepath.Join(os.Getenv("HOME"), "ready")
	attemptedDir := filepath.Join(os.Getenv("HOME"), "attempted")
	if err := os.MkdirAll(readyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	startPath := filepath.Join(os.Getenv("HOME"), "start")
	releasePath := filepath.Join(os.Getenv("HOME"), "release")
	commands := make([]*exec.Cmd, 32)
	outputs := make([]*bytes.Buffer, len(commands))
	for i := range commands {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCloseoutToken_IndependentProcessesSingleWinner$")
		cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"), "RAWCLAW_CLOSEOUT_HELPER=stale-taker", "RAWCLAW_CLOSEOUT_SESSION="+sid, "RAWCLAW_CLOSEOUT_READY="+readyDir, "RAWCLAW_CLOSEOUT_ATTEMPTED="+attemptedDir, "RAWCLAW_CLOSEOUT_START="+startPath, "RAWCLAW_CLOSEOUT_RELEASE="+releasePath)
		outputs[i] = new(bytes.Buffer)
		cmd.Stdout = outputs[i]
		commands[i] = cmd
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(readyDir)
		if len(entries) == len(commands) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if entries, _ := os.ReadDir(readyDir); len(entries) != len(commands) {
		t.Fatalf("ready helpers = %d, want %d", len(entries), len(commands))
	}
	if err := os.WriteFile(startPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(attemptedDir)
		if len(entries) == len(commands) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if entries, _ := os.ReadDir(attemptedDir); len(entries) != len(commands) {
		t.Fatalf("attempted helpers = %d, want %d", len(entries), len(commands))
	}
	if err := os.WriteFile(releasePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	winners := 0
	for i, cmd := range commands {
		err := cmd.Wait()
		if err == nil && strings.Contains(outputs[i].String(), "acquired") {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("independent stale takeover winners = %d, want exactly one", winners)
	}
}

func TestCloseoutToken_UnrelatedSessionsDoNotShareGuard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(store.CacheDir(), "ingest-spawns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	first := "78787878-9090-abab-cdcd-efefefefefef"
	second := "89898989-0101-bcbc-dede-f0f0f0f0f0f0"
	guard := flock.New(closeoutGuardPath(dir, first))
	if err := guard.Lock(); err != nil {
		t.Fatal(err)
	}
	defer guard.Unlock()
	token, ok := acquireCloseoutToken(second)
	if !ok {
		t.Fatal("unrelated session was blocked by another session guard")
	}
	releaseCloseoutToken(second, token)
}

func TestRunCloseout_IndependentOwnerChildBlocksRetry(t *testing.T) {
	if os.Getenv("RAWCLAW_CLOSEOUT_HELPER") == "live-owner" {
		liveOwnerHelper()
		return
	}
	if os.Getenv("RAWCLAW_CLOSEOUT_HELPER") == "retry-owner" {
		t.Setenv("HOME", os.Getenv("RAWCLAW_CLOSEOUT_HOME"))
		var out bytes.Buffer
		if err := runCloseout(&out, os.Getenv("RAWCLAW_CLOSEOUT_SESSION")); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout, out.String())
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("live detached child test uses a POSIX shell helper")
	}
	home := t.TempDir()
	sid := "67676767-8989-0101-bcbc-dededededede"
	launchLog := filepath.Join(home, "launches")
	pidFile := filepath.Join(home, "child.pid")
	childScript := filepath.Join(home, "child.sh")
	script := fmt.Sprintf("#!/bin/sh\necho $$ > %s\necho launch >> %s\nsleep 30\n", strconv.Quote(pidFile), strconv.Quote(launchLog))
	if err := os.WriteFile(childScript, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, ".cache", "session-search", "tagger-config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(closeoutTaggerConfig{Argv: []string{childScript}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}

	owner := exec.Command(os.Args[0], "-test.run=^TestRunCloseout_IndependentOwnerChildBlocksRetry$")
	owner.Env = append(os.Environ(), "HOME="+home, "RAWCLAW_CLOSEOUT_HELPER=live-owner", "RAWCLAW_CLOSEOUT_SESSION="+sid, "RAWCLAW_CLOSEOUT_CHILD_SCRIPT="+childScript)
	if out, err := owner.CombinedOutput(); err != nil {
		t.Fatalf("live owner = %v, output %q", err, out)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatal("detached child did not start")
	}
	t.Cleanup(func() {
		pidBytes, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
		if err == nil {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
		}
	})

	retry := exec.Command(os.Args[0], "-test.run=^TestRunCloseout_IndependentOwnerChildBlocksRetry$")
	retry.Env = append(os.Environ(), "HOME="+home, "RAWCLAW_CLOSEOUT_HOME="+home, "RAWCLAW_CLOSEOUT_HELPER=retry-owner", "RAWCLAW_CLOSEOUT_SESSION="+sid)
	out, err := retry.CombinedOutput()
	if err != nil {
		t.Fatalf("retry = %v, output %q", err, out)
	}
	if !strings.Contains(string(out), "already queued") {
		t.Fatalf("retry output = %q, want already queued while child is live", out)
	}
	if launches, _ := os.ReadFile(launchLog); strings.Count(string(launches), "launch\n") != 1 {
		t.Fatalf("child launches = %q, want one", launches)
	}
}

func liveOwnerHelper() {
	oldSelfExe := selfExe
	selfExe = func() (string, error) { return os.Getenv("RAWCLAW_CLOSEOUT_CHILD_SCRIPT"), nil }
	defer func() { selfExe = oldSelfExe }()
	if err := runCloseout(io.Discard, os.Getenv("RAWCLAW_CLOSEOUT_SESSION")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestRunCloseoutChild_ReleasesTokenAfterCompletionOrFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, tc := range []struct {
		name   string
		config []byte
	}{
		{name: "completion", config: nil},
		{name: "failure", config: []byte("not-json")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sid := "dddddddd-eeee-ffff-0000-111111111111"
			if tc.config != nil {
				path := filepath.Join(os.Getenv("HOME"), ".cache", "session-search", "tagger-config.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, tc.config, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			token, ok := acquireCloseoutToken(sid)
			if !ok {
				t.Fatal("acquireCloseoutToken failed")
			}
			defer releaseCloseoutToken(sid, token)
			if err := runCloseoutChild(new(bytes.Buffer), sid, token); tc.name == "failure" && err == nil {
				t.Fatal("runCloseoutChild unexpectedly succeeded")
			}
			next, ok := acquireCloseoutToken(sid)
			if !ok {
				t.Fatal("closeout token was not recoverable")
			}
			releaseCloseoutToken(sid, next)
		})
	}
}

func TestRunCloseoutChild_RequiresOwnedLease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := runCloseoutChild(new(bytes.Buffer), "11111111-2222-3333-4444-555555555555", "missing"); err == nil {
		t.Fatal("ownerless closeout child unexpectedly proceeded")
	}
}

func TestRunCloseout_RequiresFullSessionID(t *testing.T) {
	var out bytes.Buffer
	if err := runCloseout(&out, "deadbeef"); err == nil {
		t.Fatal("runCloseout accepted an 8-character session prefix")
	}
}

func TestRunCloseoutTagger_RejectsFailureAndMalformedOutput(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	printf, err := exec.LookPath("printf")
	if err != nil {
		t.Skip("no printf available")
	}
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true available")
	}
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "nonzero", argv: []string{sh, "-c", "exit 7"}, want: "unsuccessfully"},
		{name: "malformed", argv: []string{printf, "not-json"}, want: "malformed JSON"},
		{name: "empty", argv: []string{truePath}, want: "empty output"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var log bytes.Buffer
			_, err := runCloseoutTagger(tc.argv, []byte("prep"), &log)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runCloseoutTagger error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRunCloseoutTagger_TimeoutKillsDescendantHoldingStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process-group behavior")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	oldTimeout := closeoutTaggerTimeout
	closeoutTaggerTimeout = 100 * time.Millisecond
	t.Cleanup(func() { closeoutTaggerTimeout = oldTimeout })
	var stderr bytes.Buffer
	started := time.Now()
	_, err = runCloseoutTagger([]string{sh, "-c", "sleep 30 & wait"}, []byte("prep"), &stderr)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("runCloseoutTagger error = %v, want timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("timeout cleanup took %s; descendant likely held stdio", elapsed)
	}
}

func TestRunCloseoutTagger_TimeoutReturnsWhenTerminationFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell helper")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	oldTimeout := closeoutTaggerTimeout
	oldTerminate := terminateCloseout
	t.Cleanup(func() {
		closeoutTaggerTimeout = oldTimeout
		terminateCloseout = oldTerminate
	})
	closeoutTaggerTimeout = 50 * time.Millisecond
	var process *os.Process
	terminateCloseout = func(cmd *exec.Cmd) error {
		process = cmd.Process
		return errors.New("synthetic termination failure")
	}
	started := time.Now()
	_, err = runCloseoutTagger([]string{sh, "-c", "sleep 30"}, []byte("prep"), new(bytes.Buffer))
	if process != nil {
		_ = process.Kill()
	}
	if err == nil || !strings.Contains(err.Error(), "process cleanup") {
		t.Fatalf("runCloseoutTagger error = %v, want bounded cleanup error", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("timeout failure path took %s", elapsed)
	}
}
