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
// user: the machine name and the "<machine>/<label>" scope labels the
// foreign enumeration builds. Offline, clone-only; an absent clone matches
// nothing.
func (a *Archive) ForeignProjectMatches(project string) []string {
	if project == "" {
		return nil
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
// its scope labels contain project.
func (a *Archive) foreignMachineMatches(m manifest, project string) bool {
	if strings.Contains(m.Name, project) {
		return true
	}
	claudeRoot := filepath.Join(a.clone, m.Name, "claude")
	if entries, err := os.ReadDir(claudeRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			label := m.Name + "/" + paths.ProjectLabel(filepath.Join(claudeRoot, e.Name()))
			if strings.Contains(label, project) {
				return true
			}
		}
	}
	if containers, err := codex.New().DiscoverRoot(filepath.Join(a.clone, m.Name, "codex")); err == nil {
		for _, c := range containers {
			if strings.Contains(m.Name+"/"+codexGroupLabel(c.CWD), project) {
				return true
			}
		}
	}
	return false
}
