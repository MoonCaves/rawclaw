package scopes

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCodexDBPath_Injective guards the cross-prune contract (see
// index.EnsureIndexedContainers): each Codex cwd group is ingested into its OWN
// db and passed as the COMPLETE set for that db. If two DISTINCT cwds map to the
// same db path they share one db, and each call then prunes the other's sessions
// (or wipes them under reindex) — the exact contract violation the commit warns
// about, WITHIN Codex.
//
// encodeCWD's readable slug is lossy ('/', '.', literal '-' all fold to '-'), so
// codexDBPath appends a hash of the full cwd to stay injective. These pairs would
// collide on the slug alone; they must map to distinct db paths.
func TestCodexDBPath_Injective(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{"/Users/a/b", "/Users/a-b"}, // '/' vs literal '-'
		{"/Users/a.b", "/Users/a-b"}, // '.' vs literal '-'
		{"/x/y", "/x.y"},             // '/' vs '.'
	}
	for _, p := range pairs {
		if got0, got1 := codexDBPath(p[0]), codexDBPath(p[1]); got0 == got1 {
			t.Errorf("codexDBPath collision: %q and %q both -> %q; distinct cwds share a db and cross-prune", p[0], p[1], got0)
		}
	}
}

// TestCodexDBPath_PrefixedAwayFromClaude guards the property the "codex-" prefix
// exists to guarantee: a Codex cwd group can never land on the Claude project db
// for the same cwd.
func TestCodexDBPath_PrefixedAwayFromClaude(t *testing.T) {
	t.Parallel()
	base := filepath.Base(codexDBPath("/Users/octocat/proj"))
	if !strings.HasPrefix(base, "codex-") {
		t.Errorf("codexDBPath base %q lacks the codex- prefix that separates it from the Claude db", base)
	}
}

// TestCodexDBPath_Stable pins that a given cwd (including empty) maps to the SAME
// db across runs — the hash is deterministic, so a cwd group keeps its index.
func TestCodexDBPath_Stable(t *testing.T) {
	t.Parallel()
	for _, cwd := range []string{"", "/Users/octocat/proj"} {
		if a, b := codexDBPath(cwd), codexDBPath(cwd); a != b {
			t.Errorf("codexDBPath(%q) not stable: %q vs %q", cwd, a, b)
		}
	}
	if got := encodeCWD(""); got != "unknown" {
		t.Errorf("encodeCWD(%q) = %q, want %q", "", got, "unknown")
	}
}
