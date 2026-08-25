package agentproto

import (
	"context"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// fakeEmbedder is a deterministic test embedder returning fixed vectors for test strings.
type fakeEmbedder struct {
	vecs map[string][]float64
}

func (f *fakeEmbedder) Embed(_ context.Context, text string) []float64 {
	for k, v := range f.vecs {
		if strings.Contains(text, k) {
			return v
		}
	}
	return []float64{0.1, 0.2, 0.3, 0.4}
}

// setupRoutineTestCorpus creates two sessions in a temp project, returns proj dir and sids.
func setupRoutineTestCorpus(t *testing.T) (proj string, s1, s2 string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	proj = t.TempDir()

	s1 = "11111111-aaaa-bbbb-cccc-000000000001"
	s2 = "22222222-aaaa-bbbb-cccc-000000000002"

	// Both sessions match the query "deploy kubernetes cluster" with identical coverage
	writeSession(t, proj, s1, "u1111111-aaaa-bbbb-cccc-000000000001", "deploy kubernetes cluster")
	writeSession(t, proj, s2, "u2222222-aaaa-bbbb-cccc-000000000002", "deploy kubernetes cluster")

	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	// Consolidate into the one store
	if _, err := index.ConsolidateFrom([]string{dbp}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	return proj, s1, s2
}

// markRoutine writes a routine verdict directly on dbp and syncs to consolidated store.
func markRoutine(t *testing.T, dbp, sid string) {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("ConnectRW(%s): %v", dbp, err)
	}
	defer con.Close()

	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.UpsertVerdict(con, store.Verdict{
		SessionID: sid,
		Verdict:   store.VerdictRoutine,
		Source:    store.VerdictSourceAgent,
		TaggedAt:  100.0,
	}); err != nil {
		t.Fatalf("UpsertVerdict: %v", err)
	}
	if err := store.ReplaceSessionSegments(con, sid, nil); err != nil {
		t.Fatalf("ReplaceSessionSegments: %v", err)
	}
	if err := index.SyncConsolidatedFrom(dbp); err != nil {
		t.Fatalf("SyncConsolidatedFrom: %v", err)
	}
}

// tagTopicSegment writes a topic segment for sid and syncs.
func tagTopicSegment(t *testing.T, dbp, sid, startUUID, topic string) {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("ConnectRW(%s): %v", dbp, err)
	}
	defer con.Close()

	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{
		{SessionID: sid, StartUUID: startUUID, Topic: topic, TaggedAt: 100.0},
	}); err != nil {
		t.Fatalf("ReplaceSessionSegments: %v", err)
	}
	if err := index.SyncConsolidatedFrom(dbp); err != nil {
		t.Fatalf("SyncConsolidatedFrom: %v", err)
	}
}

// TestRoutine_FTS_SortsDownAtEqualRelevance verifies that in keyword FTS search,
// a routine-marked session sorts below an equal-relevance normal hit.
func TestRoutine_FTS_SortsDownAtEqualRelevance(t *testing.T) {
	proj, s1, s2 := setupRoutineTestCorpus(t)
	dbp := index.DBPath(proj)

	// Mark s1 as routine; s2 remains normal
	markRoutine(t, dbp, s1)

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	env := Search("kubernetes", scope, SearchOpts{Limit: 8}, nil)

	if len(env.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(env.Results))
	}

	// s2 (normal) must sort before s1 (routine)
	if env.Results[0].SessionID != s2 {
		t.Errorf("result[0] = %s, want normal session %s", env.Results[0].SessionID, s2)
	}
	if env.Results[1].SessionID != s1 {
		t.Errorf("result[1] = %s, want routine session %s", env.Results[1].SessionID, s1)
	}
	if !env.Results[1].Routine {
		t.Errorf("expected routine session %s to have Routine=true in SearchRef", s1)
	}
	if env.Results[0].Routine {
		t.Errorf("expected normal session %s to have Routine=false in SearchRef", s2)
	}
}

// TestRoutine_FTS_SoleMatchReturned verifies that a sole-match routine session is
// still returned (never hidden).
func TestRoutine_FTS_SoleMatchReturned(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()

	sid := "sole-routine-sess-1"
	writeSession(t, proj, sid, "u1111111-sole", "unique needle in routine session")
	dbp, _, _, _ := index.EnsureIndexed(proj, false)
	index.ConsolidateFrom([]string{dbp}, true)
	markRoutine(t, dbp, sid)

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	env := Search("needle", scope, SearchOpts{Limit: 8}, nil)

	if len(env.Results) != 1 {
		t.Fatalf("got %d results, want 1 (sole match routine must not be hidden)", len(env.Results))
	}
	if env.Results[0].SessionID != sid {
		t.Errorf("got session %s, want %s", env.Results[0].SessionID, sid)
	}
	if !env.Results[0].Routine {
		t.Errorf("expected SearchRef.Routine to be true")
	}
}

