package store_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// seedSearchCorpus builds the shared FTS fixture:
//
//	alpha (top-level, 5 msgs): user "needle one …" @2026-01-01, assistant "needle two …" @2026-01-02
//	beta  (subagent thread):   user "needle sub …" @2026-01-03
//	gamma (top-level, 1 msg):  user "haystack only" (no needle)
func seedSearchCorpus(t *testing.T) *sql.DB {
	t.Helper()
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "alpha", MessageCount: 5})
	storetest.InsertSession(t, con, storetest.Session{ID: "beta", IsSubagent: true, ParentID: "alpha", MessageCount: 2})
	storetest.InsertSession(t, con, storetest.Session{ID: "gamma", MessageCount: 1})
	storetest.InsertMessage(t, con, storetest.Message{
		SessionID: "alpha", Role: "user", Content: "needle one in the alpha session",
		TS: 100, ISO: "2026-01-01T10:00:00Z", UUID: "uuid-a1"})
	storetest.InsertMessage(t, con, storetest.Message{
		SessionID: "alpha", Role: "assistant", Content: "needle two replies here",
		TS: 200, ISO: "2026-01-02T10:00:00Z", UUID: "uuid-a2"})
	storetest.InsertMessage(t, con, storetest.Message{
		SessionID: "beta", Role: "user", Content: "needle sub thread text",
		TS: 300, ISO: "2026-01-03T10:00:00Z", UUID: "uuid-b1"})
	storetest.InsertMessage(t, con, storetest.Message{
		SessionID: "gamma", Role: "user", Content: "haystack only",
		TS: 400, ISO: "2026-01-04T10:00:00Z", UUID: "uuid-g1"})
	return con
}

func hitSessions(hits []store.SearchHit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.SessionID)
	}
	return out
}

