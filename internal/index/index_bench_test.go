package index

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func writeBenchJSONL(b testing.TB, path string, lines ...string) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatal(err)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
}

func seedBenchmarkCorpus(b testing.TB, tdir string, sessionCount int) {
	b.Helper()
	topics := []string{
		"optimization for postgres query planning with indexing strategies",
		"refactoring kubernetes deployment manifests for high availability",
		"debugging network timeout in grpc connection pool handler",
		"implementing transactional rollback with sqlite fts5 search triggers",
		"analyzing memory allocation benchmarks and garbage collection overhead",
	}

	for i := 0; i < sessionCount; i++ {
		sid := fmt.Sprintf("bench-sess-%04d", i)
		f := filepath.Join(tdir, sid+".jsonl")
		topic := topics[i%len(topics)]
		lines := []string{
			fmt.Sprintf(`{"type":"user","timestamp":"2026-08-01T10:00:00Z","message":{"role":"user","content":"Please help with %s in module %d"}}`, topic, i),
			fmt.Sprintf(`{"type":"assistant","timestamp":"2026-08-01T10:00:05Z","message":{"role":"assistant","content":"Analyzing %s. Here are the recommendations for tuning performance and index utilization."}}`, topic),
			`{"type":"user","timestamp":"2026-08-01T10:01:00Z","message":{"role":"user","content":"How do we measure the impact on search latency and throughput?"}}`,
			`{"type":"assistant","timestamp":"2026-08-01T10:01:10Z","message":{"role":"assistant","content":"We can run hermetic benchmarks comparing cold vs warm index queries with FTS5 bm25 ranking."}}`,
			`{"type":"user","timestamp":"2026-08-01T10:02:00Z","message":{"role":"user","content":"Deploy the revised configuration to staging."}}`,
			`{"type":"assistant","timestamp":"2026-08-01T10:02:15Z","message":{"role":"assistant","content":"Deployment complete. Verified 100% tests pass and zero regressions detected."}}`,
		}
		writeBenchJSONL(b, f, lines...)
	}
}

// BenchmarkFTS5Search benchmarks FTS5 keyword queries against warm (reused connection)
// and cold (new connection per query) SQLite indexes.
func BenchmarkFTS5Search(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	tmp := b.TempDir()
	tdir := filepath.Join(tmp, "transcripts")
	dbp := filepath.Join(tmp, "bench.db")

	seedBenchmarkCorpus(b, tdir, 100)

	con, err := store.ConnectRW(dbp)
	if err != nil {
		b.Fatalf("store.ConnectRW: %v", err)
	}
	if err := EnsureSchema(con, sourceClaude); err != nil {
		con.Close()
		b.Fatalf("EnsureSchema: %v", err)
	}
	if err := UpdateIndex(con, tdir); err != nil {
		con.Close()
		b.Fatalf("UpdateIndex: %v", err)
	}
	con.Close()

	b.Run("Warm", func(b *testing.B) {
		roCon, err := store.ConnectRO(dbp)
		if err != nil {
			b.Fatalf("store.ConnectRO: %v", err)
		}
		defer roCon.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			hits, err := store.SearchHits(roCon, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil || len(hits) == 0 {
				b.Fatalf("SearchHits failed: %v (hits=%d)", err, len(hits))
			}
		}
	})

	b.Run("Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			roCon, err := store.ConnectRO(dbp)
			if err != nil {
				b.Fatalf("store.ConnectRO: %v", err)
			}
			hits, err := store.SearchHits(roCon, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil || len(hits) == 0 {
				roCon.Close()
				b.Fatalf("SearchHits failed: %v (hits=%d)", err, len(hits))
			}
			roCon.Close()
		}
	})
}

// BenchmarkUnchangedFastPath benchmarks incremental indexing over a directory where
// all transcript files are unchanged on disk, exercising the file_index watermark check.
func BenchmarkUnchangedFastPath(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	tmp := b.TempDir()
	tdir := filepath.Join(tmp, "transcripts")
	dbp := filepath.Join(tmp, "bench.db")

	seedBenchmarkCorpus(b, tdir, 50)

	con, err := store.ConnectRW(dbp)
	if err != nil {
		b.Fatalf("store.ConnectRW: %v", err)
	}
	defer con.Close()

	if err := EnsureSchema(con, sourceClaude); err != nil {
		b.Fatalf("EnsureSchema: %v", err)
	}
	if err := UpdateIndex(con, tdir); err != nil {
		b.Fatalf("initial UpdateIndex: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		if err := UpdateIndex(con, tdir); err != nil {
			b.Fatalf("fast-path UpdateIndex failed: %v", err)
		}
	}
}

// BenchmarkChangedSessionReplacement benchmarks transactional replacement of a modified session's
// rows in SQLite (messages delete/insert, sessions row replace, watermark update, vault write).
func BenchmarkChangedSessionReplacement(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	tmp := b.TempDir()
	tdir := filepath.Join(tmp, "transcripts")
	dbp := filepath.Join(tmp, "bench.db")

	seedBenchmarkCorpus(b, tdir, 20)

	con, err := store.ConnectRW(dbp)
	if err != nil {
		b.Fatalf("store.ConnectRW: %v", err)
	}
	defer con.Close()

	if err := EnsureSchema(con, sourceClaude); err != nil {
		b.Fatalf("EnsureSchema: %v", err)
	}
	if err := UpdateIndex(con, tdir); err != nil {
		b.Fatalf("initial UpdateIndex: %v", err)
	}

	targetFile := filepath.Join(tdir, "bench-sess-0000.jsonl")
	iter := 0

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		iter++
		lines := []string{
			fmt.Sprintf(`{"type":"user","timestamp":"2026-08-01T10:00:00Z","message":{"role":"user","content":"Updated query optimization request iteration %d"}}`, iter),
			fmt.Sprintf(`{"type":"assistant","timestamp":"2026-08-01T10:00:05Z","message":{"role":"assistant","content":"Execution plan updated for iteration %d with index scans."}}`, iter),
		}
		writeBenchJSONL(b, targetFile, lines...)

		if !ReindexFile(con, targetFile, tdir) {
			b.Fatalf("ReindexFile failed on iteration %d", iter)
		}
	}
}

// BenchmarkIncrementalAppend measures tail ingestion after a 10 MB JSONL
// transcript has already been indexed. BenchmarkChangedSessionReplacement is
// the corresponding full-file replacement baseline.
func BenchmarkIncrementalAppend(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	tmp := b.TempDir()
	f := filepath.Join(tmp, "large.jsonl")
	payload := strings.Repeat("payload ", 120)
	lines := make([]string, 10000)
	for i := range lines {
		lines[i] = fmt.Sprintf(`{"type":"user","message":{"role":"user","content":%q},"uuid":"seed-%d","timestamp":"2026-08-01T10:00:00Z"}`, payload, i)
	}
	writeBenchJSONL(b, f, lines...)
	c := source.Container{ID: "bench-large", Path: f, CWD: "/bench"}
	dbp := filepath.Join(tmp, "incremental.db")
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, claudeTailMsgsFn(), sourceClaude, ""); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		appendFile(b, f, fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":"append %d"},"uuid":"append-%d","timestamp":"2026-08-01T11:00:00Z"}`+"\n", i, i))
		if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, claudeTailMsgsFn(), sourceClaude, ""); err != nil {
			b.Fatal(err)
		}
	}
}
