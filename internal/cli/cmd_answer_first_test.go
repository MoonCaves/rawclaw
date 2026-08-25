package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

func countIngestSpawns(t *testing.T) (*int, *[]string) {
	t.Helper()
	calls := 0
	var args []string
	old := spawnIngest
	spawnIngest = func(sessionArg string) {
		calls++
		args = append(args, sessionArg)
	}
	t.Cleanup(func() { spawnIngest = old })
	return &calls, &args
}

// TestAnswerFirst_StaleIndexSearch_AnswersImmediatelyWithStalenessNoteAndSpawnsIngest
// verifies that a stale index answers immediately without blocking, surfaces an honest
// staleness note in text and structured boolean in --json, and kicks background ingest.
func TestAnswerFirst_StaleIndexSearch_AnswersImmediatelyWithStalenessNoteAndSpawnsIngest(t *testing.T) {
	cfg, _, sessionID, _, _ := setupFreshnessTestEnv(t)

	// 1. Fresh search: no note, no ingest spawn
	calls, _ := countIngestSpawns(t)
	freshOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "Investigate O1")
	if err != nil {
		t.Fatalf("fresh search failed: %v\nout: %s", err, freshOut)
	}
	if !strings.Contains(freshOut, sessionID[:8]) {
		t.Errorf("fresh search missing session %s: %s", sessionID[:8], freshOut)
	}
	if strings.Contains(freshOut, "sessions not yet ingested") {
		t.Errorf("fresh search surfaced staleness note: %s", freshOut)
	}
	if *calls != 0 {
		t.Errorf("fresh search spawned %d ingest child, want 0", *calls)
	}

	// Fresh search in --json: no stale field
	freshJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "Investigate O1")
	if err != nil {
		t.Fatalf("fresh json search failed: %v\nout: %s", err, freshJSONOut)
	}
	var freshEnv struct {
		agentproto.SearchEnvelope
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(freshJSONOut), &freshEnv); err != nil {
		t.Fatalf("unmarshal fresh json: %v\nout: %s", err, freshJSONOut)
	}
	if freshEnv.Stale {
		t.Errorf("fresh search json reported stale=true, want false")
	}

	// 2. Make index stale by creating a new session in catalog
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

	// 3. Stale search (text): answers from store immediately + surfaces staleness note + spawns ingest
	staleCalls, staleArgs := countIngestSpawns(t)
	staleOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "Investigate O1")
	if err != nil {
		t.Fatalf("stale search failed: %v\nout: %s", err, staleOut)
	}
	if !strings.Contains(staleOut, sessionID[:8]) {
		t.Errorf("stale search did not answer with session %s: %s", sessionID[:8], staleOut)
	}
	if !strings.Contains(staleOut, "note: sessions not yet ingested — background ingest triggered") {
		t.Errorf("stale search missing staleness note: %s", staleOut)
	}
	if *staleCalls != 1 {
		t.Errorf("stale search spawned %d ingest child, want 1", *staleCalls)
	}
	if len(*staleArgs) > 0 && (*staleArgs)[0] != "" {
		t.Errorf("global search spawned ingest for specific session %q, want empty (all)", (*staleArgs)[0])
	}

	// 4. Stale search (--json): structured stale: true, stale_note, and answers from store
	jsonCalls, _ := countIngestSpawns(t)
	staleJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "Investigate O1")
	if err != nil {
		t.Fatalf("stale json search failed: %v\nout: %s", err, staleJSONOut)
	}
	var staleEnv struct {
		agentproto.SearchEnvelope
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(staleJSONOut), &staleEnv); err != nil {
		t.Fatalf("unmarshal stale json: %v\nout: %s", err, staleJSONOut)
	}
	if !staleEnv.Stale {
		t.Errorf("stale search json reported stale=false, want true")
	}
	if !strings.Contains(staleEnv.StaleNote, "sessions not yet ingested") {
		t.Errorf("stale search json missing stale_note: %q", staleEnv.StaleNote)
	}
	if staleEnv.Count < 1 || len(staleEnv.Results) == 0 || staleEnv.Results[0].SessionID != sessionID {
		t.Errorf("stale search json missing session results: %+v", staleEnv)
	}
	if *jsonCalls != 1 {
		t.Errorf("stale json search spawned %d ingest child, want 1", *jsonCalls)
	}
}

