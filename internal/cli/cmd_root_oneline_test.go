package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
)

// writeCustomSession writes a transcript line with a specific message uuid, timestamp, and content.
func writeCustomSession(t *testing.T, root, project, id, msgUUID, ts, text string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rawJSON, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join("/home/user", strings.TrimPrefix(project, "-"))
	line := `{"type":"user","uuid":"` + msgUUID + `","timestamp":"` + ts + `","cwd":"` + cwd + `",` +
		`"message":{"role":"user","content":` + string(rawJSON) + `}}`
	p := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(p, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestOnelineFormat_SingleHit verifies that search hits in oneline mode are output
// as exactly one tab-separated line per match: <sess8>:<uuid8>\t<started_iso>\t<project>\t<snippet_prose>.
func TestOnelineFormat_SingleHit(t *testing.T) {
	root := newCfgRoot(t)
	writeCustomSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"9f3e1c20-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "distinctive keyword target text")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "distinctive", "--oneline", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("search --oneline: %v\n%s", err, out)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 line, got %d lines: %q", len(lines), out)
	}

	cols := strings.Split(lines[0], "\t")
	if len(cols) != 4 {
		t.Fatalf("want 4 tab-separated columns, got %d: %q", len(cols), lines[0])
	}

	wantRef := "aaaa1111:9f3e1c20"
	wantISO := "2026-06-01T10:00:00Z"
	wantProject := "home-u-proj-a"

	if cols[0] != wantRef {
		t.Errorf("col 0 (read_ref) = %q, want %q", cols[0], wantRef)
	}
	if cols[1] != wantISO {
		t.Errorf("col 1 (iso) = %q, want %q", cols[1], wantISO)
	}
	if cols[2] != wantProject {
		t.Errorf("col 2 (project) = %q, want %q", cols[2], wantProject)
	}
	if !strings.Contains(cols[3], "distinctive") || !strings.Contains(cols[3], "keyword target text") {
		t.Errorf("col 3 (snippet) = %q, want to contain match text", cols[3])
	}

	// Must not contain conversational headers or footers.
	for _, unwanted := range []string{"conversation(s) matching", "showing", "matches", "note:", "raw session history"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("oneline output contains conversational banner/footer %q:\n%s", unwanted, out)
		}
	}
}

// TestOnelineFormat_FormatFlagAliases verifies that --format oneline and --format line
// behave identically to --oneline.
func TestOnelineFormat_FormatFlagAliases(t *testing.T) {
	root := newCfgRoot(t)
	writeCustomSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"9f3e1c20-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "searchable phrase here")

	outOneline, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "searchable", "--oneline", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--oneline: %v", err)
	}

	outFormatOneline, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "searchable", "--format", "oneline", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--format oneline: %v", err)
	}

	outFormatLine, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "searchable", "--format", "line", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--format line: %v", err)
	}

	if outFormatOneline != outOneline {
		t.Errorf("--format oneline output = %q, want %q", outFormatOneline, outOneline)
	}
	if outFormatLine != outOneline {
		t.Errorf("--format line output = %q, want %q", outFormatLine, outOneline)
	}
}

// TestOnelineFormat_FormatValidation verifies that invalid --format values are rejected.
func TestOnelineFormat_FormatValidation(t *testing.T) {
	newCfgRoot(t)

	_, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "query", "--format", "invalid")
	if err == nil {
		t.Fatalf("expected error for --format invalid, got nil")
	}
	var exitErr ExitError
	if !asExit(err, &exitErr) || exitErr.Code != 2 {
		t.Errorf("err = %v, want ExitError(2)", err)
	}
	if !strings.Contains(err.Error(), "invalid choice") {
		t.Errorf("error %q missing 'invalid choice'", err.Error())
	}
}

// TestOnelineFormat_MultipleHits verifies multiple matches produce exactly N lines.
func TestOnelineFormat_MultipleHits(t *testing.T) {
	root := newCfgRoot(t)
	writeCustomSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"11111111-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "common apple keyword in project a")
	writeCustomSession(t, root, "-home-u-proj-b", "bbbb2222-0000-0000-0000-000000000002",
		"22222222-0000-0000-0000-000000000002", "2026-06-02T10:00:00Z", "common apple keyword in project b")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "apple", "--oneline", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("search apple: %v\n%s", err, out)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d:\n%s", len(lines), out)
	}

	for _, line := range lines {
		cols := strings.Split(line, "\t")
		if len(cols) != 4 {
			t.Errorf("line %q has %d columns, want 4", line, len(cols))
		}
	}

	if !strings.Contains(out, "aaaa1111:11111111") || !strings.Contains(out, "bbbb2222:22222222") {
		t.Errorf("output missing expected refs:\n%s", out)
	}
}

