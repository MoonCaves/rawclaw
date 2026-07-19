package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/archive"
)

// autosyncChildTimeout is the explicit --timeout the detached sync child (and
// the hourly timer) runs under: the 30s default watchdog would kill a
// legitimate first big push mid-transfer, while a hard cap must still exist —
// a hung push dies at the deadline instead of lingering forever. Carried in
// argv so the cap is visible in the receipt log and to ps(1).
const autosyncChildTimeout = "10m"

// autosyncLogMax caps the receipt log's growth: above it the log is rotated
// (one .old generation kept) before the next child is spawned, so years of
// background syncs can't grow an unbounded file in the state dir.
const autosyncLogMax = 512 * 1024

// selfExe resolves the running rawclaw binary for self-referential wiring
// (the autosync spawn, the timer's ProgramArguments) — a seam so tests can
// point at a fake instead of the test binary.
var selfExe = os.Executable

// spawnAutosync launches the detached sync child — a seam so unit tests can
// count spawn decisions without forking processes.
var spawnAutosync = spawnAutosyncChild

// maybeAutosync quietly keeps the archive current after an ordinary
// search/read/outline: results are already printed, then a DETACHED
// self-invocation (`archive autosync`: push + throttled pull) is fired so the
// caller's tool call never waits on the network. Gates, cheapest first:
// RAWCLAW_ARCHIVE_AUTOSYNC=off kills the feature; an unconfigured archive
// means zero child spawns; the spawn-throttle token limits a burst of
// invocations to one child per window. Every failure path is silent — a
// background nicety must never break or noise up a search.
func maybeAutosync() {
	if strings.EqualFold(os.Getenv("RAWCLAW_ARCHIVE_AUTOSYNC"), "off") {
		return
	}
	a, err := archive.Load()
	if a == nil || err != nil {
		return
	}
	if !archive.AcquireAutosyncToken(time.Now()) {
		return
	}
	spawnAutosync()
}

// spawnAutosyncChild fires the detached self-invocation: own session (setsid /
// detached console), stdin closed, stdout+stderr appended to the receipt log
// in the state dir — the child may outlive the parent, so it must never hold
// open a pipe the parent's caller is draining. Started with a bare
// exec.Command (never CommandContext: the parent exiting is the design, not a
// reason to kill the child); the child self-bounds via its own --timeout
// watchdog. Start-and-release — the parent never waits.
func spawnAutosyncChild() {
	exe, err := selfExe()
	if err != nil {
		return
	}
	logf, err := openAutosyncLog()
	if err != nil {
		return
	}
	defer logf.Close() // parent's handle only; the child holds its own

	cmd := exec.Command(exe, "archive", "autosync", "--timeout", autosyncChildTimeout)
	detach(cmd)
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Release() // one-shot parent: never Wait on a detached child
}

// openAutosyncLog opens the receipt log for append, rotating an oversized log
// to a single .old generation first.
func openAutosyncLog() (*os.File, error) {
	p := archive.AutosyncLogPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(p), err)
	}
	if st, err := os.Stat(p); err == nil && st.Size() > autosyncLogMax {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", p, err)
	}
	return f, nil
}
