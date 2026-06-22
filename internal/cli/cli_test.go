package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/render"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
)

// asExit is errors.As for ExitError (test convenience).
func asExit(err error, target *ExitError) bool {
	return errors.As(err, target)
}

// TestNewRootCmd is the command-tree smoke test: the command tree builds and carries
// the expected binary name and the flags.
func TestNewRootCmd(t *testing.T) {
	t.Parallel()

	cmd := NewRootCmd(BuildInfo{})
	if cmd == nil {
		t.Fatal("NewRootCmd returned nil")
	}
	if cmd.Name() != "rawclaw" {
		t.Errorf("root Name() = %q, want %q", cmd.Name(), "rawclaw")
	}

	// Every flag must be wired (local flags on root).
	wantFlags := []string{
		"limit", "dir", "this-project", "all",
		"list", "role", "sort", "include-tools", "include-subagents",
		"reindex", "json", "resume", "stats", "since", "before", "no-vector",
		"reindex-vectors", "include-path", "exclude-path", "min-messages",
		"debug-search",
	}
	for _, name := range wantFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("root missing flag --%s", name)
		}
	}
}

// TestPrintResults checks the human-readable per-project result rendering byte-for-byte.
func TestPrintResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		res      []retrieve.Hit
		nSess    int
		contains []string
		exact    string
	}{
		{
			name:  "empty",
			res:   nil,
			nSess: 5,
			exact: "No matches. (Default searches top-level human text only — try --include-subagents " +
				"and/or --include-tools to widen, or rephrase: keyword > full sentence.)\n",
		},
		{
			name: "single known session count",
			// SessionID exercises last path segment truncated to 8 chars.
			res:   []retrieve.Hit{{ISO: "2026-06-18", SessionID: "abc/def/a1b2c3d4e5", Role: "user", Snippet: "hello"}},
			nSess: 5,
			exact: "Top 1 match(es) across 5 of this project's sessions:\n\n" +
				"[2026-06-18 · a1b2c3d4 · user] …hello…\n\n",
		},
		{
			name: "unknown count + subagent tag + empty iso",
			res: []retrieve.Hit{
				{ISO: "", SessionID: "sess", Role: "assistant", IsSubagent: true, Parent: "parent99xx", Snippet: "snip"},
			},
			nSess: -1, // unknown count → "this project's sessions"
			exact: "Top 1 match(es) across this project's sessions:\n\n" +
				"[? · sess · assistant · subagent⟵parent99] …snip…\n\n",
		},
		{
			name: "subagent flag but empty parent suppresses tag",
			res: []retrieve.Hit{
				{ISO: "x", SessionID: "s", Role: "user", IsSubagent: true, Parent: "", Snippet: "z"},
			},
			nSess:    1,
			contains: []string{"[x · s · user] …z…"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			PrintResults(&buf, tc.res, tc.nSess)
			got := buf.String()
			if tc.exact != "" && got != tc.exact {
				t.Errorf("PrintResults\n got: %q\nwant: %q", got, tc.exact)
			}
			for _, sub := range tc.contains {
				if !strings.Contains(got, sub) {
					t.Errorf("PrintResults missing %q in %q", sub, got)
				}
			}
		})
	}
}

// TestEmitJSON checks EmitJSON output: 2-space indent, UTF-8 preserved, no HTML escaping.
func TestEmitJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := EmitJSON(&buf, map[string]any{"a": 1, "b": "<x>"}); err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	got := buf.String()
	// 2-space indent + raw < > (not <). Map keys sorted by Go (a,b).
	want := "{\n  \"a\": 1,\n  \"b\": \"<x>\"\n}\n"
	if got != want {
		t.Errorf("EmitJSON\n got: %q\nwant: %q", got, want)
	}
}

// TestEmitJSONRows checks the --brief --json shape matches _rows_to_json keys/order.
func TestEmitJSONRows(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	rows := rowsToJSON([]retrieve.Hit{{ISO: "2026", SessionID: "sidX", Role: "user", Snippet: "snp"}})
	if err := EmitJSON(&buf, rows); err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	got := buf.String()
	for _, key := range []string{`"iso": "2026"`, `"session_id": "sidX"`, `"role": "user"`, `"is_subagent": false`, `"snippet": "snp"`} {
		if !strings.Contains(got, key) {
			t.Errorf("rows json missing %q in %q", key, got)
		}
	}
}

// TestChoiceValidation checks the allowed-value enforcement for --role / --sort.
func TestChoiceValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flag    string
		val     string
		wantErr bool
	}{
		{"role empty ok", "role", "", false},
		{"role user ok", "role", "user", false},
		{"role bad", "role", "bogus", true},
		{"sort newest ok", "sort", "newest", false},
		{"sort bad", "sort", "down", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.flag == "role" {
				err = validateChoice("role", tc.val, "user", "assistant")
			} else {
				err = validateChoice("sort", tc.val, "newest", "oldest")
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("validateChoice(%s=%q) err=%v, wantErr=%v", tc.flag, tc.val, err, tc.wantErr)
			}
			if err != nil {
				var ee ExitError
				if !asExit(err, &ee) || ee.Code != 2 {
					t.Errorf("bad choice should be ExitError{Code:2}, got %v", err)
				}
			}
		})
	}
}

