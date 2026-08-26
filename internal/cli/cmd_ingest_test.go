package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestDiscoverAllIngestSources_ReportsAdapterErrors(t *testing.T) {
	broken := &tagTestSource{discoverErr: errors.New("broken adapter")}
	working := &tagTestSource{containers: []source.Container{{ID: "working-session", Path: "/tmp/working.jsonl"}}}

	matches, err := discoverAllIngestSources([]source.Registration{
		tagTestRegistration("broken", broken),
		tagTestRegistration("working", working),
	})
	if err == nil || !strings.Contains(err.Error(), "broken adapter") {
		t.Fatalf("discoverAllIngestSources error = %v, want broken adapter error", err)
	}
	if len(matches) != 1 || matches[0].container.ID != "working-session" {
		t.Fatalf("discoverAllIngestSources matches = %+v, want working session", matches)
	}
}

func TestResolveIngestMatches_ReportsPartialDiscoveryError(t *testing.T) {
	broken := &tagTestSource{discoverErr: errors.New("broken adapter")}
	working := &tagTestSource{containers: []source.Container{{ID: "target-session", Path: "/tmp/target.jsonl"}}}

	matches, err := resolveIngestMatches("target-session", []source.Registration{
		tagTestRegistration("broken", broken),
		tagTestRegistration("working", working),
	})
	if err == nil || !strings.Contains(err.Error(), "broken adapter") {
		t.Fatalf("resolveIngestMatches error = %v, want broken adapter error", err)
	}
	if len(matches) != 1 || matches[0].container.ID != "target-session" {
		t.Fatalf("resolveIngestMatches matches = %+v, want target session", matches)
	}
}

// TestPrimeScripts_EmitDetachedIngest verifies that both Claude and Codex prime
// scripts contain the detached background ingest invocation.
func TestPrimeScripts_EmitDetachedIngest(t *testing.T) {
	claudeScript := renderHookScript(rawclawPrimeScript, "'/usr/local/bin/rawclaw'")
	for _, want := range []string{
		`nohup "$RAWCLAW" ingest "$session_id"`,
		`</dev/null >/dev/null 2>&1 &`,
	} {
		if !strings.Contains(claudeScript, want) {
			t.Errorf("Claude prime script missing %q", want)
		}
	}

	codexScript := renderHookScript(rawclawCodexPrimeScript, "'/usr/local/bin/rawclaw'")
	for _, want := range []string{
		`nohup "$RAWCLAW" ingest "$session_id"`,
		`</dev/null >/dev/null 2>&1 &`,
	} {
		if !strings.Contains(codexScript, want) {
			t.Errorf("Codex prime script missing %q", want)
		}
	}
}

// TestPrimeScripts_StopLaunchDetachedPrewarm verifies that both targets
// dispatch Stop without emitting the SessionStart banner or waiting for the
// prewarm command to finish.
func TestPrimeScripts_StopLaunchDetachedPrewarm(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}

	for _, tc := range []struct {
		name string
		tmpl string
	}{
		{name: "claude", tmpl: rawclawPrimeScript},
		{name: "codex", tmpl: rawclawCodexPrimeScript},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubDir := t.TempDir()
			logPath := filepath.Join(t.TempDir(), "calls.log")
			stubPath := filepath.Join(stubDir, "rawclaw")
			stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$RAWCLAW_TEST_LOG\"\n"
			if err := os.WriteFile(stubPath, []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}

			scriptPath := filepath.Join(t.TempDir(), "prime.sh")
			if err := os.WriteFile(scriptPath, []byte(renderHookScript(tc.tmpl, "''")), 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(sh, scriptPath)
			cmd.Env = append(os.Environ(),
				"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"RAWCLAW_TEST_LOG="+logPath,
			)
			cmd.Stdin = strings.NewReader(`{"hook_event_name":"Stop","session_id":"session-stop-123456"}`)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Stop hook failed: %v, out: %s", err, out)
			}

			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if b, err := os.ReadFile(logPath); err == nil {
					if got := strings.TrimSpace(string(b)); got == "prewarm session-stop-123456" {
						return
					} else if got != "" {
						t.Fatalf("prewarm args = %q", got)
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("detached prewarm did not run")
		})
	}
}