func TestSearchHitsFilters(t *testing.T) {
	con := seedSearchCorpus(t)

	// Default filter: subagent rows excluded.
	hits, err := store.SearchHits(con, "needle", store.Filter{}, store.SortRelevance, 10)
	if err != nil {
		t.Fatalf("SearchHits: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("SearchHits default = %d (%v), want 2 (beta excluded)", len(hits), hitSessions(hits))
	}
	for _, h := range hits {
		if h.SessionID != "alpha" || h.IsSubagent {
			t.Errorf("unexpected hit %+v", h)
		}
	}
	// Snippet carries the byte-identical >>> <<< markers; Content is the raw text.
	if !strings.Contains(hits[0].Snippet, ">>>needle<<<") {
		t.Errorf("Snippet = %q, want >>>needle<<< markers", hits[0].Snippet)
	}
	if hits[0].Content == "" {
		t.Error("Content empty — the coverage/snippet-rebuild input must be carried")
	}

	// IncludeSubagents pulls beta in, with parent + flag set.
	hits, _ = store.SearchHits(con, "needle", store.Filter{IncludeSubagents: true}, store.SortRelevance, 10)
	if len(hits) != 3 {
		t.Fatalf("SearchHits incl-sub = %d, want 3", len(hits))
	}
	var sawBeta bool
	for _, h := range hits {
		if h.SessionID == "beta" {
			sawBeta = true
			if !h.IsSubagent || h.Parent != "alpha" {
				t.Errorf("beta hit = %+v, want IsSubagent + Parent alpha", h)
			}
		}
	}
	if !sawBeta {
		t.Error("beta hit missing under IncludeSubagents")
	}

	// Role filter.
	hits, _ = store.SearchHits(con, "needle", store.Filter{Role: "assistant"}, store.SortRelevance, 10)
	if len(hits) != 1 || hits[0].Role != "assistant" {
		t.Errorf("SearchHits role = %v, want the one assistant row", hitSessions(hits))
	}

	// MinMessages: beta (2) drops under a 5-message floor even when included.
	hits, _ = store.SearchHits(con, "needle", store.Filter{IncludeSubagents: true, MinMessages: 5}, store.SortRelevance, 10)
	if len(hits) != 2 {
		t.Errorf("SearchHits min-messages = %d (%v), want 2", len(hits), hitSessions(hits))
	}

	// Date bounds are inclusive on the ISO date prefix.
	hits, _ = store.SearchHits(con, "needle", store.Filter{SinceDate: "2026-01-02"}, store.SortRelevance, 10)
	if len(hits) != 1 || hits[0].ISO != "2026-01-02T10:00:00Z" {
		t.Errorf("SearchHits since = %v, want the 01-02 row", hits)
	}
	hits, _ = store.SearchHits(con, "needle", store.Filter{BeforeDate: "2026-01-01"}, store.SortRelevance, 10)
	if len(hits) != 1 || hits[0].ISO != "2026-01-01T10:00:00Z" {
		t.Errorf("SearchHits before = %v, want the 01-01 row", hits)
	}
	// Combined since+before window.
	hits, _ = store.SearchHits(con, "needle", store.Filter{IncludeSubagents: true, SinceDate: "2026-01-02", BeforeDate: "2026-01-02"}, store.SortRelevance, 10)
	if len(hits) != 1 || hits[0].SessionID != "alpha" {
		t.Errorf("SearchHits window = %v, want one alpha row", hitSessions(hits))
	}

	// limit caps; no match reads as empty, not error.
	if hits, _ := store.SearchHits(con, "needle", store.Filter{IncludeSubagents: true}, store.SortRelevance, 1); len(hits) != 1 {
		t.Errorf("SearchHits limit = %d, want 1", len(hits))
	}
	if hits, err := store.SearchHits(con, "zzzmissing", store.Filter{}, store.SortRelevance, 10); err != nil || len(hits) != 0 {
		t.Errorf("SearchHits miss = %v (%v), want empty", hits, err)
	}
}

// seedScopedCorpus builds a corpus that spans projects and sources, which is
// what the one store looks like:
//
//	ledger  (project "ledger",  source "claude"): "needle in the ledger"
//	billing (project "billing", source "claude"): "needle in billing"
//	rollout (project "billing", source "codex"):  "needle in the rollout"
//	nolabel (no project, no source):              "needle with no label"
//
// "billing" is recorded under two different directories, which is what a
// project checked out twice looks like.
func seedScopedCorpus(t *testing.T) *sql.DB {
	t.Helper()
	con, _ := storetest.NewDB(t)
	rows := []struct {
		id, project, cwd, source, text string
	}{
		{"ledger", "ledger", "/src/ledger", "claude", "needle in the ledger"},
		{"billing", "billing", "/src/billing", "claude", "needle in billing"},
		{"rollout", "billing", "/work/billing", "codex", "needle in the rollout"},
		{"nolabel", "", "", "", "needle with no label"},
	}
	for i, r := range rows {
		storetest.InsertSession(t, con, storetest.Session{
			ID: r.id, MessageCount: 1, Project: r.project, CWD: r.cwd, SourceTool: r.source})
		storetest.InsertMessage(t, con, storetest.Message{
			SessionID: r.id, Role: "user", Content: r.text,
			TS: float64(100 * (i + 1)), ISO: "2026-01-01T10:00:00Z", UUID: "uuid-" + r.id})
	}
	return con
}

func TestSearchHitsScopeFilters(t *testing.T) {
	con := seedScopedCorpus(t)

	// No scope set searches every project — including the rows that carry no
	// label at all, which is the whole point of the default being empty.
	hits, err := store.SearchHits(con, "needle", store.Filter{}, store.SortRelevance, 10)
	if err != nil {
		t.Fatalf("SearchHits: %v", err)
	}
	if len(hits) != 4 {
		t.Fatalf("unscoped = %d (%v), want all 4", len(hits), hitSessions(hits))
	}

	// One project narrows to that project's sessions.
	hits, _ = store.SearchHits(con, "needle", store.Filter{Projects: []string{"ledger"}}, store.SortRelevance, 10)
	if len(hits) != 1 || hits[0].SessionID != "ledger" {
		t.Errorf("one project = %v, want [ledger]", hitSessions(hits))
	}

	// Several projects union, and one project holding two sessions returns both.
	hits, _ = store.SearchHits(con, "needle", store.Filter{Projects: []string{"ledger", "billing"}}, store.SortNewest, 10)
	if len(hits) != 3 {
		t.Errorf("two projects = %v, want ledger + both billing rows", hitSessions(hits))
	}
	for _, h := range hits {
		if h.SessionID == "nolabel" {
			t.Error("an unlabeled row answered a project-scoped search")
		}
	}

	// A project nobody has reads as empty, not as everything.
	if hits, err := store.SearchHits(con, "needle", store.Filter{Projects: []string{"absent"}}, store.SortRelevance, 10); err != nil || len(hits) != 0 {
		t.Errorf("unknown project = %v (%v), want empty", hitSessions(hits), err)
	}

	// Source narrows independently, and composes with project: "billing" holds
	// one claude session and one codex session, so together they pick out one.
	hits, _ = store.SearchHits(con, "needle", store.Filter{SourceTool: "codex"}, store.SortRelevance, 10)
	if len(hits) != 1 || hits[0].SessionID != "rollout" {
		t.Errorf("source filter = %v, want [rollout]", hitSessions(hits))
	}
	hits, _ = store.SearchHits(con, "needle", store.Filter{Projects: []string{"billing"}, SourceTool: "claude"}, store.SortRelevance, 10)
	if len(hits) != 1 || hits[0].SessionID != "billing" {
		t.Errorf("project+source = %v, want [billing]", hitSessions(hits))
	}

	// Anchors compose the same filters as hits.
	anchors, err := store.SearchAnchors(con, "needle", store.Filter{Projects: []string{"ledger"}}, store.SortRelevance, 10)
	if err != nil || len(anchors) != 1 || anchors[0].SessionID != "ledger" {
		t.Errorf("scoped anchors = %v (%v), want the one ledger row", anchors, err)
	}
}

func TestSearchAnchorsCarryProject(t *testing.T) {
	con := seedScopedCorpus(t)

	// Every anchor names the project it came from. An unscoped search over the
	// one store returns rows from several projects at once, so a row that does
	// not say where it came from cannot be rendered or grouped.
	anchors, err := store.SearchAnchors(con, "needle", store.Filter{}, store.SortNewest, 10)
	if err != nil {
		t.Fatalf("SearchAnchors: %v", err)
	}
	got := map[string]string{}
	for _, a := range anchors {
		got[a.SessionID] = a.Project
	}
	want := map[string]string{
		"ledger":  "ledger",
		"billing": "billing",
		"rollout": "billing",
		"nolabel": "", // a row indexed before the scope columns has no label to carry
	}
	for sid, wantProj := range want {
		if got[sid] != wantProj {
			t.Errorf("anchor %s project = %q, want %q", sid, got[sid], wantProj)
		}
	}
}

func TestDistinctScopes(t *testing.T) {
	con := seedScopedCorpus(t)

	got, err := store.DistinctScopes(con)
	if err != nil {
		t.Fatalf("DistinctScopes: %v", err)
	}
	// One pair per directory a project was seen in, so a path pattern matching
	// either of "billing"'s two directories still selects the one label. The
	// unlabeled row contributes nothing — there is no label to select.
	want := []store.ProjectScope{
		{Project: "billing", CWD: "/src/billing"},
		{Project: "billing", CWD: "/work/billing"},
		{Project: "ledger", CWD: "/src/ledger"},
	}
	if len(got) != len(want) {
		t.Fatalf("DistinctScopes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DistinctScopes = %v, want %v", got, want)
		}
	}
}

