package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
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
	spawnCloseout = func(sessionID string) error {
		launches = append(launches, sessionID)
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
}

func TestRunCloseout_ConcurrentParentsLaunchOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCloseoutConfig(t, []string{"/absolute/tagger"})
	sid := "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	oldSpawn := spawnCloseout
	t.Cleanup(func() { spawnCloseout = oldSpawn; releaseCloseoutToken(sid) })
	var mu sync.Mutex
	launches := 0
	spawnCloseout = func(string) error {
		mu.Lock()
		launches++
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
	spawnCloseout = func(string) error { return errors.New("startup failed") }
	t.Cleanup(func() { spawnCloseout = oldSpawn; releaseCloseoutToken(sid) })
	if err := runCloseout(new(bytes.Buffer), sid); err == nil {
		t.Fatal("runCloseout unexpectedly succeeded")
	}
	if release, ok := acquireCloseoutToken(sid); !ok {
		t.Fatal("token remained held after spawn failure")
	} else {
		release()
	}
}

func TestCloseoutToken_ReclaimsDeadLeaseButNotLiveLease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "ffffffff-0000-1111-2222-333333333333"
	dir := filepath.Join(store.CacheDir(), "ingest-spawns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := closeoutTokenPath(dir, sid)
	old := time.Now().Add(-2 * closeoutLeaseWindow)
	writeLease := func(pid int) {
		t.Helper()
		data, err := json.Marshal(closeoutLease{PID: pid, CreatedAt: old.UnixNano()})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	writeLease(os.Getpid())
	if _, ok := acquireCloseoutToken(sid); ok {
		t.Fatal("reclaimed a live closeout lease")
	}
	writeLease(999999)
	if release, ok := acquireCloseoutToken(sid); !ok {
		t.Fatal("did not reclaim a dead closeout lease")
	} else {
		release()
	}
}

func TestCloseoutTokenHeldUntilExplicitRelease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "cccccccc-dddd-eeee-ffff-000000000000"
	release, ok := acquireCloseoutToken(sid)
	if !ok {
		t.Fatal("first closeout token acquisition failed")
	}
	defer release()
	if _, ok := acquireCloseoutToken(sid); ok {
		t.Fatal("active closeout token was reacquired")
	}
	if _, ok := acquireCloseoutToken(sid); ok {
		t.Fatal("active closeout token was reacquired after elapsed time")
	}
	release()
	if next, ok := acquireCloseoutToken(sid); !ok {
		t.Fatal("closeout token was not recoverable after release")
	} else {
		next()
	}
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
			unlock, ok := acquireCloseoutToken(sid)
			if !ok {
				t.Fatal("acquireCloseoutToken failed")
			}
			defer unlock()
			if err := runCloseoutChild(new(bytes.Buffer), sid); tc.name == "failure" && err == nil {
				t.Fatal("runCloseoutChild unexpectedly succeeded")
			}
			if next, ok := acquireCloseoutToken(sid); !ok {
				t.Fatal("closeout token was not recoverable")
			} else {
				next()
			}
		})
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
