package goose

import (
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestGooseUpstreamSchema_DiscoveryAndContainerMetadata verifies that the Goose adapter
// discovers sessions from a real upstream Block/AAIF Goose v1.10.0+ SQLite database,
// accurately mapping columns, CWD, default subagent/lineage fields, and resume commands.
func TestGooseUpstreamSchema_DiscoveryAndContainerMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	fix := setupUpstreamGooseFixture(t, tmpDir)

	adapter := NewRoot(fix.SessionsDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(containers) != 4 {
		t.Fatalf("got %d containers, want 4", len(containers))
	}

	byID := make(map[string]source.Container)
	for _, c := range containers {
		byID[c.ID] = c
	}

	// 1. Session 1: Standard coding session
	c1, ok := byID["01918a3b-745a-7140-84a1-000000000001"]
	if !ok {
		t.Fatalf("missing container for session 1")
	}
	if c1.CWD != "/Users/alice/projects/api-service" {
		t.Errorf("c1.CWD = %q, want /Users/alice/projects/api-service", c1.CWD)
	}
	// Upstream schema does NOT have parent_id or is_subagent columns; verify clean defaults
	if c1.IsSubagent {
		t.Errorf("c1.IsSubagent = true, want false (default)")
	}
	if c1.ParentID != "" {
		t.Errorf("c1.ParentID = %q, want empty (default)", c1.ParentID)
	}
	if wantPath := fix.DBPath + "#01918a3b-745a-7140-84a1-000000000001"; c1.Path != wantPath {
		t.Errorf("c1.Path = %q, want %q", c1.Path, wantPath)
	}
	wantResume := []string{"goose", "session", "--resume", "--session-id", "01918a3b-745a-7140-84a1-000000000001"}
	if len(c1.ResumeArgv) != len(wantResume) || c1.ResumeArgv[3] != "--session-id" || c1.ResumeArgv[4] != "01918a3b-745a-7140-84a1-000000000001" {
		t.Errorf("c1.ResumeArgv = %v, want %v", c1.ResumeArgv, wantResume)
	}

	// 2. Session 2: Web UI session
	c2, ok := byID["01918a3b-745a-7140-84a1-000000000002"]
	if !ok {
		t.Fatalf("missing container for session 2")
	}
	if c2.CWD != "/Users/alice/projects/web-ui" {
		t.Errorf("c2.CWD = %q, want /Users/alice/projects/web-ui", c2.CWD)
	}

	// 3. Session 3: System session
	c3, ok := byID["01918a3b-745a-7140-84a1-000000000003"]
	if !ok {
		t.Fatalf("missing container for session 3")
	}
	if c3.CWD != "/Users/alice/projects/infra" {
		t.Errorf("c3.CWD = %q, want /Users/alice/projects/infra", c3.CWD)
	}

	// 4. Session 4: Empty session
	c4, ok := byID["01918a3b-745a-7140-84a1-000000000004"]
	if !ok {
		t.Fatalf("missing container for session 4")
	}
	if c4.CWD != "/Users/alice/projects/empty" {
		t.Errorf("c4.CWD = %q, want /Users/alice/projects/empty", c4.CWD)
	}
}

// TestGooseUpstreamSchema_MessageExtractionAndContentParsing exercises message extraction,
// content block normalization (MCP JSON arrays, tool results, plain text), role normalization,
// and chronological ordering.
func TestGooseUpstreamSchema_MessageExtractionAndContentParsing(t *testing.T) {
	tmpDir := t.TempDir()
	fix := setupUpstreamGooseFixture(t, tmpDir)

	adapter := NewRoot(fix.SessionsDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	byID := make(map[string]source.Container)
	for _, c := range containers {
		byID[c.ID] = c
	}

	// --- Test Session 1: MCP JSON Content, Multi-block Text, Tool Result ---
	c1 := byID["01918a3b-745a-7140-84a1-000000000001"]
	msgs1, err := adapter.Messages(c1)
	if err != nil {
		t.Fatalf("Messages(c1) failed: %v", err)
	}
	if len(msgs1) != 4 {
		t.Fatalf("got %d messages for session 1, want 4", len(msgs1))
	}

	// Message 1: User single-block text
	if msgs1[0].Role != "user" || msgs1[0].Text != "Why is /api/v1/checkout taking 800ms?" {
		t.Errorf("msgs1[0] mismatch: role=%q text=%q", msgs1[0].Role, msgs1[0].Text)
	}

	// Message 2: Assistant multi-block text
	wantMsg2 := "I analyzed the profile traces.\nThe database connection pool is exhausted on checkout calls."
	if msgs1[1].Role != "assistant" || msgs1[1].Text != wantMsg2 {
		t.Errorf("msgs1[1] mismatch: role=%q text=%q", msgs1[1].Role, msgs1[1].Text)
	}

	// Message 3: User follow-up
	if msgs1[2].Role != "user" || msgs1[2].Text != "Can you increase the pool limit to 50?" {
		t.Errorf("msgs1[2] mismatch: role=%q text=%q", msgs1[2].Role, msgs1[2].Text)
	}

	// Message 4: Tool result block
	if msgs1[3].Role != "assistant" || msgs1[3].Text != "Updated config/db.yaml max_connections: 50" {
		t.Errorf("msgs1[3] mismatch: role=%q text=%q", msgs1[3].Role, msgs1[3].Text)
	}

	// Verify timestamp monotonicity
	for i := 1; i < len(msgs1); i++ {
		if msgs1[i].TS < msgs1[i-1].TS {
			t.Errorf("msgs1[%d].TS (%f) < msgs1[%d].TS (%f): not strictly ordered", i, msgs1[i].TS, i-1, msgs1[i-1].TS)
		}
	}

	// --- Test Session 3: Plain text content & System role ---
	c3 := byID["01918a3b-745a-7140-84a1-000000000003"]
	msgs3, err := adapter.Messages(c3)
	if err != nil {
		t.Fatalf("Messages(c3) failed: %v", err)
	}
	if len(msgs3) != 3 {
		t.Fatalf("got %d messages for session 3, want 3", len(msgs3))
	}
	if msgs3[0].Role != "system" || msgs3[0].Text != "System prompt initialized: You are a SQLite database optimization assistant." {
		t.Errorf("msgs3[0] mismatch: role=%q text=%q", msgs3[0].Role, msgs3[0].Text)
	}
	if msgs3[1].Role != "user" || msgs3[1].Text != "Explain WAL checkpoint behavior in SQLite." {
		t.Errorf("msgs3[1] mismatch: role=%q text=%q", msgs3[1].Role, msgs3[1].Text)
	}
	if msgs3[2].Role != "assistant" || msgs3[2].Text != "WAL mode buffers writes in a separate log file until checkpointed into the main database." {
		t.Errorf("msgs3[2] mismatch: role=%q text=%q", msgs3[2].Role, msgs3[2].Text)
	}

	// --- Test Session 4: Empty session ---
	c4 := byID["01918a3b-745a-7140-84a1-000000000004"]
	msgs4, err := adapter.Messages(c4)
	if err != nil {
		t.Fatalf("Messages(c4) failed: %v", err)
	}
	if len(msgs4) != 0 {
		t.Errorf("got %d messages for empty session 4, want 0", len(msgs4))
	}
}

// TestGooseUpstreamSchema_FullRawClawIndexingAndSearch performs full end-to-end indexing
// of the upstream Goose database into a RawClaw cache store and verifies FTS5 keyword recall.
func TestGooseUpstreamSchema_FullRawClawIndexingAndSearch(t *testing.T) {
	tmpDir := t.TempDir()
	fix := setupUpstreamGooseFixture(t, tmpDir)
	storeDB := filepath.Join(tmpDir, "rawclaw_index.db")

	adapter := NewRoot(fix.SessionsDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// 1. Index all discovered containers into RawClaw SQLite store
	nSessions, status, err := index.EnsureIndexedContainers(storeDB, false, containers, adapter.Messages, ID, "")
	if err != nil {
		t.Fatalf("EnsureIndexedContainers failed: %v", err)
	}
	if nSessions != 4 || status != index.IndexFresh {
		t.Errorf("EnsureIndexedContainers = (%d, %v), want (4, %v)", nSessions, status, index.IndexFresh)
	}

	// 2. Query store to verify session rows and message indexing
	con, err := store.ConnectRO(storeDB)
	if err != nil {
		t.Fatalf("ConnectRO failed: %v", err)
	}
	defer con.Close()

	// Total messages across sessions: 4 (sess1) + 2 (sess2) + 3 (sess3) + 0 (sess4) = 9
	var totalMsgs int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages").Scan(&totalMsgs); err != nil {
		t.Fatalf("count messages failed: %v", err)
	}
	if totalMsgs != 9 {
		t.Errorf("total indexed messages = %d, want 9", totalMsgs)
	}

	// Verify FTS5 keyword indexing for Goose session messages
	type searchCheck struct {
		term          string
		wantSessionID string
		wantCount     int
	}

	checks := []searchCheck{
		{
			term:          "checkout",
			wantSessionID: "01918a3b-745a-7140-84a1-000000000001",
			wantCount:     2, // in msg 1 and msg 2
		},
		{
			term:          "navbar",
			wantSessionID: "01918a3b-745a-7140-84a1-000000000002",
			wantCount:     1,
		},
		{
			term:          "checkpoint",
			wantSessionID: "01918a3b-745a-7140-84a1-000000000003",
			wantCount:     2,
		},
	}

	for _, tc := range checks {
		rows, err := con.Query(`
			SELECT m.session_id, m.text
			FROM messages_fts f
			JOIN messages m ON f.rowid = m.rowid
			WHERE messages_fts MATCH ?
		`, tc.term)
		if err != nil {
			t.Fatalf("search %q failed: %v", tc.term, err)
		}
		var foundSessions []string
		for rows.Next() {
			var sid, text string
			if err := rows.Scan(&sid, &text); err == nil {
				foundSessions = append(foundSessions, sid)
			}
		}
		rows.Close()

		if len(foundSessions) != tc.wantCount {
			t.Errorf("search %q returned %d hits (%v), want %d", tc.term, len(foundSessions), foundSessions, tc.wantCount)
		}
		for _, sid := range foundSessions {
			if sid != tc.wantSessionID {
				t.Errorf("search %q returned wrong session %q, want %q", tc.term, sid, tc.wantSessionID)
			}
		}
	}
}

// TestGooseAdapter_DetectAndRoots tests root path discovery and detection.
func TestGooseAdapter_DetectAndRoots(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GOOSE_HOME", filepath.Join(tmpDir, "custom_goose"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "xdg_data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "xdg_config"))

	roots := SessionsRoots()
	if len(roots) == 0 {
		t.Fatalf("expected non-empty roots from env")
	}

	// Verify detect helper
	if !detect(filepath.Join(tmpDir, "custom_goose", "sessions", "sessions.db")) {
		t.Errorf("detect failed for GOOSE_HOME path")
	}
	if !detect("/Users/alice/.local/share/goose/sessions/sessions.db#sess-1") {
		t.Errorf("detect failed for composite container path with #id")
	}
	if detect("/Users/alice/.local/share/claude/sessions.jsonl") {
		t.Errorf("detect matched non-goose JSONL path")
	}
}
