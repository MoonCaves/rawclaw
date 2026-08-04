package store_test

import (
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// TestMatchTopicsStillMatchesSummaryOnly guards the cost of weighting the label
// column above the summary (see topicColumnWeight): the summary must not become
// dead weight. A term that appears ONLY in a summary still has to come back —
// the summary is there for recall.
//
// The ORDERING half of that change is pinned in agentproto
// (TestTopicsRanksAcrossProjectsNotByScopeOrder), because the defect that
// prompted it was cross-database: within one database bm25's length
// normalization already favours the short label, so a single-db fixture cannot
// distinguish weighted from unweighted.
func TestMatchTopicsStillMatchesSummaryOnly(t *testing.T) {
	con, _ := storetest.NewDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	const sid = "sess0002"

	storetest.InsertSession(t, con, storetest.Session{ID: sid, MessageCount: 1})
	storetest.InsertMessage(t, con, storetest.Message{SessionID: sid, Role: "user", Content: "first", UUID: "u-only"})

	if err := store.UpsertTopicSegment(con, sid, "u-only", "",
		"Session orientation and handoff review",
		"Explored the prior handoff and traced a wget-versus-curl IPv6 mismatch.",
		1); err != nil {
		t.Fatal(err)
	}

	hits, err := store.MatchTopics(con, "IPv6", 10)
	if err != nil {
		t.Fatalf("MatchTopics: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("a summary-only term must still match; got %d hits", len(hits))
	}
}