// TestRoutine_Semantic_SortsDownAtEqualRelevance verifies that in semantic search,
// a routine-marked session sorts below an equal-relevance normal hit.
func TestRoutine_Semantic_SortsDownAtEqualRelevance(t *testing.T) {
	proj, s1, s2 := setupRoutineTestCorpus(t)
	dbp := index.DBPath(proj)

	// Mark s1 as routine; s2 remains normal
	markRoutine(t, dbp, s1)

	// Seed identical vectors for s1 and s2 in the project db so they have equal semantic relevance
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("ConnectRW: %v", err)
	}
	if err := store.EnsureVecSchema(con); err != nil {
		t.Fatalf("EnsureVecSchema: %v", err)
	}
	testVec := []float64{1.0, 0.0, 0.0, 0.0}
	m1, _ := store.SessionMessages(con, s1)
	m2, _ := store.SessionMessages(con, s2)
	if len(m1) == 0 || len(m2) == 0 {
		t.Fatalf("missing messages")
	}
	con.Close()

	emb := &fakeEmbedder{vecs: map[string][]float64{
		"deploy": testVec,
	}}

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	env := Search("deploy", scope, SearchOpts{Limit: 8}, emb)

	if len(env.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(env.Results))
	}
	if env.Results[0].SessionID != s2 {
		t.Errorf("semantic search result[0] = %s, want normal session %s", env.Results[0].SessionID, s2)
	}
	if env.Results[1].SessionID != s1 {
		t.Errorf("semantic search result[1] = %s, want routine session %s", env.Results[1].SessionID, s1)
	}
}

// TestRoutine_Topics_SortsDownAtEqualRelevance verifies that in topic search,
// a routine-marked session sorts below an equal-relevance normal hit.
func TestRoutine_Topics_SortsDownAtEqualRelevance(t *testing.T) {
	proj, s1, s2 := setupRoutineTestCorpus(t)
	dbp := index.DBPath(proj)

	// Tag both sessions with the same topic label
	tagTopicSegment(t, dbp, s1, "u1111111-aaaa-bbbb-cccc-000000000001", "infra deployment")
	tagTopicSegment(t, dbp, s2, "u2222222-aaaa-bbbb-cccc-000000000002", "infra deployment")

	// Mark s1 as routine explicitly via verdict
	con, _ := store.ConnectRW(dbp)
	_ = store.UpsertVerdict(con, store.Verdict{
		SessionID: s1,
		Verdict:   store.VerdictRoutine,
		Source:    store.VerdictSourceAgent,
		TaggedAt:  100.0,
	})
	con.Close()
	_ = index.SyncConsolidatedFrom(dbp)

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	res, err := Topics("infra deployment", scope, TopicsOpts{Limit: 8})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(res.Hits) < 2 {
		t.Fatalf("got %d hits, want 2", len(res.Hits))
	}

	// Normal hit must sort before routine hit
	if strings.Contains(res.Hits[0].ReadRef, s1[:8]) {
		t.Errorf("topic hit[0] is routine session %s, want normal session %s", s1, s2)
	}
}

// TestRoutine_Topics_SoleMatchReturned verifies that a sole-match routine topic hit
// is still returned.
func TestRoutine_Topics_SoleMatchReturned(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()

	sid := "sole-topic-routine-1"
	writeSession(t, proj, sid, "u1111111-topic", "some message")
	dbp, _, _, _ := index.EnsureIndexed(proj, false)
	index.ConsolidateFrom([]string{dbp}, true)
	tagTopicSegment(t, dbp, sid, "u1111111-topic", "unique topic concept")

	con, _ := store.ConnectRW(dbp)
	_ = store.UpsertVerdict(con, store.Verdict{
		SessionID: sid,
		Verdict:   store.VerdictRoutine,
		Source:    store.VerdictSourceAgent,
		TaggedAt:  100.0,
	})
	con.Close()
	_ = index.SyncConsolidatedFrom(dbp)

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	res, err := Topics("unique topic", scope, TopicsOpts{Limit: 8})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(res.Hits))
	}
	if !strings.Contains(res.Hits[0].ReadRef, sid[:8]) {
		t.Errorf("got hit %s, want %s", res.Hits[0].ReadRef, sid)
	}
}

