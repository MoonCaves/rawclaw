package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/archive/archivetest"
)

// foreignClonePath is the pulled foreign session's path inside the local clone.
func foreignClonePath(fx *archivetest.Fixture) string {
	return filepath.Join(fx.Archive.ClonePath(),
		archivetest.ForeignName, "claude", "-remote-proj", archivetest.ForeignSession+".jsonl")
}

// TestDeleteCmd_ForeignOnlyRefuses: a delete whose filter matches ONLY
// another machine's archived sessions is refused with a clear error naming
// the machine — foreign sessions are read-only from every box; nothing is
// deleted and nothing is tombstoned.
func TestDeleteCmd_ForeignOnlyRefuses(t *testing.T) {
	fx := archivetest.Setup(t, "")

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "delete", "--project", archivetest.ForeignName, "--yes")
	if err == nil {
		t.Fatalf("delete of foreign-only match succeeded, want refusal\n%s", out)
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("refusal should say foreign sessions are read-only, got: %v", err)
	}
	if !strings.Contains(err.Error(), archivetest.ForeignName) {
		t.Errorf("refusal should name the origin machine, got: %v", err)
	}

	if _, serr := os.Stat(foreignClonePath(fx)); serr != nil {
		t.Errorf("foreign session touched by refused delete: %v", serr)
	}
	tomb := filepath.Join(fx.Home, ".cache", "session-search", ".deleted")
	if b, rerr := os.ReadFile(tomb); rerr == nil && strings.Contains(string(b), archivetest.ForeignSession) {
		t.Error("refused delete tombstoned a foreign session")
	}
}

// TestDeleteCmd_ForeignMatchesNotedLocalStillDeletes: when the filter matches
// both local and foreign sessions, the local delete proceeds and the foreign
// half is called out as read-only — never silently skipped, never touched.
func TestDeleteCmd_ForeignMatchesNotedLocalStillDeletes(t *testing.T) {
	fx := archivetest.Setup(t, "")

	// "proj" substring-matches the local "-local-proj" AND the foreign
	// "-remote-proj" scopes.
	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "delete", "--project", "proj", "--yes", "--files")
	if err != nil {
		t.Fatalf("delete with mixed matches: %v\n%s", err, out)
	}
	if !strings.Contains(out, "read-only") {
		t.Errorf("output should note the read-only foreign matches:\n%s", out)
	}

	local := filepath.Join(fx.Home, ".claude", "projects", "-local-proj", "localsess.jsonl")
	if _, serr := os.Stat(local); !os.IsNotExist(serr) {
		t.Error("local session not deleted")
	}
	if _, serr := os.Stat(foreignClonePath(fx)); serr != nil {
		t.Errorf("foreign session touched by a local delete: %v", serr)
	}
}
