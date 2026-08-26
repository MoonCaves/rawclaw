package cli

import (
	"context"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestPublishSessionAuthorityAndRevision(t *testing.T) {
	con := newTagTestDB(t)
	sid := "shared-session"
	foreign := store.TopicSegment{SessionID: sid, StartUUID: "foreign", EndUUID: "foreign", Topic: "foreign", OriginMachine: "other", TaggedAt: 20}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{foreign}); err != nil {
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
	if len(got) != 1 || got[0].Topic != "foreign" {
		t.Fatalf("segments = %#v, want authoritative foreign set", got)
	}
	higher := store.TopicSegment{SessionID: sid, StartUUID: "higher", EndUUID: "higher", Topic: "higher", OriginMachine: "zz", TaggedAt: 1}
	if err := publishSession(context.Background(), con, sid, []store.TopicSegment{higher}, store.Verdict{}, false); err != nil {
		t.Fatal(err)
	}
	got, err = store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Topic != "higher" {
		t.Fatalf("segments = %#v, want higher-authority replacement", got)
	}
}
