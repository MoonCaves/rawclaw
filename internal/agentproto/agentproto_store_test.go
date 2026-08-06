package agentproto

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// storeSession writes a transcript of nmsg messages, every one of them carrying
// the same text, and records the working directory the session ran in. The cwd
// line matters twice over: it is what the index turns into the project label,
// and it is what an --include-path regex is matched against, so a fixture
// without one cannot exercise either narrowing.
func storeSession(t *testing.T, tdir, stem, cwd string, nmsg int, content string) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < nmsg; i++ {
		fmt.Fprintf(&b,
			`{"type":"user","uuid":"%s","cwd":%q,"timestamp":"2026-06-01T10:00:00Z",`+
				`"message":{"role":"user","content":%q}}`+"\n",
			msgUUID(stem, i), cwd, content)
	}
	path := filepath.Join(tdir, stem+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// msgUUID mints a uuid that is unique per (session, message) and still parses
// as the hex-prefixed shape a read-ref is built from.
func msgUUID(stem string, i int) string {
	sum := 0
	for _, r := range stem {
		sum = sum*31 + int(r)
	}
	return fmt.Sprintf("%08x-aaaa-bbbb-cccc-%012d", sum&0xffffffff, i)
}

// seedTwoProjectStore builds the shape the one store exists for: two projects
// with separate databases, both folded into one store by the write-through that
// runs on every index. It returns the two project labels.
func seedTwoProjectStore(t *testing.T) (bigLabel, smallLabel string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	big := t.TempDir()
	storeSession(t, big, "sessbig", "/src/billing", 1, "advancing the retention watermark")
	small := t.TempDir()
	storeSession(t, small, "sesssml", "/src/ledger", 1, "the watermark came up in weekly planning")

	for _, tdir := range []string{big, small} {
		if _, _, _, err := index.EnsureIndexed(tdir, false); err != nil {
			t.Fatalf("EnsureIndexed(%s): %v", tdir, err)
		}
	}
	return paths.ProjectLabel(big), paths.ProjectLabel(small)
}

// noFallback is the scope fallback for a test that must prove the one store
// answered: it hands the fan-out nothing, so any result at all can only have
// come from the store.
func noFallback() []view.Scope { return nil }

// projectsIn collects the distinct project labels a result set covers.
func projectsIn(env SearchEnvelope) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range env.Results {
		if !seen[r.Project] {
			seen[r.Project] = true
			out = append(out, r.Project)
		}
	}
	return out
}

// TestSearchAnswersFromTheOneStore is the shape of the change: a search with no
// project list decided for it is answered by one database and one query, and the
// single ranked list spans every project in the store. The fan-out is given
// nothing to look in, so a hit here cannot have come from it.
func TestSearchAnswersFromTheOneStore(t *testing.T) {
	bigLabel, smallLabel := seedTwoProjectStore(t)

	env := Search("watermark", nil, SearchOpts{Limit: 8, ScopeFallback: noFallback}, nil)

	if env.Store != StoreConsolidated {
		t.Fatalf("Store = %q (note %q), want the one store to answer", env.Store, env.StoreNote)
	}
	if len(env.Results) != 2 {
		t.Fatalf("results = %d, want both projects in one list: %+v", len(env.Results), env.Results)
	}
	got := projectsIn(env)
	for _, want := range []string{bigLabel, smallLabel} {
		if !contains(got, want) {
			t.Errorf("no result labelled %q; got %v", want, got)
		}
	}
	// One store row in the completeness report — the scope report keeps its job,
	// it just reports on a store instead of on a list of projects.
	if len(env.Scopes) != 1 || env.Scopes[0].Status != ScopeSearched {
		t.Fatalf("scopes report = %+v, want one searched store row", env.Scopes)
	}
	if !strings.Contains(env.Scopes[0].Detail, "2 sessions") {
		t.Errorf("store row detail = %q, want it to state the store's size", env.Scopes[0].Detail)
	}
	if !env.Complete {
		t.Errorf("Complete = false on a fully-served search: %+v", env)
	}
}

