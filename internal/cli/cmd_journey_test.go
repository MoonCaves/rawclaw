package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestCLIJourney_AntigravityEndToEnd exercises the complete black-box user journey
// against the Cobra command tree:
// 1. Fresh Ingest: Seed Antigravity session -> Search -> Verify JSON output
// 2. Read & Outline: Read session messages -> Outline session arc
// 3. Resume: Verify resume command syntax (agy --conversation)
// 4. Incremental Append: Append turn to transcript -> Search -> Verify updated count
// 5. Retention & Delete: Purge source file -> Verify retained history -> Delete session
func TestCLIJourney_AntigravityEndToEnd(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(cfg, ".claude"))
	t.Setenv("ANTIGRAVITY_HOME", filepath.Join(cfg, ".gemini", "antigravity-cli"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	// Hard-fail if the resolved cache store is not isolated inside cfg
	cacheDir := store.CacheDir()
	if !strings.HasPrefix(cacheDir, cfg) {
		t.Fatalf("CRITICAL: store.CacheDir() = %q is not isolated inside temp dir %q", cacheDir, cfg)
	}

	agRoot := filepath.Join(cfg, ".gemini", "antigravity-cli")
	sessID := "88880001-0000-0000-0000-000000000001"
	workDir := filepath.Join(cfg, "work", "payment-service")

	// Seed history.jsonl
	histPath := filepath.Join(agRoot, "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(histPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(histPath, []byte(`{"conversationId":"`+sessID+`","workspace":"`+workDir+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Seed session transcript
	transPath := filepath.Join(agRoot, "brain", sessID, ".system_generated", "logs", "transcript_full.jsonl")
	if err := os.MkdirAll(filepath.Dir(transPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initialJSONL := strings.Join([]string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>deploy stripe webhook handler</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:05Z","thinking":"inspecting handlers","content":"Checking handler implementation","tool_calls":[{"name":"run_command","args":{"CommandLine":"go test ./pkg/webhook"}}]}`,
		`{"step_index":2,"source":"MODEL","type":"RUN_COMMAND","created_at":"2026-08-15T10:00:10Z","content":"PASS\nok pkg/webhook 0.05s"}`,
	}, "\n") + "\n"

	if err := os.WriteFile(transPath, []byte(initialJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Fresh Search Journey (JSON)
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "stripe webhook")
	if err != nil {
		t.Fatalf("search --json failed: %v\nout: %s", err, out)
	}
	var env agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("unmarshal search output: %v\nout: %s", err, out)
	}
	if env.Count < 1 {
		t.Fatalf("search returned count %d, want >= 1; out: %s", env.Count, out)
	}
	var readRef string
	found := false
	for _, hit := range env.Results {
		if hit.SessionID == sessID {
			found = true
			readRef = hit.ReadRef
			if hit.Project != "payment-service" {
				t.Errorf("hit.Project = %q, want payment-service", hit.Project)
			}
		}
	}
	if !found {
		t.Fatalf("session %s not found in search results: %s", sessID, out)
	}

	// 2. Read Journey (takes <session8>:<uuid8>)
	outRead, err := runCmd(t, newReadCmd(), "", readRef)
	if err != nil {
		t.Fatalf("read failed: %v\nout: %s", err, outRead)
	}
	if !strings.Contains(outRead, "deploy stripe webhook handler") {
		t.Errorf("read output missing user prompt:\n%s", outRead)
	}

	// 3. Outline Journey
	outOutline, err := runCmd(t, newOutlineCmd(), "", sessID[:8])
	if err != nil {
		t.Fatalf("outline failed: %v\nout: %s", err, outOutline)
	}
	if !strings.Contains(outOutline, "deploy stripe webhook handler") {
		t.Errorf("outline output missing user prompt:\n%s", outOutline)
	}

	// 4. Resume Journey
	outResume, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--resume", sessID[:8])
	if err != nil {
		t.Fatalf("resume failed: %v\nout: %s", err, outResume)
	}
	if !strings.Contains(outResume, "agy --conversation "+sessID) {
		t.Errorf("resume output missing agy command:\n%s", outResume)
	}

	// 5. Incremental Append Journey
	appendedJSONL := initialJSONL + `{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:15Z","thinking":"complete","content":"Deployed webhook handler successfully"}` + "\n"
	if err := os.WriteFile(transPath, []byte(appendedJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	outAppend, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "Deployed webhook handler")
	if err != nil {
		t.Fatalf("search append failed: %v\nout: %s", err, outAppend)
	}
	var envAppend agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outAppend), &envAppend); err != nil {
		t.Fatalf("unmarshal append output: %v\nout: %s", err, outAppend)
	}
	if envAppend.Count < 1 {
		t.Fatalf("appended message not found; out: %s", outAppend)
	}

	// 6. Retention & Delete Journey: Remove source file and verify retained history
	if err := os.Remove(transPath); err != nil {
		t.Fatal(err)
	}
	// Run search with --reindex to trigger retention reconciliation
	outRetained, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", "deploy stripe")
	if err != nil {
		t.Fatalf("search retained failed: %v\nout: %s", err, outRetained)
	}
	if !strings.Contains(outRetained, sessID) {
		t.Errorf("retained session not found in search after file deletion:\n%s", outRetained)
	}

	// 7. Delete Session with --yes
	outDel, err := runCmd(t, newDeleteCmd(), "", "--yes", sessID[:8])
	if err != nil {
		t.Fatalf("delete failed: %v\nout: %s", err, outDel)
	}
	if !strings.Contains(outDel, "Deleted 1 session") {
		t.Errorf("delete summary missing; out: %s", outDel)
	}
}

