package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/retrieve"
)

// TestFmtScoreExplain checks the compact human block states the regime and the
// REAL inputs honestly across all three ranking regimes.
func TestFmtScoreExplain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		e    retrieve.ScoreExplain
		want []string // substrings that MUST appear
	}{
		{
			name: "bm25-only shows ordinal and single term with tier",
			e: retrieve.ScoreExplain{
				BM25Rank: 0, Coverage: 1, Recency: 0, Final: 0,
				Method: retrieve.MethodBM25, Terms: []string{"kubernetes"},
				Tier: "normal",
			},
			want: []string{"rank 0", retrieve.MethodBM25, "bm25-order=#0", "coverage=1/1 term(s)", "recency-overlay=no", "tier=normal"},
		},
		{
			name: "coverage re-rank shows n/a bm25, real coverage fraction, and routine tier",
			e: retrieve.ScoreExplain{
				BM25Rank: -1, Coverage: 2, Recency: 0, Final: 0,
				Method: retrieve.MethodBM25Coverage, Terms: []string{"kubernetes", "redis"},
				Tier: "routine",
			},
			want: []string{"rank 0", retrieve.MethodBM25Coverage, "bm25-order=n/a", "coverage=2/2 term(s)", "recency-overlay=no", "tier=routine"},
		},
		{
			name: "sort overlay flags recency yes, bm25 n/a, default tier normal",
			e: retrieve.ScoreExplain{
				BM25Rank: -1, Coverage: 1, Recency: 1, Final: 3,
				Method: retrieve.MethodSortOverlay, Terms: []string{"kubernetes", "redis"},
			},
			want: []string{"rank 3", retrieve.MethodSortOverlay, "bm25-order=n/a", "coverage=1/2 term(s)", "recency-overlay=yes", "tier=normal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FmtScoreExplain(tt.e)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("FmtScoreExplain() = %q, missing %q", got, w)
				}
			}
		})
	}
}

// TestPrintDebugSearch checks the human explainer renders a header + breakdown
// per hit and that a short explains slice degrades gracefully.
func TestPrintDebugSearch(t *testing.T) {
	t.Parallel()

	hits := []retrieve.Hit{
		// Full parseable ISO: pins the timefmt-seam normalization to marked UTC.
		{SessionID: "alphasession", ISO: "2026-06-01T10:00:00.123Z", Role: "user", Snippet: "kube"},
		{SessionID: "betasession", ISO: "", Role: "user", Snippet: "redis", Routine: true},
		{SessionID: "gammasession", ISO: "2026-06-01T11:00:00Z", Role: "assistant", IsSubagent: true, Parent: "alphasession", Snippet: "child task"},
		{SessionID: "deltasession", ISO: "2026-06-01T12:00:00Z", Role: "assistant", IsSubagent: true, Parent: "betasession", Snippet: "sub routine", Routine: true},
	}
	explains := []retrieve.ScoreExplain{
		{BM25Rank: 0, Coverage: 1, Recency: 0, Final: 0, Method: retrieve.MethodBM25, Terms: []string{"kube"}, Tier: "normal"},
		{BM25Rank: 1, Coverage: 1, Recency: 0, Final: 1, Method: retrieve.MethodBM25, Terms: []string{"redis"}, Tier: "routine"},
		{BM25Rank: 2, Coverage: 1, Recency: 0, Final: 2, Method: retrieve.MethodBM25, Terms: []string{"child"}, Tier: "normal"},
		// deliberately omit the fourth explain to exercise the short-slice path
	}

	var buf bytes.Buffer
	PrintDebugSearch(&buf, hits, explains)
	out := buf.String()

	for _, w := range []string{
		"4 hit(s)", "scoring explainer",
		"alphases",             // sid8 truncation of "alphasession"
		"2026-06-01T10:00:00Z", // stored ISO normalized to marked UTC (timefmt seam)
		retrieve.MethodBM25,
		"tier=normal",
		"betasess · routine",                     // second hit has routine marker
		"gammases · subagent⟵alphases",           // third hit has subagent attribution
		"deltases · subagent⟵betasess · routine", // fourth hit has subagent and routine marker
		"?",              // empty ISO rendered as "?"
		"(no breakdown)", // fourth hit has no matching explain
	} {
		if !strings.Contains(out, w) {
			t.Errorf("PrintDebugSearch output missing %q in:\n%s", w, out)
		}
	}
}

// TestPrintDebugSearchEmpty checks the zero-hit path prints the explainer hint.
func TestPrintDebugSearchEmpty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	PrintDebugSearch(&buf, nil, nil)
	if !strings.Contains(buf.String(), "No matches to explain") {
		t.Errorf("empty PrintDebugSearch = %q, want 'No matches to explain'", buf.String())
	}
}

// TestDebugSearchJSON checks the JSON projection carries hit fields + the nested
// score, and round-trips to the real numbers.
func TestDebugSearchJSON(t *testing.T) {
	t.Parallel()

	hits := []retrieve.Hit{
		{SessionID: "s1", ISO: "2026-06-01", Role: "user", Snippet: "kube redis", Routine: true},
	}
	explains := []retrieve.ScoreExplain{
		{BM25Rank: -1, Coverage: 2, Recency: 0, Final: 0, Method: retrieve.MethodBM25Coverage, Terms: []string{"kube", "redis"}, Tier: "routine"},
	}

	b, err := DebugSearchJSON(hits, explains)
	if err != nil {
		t.Fatalf("DebugSearchJSON: %v", err)
	}

	var got []struct {
		SessionID string                `json:"session_id"`
		ISO       string                `json:"iso"`
		Role      string                `json:"role"`
		Snippet   string                `json:"snippet"`
		Score     retrieve.ScoreExplain `json:"score"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v\njson: %s", err, b)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 row, got %d", len(got))
	}
	if got[0].SessionID != "s1" || got[0].Snippet != "kube redis" {
		t.Errorf("hit fields wrong: %+v", got[0])
	}
	sc := got[0].Score
	if sc.Method != retrieve.MethodBM25Coverage || sc.Coverage != 2 || sc.BM25Rank != -1 || sc.Final != 0 || sc.Tier != "routine" {
		t.Errorf("score round-trip wrong: %+v", sc)
	}
	// Confirm the json keys are the snake_case tags the cli/agents expect.
	for _, key := range []string{`"session_id"`, `"score"`, `"bm25_rank"`, `"coverage"`, `"recency"`, `"final"`, `"method"`, `"terms"`, `"tier"`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("json missing key %s in:\n%s", key, b)
		}
	}
}

// TestDebugSearchJSONShortExplains checks a hit with no matching explain emits a
// zero-value score (empty Method) rather than panicking.
func TestDebugSearchJSONShortExplains(t *testing.T) {
	t.Parallel()

	hits := []retrieve.Hit{{SessionID: "s1"}, {SessionID: "s2"}}
	b, err := DebugSearchJSON(hits, nil)
	if err != nil {
		t.Fatalf("DebugSearchJSON: %v", err)
	}
	if !strings.Contains(string(b), `"s2"`) {
		t.Errorf("second hit dropped from json:\n%s", b)
	}
}
