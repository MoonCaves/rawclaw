package archive

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
)

// removeTombstoned removes tombstoned OWN sessions' files from the clone so
// the next commit propagates the deletion — an explicit `delete` is never
// resurrected by the archive. Propagation is driven ONLY by the tombstone
// sidecar (recorded intent); file absence never deletes anything, which is
// what keeps the archive a durable mirror under every retention setting.
//
// Only this machine's dir is touched: foreign sessions are read-only from
// every box (no cross-machine delete in v1). Claude sessions are addressed by
// file stem (the stem IS the session id); codex rollouts carry their id in
// the session_meta header, so those are resolved through the codex adapter
// against the clone's own tree. Runs AFTER the copy pass, so a tombstoned
// session whose local file still exists is copied and then removed in the
// same run — the tombstone outranks presence.
func (a *Archive) removeTombstoned() (int, error) {
	tombs, err := lifecycle.LoadTombstones("")
	if err != nil {
		return 0, fmt.Errorf("read delete tombstones: %w", err)
	}
	if len(tombs) == 0 {
		return 0, nil
	}

	removed := 0
	// Claude: <machine>/claude/<project-dir>/<session-id>.jsonl — stem = id.
	claudeRoot := filepath.Join(a.machineDir(), "claude")
	werr := filepath.WalkDir(claudeRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil // unreadable entries: leave them; deletion must be deliberate
		}
		stem := strings.TrimSuffix(d.Name(), ".jsonl")
		if _, ok := tombs[stem]; !ok {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove tombstoned session from clone: %w", err)
		}
		removed++
		return nil
	})
	if werr != nil && !os.IsNotExist(werr) {
		return removed, werr
	}

	// Codex: rollout ids live in the session_meta header, not the filename —
	// resolve them through the same adapter the scopes use, pointed at the
	// clone's own codex tree. An absent tree yields no containers.
	containers, derr := codex.New().DiscoverRoot(filepath.Join(a.machineDir(), "codex"))
	if derr != nil {
		return removed, fmt.Errorf("resolve codex sessions in clone: %w", derr)
	}
	for _, c := range containers {
		if _, ok := tombs[c.ID]; !ok {
			continue
		}
		if err := os.Remove(c.Path); err != nil {
			return removed, fmt.Errorf("remove tombstoned codex rollout from clone: %w", err)
		}
		removed++
	}
	return removed, nil
}
