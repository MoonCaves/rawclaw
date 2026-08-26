package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/paths"
)

// TestClaudePrimeScript_CreatesSessionCatalogEntry verifies that starting a new
// Claude session creates a catalog entry named by the full session_id, recording
// transcript_path + cwd + source ("claude"), prints the banner once, and dedups
// subsequent calls (including simulated reboots).
func TestClaudePrimeScript_CreatesSessionCatalogEntry(t *testing.T) {
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
	sessionID := "12345678-abcd-ef01-2345-6789abcdef01"
	nonExistentTranscript := "/path/to/nonexistent/session.jsonl"
	cwd := "/Users/test/workspace"

	payload := `{
		"session_id": "` + sessionID + `",
		"transcript_path": "` + nonExistentTranscript + `",
		"cwd": "` + cwd + `"
	}`

	// 1. First run: writes catalog entry and prints discovery banner
	cmd1 := exec.Command(sh, scriptPath)
	cmd1.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RAWCLAW_CATALOG_DIR="+catalogDir,
	)
	cmd1.Stdin = strings.NewReader(payload)
	out1, err := cmd1.Output()
	if err != nil {
		t.Fatalf("first invocation failed: %v, out: %s", err, out1)
	}

	if !strings.Contains(string(out1), "[rawclaw] Raw transcript history") {
		t.Errorf("first invocation missing banner; got: %q", string(out1))
	}

	// Verify catalog entry on disk
	entryPath := filepath.Join(catalogDir, sessionID)
	entry, err := paths.ReadCatalogEntry(entryPath)
	if err != nil {
		t.Fatalf("read catalog entry at %s: %v", entryPath, err)
	}

	if entry.SessionID != sessionID {
		t.Errorf("entry.SessionID = %q, want %q", entry.SessionID, sessionID)
	}
	if entry.TranscriptPath != nonExistentTranscript {
		t.Errorf("entry.TranscriptPath = %q, want %q", entry.TranscriptPath, nonExistentTranscript)
	}
	if entry.CWD != cwd {
		t.Errorf("entry.CWD = %q, want %q", entry.CWD, cwd)
	}
	if entry.Source != "claude" {
		t.Errorf("entry.Source = %q, want \"claude\"", entry.Source)
	}

	// 2. Second run (same session, same process or simulated reboot with persistent catalog):
	// Must exit 0 with empty stdout (dedup).
	cmd2 := exec.Command(sh, scriptPath)
	cmd2.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RAWCLAW_CATALOG_DIR="+catalogDir,
	)
	cmd2.Stdin = strings.NewReader(payload)
	out2, err := cmd2.Output()
	if err != nil {
		t.Fatalf("second invocation failed: %v, out: %s", err, out2)
	}

	if len(strings.TrimSpace(string(out2))) != 0 {
		t.Errorf("second invocation should produce empty output (once-per-session dedup), got: %q", string(out2))
	}
}

// TestCodexPrimeScript_CreatesSessionCatalogEntry_FullPayload verifies that
// starting a Codex session with full payload creates a catalog entry with source="codex".
func TestCodexPrimeScript_CreatesSessionCatalogEntry_FullPayload(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3 available")
	}

	stubDir := t.TempDir()
	stubRawclaw(t, stubDir)

	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(renderHookScript(rawclawCodexPrimeScript, "''")), 0o755); err != nil {
		t.Fatal(err)
	}

	catalogDir := filepath.Join(t.TempDir(), "catalog")
	sessionID := "codex-full-session-999"
	transcriptPath := "/Users/test/.codex/sessions/rollout-1.jsonl"
	cwd := "/Users/test/codex-project"

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
		t.Fatalf("codex invocation failed: %v, out: %s", err, out)
	}

	// Verify valid hook-JSON envelope
	var env struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("stdout is not valid hook JSON: %v, raw: %q", err, string(out))
	}
	if env.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q, want SessionStart", env.HookSpecificOutput.HookEventName)
	}

	// Verify catalog entry
	entryPath := filepath.Join(catalogDir, sessionID)
	entry, err := paths.ReadCatalogEntry(entryPath)
	if err != nil {
		t.Fatalf("read codex catalog entry: %v", err)
	}

	if entry.SessionID != sessionID {
		t.Errorf("entry.SessionID = %q, want %q", entry.SessionID, sessionID)
	}
	if entry.TranscriptPath != transcriptPath {
		t.Errorf("entry.TranscriptPath = %q, want %q", entry.TranscriptPath, transcriptPath)
	}
	if entry.CWD != cwd {
		t.Errorf("entry.CWD = %q, want %q", entry.CWD, cwd)
	}
	if entry.Source != "codex" {
		t.Errorf("entry.Source = %q, want \"codex\"", entry.Source)
	}

	// Second run dedups
	cmd2 := exec.Command(sh, scriptPath)
	cmd2.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RAWCLAW_CATALOG_DIR="+catalogDir,
	)
	cmd2.Stdin = strings.NewReader(payload)
	out2, err := cmd2.Output()
	if err != nil {
		t.Fatalf("second codex invocation failed: %v", err)
	}
	if len(strings.TrimSpace(string(out2))) != 0 {
		t.Errorf("second codex invocation should produce empty output, got: %q", string(out2))
	}
}

