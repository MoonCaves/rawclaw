package archive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// PrivacyWarning is printed by `archive init`. Transcripts contain whatever
// was pasted into sessions; no host-side detection of repo visibility is
// attempted (unreliable across git hosts) — this warning is the mechanism.
const PrivacyWarning = `WARNING: transcripts contain whatever you and your agents pasted into
sessions - API keys, tokens, private code. The archive remote MUST be a
PRIVATE repository. rawclaw does not (and cannot reliably) verify this.`

// Init bootstraps the archive: clones the remote (an empty remote clones fine;
// its default branch is born on the first push), registers this machine (a
// human-readable dir name + a manifest carrying the stable machine id), pushes
// the registration, and writes the config Load reads from then on.
//
// Init refuses a machine dir already claimed by a DIFFERENT machine_id — pick
// another name with --name. A live config also refuses (with a pointer at the
// file to remove); after state loss (config gone, machine-id intact) re-init
// against the same remote is idempotent — the machine reclaims its own dir.
func Init(ctx context.Context, remote, name string) (*Archive, error) {
	if remote == "" {
		return nil, errors.New("archive init: remote url required")
	}
	if _, err := readConfig(); err == nil {
		return nil, fmt.Errorf(
			"archive already initialized; remove %s (and %s) to re-initialize",
			configPath(), cloneDir())
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf(
			"archive config exists but is unreadable; remove %s to re-initialize: %w",
			configPath(), err)
	}

	if name == "" {
		name = defaultMachineName()
	}
	if err := validateMachineName(name); err != nil {
		return nil, fmt.Errorf("archive init: %w", err)
	}

	a := newArchive(Config{Remote: remote, Name: name})

	// Same single-writer lock the sync verbs hold: a re-init's RemoveAll must
	// never race a still-running background sync (spawned before the old
	// config was removed) over the same clone path.
	release, err := acquireSyncLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// A clone left by a previous failed/abandoned init is stale state, not
	// truth — the remote is truth. Start from a fresh clone — but never
	// silently destroy unpushed work (same doctrine as ensureClone's rebuild
	// guard): a leftover clone holding commits no remote has (config lost
	// after a sync committed but before it pushed) is refused with the
	// recovery path. A clone too torn to even answer the probe is the
	// documented start-fresh case and proceeds.
	if _, err := os.Stat(filepath.Join(a.clone, ".git")); err == nil {
		if n, serr := a.strandedCommits(ctx); serr == nil && n > 0 {
			return nil, fmt.Errorf(
				"existing clone %s holds %d unpushed commit(s); push or back them up (git -C %s status), or delete the dir, then re-run init",
				a.clone, n, a.clone)
		}
	}
	if err := os.RemoveAll(a.clone); err != nil {
		return nil, fmt.Errorf("clear stale clone: %w", err)
	}
	if err := a.ensureClone(ctx); err != nil {
		return nil, err
	}
	if err := a.ensureRegistered(); err != nil {
		return nil, err
	}

	// Commit + push the registration so the claim is visible to other machines
	// immediately (and the name-claim race window stays one push wide).
	changed, err := a.stageMachineDir(ctx)
	if err != nil {
		return nil, err
	}
	if changed {
		if err := a.commit(ctx, fmt.Sprintf("%s: register machine", a.cfg.Name)); err != nil {
			return nil, err
		}
		if _, err := a.pushWithRetry(ctx); err != nil {
			return nil, err
		}
	}

	if err := writeConfig(a.cfg); err != nil {
		return nil, err
	}
	return a, nil
}

// ensureRegistered guarantees this machine's dir in the clone carries a
// manifest with OUR machine id. A dir claimed by a different id is a hard
// conflict; an unclaimed dir (or one missing its manifest) is claimed now.
// The manifest is rewritten only when its content actually changed, so a
// routine push of an already-registered machine stages nothing here.
func (a *Archive) ensureRegistered() error {
	dir := a.machineDir()
	cur, err := readManifest(dir)
	switch {
	case err == nil && cur.MachineID != a.machineID:
		return fmt.Errorf(
			"machine dir %q in the archive is already claimed by another machine (machine_id %s); choose a different name with --name",
			a.cfg.Name, cur.MachineID)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("read machine manifest: %w", err)
	}

	host, herr := os.Hostname()
	if herr != nil {
		host = "unknown"
	}
	if err == nil && cur.Name == a.cfg.Name && cur.Hostname == host {
		return nil // already registered, nothing changed
	}
	return writeManifest(dir, manifest{
		MachineID: a.machineID,
		Name:      a.cfg.Name,
		Hostname:  host,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