// TestOnelineFormat_SanitizesSnippet verifies that newlines, carriage returns,
// tabs, ANSI escapes, and control characters are stripped from the snippet prose.
func TestOnelineFormat_SanitizesSnippet(t *testing.T) {
	root := newCfgRoot(t)
	rawText := "\x1b[31mError:\x1b[0m line1\nline2\r\nline3\twith\t\ttabs\x00and\x07control \x1b[1;32mchars\x1b[0m"
	writeCustomSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"9f3e1c20-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", rawText)

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "line1", "--oneline", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("search: %v\n%s", err, out)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 line, got %d lines: %q", len(lines), out)
	}

	cols := strings.Split(lines[0], "\t")
	if len(cols) != 4 {
		t.Fatalf("want 4 columns, got %d: %q", len(cols), lines[0])
	}

	snip := cols[3]
	// Must not contain ANSI escape byte
	if strings.Contains(snip, "\x1b") {
		t.Errorf("snippet contains ANSI escape: %q", snip)
	}
	// Must not contain newline or carriage return
	if strings.ContainsAny(snip, "\r\n") {
		t.Errorf("snippet contains newlines/CR: %q", snip)
	}
	// Must not contain null, bell, or other raw control characters
	if strings.ContainsAny(snip, "\x00\x07") {
		t.Errorf("snippet contains control characters: %q", snip)
	}

	for _, want := range []string{"Error:", "line1", "line2", "line3", "with tabs and control chars"} {
		if !strings.Contains(snip, want) {
			t.Errorf("snippet = %q, want to contain %q", snip, want)
		}
	}
}

// TestOnelineFormat_NoMatchesEmpty verifies that zero matching hits produce no output.
func TestOnelineFormat_NoMatchesEmpty(t *testing.T) {
	root := newCfgRoot(t)
	writeCustomSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"9f3e1c20-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "some unrelated text")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "nonexistentterm12345", "--oneline", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("search: %v\n%s", err, out)
	}
	if out != "" {
		t.Errorf("want empty output on no matches with --oneline, got: %q", out)
	}
}

// TestOnelineFormat_EmptyQueryEmpty verifies that an empty query with --oneline produces no output.
func TestOnelineFormat_EmptyQueryEmpty(t *testing.T) {
	newCfgRoot(t)

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "", "--oneline")
	if err != nil {
		t.Fatalf("empty query: %v\n%s", err, out)
	}
	if out != "" {
		t.Errorf("want empty output on empty query with --oneline, got: %q", out)
	}
}

// TestOnelineFormat_JSONUnaffected verifies that --json output remains byte-for-byte unaffected.
func TestOnelineFormat_JSONUnaffected(t *testing.T) {
	root := newCfgRoot(t)
	writeCustomSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"9f3e1c20-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "json search term text")

	outJSON, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "json", "--json", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--json: %v", err)
	}

	var env agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outJSON), &env); err != nil {
		t.Fatalf("--json output is not valid SearchEnvelope: %v\n%s", err, outJSON)
	}
	if len(env.Results) != 1 || env.Results[0].SessionID != "aaaa1111-0000-0000-0000-000000000001" {
		t.Errorf("unexpected json results: %+v", env.Results)
	}

	// Verify --format json produces valid SearchEnvelope
	root2 := newCfgRoot(t)
	writeCustomSession(t, root2, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"9f3e1c20-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "json search term text")

	outFormatJSON, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "json", "--format", "json", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--format json: %v", err)
	}
	var env2 agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outFormatJSON), &env2); err != nil {
		t.Fatalf("--format json output is not valid SearchEnvelope: %v\n%s", err, outFormatJSON)
	}
	if len(env2.Results) != 1 || env2.Results[0].SessionID != "aaaa1111-0000-0000-0000-000000000001" {
		t.Errorf("unexpected format json results: %+v", env2.Results)
	}
}