// TestCodexPrimeScript_CreatesPartialCatalogEntry verifies that when Codex hook
// stdin only carries session_id (unverified optional fields omitted), a partial
// catalog entry is written and is valid.
func TestCodexPrimeScript_CreatesPartialCatalogEntry(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3 available")
	}

	stubDir := t.TempDir()
	stubRawclaw(t, stubDir)

	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(renderHookScript(rawclawCodexPrimeScript, "''")), 0o755); err != nil {
		t.Fatal(err)
	}

	catalogDir := filepath.Join(t.TempDir(), "catalog")
	sessionID := "codex-partial-id-only"

	payload := `{"session_id": "` + sessionID + `"}`

	cmd := exec.Command(sh, scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RAWCLAW_CATALOG_DIR="+catalogDir,
	)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("codex partial invocation failed: %v, out: %s", err, out)
	}

	entryPath := filepath.Join(catalogDir, sessionID)
	entry, err := paths.ReadCatalogEntry(entryPath)
	if err != nil {
		t.Fatalf("read partial catalog entry: %v", err)
	}

	if entry.SessionID != sessionID {
		t.Errorf("entry.SessionID = %q, want %q", entry.SessionID, sessionID)
	}
	if entry.Source != "codex" {
		t.Errorf("entry.Source = %q, want \"codex\"", entry.Source)
	}
	if entry.TranscriptPath != "" {
		t.Errorf("entry.TranscriptPath = %q, want \"\"", entry.TranscriptPath)
	}
	if entry.CWD != "" {
		t.Errorf("entry.CWD = %q, want \"\"", entry.CWD)
	}
}

// TestPrimeScript_CatalogWriteFailure_NeverFailsHook verifies that a catalog
// directory write failure (e.g. catalogDir is an unwritable file) is best-effort:
// the hook never fails and still prints the discovery banner.
func TestPrimeScript_CatalogWriteFailure_NeverFailsHook(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}

	logPath := filepath.Join(t.TempDir(), "calls.log")
	stubDir := t.TempDir()
	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$RAWCLAW_TEST_LOG\"\n"
	if err := os.WriteFile(filepath.Join(stubDir, "rawclaw"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(renderHookScript(rawclawPrimeScript, "''")), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a regular file at catalogDir path so mkdir -p fails
	badCatalogDir := filepath.Join(t.TempDir(), "catalog-as-file")
	if err := os.WriteFile(badCatalogDir, []byte("blocking-file"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(sh, scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RAWCLAW_TEST_LOG="+logPath,
		"RAWCLAW_CATALOG_DIR="+badCatalogDir,
	)
	cmd.Stdin = strings.NewReader(`{"session_id":"fail-test-sess","cwd":"/test"}`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook should not fail on catalog write error, got err: %v, out: %s", err, out)
	}
	if !strings.Contains(string(out), "[rawclaw] Raw transcript history") {
		t.Errorf("banner should still print despite catalog write failure; got: %q", string(out))
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(logPath); err == nil {
			if got := strings.TrimSpace(string(b)); got == "ingest fail-test-sess" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("detached ingest did not run on catalog write failure")
}

// TestSetupCmd_UpgradesLegacyPrimeScript verifies that re-running `rawclaw setup`
// replaces an older prime.sh (e.g. one using /tmp/rawclaw-prime) with the new catalog-writing script.
func TestSetupCmd_UpgradesLegacyPrimeScript(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", cfg)

	// First setup run
	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("setup --yes: %v", err)
	}

	scriptFile := hookScriptPath(cfg)
	b1, err := os.ReadFile(scriptFile)
	if err != nil {
		t.Fatalf("read prime.sh: %v", err)
	}
	if !strings.Contains(string(b1), "RAWCLAW_CATALOG_DIR") {
		t.Errorf("installed script missing RAWCLAW_CATALOG_DIR: %s", string(b1))
	}

	// Simulate old/legacy prime script on disk
	legacyContent := "#!/bin/sh\n# Old script\nmarker_dir=/tmp/rawclaw-prime\nexit 0\n"
	if err := os.WriteFile(scriptFile, []byte(legacyContent), 0o755); err != nil {
		t.Fatal(err)
	}

	// Re-run setup
	if _, err := runCmd(t, newSetupCmd(), "", "--yes"); err != nil {
		t.Fatalf("second setup --yes: %v", err)
	}

	b2, err := os.ReadFile(scriptFile)
	if err != nil {
		t.Fatalf("read prime.sh after upgrade: %v", err)
	}
	if !strings.Contains(string(b2), "RAWCLAW_CATALOG_DIR") {
		t.Errorf("upgraded script missing RAWCLAW_CATALOG_DIR: %s", string(b2))
	}
	if strings.Contains(string(b2), "/tmp/rawclaw-prime") {
		t.Errorf("upgraded script still has legacy /tmp/rawclaw-prime marker: %s", string(b2))
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
