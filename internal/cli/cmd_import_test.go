package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/source/claudeweb"
)

// isolate points every rawclaw state dir (including the claude-web transcript
// root) at a fresh temp HOME so the import→search e2e touches no real data.
func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "nocodex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
}

func writeExport(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "conversations.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// searchClaudeWeb runs the REAL search path (scope enumeration → per-account
// index rebuilt from the transcript files → FTS) for a needle, returning the
// matching session ids.
func searchClaudeWeb(t *testing.T, needle string) []string {
	t.Helper()
	scope := scopes.All(context.Background(), claudeweb.ID, false)
	env := agentproto.Search(needle, scope, agentproto.SearchOpts{}, nil)
	ids := make([]string, 0, len(env.Results))
	for _, r := range env.Results {
		ids = append(ids, r.SessionID)
	}
	return ids
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// TestImportSearchRoundTrip is the end-to-end guard: `rawclaw import` an export,
// then a real FTS search returns the imported cloud conversation — exercising
// materialize → reindex-from-files → search, not just the per-piece units.
func TestImportSearchRoundTrip(t *testing.T) {
	isolate(t)
	exp := writeExport(t, `[{"uuid":"c1111111-1111-1111-1111-111111111111","account":{"uuid":"acc00001-0000-0000-0000-000000000000"},"created_at":"2026-07-15T10:00:00Z","updated_at":"2026-07-15T10:00:00Z","chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"needleinthecloudhaystack"}]}]}]`)

	if err := runImport(io.Discard, exp, false); err != nil {
		t.Fatalf("import: %v", err)
	}
	if ids := searchClaudeWeb(t, "needleinthecloudhaystack"); !contains(ids, "c1111111-1111-1111-1111-111111111111") {
		t.Errorf("import→search round-trip failed: %v", ids)
	}
}

// TestImportSearchMirrorAndKeep is the retention/search e2e (the class the
// mirror-wipe once slipped past): under mirror a conversation dropped from a
// FRESHER re-export disappears from search; under keep it is retained.
func TestImportSearchMirrorAndKeep(t *testing.T) {
	const acc = `"account":{"uuid":"acc00002-0000-0000-0000-000000000000"}`
	twoConvs := `[` +
		`{"uuid":"c1",` + acc + `,"updated_at":"2026-07-15T10:00:00Z","chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"keepneedle"}]}]},` +
		`{"uuid":"c2",` + acc + `,"updated_at":"2026-07-15T10:00:00Z","chat_messages":[{"uuid":"m2","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"pruneneedle"}]}]}]`
	fresherOne := `[{"uuid":"c1",` + acc + `,"updated_at":"2026-07-20T10:00:00Z","chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-20T10:00:00Z","content":[{"type":"text","text":"keepneedle"}]}]}]`

	t.Run("mirror prunes the absent conversation from search", func(t *testing.T) {
		isolate(t)
		t.Setenv("RAWCLAW_RETENTION", "mirror")
		if err := runImport(io.Discard, writeExport(t, twoConvs), false); err != nil {
			t.Fatal(err)
		}
		if ids := searchClaudeWeb(t, "pruneneedle"); !contains(ids, "c2") {
			t.Fatalf("c2 not searchable after first import: %v", ids)
		}
		if err := runImport(io.Discard, writeExport(t, fresherOne), false); err != nil {
			t.Fatal(err)
		}
		if ids := searchClaudeWeb(t, "pruneneedle"); contains(ids, "c2") {
			t.Errorf("c2 still searchable after a fresher mirror re-import that dropped it: %v", ids)
		}
		if ids := searchClaudeWeb(t, "keepneedle"); !contains(ids, "c1") {
			t.Errorf("c1 must survive: %v", ids)
		}
	})

	t.Run("keep retains the absent conversation in search", func(t *testing.T) {
		isolate(t) // no RAWCLAW_RETENTION → keep
		if err := runImport(io.Discard, writeExport(t, twoConvs), false); err != nil {
			t.Fatal(err)
		}
		if err := runImport(io.Discard, writeExport(t, fresherOne), false); err != nil {
			t.Fatal(err)
		}
		if ids := searchClaudeWeb(t, "pruneneedle"); !contains(ids, "c2") {
			t.Errorf("c2 was dropped under keep (keep must retain absent conversations): %v", ids)
		}
	})
}
