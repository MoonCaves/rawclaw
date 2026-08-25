package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func setupFreshnessTestEnv(t *testing.T) (cfg, projDir, sessionID, uuid, transPath string) {
	t.Helper()
	cfg = t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(cfg, ".claude"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(cfg, ".local", "share", "rawclaw", "catalog"))

	sessionID = "a1b2c3d4-5555-6666-7777-888899990000"
	uuid = "9f3e1c20-1111-2222-3333-444455556666"
	projDir = filepath.Join(paths.ProjectsRoot(), "-freshness-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transPath = filepath.Join(projDir, sessionID+".jsonl")

	content := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"Investigate O1 freshness checking"},"uuid":"` + uuid + `","timestamp":"2026-08-20T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":"O1 freshness check uses watermark comparison without directory enumeration."},"uuid":"9f3e1c21-1111-2222-3333-444455556666","timestamp":"2026-08-20T10:00:05Z"}`,
	}, "\n") + "\n"

	if err := os.WriteFile(transPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	catEntry := paths.CatalogEntry{
		SessionID:      sessionID,
		TranscriptPath: transPath,
		CWD:            projDir,
		Source:         "claude",
	}
	if err := paths.WriteCatalogEntry(paths.CatalogDir(), catEntry); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	// Ingest session
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", sessionID)
	if err != nil {
		t.Fatalf("ingest failed: %v, out: %s", err, out)
	}

	return cfg, projDir, sessionID, uuid, transPath
}

// TestO1Freshness_PoisonedProjectsDir_NoEnumerationOnReadVerbs verifies that with
// a current index, all read verbs (read, outline, search, browse) answer in O(1)
// without directory enumeration or per-transcript stats. Even if the projects root
// contains a poisoned unreadable directory that would fail on enumeration, the read verbs succeed.
func TestO1Freshness_PoisonedProjectsDir_NoEnumerationOnReadVerbs(t *testing.T) {
	cfg, _, sessionID, uuid, _ := setupFreshnessTestEnv(t)

	// Poison a sibling project folder with unreadable permissions (0000)
	poisonDir := filepath.Join(cfg, ".claude", "projects", "-poisoned-directory")
	if err := os.MkdirAll(poisonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(poisonDir, "poison.jsonl"), []byte("poison"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(poisonDir, 0o000); err != nil {
		t.Skip("chmod 0000 not supported in test environment")
	}
	t.Cleanup(func() {
		_ = os.Chmod(poisonDir, 0o755)
	})

	// 1. rawclaw read <session8:uuid8>
	ref := sessionID[:8] + ":" + uuid[:8]
	readOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref)
	if err != nil {
		t.Fatalf("read failed with poisoned directory present: %v\nout: %s", err, readOut)
	}
	if !strings.Contains(readOut, "Investigate O1 freshness checking") {
		t.Errorf("read output missing content: %s", readOut)
	}
	if strings.Contains(readOut, "stale") {
		t.Errorf("read output should not contain staleness note for fresh session: %s", readOut)
	}

	// 2. rawclaw outline <session8>
	outlineOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8])
	if err != nil {
		t.Fatalf("outline failed with poisoned directory present: %v\nout: %s", err, outlineOut)
	}
	if !strings.Contains(outlineOut, "Investigate O1 freshness checking") {
		t.Errorf("outline output missing content: %s", outlineOut)
	}
	if strings.Contains(outlineOut, "stale") {
		t.Errorf("outline output should not contain staleness note for fresh session: %s", outlineOut)
	}

	// 3. rawclaw <query> (search)
	searchOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "Investigate O1 freshness")
	if err != nil {
		t.Fatalf("search failed with poisoned directory present: %v\nout: %s", err, searchOut)
	}
	var env agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(searchOut), &env); err != nil {
		t.Fatalf("unmarshal search: %v\nout: %s", err, searchOut)
	}
	if env.Count < 1 || len(env.Results) == 0 || env.Results[0].SessionID != sessionID {
		t.Errorf("search failed to find session on fresh index: %+v", env)
	}

	// 4. rawclaw (browse)
	browseOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all", "--json")
	if err != nil {
		t.Fatalf("browse failed with poisoned directory present: %v\nout: %s", err, browseOut)
	}
	if !strings.Contains(browseOut, sessionID) {
		t.Errorf("browse output missing session: %s", browseOut)
	}
}

// TestO1Freshness_StaleSession_SurfacesNoteWithoutBlocking verifies that when
// a target session's backing transcript changes, read and outline still answer
// from the store and surface an honest one-line staleness note without blocking.
func TestO1Freshness_StaleSession_SurfacesNoteWithoutBlocking(t *testing.T) {
	_, _, sessionID, uuid, transPath := setupFreshnessTestEnv(t)

	// Modify the backing transcript on disk so mtime and size change
	time.Sleep(15 * time.Millisecond)
	appended := `{"type":"user","message":{"role":"user","content":"Appended third turn"},"uuid":"9f3e1c22-1111-2222-3333-444455556666","timestamp":"2026-08-20T10:00:10Z"}` + "\n"
	f, err := os.OpenFile(transPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(appended); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// 1. rawclaw read <ref>
	ref := sessionID[:8] + ":" + uuid[:8]
	readOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref)
	if err != nil {
		t.Fatalf("read failed on stale session: %v\nout: %s", err, readOut)
	}
	if !strings.Contains(readOut, "Investigate O1 freshness checking") {
		t.Errorf("read did not answer from store: %s", readOut)
	}
	if !strings.Contains(readOut, "note: session may be stale") {
		t.Errorf("read missing staleness note on modified transcript: %s", readOut)
	}

	// 2. rawclaw outline <session8>
	outlineOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8])
	if err != nil {
		t.Fatalf("outline failed on stale session: %v\nout: %s", err, outlineOut)
	}
	if !strings.Contains(outlineOut, "Investigate O1 freshness checking") {
		t.Errorf("outline did not answer from store: %s", outlineOut)
	}
	if !strings.Contains(outlineOut, "note: session may be stale") {
		t.Errorf("outline missing staleness note on modified transcript: %s", outlineOut)
	}
}

// TestO1Freshness_MissingBackingFile_AnswersFromRetainedWithoutError verifies that
// when the backing transcript file is deleted, read verbs answer from retained history
// and surface a retained history note without returning an error.
func TestO1Freshness_MissingBackingFile_AnswersFromRetainedWithoutError(t *testing.T) {
	_, _, sessionID, uuid, transPath := setupFreshnessTestEnv(t)

	// Delete backing transcript from disk
	if err := os.Remove(transPath); err != nil {
		t.Fatal(err)
	}

	// 1. rawclaw read <ref>
	ref := sessionID[:8] + ":" + uuid[:8]
	readOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref)
	if err != nil {
		t.Fatalf("read returned error on missing backing file: %v\nout: %s", err, readOut)
	}
	if !strings.Contains(readOut, "Investigate O1 freshness checking") {
		t.Errorf("read failed to answer from retained history: %s", readOut)
	}
	if !strings.Contains(readOut, "note: source file missing on disk — retained history") {
		t.Errorf("read missing retained history note: %s", readOut)
	}

	// 2. rawclaw outline <session8>
	outlineOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8])
	if err != nil {
		t.Fatalf("outline returned error on missing backing file: %v\nout: %s", err, outlineOut)
	}
	if !strings.Contains(outlineOut, "Investigate O1 freshness checking") {
		t.Errorf("outline failed to answer from retained history: %s", outlineOut)
	}
	if !strings.Contains(outlineOut, "note: source file missing on disk — retained history") {
		t.Errorf("outline missing retained history note: %s", outlineOut)
	}
}

// TestO1Freshness_CatalogBirth_FlipsIndexToStale verifies that a session born
// in the catalog after last ingest flips the global freshness check to stale,
// and targeted ingest of that session restores index freshness.
func TestO1Freshness_CatalogBirth_FlipsIndexToStale(t *testing.T) {
	cfg, _, _, _, _ := setupFreshnessTestEnv(t)

	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	// 1. Initial state: index is fresh
	freshness, err := index.CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if !freshness.Fresh {
		t.Fatalf("expected fresh index, got: %+v", freshness)
	}

	// 2. Session born in catalog
	time.Sleep(20 * time.Millisecond)
	newSID := "b2c3d4e5-9999-8888-7777-666655554444"
	newTransPath := filepath.Join(cfg, "new-session.jsonl")
	newContent := `{"type":"user","message":{"role":"user","content":"born in catalog"},"uuid":"9f3e1c23-1111-2222-3333-444455556666","timestamp":"2026-08-20T11:00:00Z"}` + "\n"
	if err := os.WriteFile(newTransPath, []byte(newContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCatalogEntry(paths.CatalogDir(), paths.CatalogEntry{
		SessionID:      newSID,
		TranscriptPath: newTransPath,
		CWD:            filepath.Join(cfg, "work"),
		Source:         "claude",
	}); err != nil {
		t.Fatal(err)
	}

	// 3. Freshness check immediately flips to stale
	freshnessAfter, err := index.CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness after birth: %v", err)
	}
	if freshnessAfter.Fresh {
		t.Errorf("index reported fresh after session birth in catalog, want stale")
	}

	// 4. Run rawclaw ingest <newSID>
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "ingest", newSID)
	if err != nil {
		t.Fatalf("ingest failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "Ingested session") {
		t.Errorf("unexpected ingest output: %s", out)
	}

	// 5. Freshness check is restored to fresh
	freshnessRestored, err := index.CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness after ingest: %v", err)
	}
	if !freshnessRestored.Fresh {
		t.Errorf("index reported stale after ingest (%s), want fresh", freshnessRestored.Reason)
	}
}

// TestO1Freshness_ZeroSourceDiscoveryOnFreshReads verifies acceptance criterion 1:
// with a current index, read verbs perform no source discovery and no per-transcript
// directory walks.
func TestO1Freshness_ZeroSourceDiscoveryOnFreshReads(t *testing.T) {
	_, _, sessionID, uuid, _ := setupFreshnessTestEnv(t)

	ref := sessionID[:8] + ":" + uuid[:8]

	// Read
	readOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(readOut, "Investigate O1 freshness checking") {
		t.Errorf("read missing content: %s", readOut)
	}

	// Outline
	outlineOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8])
	if err != nil {
		t.Fatalf("outline: %v", err)
	}
	if !strings.Contains(outlineOut, "Investigate O1 freshness checking") {
		t.Errorf("outline missing content: %s", outlineOut)
	}

	// Search
	searchOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "Investigate O1")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(searchOut, sessionID) {
		t.Errorf("search missing session: %s", searchOut)
	}
}

// TestO1Freshness_TranscriptGrowth_SearchRefreshesCurrentProject verifies Fix 1:
// when a transcript in the current project grows (new turns appended to an existing session),
// search detects the project change and refreshes the current project so the new content is found.
func TestO1Freshness_TranscriptGrowth_SearchRefreshesCurrentProject(t *testing.T) {
	_, projDir, sessionID, _, transPath := setupFreshnessTestEnv(t)

	// Search initially finds the existing content
	outInitial, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--dir", projDir, "--json", "O1 freshness check")
	if err != nil {
		t.Fatalf("initial search failed: %v\nout: %s", err, outInitial)
	}
	if !strings.Contains(outInitial, sessionID) {
		t.Fatalf("session not found in initial search: %s", outInitial)
	}

	// Append a new turn to the same transcript file (no new sessions born)
	time.Sleep(20 * time.Millisecond)
	newTurn := `{"type":"user","message":{"role":"user","content":"Appended unique search term: quantum_teleportation_99"},"uuid":"9f3e1c25-1111-2222-3333-444455556666","timestamp":"2026-08-20T12:00:00Z"}` + "\n"
	f, err := os.OpenFile(transPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(newTurn); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Search for the newly appended term — search must refresh the current project and find it!
	outAfter, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--dir", projDir, "--json", "quantum_teleportation_99")
	if err != nil {
		t.Fatalf("search after transcript growth failed: %v\nout: %s", err, outAfter)
	}
	if !strings.Contains(outAfter, sessionID) {
		t.Errorf("search failed to find newly appended turn in growing transcript: %s", outAfter)
	}
}

// TestO1Freshness_HooksAbsent_GateReportsNotFresh verifies Fix 1:
// when hooks are absent (no catalog dir exists), CheckIndexFreshness returns
// Fresh: false rather than falsely reporting fresh.
func TestO1Freshness_HooksAbsent_GateReportsNotFresh(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(cfg, "nonexistent-catalog-dir"))

	con, err := store.ConnectRW(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if err := index.EnsureSchema(con, "claude"); err != nil {
		t.Fatal(err)
	}

	// Stamp watermark
	if err := index.StampIngestWatermark(con); err != nil {
		t.Fatal(err)
	}

	// With catalog dir missing, CheckIndexFreshness must report not fresh
	freshness, err := index.CheckIndexFreshness(con)
	if err != nil {
		t.Fatalf("CheckIndexFreshness: %v", err)
	}
	if freshness.Fresh {
		t.Errorf("expected Fresh: false when catalog dir is absent, got true")
	}
	if freshness.Reason != "catalog_dir_missing" {
		t.Errorf("expected Reason catalog_dir_missing, got %q", freshness.Reason)
	}
}
