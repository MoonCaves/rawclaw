package agentproto

import (
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// TestTopicsRanksAcrossProjectsNotByScopeOrder pins the real defect.
//
// Each project is its own SQLite file, so Topics sweeps them one at a time.
// It used to append each project's hits in iteration order, which meant the
// FIRST project swept owned the top of the list however weak its matches were.
// Observed on a large real-world corpus: `topics "adversarial"` led with two
// segments that only mention the word inside a summary, while sixteen segments
// LABELLED "Adversarial …" sat below them — purely because those lived in a
// project that sorted later.
//
// Fixture: project A (swept first, alphabetically earlier temp label is not
// guaranteed, so this asserts on CONTENT not position) holds a weak
// summary-only match; project B holds the strong label match. The strong one
// must come first regardless of which file was opened first.
func TestTopicsRanksAcrossProjectsNotByScopeOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	weakProj := t.TempDir()
	strongProj := t.TempDir()

	const weakUUID = "9f3e1c20-aaaa-bbbb-cccc-00000000000a"
	const strongUUID = "9f3e1c20-aaaa-bbbb-cccc-00000000000b"

	writeSession(t, weakProj, "sessweak", weakUUID, "a session about handoffs")
	writeSession(t, strongProj, "sessstrong", strongUUID, "a session about review")

	// Weak: the query word appears only in a long summary, never in the label.
	tagProject(t, weakProj, "sessweak", weakUUID,
		"Release checklist for the next tag",
		"Walked the release checklist end to end, re-read the tagging conventions, and "+
			"drafted the notes summarizing the wrapper design so it could get an adversarial read later.")

	// Strong: the query word IS the label.
	tagProject(t, strongProj, "sessstrong", strongUUID,
		"Adversarial review of tag conflict design",
		"Worked through the merge approach and a provenance-authority alternative.")

	// Sweep with the WEAK project first — the order that used to decide the winner.
	scope := []view.Scope{
		{Project: paths.ProjectLabel(weakProj), TDir: weakProj},
		{Project: paths.ProjectLabel(strongProj), TDir: strongProj},
	}

	res, err := Topics("adversarial", scope, 8, "")
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("want both projects to match, got %d: %+v", len(res.Hits), res.Hits)
	}
	if res.Hits[0].Topic != "Adversarial review of tag conflict design" {
		t.Errorf("label match must outrank a summary-only match from an earlier-swept project;\n got order: %q then %q",
			res.Hits[0].Topic, res.Hits[1].Topic)
	}
}

// TestTopicsLimitCapsCombinedList pins the flag contract the ranking change
// redefined: --limit caps the COMBINED, globally-ranked list. Capping per
// project would return 2 here (one from each), which is the old behaviour and
// would make a global "top N" impossible to ask for.
func TestTopicsLimitCapsCombinedList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	projA := t.TempDir()
	projB := t.TempDir()
	const uuidA = "9f3e1c20-aaaa-bbbb-cccc-00000000000c"
	const uuidB = "9f3e1c20-aaaa-bbbb-cccc-00000000000d"

	writeSession(t, projA, "sessaaa", uuidA, "first session")
	writeSession(t, projB, "sessbbb", uuidB, "second session")
	tagProject(t, projA, "sessaaa", uuidA, "Rollback discussion one", "explored the rollback")
	tagProject(t, projB, "sessbbb", uuidB, "Rollback discussion two", "explored the rollback again")

	scope := []view.Scope{
		{Project: paths.ProjectLabel(projA), TDir: projA},
		{Project: paths.ProjectLabel(projB), TDir: projB},
	}

	res, err := Topics("rollback", scope, 1, "")
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Errorf("--limit 1 must cap the combined list, got %d hits: %+v", len(res.Hits), res.Hits)
	}
}

// tagProject indexes a project and attaches one topic segment to a session in it.
func tagProject(t *testing.T, proj, sessionID, startUUID, topic, summary string) {
	t.Helper()
	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(%s): %v", proj, err)
	}
	con := openCacheRW(t, dbp)
	defer con.Close()
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, sessionID, startUUID, "", topic, summary, 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
}