// TestCLIJourney_CrossRuntimeDisambiguation proves that Claude, Codex, and Antigravity
// sessions can coexist in the same project directory and are correctly isolated by --source.
func TestCLIJourney_CrossRuntimeDisambiguation(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("ANTIGRAVITY_HOME", filepath.Join(cfg, ".gemini", "antigravity-cli"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	// 1. Seed Claude session
	claudeProjDir := filepath.Join(cfg, "projects", "backend")
	if err := os.MkdirAll(claudeProjDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeProjDir, "c1110001-0000-0000-0000-000000000001.jsonl"),
		[]byte(`{"type":"user","uuid":"u-claude-1","message":{"role":"user","content":"optimize database schema"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Seed Antigravity session
	agRoot := filepath.Join(cfg, ".gemini", "antigravity-cli")
	agSess := "a1110001-0000-0000-0000-000000000001"
	if err := os.MkdirAll(filepath.Join(agRoot, "brain", agSess, ".system_generated", "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agRoot, "history.jsonl"),
		[]byte(`{"conversationId":"`+agSess+`","workspace":"/workspace/backend"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agRoot, "brain", agSess, ".system_generated", "logs", "transcript.jsonl"),
		[]byte(`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>optimize database schema</USER_REQUEST>"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. Search with --source claude
	outClaude, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--source", "claude", "--json", "database schema")
	if err != nil {
		t.Fatalf("search claude: %v\nout: %s", err, outClaude)
	}
	var envClaude agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outClaude), &envClaude); err != nil {
		t.Fatalf("unmarshal claude: %v", err)
	}
	if envClaude.Count != 1 || envClaude.Results[0].SessionID != "c1110001-0000-0000-0000-000000000001" {
		t.Errorf("search --source claude got %+v, want only c1110001", envClaude.Results)
	}

	// 4. Search with --source antigravity
	outAg, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--source", "antigravity", "--json", "database schema")
	if err != nil {
		t.Fatalf("search antigravity: %v\nout: %s", err, outAg)
	}
	var envAg agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outAg), &envAg); err != nil {
		t.Fatalf("unmarshal ag: %v", err)
	}
	if envAg.Count != 1 || envAg.Results[0].SessionID != agSess {
		t.Errorf("search --source antigravity got %+v, want only %s", envAg.Results, agSess)
	}
}

// TestCLIJourney_AntigravityCurrentSessionExclusion verifies that when searching
// from inside an AGY session (ANTIGRAVITY_CONVERSATION_ID exported), the caller's
// just-typed prompt is excluded while earlier history of the same session remains
// searchable, and --current-session off restores live-turn search.
func TestCLIJourney_AntigravityCurrentSessionExclusion(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(cfg, ".claude"))
	t.Setenv("ANTIGRAVITY_HOME", filepath.Join(cfg, ".gemini", "antigravity-cli"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")

	agRoot := filepath.Join(cfg, ".gemini", "antigravity-cli")
	sessID := "a9990001-0000-0000-0000-000000000001"
	workDir := filepath.Join(cfg, "work", "inventory-service")

	// Export the live AGY conversation ID into the environment
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", sessID)

	histPath := filepath.Join(agRoot, "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(histPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(histPath, []byte(`{"conversationId":"`+sessID+`","workspace":"`+workDir+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	transPath := filepath.Join(agRoot, "brain", sessID, ".system_generated", "logs", "transcript_full.jsonl")
	if err := os.MkdirAll(filepath.Dir(transPath), 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptJSONL := strings.Join([]string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>back then we wrote the runbook for rolling back an inventory image</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:05Z","thinking":"planning migration","content":"Ran inventory migration","tool_calls":[{"name":"run_command","args":{"CommandLine":"make migrate"}}]}`,
		`{"step_index":2,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:10Z","content":"<USER_REQUEST>runbook runbook runbook what did we ever decide about the runbook</USER_REQUEST>"}`,
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:15Z","thinking":"searching runbook","content":"Checking runbook status"}`,
	}, "\n") + "\n"

	if err := os.WriteFile(transPath, []byte(transcriptJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Search without flags: ANTIGRAVITY_CONVERSATION_ID automatically excludes the just-typed turn
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", "runbook")
	if err != nil {
		t.Fatalf("search failed: %v\nout: %s", err, out)
	}
	var env agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, out)
	}
	if env.ExcludedCurrentTurn == 0 {
		t.Fatalf("expected ExcludedCurrentTurn > 0 via ANTIGRAVITY_CONVERSATION_ID, got 0")
	}
	if len(env.Results) != 1 {
		t.Fatalf("expected 1 result from earlier history, got %d: %+v", len(env.Results), env.Results)
	}
	if strings.Contains(env.Results[0].Snippet, "what did we ever decide") {
		t.Errorf("just-typed prompt was not excluded: %s", env.Results[0].Snippet)
	}
	if !strings.Contains(env.Results[0].Snippet, "rolling back an inventory image") {
		t.Errorf("earlier history not found in snippet: %s", env.Results[0].Snippet)
	}

	// 2. Search with --current-session off: live turn is returned
	outOff, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--current-session", "off", "--json", "runbook")
	if err != nil {
		t.Fatalf("search with --current-session off failed: %v\nout: %s", err, outOff)
	}
	var envOff agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outOff), &envOff); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, outOff)
	}
	if envOff.ExcludedCurrentTurn != 0 {
		t.Errorf("expected ExcludedCurrentTurn = 0 with --current-session off, got %d", envOff.ExcludedCurrentTurn)
	}
	if len(envOff.Results) < 1 || !strings.Contains(envOff.Results[0].Snippet, "what did we ever decide") {
		t.Errorf("expected just-typed prompt in results with --current-session off, got: %+v", envOff.Results)
	}
}

// TestCLIJourney_ConsolidatePrunesTombstonesWhenSourceUnchanged verifies tracker #217:
// A session is indexed and consolidated into consolidated.db. A tombstone is then
// added for that session without touching the source project or its db.
// Re-running consolidation without --rebuild must prune the tombstoned session so
// that it is no longer searchable.
func TestCLIJourney_ConsolidatePrunesTombstonesWhenSourceUnchanged(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(cfg, ".claude"))
	t.Setenv("ANTIGRAVITY_HOME", filepath.Join(cfg, ".gemini", "antigravity-cli"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(cfg, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "")

	cacheDir := store.CacheDir()
	if !strings.HasPrefix(cacheDir, cfg) {
		t.Fatalf("CRITICAL: store.CacheDir() = %q is not isolated inside temp dir %q", cacheDir, cfg)
	}

	// 1. Seed two Claude sessions in the project
	claudeProjDir := filepath.Join(cfg, ".claude", "projects", "my-project")
	if err := os.MkdirAll(claudeProjDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keepID := "c2170001-0000-0000-0000-000000000001"
	dropID := "c2170002-0000-0000-0000-000000000002"
	keepJSONL := filepath.Join(claudeProjDir, keepID+".jsonl")
	dropJSONL := filepath.Join(claudeProjDir, dropID+".jsonl")
	if err := os.WriteFile(keepJSONL, []byte(`{"type":"user","uuid":"u-keep-1","message":{"role":"user","content":"quantum cryptography protocol"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dropJSONL, []byte(`{"type":"user","uuid":"u-drop-1","message":{"role":"user","content":"quantum teleportation algorithm"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Initial search with --reindex to build project db, followed by consolidate
	outSearch1, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--reindex", "--json", "quantum")
	if err != nil {
		t.Fatalf("initial search failed: %v\nout: %s", err, outSearch1)
	}
	var env1 agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outSearch1), &env1); err != nil {
		t.Fatalf("unmarshal initial search: %v\nout: %s", err, outSearch1)
	}
	if env1.Count != 2 {
		t.Fatalf("expected 2 sessions in initial search, got %d: %+v", env1.Count, env1.Results)
	}

	outConsolidate1, err := runCmd(t, newConsolidateCmd(), "")
	if err != nil {
		t.Fatalf("initial consolidate failed: %v\nout: %s", err, outConsolidate1)
	}

	// Confirm both are searchable via consolidated.db
	outSearchConsolidated, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "quantum")
	if err != nil {
		t.Fatalf("consolidated search failed: %v\nout: %s", err, outSearchConsolidated)
	}
	var envConsolidated agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outSearchConsolidated), &envConsolidated); err != nil {
		t.Fatalf("unmarshal consolidated search: %v\nout: %s", err, outSearchConsolidated)
	}
	if envConsolidated.Count != 2 {
		t.Fatalf("expected 2 sessions in consolidated search, got %d: %+v", envConsolidated.Count, envConsolidated.Results)
	}

	// 3. Add tombstone for dropID WITHOUT touching the source project or its per-project db
	if err := lifecycle.TombstoneIDs("", []string{dropID}); err != nil {
		t.Fatalf("write tombstone: %v", err)
	}

	// 4. Re-run consolidation WITHOUT --rebuild
	outConsolidate2, err := runCmd(t, newConsolidateCmd(), "")
	if err != nil {
		t.Fatalf("second consolidate failed: %v\nout: %s", err, outConsolidate2)
	}

	// 5. Assert the deleted session is no longer searchable in consolidated.db, but keepID is
	outSearchAfterDrop, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "teleportation")
	if err != nil {
		t.Fatalf("search for dropped session failed: %v\nout: %s", err, outSearchAfterDrop)
	}
	var envAfterDrop agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outSearchAfterDrop), &envAfterDrop); err != nil {
		t.Fatalf("unmarshal search after drop: %v\nout: %s", err, outSearchAfterDrop)
	}
	for _, res := range envAfterDrop.Results {
		if res.SessionID == dropID {
			t.Fatalf("deleted session %s is still searchable in consolidated store after consolidate without --rebuild: %+v", dropID, res)
		}
	}
	if envAfterDrop.Count != 0 {
		t.Fatalf("expected 0 results for deleted session, got count %d: %+v", envAfterDrop.Count, envAfterDrop.Results)
	}

	outSearchAfterKeep, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--json", "cryptography")
	if err != nil {
		t.Fatalf("search for kept session failed: %v\nout: %s", err, outSearchAfterKeep)
	}
	var envAfterKeep agentproto.SearchEnvelope
	if err := json.Unmarshal([]byte(outSearchAfterKeep), &envAfterKeep); err != nil {
		t.Fatalf("unmarshal search after keep: %v\nout: %s", err, outSearchAfterKeep)
	}
	if envAfterKeep.Count != 1 || len(envAfterKeep.Results) == 0 || envAfterKeep.Results[0].SessionID != keepID {
		t.Fatalf("expected kept session %s in search, got count %d: %+v", keepID, envAfterKeep.Count, envAfterKeep.Results)
	}
}
