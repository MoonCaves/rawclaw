package archive

import "testing"

func seg(start, topic string) TagSegment { return TagSegment{StartUUID: start, Topic: topic} }

func TestResolveSegments(t *testing.T) {
	tests := []struct {
		name         string
		files        []TagFile
		wantTopics   []string // start-uuids of the winning set, in stored order; nil = no winner
		wantOrigin   string
		wantConflict bool
	}{
		{
			name:       "single real file wins (origin-authority falls out)",
			files:      []TagFile{{OriginMachine: "box-a", Segments: []TagSegment{seg("u1", "auth"), seg("u2", "db")}}},
			wantTopics: []string{"u1", "u2"},
			wantOrigin: "box-a",
		},
		{
			name: "identical sets (any row order) from two machines are not a conflict",
			files: []TagFile{
				{OriginMachine: "box-a", Segments: []TagSegment{seg("u2", "db"), seg("u1", "auth")}}, // shuffled
				{OriginMachine: "box-b", Segments: []TagSegment{seg("u1", "auth"), seg("u2", "db")}}, // canonical
			},
			// Same content (hash is order-independent) → no conflict; attribution is
			// deterministic (box-b > box-a), so the winner's row order is returned.
			wantTopics:   []string{"u1", "u2"},
			wantOrigin:   "box-b",
			wantConflict: false,
		},
		{
			name: "real beats routine/empty regardless of machine",
			files: []TagFile{
				{OriginMachine: "box-a", Segments: []TagSegment{seg("u9", "")}}, // empty topic = not real
				{OriginMachine: "box-b", Segments: []TagSegment{seg("u1", "real work")}},
			},
			wantTopics: []string{"u1"},
			wantOrigin: "box-b",
		},
		{
			name: "two DISTINCT real sets = conflict, deterministic winner (highest origin)",
			files: []TagFile{
				{OriginMachine: "aaa", Segments: []TagSegment{seg("u1", "thin")}},
				{OriginMachine: "zzz", Segments: []TagSegment{seg("u1", "rich"), seg("u2", "more")}},
			},
			wantTopics:   []string{"u1", "u2"}, // zzz > aaa
			wantOrigin:   "zzz",
			wantConflict: true,
		},
		{
			name:       "no real segments anywhere -> nil, no conflict",
			files:      []TagFile{{OriginMachine: "box-a", Verdict: &TagVerdict{Verdict: "routine", Source: "floor"}}},
			wantTopics: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segs, origin, conflict := resolveSegments(tt.files)
			if conflict != tt.wantConflict {
				t.Errorf("conflict = %v, want %v", conflict, tt.wantConflict)
			}
			if tt.wantOrigin != "" && origin != tt.wantOrigin {
				t.Errorf("origin = %q, want %q", origin, tt.wantOrigin)
			}
			var gotStarts []string
			for _, s := range segs {
				gotStarts = append(gotStarts, s.StartUUID)
			}
			if len(gotStarts) != len(tt.wantTopics) {
				t.Fatalf("winning set starts = %v, want %v", gotStarts, tt.wantTopics)
			}
			for i := range gotStarts {
				if gotStarts[i] != tt.wantTopics[i] {
					t.Errorf("start[%d] = %q, want %q", i, gotStarts[i], tt.wantTopics[i])
				}
			}
		})
	}
}

func TestResolveSegments_IdenticalSetAttributionIsDeterministic(t *testing.T) {
	// Same content on two machines, different origin — NOT a conflict, but the
	// attribution must not depend on gatherTagFiles ordering (else origin_machine
	// diverges across boxes). Highest origin wins deterministically, both orders.
	a := TagFile{OriginMachine: "box-a", Segments: []TagSegment{seg("u1", "same")}}
	z := TagFile{OriginMachine: "box-z", Segments: []TagSegment{seg("u1", "same")}}
	for _, order := range [][]TagFile{{a, z}, {z, a}} {
		_, origin, conflict := resolveSegments(order)
		if conflict {
			t.Error("identical content is not a conflict")
		}
		if origin != "box-z" {
			t.Errorf("attribution = %q, want box-z (deterministic, order-independent)", origin)
		}
	}
}

func TestResolveSegments_ConflictWinnerIsDeterministic(t *testing.T) {
	// Same inputs in any order must yield the same winner (convergence).
	a := TagFile{OriginMachine: "box-a", Segments: []TagSegment{seg("u1", "A")}}
	b := TagFile{OriginMachine: "box-b", Segments: []TagSegment{seg("u1", "B")}}
	_, o1, c1 := resolveSegments([]TagFile{a, b})
	_, o2, c2 := resolveSegments([]TagFile{b, a})
	if !c1 || !c2 {
		t.Fatal("both orders should report a conflict")
	}
	if o1 != o2 || o1 != "box-b" {
		t.Errorf("winner not order-independent: %q vs %q (want box-b)", o1, o2)
	}
}

func TestResolveVerdict_LWW(t *testing.T) {
	files := []TagFile{
		{OriginMachine: "box-a", Verdict: &TagVerdict{Verdict: "routine", Source: "floor", TaggedAt: 10}},
		{OriginMachine: "box-b", Verdict: &TagVerdict{Verdict: "routine", Source: "agent", TaggedAt: 20}},
		{OriginMachine: "aaa", Verdict: nil},
	}
	v, origin := resolveVerdict(files)
	if v == nil || v.TaggedAt != 20 || origin != "box-b" {
		t.Errorf("verdict = %+v origin=%q, want tagged_at 20 / box-b", v, origin)
	}

	// tie on tagged_at -> higher origin wins
	tie := []TagFile{
		{OriginMachine: "aaa", Verdict: &TagVerdict{Verdict: "routine", Source: "floor", TaggedAt: 7}},
		{OriginMachine: "zzz", Verdict: &TagVerdict{Verdict: "routine", Source: "floor", TaggedAt: 7}},
	}
	if _, origin := resolveVerdict(tie); origin != "zzz" {
		t.Errorf("tie origin = %q, want zzz", origin)
	}

	// no verdicts anywhere
	if v, _ := resolveVerdict([]TagFile{{OriginMachine: "box-a"}}); v != nil {
		t.Errorf("no-verdict case returned %+v, want nil", v)
	}
}
