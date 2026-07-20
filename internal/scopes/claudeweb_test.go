package scopes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source/claudeweb"
)

const (
	acctA = "aaaaaaaa-1111-2222-3333-444444444444"
	acctB = "bbbbbbbb-5555-6666-7777-888888888888"
)

// seedAccount materializes one account's transcript tree (as `rawclaw import`
// does), so ClaudeWeb() discovers it as the source of truth and rebuilds its
// cache db. Requires an isolated XDG_DATA_HOME (emptyStore sets it).
func seedAccount(t *testing.T, account string) {
	t.Helper()
	exp := t.TempDir()
	body := fmt.Sprintf(`[{"uuid":"conv-%s","account":{"uuid":%q},"chat_messages":[`+
		`{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"beacon %s"}]}]}]`,
		account, account, account)
	if err := os.WriteFile(filepath.Join(exp, "conversations.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := claudeweb.Materialize(exp, paths.ClaudeWebRoot(), false); err != nil {
		t.Fatalf("seed account %s: %v", account, err)
	}
}

// TestClaudeWeb_TwoAccountsTwoScopes: two imported accounts surface as two
// distinct acct-<uuid8> scopes (source claude-web, no CWD), zero user input.
func TestClaudeWeb_TwoAccountsTwoScopes(t *testing.T) {
	emptyStore(t)
	seedAccount(t, acctA)
	seedAccount(t, acctB)

	scs := ClaudeWeb()
	if len(scs) != 2 {
		t.Fatalf("want 2 account scopes, got %d: %+v", len(scs), scs)
	}
	for _, s := range scs {
		if s.Source != claudeweb.ID {
			t.Errorf("scope source = %q, want claude-web", s.Source)
		}
		if s.CWD != "" {
			t.Errorf("claude-web scope must have empty CWD (excluded from --this-project), got %q", s.CWD)
		}
		if !strings.HasPrefix(s.Project, "acct-") {
			t.Errorf("scope label %q must be an acct-<uuid8> id", s.Project)
		}
	}
	if scs[0].Project == scs[1].Project {
		t.Errorf("two accounts collapsed to one label: %q", scs[0].Project)
	}
}

// TestClaudeWeb_OldFormatDBNotServed is the fail-closed old-format guard: a
// claude-web cache db with NO backing transcript tree (the pre-materialize
// shape) must NOT be served — serving it would stale-serve and a future rebuild
// would empty it (no files to rebuild from).
func TestClaudeWeb_OldFormatDBNotServed(t *testing.T) {
	emptyStore(t)
	// A cache db at the account's path, but no transcript tree for it.
	orphan := ClaudeWebDBPath("orphan00-1111-2222-3333-444444444444")
	if err := os.MkdirAll(filepath.Dir(orphan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	scs := ClaudeWeb()
	for _, s := range scs {
		if s.DBP == orphan {
			t.Errorf("old-format claude-web db (no backing tree) was served: %+v", s)
		}
	}
	if len(scs) != 0 {
		t.Errorf("no transcript tree → no claude-web scopes, got %+v", scs)
	}
	// A subsequent import of that account creates the tree → it IS then served.
	seedAccount(t, "orphan00-1111-2222-3333-444444444444")
	if got := ClaudeWeb(); len(got) != 1 {
		t.Errorf("after materializing the tree, want 1 scope, got %+v", got)
	}
}

// TestClaudeWebDBPath_SlashFree: no "/" or ":" in any account identifier,
// verified against db-name AND scope-label formation, even for pathological
// account strings.
func TestClaudeWebDBPath_SlashFree(t *testing.T) {
	for _, account := range []string{acctA, "weird/slashes:colons", "acct/../escape", "::::"} {
		dbp := ClaudeWebDBPath(account)
		if strings.ContainsAny(filepath.Base(dbp), "/:") {
			t.Errorf("db name for %q contains / or :", account)
		}
		if strings.ContainsAny(accountSlug(account), "/:") {
			t.Errorf("account slug for %q contains / or :", account)
		}
		if strings.ContainsAny(accountLabelFromDB(dbp), "/:") {
			t.Errorf("scope label for %q contains / or :", account)
		}
	}
}

// TestClaudeWebDBPath_Injective: two accounts sharing the first 8 chars still map
// to DISTINCT dbs (the full-key hash), so one account can never merge into another.
func TestClaudeWebDBPath_Injective(t *testing.T) {
	a := "abcd1234-0000-0000-0000-000000000001"
	b := "abcd1234-0000-0000-0000-000000000002"
	if accountSlug(a) != accountSlug(b) {
		t.Fatalf("test premise broken: slugs differ (%q vs %q)", accountSlug(a), accountSlug(b))
	}
	if ClaudeWebDBPath(a) == ClaudeWebDBPath(b) {
		t.Errorf("accounts sharing an 8-char slug collided on one db: %q", ClaudeWebDBPath(a))
	}
}
