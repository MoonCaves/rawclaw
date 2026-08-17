package antigravity

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
)

func BenchmarkAntigravityDiscover(b *testing.B) {
	tmp := b.TempDir()
	ad := NewRoot(tmp)

	// Seed 50 sessions
	var histLines []string
	for i := 0; i < 50; i++ {
		sessID := fmt.Sprintf("bench-sess-%04d", i)
		histLines = append(histLines, fmt.Sprintf(`{"conversationId":"%s","workspace":"/workspace/bench"}`, sessID))
		tPath := filepath.Join(tmp, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")
		writeJSONL(&testing.T{}, tPath,
			`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>run query benchmark</USER_REQUEST>"}`,
			`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:05Z","content":"Optimizing execution plan"}`,
		)
	}
	writeJSONL(&testing.T{}, filepath.Join(tmp, "history.jsonl"), histLines...)

	b.ResetTimer()
	for b.Loop() {
		cs, err := ad.Discover()
		if err != nil || len(cs) != 50 {
			b.Fatalf("discover failed: %v (got %d)", err, len(cs))
		}
	}
}

func BenchmarkAntigravityNormalize(b *testing.B) {
	record := map[string]any{
		"step_index": float64(1),
		"source":     "MODEL",
		"type":       "PLANNER_RESPONSE",
		"thinking":   "evaluating the index lookup plan across postgres tables",
		"content":    "Here is the optimized query plan with index scan on primary keys.",
		"tool_calls": []any{
			map[string]any{
				"name": "run_command",
				"args": map[string]any{"CommandLine": "explain analyze select * from users;"},
			},
		},
	}

	b.ResetTimer()
	for b.Loop() {
		role, text, ok := normalize(record)
		if !ok || role != "assistant" || !strings.Contains(text, "optimized query plan") {
			b.Fatalf("normalize failed: %s %s %v", role, text, ok)
		}
	}
}

func BenchmarkAntigravityEnsureIndexed_Incremental(b *testing.B) {
	tmp := b.TempDir()
	dbp := filepath.Join(tmp, "bench.db")
	ad := NewRoot(tmp)

	sessID := "bench-incremental"
	writeJSONL(&testing.T{}, filepath.Join(tmp, "history.jsonl"),
		fmt.Sprintf(`{"conversationId":"%s","workspace":"/workspace/bench"}`, sessID),
	)
	tPath := filepath.Join(tmp, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(&testing.T{}, tPath,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>initial build</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:05Z","content":"Build complete"}`,
	)

	cs, _ := ad.Discover()
	// Warm up initial index
	_, _, err := index.EnsureIndexedContainers(dbp, true, cs, ad.Messages, ID, "")
	if err != nil {
		b.Fatalf("initial indexing failed: %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		// Run incremental index over unchanged file
		_, status, err := index.EnsureIndexedContainers(dbp, false, cs, ad.Messages, ID, "")
		if err != nil || status != index.IndexFresh {
			b.Fatalf("incremental indexing failed: %v, status=%v", err, status)
		}
	}
}
