package archive

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
)

// ForeignProjectMatches reports the foreign machine names whose dir name or
// scope labels contain the project substring — the delete verb's guard.
// Foreign sessions are read-only from every box (no cross-machine delete in
// v1), so a delete filter that reaches into a foreign machine's scopes must
// be named, never silently ignored. Matching mirrors what search shows the
// user: the machine name and the "<machine>/<label>" scope labels — labels
// rebuilt here from the same layout claudeScopes/codexScopes read, rather
// than through Scopes() itself, because Scopes() ingests every scope into
// its cache db (far too heavy for a delete-time guard). Offline, clone-only;
// an absent clone matches nothing. A machine matches only if it actually
// holds at least one matching SESSION-bearing scope (or its name matches and
// it holds any sessions at all): naming a machine with nothing in it would
// send the user chasing sessions that do not exist.
func (a *Archive) ForeignProjectMatches(project string) []string {
	if project == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(a.clone, ".git", cloneSentinel)); err != nil {
		return nil // no usable clone: nothing foreign is reachable (or warn-worthy)
	}
	var hits []string
	for _, m := range a.foreignMachines() {
		if a.foreignMachineMatches(m, project) {
			hits = append(hits, m.Name)
		}
	}
	return hits
}

// foreignMachineMatches reports whether one foreign machine's name or any of
// its session-bearing scope labels contain project. A name-only match still
// requires the machine to hold at least one session.
func (a *Archive) foreignMachineMatches(m manifest, project string) bool {
	nameHit := strings.Contains(m.Name, project)
	hasSessions := false

	claudeRoot := filepath.Join(a.clone, m.Name, "claude")
	if entries, err := os.ReadDir(claudeRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() || !hasTopLevelJSONL(filepath.Join(claudeRoot, e.Name())) {
				continue // same session-bearing filter the scope enumeration applies
			}
			hasSessions = true
			label := m.Name + "/" + paths.ProjectLabel(filepath.Join(claudeRoot, e.Name()))
			if strings.Contains(label, project) {
				return true
			}
		}
	}
	if containers, err := codex.New().DiscoverRoot(filepath.Join(a.clone, m.Name, "codex")); err == nil {
		for _, c := range containers {
			hasSessions = true
			if strings.Contains(m.Name+"/"+codexGroupLabel(c.CWD), project) {
				return true
			}
		}
	}
	return nameHit && hasSessions
}
