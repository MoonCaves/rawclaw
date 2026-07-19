package archive

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// StatusReport is the raw material for `archive status` and doctor-style
// output: where the archive lives, when this machine last synced, and how
// fresh each machine's dir is. Status is an OFFLINE read — recorded state
// only (config, stamp files, the clone's git history); it never fetches.
type StatusReport struct {
	Remote   string          // configured remote URL
	Clone    string          // local clone path
	CloneOK  bool            // a usable clone exists on disk
	LastPush time.Time       // last successful push from this machine (zero = never)
	LastPull time.Time       // last successful pull on this machine (zero = never)
	Machines []MachineStatus // one entry per machine dir in the clone, own first
}

// MachineStatus is one machine dir's freshness as recorded in the clone.
type MachineStatus struct {
	Name       string    // top-level dir name in the archive
	MachineID  string    // stable machine id from the dir's manifest
	Own        bool      // this machine's own dir
	LastCommit time.Time // last commit touching the dir (zero = none yet)
	Stale      bool      // older than the staleAfter window (same window search uses)
}

// Status reports clone path, remote, last push/pull, and per-machine
// last-commit staleness. A missing clone is a reported state (CloneOK=false,
// no machines), not an error — `archive pull` is the repair path.
func (a *Archive) Status(ctx context.Context) (StatusReport, error) {
	st := StatusReport{
		Remote:   a.cfg.Remote,
		Clone:    a.clone,
		LastPush: stampTime(pushStampPath()),
		LastPull: stampTime(pullStampPath()),
	}
	if _, err := os.Stat(filepath.Join(a.clone, ".git")); err != nil {
		return st, nil
	}
	st.CloneOK = true

	now := time.Now()
	own := MachineStatus{Name: a.cfg.Name, MachineID: a.machineID, Own: true}
	own.LastCommit = a.dirLastCommit(ctx, a.cfg.Name)
	own.Stale = staleAt(own.LastCommit, now)
	st.Machines = append(st.Machines, own)

	for _, m := range a.foreignMachines() {
		lc := a.dirLastCommit(ctx, m.Name)
		st.Machines = append(st.Machines, MachineStatus{
			Name:       m.Name,
			MachineID:  m.MachineID,
			LastCommit: lc,
			Stale:      staleAt(lc, now),
		})
	}
	return st, nil
}

// dirLastCommit resolves the last commit time touching name's dir in the
// clone — the clone's knowledge of that machine IS what search serves, so an
// un-pulled clone and a silent machine both (correctly) read as old. Zero
// when the probe fails or the dir has no history yet.
func (a *Archive) dirLastCommit(ctx context.Context, name string) time.Time {
	out, err := a.run(ctx, a.clone, "log", "-1", "--format=%ct", "--", name)
	if err != nil {
		return time.Time{}
	}
	ct, perr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if perr != nil {
		return time.Time{}
	}
	return time.Unix(ct, 0).UTC()
}

// staleAt applies the shared staleness window: unknown freshness (zero time)
// is reported stale, never silently passed off as fresh.
func staleAt(lastCommit, now time.Time) bool {
	return lastCommit.IsZero() || now.Sub(lastCommit) > staleAfter
}

// pushStampPath is <state-dir>/archive/last-push — the last-successful-push
// record. Like the pull stamp, its MTIME is the record (the body stays empty).
func pushStampPath() string {
	return filepath.Join(store.CacheDir(), "archive", "last-push")
}

// stampPush records a successful push by (re)writing the stamp file.
// Best-effort: a failed stamp only under-reports `archive status`.
func stampPush() {
	p := pushStampPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(p, nil, 0o644)
}

// stampTime reads a stamp file's mtime; zero when the stamp does not exist.
func stampTime(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}
