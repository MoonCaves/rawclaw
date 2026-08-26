package cli

import (
	"context"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestPublishSessionRejectsOutOfOrderSnapshot proves that two detached
// publishers can commit in reverse order: an older snapshot can overwrite a
// newer authoritative tag set because publishSession has no revision/ordering
// guard. The expected invariant is that the newer publication remains visible.
func TestPublishSessionRejectsOutOfOrderSnapshot(t *testing.T) {
	con := newTagTestDB(t)
	sid := "ordering-session"
	older := []store.TopicSegment{{SessionID: sid, StartUUID: "u-a", Topic: "older"}}
	newer := []store.TopicSegment{{SessionID: sid, StartUUID: "u-a", Topic: "newer"}}
	if err := publishSession(context.Background(), con, sid, newer, store.Verdict{}, false); err != nil {
		t.Fatalf("publish newer: %v", err)
	}
	if err := publishSession(context.Background(), con, sid, older, store.Verdict{}, false); err != nil {
		t.Fatalf("publish older: %v", err)
	}
	segs, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 || segs[0].Topic != "newer" {
		t.Fatalf("out-of-order publication left %#v, want newer snapshot", segs)
	}
}
