package retrieve

import (
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestRRFContracts(t *testing.T) {
	lastHit := store.SearchHit{ID: 30, Rank: 300, SessionID: "last", Content: "final payload", Role: "assistant", ISO: "2026-11-04", IsSubagent: true, Parent: "parent", Snippet: "final snippet", BM25: -9}
	hits := []store.SearchHit{
		{ID: 30, Rank: 300, SessionID: "first", Content: "first payload"},
		{ID: 20, Rank: 1, SessionID: "twenty"},
		{ID: 30, Rank: 300, SessionID: "middle", Content: "same-list replacement"},
		{ID: 10, Rank: 1, SessionID: "ten"},
	}
	lastAnchor := store.SearchAnchor{ID: 70, Rank: 700, SessionID: "final", UUID: "last-uuid", Content: "final anchor", Role: "assistant", ISO: "2026-11-03", Parent: "root", Snippet: "anchor snippet", OnlyCopySince: 42, Project: "final-project", BM25: -7}
	anchors := []store.SearchAnchor{
		{ID: 70, Rank: 700, UUID: "first-uuid"},
		{ID: 8, Rank: 2, SessionID: "eight"},
		{ID: 4, Rank: 2, SessionID: "four"},
	}
	for _, limit := range []int{1, 0, -1} {
		gotHits, hitScores := rrfHits(60, limit, hits, []store.SearchHit{lastHit})
		gotAnchors, anchorScores := rrfAnchors(60, limit, anchors, []store.SearchAnchor{{ID: 70, Rank: 700, UUID: "middle-uuid"}, lastAnchor})
		hitIDs := make([]int, 0, len(gotHits))
		for _, row := range gotHits {
			hitIDs = append(hitIDs, row.ID)
		}
		anchorIDs := make([]int, 0, len(gotAnchors))
		for _, row := range gotAnchors {
			anchorIDs = append(anchorIDs, row.ID)
		}
		for _, tc := range []struct {
			name               string
			ids, wantIDs       []int
			scores, wantScores map[int]float64
			last, wantLast     any
		}{
			{"hits", hitIDs, []int{10, 20, 30}, hitScores, map[int]float64{10: 1.0 / 61, 20: 1.0 / 61, 30: 1.0 / 120}, gotHits[len(gotHits)-1], lastHit},
			{"anchors", anchorIDs, []int{4, 8, 70}, anchorScores, map[int]float64{4: 1.0 / 62, 8: 1.0 / 62, 70: 3.0 / 760}, gotAnchors[len(gotAnchors)-1], lastAnchor},
		} {
			wantIDs := tc.wantIDs
			if limit > 0 {
				wantIDs = wantIDs[:limit]
			}
			if !slices.Equal(tc.ids, wantIDs) {
				t.Errorf("%s limit %d: IDs = %v, want %v (stored ranks, then ID ties)", tc.name, limit, tc.ids, wantIDs)
			}
			if limit <= 0 && !reflect.DeepEqual(tc.last, tc.wantLast) {
				t.Errorf("%s: repeated ID kept %#v, want final row %#v", tc.name, tc.last, tc.wantLast)
			}
			// The score map keeps every ID and every repeated contribution, even when rows are trimmed.
			if len(tc.scores) != len(tc.wantScores) {
				t.Errorf("%s limit %d: score count = %d, want %d", tc.name, limit, len(tc.scores), len(tc.wantScores))
			}
			for id, want := range tc.wantScores {
				if got, ok := tc.scores[id]; !ok || math.IsNaN(got) || math.Abs(got-want) > 1e-15 {
					t.Errorf("%s limit %d: score[%d] = %v, present=%v, want %v", tc.name, limit, id, got, ok, want)
				}
			}
		}
	}
}

func TestRRFEmptyContracts(t *testing.T) {
	hits, hitScores := rrfHits(60, 1)
	anchors, anchorScores := rrfAnchors(60, 1)
	emptyHits, emptyHitScores := rrfHits(60, 1, nil, []store.SearchHit{})
	emptyAnchors, emptyAnchorScores := rrfAnchors(60, 1, nil, []store.SearchAnchor{})
	for _, tc := range []struct {
		name      string
		emptyRows bool
		scores    map[int]float64
	}{
		{"hits without lists", hits != nil && len(hits) == 0, hitScores},
		{"anchors without lists", anchors != nil && len(anchors) == 0, anchorScores},
		{"hits with empty lists", emptyHits != nil && len(emptyHits) == 0, emptyHitScores},
		{"anchors with empty lists", emptyAnchors != nil && len(emptyAnchors) == 0, emptyAnchorScores},
	} {
		if !tc.emptyRows || tc.scores == nil || len(tc.scores) != 0 {
			t.Errorf("%s: empty rows=%v, scores=%#v; want nonnil empty slice and map", tc.name, tc.emptyRows, tc.scores)
		}
	}
}
