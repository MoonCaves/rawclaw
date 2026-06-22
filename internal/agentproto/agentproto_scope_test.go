package agentproto

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// msgSpec is one transcript message for writeRichSession: role + uuid + content.
type msgSpec struct {
	role    string
	uuid    string
	content string
}

// writeRichSession writes a multi-message transcript under proj with per-message
// role control and a fixed date (date), so the scope-flag tests (#1) can exercise
// --role / --since / --before / --min-messages against real indexed rows.
func writeRichSession(t *testing.T, proj, stem, date string, msgs []msgSpec) {
	t.Helper()
	var b strings.Builder
	for i, m := range msgs {
		ts := fmt.Sprintf("%sT10:%02d:00Z", date, i%60)
		b.WriteString(`{"type":"` + m.role + `","uuid":"` + m.uuid + `","timestamp":"` + ts + `",` +
			`"message":{"role":"` + m.role + `","content":"` + m.content + `"}}` + "\n")
	}
	path := filepath.Join(proj, stem+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func refsByProject(env SearchEnvelope) map[string]int {
	out := map[string]int{}
	for _, r := range env.Results {
		out[r.Project]++
	}
	return out
}

// TestSearchRoleFilter: --role narrows to one author role AND the role value
// ("assistant") never reaches the FTS5 query as a junk OR-term (#1). The corpus
// has the query token "deployseam" in both a user and an assistant message; a
// role filter must drop the user one, and the role word must NOT itself match.
func TestSearchRoleFilter(t *testing.T) {
	proj := t.TempDir()
	writeRichSession(t, proj, "a1b2c3d4eeee", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaaa", content: "deployseam from the user side"},
		{role: "assistant", uuid: "f0000000bbbb", content: "deployseam from the assistant side"},
	})
	scope := scopeFor(t, proj)

	all := Search("deployseam", scope, SearchOpts{}, nil)
	if len(all.Results) == 0 {
		t.Fatal("baseline (no role) must return at least one ref")
	}

	asst := Search("deployseam", scope, SearchOpts{Role: "assistant"}, nil)
	if len(asst.Results) == 0 {
		t.Fatal("role=assistant must still match the assistant message")
	}
	user := Search("deployseam", scope, SearchOpts{Role: "user"}, nil)
	if len(user.Results) == 0 {
		t.Fatal("role=user must still match the user message")
	}

	// The role value must NOT leak as a query term: a query whose ONLY token is a
	// role keyword, with a role filter set, must not self-match on the keyword.
	leak := Search("zzznotacorpusterm", scope, SearchOpts{Role: "assistant"}, nil)
	if len(leak.Results) != 0 {
		t.Errorf("a non-matching query with --role must return 0, got %d (role value leaked into the query?)",
			len(leak.Results))
	}
}

// TestSearchSinceBeforeBounds: --since / --before filter by date BEFORE the
// limit, and the date value never leaks into the query (#1).
func TestSearchSinceBeforeBounds(t *testing.T) {
	proj := t.TempDir()
	writeRichSession(t, proj, "a1b2c3d4eeee", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaaa", content: "datedmatch content here"},
	})
	scope := scopeFor(t, proj)

	if env := Search("datedmatch", scope, SearchOpts{}, nil); len(env.Results) == 0 {
		t.Fatal("baseline must match")
	}
	if env := Search("datedmatch", scope, SearchOpts{Since: "2030-01-01"}, nil); len(env.Results) != 0 {
		t.Errorf("--since 2030-01-01 must return 0, got %d", len(env.Results))
	}
	if env := Search("datedmatch", scope, SearchOpts{Before: "2000-01-01"}, nil); len(env.Results) != 0 {
		t.Errorf("--before 2000-01-01 must return 0, got %d", len(env.Results))
	}
	if env := Search("datedmatch", scope, SearchOpts{Since: "2026-01-01", Before: "2026-12-31"}, nil); len(env.Results) == 0 {
		t.Error("a bracketing since/before must still match")
	}
}

// TestSearchMinMessages: --min-messages drops thin sessions, and the number
// ("999999") never leaks into the query (#1).
func TestSearchMinMessages(t *testing.T) {
	proj := t.TempDir()
	writeRichSession(t, proj, "a1b2c3d4eeee", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaaa", content: "thinthreadmatch one"},
		{role: "assistant", uuid: "f0000000bbbb", content: "thinthreadmatch two"},
	})
	scope := scopeFor(t, proj)

	if env := Search("thinthreadmatch", scope, SearchOpts{}, nil); len(env.Results) == 0 {
		t.Fatal("baseline must match")
	}
	if env := Search("thinthreadmatch", scope, SearchOpts{MinMessages: 999999}, nil); len(env.Results) != 0 {
		t.Errorf("--min-messages 999999 must return 0, got %d", len(env.Results))
	}
	if env := Search("thinthreadmatch", scope, SearchOpts{MinMessages: 2}, nil); len(env.Results) == 0 {
		t.Error("--min-messages 2 must still match a 2-message session")
	}
}

