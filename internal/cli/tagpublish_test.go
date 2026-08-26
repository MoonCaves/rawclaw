package cli

import (
	"context"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// This red test protects co-contributors: a local publication for one source
// must not erase another source's valid contribution for the same session.
func TestPublishSession_PreservesForeignContributor(t *testing.T) {
	con := newTagTestDB(t)
	sid := "shared-session"
	foreign := store.TopicSegment{SessionID: sid, StartUUID: "foreign", EndUUID: "foreign", Topic: "foreign", OriginMachine: "other-machine"}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{foreign}); err != nil {
		t.Fatal(err)
	}
	local := store.TopicSegment{SessionID: sid, StartUUID: "local", EndUUID: "local", Topic: "local"}
	if err := publishSession(context.Background(), con, sid, []store.TopicSegment{local}, store.Verdict{}, false); err != nil {
		t.Fatal(err)
	}
	segments, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 {
		t.Fatalf("published segments = %#v, want local plus foreign contributor", segments)
	}
}