// TestSearchNarrowsTheOneStoreByProject confirms scope became a WHERE clause:
// naming a project drops the other project's rows from the same one query,
// rather than choosing which databases to open.
func TestSearchNarrowsTheOneStoreByProject(t *testing.T) {
	_, smallLabel := seedTwoProjectStore(t)

	env := Search("watermark", nil,
		SearchOpts{Limit: 8, Project: smallLabel, ScopeFallback: noFallback}, nil)

	if env.Store != StoreConsolidated {
		t.Fatalf("Store = %q (note %q), want the one store", env.Store, env.StoreNote)
	}
	if len(env.Results) != 1 || env.Results[0].Project != smallLabel {
		t.Fatalf("narrowed results = %+v, want only %q", env.Results, smallLabel)
	}
	if !strings.Contains(env.Scopes[0].Detail, "narrowed to 1 project") {
		t.Errorf("store row detail = %q, want the narrowing stated", env.Scopes[0].Detail)
	}
}

// TestSearchNarrowsTheOneStoreBySource confirms --source is the same kind of
// WHERE clause. These fixtures are all Claude transcripts, so asking for the
// other tool must come back empty from a store that genuinely holds none of it —
// and empty here is a real answer, not a degraded one, so the envelope still
// reports itself complete.
func TestSearchNarrowsTheOneStoreBySource(t *testing.T) {
	seedTwoProjectStore(t)

	kept := Search("watermark", nil,
		SearchOpts{Limit: 8, Source: "claude", ScopeFallback: noFallback}, nil)
	if kept.Store != StoreConsolidated || len(kept.Results) != 2 {
		t.Fatalf("--source claude = store %q with %d results, want both from the one store",
			kept.Store, len(kept.Results))
	}

	none := Search("watermark", nil,
		SearchOpts{Limit: 8, Source: "codex", ScopeFallback: noFallback}, nil)
	if none.Store != StoreConsolidated {
		t.Fatalf("Store = %q (note %q), want the one store", none.Store, none.StoreNote)
	}
	if len(none.Results) != 0 {
		t.Fatalf("--source codex = %+v, want no results from a corpus with no codex sessions", none.Results)
	}
	if !none.Complete {
		t.Errorf("Complete = false on a proven-empty answer: %+v", none)
	}
}

// TestSearchNarrowsTheOneStoreByIncludePath confirms the path regex is resolved
// in Go against the working dirs the store knows, then travels as an exact
// project list. The pattern never reaches SQL, so a Go-only construct still
// means what the flag says it means.
func TestSearchNarrowsTheOneStoreByIncludePath(t *testing.T) {
	_, smallLabel := seedTwoProjectStore(t)

	env := Search("watermark", nil,
		SearchOpts{Limit: 8, IncludePath: `led(g|j)er$`, ScopeFallback: noFallback}, nil)

	if env.Store != StoreConsolidated {
		t.Fatalf("Store = %q (note %q), want the one store", env.Store, env.StoreNote)
	}
	if len(env.Results) != 1 || env.Results[0].Project != smallLabel {
		t.Fatalf("--include-path results = %+v, want only the project whose cwd matched", env.Results)
	}
}

// TestSearchFallsBackWhenTheStoreIsAbsent covers the honest degradation: with no
// store on disk, a search must say so and go to the per-project databases rather
// than return a confident empty answer from a store that was never filled.
func TestSearchFallsBackWhenTheStoreIsAbsent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	storeSession(t, proj, "sessone", "/src/billing", 1, "advancing the retention watermark")
	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	// Remove the store the write-through just filled, leaving the project db.
	if err := os.Remove(index.ConsolidatedPath()); err != nil {
		t.Fatalf("remove consolidated store: %v", err)
	}

	label := paths.ProjectLabel(proj)
	env := Search("watermark", nil, SearchOpts{
		Limit:         8,
		ScopeFallback: func() []view.Scope { return []view.Scope{{Project: label, TDir: proj}} },
	}, nil)

	if env.Store != StorePerProject {
		t.Fatalf("Store = %q, want the fan-out to have answered", env.Store)
	}
	if env.StoreNote == "" {
		t.Error("StoreNote is empty — a fallback must announce itself")
	}
	if len(env.Results) != 1 || env.Results[0].Project != label {
		t.Fatalf("results = %+v, want the project db's hit via the fan-out", env.Results)
	}
}

