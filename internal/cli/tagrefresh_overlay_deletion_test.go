package cli

import (
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// TestOverlayAuthoritativeTopicsDropsDeletedBoundary is an immutable red
// proof for the candidate overlay contract: a whole-session authoritative
// replacement must not resurrect a derived-only boundary that retag-all
// deleted. The fixtures are real SQLite topic stores; the candidate seam is
// the only implementation under test.
func TestOverlayAuthoritativeTopicsDropsDeletedBoundary(t *testing.T) {
	const sid = "overlay-delete-session"
	derivedDB, _ := storetest.NewDB(t)
	authoritativeDB, _ := storetest.NewDB(t)
	if err := store.EnsureTopicSchema(derivedDB); err != nil {
		t.Fatalf("derived topic schema: %v", err)
	}
	if err := store.EnsureTopicSchema(authoritativeDB); err != nil {
		t.Fatalf("authoritative topic schema: %v", err)
	}
	for _, row := range []struct {
		start, topic string
	}{
		{"aaa", "current"},
		{"bbb", "deleted-by-retag"},
	} {
		if err := store.UpsertTopicSegment(derivedDB, sid, row.start, row.start, row.topic, "derived", 1); err != nil {
			t.Fatalf("derived topic %q: %v", row.start, err)
		}
	}
	if err := store.UpsertTopicSegment(authoritativeDB, sid, "aaa", "aaa", "current-authoritative", "authoritative", 2); err != nil {
		t.Fatalf("authoritative topic: %v", err)
	}

	derived, err := store.TopicsForSession(derivedDB, sid)
	if err != nil {
		t.Fatalf("read derived topics: %v", err)
	}
	authoritative, err := store.TopicsForSession(authoritativeDB, sid)
	if err != nil {
		t.Fatalf("read authoritative topics: %v", err)
	}
	got := overlayAuthoritativeTopics(derived, authoritative)
	if len(got) != 1 || got[0].Topic != "current-authoritative" {
		t.Fatalf("overlay result = %#v, want only the authoritative whole-session set", got)
	}
}