// TestAnswerFirst_StaleSessionRead_AnswersImmediatelyWithStalenessNoteAndSpawnsIngest
// verifies that reading a stale session answers immediately, surfaces staleness note in text and
// structured boolean in --json, and kicks targeted background ingest.
func TestAnswerFirst_StaleSessionRead_AnswersImmediatelyWithStalenessNoteAndSpawnsIngest(t *testing.T) {
	_, _, sessionID, uuid, transPath := setupFreshnessTestEnv(t)

	// 1. Fresh read: no note, no ingest spawn
	ref := sessionID[:8] + ":" + uuid[:8]
	calls, _ := countIngestSpawns(t)
	freshOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref)
	if err != nil {
		t.Fatalf("fresh read failed: %v\nout: %s", err, freshOut)
	}
	if !strings.Contains(freshOut, "Investigate O1 freshness checking") {
		t.Errorf("fresh read missing content: %s", freshOut)
	}
	if strings.Contains(freshOut, "session may be stale") {
		t.Errorf("fresh read surfaced staleness note: %s", freshOut)
	}
	if *calls != 0 {
		t.Errorf("fresh read spawned %d ingest child, want 0", *calls)
	}

	// Fresh read --json
	freshJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref, "--json")
	if err != nil {
		t.Fatalf("fresh read json failed: %v\nout: %s", err, freshJSONOut)
	}
	var freshRes struct {
		agentproto.ReadResult
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(freshJSONOut), &freshRes); err != nil {
		t.Fatalf("unmarshal fresh read json: %v\nout: %s", err, freshJSONOut)
	}
	if freshRes.Stale {
		t.Errorf("fresh read json reported stale=true, want false")
	}

	// 2. Modify backing transcript
	time.Sleep(20 * time.Millisecond)
	appended := `{"type":"user","message":{"role":"user","content":"New turn appended to transcript"},"uuid":"9f3e1c22-1111-2222-3333-444455556666","timestamp":"2026-08-20T10:00:10Z"}` + "\n"
	f, err := os.OpenFile(transPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(appended); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// 3. Stale read (text): answers from store + surfaces staleness note + spawns targeted ingest
	staleCalls, staleArgs := countIngestSpawns(t)
	staleOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref)
	if err != nil {
		t.Fatalf("stale read failed: %v\nout: %s", err, staleOut)
	}
	if !strings.Contains(staleOut, "Investigate O1 freshness checking") {
		t.Errorf("stale read did not answer from store: %s", staleOut)
	}
	if !strings.Contains(staleOut, "note: session may be stale") {
		t.Errorf("stale read missing staleness note: %s", staleOut)
	}
	if *staleCalls != 1 {
		t.Errorf("stale read spawned %d ingest child, want 1", *staleCalls)
	}
	if len(*staleArgs) > 0 && (*staleArgs)[0] != sessionID {
		t.Errorf("stale read spawned ingest for session %q, want %q", (*staleArgs)[0], sessionID)
	}

	// 4. Stale read (--json): structured stale: true, stale_note, and answers from store
	jsonCalls, jsonArgs := countIngestSpawns(t)
	staleJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref, "--json")
	if err != nil {
		t.Fatalf("stale read json failed: %v\nout: %s", err, staleJSONOut)
	}
	var staleRes struct {
		agentproto.ReadResult
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(staleJSONOut), &staleRes); err != nil {
		t.Fatalf("unmarshal stale read json: %v\nout: %s", err, staleJSONOut)
	}
	if !staleRes.Stale {
		t.Errorf("stale read json reported stale=false, want true")
	}
	if !strings.Contains(staleRes.StaleNote, "session may be stale") {
		t.Errorf("stale read json missing stale_note: %q", staleRes.StaleNote)
	}
	if *jsonCalls != 1 {
		t.Errorf("stale read json spawned %d ingest child, want 1", *jsonCalls)
	}
	if len(*jsonArgs) > 0 && (*jsonArgs)[0] != sessionID {
		t.Errorf("stale read json spawned ingest for session %q, want %q", (*jsonArgs)[0], sessionID)
	}
}

