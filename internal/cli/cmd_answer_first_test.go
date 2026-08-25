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
}

// TestAnswerFirst_SpawnThrottling_RestrainsProcessStorm verifies Fix 1:
// multiple concurrent/repeated stale reads within the window acquire at most
// 1 background ingest spawn per session/global, preventing process storms.
func TestAnswerFirst_SpawnThrottling_RestrainsProcessStorm(t *testing.T) {
	cfg, _, sessionID, _, _ := setupFreshnessTestEnv(t)

	// Make index stale
	newSID := "d4e5f6a7-9999-8888-7777-666655554444"
	newTransPath := filepath.Join(cfg, "throttle-session.jsonl")
	newContent := `{"type":"user","message":{"role":"user","content":"throttle test"},"uuid":"9f3e1c25-1111-2222-3333-444455556666","timestamp":"2026-08-20T11:00:00Z"}` + "\n"
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

	calls, _ := countIngestSpawns(t)

	// 1st stale search: triggers background ingest spawn
	out1, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "Investigate O1")
	if err != nil {
		t.Fatalf("first search failed: %v", err)
	}
	if !strings.Contains(out1, "note: sessions not yet ingested — background ingest triggered") {
		t.Errorf("first search note missing trigger phrase: %s", out1)
	}
	if *calls != 1 {
		t.Fatalf("first search spawn count = %d, want 1", *calls)
	}

	// 2nd stale search immediately after: throttled, zero new spawns, note does not claim trigger
	out2, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "Investigate O1")
	if err != nil {
		t.Fatalf("second search failed: %v", err)
	}
	if strings.Contains(out2, "background ingest triggered") {
		t.Errorf("second search falsely claimed background ingest triggered: %s", out2)
	}
	if !strings.Contains(out2, "note: sessions not yet ingested — run 'rawclaw ingest' to refresh") {
		t.Errorf("second search missing honest refreshed note: %s", out2)
	}
	if *calls != 1 {
		t.Errorf("second search spawn count = %d, want 1 (throttled)", *calls)
	}

	// 3rd search in --json: throttled, zero new spawns, honest json note
	jsonOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "Investigate O1")
	if err != nil {
		t.Fatalf("third json search failed: %v", err)
	}
	var env struct {
		agentproto.SearchEnvelope
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &env); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if !env.Stale {
		t.Errorf("json report stale=false, want true")
	}
	if strings.Contains(env.StaleNote, "background ingest triggered") {
		t.Errorf("json stale_note falsely claimed background ingest triggered: %q", env.StaleNote)
	}
	if !strings.Contains(env.StaleNote, "sessions not yet ingested") {
		t.Errorf("json stale_note missing explanation: %q", env.StaleNote)
	}
	if *calls != 1 {
		t.Errorf("third json search spawn count = %d, want 1", *calls)
	}
	if env.Count < 1 || env.Results[0].SessionID != sessionID {
		t.Errorf("json search did not answer with session: %+v", env)
	}
}