// TestDebugSearchFlagRoutes drives the full cobra path with --debug-search: the
// flag must parse and route into runDebugSearch. Pointed at an empty --dir (no
// transcript history) under --this-project, the handler resolves the scope, finds
// nothing, and exits cleanly — proving the flag is wired through Execute without
// requiring a real corpus. Composes with --this-project.
func TestDebugSearchFlagRoutes(t *testing.T) {
	t.Parallel()

	cmd := NewRootCmd(BuildInfo{})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	// Empty temp dir → no encoded transcript dir → "No transcript history" hint.
	cmd.SetArgs([]string{"--debug-search", "--this-project", "--dir", t.TempDir(), "needle"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--debug-search should route cleanly, got err: %v", err)
	}
	if !strings.Contains(out.String(), "No transcript history") {
		t.Errorf("--debug-search --this-project on empty dir should print the no-history hint, got %q", out.String())
	}
}

// TestDebugSearchBreakdownShows asserts the scoring breakdown actually renders:
// the explainer block (rank, regime, bm25-order, coverage, recency overlay) shows
// for each hit, and the --json shape nests the same score fields. Inputs are
// synthetic so the test is deterministic and needs no corpus.
func TestDebugSearchBreakdownShows(t *testing.T) {
	t.Parallel()

	hits := []retrieve.Hit{
		{ISO: "2026-06-18", SessionID: "abc/a1b2c3d4e5", Role: "user", Snippet: "needle"},
	}
	explains := []retrieve.ScoreExplain{
		{BM25Rank: 0, Coverage: 2, Recency: 0, Final: 0, Method: retrieve.MethodBM25Coverage, Terms: []string{"need", "le"}},
	}

	// Human breakdown: the regime + the REAL scoring inputs must appear.
	var human bytes.Buffer
	render.PrintDebugSearch(&human, hits, explains)
	for _, want := range []string{
		"scoring explainer",
		"rank 0",
		retrieve.MethodBM25Coverage,
		"coverage=2/2 term(s)",
		"recency-overlay=no",
	} {
		if !strings.Contains(human.String(), want) {
			t.Errorf("PrintDebugSearch breakdown missing %q in:\n%s", want, human.String())
		}
	}

	// --json composition: the score block nests under each hit.
	b, err := render.DebugSearchJSON(hits, explains)
	if err != nil {
		t.Fatalf("DebugSearchJSON: %v", err)
	}
	for _, want := range []string{`"score"`, `"bm25_rank": 0`, `"coverage": 2`, `"method": "bm25 + coverage re-rank"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("DebugSearchJSON missing %q in:\n%s", want, string(b))
		}
	}
}

// TestVersionSubcommand: `rawclaw version` prints the injected build stamp on
// stdout and exits cleanly.
func TestVersionSubcommand(t *testing.T) {
	t.Parallel()

	cmd := NewRootCmd(BuildInfo{Version: "1.2.3", Commit: "abc1234", Date: "2026-06-21"})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version subcommand: %v", err)
	}
	for _, want := range []string{"rawclaw 1.2.3", "commit: abc1234", "built: 2026-06-21", "go:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("version output missing %q in %q", want, out.String())
		}
	}
}

// TestVersionFlag: the cobra-native `--version` flag prints the same banner.
func TestVersionFlag(t *testing.T) {
	t.Parallel()

	cmd := NewRootCmd(BuildInfo{Version: "9.9.9", Commit: "deadbee", Date: "2026-01-01"})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--version flag: %v", err)
	}
	if !strings.Contains(out.String(), "rawclaw 9.9.9") {
		t.Errorf("--version output missing banner in %q", out.String())
	}
}

// TestBuildInfoDefaults: an un-stamped (zero) BuildInfo reports the honest
// dev/unknown defaults rather than empty fields.
func TestBuildInfoDefaults(t *testing.T) {
	t.Parallel()

	got := BuildInfo{}.versionString()
	for _, want := range []string{"rawclaw dev", "commit: unknown", "built: unknown"} {
		if !strings.Contains(got, want) {
			t.Errorf("zero BuildInfo versionString missing %q in %q", want, got)
		}
	}
}

// TestEmitJSONEmptyObject: scroll --json with no result emits "{}\n".
func TestEmitJSONEmptyObject(t *testing.T) {
	t.Parallel()

	// Scroll --json with no result emits "{}\n".
	var buf bytes.Buffer
	if err := EmitJSON(&buf, struct{}{}); err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	if got := buf.String(); got != "{}\n" {
		t.Errorf("empty object json = %q, want %q", got, "{}\n")
	}
}
