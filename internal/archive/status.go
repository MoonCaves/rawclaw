package archive

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// StatusReport is the raw material for `archive status` and doctor-style
// output: where the archive lives, when this machine last synced, and when
// each machine's dir last received new content. Status is an OFFLINE read —
// recorded state only (config, stamp files, the clone's git history); it
// never fetches.
//
// The only staleness verdicts here are the own-sync overdue flags: this
// machine knows first-hand when ITS last successful push/pull ran. A foreign
// machine's freshness is deliberately NOT judged — from the clone alone an
// idle-but-healthy machine (nothing new to commit) and a dead one are
// indistinguishable, so per-machine state is reported as the honest
// LastCommit ("last new content") and nothing more.
type StatusReport struct {
	Remote      string          // configured remote URL
	Clone       string          // local clone path
	CloneOK     bool            // a COMPLETED clone exists (sentinel present — ensureClone's own predicate)
	LastPush    time.Time       // last successful push sync from this machine, incl. verified no-ops (zero = never)
	LastPull    time.Time       // last successful pull on this machine (zero = never)
	PushOverdue bool            // a recorded push sync exists but is older than the window (never ≠ overdue)
	PullOverdue bool            // a recorded pull exists but is older than the window (never ≠ overdue)
	Machines    []MachineStatus // one entry per machine dir in the clone, own first
}

// MachineStatus is one machine dir's recorded state in the clone.
type MachineStatus struct {
	Name       string    // top-level dir name in the archive
	MachineID  string    // stable machine id from the dir's manifest
	Own        bool      // this machine's own dir
	LastCommit time.Time // last commit touching the dir (zero = none yet) — last NEW CONTENT, not liveness
}

// Status reports clone path, remote, last push/pull (with own-sync overdue
// flags), and per-machine last-new-content times. A missing clone is a
// reported state (CloneOK=false, no machines), not an error — `archive pull`
// is the repair path.
func (a *Archive) Status(ctx context.Context) (StatusReport, error) {
	st := StatusReport{
		Remote:   a.cfg.Remote,
		Clone:    a.clone,
		LastPush: stampTime(pushStampPath()),
		LastPull: stampTime(pullStampPath()),
	}
	now := time.Now()
	st.PushOverdue = overdueAt(st.LastPush, now)
	st.PullOverdue = overdueAt(st.LastPull, now)
	// Same marker ensureClone stamps and scope enumeration trusts: no
	// sentinel → not a VERIFIED clone, so status won't vouch for it. (A
	// structurally complete pre-sentinel clone gets adopted — stamped — by
	// the next push/pull; status is an offline read and never probes git to
	// make that call itself.)
	if !a.cloneUsable() {
		return st, nil
	}
	st.CloneOK = true

	own := MachineStatus{Name: a.cfg.Name, MachineID: a.machineID, Own: true}
	own.LastCommit = a.dirLastCommit(ctx, a.cfg.Name)
	st.Machines = append(st.Machines, own)

	for _, m := range a.foreignMachines() {
		st.Machines = append(st.Machines, MachineStatus{
			Name:       m.Name,
			MachineID:  m.MachineID,
			LastCommit: a.dirLastCommit(ctx, m.Name),
		})
	}
	return st, nil
}

// overdueAt reports whether a recorded own-sync stamp is older than the
// shared window. A zero stamp is NOT overdue: "never" is its own honest
// state (a machine that never pulled isn't a broken sync loop).
func overdueAt(stamp, now time.Time) bool {
	return !stamp.IsZero() && now.Sub(stamp) > staleAfter
}

// dirLastCommit resolves the last commit time touching name's dir in the
// clone — the clone's knowledge of that machine IS what search serves, so an
// un-pulled clone and a silent machine both (correctly) read as old. Zero
// when the probe fails or the dir has no history yet. The name is passed as
// a literal pathspec: foreign dir names arrive from other machines' pushes
// unvalidated, and a glob metachar in one must probe that dir, not pattern-
// match across the clone (same posture as the foreign scope enumeration).
func (a *Archive) dirLastCommit(ctx context.Context, name string) time.Time {
	out, err := a.run(ctx, a.clone, "log", "-1", "--format=%ct", "--", ":(literal)"+name)
	if err != nil {
		return time.Time{}
	}
	ct, perr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if perr != nil {
		return time.Time{}
	}
	return time.Unix(ct, 0).UTC()
}

// stampTime reads a stamp file's mtime; zero when the stamp does not exist.
func stampTime(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}
