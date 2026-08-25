package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestCheckIndexFreshness_WatermarkAndCatalog(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	catDir := filepath.Join(cfg, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}

	// 1. Unstamped DB
	freshness, err := CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if freshness.Fresh {
		t.Errorf("unstamped DB reported fresh, want false")
	}

	// 2. Catalog directory does not exist yet; stamp watermark
	if err := StampIngestWatermark(con); err != nil {
		t.Fatalf("StampIngestWatermark: %v", err)
	}
	freshness, err = CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if !freshness.Fresh {
		t.Errorf("stamped DB without catalog reported stale (%s), want true", freshness.Reason)
	}

	// 3. Create catalog dir and write a session entry
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := paths.WriteCatalogEntry(catDir, paths.CatalogEntry{
		SessionID:      "sess-001",
		TranscriptPath: filepath.Join(cfg, "s1.jsonl"),
	}); err != nil {
		t.Fatal(err)
	}

	// Now catalog was modified after ingest timestamp
	freshness, err = CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if freshness.Fresh {
		t.Errorf("DB reported fresh after catalog write, want stale")
	}

	// 4. Ingest/stamp again
	time.Sleep(10 * time.Millisecond)
	if err := StampIngestWatermark(con); err != nil {
		t.Fatalf("StampIngestWatermark: %v", err)
	}
	freshness, err = CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if !freshness.Fresh {
		t.Errorf("re-stamped DB reported stale (%s), want fresh", freshness.Reason)
	}
}

func TestCheckSessionFreshness_FreshStaleAndMissing(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	transDir := filepath.Join(cfg, "transcripts")
	if err := os.MkdirAll(transDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transPath := filepath.Join(transDir, "sess-001.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"hello"},"uuid":"u1","timestamp":"2026-08-20T10:00:00Z"}` + "\n"
	if err := os.WriteFile(transPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dbp := filepath.Join(store.CacheDir(), "project.db")
	if err := os.MkdirAll(store.CacheDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}

	if err := updateIndexWithOrigin(con, transDir, ""); err != nil {
		t.Fatalf("updateIndex: %v", err)
	}

	// 1. Fresh check
	sf, err := CheckSessionFreshness(con, "sess-001")
	if err != nil {
		t.Fatalf("CheckSessionFreshness: %v", err)
	}
	if sf.Status != SessionFresh {
		t.Errorf("expected SessionFresh, got %v (%s)", sf.Status, sf.Note)
	}

	// 2. Stale check (modify transcript file)
	time.Sleep(10 * time.Millisecond)
	newContent := content + `{"type":"assistant","message":{"role":"assistant","content":"world"},"uuid":"u2","timestamp":"2026-08-20T10:00:05Z"}` + "\n"
	if err := os.WriteFile(transPath, []byte(newContent), 0o644); err != nil {
		t.Fatal(err)
	}

	sf, err = CheckSessionFreshness(con, "sess-001")
	if err != nil {
		t.Fatalf("CheckSessionFreshness: %v", err)
	}
	if sf.Status != SessionStale {
		t.Errorf("expected SessionStale, got %v", sf.Status)
	}
	if sf.Note == "" {
		t.Errorf("expected staleness note, got empty")
	}

	// 3. Missing backing file check (delete transcript file)
	if err := os.Remove(transPath); err != nil {
		t.Fatal(err)
	}
	sf, err = CheckSessionFreshness(con, "sess-001")
	if err != nil {
		t.Fatalf("CheckSessionFreshness: %v", err)
	}
	if sf.Status != SessionMissingBacking {
		t.Errorf("expected SessionMissingBacking, got %v", sf.Status)
	}
	if sf.Note == "" {
		t.Errorf("expected missing backing note, got empty")
	}

	// 4. Session not found
	sf, err = CheckSessionFreshness(con, "nonexistent-id")
	if err != nil {
		t.Fatalf("CheckSessionFreshness: %v", err)
	}
	if sf.Status != SessionNotFound {
		t.Errorf("expected SessionNotFound, got %v", sf.Status)
	}
}

func TestConsolidate_CopiesFileIndex(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	transDir := filepath.Join(cfg, "transcripts")
	if err := os.MkdirAll(transDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transPath := filepath.Join(transDir, "sess-002.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"hello consolidate"},"uuid":"u1","timestamp":"2026-08-20T10:00:00Z"}` + "\n"
	if err := os.WriteFile(transPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dbp := filepath.Join(store.CacheDir(), "proj-002.db")
	if err := os.MkdirAll(store.CacheDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}
	if err := updateIndexWithOrigin(con, transDir, ""); err != nil {
		t.Fatalf("updateIndex: %v", err)
	}

	// Consolidate into consolidated.db
	if err := SyncConsolidatedFrom(dbp); err != nil {
		t.Fatalf("SyncConsolidatedFrom: %v", err)
	}

	conConsolidated, err := store.ConnectRO(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	defer conConsolidated.Close()

	var count int
	if err := conConsolidated.QueryRow("SELECT COUNT(*) FROM file_index WHERE session_id = ?", "sess-002").Scan(&count); err != nil {
		t.Fatalf("query file_index: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row in consolidated file_index for sess-002, got %d", count)
	}

	// Verify session freshness directly against consolidated store
	sf, err := CheckSessionFreshness(conConsolidated, "sess-002")
	if err != nil {
		t.Fatalf("CheckSessionFreshness on consolidated: %v", err)
	}
	if sf.Status != SessionFresh {
		t.Errorf("expected SessionFresh on consolidated store, got %v (%s)", sf.Status, sf.Note)
	}
}