func TestDistinctProjects(t *testing.T) {
	con := seedScopedCorpus(t)

	got, err := store.DistinctProjects(con)
	if err != nil {
		t.Fatalf("DistinctProjects: %v", err)
	}
	// Sorted, deduplicated, and the unlabeled row contributes nothing — there is
	// no label there for a caller's pattern to match against.
	want := []string{"billing", "ledger"}
	if len(got) != len(want) {
		t.Fatalf("DistinctProjects = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DistinctProjects = %v, want %v", got, want)
		}
	}
}

func TestSearchHitsSortVariants(t *testing.T) {
	con := seedSearchCorpus(t)

	hits, err := store.SearchHits(con, "needle", store.Filter{IncludeSubagents: true}, store.SortNewest, 10)
	if err != nil || len(hits) != 3 {
		t.Fatalf("SortNewest = %d rows (%v), want 3", len(hits), err)
	}
	if hits[0].ISO != "2026-01-03T10:00:00Z" || hits[2].ISO != "2026-01-01T10:00:00Z" {
		t.Errorf("SortNewest order = %v/%v, want ts DESC", hits[0].ISO, hits[2].ISO)
	}

	hits, _ = store.SearchHits(con, "needle", store.Filter{IncludeSubagents: true}, store.SortOldest, 10)
	if hits[0].ISO != "2026-01-01T10:00:00Z" || hits[2].ISO != "2026-01-03T10:00:00Z" {
		t.Errorf("SortOldest order = %v/%v, want ts ASC", hits[0].ISO, hits[2].ISO)
	}

	// Relevance: bm25 rank with m.id tiebreak — same-rank rows come back in id
	// order (alpha's two rows precede beta's here by insertion).
	hits, _ = store.SearchHits(con, "needle", store.Filter{IncludeSubagents: true}, store.SortRelevance, 10)
	if len(hits) != 3 {
		t.Fatalf("SortRelevance = %d rows, want 3", len(hits))
	}
}