// TestAnswerFirst_StaleSessionOutline_AnswersImmediatelyWithStalenessNoteAndSpawnsIngest
// verifies outline on a stale session answers immediately, surfaces staleness note in text
// and structured boolean in --json, and kicks targeted background ingest.
func TestAnswerFirst_StaleSessionOutline_AnswersImmediatelyWithStalenessNoteAndSpawnsIngest(t *testing.T) {
	_, _, sessionID, _, transPath := setupFreshnessTestEnv(t)

	// 1. Fresh outline: no note, no ingest spawn
	calls, _ := countIngestSpawns(t)
	freshOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8])
	if err != nil {
		t.Fatalf("fresh outline failed: %v\nout: %s", err, freshOut)
	}
	if !strings.Contains(freshOut, "Investigate O1 freshness checking") {
		t.Errorf("fresh outline missing content: %s", freshOut)
	}
	if strings.Contains(freshOut, "session may be stale") {
		t.Errorf("fresh outline surfaced staleness note: %s", freshOut)
	}
	if *calls != 0 {
		t.Errorf("fresh outline spawned %d ingest child, want 0", *calls)
	}

	// Fresh outline --json
	freshJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8], "--json")
	if err != nil {
		t.Fatalf("fresh outline json failed: %v\nout: %s", err, freshJSONOut)
	}
	var freshRes struct {
		agentproto.OutlineResult
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(freshJSONOut), &freshRes); err != nil {
		t.Fatalf("unmarshal fresh outline json: %v\nout: %s", err, freshJSONOut)
	}
	if freshRes.Stale {
		t.Errorf("fresh outline json reported stale=true, want false")
	}

	// 2. Modify backing transcript
	time.Sleep(20 * time.Millisecond)
	appended := `{"type":"user","message":{"role":"user","content":"New turn appended to transcript"},"uuid":"9f3e1c22-1111-2222-3333-444455556666","timestamp":"2026-08-20T10:00:10Z"}` + "\n"
	f, err := os.OpenFile(transPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(appended); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// 3. Stale outline (text): answers from store + surfaces staleness note + spawns targeted ingest
	staleCalls, staleArgs := countIngestSpawns(t)
	staleOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8])
	if err != nil {
		t.Fatalf("stale outline failed: %v\nout: %s", err, staleOut)
	}
	if !strings.Contains(staleOut, "Investigate O1 freshness checking") {
		t.Errorf("stale outline did not answer from store: %s", staleOut)
	}
	if !strings.Contains(staleOut, "note: session may be stale") {
		t.Errorf("stale outline missing staleness note: %s", staleOut)
	}
	if *staleCalls != 1 {
		t.Errorf("stale outline spawned %d ingest child, want 1", *staleCalls)
	}
	if len(*staleArgs) > 0 && (*staleArgs)[0] != sessionID {
		t.Errorf("stale outline spawned ingest for session %q, want %q", (*staleArgs)[0], sessionID)
	}

	// 4. Stale outline (--json): structured stale: true, stale_note, and answers from store
	jsonCalls, jsonArgs := countIngestSpawns(t)
	staleJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8], "--json")
	if err != nil {
		t.Fatalf("stale outline json failed: %v\nout: %s", err, staleJSONOut)
	}
	var staleRes struct {
		agentproto.OutlineResult
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(staleJSONOut), &staleRes); err != nil {
		t.Fatalf("unmarshal stale outline json: %v\nout: %s", err, staleJSONOut)
	}
	if !staleRes.Stale {
		t.Errorf("stale outline json reported stale=false, want true")
	}
	if !strings.Contains(staleRes.StaleNote, "session may be stale") {
		t.Errorf("stale outline json missing stale_note: %q", staleRes.StaleNote)
	}
	if *jsonCalls != 1 {
		t.Errorf("stale outline json spawned %d ingest child, want 1", *jsonCalls)
	}
	if len(*jsonArgs) > 0 && (*jsonArgs)[0] != sessionID {
		t.Errorf("stale outline json spawned ingest for session %q, want %q", (*jsonArgs)[0], sessionID)
	}
}