// TestRoutine_HigherRelevanceOutranksNormal checks that a routine session with
// higher relevance (e.g. higher term coverage) still ranks above a lower-relevance
// normal session (no destructive demotion).
func TestRoutine_HigherRelevanceOutranksNormal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()

	sRoutine := "11111111-routine-high-cov"
	sNormal := "22222222-normal-low-cov"

	// sRoutine matches both "database" and "migration" (cov=2)
	writeSession(t, proj, sRoutine, "u1111111-cov2", "database migration in production")
	// sNormal matches only "database" (cov=1)
	writeSession(t, proj, sNormal, "u2222222-cov1", "database query")

	dbp, _, _, _ := index.EnsureIndexed(proj, false)
	index.ConsolidateFrom([]string{dbp}, true)
	markRoutine(t, dbp, sRoutine)

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	env := Search("database migration", scope, SearchOpts{Limit: 8}, nil)

	if len(env.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(env.Results))
	}

	// sRoutine (cov=2) must rank above sNormal (cov=1) because relevance takes precedence
	if env.Results[0].SessionID != sRoutine {
		t.Errorf("result[0] = %s (cov %d), want higher-relevance routine session %s",
			env.Results[0].SessionID, 1, sRoutine)
	}
	if env.Results[1].SessionID != sNormal {
		t.Errorf("result[1] = %s, want %s", env.Results[1].SessionID, sNormal)
	}
}

// TestRoutine_Rendering_ShowsRoutineMarker verifies that rendered search and topic
// results display the routine marker.
func TestRoutine_Rendering_ShowsRoutineMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()

	sid := "11111111-routine-marker-test"
	writeSession(t, proj, sid, "u1111111-marker", "marker test message")
	dbp, _, _, _ := index.EnsureIndexed(proj, false)
	index.ConsolidateFrom([]string{dbp}, true)
	markRoutine(t, dbp, sid)

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	env := Search("marker", scope, SearchOpts{Limit: 8}, nil)

	var sBuf strings.Builder
	renderSearch(&sBuf, env, "marker", "across this project")
	sOut := sBuf.String()
	if !strings.Contains(sOut, " · routine") {
		t.Errorf("renderSearch missing routine marker in output:\n%s", sOut)
	}

	// Topic search rendering
	tagTopicSegment(t, dbp, sid, "u1111111-marker", "maintenance task")
	con, _ := store.ConnectRW(dbp)
	_ = store.UpsertVerdict(con, store.Verdict{
		SessionID: sid,
		Verdict:   store.VerdictRoutine,
		Source:    store.VerdictSourceAgent,
		TaggedAt:  100.0,
	})
	con.Close()
	_ = index.SyncConsolidatedFrom(dbp)

	tRes, err := Topics("maintenance", scope, TopicsOpts{Limit: 8})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	var tBuf strings.Builder
	renderTopics(&tBuf, tRes)
	tOut := tBuf.String()
	if !strings.Contains(tOut, " · routine") {
		t.Errorf("renderTopics missing routine marker in output:\n%s", tOut)
	}
}

// TestRoutine_ReTagReverses_SearchPartition verifies that re-tagging a routine session
// with real topic segments reverses its routine status in search ranking, and re-tagging
// with routine restores it.
func TestRoutine_ReTagReverses_SearchPartition(t *testing.T) {
	proj, s1, s2 := setupRoutineTestCorpus(t)
	dbp := index.DBPath(proj)

	// Step A: Mark s1 as routine
	markRoutine(t, dbp, s1)

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	envA := Search("kubernetes", scope, SearchOpts{Limit: 8}, nil)
	if len(envA.Results) != 2 || envA.Results[0].SessionID != s2 {
		t.Fatalf("step A: want s2 first, got %v", envA.Results)
	}

	// Step B: Re-tag s1 with a real topic segment -> demotes routine
	tagTopicSegment(t, dbp, s1, "u1111111-aaaa-bbbb-cccc-000000000001", "kubernetes cluster rollout")

	envB := Search("kubernetes", scope, SearchOpts{Limit: 8}, nil)
	if len(envB.Results) != 2 {
		t.Fatalf("step B: want 2 results, got %d", len(envB.Results))
	}
	if envB.Results[0].Routine || envB.Results[1].Routine {
		t.Errorf("step B: expected neither session to be routine after real tag, got r0=%v, r1=%v",
			envB.Results[0].Routine, envB.Results[1].Routine)
	}

	// Step C: Re-tag s1 with routine -> restores routine status
	markRoutine(t, dbp, s1)

	envC := Search("kubernetes", scope, SearchOpts{Limit: 8}, nil)
	if len(envC.Results) != 2 || envC.Results[0].SessionID != s2 {
		t.Fatalf("step C: want s2 first, got %v", envC.Results)
	}
	if !envC.Results[1].Routine {
		t.Errorf("step C: expected s1 to be routine again")
	}
}
