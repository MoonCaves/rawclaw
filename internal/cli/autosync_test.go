package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/archive/archivetest"
)

// countSpawns swaps the spawn seam for a counter so gate tests observe spawn
// DECISIONS without forking processes.
func countSpawns(t *testing.T) *int {
	t.Helper()
	calls := 0
	old := spawnAutosync
	spawnAutosync = func() { calls++ }
	t.Cleanup(func() { spawnAutosync = old })
	return &calls
}

// TestMaybeAutosync_KillSwitchMeansZeroSpawns: RAWCLAW_ARCHIVE_AUTOSYNC=off
// spawns nothing, even with a configured archive and a free throttle slot.
func TestMaybeAutosync_KillSwitchMeansZeroSpawns(t *testing.T) {
	archivetest.Setup(t, "") // fixture leaves the kill switch ON ("off")
	calls := countSpawns(t)

	maybeAutosync()
	if *calls != 0 {
		t.Errorf("spawns under kill switch = %d, want 0", *calls)
	}
}

// TestMaybeAutosync_UnconfiguredMeansZeroSpawns: no archive configured → the
// trigger is a no-op, no child, no token.
func TestMaybeAutosync_UnconfiguredMeansZeroSpawns(t *testing.T) {
	newArchiveHome(t)
	t.Setenv("RAWCLAW_ARCHIVE_AUTOSYNC", "")
	calls := countSpawns(t)

	maybeAutosync()
	if *calls != 0 {
		t.Errorf("spawns with no archive = %d, want 0", *calls)
	}
}

// TestMaybeAutosync_ThrottleOneSpawnPerWindow: two triggers in a minute → one
// spawn; the token claimed at spawn time holds until the window elapses.
func TestMaybeAutosync_ThrottleOneSpawnPerWindow(t *testing.T) {
	archivetest.Setup(t, "")
	t.Setenv("RAWCLAW_ARCHIVE_AUTOSYNC", "")
	calls := countSpawns(t)

	maybeAutosync()
	maybeAutosync()
	if *calls != 1 {
		t.Errorf("spawns for two triggers in one window = %d, want 1", *calls)
	}
}

// TestSearch_TriggersAutosyncOnce: the real trigger point — two plain searches
// through the actual command tree spawn exactly one background sync ("two
// searches in a minute → one sync"), and the results are already printed when
// it fires.
func TestSearch_TriggersAutosyncOnce(t *testing.T) {
	archivetest.Setup(t, "")
	t.Setenv("RAWCLAW_ARCHIVE_AUTOSYNC", "")
	calls := countSpawns(t)

	for i := 0; i < 2; i++ {
		root := NewRootCmd(BuildInfo{})
		out, err := runCmd(t, root, "", archivetest.LocalBeacon)
		if err != nil {
			t.Fatalf("search %d: %v\n%s", i, err, out)
		}
	}
	if *calls != 1 {
		t.Errorf("spawns after two searches = %d, want 1", *calls)
	}
}

// TestFailedVerb_NeverSpawns: an erroring verb (bad read ref) syncs nothing —
// the trigger fires only after a successful, fully-printed result.
func TestFailedVerb_NeverSpawns(t *testing.T) {
	archivetest.Setup(t, "")
	t.Setenv("RAWCLAW_ARCHIVE_AUTOSYNC", "")
	calls := countSpawns(t)

	root := NewRootCmd(BuildInfo{})
	if _, err := runCmd(t, root, "", "read", "not-a-ref"); err == nil {
		t.Fatal("bogus read ref succeeded; test premise broken")
	}
	if *calls != 0 {
		t.Errorf("spawns after failed verb = %d, want 0", *calls)
	}
}

// TestSpawnAutosyncChild_DetachedChildRunsWithLog: the REAL spawn path, with a
// fake self-binary: the detached child runs `archive autosync --timeout 0`
// (wall-clock watchdog off; transfers are stall-bounded by the git runner)
// and its output lands in the state-dir receipt log — while the spawner
// returns immediately.
func TestSpawnAutosyncChild_DetachedChildRunsWithLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh-script fake child")
	}
	newArchiveHome(t)

	script := filepath.Join(t.TempDir(), "fake-rawclaw")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"child-argv $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe := selfExe
	selfExe = func() (string, error) { return script, nil }
	t.Cleanup(func() { selfExe = oldExe })

	spawnAutosyncChild()

	deadline := time.Now().Add(5 * time.Second)
	want := "child-argv archive autosync --timeout " + autosyncChildTimeoutArg
	for {
		b, _ := os.ReadFile(archive.AutosyncLogPath())
		if strings.Contains(string(b), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("receipt log never showed %q; log:\n%s", want, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestOpenAutosyncLog_RotatesOversized: an oversized receipt log is rotated to
// one .old generation before the next spawn appends.
func TestOpenAutosyncLog_RotatesOversized(t *testing.T) {
	newArchiveHome(t)
	p := archive.AutosyncLogPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", autosyncLogMax+1)
	if err := os.WriteFile(p, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := openAutosyncLog()
	if err != nil {
		t.Fatalf("openAutosyncLog: %v", err)
	}
	defer f.Close()

	st, err := os.Stat(p)
	if err != nil {
		t.Fatalf("fresh log missing after rotation: %v", err)
	}
	if st.Size() != 0 {
		t.Errorf("fresh log size = %d, want empty", st.Size())
	}
	old, err := os.Stat(p + ".old")
	if err != nil {
		t.Fatalf("rotated .old generation missing: %v", err)
	}
	if old.Size() <= int64(autosyncLogMax) {
		t.Errorf(".old size = %d, want the oversized original", old.Size())
	}
}