// TestSearchFallsBackWhenTheStoreHasNotHeardOfTheProject covers the other
// degradation. The store is present and non-empty, but was rebuilt from a
// narrower set of sources, so the requested project is genuinely outside it.
// Answering empty would hide a corpus that is sitting on disk.
func TestSearchFallsBackWhenTheStoreHasNotHeardOfTheProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	kept := t.TempDir()
	storeSession(t, kept, "sesskpt", "/src/billing", 1, "advancing the retention watermark")
	keptDB, _, _, err := index.EnsureIndexed(kept, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(kept): %v", err)
	}
	dropped := t.TempDir()
	storeSession(t, dropped, "sessdrp", "/src/ledger", 1, "the watermark in weekly planning")
	if _, _, _, err := index.EnsureIndexed(dropped, false); err != nil {
		t.Fatalf("EnsureIndexed(dropped): %v", err)
	}
	// Rebuild the store from the kept project alone, so the other project is
	// absent from it while its own database still holds the answer.
	if _, err := index.ConsolidateFrom([]string{keptDB}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	// The fallback offers BOTH projects: on the fan-out the scope IS the filter,
	// so a request narrowed to one project must not silently widen to every
	// project just because the store could not serve it.
	label := paths.ProjectLabel(dropped)
	env := Search("watermark", nil, SearchOpts{
		Limit:   8,
		Project: label,
		ScopeFallback: func() []view.Scope {
			return []view.Scope{
				{Project: paths.ProjectLabel(kept), TDir: kept},
				{Project: label, TDir: dropped},
			}
		},
	}, nil)

	if env.Store != StorePerProject {
		t.Fatalf("Store = %q, want the fan-out for a project the store never held", env.Store)
	}
	if !strings.Contains(env.StoreNote, "knows no project") {
		t.Errorf("StoreNote = %q, want it to name the reason", env.StoreNote)
	}
	if len(env.Results) != 1 || env.Results[0].Project != label {
		t.Fatalf("results = %+v, want the dropped project's hit via the fan-out", env.Results)
	}
}

