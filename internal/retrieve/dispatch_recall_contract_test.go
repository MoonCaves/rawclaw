package retrieve

import (
	"slices"
	"testing"
)

func TestRecallDispatchContracts(t *testing.T) {
	modes := []struct {
		name   string
		params SearchParams
	}{
		{name: "default relevance"},
		{name: "exact", params: SearchParams{Exact: true}},
		{name: "newest", params: SearchParams{Sort: "newest"}},
		{name: "oldest", params: SearchParams{Sort: "oldest"}},
	}
	fixtures := []struct {
		name string
		msgs []testMsg
		want map[string][]string
	}{
		{
			name: "AND hit excludes OR-only distractors",
			msgs: []testMsg{
				{sessionID: "all", role: "user", ts: 1, content: "cobalt and nexus"},
				{sessionID: "cobalt", role: "user", ts: 2, content: "cobalt only"},
				{sessionID: "nexus", role: "user", ts: 3, content: "nexus only"},
			},
			want: map[string][]string{
				"default relevance": {"all"},
				"exact":             {"all"},
				"newest":            {"all"},
				"oldest":            {"all"},
			},
		},
		{
			name: "AND miss retries OR only in relevance and exact modes",
			msgs: []testMsg{
				{sessionID: "cobalt", role: "user", ts: 1, content: "cobalt only"},
				{sessionID: "nexus", role: "user", ts: 2, content: "nexus only"},
			},
			want: map[string][]string{
				"default relevance": {"cobalt", "nexus"},
				"exact":             {"cobalt", "nexus"},
				"newest":            {},
				"oldest":            {},
			},
		},
	}

	// Neither fixture contains the literal phrase "cobalt nexus", so substring fallback cannot satisfy these expectations.
	for _, fixture := range fixtures {
		for _, mode := range modes {
			t.Run(fixture.name+"/"+mode.name, func(t *testing.T) {
				con, dbp := newTestDB(t, []testSession{
					{id: "all", msgCount: 1}, {id: "cobalt", msgCount: 1}, {id: "nexus", msgCount: 1},
				}, fixture.msgs)
				defer con.Close()

				want := append([]string(nil), fixture.want[mode.name]...)
				hitIDs := sids(Search(dbp, "cobalt nexus", 10, mode.params))
				anchors := MatchAnchors(con, "cobalt nexus", 10, mode.params)
				anchorIDs := make([]string, len(anchors))
				for i, anchor := range anchors {
					anchorIDs[i] = anchor.SessionID
				}
				slices.Sort(want)
				slices.Sort(hitIDs)
				slices.Sort(anchorIDs)
				if !slices.Equal(hitIDs, want) {
					t.Errorf("Search IDs = %v, want %v", hitIDs, want)
				}
				if !slices.Equal(anchorIDs, want) {
					t.Errorf("MatchAnchors IDs = %v, want %v", anchorIDs, want)
				}
			})
		}
	}
}
