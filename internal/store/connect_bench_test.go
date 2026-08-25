package store_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

func seedRealisticBenchStore(b testing.TB, sessionCount int) string {
	b.Helper()
	dbp := filepath.Join(b.TempDir(), "realistic_bench.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		b.Fatalf("store.ConnectRW: %v", err)
	}
	defer con.Close()

	if err := store.Rebuild(con); err != nil {
		b.Fatalf("store.Rebuild: %v", err)
	}

	topics := []string{
		"optimization for postgres query planning with indexing strategies and vacuum analyze",
		"refactoring kubernetes deployment manifests for high availability and rolling updates",
		"debugging network timeout in grpc connection pool handler and transport layer",
		"implementing transactional rollback with sqlite fts5 search triggers and trigram indices",
		"analyzing memory allocation benchmarks and garbage collection overhead in go runtime",
		"configuring prometheus metrics collection and grafana dashboards for alerting",
		"building distributed tracing with opentelemetry context propagation across microservices",
		"hardening tls encryption cipher suites and certificate rotation workflows",
	}

	projects := []string{"apollo", "mercury", "gemini", "artemis", "voyager"}

	for i := 0; i < sessionCount; i++ {
		sid := fmt.Sprintf("bench-session-%04d", i)
		proj := projects[i%len(projects)]
		cwd := filepath.Join("/workspace", proj)
		isSub := (i % 5) == 0
		parentID := ""
		if isSub && i > 0 {
			parentID = fmt.Sprintf("bench-session-%04d", i-1)
		}

		storetest.InsertSession(b, con, storetest.Session{
			ID:           sid,
			StartedAt:    float64(1700000000 + i*3600),
			LastTS:       float64(1700000000 + i*3600 + 600),
			MessageCount: 6,
			IsSubagent:   isSub,
			ParentID:     parentID,
			Project:      proj,
			CWD:          cwd,
			SourceTool:   "claude",
		})

		topic := topics[i%len(topics)]
		msgs := []struct {
			role    string
			content string
		}{
			{"user", fmt.Sprintf("How do we implement %s in project %s?", topic, proj)},
			{"assistant", fmt.Sprintf("Here is the detailed guide on %s. First, review the system architecture and current performance bottlenecks.", topic)},
			{"user", "What are the key trade-offs between memory footprint and query latency?"},
			{"assistant", "Memory footprint increases with larger cache sizes and mmap buffers, while query latency drops due to fewer syscalls and page faults."},
			{"user", "Can you show a concrete benchmark comparison?"},
			{"assistant", "Yes, running benchmark tests with FTS5 and trigram indices shows significant speedups on warm and cold connections."},
		}

		for mIdx, m := range msgs {
			storetest.InsertMessage(b, con, storetest.Message{
				SessionID: sid,
				Role:      m.role,
				Content:   m.content,
				TS:        float64(1700000000 + i*3600 + mIdx*60),
				ISO:       fmt.Sprintf("2026-08-01T%02d:%02d:00Z", (i/60)%24, i%60),
				UUID:      fmt.Sprintf("msg-uuid-%04d-%d", i, mIdx),
			})
		}
	}

	return dbp
}

func connectBaselineRO(dbp string) (*sql.DB, error) {
	dsn := "file:" + dbp + "?mode=ro&_pragma=busy_timeout(5000)"
	con, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	con.SetMaxOpenConns(1)
	return con, nil
}

func connectMmapRO(dbp string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=mmap_size(%d)", dbp, store.ROMmapSize)
	con, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	con.SetMaxOpenConns(1)
	return con, nil
}

func connectMmapQueryOnlyRO(dbp string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=mmap_size(%d)&_pragma=query_only(1)", dbp, store.ROMmapSize)
	con, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	con.SetMaxOpenConns(1)
	return con, nil
}

func connectFullTunedRO(dbp string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=mmap_size(%d)&_pragma=query_only(1)&_pragma=cache_size(-16000)&_pragma=temp_store(MEMORY)", dbp, store.ROMmapSize)
	con, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	con.SetMaxOpenConns(1)
	return con, nil
}