// TestAnswerFirst_SuppressedSpawn_HonestNote verifies Fix 2:
// when RAWCLAW_BACKGROUND_INGEST=off suppresses the spawn, the stale note in both
// text and --json shapes does NOT falsely claim "background ingest triggered".
func TestAnswerFirst_SuppressedSpawn_HonestNote(t *testing.T) {
	t.Setenv("RAWCLAW_BACKGROUND_INGEST", "off")
	cfg, _, sessionID, uuid, transPath := setupFreshnessTestEnv(t)

	// Modify session to make it stale
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

	// Also make index stale
	newSID := "e5f6a7b8-9999-8888-7777-666655554444"
	newTransPath := filepath.Join(cfg, "suppressed-session.jsonl")
	newContent := `{"type":"user","message":{"role":"user","content":"suppressed test"},"uuid":"9f3e1c26-1111-2222-3333-444455556666","timestamp":"2026-08-20T11:00:00Z"}` + "\n"
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

	calls, _ := countIngestSpawns(t)

	// 1. Browse (text and json)
	browseText, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all")
	if err != nil {
		t.Fatalf("browse text: %v", err)
	}
	if strings.Contains(browseText, "background ingest triggered") {
		t.Errorf("browse text falsely claimed ingest triggered: %s", browseText)
	}
	if !strings.Contains(browseText, "note: sessions not yet ingested — run 'rawclaw ingest' to refresh") {
		t.Errorf("browse text missing honest note: %s", browseText)
	}

	browseJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all", "--json")
	if err != nil {
		t.Fatalf("browse json: %v", err)
	}
	var bJSON struct {
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(browseJSONOut), &bJSON); err != nil {
		t.Fatalf("unmarshal browse json: %v", err)
	}
	if !bJSON.Stale {
		t.Errorf("browse json reported stale=false, want true")
	}
	if strings.Contains(bJSON.StaleNote, "background ingest triggered") {
		t.Errorf("browse json falsely claimed ingest triggered: %q", bJSON.StaleNote)
	}
	if !strings.Contains(bJSON.StaleNote, "sessions not yet ingested") {
		t.Errorf("browse json missing honest stale note: %q", bJSON.StaleNote)
	}

	// 2. Search (text and json)
	searchText, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "Investigate O1")
	if err != nil {
		t.Fatalf("search text: %v", err)
	}
	if strings.Contains(searchText, "background ingest triggered") {
		t.Errorf("search text falsely claimed ingest triggered: %s", searchText)
	}
	if !strings.Contains(searchText, "note: sessions not yet ingested — run 'rawclaw ingest' to refresh") {
		t.Errorf("search text missing honest note: %s", searchText)
	}

	searchJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "Investigate O1")
	if err != nil {
		t.Fatalf("search json: %v", err)
	}
	var sJSON struct {
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(searchJSONOut), &sJSON); err != nil {
		t.Fatalf("unmarshal search json: %v", err)
	}
	if !sJSON.Stale {
		t.Errorf("search json reported stale=false, want true")
	}
	if strings.Contains(sJSON.StaleNote, "background ingest triggered") {
		t.Errorf("search json falsely claimed ingest triggered: %q", sJSON.StaleNote)
	}

	// 3. Read (text and json)
	ref := sessionID[:8] + ":" + uuid[:8]
	readText, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref)
	if err != nil {
		t.Fatalf("read text: %v", err)
	}
	if strings.Contains(readText, "background ingest triggered") {
		t.Errorf("read text falsely claimed ingest triggered: %s", readText)
	}
	if !strings.Contains(readText, "note: session may be stale (transcript updated) — run 'rawclaw ingest' to refresh") {
		t.Errorf("read text missing honest note: %s", readText)
	}

	readJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "read", ref, "--json")
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var rJSON struct {
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(readJSONOut), &rJSON); err != nil {
		t.Fatalf("unmarshal read json: %v", err)
	}
	if !rJSON.Stale {
		t.Errorf("read json reported stale=false, want true")
	}
	if strings.Contains(rJSON.StaleNote, "background ingest triggered") {
		t.Errorf("read json falsely claimed ingest triggered: %q", rJSON.StaleNote)
	}

	// 4. Outline (text and json)
	outlineText, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8])
	if err != nil {
		t.Fatalf("outline text: %v", err)
	}
	if strings.Contains(outlineText, "background ingest triggered") {
		t.Errorf("outline text falsely claimed ingest triggered: %s", outlineText)
	}
	if !strings.Contains(outlineText, "note: session may be stale (transcript updated) — run 'rawclaw ingest' to refresh") {
		t.Errorf("outline text missing honest note: %s", outlineText)
	}

	outlineJSONOut, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "outline", sessionID[:8], "--json")
	if err != nil {
		t.Fatalf("outline json: %v", err)
	}
	var oJSON struct {
		Stale     bool   `json:"stale"`
		StaleNote string `json:"stale_note"`
	}
	if err := json.Unmarshal([]byte(outlineJSONOut), &oJSON); err != nil {
		t.Fatalf("unmarshal outline json: %v", err)
	}
	if !oJSON.Stale {
		t.Errorf("outline json reported stale=false, want true")
	}
	if strings.Contains(oJSON.StaleNote, "background ingest triggered") {
		t.Errorf("outline json falsely claimed ingest triggered: %q", oJSON.StaleNote)
	}

	// Spawns must be exactly 0
	if *calls != 0 {
		t.Errorf("suppressed runs spawned %d ingest child, want 0", *calls)
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