func TestSearchAnchors(t *testing.T) {
	con := seedSearchCorpus(t)

	anchors, err := store.SearchAnchors(con, "needle", store.Filter{}, store.SortRelevance, 10)
	if err != nil {
		t.Fatalf("SearchAnchors: %v", err)
	}
	if len(anchors) != 2 {
		t.Fatalf("SearchAnchors = %d, want 2 (subagent excluded)", len(anchors))
	}
	// Anchor rows carry the message id, the source uuid, and the snippet.
	for _, a := range anchors {
		if a.ID == 0 || a.UUID == "" || !strings.Contains(a.Snippet, ">>>needle<<<") || a.Content == "" {
			t.Errorf("anchor %+v missing id/uuid/snippet/content", a)
		}
		if a.OnlyCopySince != 0 {
			t.Errorf("anchor %+v OnlyCopySince = %v, want 0 (present)", a, a.OnlyCopySince)
		}
	}

	// The session's only_copy_since watermark surfaces.
	storetest.SetSessionField(t, con, "alpha", "only_copy_since", 99.5)
	anchors, _ = store.SearchAnchors(con, "needle", store.Filter{}, store.SortRelevance, 10)
	for _, a := range anchors {
		if a.OnlyCopySince != 99.5 {
			t.Errorf("anchor OnlyCopySince = %v, want 99.5", a.OnlyCopySince)
		}
	}

	// Filter + sort compose the same way as SearchHits.
	anchors, _ = store.SearchAnchors(con, "needle", store.Filter{IncludeSubagents: true, Role: "user"}, store.SortNewest, 10)
	if len(anchors) != 2 || anchors[0].SessionID != "beta" {
		t.Errorf("SearchAnchors filtered = %v, want beta first (newest user rows)", anchors)
	}

	// Miss reads empty.
	if anchors, err := store.SearchAnchors(con, "zzzmissing", store.Filter{}, store.SortRelevance, 10); err != nil || len(anchors) != 0 {
		t.Errorf("SearchAnchors miss = %v (%v), want empty", anchors, err)
	}
}

