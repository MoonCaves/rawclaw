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

// TestBrowseIncludePathScopesAcrossProjects: a no-query browse honors
// --include-path the way search does — the path flag selects projects by
// working dir, so it covers every matching project rather than the cwd, and the
// header names THAT scope. The regression it pins: `rawclaw --include-path X`
// used to ignore the flag and browse the shell's cwd under a header naming the
// cwd's project.
func TestBrowseIncludePathScopesAcrossProjects(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "older question about apples")
	writeIndexedSession(t, root, "-home-u-proj-b", "bbbb2222-0000-0000-0000-000000000002",
		"2026-06-02T10:00:00Z", "newer question about bananas")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--include-path", "proj-a", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("browse --include-path: %v\n%s", err, out)
	}
	if !strings.Contains(out, "aaaa1111") || !strings.Contains(out, "apples") {
		t.Errorf("browse --include-path dropped the matching project:\n%s", out)
	}
	if strings.Contains(out, "bbbb2222") || strings.Contains(out, "bananas") {
		t.Errorf("browse --include-path kept a project the filter excludes:\n%s", out)
	}
	if strings.Contains(out, "No transcript history") {
		t.Errorf("browse --include-path fell back to the cwd's single-folder path:\n%s", out)
	}
	// The header must name the scope asked for, never the wider one.
	if !strings.Contains(out, "matching --include-path proj-a") {
		t.Errorf("browse --include-path header does not name the path scope:\n%s", out)
	}
	if strings.Contains(out, "across all projects:") {
		t.Errorf("browse --include-path header claims the unfiltered scope:\n%s", out)
	}
}

// TestBrowseExcludePathDropsProject: --exclude-path is the same Scope flag from
// the other side — the excluded project's sessions must not appear.
func TestBrowseExcludePathDropsProject(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "older question about apples")
	writeIndexedSession(t, root, "-home-u-proj-b", "bbbb2222-0000-0000-0000-000000000002",
		"2026-06-02T10:00:00Z", "newer question about bananas")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--exclude-path", "proj-b", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("browse --exclude-path: %v\n%s", err, out)
	}
	if !strings.Contains(out, "aaaa1111") {
		t.Errorf("browse --exclude-path dropped a project it should keep:\n%s", out)
	}
	if strings.Contains(out, "bbbb2222") {
		t.Errorf("browse --exclude-path kept the excluded project:\n%s", out)
	}
}

// TestBrowseIncludePathNoMatchIsHonest: a path scope that matches no project is
// reported as empty WITH its real boundary — never relaxed into a wider browse,
// and never silently answered from the cwd. Exit 0: an empty scope is an answer.
func TestBrowseIncludePathNoMatchIsHonest(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "older question about apples")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--include-path", "no-such-project", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("browse --include-path with no match should exit 0: %v\n%s", err, out)
	}
	if strings.Contains(out, "aaaa1111") || strings.Contains(out, "apples") {
		t.Errorf("browse relaxed an unmatched path scope and returned rows anyway:\n%s", out)
	}
	for _, want := range []string{"No project matches --include-path no-such-project", "0 of 1", "--list"} {
		if !strings.Contains(out, want) {
			t.Errorf("unmatched path scope missing %q in its honest empty:\n%s", want, out)
		}
	}
}

// TestBrowseIncludePathJSON: the machine shape carries the scope it covered —
// the path flag verbatim and the surviving project count — so `sessions: []`
// from an unmatched filter is distinguishable from an empty corpus.
func TestBrowseIncludePathJSON(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "a question")
	writeIndexedSession(t, root, "-home-u-proj-b", "bbbb2222-0000-0000-0000-000000000002",
		"2026-06-02T10:00:00Z", "another question")

	type browseJSON struct {
		Scope       string `json:"scope"`
		IncludePath string `json:"include_path"`
		Projects    int    `json:"projects"`
		Sessions    []struct {
			Project   string `json:"project"`
			SessionID string `json:"session_id"`
		} `json:"sessions"`
	}

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--include-path", "proj-a", "--json", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("browse --include-path --json: %v\n%s", err, out)
	}
	var got browseJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("browse --include-path --json is not JSON: %v\n%s", err, out)
	}
	if got.IncludePath != "proj-a" || got.Projects != 1 {
		t.Errorf("scope report = %+v, want include_path=proj-a projects=1", got)
	}
	if len(got.Sessions) != 1 || !strings.HasPrefix(got.Sessions[0].SessionID, "aaaa1111") {
		t.Errorf("sessions = %+v, want only the proj-a session", got.Sessions)
	}

	// The unmatched case: same shape, zero projects, empty (not null) sessions.
	out, err = runCmd(t, NewRootCmd(BuildInfo{}), "", "--include-path", "no-such-project", "--json", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("browse --include-path --json (no match): %v\n%s", err, out)
	}
	var empty browseJSON
	if err := json.Unmarshal([]byte(out), &empty); err != nil {
		t.Fatalf("unmatched browse --json is not JSON: %v\n%s", err, out)
	}
	if empty.Projects != 0 || len(empty.Sessions) != 0 || empty.IncludePath != "no-such-project" {
		t.Errorf("unmatched scope report = %+v, want projects=0 sessions=[] include_path echoed", empty)
	}
	if !strings.Contains(out, `"sessions": []`) {
		t.Errorf("unmatched browse --json emitted null sessions instead of []:\n%s", out)
	}
}

// TestBrowseThisProjectAndPathAreHardAND: Scope flags AND together. With
// --this-project the universe is the one project, and a path flag that rejects
// it empties the browse rather than being dropped so the project prints anyway.
func TestBrowseThisProjectAndPathAreHardAND(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "a question")
	// The encoded transcripts dir is accepted verbatim as --dir, so the test
	// needs no real working dir on disk.
	tdir := filepath.Join(root, "-home-u-proj-a")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--this-project", "--dir", tdir, "--include-path", "proj-a")
	if err != nil {
		t.Fatalf("--this-project --include-path (match): %v\n%s", err, out)
	}
	if !strings.Contains(out, "aaaa1111") {
		t.Errorf("--this-project --include-path dropped the project it matches:\n%s", out)
	}

	out, err = runCmd(t, NewRootCmd(BuildInfo{}), "", "--this-project", "--dir", tdir, "--include-path", "proj-b")
	if err != nil {
		t.Fatalf("--this-project --include-path (mismatch): %v\n%s", err, out)
	}
	if strings.Contains(out, "aaaa1111") {
		t.Errorf("--this-project won over the path scope instead of ANDing with it:\n%s", out)
	}
	if !strings.Contains(out, "No project matches --include-path proj-b") {
		t.Errorf("mismatched --this-project scope is not reported honestly:\n%s", out)
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
