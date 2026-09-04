package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestActiveSession_LiveSearchFreshness(t *testing.T) {
	cfg := t.TempDir()
	catDir := filepath.Join(cfg, ".local", "share", "rawclaw", "catalog")
	t.Setenv("HOME", cfg)
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	t.Setenv("ANTIGRAVITY_HOME", filepath.Join(cfg, ".gemini", "antigravity-cli"))
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "")

	sessID := "active-freshness-sess-001"
	workDir := filepath.Join(cfg, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	agRoot := filepath.Join(cfg, ".gemini", "antigravity-cli")
	transPath := filepath.Join(agRoot, "brain", sessID, ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(transPath), 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Seed initial 2 messages in Antigravity session transcript
	initMsgs := strings.Join([]string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-20T10:00:00Z","content":"<USER_REQUEST>initial alpha question</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-20T10:00:05Z","content":"initial alpha response"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transPath, []byte(initMsgs), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Write catalog entry
	catEntry := paths.CatalogEntry{
		SessionID:      sessID,
		TranscriptPath: transPath,
		CWD:            workDir,
		Source:         "antigravity",
	}
	if err := paths.WriteCatalogEntry(paths.CatalogDir(), catEntry); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	// 3. Ingest initial 2 messages
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", sessID)
	if err != nil {
		t.Fatalf("initial ingest failed: %v\nout: %s", err, out)
	}

	con, _, err := index.OpenConsolidated()
	if err != nil {
		t.Fatalf("OpenConsolidated: %v", err)
	}
	var count int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sessID).Scan(&count); err != nil {
		con.Close()
		t.Fatalf("query count: %v", err)
	}
	con.Close()
	if count != 2 {
		t.Fatalf("expected initial message count 2, got %d", count)
	}

	// 4. Append 3 new turn messages to transcript
	time.Sleep(20 * time.Millisecond)
	f, err := os.OpenFile(transPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	newMsgs := strings.Join([]string{
		`{"step_index":2,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-20T10:01:00Z","content":"<USER_REQUEST>explain quantum teleportation protocol</USER_REQUEST>"}`,
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-20T10:01:05Z","content":"quantum teleportation transmits state using entanglement"}`,
		`{"step_index":4,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-20T10:02:00Z","content":"<USER_REQUEST>subsequent question</USER_REQUEST>"}`,
	}, "\n") + "\n"
	if _, err := f.WriteString(newMsgs); err != nil {
		f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	// 5. Run rawclaw "query" with --current-session <sessionID>
	searchOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--current-session", sessID, "teleportation")
	if err != nil {
		t.Fatalf("search failed: %v\nout: %s", err, searchOut)
	}

	// 6. Assert newly appended messages appear in search results
	if !strings.Contains(searchOut, "teleportation") {
		t.Errorf("expected search output to contain 'teleportation', got:\n%s", searchOut)
	}

	// 7. Assert message count in consolidated.db is updated to 5
	con, _, err = index.OpenConsolidated()
	if err != nil {
		t.Fatalf("OpenConsolidated after search: %v", err)
	}
	defer con.Close()
	var finalCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sessID).Scan(&finalCount); err != nil {
		t.Fatalf("query final count: %v", err)
	}
	if finalCount != 5 {
		t.Errorf("expected 5 messages in consolidated.db, got %d", finalCount)
	}
}

func TestCheckIndexFreshness_ActiveTranscriptModified(t *testing.T) {
	cfg := t.TempDir()
	catDir := filepath.Join(cfg, ".local", "share", "rawclaw", "catalog")
	t.Setenv("HOME", cfg)
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}

	transDir := filepath.Join(cfg, "transcripts")
	if err := os.MkdirAll(transDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transPath := filepath.Join(transDir, "active-check.jsonl")
	if err := os.WriteFile(transPath, []byte("initial transcript content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	catEntry := paths.CatalogEntry{
		SessionID:      "active-check-sess",
		TranscriptPath: transPath,
		Source:         "antigravity",
	}
	if err := paths.WriteCatalogEntry(catDir, catEntry); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	if err := index.EnsureSchema(con, "antigravity"); err != nil {
		t.Fatal(err)
	}

	if err := index.StampIngestWatermark(con); err != nil {
		t.Fatalf("StampIngestWatermark: %v", err)
	}

	freshness, err := index.CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if !freshness.Fresh {
		t.Fatalf("expected initial index to be fresh, got: %+v", freshness)
	}

	// Modify the transcript file after watermark
	time.Sleep(20 * time.Millisecond)
	f, err := os.OpenFile(transPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("newly appended turn\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	freshnessAfter, err := index.CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness after mod: %v", err)
	}
	if freshnessAfter.Fresh {
		t.Errorf("expected Fresh == false after transcript modification, got true")
	}
	if freshnessAfter.Reason != "active_sessions_modified" {
		t.Errorf("expected Reason == active_sessions_modified, got %q", freshnessAfter.Reason)
	}
}