// TestSearchIncludeExcludePath: --include-path narrows to matching project dirs;
// --exclude-path drops them (#1). Two projects, the query matches in both.
func TestSearchIncludeExcludePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	// paths.ProjectCWD decodes the transcript-dir basename; build two dirs whose
	// decoded cwd contains "alpha" vs "bravo".
	libDir := filepath.Join(root, "-Users-octocat-org-alpha")
	cooDir := filepath.Join(root, "-Users-octocat-org-bravo")
	for _, d := range []string{libDir, cooDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeRichSession(t, libDir, "a1b2c3d4eeee", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaaa", content: "pathscopematch in alpha"},
	})
	writeRichSession(t, cooDir, "b2c3d4e5ffff", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f1111111cccc", content: "pathscopematch in bravo"},
	})
	scope := []view.Scope{
		{Project: paths.ProjectLabel(libDir), TDir: libDir},
		{Project: paths.ProjectLabel(cooDir), TDir: cooDir},
	}

	base := Search("pathscopematch", scope, SearchOpts{}, nil)
	if len(base.Results) < 2 {
		t.Fatalf("baseline must match both projects, got %d", len(base.Results))
	}

	inc := Search("pathscopematch", scope, SearchOpts{IncludePath: "alpha"}, nil)
	byProj := refsByProject(inc)
	if byProj[paths.ProjectLabel(cooDir)] != 0 {
		t.Errorf("--include-path alpha must drop bravo, got %+v", byProj)
	}
	if byProj[paths.ProjectLabel(libDir)] == 0 {
		t.Errorf("--include-path alpha must keep alpha, got %+v", byProj)
	}

	exc := Search("pathscopematch", scope, SearchOpts{ExcludePath: "alpha"}, nil)
	byProj = refsByProject(exc)
	if byProj[paths.ProjectLabel(libDir)] != 0 {
		t.Errorf("--exclude-path alpha must drop alpha, got %+v", byProj)
	}
	if byProj[paths.ProjectLabel(cooDir)] == 0 {
		t.Errorf("--exclude-path alpha must keep bravo, got %+v", byProj)
	}
}

// TestSearchTruncationSignal: the envelope reports Complete=false when the limit
// caps a larger candidate set, and Complete=true when nothing was hidden (#2).
func TestSearchTruncationSignal(t *testing.T) {
	proj := t.TempDir()
	// Five distinct sessions all matching "capsignal" — more than a limit of 2.
	for i := 0; i < 5; i++ {
		stem := fmt.Sprintf("a1b2c3d%d0000", i)
		uuid := fmt.Sprintf("f000000%daaaa", i)
		writeRichSession(t, proj, stem, "2026-06-01", []msgSpec{
			{role: "user", uuid: uuid, content: "capsignal content"},
		})
	}
	scope := scopeFor(t, proj)

	capped := Search("capsignal", scope, SearchOpts{Limit: 2}, nil)
	if len(capped.Results) != 2 {
		t.Fatalf("limit 2 must return exactly 2 refs, got %d", len(capped.Results))
	}
	if capped.Complete {
		t.Error("a limit that hides candidates must report Complete=false (#2)")
	}

	whole := Search("capsignal", scope, SearchOpts{Limit: 100}, nil)
	if !whole.Complete {
		t.Errorf("a limit larger than the candidate set must report Complete=true, got false; scopes=%+v", whole.Scopes)
	}
}

// TestDefaultSearchLimit pins the agent default at >= the human --limit default
// (8), so an agent's default discovery is not narrower than a human's (#2).
func TestDefaultSearchLimit(t *testing.T) {
	if DefaultSearchLimit < 8 {
		t.Errorf("DefaultSearchLimit = %d, want >= 8 (the human --limit default)", DefaultSearchLimit)
	}
}

// TestSearchNotExclusion: `deploy NOT staging` excludes the staging hit through
// the agent Search path (#4). Before the fix, the agent path dropped "not" as a
// stopword and the exclusion silently no-opped.
func TestSearchNotExclusion(t *testing.T) {
	proj := t.TempDir()
	writeRichSession(t, proj, "a1b2c3d40000", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaaa", content: "notexcl deploy to prod"},
	})
	writeRichSession(t, proj, "b2c3d4e50000", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f1111111bbbb", content: "notexcl deploy to staging only"},
	})
	scope := scopeFor(t, proj)

	all := Search("notexcl deploy", scope, SearchOpts{Limit: 100}, nil)
	excl := Search("notexcl deploy NOT staging", scope, SearchOpts{Limit: 100}, nil)
	if len(excl.Results) >= len(all.Results) {
		t.Fatalf("NOT staging must exclude the staging hit: all=%d excl=%d", len(all.Results), len(excl.Results))
	}
	for _, r := range excl.Results {
		if strings.Contains(strings.ToLower(r.Snippet), "staging") {
			t.Errorf("a NOT-staging result still mentions staging: %q", r.Snippet)
		}
	}
}

// TestSearchPrefixAndPath: an `<identifier>*` prefix and a `/`-path token both
// return hits through the agent Search path (#3, #5) — the shared query layer
// means the agent path benefits from the sanitizer fixes.
func TestSearchPrefixAndPath(t *testing.T) {
	proj := t.TempDir()
	writeRichSession(t, proj, "a1b2c3d40000", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaaa", content: "the self-update seam lives under ~/.claude/projects here"},
	})
	scope := scopeFor(t, proj)

	cases := []string{"self-update*", "~/.claude/projects", ".claude/projects"}
	for _, q := range cases {
		if env := Search(q, scope, SearchOpts{}, nil); len(env.Results) == 0 {
			t.Errorf("query %q returned 0 results, want >0", q)
		}
	}
}
