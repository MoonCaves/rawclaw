// Package archive implements the transcript archive: one git repository shared
// by all of a user's machines, holding each machine's raw transcript trees
// under a top-level directory per machine (`<machine>/<source>/...`). The repo
// is the durable home for raw session bytes — local index dbs stay disposable
// derived caches.
//
// All git knowledge, clone-layout knowledge, and machine-manifest handling live
// behind this package's small interface; nothing else in the codebase learns
// git. Git runs via the system binary through the unexported run seam (one real
// exec adapter, a fake for unit tests).
package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/provenance"
)

// Archive is a configured transcript archive: a remote, this machine's dir
// name, and the local clone the push/pull verbs operate on. Obtain one via
// Load (nil when the feature is unconfigured) or Init.
type Archive struct {
	cfg       Config
	clone     string     // local clone path, under the state dir
	machineID string     // this machine's stable id (provenance.MachineID)
	run       runGitFunc // git seam: the exec adapter, swapped by unit tests
}

// PushReport summarizes one PushLocal run for status output and logging.
type PushReport struct {
	Copied    int  // files copied into the clone this push
	Removed   int  // tombstoned own sessions removed from the clone this push
	Committed bool // a commit was created (false = nothing changed)
	Pushed    bool // the commit reached the remote
	Retries   int  // rebase-retry rounds needed before the push landed
}

// Load resolves the archive configuration (env + config file). It returns
// (nil, nil) when unconfigured — every caller treats nil as "feature off", so
// the zero state costs one nil-check. RAWCLAW_ARCHIVE=off force-disables a
// configured archive. A present-but-unreadable config is an error: the user
// configured the feature and deserves to know it broke.
func Load() (*Archive, error) {
	if strings.EqualFold(os.Getenv("RAWCLAW_ARCHIVE"), "off") {
		return nil, nil
	}
	cfg, err := readConfig()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read archive config: %w", err)
	}
	return newArchive(cfg), nil
}

// newArchive assembles a ready Archive for cfg with the real git adapter.
func newArchive(cfg Config) *Archive {
	return &Archive{
		cfg:       cfg,
		clone:     cloneDir(),
		machineID: provenance.MachineID(),
		run:       runGit,
	}
}

// Remote returns the configured archive remote URL.
func (a *Archive) Remote() string { return a.cfg.Remote }

// Name returns this machine's top-level dir name in the archive.
func (a *Archive) Name() string { return a.cfg.Name }

// ClonePath returns the local clone path the verbs operate on.
func (a *Archive) ClonePath() string { return a.clone }