// TestCleanSnippetOneline unit tests the snippet sanitization helper directly.
func TestCleanSnippetOneline(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"simple text", "simple text"},
		{"\x1b[31mcolored\x1b[0m text", "colored text"},
		{"multi\nline\r\nwith\r\nnewlines", "multi line with newlines"},
		{"tab\tseparated\t\twords", "tab separated words"},
		{"null\x00bell\x07char", "null bell char"},
		{"  leading and trailing whitespace  \n", "leading and trailing whitespace"},
		{"\x1b[1;34m\x1b[47mbold blue on white\x1b[0m", "bold blue on white"},
	}

	for _, tc := range cases {
		got := agentproto.CleanSnippetOneline(tc.input)
		if got != tc.want {
			t.Errorf("CleanSnippetOneline(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestOnelineFormat_ScopingAndLimit verifies that --limit, --this-project, and path scoping
// are respected in oneline mode.
func TestOnelineFormat_ScopingAndLimit(t *testing.T) {
	root := newCfgRoot(t)
	writeCustomSession(t, root, "-home-u-alpha", "aaaa1111-0000-0000-0000-000000000001",
		"11111111-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "target search in alpha")
	writeCustomSession(t, root, "-home-u-alpha", "aaaa2222-0000-0000-0000-000000000002",
		"22222222-0000-0000-0000-000000000002", "2026-06-02T10:00:00Z", "target search in alpha 2")
	writeCustomSession(t, root, "-home-u-beta", "bbbb3333-0000-0000-0000-000000000003",
		"33333333-0000-0000-0000-000000000003", "2026-06-03T10:00:00Z", "target search in beta")

	// Test --limit 1
	outLimit, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "target", "--oneline", "--limit", "1", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--limit 1: %v", err)
	}
	linesLimit := strings.Split(strings.TrimRight(outLimit, "\n"), "\n")
	if len(linesLimit) != 1 {
		t.Errorf("want 1 line with --limit 1, got %d: %q", len(linesLimit), outLimit)
	}

	// Test --include-path alpha
	outInc, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "target", "--oneline", "--include-path", "alpha", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--include-path: %v", err)
	}
	if strings.Contains(outInc, "bbbb3333") {
		t.Errorf("--include-path alpha output contains beta session:\n%s", outInc)
	}

	// Test --exclude-path alpha
	outExc, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "target", "--oneline", "--exclude-path", "alpha", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--exclude-path: %v", err)
	}
	if strings.Contains(outExc, "aaaa1111") || strings.Contains(outExc, "aaaa2222") {
		t.Errorf("--exclude-path alpha output contains alpha session:\n%s", outExc)
	}
	if !strings.Contains(outExc, "bbbb3333") {
		t.Errorf("--exclude-path alpha output missing beta session:\n%s", outExc)
	}
}

// TestOnelineFormat_JSONTakesPrecedenceWhenBothSet verifies --json preserves JSON format.
func TestOnelineFormat_JSONTakesPrecedenceWhenBothSet(t *testing.T) {
	root := newCfgRoot(t)
	writeCustomSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"9f3e1c20-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "keyword text")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "keyword", "--json", "--oneline", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("--json --oneline: %v", err)
	}

	var env agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("expected JSON output when --json passed with --oneline: %v\n%s", err, out)
	}
	if len(env.Results) != 1 {
		t.Errorf("unexpected results: %+v", env.Results)
	}
}

// TestOnelineFormat_Browse verifies that rawclaw --oneline with no query produces oneline browse output.
func TestOnelineFormat_Browse(t *testing.T) {
	root := newCfgRoot(t)
	writeCustomSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"9f3e1c20-0000-0000-0000-000000000001", "2026-06-01T10:00:00Z", "first message preview")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--oneline", "--dir", filepath.Join(root, "-home-u-proj-a"))
	if err != nil {
		t.Fatalf("browse --oneline: %v\n%s", err, out)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d:\n%q", len(lines), out)
	}
	cols := strings.Split(lines[0], "\t")
	if len(cols) != 4 {
		t.Fatalf("want 4 columns, got %d: %q", len(cols), lines[0])
	}
	if !strings.HasPrefix(cols[0], "aaaa1111:") {
		t.Errorf("ref = %q, want prefix aaaa1111:", cols[0])
	}
}
