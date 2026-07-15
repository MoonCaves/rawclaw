package scopes

import (
	"path/filepath"
	"testing"
)

// TestEncodeCWD_Injective is an adversarial edge-case guard for the cross-prune
// contract (see index.EnsureIndexedContainers): each Codex cwd group is ingested
// into its OWN db and passed as the COMPLETE set for that db. If two DISTINCT
// cwds encode to the same db key they share one db, and each call then prunes the
// other's sessions (or wipes them under reindex) — the exact contract violation
// the commit warns about, hiding WITHIN Codex.
//
// encodeCWD folds '/', '.', AND a literal '-' all onto '-', so it is not
// injective. Hyphens and dots are common in real paths, so this is a latent
// collision, not a theoretical one. A correct encoder MUST keep distinct cwds
// distinct. This test asserts that property and currently FAILS on encodeCWD —
// it documents the bug as executable evidence.
func TestEncodeCWD_Injective(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{"/Users/a/b", "/Users/a-b"}, // '/' vs literal '-'
		{"/Users/a.b", "/Users/a-b"}, // '.' vs literal '-'
		{"/x/y", "/x.y"},             // '/' vs '.'
	}
	for _, p := range pairs {
		if got0, got1 := encodeCWD(p[0]), encodeCWD(p[1]); got0 == got1 {
			t.Errorf("encodeCWD collision: %q and %q both -> %q; distinct cwds share a db and cross-prune", p[0], p[1], got0)
		}
	}
}

// TestCodexDBPath_PrefixedAwayFromClaude guards the property the "codex-" prefix
// exists to guarantee: a Codex cwd group can never land on the Claude project db
// for the same cwd. This one passes on current code and locks the guarantee in.
func TestCodexDBPath_PrefixedAwayFromClaude(t *testing.T) {
	t.Parallel()
	base := filepath.Base(codexDBPath("/Users/octocat/proj"))
	if len(base) < len("codex-") || base[:len("codex-")] != "codex-" {
		t.Errorf("codexDBPath base %q lacks the codex- prefix that separates it from the Claude db", base)
	}
}

// TestEncodeCWD_EmptyIsStable pins the empty-cwd → "unknown" grouping so
// empty-cwd sessions keep sharing one stable db across runs.
func TestEncodeCWD_EmptyIsStable(t *testing.T) {
	t.Parallel()
	if got := encodeCWD(""); got != "unknown" {
		t.Errorf("encodeCWD(%q) = %q, want %q", "", got, "unknown")
	}
}
