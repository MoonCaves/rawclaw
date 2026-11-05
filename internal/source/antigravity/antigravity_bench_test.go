package antigravity_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source/antigravity"
)

// Lifted from golang/go src/testing/testing.go L912-L942 @3901409b5d0fb7c85a3e6730a59943cc93b2835c (BSD-3-Clause).
// rawclaw: use the native shared T/B API for fixture failure ownership.
func writeJSONL(t testing.TB, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkAntigravityDiscover(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	tmp := b.TempDir()
	ad := antigravity.NewRoot(tmp)

	// Seed 50 sessions
	var histLines []string
	for i := 0; i < 50; i++ {
		sessID := fmt.Sprintf("bench-sess-%04d", i)
		histLines = append(histLines, fmt.Sprintf(`{"conversationId":"%s","workspace":"/workspace/bench"}`, sessID))
		tPath := filepath.Join(tmp, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")
		writeJSONL(b, tPath,
			`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>run query benchmark</USER_REQUEST>"}`,
			`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:05Z","content":"Optimizing execution plan"}`,
		)
	}
	writeJSONL(b, filepath.Join(tmp, "history.jsonl"), histLines...)

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
		role, text, ok := antigravity.NormalizeRecord(record)
		if !ok || role != "assistant" || !strings.Contains(text, "optimized query plan") {
			b.Fatalf("normalize failed: %s %s %v", role, text, ok)
		}
	}
}

func BenchmarkAntigravityEnsureIndexed_Incremental(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	tmp := b.TempDir()
	dbp := filepath.Join(tmp, "bench.db")
	ad := antigravity.NewRoot(tmp)

	sessID := "bench-incremental"
	writeJSONL(b, filepath.Join(tmp, "history.jsonl"),
		fmt.Sprintf(`{"conversationId":"%s","workspace":"/workspace/bench"}`, sessID),
	)
	tPath := filepath.Join(tmp, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")
	writeJSONL(b, tPath,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>initial build</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:05Z","content":"Build complete"}`,
	)

	// Lifted from MoonCaves/rawclaw internal/source/goose/goose_test.go L357-L361 @024a68702cc9c72ef945350cd31e0d3cd0d5bf91 (MIT).
	// rawclaw: substitute this adapter, cs, b and the benchmark diagnostic.
	cs, err := ad.Discover()
	if err != nil || len(cs) != 1 {
		b.Fatalf("discover failed: %v (got %d, want 1)", err, len(cs))
	}
	// Warm up initial index
	// Lifted from MoonCaves/rawclaw internal/index/containers_test.go L362-L368 @024a68702cc9c72ef945350cd31e0d3cd0d5bf91 (MIT).
	// rawclaw: substitute the existing fixture arguments, nSessions and b.
	nSessions, _, err := index.EnsureIndexedContainers(dbp, true, cs, ad.Messages, antigravity.ID, "")
	if err != nil {
		b.Fatalf("initial EnsureIndexedContainers: %v", err)
	}
	if nSessions != 1 {
		b.Fatalf("sessions = %d, want 1", nSessions)
	}

	b.ResetTimer()
	for b.Loop() {
		// Run incremental index over unchanged file
		_, status, err := index.EnsureIndexedContainers(dbp, false, cs, ad.Messages, antigravity.ID, "")
		if err != nil || status != index.IndexFresh {
			b.Fatalf("incremental indexing failed: %v, status=%v", err, status)
		}
	}
}
