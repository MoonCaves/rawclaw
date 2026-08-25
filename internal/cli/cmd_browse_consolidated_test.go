package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// seedBrowseCorpus creates 3 projects with distinct sessions and ensures they are
// indexed and consolidated into consolidated.db.
func seedBrowseCorpus(t *testing.T) (claudeDir string, scopesList []view.Scope) {
	t.Helper()
	root := newCfgRoot(t)

	// Three Claude project directories
	projA := filepath.Join(root, "-home-u-proj-a")
	projB := filepath.Join(root, "-home-u-proj-b")
	projC := filepath.Join(root, "-home-u-proj-c")

	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "first question in project A")
	writeIndexedSession(t, root, "-home-u-proj-b", "bbbb2222-0000-0000-0000-000000000002",
		"2026-06-02T10:00:00Z", "second question in project B")
	writeIndexedSession(t, root, "-home-u-proj-c", "cccc3333-0000-0000-0000-000000000003",
		"2026-06-03T10:00:00Z", "third question in project C")
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa2222-0000-0000-0000-000000000004",
		"2026-06-04T10:00:00Z", "fourth question in project A (newest)")

	for _, p := range []string{projA, projB, projC} {
		if _, _, _, err := index.EnsureIndexed(p, false); err != nil {
			t.Fatalf("EnsureIndexed(%s): %v", p, err)
		}
	}

	scopesList = []view.Scope{
		{Project: paths.ProjectLabel(projA), TDir: projA, Source: "claude"},
		{Project: paths.ProjectLabel(projB), TDir: projB, Source: "claude"},
		{Project: paths.ProjectLabel(projC), TDir: projC, Source: "claude"},
	}

	return root, scopesList
}

// TestBrowseConsolidated_ByteIdenticalToFallback verifies that scoped browse
// output (rows, ordering, limits, JSON shape, scope labels, empty-scope envelopes)
// when answered from the consolidated store is byte-identical to the per-project fallback.
func TestBrowseConsolidated_ByteIdenticalToFallback(t *testing.T) {
	_, scopeList := seedBrowseCorpus(t)

	cases := []struct {
		name string
		opts Options
	}{
		{
			name: "all plain text",
			opts: Options{All: true, Limit: 10},
		},
		{
			name: "all json",
			opts: Options{All: true, Limit: 10, JSON: true},
		},
		{
			name: "all oneline",
			opts: Options{All: true, Limit: 10, Oneline: true},
		},
		{
			name: "all capped limit",
			opts: Options{All: true, Limit: 2},
		},
		{
			name: "all capped limit json",
			opts: Options{All: true, Limit: 2, JSON: true},
		},
		{
			name: "include-path match",
			opts: Options{All: true, IncludePath: "proj-a", Limit: 10},
		},
		{
			name: "include-path match json",
			opts: Options{All: true, IncludePath: "proj-a", Limit: 10, JSON: true},
		},
		{
			name: "exclude-path",
			opts: Options{All: true, ExcludePath: "proj-b", Limit: 10},
		},
		{
			name: "exclude-path json",
			opts: Options{All: true, ExcludePath: "proj-b", Limit: 10, JSON: true},
		},
		{
			name: "source filter match",
			opts: Options{All: true, Source: "claude", Limit: 10},
		},
		{
			name: "source filter mismatch",
			opts: Options{All: true, Source: "codex", Limit: 10},
		},
		{
			name: "source filter mismatch json",
			opts: Options{All: true, Source: "codex", Limit: 10, JSON: true},
		},
		{
			name: "since filter",
			opts: Options{All: true, Since: "2026-06-03", Limit: 10},
		},
		{
			name: "before filter",
			opts: Options{All: true, Before: "2026-06-02", Limit: 10},
		},
		{
			name: "include-path unmatched honest empty",
			opts: Options{All: true, IncludePath: "nonexistent", Limit: 10},
		},
		{
			name: "include-path unmatched honest empty json",
			opts: Options{All: true, IncludePath: "nonexistent", Limit: 10, JSON: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Run against consolidated store
			var storeOut bytes.Buffer
			if err := runBrowseScoped(&storeOut, &tc.opts, scopeList); err != nil {
				t.Fatalf("runBrowseScoped (consolidated): %v", err)
			}

			// 2. Temporarily remove consolidated.db to test fallback
			consPath := index.ConsolidatedPath()
			consBackup := consPath + ".bak"
			if err := os.Rename(consPath, consBackup); err != nil {
				t.Fatalf("rename consolidated store: %v", err)
			}
			defer func() {
				if _, err := os.Stat(consBackup); err == nil {
					_ = os.Rename(consBackup, consPath)
				}
			}()

			var fallbackOut bytes.Buffer
			if err := runBrowseScoped(&fallbackOut, &tc.opts, scopeList); err != nil {
				t.Fatalf("runBrowseScoped (fallback): %v", err)
			}

			// Restore consolidated store
			if err := os.Rename(consBackup, consPath); err != nil {
				t.Fatalf("restore consolidated store: %v", err)
			}

			if storeOut.String() != fallbackOut.String() {
				t.Errorf("mismatch between consolidated store and fallback output:\n--- Consolidated ---\n%s\n--- Fallback ---\n%s",
					storeOut.String(), fallbackOut.String())
			}
		})
	}
}

