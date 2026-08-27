package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// ingestLogMax caps the background ingest log's growth (512KB).
const ingestLogMax = 512 * 1024

// ingestSpawnWindow is the throttle window for detached ingest spawns (1 minute).
// A session (or global sweep) spawned within this window will not spawn again,
// preventing process storms from concurrent reads on wholesale-stale stores.
var ingestSpawnWindow = 1 * time.Minute

const closeoutTokenTTL = closeoutChildTimeout * 2

// closeoutTokenMu serializes stale-lease compare-and-swap in this process.
// os.Rename is atomic, but stat-then-rename can otherwise let a stale waiter
// rename a newly-created live lease at the same pathname.
var closeoutTokenMu sync.Mutex

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
		return false
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
	if err := os.WriteFile(markerPath, []byte(now.UTC().Format(time.RFC3339)), 0o644); err != nil {
		return false
	}
	return true
}

// acquireCloseoutToken atomically claims a session's closeout for its whole
// lifetime. Unlike the ingest throttle marker, this lock never expires: the
// detached worker removes it only after completion or failure.
func acquireCloseoutToken(sessionID string) (string, bool) {
	closeoutTokenMu.Lock()
	defer closeoutTokenMu.Unlock()
	dir := filepath.Join(store.CacheDir(), "ingest-spawns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	lockDir := closeoutTokenPath(dir, sessionID)
	for range 2 {
		if err := os.Mkdir(lockDir, 0o755); err != nil {
			if !os.IsExist(err) || !reclaimCloseoutToken(lockDir) {
				return "", false
			}
			continue
		}
		token, err := randomCloseoutToken()
		if err != nil {
			_ = os.Remove(lockDir)
			return "", false
		}
		marker := filepath.Join(lockDir, token)
		if err := os.WriteFile(marker, nil, 0o400); err != nil {
			_ = os.Remove(lockDir)
			return "", false
		}
		owner, err := json.Marshal(closeoutLease{PID: os.Getpid()})
		if err != nil || os.WriteFile(filepath.Join(lockDir, ".owner"), owner, 0o400) != nil {
			_ = os.RemoveAll(lockDir)
			return "", false
		}
		return token, true
	}
	return "", false
}

func releaseCloseoutToken(sessionID, token string) {
	closeoutTokenMu.Lock()
	defer closeoutTokenMu.Unlock()
	lockDir := closeoutTokenPath(filepath.Join(store.CacheDir(), "ingest-spawns"), sessionID)
	if !validCloseoutToken(token) {
		return
	}
	if _, err := os.Stat(filepath.Join(lockDir, token)); err == nil {
		_ = os.Remove(filepath.Join(lockDir, token))
		_ = os.Remove(filepath.Join(lockDir, ".owner"))
		_ = os.Remove(lockDir)
	}
}

func closeoutTokenPath(dir, sessionID string) string {
	safeID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, sessionID)
	return filepath.Join(dir, "closeout-"+safeID+".lock")
}

func reclaimCloseoutToken(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	owner, err := os.ReadFile(filepath.Join(path, ".owner"))
	if err == nil {
		var lease closeoutLease
		if json.Unmarshal(owner, &lease) != nil || lease.PID <= 0 || closeoutProcessAlive(lease.PID) {
			return false
		}
	} else if time.Since(st.ModTime()) < closeoutTokenTTL {
		return false
	}
	suffix, err := randomCloseoutToken()
	if err != nil {
		return false
	}
	quarantine := path + ".stale-" + suffix
	if err := os.Rename(path, quarantine); err != nil {
		return false
	}
	return os.RemoveAll(quarantine) == nil
}

type closeoutLease struct {
	PID int `json:"pid"`
}

func closeoutProcessAlive(pid int) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func validateCloseoutToken(sessionID, token string) bool {
	if !validCloseoutToken(token) {
		return false
	}
	_, err := os.Stat(filepath.Join(closeoutTokenPath(filepath.Join(store.CacheDir(), "ingest-spawns"), sessionID), token))
	return err == nil
}

func validCloseoutToken(token string) bool {
	b, err := hex.DecodeString(token)
	return err == nil && len(b) == 32
}

func randomCloseoutToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
	// Do not release the process in a long-lived test binary: the detached
	// child still needs to be waited on, or every stale read can leave a
	// cli.test ingest orphan behind. The goroutine is deliberately
	// best-effort; production callers may exit immediately after this
	// function returns, while test callers stay alive long enough to reap it.
	go func() { _ = cmd.Wait() }()
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
