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

func TestPublishSessionEqualRevisionKeepsExisting(t *testing.T) {
	con := newTagTestDB(t)
	sid := "equal-revision"
	first := store.TopicSegment{SessionID: sid, StartUUID: "first", EndUUID: "first", Topic: "first", TaggedAt: 10}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{first}); err != nil {
		t.Fatal(err)
	}
	second := store.TopicSegment{SessionID: sid, StartUUID: "second", EndUUID: "second", Topic: "second", TaggedAt: 10}
	if err := publishSession(context.Background(), con, sid, []store.TopicSegment{second}, store.Verdict{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Topic != "first" {
		t.Fatalf("segments = %#v, want first equal-revision set", got)
	}
}

// Empty source state has no durable revision, so an older snapshot must not
// silently clear a current local set. A deletion requires a recorded revision.
func TestPublishSessionEmptyWithoutRevisionCannotClear(t *testing.T) {
	con := newTagTestDB(t)
	sid := "empty-no-revision"
	current := store.TopicSegment{SessionID: sid, StartUUID: "current", EndUUID: "current", Topic: "current", TaggedAt: 10}
	if err := store.ReplaceSessionSegments(con, sid, []store.TopicSegment{current}); err != nil {
		t.Fatal(err)
	}
	if err := publishSession(context.Background(), con, sid, nil, store.Verdict{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Topic != "current" {
		t.Fatalf("segments = %#v, want current set preserved", got)
	}
}