// TestAnswerFirst_StaleIndexBrowse_AnswersImmediatelyWithStalenessNoteAndSpawnsIngest
// verifies browse on a stale index answers immediately, surfaces staleness note in text and
// structured boolean in --json, and kicks background ingest.
func TestAnswerFirst_StaleIndexBrowse_AnswersImmediatelyWithStalenessNoteAndSpawnsIngest(t *testing.T) {
	cfg, _, sessionID, _, _ := setupFreshnessTestEnv(t)

	// 1. Fresh browse: no note, no ingest spawn
	calls, _ := countIngestSpawns(t)
	freshOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all")
	if err != nil {
		t.Fatalf("fresh browse failed: %v\nout: %s", err, freshOut)
	}
	if !strings.Contains(freshOut, sessionID[:8]) {
		t.Errorf("fresh browse missing session %s: %s", sessionID[:8], freshOut)
	}
	if strings.Contains(freshOut, "sessions not yet ingested") {
		t.Errorf("fresh browse surfaced staleness note: %s", freshOut)
	}
	if *calls != 0 {
		t.Errorf("fresh browse spawned %d ingest child, want 0", *calls)
	}

	// 2. Make index stale by adding session in catalog
	time.Sleep(20 * time.Millisecond)
	newSID := "c3d4e5f6-9999-8888-7777-666655554444"
	newTransPath := filepath.Join(cfg, "browse-session.jsonl")
	newContent := `{"type":"user","message":{"role":"user","content":"born for browse"},"uuid":"9f3e1c24-1111-2222-3333-444455556666","timestamp":"2026-08-20T11:00:00Z"}` + "\n"
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

	// 3. Stale browse (text): answers from store + surfaces staleness note + spawns ingest
	staleCalls, _ := countIngestSpawns(t)
	staleOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all")
	if err != nil {
		t.Fatalf("stale browse failed: %v\nout: %s", err, staleOut)
	}
	if !strings.Contains(staleOut, sessionID[:8]) {
		t.Errorf("stale browse did not answer with session %s: %s", sessionID[:8], staleOut)
	}
	if !strings.Contains(staleOut, "note: sessions not yet ingested — background ingest triggered") {
		t.Errorf("stale browse missing staleness note: %s", staleOut)
	}
	if *staleCalls != 1 {
		t.Errorf("stale browse spawned %d ingest child, want 1", *staleCalls)
	}

	// 4. Stale browse (--json): structured stale: true, stale_note, and answers from store
	jsonCalls, _ := countIngestSpawns(t)
	staleJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all", "--json")
	if err != nil {
		t.Fatalf("stale browse json failed: %v\nout: %s", err, staleJSONOut)
	}
	var staleBrowse struct {
		Scope     string              `json:"scope"`
		Stale     bool                `json:"stale"`
		StaleNote string              `json:"stale_note"`
		Sessions  []view.BrowseAllRow `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(staleJSONOut), &staleBrowse); err != nil {
		t.Fatalf("unmarshal stale browse json: %v\nout: %s", err, staleJSONOut)
	}
	if !staleBrowse.Stale {
		t.Errorf("stale browse json reported stale=false, want true")
	}
	if !strings.Contains(staleBrowse.StaleNote, "sessions not yet ingested") {
		t.Errorf("stale browse json missing stale_note: %q", staleBrowse.StaleNote)
	}
	if len(staleBrowse.Sessions) == 0 || staleBrowse.Sessions[0].SessionID != sessionID {
		t.Errorf("stale browse json missing sessions: %+v", staleBrowse)
	}
	if *jsonCalls != 1 {
		t.Errorf("stale browse json spawned %d ingest child, want 1", *jsonCalls)
	}
}

// TestAnswerFirst_ReindexForcesSynchronousRebuild verifies acceptance criterion 4:
// passing --reindex forces synchronous index rebuild.
func TestAnswerFirst_ReindexForcesSynchronousRebuild(t *testing.T) {
	_, _, sessionID, _, _ := setupFreshnessTestEnv(t)

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "Investigate O1")
	if err != nil {
		t.Fatalf("reindex search failed: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, sessionID[:8]) {
		t.Errorf("reindex search missing session %s: %s", sessionID[:8], out)
	}
}

// TestAnswerFirst_StoreMissing_FallsBackToPerProjectDBs verifies that when the consolidated
// store is absent or cannot be opened, read verbs fall back to per-project databases cleanly.
func TestAnswerFirst_StoreMissing_FallsBackToPerProjectDBs(t *testing.T) {
	root := newCfgRoot(t)
	writeIndexedSession(t, root, "-home-u-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"2026-06-01T10:00:00Z", "hello store missing fallback")

	// Search falls back and finds session
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "hello store missing")
	if err != nil {
		t.Fatalf("search with missing consolidated store failed: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "aaaa1111") {
		t.Errorf("search failed to find session from per-project fallback: %s", out)
	}
}

// TestSpawnIngestChild_DetachedChildRunsWithLog verifies that spawnIngestChild launches
// a detached process and writes its output to ingest.log.
func TestSpawnIngestChild_DetachedChildRunsWithLog(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	script := filepath.Join(t.TempDir(), "fake-rawclaw")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"ingest-child-argv $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe := selfExe
	selfExe = func() (string, error) { return script, nil }
	t.Cleanup(func() { selfExe = oldExe })

	spawnIngestChild("target-session-123")

	deadline := time.Now().Add(5 * time.Second)
	want := "ingest-child-argv ingest target-session-123"
	logPath := filepath.Join(cfg, ".cache", "session-search", "ingest.log")
	for {
		b, _ := os.ReadFile(logPath)
		if strings.Contains(string(b), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("receipt log never showed %q; log:\n%s", want, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