// TestSearchNamesProjectDatabasesTheStoreNeverFolded is the anti-shrink guard: a
// project whose database exists but was never merged is missing from every
// one-store answer, so it is named in the report and the answer declares itself
// incomplete instead of passing for a full corpus.
func TestSearchNamesProjectDatabasesTheStoreNeverFolded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	kept := t.TempDir()
	storeSession(t, kept, "sesskpt", "/src/billing", 1, "advancing the retention watermark")
	keptDB, _, _, err := index.EnsureIndexed(kept, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(kept): %v", err)
	}
	unfolded := t.TempDir()
	storeSession(t, unfolded, "sessunf", "/src/ledger", 1, "the watermark in weekly planning")
	if _, _, _, err := index.EnsureIndexed(unfolded, false); err != nil {
		t.Fatalf("EnsureIndexed(unfolded): %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{keptDB}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	// No narrowing this time, so the store answers — and must still admit what it
	// could not see.
	env := Search("watermark", nil, SearchOpts{Limit: 8, ScopeFallback: noFallback}, nil)
	if env.Store != StoreConsolidated {
		t.Fatalf("Store = %q (note %q), want the one store", env.Store, env.StoreNote)
	}
	var named []string
	for _, s := range env.Scopes {
		if s.Status == ScopeNotConsolidated {
			named = append(named, s.Dir)
		}
	}
	if len(named) != 1 {
		t.Fatalf("not-consolidated rows = %v, want the one unfolded project db; report %+v", named, env.Scopes)
	}
	if env.Complete {
		t.Error("Complete = true while a project database was outside the answer")
	}
}

// TestSearchWidensUntilItHasEnoughConversations guards the candidate window. The
// fan-out funded every project with its own window; one global window of the
// same size is spent by the first few conversations, because the top anchors are
// several messages of the SAME conversation. Without widening, moving to one
// store would hand back a visibly thinner answer for the same query.
func TestSearchWidensUntilItHasEnoughConversations(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	// Six conversations, each long enough that two of them exhaust the opening
	// window on their own (limit 4 opens a window of 30 anchors).
	for i := 0; i < 6; i++ {
		storeSession(t, proj, fmt.Sprintf("sess%03d", i), "/src/billing", 25,
			"advancing the retention watermark")
	}
	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	env := Search("watermark", nil, SearchOpts{Limit: 4, ScopeFallback: noFallback}, nil)
	if env.Store != StoreConsolidated {
		t.Fatalf("Store = %q (note %q), want the one store", env.Store, env.StoreNote)
	}
	if len(env.Results) != 4 {
		t.Fatalf("results = %d, want the limit filled: the window must widen until it "+
			"holds enough distinct conversations", len(env.Results))
	}
}

// TestSearchProvesExhaustionBeforeCallingATotalExact is the honesty half of the
// widening. A window that stopped as soon as it had enough conversations says
// nothing about how many more there were, so the total stays a declared floor.
// Only a wider window that finds nothing new proves the corpus is exhausted.
func TestSearchProvesExhaustionBeforeCallingATotalExact(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	for i := 0; i < 6; i++ {
		storeSession(t, proj, fmt.Sprintf("sess%03d", i), "/src/billing", 25,
			"advancing the retention watermark")
	}
	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	// Limit 4 out of six conversations: the window stops on "enough", so the
	// total it produced is a floor and the envelope must say so.
	floor := Search("watermark", nil, SearchOpts{Limit: 4, ScopeFallback: noFallback}, nil)
	if !floor.TotalIsLowerBound {
		t.Errorf("TotalIsLowerBound = false while the window stopped on enough, not on exhaustion: %+v", floor)
	}
	if floor.Complete {
		t.Error("Complete = true while more matches may exist beyond the window")
	}

	// A limit no corpus this size can fill drives the widening to exhaustion, and
	// an exhausted window makes the total exact.
	exact := Search("watermark", nil, SearchOpts{Limit: 50, ScopeFallback: noFallback}, nil)
	if exact.TotalIsLowerBound {
		t.Errorf("TotalIsLowerBound = true after a wider window found nothing new: %+v", exact)
	}
	if exact.TotalMatches != 6 {
		t.Errorf("TotalMatches = %d, want all 6 conversations once the corpus is exhausted", exact.TotalMatches)
	}
	if !exact.Complete {
		t.Errorf("Complete = false on an exhausted, unfiltered answer: %+v", exact)
	}
}

// TestRenderScopeFooterTellsTheTruthInItsOneStoreShape covers the human surface
// of the completeness report. The JSON envelope carrying an honest
// not_consolidated row is worth nothing if the text a person reads stays silent
// about it, and the same goes for a search that quietly fell back to the
// fan-out: both are degradations, and both have to be visible without opening
// the JSON.
func TestRenderScopeFooterTellsTheTruthInItsOneStoreShape(t *testing.T) {
	var full strings.Builder
	renderScopeFooter(&full, SearchEnvelope{
		Store:    StoreConsolidated,
		Complete: true,
		Scopes:   []ScopeReport{{Dir: "p.db", Status: ScopeSearched, Detail: "12 sessions"}},
	})
	if full.String() != "" {
		t.Errorf("a complete one-store answer printed a footer: %q", full.String())
	}

	var partial strings.Builder
	renderScopeFooter(&partial, SearchEnvelope{
		Store: StoreConsolidated,
		Scopes: []ScopeReport{
			{Dir: "p.db", Status: ScopeSearched, Detail: "12 sessions"},
			{Dir: "billing.db", Status: ScopeNotConsolidated},
			{Dir: "ledger.db", Status: ScopeNotConsolidated},
		},
	})
	out := partial.String()
	if !strings.Contains(out, "2 project database(s) are not in the one store") {
		t.Errorf("footer hides the unfolded project databases:\n%s", out)
	}
	if !strings.Contains(out, "rawclaw consolidate") {
		t.Errorf("footer names the problem without the fix:\n%s", out)
	}

	var fell strings.Builder
	renderScopeFooter(&fell, SearchEnvelope{
		Store:     StorePerProject,
		StoreNote: "one store unavailable — searched per project instead",
		Complete:  true,
		Scopes:    []ScopeReport{{Dir: "p.db", Status: ScopeSearched}},
	})
	if !strings.Contains(fell.String(), "one store unavailable") {
		t.Errorf("footer hides the fallback to the fan-out:\n%s", fell.String())
	}
}

// contains reports whether xs holds s.
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