func BenchmarkConnectionPragmas(b *testing.B) {
	dbp := seedRealisticBenchStore(b, 500)

	b.Run("Search/Baseline/Warm", func(b *testing.B) {
		con, err := connectBaselineRO(dbp)
		if err != nil {
			b.Fatal(err)
		}
		defer con.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			hits, err := store.SearchHits(con, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil {
				b.Fatalf("SearchHits: %v", err)
			}
			if len(hits) == 0 {
				b.Fatal("SearchHits: 0 hits returned")
			}
		}
	})

	b.Run("Search/MmapOnly/Warm", func(b *testing.B) {
		con, err := connectMmapRO(dbp)
		if err != nil {
			b.Fatal(err)
		}
		defer con.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			hits, err := store.SearchHits(con, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil {
				b.Fatalf("SearchHits: %v", err)
			}
			if len(hits) == 0 {
				b.Fatal("SearchHits: 0 hits returned")
			}
		}
	})

	b.Run("Search/MmapQueryOnly/Warm", func(b *testing.B) {
		con, err := connectMmapQueryOnlyRO(dbp)
		if err != nil {
			b.Fatal(err)
		}
		defer con.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			hits, err := store.SearchHits(con, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil {
				b.Fatalf("SearchHits: %v", err)
			}
			if len(hits) == 0 {
				b.Fatal("SearchHits: 0 hits returned")
			}
		}
	})

	b.Run("Search/FullTuned/Warm", func(b *testing.B) {
		con, err := connectFullTunedRO(dbp)
		if err != nil {
			b.Fatal(err)
		}
		defer con.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			hits, err := store.SearchHits(con, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil {
				b.Fatalf("SearchHits: %v", err)
			}
			if len(hits) == 0 {
				b.Fatal("SearchHits: 0 hits returned")
			}
		}
	})

	b.Run("Search/Baseline/Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			con, err := connectBaselineRO(dbp)
			if err != nil {
				b.Fatal(err)
			}
			hits, err := store.SearchHits(con, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil {
				con.Close()
				b.Fatalf("SearchHits: %v", err)
			}
			if len(hits) == 0 {
				con.Close()
				b.Fatal("SearchHits: 0 hits returned")
			}
			con.Close()
		}
	})

	b.Run("Search/MmapOnly/Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			con, err := connectMmapRO(dbp)
			if err != nil {
				b.Fatal(err)
			}
			hits, err := store.SearchHits(con, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil {
				con.Close()
				b.Fatalf("SearchHits: %v", err)
			}
			if len(hits) == 0 {
				con.Close()
				b.Fatal("SearchHits: 0 hits returned")
			}
			con.Close()
		}
	})

	b.Run("Search/MmapQueryOnly/Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			con, err := connectMmapQueryOnlyRO(dbp)
			if err != nil {
				b.Fatal(err)
			}
			hits, err := store.SearchHits(con, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil {
				con.Close()
				b.Fatalf("SearchHits: %v", err)
			}
			if len(hits) == 0 {
				con.Close()
				b.Fatal("SearchHits: 0 hits returned")
			}
			con.Close()
		}
	})

	b.Run("Search/FullTuned/Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			con, err := connectFullTunedRO(dbp)
			if err != nil {
				b.Fatal(err)
			}
			hits, err := store.SearchHits(con, "optimization", store.Filter{}, store.SortRelevance, 20)
			if err != nil {
				con.Close()
				b.Fatalf("SearchHits: %v", err)
			}
			if len(hits) == 0 {
				con.Close()
				b.Fatal("SearchHits: 0 hits returned")
			}
			con.Close()
		}
	})

	b.Run("Browse/Baseline/Warm", func(b *testing.B) {
		con, err := connectBaselineRO(dbp)
		if err != nil {
			b.Fatal(err)
		}
		defer con.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			sessions, err := store.BrowseSessions(con, "", "", 50)
			if err != nil {
				b.Fatalf("BrowseSessions: %v", err)
			}
			if len(sessions) == 0 {
				b.Fatal("BrowseSessions: 0 sessions returned")
			}
		}
	})

	b.Run("Browse/MmapOnly/Warm", func(b *testing.B) {
		con, err := connectMmapRO(dbp)
		if err != nil {
			b.Fatal(err)
		}
		defer con.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			sessions, err := store.BrowseSessions(con, "", "", 50)
			if err != nil {
				b.Fatalf("BrowseSessions: %v", err)
			}
			if len(sessions) == 0 {
				b.Fatal("BrowseSessions: 0 sessions returned")
			}
		}
	})

	b.Run("Browse/MmapQueryOnly/Warm", func(b *testing.B) {
		con, err := connectMmapQueryOnlyRO(dbp)
		if err != nil {
			b.Fatal(err)
		}
		defer con.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			sessions, err := store.BrowseSessions(con, "", "", 50)
			if err != nil {
				b.Fatalf("BrowseSessions: %v", err)
			}
			if len(sessions) == 0 {
				b.Fatal("BrowseSessions: 0 sessions returned")
			}
		}
	})

	b.Run("Browse/FullTuned/Warm", func(b *testing.B) {
		con, err := connectFullTunedRO(dbp)
		if err != nil {
			b.Fatal(err)
		}
		defer con.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			sessions, err := store.BrowseSessions(con, "", "", 50)
			if err != nil {
				b.Fatalf("BrowseSessions: %v", err)
			}
			if len(sessions) == 0 {
				b.Fatal("BrowseSessions: 0 sessions returned")
			}
		}
	})

	b.Run("Browse/Baseline/Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			con, err := connectBaselineRO(dbp)
			if err != nil {
				b.Fatal(err)
			}
			sessions, err := store.BrowseSessions(con, "", "", 50)
			if err != nil {
				con.Close()
				b.Fatalf("BrowseSessions: %v", err)
			}
			if len(sessions) == 0 {
				con.Close()
				b.Fatal("BrowseSessions: 0 sessions returned")
			}
			con.Close()
		}
	})

	b.Run("Browse/MmapOnly/Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			con, err := connectMmapRO(dbp)
			if err != nil {
				b.Fatal(err)
			}
			sessions, err := store.BrowseSessions(con, "", "", 50)
			if err != nil {
				con.Close()
				b.Fatalf("BrowseSessions: %v", err)
			}
			if len(sessions) == 0 {
				con.Close()
				b.Fatal("BrowseSessions: 0 sessions returned")
			}
			con.Close()
		}
	})

	b.Run("Browse/MmapQueryOnly/Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			con, err := connectMmapQueryOnlyRO(dbp)
			if err != nil {
				b.Fatal(err)
			}
			sessions, err := store.BrowseSessions(con, "", "", 50)
			if err != nil {
				con.Close()
				b.Fatalf("BrowseSessions: %v", err)
			}
			if len(sessions) == 0 {
				con.Close()
				b.Fatal("BrowseSessions: 0 sessions returned")
			}
			con.Close()
		}
	})

	b.Run("Browse/FullTuned/Cold", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			con, err := connectFullTunedRO(dbp)
			if err != nil {
				b.Fatal(err)
			}
			sessions, err := store.BrowseSessions(con, "", "", 50)
			if err != nil {
				con.Close()
				b.Fatalf("BrowseSessions: %v", err)
			}
			if len(sessions) == 0 {
				con.Close()
				b.Fatal("BrowseSessions: 0 sessions returned")
			}
			con.Close()
		}
	})
}