func TestTopicRowsExistDegrade(t *testing.T) {
	con, _ := storetest.NewDB(t)

	// Missing topic_segment table reads as false, not an error.
	if store.TopicRowsExist(con) {
		t.Error("TopicRowsExist on missing table = true, want false")
	}

	// Present but empty still reads false.
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if store.TopicRowsExist(con) {
		t.Error("TopicRowsExist on empty table = true, want false")
	}

	// One row flips it.
	if err := store.UpsertTopicSegment(con, "s", "u1", "u2", "topic", "summary", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
	if !store.TopicRowsExist(con) {
		t.Error("TopicRowsExist with a row = false, want true")
	}
}

// TestSearchSubstringIndexAnswersMidToken pins the reason the substring index
// exists: a probe that lands inside a word is not something the word index ranks
// badly, it is something the word index cannot answer at all. The same probe
// against the trigram index returns the row, and the shared filters still apply.
func TestSearchSubstringIndexAnswersMidToken(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "ledger", MessageCount: 5})
	storetest.InsertSession(t, con, storetest.Session{ID: "billing", IsSubagent: true, ParentID: "ledger", MessageCount: 2})
	storetest.InsertMessage(t, con, storetest.Message{
		SessionID: "ledger", Role: "user", Content: "the reconciliation finished cleanly",
		TS: 100, ISO: "2026-01-01T10:00:00Z", UUID: "uuid-l1"})
	storetest.InsertMessage(t, con, storetest.Message{
		SessionID: "billing", Role: "user", Content: "reconciliation notes from the thread",
		TS: 200, ISO: "2026-01-02T10:00:00Z", UUID: "uuid-b1"})

	// "iliat" sits inside "reconciliation" and is not a word, so it falls on no
	// token boundary the word tokenizer produced.
	const probe = `"iliat"`

	hits, err := store.SearchHits(con, probe, store.Filter{IncludeSubagents: true}, store.SortRelevance, 10)
	if err != nil {
		t.Fatalf("SearchHits: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("word index answered a mid-token probe with %v — the premise of the substring index is wrong", hitSessions(hits))
	}

	hits, err = store.SearchHitsSubstring(con, probe, store.Filter{IncludeSubagents: true}, store.SortRelevance, 10)
	if err != nil {
		t.Fatalf("SearchHitsSubstring: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("SearchHitsSubstring = %v, want both rows", hitSessions(hits))
	}
	if hits[0].Content == "" {
		t.Error("Content empty — the snippet-rebuild input must be carried on the substring path too")
	}

	// The filters are the same composition, so the subagent row drops by default.
	hits, err = store.SearchHitsSubstring(con, probe, store.Filter{}, store.SortRelevance, 10)
	if err != nil || len(hits) != 1 || hits[0].SessionID != "ledger" {
		t.Errorf("SearchHitsSubstring default filter = %v (%v), want the one top-level row", hitSessions(hits), err)
	}

	// Anchor recall reaches the same row and carries the message id + uuid.
	anchors, err := store.SearchAnchorsSubstring(con, probe, store.Filter{}, store.SortRelevance, 10)
	if err != nil {
		t.Fatalf("SearchAnchorsSubstring: %v", err)
	}
	if len(anchors) != 1 || anchors[0].UUID != "uuid-l1" || anchors[0].ID == 0 {
		t.Errorf("SearchAnchorsSubstring = %+v, want the ledger anchor with its uuid", anchors)
	}
	if anchors, err := store.SearchAnchors(con, probe, store.Filter{}, store.SortRelevance, 10); err != nil || len(anchors) != 0 {
		t.Errorf("word anchors answered a mid-token probe = %+v (%v), want empty", anchors, err)
	}
}

// TestSubstringIndexTracksMessages pins that the substring index is kept in step
// with messages by the same three sync triggers the word index uses — an index
// that only tracks inserts would go quietly wrong on every edit and delete.
func TestSubstringIndexTracksMessages(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "ledger", MessageCount: 5})
	id := storetest.InsertMessage(t, con, storetest.Message{
		SessionID: "ledger", Role: "user", Content: "the reconciliation finished cleanly",
		TS: 100, ISO: "2026-01-01T10:00:00Z", UUID: "uuid-l1"})

	countSub := func(probe string) int {
		t.Helper()
		hits, err := store.SearchHitsSubstring(con, probe, store.Filter{}, store.SortRelevance, 10)
		if err != nil {
			t.Fatalf("SearchHitsSubstring(%s): %v", probe, err)
		}
		return len(hits)
	}

	// AFTER INSERT put it there.
	if n := countSub(`"iliat"`); n != 1 {
		t.Fatalf("after insert = %d rows, want 1 (the insert trigger did not index it)", n)
	}

	// AFTER UPDATE must retire the old content and index the new.
	storetest.SetMessageContent(t, con, id, "the settlement finished cleanly")
	if n := countSub(`"iliat"`); n != 0 {
		t.Errorf("after update = %d rows for the OLD substring, want 0 (stale entry left behind)", n)
	}
	if n := countSub(`"tleme"`); n != 1 {
		t.Errorf("after update = %d rows for the NEW substring, want 1", n)
	}

	// AFTER DELETE must remove it. This one is asserted against the index
	// directly: the search join drops a row whose message is gone, so a stale
	// entry would still read as "not searchable" while sitting in the index
	// forever.
	storetest.DeleteMessage(t, con, id)
	if n := storetest.CountTrigramRows(t, con, `"tleme"`); n != 0 {
		t.Errorf("after delete = %d entries left in the index, want 0", n)
	}
}
