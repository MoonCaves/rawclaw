package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/paths"
)

func TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath(t *testing.T) {
	shells := []string{"/bin/sh", "dash", "bash"}
	templates := []struct {
		name string
		tmpl string
		json bool
	}{
		{name: "claude", tmpl: rawclawPrimeScript},
		{name: "codex", tmpl: rawclawCodexPrimeScript, json: true},
	}
	kinds := []string{
		"new",
		"regular",
		"fifo",
		"directory",
		"symlink",
		"dangling-symlink",
		"symlink-directory",
		"symlink-fifo",
		"socket",
	}

	for _, shellName := range shells {
		shell, err := exec.LookPath(shellName)
		if err != nil {
			t.Logf("shell %s unavailable: %v", shellName, err)
			continue
		}
		for _, tc := range templates {
			if tc.json {
				if _, err := exec.LookPath("python3"); err != nil {
					t.Logf("python3 unavailable; skipping %s under %s", tc.name, shellName)
					continue
				}
			}
			for _, kind := range kinds {
				t.Run(tc.name+"/"+shellName+"/"+kind, func(t *testing.T) {
					stubDir := t.TempDir()
					logPath := filepath.Join(t.TempDir(), "rawclaw.log")
					rawclawPath := stubRawclaw(t, stubDir)
					if err := os.WriteFile(rawclawPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$RAWCLAW_TEST_LOG\"\n"), 0o755); err != nil {
						t.Fatal(err)
					}
					catalogDir := filepath.Join(os.TempDir(), fmt.Sprintf("rawclaw-cat-%d", time.Now().UnixNano()))
					if err := os.MkdirAll(catalogDir, 0o755); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.RemoveAll(catalogDir) })
					scriptPath := filepath.Join(t.TempDir(), "prime.sh")
					if err := os.WriteFile(scriptPath, []byte(renderHookScript(tc.tmpl, "''")), 0o755); err != nil {
						t.Fatal(err)
					}

					sessionID := "fifo-claim-test"
					entry := filepath.Join(catalogDir, sessionID)
					var listener net.Listener
					var symlinkTarget string
					switch kind {
					case "regular":
						if err := os.WriteFile(entry, []byte("existing"), 0o644); err != nil {
							t.Fatal(err)
						}
					case "fifo":
						if err := syscall.Mkfifo(entry, 0o644); err != nil {
							t.Fatal(err)
						}
					case "directory":
						if err := os.Mkdir(entry, 0o755); err != nil {
							t.Fatal(err)
						}
					case "symlink":
						symlinkTarget = filepath.Join(catalogDir, "target")
						if err := os.WriteFile(symlinkTarget, []byte("target"), 0o644); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(symlinkTarget, entry); err != nil {
							t.Fatal(err)
						}
					case "dangling-symlink":
						symlinkTarget = filepath.Join(catalogDir, "missing-target")
						if err := os.Symlink(symlinkTarget, entry); err != nil {
							t.Fatal(err)
						}
					case "symlink-directory":
						symlinkTarget = filepath.Join(catalogDir, "target-dir")
						if err := os.Mkdir(symlinkTarget, 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(symlinkTarget, entry); err != nil {
							t.Fatal(err)
						}
					case "symlink-fifo":
						symlinkTarget = filepath.Join(catalogDir, "target-fifo")
						if err := syscall.Mkfifo(symlinkTarget, 0o644); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(symlinkTarget, entry); err != nil {
							t.Fatal(err)
						}
					case "socket":
						var err error
						listener, err = net.Listen("unix", entry)
						if err != nil {
							t.Fatal(err)
						}
						defer listener.Close()
					}
					var beforeMode os.FileMode
					if kind != "new" {
						info, err := os.Lstat(entry)
						if err != nil {
							t.Fatalf("lstat existing %s path before hook: %v", kind, err)
						}
						beforeMode = info.Mode()
					}

					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					cmd := exec.CommandContext(ctx, shell, scriptPath)
					cmd.Env = append(os.Environ(),
						"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
						"RAWCLAW_CATALOG_DIR="+catalogDir,
						"RAWCLAW_TEST_LOG="+logPath,
					)
					cmd.Stdin = strings.NewReader(`{"session_id":"` + sessionID + `"}`)
					out, err := cmd.Output()
					if ctx.Err() == context.DeadlineExceeded {
						t.Fatalf("hook hung on existing %s path under %s", kind, shellName)
					}
					if err != nil {
						t.Fatalf("hook failed for %s path under %s: %v; output=%q", kind, shellName, err, out)
					}

					calls := readDetachedIngestCalls(t, logPath, kind == "new")
					if kind == "new" {
						if len(calls) != 1 || calls[0] != "ingest "+sessionID {
							t.Fatalf("detached rawclaw calls = %q, want [%q]", calls, "ingest "+sessionID)
						}
						if info, err := os.Stat(entry); err != nil || !info.Mode().IsRegular() {
							t.Fatalf("new claim entry = %v, want regular file", err)
						}
						if _, err := paths.ReadCatalogEntry(entry); err != nil {
							t.Fatalf("new claim entry is not valid catalog JSON: %v", err)
						}
					} else {
						if len(calls) != 0 {
							t.Fatalf("detached rawclaw calls for existing %s path = %q, want none", kind, calls)
						}

						after, err := os.Lstat(entry)
						if err != nil {
							t.Fatalf("lstat existing %s path after hook: %v", kind, err)
						}
						if beforeMode != after.Mode() {
							t.Fatalf("existing %s mode changed from %v to %v", kind, beforeMode, after.Mode())
						}

						if strings.HasPrefix(kind, "symlink") || kind == "dangling-symlink" {
							gotTarget, err := os.Readlink(entry)
							if err != nil {
								t.Fatalf("read existing %s target: %v", kind, err)
							}
							if gotTarget != symlinkTarget {
								t.Fatalf("existing %s target changed from %q to %q", kind, symlinkTarget, gotTarget)
							}
						}
						if kind == "directory" {
							entries, err := os.ReadDir(entry)
							if err != nil {
								t.Fatalf("read existing directory: %v", err)
							}
							if len(entries) != 0 {
								t.Fatalf("existing directory received nested artifacts: %v", entries)
							}
						}
						if kind == "symlink-directory" {
							entries, err := os.ReadDir(symlinkTarget)
							if err != nil {
								t.Fatalf("read symlink target directory: %v", err)
							}
							if len(entries) != 0 {
								t.Fatalf("symlink target directory received nested artifacts: %v", entries)
							}
						}
						if kind == "regular" {
							got, err := os.ReadFile(entry)
							if err != nil {
								t.Fatal(err)
							}
							if string(got) != "existing" {
								t.Fatalf("existing regular claim changed to %q", got)
							}
						}
					}
					catalogEntries, err := os.ReadDir(catalogDir)
					if err != nil {
						t.Fatalf("read catalog directory after hook: %v", err)
					}
					for _, catalogEntry := range catalogEntries {
						if strings.HasPrefix(catalogEntry.Name(), ".tmp.") {
							t.Fatalf("temporary claim directory leaked: %s", catalogEntry.Name())
						}
					}
				})
			}
		}
	}
}

func readDetachedIngestCalls(t *testing.T, logPath string, waitForCall bool) []string {
	t.Helper()
	wait := 100 * time.Millisecond
	if waitForCall {
		wait = 2 * time.Second
	}
	deadline := time.Now().Add(wait)
	for {
		data, err := os.ReadFile(logPath)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		var calls []string
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line != "" {
				calls = append(calls, line)
			}
		}
		if (!waitForCall && len(calls) > 0) || (waitForCall && len(calls) >= 1) || time.Now().After(deadline) {
			return calls
		}
		time.Sleep(10 * time.Millisecond)
	}
}

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

	stubDir := t.TempDir()
	stubRawclaw(t, stubDir)

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
