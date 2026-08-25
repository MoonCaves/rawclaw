package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// ingestLogMax caps the background ingest log's growth (512KB).
const ingestLogMax = 512 * 1024

// ingestSpawnWindow is the throttle window for detached ingest spawns (1 minute).
// A session (or global sweep) spawned within this window will not spawn again,
// preventing process storms from concurrent reads on wholesale-stale stores.
var ingestSpawnWindow = 1 * time.Minute

// spawnIngest launches the detached ingest child — a seam so tests can
// count or hook spawn decisions without forking processes.
var spawnIngest = spawnIngestChild

// acquireIngestSpawnToken checks if a background ingest spawn token can be acquired.
// Choice: We pick option (a) from FIXES2.md — a per-session spawn marker file with an
// age check in the cache dir. This stops the process storm before the child is ever forked,
// keeping spawn-side overhead at zero without launching short-lived redundant processes.
func acquireIngestSpawnToken(sessionArg string, now time.Time) bool {
	dir := filepath.Join(store.CacheDir(), "ingest-spawns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return true // best effort; do not block ingest spawn on dir error
	}
	markerName := "_global.token"
	if sessionArg != "" {
		safeID := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			return '_'
		}, sessionArg)
		markerName = safeID + ".token"
	}
	markerPath := filepath.Join(dir, markerName)
	if st, err := os.Stat(markerPath); err == nil {
		if now.Sub(st.ModTime()) < ingestSpawnWindow {
			return false // throttled: spawn already triggered within ingestSpawnWindow
		}
	}
	_ = os.WriteFile(markerPath, []byte(now.UTC().Format(time.RFC3339)), 0o644)
	return true
}

// maybeSpawnIngest fires a detached self-invocation of `rawclaw ingest [session]`
// when staleness is detected on read paths, ensuring self-healing without delaying answers.
// It returns true if a background ingest was actually triggered, and false if suppressed
// (via RAWCLAW_BACKGROUND_INGEST=off or spawn rate-limit token).
func maybeSpawnIngest(sessionArg string) bool {
	if strings.EqualFold(os.Getenv("RAWCLAW_BACKGROUND_INGEST"), "off") {
		return false
	}
	if !acquireIngestSpawnToken(sessionArg, time.Now()) {
		return false
	}
	spawnIngest(sessionArg)
	return true
}

// spawnIngestChild fires the detached self-invocation of `rawclaw ingest [session]`:
// own session (setsid / detached console), stdin closed, stdout+stderr appended to ingest.log.
// Start-and-release — the caller never waits.
func spawnIngestChild(sessionArg string) {
	exe, err := selfExe()
	if err != nil {
		return
	}
	logf, err := openIngestLog()
	if err != nil {
		return
	}
	defer logf.Close()

	args := []string{"ingest"}
	if sessionArg != "" {
		args = append(args, sessionArg)
	}
	cmd := exec.Command(exe, args...)
	detach(cmd)
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Release()
}

// openIngestLog opens the ingest log in the cache dir for append, rotating an
// oversized log to .old first.
func openIngestLog() (*os.File, error) {
	logDir := store.CacheDir()
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", logDir, err)
	}
	p := filepath.Join(logDir, "ingest.log")
	if st, err := os.Stat(p); err == nil && st.Size() > ingestLogMax {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", p, err)
	}
	return f, nil
}
