package archive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/paths"
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
// against the clone's own tree. Runs after the copy pass, which itself skips
// tombstoned sources — between them a tombstoned session can neither survive
// in nor re-enter the archive.
func (a *Archive) removeTombstoned(ctx context.Context, tombs map[string]struct{}) (int, error) {
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
		if cerr := ctx.Err(); cerr != nil {
			return cerr // honor cancellation between removals
		}
		stem := strings.TrimSuffix(d.Name(), ".jsonl")
		if _, ok := tombs[stem]; !ok {
			return nil
		}
		if rerr := os.Remove(path); rerr != nil {
			if errors.Is(rerr, fs.ErrNotExist) {
				return nil // vanished concurrently: the deletion already holds
			}
			return fmt.Errorf("remove tombstoned session from clone: %w", rerr)
		}
		removed++
		return nil
	})
	if werr != nil {
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
		if cerr := ctx.Err(); cerr != nil {
			return removed, cerr
		}
		if _, ok := tombs[c.ID]; !ok {
			continue
		}
		if err := os.Remove(c.Path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return removed, fmt.Errorf("remove tombstoned codex rollout from clone: %w", err)
		}
		removed++
	}
	return removed, nil
}

// tombstonedSources resolves the tombstone set against one LOCAL source tree,
// returning the set of source file paths that must never be copied into the
// archive — the copy-side half of delete propagation, covering a tombstoned
// session whose local file still exists (restored backup, delete race).
// Without it, such a zombie would be re-copied and re-removed on every push,
// inflating the report and churning I/O forever. Claude files resolve by
// stem; codex rollouts by their session_meta id (header reads, paid only
// when tombstones exist at all).
func tombstonedSources(tree sourceTree, tombs map[string]struct{}) map[string]struct{} {
	if len(tombs) == 0 {
		return nil
	}
	skip := map[string]struct{}{}
	switch tree.id {
	case "claude":
		for _, src := range paths.ContainedJSONL(tree.root) {
			stem := strings.TrimSuffix(filepath.Base(src), ".jsonl")
			if _, ok := tombs[stem]; ok {
				skip[src] = struct{}{}
			}
		}
	case "codex":
		containers, err := codex.New().DiscoverRoot(tree.root)
		if err != nil {
			return skip // enumeration failure: copy as before, the rm pass still guards
		}
		for _, c := range containers {
			if _, ok := tombs[c.ID]; ok {
				skip[c.Path] = struct{}{}
			}
		}
	}
	return skip
}
