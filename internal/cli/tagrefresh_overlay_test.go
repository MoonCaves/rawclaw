package cli

import (
	"reflect"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestTagRefreshOverlaysAuthoritativeTags(t *testing.T) {
	consolidated := []store.TopicSegment{
		{SessionID: "session", StartUUID: "one", Topic: "old one"},
		{SessionID: "session", StartUUID: "history", Topic: "consolidated history"},
	}
	authoritative := []store.TopicSegment{
		{SessionID: "session", StartUUID: "one", Topic: "fresh one"},
		{SessionID: "session", StartUUID: "two", Topic: "fresh two"},
	}

	got := overlayAuthoritativeTopics(consolidated, authoritative)
	want := []store.TopicSegment{
		{SessionID: "session", StartUUID: "one", Topic: "fresh one"},
		{SessionID: "session", StartUUID: "history", Topic: "consolidated history"},
		{SessionID: "session", StartUUID: "two", Topic: "fresh two"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("overlay = %#v, want %#v", got, want)
	}
}
