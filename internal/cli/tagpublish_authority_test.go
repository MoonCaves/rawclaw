package cli

import (
	"context"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestPublishSessionPreservesForeignAndRejectsOlderLocal(t *testing.T) {
	con := newTagTestDB(t)
	sid := "shared-session"
	foreign := store.TopicSegment{SessionID: sid, StartUUID: "foreign", EndUUID: "foreign", Topic: "foreign", OriginMachine: "other", TaggedAt: 20}
	local := store.TopicSegment{SessionID: sid, StartUUID: "new", EndUUID: "new", Topic: "new", TaggedAt: 10}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{foreign, local}); err != nil {
		t.Fatal(err)
	}
	older := store.TopicSegment{SessionID: sid, StartUUID: "old", EndUUID: "old", Topic: "old", TaggedAt: 5}
	if err := publishSession(context.Background(), con, sid, []store.TopicSegment{older}, store.Verdict{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Topic != "foreign" || got[1].Topic != "new" {
		t.Fatalf("segments = %#v, want foreign + newer local", got)
	}
}