func TestPrimeScripts_SessionStartDeduplicatesDetachedIngest(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}

	for _, tc := range []struct {
		name string
		tmpl string
	}{
		{name: "claude", tmpl: rawclawPrimeScript},
		{name: "codex", tmpl: rawclawCodexPrimeScript},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			stubDir := filepath.Join(root, "bin")
			if err := os.MkdirAll(stubDir, 0o755); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(root, "calls.log")
			stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$RAWCLAW_TEST_LOG\"\n"
			if err := os.WriteFile(filepath.Join(stubDir, "rawclaw"), []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			scriptPath := filepath.Join(root, "prime.sh")
			if err := os.WriteFile(scriptPath, []byte(renderHookScript(tc.tmpl, "''")), 0o755); err != nil {
				t.Fatal(err)
			}
			catalogDir := filepath.Join(root, "catalog")
			env := append(os.Environ(),
				"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"RAWCLAW_TEST_LOG="+logPath,
				"RAWCLAW_CATALOG_DIR="+catalogDir,
				"HOME="+root,
			)
			payload := `{"session_id":"dedup-session-123"}`
			for run := range 2 {
				cmd := exec.Command(sh, scriptPath)
				cmd.Env = env
				cmd.Stdin = strings.NewReader(payload)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("SessionStart %d failed: %v (out=%q)", run+1, err, out)
				}
			}

			deadline := time.Now().Add(5 * time.Second)
			seen := false
			for time.Now().Before(deadline) {
				if b, err := os.ReadFile(logPath); err == nil {
					if got := strings.TrimSpace(string(b)); got == "ingest dedup-session-123" {
						seen = true
						break
					} else if got != "" {
						t.Fatalf("ingest calls = %q, want exactly one call", got)
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !seen {
				t.Fatal("detached ingest did not run")
			}
			time.Sleep(100 * time.Millisecond)
			b, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("read ingest log: %v", err)
			}
			if got := strings.TrimSpace(string(b)); got != "ingest dedup-session-123" {
				t.Fatalf("ingest calls = %q, want exactly one call", got)
			}
		})
	}
}

// TestClaudePrimeScript_ExecutesDetachedIngest verifies that running the Claude
// prime script in sh executes without blocking and exits 0 when rawclaw is stubbed.
func TestClaudePrimeScript_ExecutesDetachedIngest(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}

	stubDir := t.TempDir()
	stubRawclaw(t, stubDir)

	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(renderHookScript(rawclawPrimeScript, "''")), 0o755); err != nil {
		t.Fatal(err)
	}

	catalogDir := filepath.Join(t.TempDir(), "catalog")
	sessionID := "12345678-hook-ingest-0001"
	transcriptPath := filepath.Join(t.TempDir(), "session.jsonl")
	cwd := "/Users/test/workspace"

	payload := `{
		"session_id": "` + sessionID + `",
		"transcript_path": "` + transcriptPath + `",
		"cwd": "` + cwd + `"
	}`

	cmd := exec.Command(sh, scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RAWCLAW_CATALOG_DIR="+catalogDir,
	)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook execution failed: %v, out: %s", err, out)
	}

	if !strings.Contains(string(out), "[rawclaw] Raw transcript history") {
		t.Errorf("expected banner output, got: %q", string(out))
	}
}

// TestIngestCmd_IndexesFreshSession_EndToEnd exercises end-to-end ingestion of
// a newly created session transcript into consolidated.db and proves it is searchable.
func TestIngestCmd_IndexesFreshSession_EndToEnd(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(cfg, ".claude"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(cfg, ".local", "share", "rawclaw", "catalog"))

	cacheDir := store.CacheDir()
	if !strings.HasPrefix(cacheDir, cfg) {
		t.Fatalf("store.CacheDir() = %q not isolated inside %q", cacheDir, cfg)
	}

	sessionID := "11112222-3333-4444-5555-666677778888"
	projectDir := filepath.Join(cfg, "projects", "my-app")
	transPath := filepath.Join(projectDir, sessionID+".jsonl")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create Claude JSONL transcript
	jsonlContent := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"How do I configure redis cache pool?"},"uuid":"u-101","timestamp":"2026-08-20T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":"Configure redis with PoolSize and MinIdleConns."},"uuid":"u-102","timestamp":"2026-08-20T10:00:05Z"}`,
	}, "\n") + "\n"

	if err := os.WriteFile(transPath, []byte(jsonlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write catalog entry as prime.sh would
	catalogEntry := paths.CatalogEntry{
		SessionID:      sessionID,
		TranscriptPath: transPath,
		CWD:            filepath.Join(cfg, "work", "my-app"),
		Source:         "claude",
	}
	if err := paths.WriteCatalogEntry(paths.CatalogDir(), catalogEntry); err != nil {
		t.Fatalf("write catalog entry: %v", err)
	}

	// 1. Run rawclaw ingest <session_id>
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", sessionID)
	if err != nil {
		t.Fatalf("ingest failed: %v, out: %s", err, out)
	}

	if !strings.Contains(out, "Ingested session") || !strings.Contains(out, "2 messages") {
		t.Errorf("unexpected ingest output: %q", out)
	}

	// 2. Verify consolidated.db holds the session and messages
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()

	var msgCount int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id=?", sessionID).Scan(&msgCount); err != nil {
		t.Fatalf("query session from consolidated: %v", err)
	}
	if msgCount != 2 {
		t.Errorf("session message_count = %d, want 2", msgCount)
	}

	var rowCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sessionID).Scan(&rowCount); err != nil {
		t.Fatalf("query messages count: %v", err)
	}
	if rowCount != 2 {
		t.Errorf("messages rows count = %d, want 2", rowCount)
	}

	// 3. Search query finds the freshly ingested session
	searchOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "redis cache pool")
	if err != nil {
		t.Fatalf("search failed: %v, out: %s", err, searchOut)
	}

	var sRes struct {
		Results []struct {
			SessionID string `json:"session_id"`
			Role      string `json:"role"`
			Snippet   string `json:"snippet"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(searchOut), &sRes); err != nil {
		t.Fatalf("unmarshal search json: %v, raw: %s", err, searchOut)
	}
	if len(sRes.Results) == 0 {
		t.Fatalf("expected search results, got 0; search output: %s", searchOut)
	}
	if sRes.Results[0].SessionID != sessionID {
		t.Errorf("hit session_id = %q, want %q", sRes.Results[0].SessionID, sessionID)
	}
}

