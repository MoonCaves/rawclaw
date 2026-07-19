package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeIndexedSession writes a real (parseable) transcript under
// <root>/<project>/<id>.jsonl so browse/stats can index it: one substantive
// user message with a timestamp.
func writeIndexedSession(t *testing.T, root, project, id, ts, text string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","uuid":"` + id + `-u0","timestamp":"` + ts + `",` +
		`"message":{"role":"user","content":"` + text + `"}}`
	p := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(p, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// emptyQueryLine is the exact coaching line `rawclaw ""` must print — a
// distinct message, NOT the no-matches coaching.
const emptyQueryLine = "Empty query. Add a search term, or run bare rawclaw to browse this folder (--all for every project).\n"

// TestEmptyQueryPrintsCoaching: `rawclaw ""` prints exactly the empty-query
// line (no search runs, no no-matches coaching).
func TestEmptyQueryPrintsCoaching(t *testing.T) {
	newCfgRoot(t)

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "")
	if err != nil {
		t.Fatalf("rawclaw \"\": %v\n%s", err, out)
	}
	if out != emptyQueryLine {
		t.Errorf("rawclaw \"\" output = %q, want exactly %q", out, emptyQueryLine)
	}
}

// TestEmptyQueryWhitespaceOnly: an all-whitespace query is the same empty
// query (the join/trim seam, not a literal-"" special case).
func TestEmptyQueryWhitespaceOnly(t *testing.T) {
	newCfgRoot(t)

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "  ", "")
	if err != nil {
		t.Fatalf("rawclaw '  ' '': %v\n%s", err, out)
	}
	if out != emptyQueryLine {
		t.Errorf("whitespace query output = %q, want exactly %q", out, emptyQueryLine)
	}
}

// TestBrowseNoHistoryPointsAtAll: the no-history hint names both escapes —
// --list and --all.
func TestBrowseNoHistoryPointsAtAll(t *testing.T) {
	newCfgRoot(t)

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("bare browse: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Try --list, or --all for every project.") {
		t.Errorf("no-history hint missing the --all pointer:\n%s", out)
	}
}

// TestBrowseAllCoversEveryProject: bare browse honors --all — sessions from
// every project appear, newest first, each row naming its project. The --dir
// (which has no history of its own) must not matter.
func TestBrowseAllCoversEveryProject(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "older question about apples")
	writeIndexedSession(t, root, "-home-u-proj-b", "bbbb2222-0000-0000-0000-000000000002",
		"2026-06-02T10:00:00Z", "newer question about bananas")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("browse --all: %v\n%s", err, out)
	}
	if strings.Contains(out, "No transcript history") {
		t.Fatalf("browse --all fell back to the single-folder no-history hint:\n%s", out)
	}
	for _, want := range []string{"aaaa1111", "bbbb2222", "proj-a", "proj-b", "apples", "bananas"} {
		if !strings.Contains(out, want) {
			t.Errorf("browse --all missing %q:\n%s", want, out)
		}
	}
	// Newest first across projects.
	if strings.Index(out, "bbbb2222") > strings.Index(out, "aaaa1111") {
		t.Errorf("browse --all not newest-first across projects:\n%s", out)
	}
}

// TestBrowseAllJSON: the --all --json browse shape is scope-tagged and each
// session row carries its project.
func TestBrowseAllJSON(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "a question")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all", "--json", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("browse --all --json: %v\n%s", err, out)
	}
	var got struct {
		Scope    string `json:"scope"`
		Sessions []struct {
			Project   string `json:"project"`
			SessionID string `json:"session_id"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("browse --all --json is not JSON: %v\n%s", err, out)
	}
	if got.Scope != "all" {
		t.Errorf("scope = %q, want %q", got.Scope, "all")
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Project == "" ||
		!strings.HasPrefix(got.Sessions[0].SessionID, "aaaa1111") {
		t.Errorf("sessions = %+v", got.Sessions)
	}
}

// TestStatsAllFromHistorylessDir pins that --stats honors --all even when the
// current --dir has no transcript history of its own: the corpus aggregate
// renders, not the single-folder no-history hint.
func TestStatsAllFromHistorylessDir(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "a question")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--stats", "--all", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--stats --all: %v\n%s", err, out)
	}
	if strings.Contains(out, "No transcript history") {
		t.Fatalf("--stats --all fell back to the single-folder no-history hint:\n%s", out)
	}
	if !strings.Contains(out, "RawClaw corpus") || !strings.Contains(out, "sessions") {
		t.Errorf("--stats --all missing the corpus aggregate:\n%s", out)
	}
}
