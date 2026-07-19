package archive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAcquireAutosyncToken_FirstClaimWins: no token yet → the first claim
// succeeds and leaves the token behind; an immediate second claim (same or
// another process — the token is a file) is refused.
func TestAcquireAutosyncToken_FirstClaimWins(t *testing.T) {
	newTestHome(t)
	now := time.Now()

	if !AcquireAutosyncToken(now) {
		t.Fatal("first claim refused; want the spawn slot")
	}
	if _, err := os.Stat(autosyncTokenPath()); err != nil {
		t.Fatalf("token not left behind: %v", err)
	}
	if AcquireAutosyncToken(now) {
		t.Error("second immediate claim granted; want the throttle to hold")
	}
}

// TestAcquireAutosyncToken_StaleTokenReclaimed: a token older than the window
// is reclaimable — the stamp throttles, it doesn't disable.
func TestAcquireAutosyncToken_StaleTokenReclaimed(t *testing.T) {
	newTestHome(t)
	if !AcquireAutosyncToken(time.Now()) {
		t.Fatal("initial claim refused")
	}
	old := time.Now().Add(-autosyncWindow - time.Minute)
	if err := os.Chtimes(autosyncTokenPath(), old, old); err != nil {
		t.Fatal(err)
	}
	if !AcquireAutosyncToken(time.Now()) {
		t.Error("stale token not reclaimed; want a new spawn slot after the window")
	}
}

// TestAcquireAutosyncToken_FutureTokenIsDue: a token stamped in the future (a
// clock stepped backwards) is reclaimable now — one extra sync beats a
// silently muted window.
func TestAcquireAutosyncToken_FutureTokenIsDue(t *testing.T) {
	newTestHome(t)
	if !AcquireAutosyncToken(time.Now()) {
		t.Fatal("initial claim refused")
	}
	future := time.Now().Add(autosyncWindow + 2*time.Minute)
	if err := os.Chtimes(autosyncTokenPath(), future, future); err != nil {
		t.Fatal(err)
	}
	if !AcquireAutosyncToken(time.Now()) {
		t.Error("far-future token not reclaimed; want due on backwards clock")
	}
}

// TestAutosyncPaths_LiveInArchiveStateDir: token and log sit beside the other
// archive state (one dir holds every piece of cross-session archive state).
func TestAutosyncPaths_LiveInArchiveStateDir(t *testing.T) {
	newTestHome(t)
	dir := filepath.Dir(autosyncTokenPath())
	if filepath.Base(dir) != "archive" {
		t.Errorf("token dir = %s, want the archive state dir", dir)
	}
	if !strings.HasPrefix(AutosyncLogPath(), dir) {
		t.Errorf("log %s not beside token %s", AutosyncLogPath(), autosyncTokenPath())
	}
}