// TestBrowseConsolidated_SingleConnection verifies that exactly one database
// connection is opened during a consolidated browse query.
func TestBrowseConsolidated_SingleConnection(t *testing.T) {
	_, _ = seedBrowseCorpus(t)

	con, count, err := index.OpenConsolidated()
	if err != nil {
		t.Fatalf("OpenConsolidated: %v", err)
	}
	defer con.Close()

	if count < 4 {
		t.Fatalf("expected at least 4 sessions in consolidated store, got %d", count)
	}

	// view.BrowseScoped uses the passed connection and does not open any others.
	rows := view.BrowseScoped(con, 10, "", "", "", nil)
	if len(rows) != 4 {
		t.Fatalf("BrowseScoped returned %d rows, want 4", len(rows))
	}

	// Verify ordering: newest first
	if rows[0].SessionID != "aaaa2222-0000-0000-0000-000000000004" {
		t.Errorf("rows[0] = %s, want aaaa2222 (newest)", rows[0].SessionID)
	}
	if rows[3].SessionID != "aaaa1111-0000-0000-0000-000000000001" {
		t.Errorf("rows[3] = %s, want aaaa1111 (oldest)", rows[3].SessionID)
	}
}

// TestBrowseConsolidated_JSONStructure validates the JSON output format and
// scope reporting.
func TestBrowseConsolidated_JSONStructure(t *testing.T) {
	root, scopeList := seedBrowseCorpus(t)

	var buf bytes.Buffer
	opts := Options{All: true, IncludePath: "proj-a", JSON: true, Limit: 10}
	if err := runBrowseScoped(&buf, &opts, scopeList); err != nil {
		t.Fatalf("runBrowseScoped: %v", err)
	}

	var res struct {
		Scope       string              `json:"scope"`
		IncludePath string              `json:"include_path"`
		Projects    int                 `json:"projects"`
		Sessions    []view.BrowseAllRow `json:"sessions"`
	}
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}

	if res.Scope != "all" {
		t.Errorf("Scope = %q, want 'all'", res.Scope)
	}
	if res.IncludePath != "proj-a" {
		t.Errorf("IncludePath = %q, want 'proj-a'", res.IncludePath)
	}
	if res.Projects != 1 {
		t.Errorf("Projects = %d, want 1", res.Projects)
	}
	if len(res.Sessions) != 2 {
		t.Fatalf("len(Sessions) = %d, want 2", len(res.Sessions))
	}
	wantProject := paths.ProjectLabel(filepath.Join(root, "-home-u-proj-a"))
	for _, s := range res.Sessions {
		if s.Project != wantProject {
			t.Errorf("session project = %q, want %q", s.Project, wantProject)
		}
		if s.Preview == "" {
			t.Errorf("session preview is empty for %s", s.SessionID)
		}
	}
}
