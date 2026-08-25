package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// ingestLogMax caps the background ingest log's growth (512KB).
const ingestLogMax = 512 * 1024

// spawnIngest launches the detached ingest child — a seam so tests can
// count or hook spawn decisions without forking processes.
var spawnIngest = spawnIngestChild

// maybeSpawnIngest fires a detached self-invocation of `rawclaw ingest [session]`
// when staleness is detected on read paths, ensuring self-healing without delaying answers.
// RAWCLAW_BACKGROUND_INGEST=off disables background ingest spawns.
func maybeSpawnIngest(sessionArg string) {
	if strings.EqualFold(os.Getenv("RAWCLAW_BACKGROUND_INGEST"), "off") {
		return
	}
	spawnIngest(sessionArg)
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
