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
		if a.MissingSince != 0 {
			t.Errorf("anchor %+v MissingSince = %v, want 0 (present)", a, a.MissingSince)
		}
	}

	// The session's missing_since watermark (retained-but-missing, D7) surfaces.
	storetest.SetSessionField(t, con, "alpha", "missing_since", 99.5)
	anchors, _ = store.SearchAnchors(con, "needle", store.Filter{}, store.SortRelevance, 10)
	for _, a := range anchors {
		if a.MissingSince != 99.5 {
			t.Errorf("anchor MissingSince = %v, want 99.5", a.MissingSince)
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
