package retrieve

import (
	"reflect"
	"testing"
)

// TestExplain checks the honest breakdown maps each ranking regime to the exact
// fields the REAL ranking uses — no invented blend. It drives Explain directly
// (the pure layer) with the cov values the ranker would have produced.
func TestExplain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		covs []int
		in   ExplainInputs
		want []ScoreExplain
	}{
		{
			name: "single-term relevance is bm25 only; coverage always 1",
			covs: []int{1, 1},
			in:   ExplainInputs{Terms: []string{"kubernetes"}, Multi: false, Sort: ""},
			want: []ScoreExplain{
				{BM25Rank: 0, Coverage: 1, Recency: 0, Final: 0, Method: MethodBM25, Terms: []string{"kubernetes"}},
				{BM25Rank: 1, Coverage: 1, Recency: 0, Final: 1, Method: MethodBM25, Terms: []string{"kubernetes"}},
			},
		},
		{
			name: "multi-term relevance is bm25+coverage; bm25 ordinal not recoverable post-resort",
			covs: []int{2, 1},
			in:   ExplainInputs{Terms: []string{"kubernetes", "redis"}, Multi: true, Sort: ""},
			want: []ScoreExplain{
				{BM25Rank: -1, Coverage: 2, Recency: 0, Final: 0, Method: MethodBM25Coverage, Terms: []string{"kubernetes", "redis"}},
				{BM25Rank: -1, Coverage: 1, Recency: 0, Final: 1, Method: MethodBM25Coverage, Terms: []string{"kubernetes", "redis"}},
			},
		},
		{
			name: "sort overlay replaces relevance: bm25 n/a, recency flag set",
			covs: []int{2, 1},
			in:   ExplainInputs{Terms: []string{"kubernetes", "redis"}, Multi: true, Sort: "newest"},
			want: []ScoreExplain{
				{BM25Rank: -1, Coverage: 2, Recency: 1, Final: 0, Method: MethodSortOverlay, Terms: []string{"kubernetes", "redis"}},
				{BM25Rank: -1, Coverage: 1, Recency: 1, Final: 1, Method: MethodSortOverlay, Terms: []string{"kubernetes", "redis"}},
			},
		},
		{
			name: "no hits yields no breakdowns",
			covs: []int{},
			in:   ExplainInputs{Terms: []string{"x"}, Multi: false, Sort: ""},
			want: []ScoreExplain{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Explain(tt.covs, tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Explain() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestExplainDefensiveTermsCopy verifies Explain never aliases the caller's
// Terms slice into the breakdowns (mutating the input must not corrupt output).
func TestExplainDefensiveTermsCopy(t *testing.T) {
	t.Parallel()

	in := ExplainInputs{Terms: []string{"alpha", "beta"}, Multi: true, Sort: ""}
	got := Explain([]int{2}, in)
	if len(got) != 1 {
		t.Fatalf("want 1 breakdown, got %d", len(got))
	}
	// Mutate the caller's slice AFTER the call.
	in.Terms[0] = "MUTATED"
	if got[0].Terms[0] != "alpha" {
		t.Errorf("Explain aliased caller Terms: got[0].Terms[0]=%q, want %q", got[0].Terms[0], "alpha")
	}
}

// TestSearchExplained asserts the breakdown numbers match the REAL ranking on a
// live FTS5 fixture: coverage equals the distinct query terms each hit matched,
// the order is byte-identical to Search, and the regime is correct end-to-end.
func TestSearchExplained(t *testing.T) {
	t.Parallel()

	sessions := []testSession{
		{id: "alpha", msgCount: 5, lastTS: 100},
		{id: "beta", msgCount: 5, lastTS: 200},
	}

	tests := []struct {
		name       string
		msgs       []testMsg
		query      string
		params     SearchParams
		wantSIDs   []string
		wantMethod string
		wantCov    []int
		wantBM25   []int // expected BM25Rank per result
		wantRec    []float64
	}{
		{
			// Symmetric content -> bm25 ties -> m.id tiebreak gives insertion
			// order (alpha then beta), the regime stays bm25-only.
			name: "single-term bm25-only regime",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "the kubernetes note here"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "the kubernetes note here"},
			},
			query:      "kubernetes",
			wantSIDs:   []string{"alpha", "beta"},
			wantMethod: MethodBM25,
			wantCov:    []int{1, 1},
			wantBM25:   []int{0, 1},
			wantRec:    []float64{0, 0},
		},
		{
			name: "multi-term coverage re-rank floats higher coverage up",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "only kubernetes here"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "kubernetes and redis together"},
			},
			query:      "kubernetes redis",
			wantSIDs:   []string{"beta", "alpha"}, // beta covers both terms
			wantMethod: MethodBM25Coverage,
			wantCov:    []int{2, 1},
			wantBM25:   []int{-1, -1}, // not recoverable after coverage re-sort
			wantRec:    []float64{0, 0},
		},
		{
			name: "sort overlay replaces relevance",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "only kubernetes here"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "kubernetes and redis together"},
			},
			query:      "kubernetes redis",
			params:     SearchParams{Sort: "newest"},
			wantSIDs:   []string{"beta", "alpha"}, // newest (ts=2) first
			wantMethod: MethodSortOverlay,
			wantCov:    []int{2, 1},
			wantBM25:   []int{-1, -1},
			wantRec:    []float64{1, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, dbp := newTestDB(t, sessions, tt.msgs)
			hits, ex := SearchExplained(dbp, tt.query, 10, tt.params)

			if gotSIDs := sids(hits); !reflect.DeepEqual(gotSIDs, tt.wantSIDs) {
				t.Fatalf("SearchExplained order = %v, want %v", gotSIDs, tt.wantSIDs)
			}
			if len(ex) != len(hits) {
				t.Fatalf("explains len %d != hits len %d (must be parallel)", len(ex), len(hits))
			}
			for i := range ex {
				if ex[i].Method != tt.wantMethod {
					t.Errorf("hit %d Method = %q, want %q", i, ex[i].Method, tt.wantMethod)
				}
				if ex[i].Coverage != tt.wantCov[i] {
					t.Errorf("hit %d Coverage = %d, want %d", i, ex[i].Coverage, tt.wantCov[i])
				}
				if ex[i].BM25Rank != tt.wantBM25[i] {
					t.Errorf("hit %d BM25Rank = %d, want %d", i, ex[i].BM25Rank, tt.wantBM25[i])
				}
				if ex[i].Recency != tt.wantRec[i] {
					t.Errorf("hit %d Recency = %v, want %v", i, ex[i].Recency, tt.wantRec[i])
				}
				if ex[i].Final != i {
					t.Errorf("hit %d Final = %d, want %d (ordinal position)", i, ex[i].Final, i)
				}
			}
		})
	}
}

// TestSearchExplainedNoMatch confirms an empty result yields parallel empty
// slices, not a nil/short mismatch.
func TestSearchExplainedNoMatch(t *testing.T) {
	t.Parallel()

	_, dbp := newTestDB(t,
		[]testSession{{id: "alpha", msgCount: 5, lastTS: 100}},
		[]testMsg{{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "nothing relevant"}},
	)
	hits, ex := SearchExplained(dbp, "absentterm", 10, SearchParams{})
	if len(hits) != 0 || len(ex) != 0 {
		t.Errorf("no-match SearchExplained = %d hits / %d explains, want 0/0", len(hits), len(ex))
	}
}