// TestIngestCmd_Idempotent_RepeatedRunIsNoOp verifies that a repeated ingest run
// on an unchanged session is a clean no-op and does not duplicate messages.
func TestIngestCmd_Idempotent_RepeatedRunIsNoOp(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(cfg, ".claude"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(cfg, ".local", "share", "rawclaw", "catalog"))

	sessionID := "22223333-4444-5555-6666-777788889999"
	projectDir := filepath.Join(cfg, "projects", "app-dup")
	transPath := filepath.Join(projectDir, sessionID+".jsonl")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	jsonlContent := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"Implement idempotent background worker"},"uuid":"u-201","timestamp":"2026-08-21T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":"Idempotency guaranteed by deduplication key."},"uuid":"u-202","timestamp":"2026-08-21T10:00:05Z"}`,
	}, "\n") + "\n"

	if err := os.WriteFile(transPath, []byte(jsonlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	catalogEntry := paths.CatalogEntry{
		SessionID:      sessionID,
		TranscriptPath: transPath,
		CWD:            filepath.Join(cfg, "work", "app-dup"),
		Source:         "claude",
	}
	if err := paths.WriteCatalogEntry(paths.CatalogDir(), catalogEntry); err != nil {
		t.Fatalf("write catalog entry: %v", err)
	}

	// First ingest run
	out1, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", sessionID)
	if err != nil {
		t.Fatalf("first ingest failed: %v, out: %s", err, out1)
	}

	// Second ingest run
	out2, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", sessionID)
	if err != nil {
		t.Fatalf("second ingest failed: %v, out: %s", err, out2)
	}

	// Verify database row counts are unchanged
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()

	var msgCount int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id=?", sessionID).Scan(&msgCount); err != nil {
		t.Fatalf("query session from consolidated: %v", err)
	}
	if msgCount != 2 {
		t.Errorf("session message_count = %d, want 2", msgCount)
	}

	var rowCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sessionID).Scan(&rowCount); err != nil {
		t.Fatalf("query messages count: %v", err)
	}
	if rowCount != 2 {
		t.Errorf("messages rows count = %d, want 2 (no duplicate rows)", rowCount)
	}
}

// TestIngestCmd_ConcurrentRuns verifies that multiple concurrent ingest invocations
// serialize safely without SQLite lock contention errors or corrupted state.
func TestIngestCmd_ConcurrentRuns(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(cfg, ".claude"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(cfg, ".local", "share", "rawclaw", "catalog"))

	const numSessions = 8
	var sessionIDs []string

	for i := 0; i < numSessions; i++ {
		sid := fmt.Sprintf("33334444-5555-6666-7777-%012d", i)
		sessionIDs = append(sessionIDs, sid)
		projectDir := filepath.Join(cfg, "projects", fmt.Sprintf("proj-%d", i))
		transPath := filepath.Join(projectDir, sid+".jsonl")
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			t.Fatal(err)
		}
		jsonlContent := strings.Join([]string{
			`{"type":"user","message":{"role":"user","content":"Concurrent test message ` + sid + `"},"uuid":"u-` + sid + `-1","timestamp":"2026-08-22T10:00:00Z"}`,
			`{"type":"assistant","message":{"role":"assistant","content":"Response for ` + sid + `"},"uuid":"u-` + sid + `-2","timestamp":"2026-08-22T10:00:05Z"}`,
		}, "\n") + "\n"
		if err := os.WriteFile(transPath, []byte(jsonlContent), 0o644); err != nil {
			t.Fatal(err)
		}
		catalogEntry := paths.CatalogEntry{
			SessionID:      sid,
			TranscriptPath: transPath,
			CWD:            filepath.Join(cfg, "work", fmt.Sprintf("proj-%d", i)),
			Source:         "claude",
		}
		if err := paths.WriteCatalogEntry(paths.CatalogDir(), catalogEntry); err != nil {
			t.Fatalf("write catalog entry: %v", err)
		}
	}

	// Run 16 concurrent ingest operations (both unique and overlapping sessions)
	var wg sync.WaitGroup
	errCh := make(chan error, 16)

	for i := 0; i < 16; i++ {
		wg.Add(1)
		targetSID := sessionIDs[i%numSessions]
		go func(sid string) {
			defer wg.Done()
			_, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", sid)
			if err != nil {
				errCh <- err
			}
		}(targetSID)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent ingest returned error: %v", err)
	}

	// Verify all sessions were properly indexed
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()

	for _, sid := range sessionIDs {
		var msgCount int
		if err := con.QueryRow("SELECT message_count FROM sessions WHERE id=?", sid).Scan(&msgCount); err != nil {
			t.Errorf("session %s missing from consolidated store: %v", sid, err)
		}
		if msgCount != 2 {
			t.Errorf("session %s message_count = %d, want 2", sid, msgCount)
		}
	}
}

// TestIngestCmd_NonExistentTranscript_SkipsGracefully verifies that attempting
// to ingest a session whose transcript file does not exist yet exits 0 cleanly.
func TestIngestCmd_NonExistentTranscript_SkipsGracefully(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(cfg, ".claude"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(cfg, ".local", "share", "rawclaw", "catalog"))

	sessionID := "99990000-1111-2222-3333-444455556666"
	catalogEntry := paths.CatalogEntry{
		SessionID:      sessionID,
		TranscriptPath: filepath.Join(cfg, "nonexistent", "session.jsonl"),
		CWD:            filepath.Join(cfg, "work"),
		Source:         "claude",
	}
	if err := paths.WriteCatalogEntry(paths.CatalogDir(), catalogEntry); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", sessionID)
	if err != nil {
		t.Fatalf("ingest should not error on nonexistent transcript, got: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "No active session matching") && !strings.Contains(out, "No sessions to ingest") {
		t.Logf("ingest output for non-existent transcript: %q", out)
	}
}

// TestIngestCmd_ErrorLogging verifies that logIngestError writes traces to ingest.log.
func TestIngestCmd_ErrorLogging(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	testErr := sql.ErrConnDone
	testSID := "err-test-session-123"
	logIngestError(testSID, testErr)

	logPath := filepath.Join(store.CacheDir(), "ingest.log")
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read ingest.log: %v", err)
	}

	content := string(b)
	if !strings.Contains(content, testSID) || !strings.Contains(content, "sql: connection is already closed") {
		t.Errorf("ingest.log missing error details; got: %q", content)
	}
}

// TestIngestCmd_HookChainDoesNotNestTheConsolidatedLock pins the #4 writer-fence
// bug: ingestContainerWithRetry -> EnsureFreshContainer -> SyncConsolidatedFrom
// must acquire the consolidated-store lock EXACTLY ONCE. Two independent
// flock.New() calls on the same path do not nest even within one process
// (flock() locks belong to the open file description, not the process), so a
// caller that ALSO locks before this chain runs would make every hook-
// triggered ingest hang for the fence's full wait window and then fail.
// Reproduced live before the fix: this exact test hung 40s+ and timed out.
func TestIngestCmd_HookChainDoesNotNestTheConsolidatedLock(t *testing.T) {
	_, _, sessionID, _, _ := setupFreshnessTestEnv(t)

	done := make(chan struct{})
	var out string
	var err error
	go func() {
		out, err = runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", sessionID)
		close(done)
	}()
	select {
	case <-done:
		if err != nil {
			t.Fatalf("ingest failed: %v, out: %s", err, out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ingest hook chain hung past 5s — the consolidated lock nested (see #4)")
	}
}
