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

func TestPublishSessionOlderLocalVerdictCannotOverwrite(t *testing.T) {
	con := newTagTestDB(t)
	sid := "verdict-order"
	if err := store.UpsertVerdict(con, store.Verdict{SessionID: sid, Verdict: store.VerdictRoutine, Source: store.VerdictSourceAgent, TaggedAt: 20}); err != nil {
		t.Fatal(err)
	}
	if err := publishSession(context.Background(), con, sid, nil, store.Verdict{SessionID: sid, Verdict: store.VerdictRoutine, Source: store.VerdictSourceFloor, TaggedAt: 10}, true); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.VerdictFor(con, sid)
	if err != nil || !ok {
		t.Fatalf("verdict read: %v, %v", got, err)
	}
	if got.TaggedAt != 20 || got.Source != store.VerdictSourceAgent {
		t.Fatalf("verdict = %#v, want newer local verdict", got)
	}
}

func TestPublishSessionOlderNoVerdictCannotDelete(t *testing.T) {
	con := newTagTestDB(t)
	sid := "verdict-delete-order"
	if err := store.UpsertVerdict(con, store.Verdict{SessionID: sid, Verdict: store.VerdictRoutine, Source: store.VerdictSourceAgent, TaggedAt: 20}); err != nil {
		t.Fatal(err)
	}
	if err := publishSession(context.Background(), con, sid, []store.TopicSegment{{SessionID: sid, StartUUID: "old", EndUUID: "old", Topic: "old", TaggedAt: 10}}, store.Verdict{}, false); err != nil {
		t.Fatal(err)
	}
	_, ok, err := store.VerdictFor(con, sid)
	if err != nil || !ok {
		t.Fatalf("newer verdict was deleted: %v, %v", ok, err)
	}
}

func TestPublishSessionVerdictTimestampBeatsOrigin(t *testing.T) {
	con := newTagTestDB(t)
	sid := "verdict-cross-origin"
	if err := store.UpsertVerdict(con, store.Verdict{SessionID: sid, Verdict: store.VerdictRoutine, Source: store.VerdictSourceAgent, OriginMachine: "zz", TaggedAt: 10}); err != nil {
		t.Fatal(err)
	}
	newer := store.Verdict{SessionID: sid, Verdict: store.VerdictRoutine, Source: store.VerdictSourceFloor, OriginMachine: "aa", TaggedAt: 20}
	if err := publishSession(context.Background(), con, sid, nil, newer, true); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.VerdictFor(con, sid)
	if err != nil || got.OriginMachine != "aa" || got.TaggedAt != 20 {
		t.Fatalf("verdict = %#v, want newer lower-origin verdict", got)
	}
}

func TestPublishSessionVerdictEqualTimeHigherOriginWins(t *testing.T) {
	con := newTagTestDB(t)
	sid := "verdict-equal-origin"
	if err := store.UpsertVerdict(con, store.Verdict{SessionID: sid, Verdict: store.VerdictRoutine, Source: store.VerdictSourceAgent, OriginMachine: "aa", TaggedAt: 10}); err != nil {
		t.Fatal(err)
	}
	higher := store.Verdict{SessionID: sid, Verdict: store.VerdictRoutine, Source: store.VerdictSourceFloor, OriginMachine: "zz", TaggedAt: 10}
	if err := publishSession(context.Background(), con, sid, nil, higher, true); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.VerdictFor(con, sid)
	if err != nil || got.OriginMachine != "zz" {
		t.Fatalf("verdict = %#v, want equal-time higher-origin verdict", got)
	}
}
