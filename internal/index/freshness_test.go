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

	// 2. Catalog directory does not exist yet; stamp watermark (hooks absent -> not fresh)
	if err := StampIngestWatermark(con); err != nil {
		t.Fatalf("StampIngestWatermark: %v", err)
	}
	freshness, err = CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if freshness.Fresh {
		t.Errorf("stamped DB without catalog dir reported fresh, want false (missing signal)")
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

func TestCheckIndexFreshness_ReturnsDatabaseError(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	catDir := filepath.Join(cfg, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}
	if err := StampIngestWatermark(con); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("UPDATE meta SET value = NULL WHERE key = ?", MetaLastIngestTime); err != nil {
		t.Fatal(err)
	}

	freshness, err := CheckIndexFreshness(con)
	if err == nil {
		t.Fatalf("CheckIndexFreshness returned nil error with unreadable watermark: %+v", freshness)
	}
	if freshness.Fresh {
		t.Fatalf("unreadable watermark reported fresh: %+v", freshness)
	}
}

func TestCheckProjectFreshness_ReturnsDirectoryError(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	catDir := filepath.Join(cfg, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}
	if err := StampIngestWatermark(con); err != nil {
		t.Fatal(err)
	}

	notADirectory := filepath.Join(cfg, "project")
	if err := os.WriteFile(notADirectory, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	freshness, err := CheckProjectFreshness(con, "project", notADirectory)
	if err == nil {
		t.Fatalf("CheckProjectFreshness returned nil error for unreadable project directory: %+v", freshness)
	}
	if freshness.Fresh {
		t.Fatalf("unreadable project directory reported fresh: %+v", freshness)
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

// TestCheckSessionFreshness_ResumedMultipleFileIndexRows verifies Fix 3:
// when a session has multiple file_index rows (e.g. from resume/fork),
// CheckSessionFreshness matches the exact s.source_path copy and does not
// compare against the wrong file's watermark.
func TestCheckSessionFreshness_ResumedMultipleFileIndexRows(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	transDir := filepath.Join(cfg, "transcripts")
	if err := os.MkdirAll(transDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pathA := filepath.Join(transDir, "copyA.jsonl")
	pathB := filepath.Join(transDir, "copyB.jsonl")

	contentA := `{"type":"user","message":{"role":"user","content":"turn A"},"uuid":"11110000","timestamp":"2026-08-20T10:00:00Z"}` + "\n"
	contentB := `{"type":"user","message":{"role":"user","content":"turn B"},"uuid":"22220000","timestamp":"2026-08-20T11:00:00Z"}` + "\n"

	if err := os.WriteFile(pathA, []byte(contentA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte(contentB), 0o644); err != nil {
		t.Fatal(err)
	}

	stA, err := os.Stat(pathA)
	if err != nil {
		t.Fatal(err)
	}
	stB, err := os.Stat(pathB)
	if err != nil {
		t.Fatal(err)
	}

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}

	sessionID := "sess-resumed-001"
	// Session points to copyB as active source_path
	if _, err := con.Exec(`
		INSERT INTO sessions(id, source_path, message_count)
		VALUES(?, ?, 1)
	`, sessionID, pathB); err != nil {
		t.Fatal(err)
	}

	// Insert TWO file_index rows with the same session_id:
	// copyA has older mtime / different size
	if _, err := con.Exec(`
		INSERT INTO file_index(path, mtime, size, fp, session_id)
		VALUES(?, ?, ?, ?, ?), (?, ?, ?, ?, ?)
	`, pathA, mtimeOf(stA)-100, stA.Size()-10, "fpA", sessionID,
		pathB, mtimeOf(stB), stB.Size(), "", sessionID); err != nil {
		t.Fatal(err)
	}

	sf, err := CheckSessionFreshness(con, sessionID)
	if err != nil {
		t.Fatalf("CheckSessionFreshness: %v", err)
	}
	if sf.Status != SessionFresh {
		t.Errorf("expected SessionFresh when copyB matches disk, got %v (%s)", sf.Status, sf.Note)
	}
}

// TestHealUpgradedConsolidatedStore_SessionSourcesInvalidation verifies Fix 2:
// an upgraded consolidated store with sessions but no session_sources rows
// has its sync: fold-in watermarks invalidated, forcing a full re-fold that
// populates session_sources.
func TestHealUpgradedConsolidatedStore_SessionSourcesInvalidation(t *testing.T) {
	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}

	// Simulate old store: 1 session, 0 session_sources, 1 sync watermark
	if _, err := con.Exec("INSERT INTO sessions(id, source_path, message_count) VALUES(?, ?, 1)", "s1", "/tmp/s1.jsonl"); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("INSERT INTO meta(key, value) VALUES('sync:project.db', 'old-mark')"); err != nil {
		t.Fatal(err)
	}

	if err := healUpgradedConsolidatedStore(con); err != nil {
		t.Fatalf("healUpgradedConsolidatedStore: %v", err)
	}

	var count int
	if err := con.QueryRow("SELECT COUNT(*) FROM meta WHERE key LIKE 'sync:%'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected sync: watermarks to be invalidated (0 remaining), got %d", count)
	}
}

// TestHealUpgradedStore_MergedSessionSurvivesSingleSourceRefold verifies Fix 2:
// pre-populated store without session_sources rows → after sync, session_sources is populated
// and a merged session's row survives a single-source refold intact.
func TestHealUpgradedStore_MergedSessionSurvivesSingleSourceRefold(t *testing.T) {
	isolateCache(t)

	proj1 := filepath.Join(t.TempDir(), "proj1")
	proj2 := filepath.Join(t.TempDir(), "proj2")
	_ = os.MkdirAll(proj1, 0o755)
	_ = os.MkdirAll(proj2, 0o755)

	t1 := `{"type":"user","message":{"role":"user","content":"turn 1"},"uuid":"11110000","timestamp":"2026-08-20T10:00:00Z"}` + "\n"
	t2 := `{"type":"assistant","message":{"role":"assistant","content":"turn 2"},"uuid":"22220000","timestamp":"2026-08-20T10:00:05Z"}` + "\n"

	path1 := filepath.Join(proj1, "merged-sess.jsonl")
	path2 := filepath.Join(proj2, "merged-sess.jsonl")

	_ = os.WriteFile(path1, []byte(t1), 0o644)
	_ = os.WriteFile(path2, []byte(t2), 0o644)

	dbp1, _, _, err := EnsureIndexed(proj1, false)
	if err != nil {
		t.Fatal(err)
	}
	dbp2, _, _, err := EnsureIndexed(proj2, false)
	if err != nil {
		t.Fatal(err)
	}

	// Consolidate both
	if _, err := ConsolidateFrom([]string{dbp1, dbp2}, false); err != nil {
		t.Fatal(err)
	}

	// Simulate upgraded store: delete session_sources rows, but leave sessions/messages
	con, err := store.ConnectRW(ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("DELETE FROM session_sources"); err != nil {
		con.Close()
		t.Fatal(err)
	}
	con.Close()

	// Run single-source sync from dbp1
	if err := SyncConsolidatedFrom(dbp1); err != nil {
		t.Fatalf("SyncConsolidatedFrom: %v", err)
	}

	// Verify consolidated store state
	conRO, err := store.ConnectRO(ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	defer conRO.Close()

	var (
		msgCount int
		sources  int
	)
	if err := conRO.QueryRow("SELECT message_count FROM sessions WHERE id = ?", "merged-sess").Scan(&msgCount); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if msgCount != 2 {
		t.Errorf("expected merged session to retain 2 messages, got %d", msgCount)
	}

	if err := conRO.QueryRow("SELECT COUNT(*) FROM session_sources WHERE session_id = ?", "merged-sess").Scan(&sources); err != nil {
		t.Fatalf("query session_sources: %v", err)
	}
	if sources < 1 {
		t.Errorf("expected session_sources to be populated after heal, got %d", sources)
	}
}

func TestCheckProjectFreshness_IsSourceScoped(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	catDir := filepath.Join(cfg, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tdir := filepath.Join(cfg, "project")
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(tdir, "claude.jsonl")
	codexPath := filepath.Join(tdir, "codex.jsonl")
	if err := os.WriteFile(claudePath, []byte("claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexPath, []byte("codex\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}

	claudeMTime, claudeSize, claudeFP, err := backingFileState(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	codexMTime, codexSize, codexFP, err := backingFileState(codexPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id, source, path string
		mtime            float64
		size             int64
		fp               string
	}{
		{id: "claude-session", source: "claude", path: claudePath, mtime: claudeMTime, size: claudeSize, fp: claudeFP},
		{id: "codex-session", source: "codex", path: codexPath, mtime: codexMTime, size: codexSize, fp: codexFP},
	} {
		if _, err := con.Exec("INSERT INTO sessions(id, source_tool, source_path, project) VALUES(?,?,?,?)", row.id, row.source, row.path, "source-project"); err != nil {
			t.Fatal(err)
		}
		if _, err := con.Exec("INSERT INTO file_index(path, mtime, size, fp, session_id) VALUES(?,?,?,?,?)", row.path, row.mtime, row.size, row.fp, row.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := StampIngestWatermark(con); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(codexPath, []byte("codex\nnew turn\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	claudeFresh, err := CheckProjectFreshness(con, "source-project", tdir, "claude")
	if err != nil {
		t.Fatalf("claude freshness: %v", err)
	}
	if !claudeFresh.Fresh {
		t.Fatalf("unchanged Claude source reported stale: %+v", claudeFresh)
	}

	codexFresh, err := CheckProjectFreshness(con, "source-project", tdir, "codex")
	if err != nil {
		t.Fatalf("codex freshness: %v", err)
	}
	if codexFresh.Fresh {
		t.Fatalf("changed Codex source reported fresh: %+v", codexFresh)
	}
}

// TestCheckProjectFreshness_UnrelatedDaemonDB_DoesNotInvalidateProject verifies that
// background writes to global daemon databases (e.g. Goose or OpenCode session DBs)
// do not spuriously invalidate freshness for an unrelated, unchanged project.
func TestCheckProjectFreshness_UnrelatedDaemonDB_DoesNotInvalidateProject(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	catDir := filepath.Join(cfg, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tdir := filepath.Join(cfg, "project")
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	transPath := filepath.Join(tdir, "sess-001.jsonl")
	if err := os.WriteFile(transPath, []byte("sess-001 content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}

	mtime, size, fp, err := backingFileState(transPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("INSERT INTO sessions(id, source_tool, source_path, project) VALUES(?,?,?,?)", "sess-001", "claude", transPath, "project"); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("INSERT INTO file_index(path, mtime, size, fp, session_id) VALUES(?,?,?,?,?)", transPath, mtime, size, fp, "sess-001"); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCatalogEntry(catDir, paths.CatalogEntry{
		SessionID:      "sess-001",
		TranscriptPath: transPath,
	}); err != nil {
		t.Fatal(err)
	}
	if err := StampIngestWatermark(con); err != nil {
		t.Fatal(err)
	}

	// 1. Project should be fresh initially
	freshness, err := CheckProjectFreshness(con, "project", tdir)
	if err != nil {
		t.Fatalf("CheckProjectFreshness: %v", err)
	}
	if !freshness.Fresh {
		t.Fatalf("initial project reported not fresh: %+v", freshness)
	}

	// 2. Simulate active background daemon writing to an unrelated global DB
	gooseDir := filepath.Join(cfg, ".local", "share", "goose", "sessions")
	if err := os.MkdirAll(gooseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(gooseDir, "unrelated_daemon.db"), []byte("daemon wal commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. Project freshness must remain fresh (no daemon DB mtime thrashing)
	freshness2, err := CheckProjectFreshness(con, "project", tdir)
	if err != nil {
		t.Fatalf("CheckProjectFreshness after daemon write: %v", err)
	}
	if !freshness2.Fresh {
		t.Fatalf("unrelated daemon DB write spuriously invalidated project freshness: %+v", freshness2)
	}
}

// TestCheckProjectFreshness_CrossWorktree_DoesNotPolluteOrInvalidate verifies that
// two separate worktrees sharing the same project label do not cross-pollute each other's
// freshness cache when one worktree's files are modified or deleted.
func TestCheckProjectFreshness_CrossWorktree_DoesNotPolluteOrInvalidate(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	catDir := filepath.Join(cfg, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}

	wt1 := filepath.Join(cfg, "worktrees", "rawclaw")
	wt2 := filepath.Join(cfg, "worktrees", "rawclaw-feature-b")
	if err := os.MkdirAll(wt1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wt2, 0o755); err != nil {
		t.Fatal(err)
	}

	t1Path := filepath.Join(wt1, "session-wt1.jsonl")
	t2Path := filepath.Join(wt2, "session-wt2.jsonl")
	if err := os.WriteFile(t1Path, []byte("session wt1 content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(t2Path, []byte("session wt2 content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatal(err)
	}

	mt1, sz1, fp1, _ := backingFileState(t1Path)
	mt2, sz2, fp2, _ := backingFileState(t2Path)

	// Both recorded under project label "rawclaw"
	if _, err := con.Exec("INSERT INTO sessions(id, source_tool, source_path, project, cwd) VALUES(?,?,?,?,?)", "sess-wt1", "claude", t1Path, "rawclaw", wt1); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("INSERT INTO file_index(path, mtime, size, fp, session_id) VALUES(?,?,?,?,?)", t1Path, mt1, sz1, fp1, "sess-wt1"); err != nil {
		t.Fatal(err)
	}

	if _, err := con.Exec("INSERT INTO sessions(id, source_tool, source_path, project, cwd) VALUES(?,?,?,?,?)", "sess-wt2", "claude", t2Path, "rawclaw", wt2); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("INSERT INTO file_index(path, mtime, size, fp, session_id) VALUES(?,?,?,?,?)", t2Path, mt2, sz2, fp2, "sess-wt2"); err != nil {
		t.Fatal(err)
	}

	if err := paths.WriteCatalogEntry(catDir, paths.CatalogEntry{
		SessionID:      "sess-wt1",
		TranscriptPath: t1Path,
		CWD:            wt1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCatalogEntry(catDir, paths.CatalogEntry{
		SessionID:      "sess-wt2",
		TranscriptPath: t2Path,
		CWD:            wt2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := StampIngestWatermark(con); err != nil {
		t.Fatal(err)
	}

	// 1. wt1 should be fresh initially
	fresh1, err := CheckProjectFreshness(con, "rawclaw", wt1)
	if err != nil {
		t.Fatalf("CheckProjectFreshness wt1: %v", err)
	}
	if !fresh1.Fresh {
		t.Fatalf("wt1 initially reported not fresh: %+v", fresh1)
	}

	// 2. Now simulate worktree 2 being deleted or modified
	if err := os.Remove(t2Path); err != nil {
		t.Fatal(err)
	}

	// 3. wt1 MUST remain fresh despite wt2's file being deleted!
	fresh1After, err := CheckProjectFreshness(con, "rawclaw", wt1)
	if err != nil {
		t.Fatalf("CheckProjectFreshness wt1 after wt2 deletion: %v", err)
	}
	if !fresh1After.Fresh {
		t.Fatalf("Cross-worktree pollution bug: wt1 falsely marked stale due to wt2 deletion: %+v", fresh1After)
	}
}
