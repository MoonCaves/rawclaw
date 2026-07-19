package archive

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
)

// sourceTree pairs a source id (the repo-layout segment under the machine dir)
// with the local transcript root it mirrors.
type sourceTree struct {
	id   string // layout segment: <machine>/<id>/...
	root string // local tree; "" or absent dirs are skipped
}

// sourceTrees returns the transcript trees this machine pushes: the Claude
// projects root and the Codex sessions root. Roots that don't exist are
// enumerated as empty by the copier — an absent runtime is not an error.
func sourceTrees() []sourceTree {
	return []sourceTree{
		{id: "claude", root: paths.ProjectsRoot()},
		{id: "codex", root: codex.SessionsRoot()},
	}
}

// sanitizeMachineName maps a hostname to a safe, human-readable machine dir
// name: the first label (domain suffixes like .local dropped), lowercased,
// with anything outside [a-z0-9_-] folded to "-". Falls back to "machine"
// when nothing survives.
func sanitizeMachineName(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if i := strings.IndexByte(host, '.'); i >= 0 {
		host = host[:i]
	}
	var b strings.Builder
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "machine"
	}
	return out
}

// defaultMachineName is the sanitized hostname — the archive dir is browsed by
// humans, so readable names beat opaque ids (the manifest carries the id).
func defaultMachineName() string {
	h, err := os.Hostname()
	if err != nil {
		return "machine"
	}
	return sanitizeMachineName(h)
}

// validateMachineName rejects explicit names that would escape the machine
// dir, hide it, or act as git pathspec magic — the name flows into
// `git add/status -- <name>`, so a glob like "ma*" would stage OTHER
// machines' dirs. Only [A-Za-z0-9._-] survives, dots may not lead. Explicit
// names are validated, not silently rewritten — the user chose them and
// deserves a loud failure over a surprising rename.
func validateMachineName(name string) error {
	if name == "" {
		return errors.New("machine name is empty")
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("machine name %q must not start with a dot", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("machine name %q: only letters, digits, '.', '-', '_' are allowed", name)
		}
	}
	return nil
}
