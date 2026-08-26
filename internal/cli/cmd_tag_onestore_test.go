package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// writeTranscript writes a Claude-shaped transcript whose filename stem is the
// session id, one user message per uuid.
func writeTranscript(t *testing.T, proj, sid string, uuids []string) {
	t.Helper()
	var b strings.Builder
	for _, uuid := range uuids {
		b.WriteString(`{"type":"user","uuid":"` + uuid + `","timestamp":"2026-06-01T10:00:00Z",` +
			`"message":{"role":"user","content":"a message worth tagging"}}` + "\n")
	}
	path := filepath.Join(proj, sid+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestTagWriteUsesCatalogBeforeCorpusSweep reproduces a deferred fold: the
// target is indexed in its project db and cataloged, but is absent from the
// consolidated store while unrelated project dbs are also present.
func TestTagWriteUsesCatalogBeforeCorpusSweep(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	catalogDir := filepath.Join(configDir, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catalogDir)

	projectsRoot := filepath.Join(configDir, "projects")
	target := filepath.Join(projectsRoot, "target-project")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target project: %v", err)
	}
	const sid = "7b2c4d6e-0000-4000-8000-0000000000aa"
	const firstUUID = "44444444-aaaa-bbbb-cccc-000000000031"
	writeTranscript(t, target, sid, []string{firstUUID})
	_, _, _, err := index.EnsureIndexed(target, false)
	if err != nil {
		t.Fatalf("EnsureIndexed target: %v", err)
	}
	if err := paths.WriteCatalogEntry(catalogDir, paths.CatalogEntry{
		SessionID:      sid,
		TranscriptPath: filepath.Join(target, sid+".jsonl"),
		CWD:            target,
	}); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}
	for i := range 12 {
		proj := filepath.Join(projectsRoot, "unrelated-"+string(rune('a'+i)))
		if err := os.MkdirAll(proj, 0o755); err != nil {
			t.Fatalf("mkdir unrelated project: %v", err)
		}
		writeTranscript(t, proj, "aaaaaaaa-0000-4000-8000-0000000000aa", []string{"55555555-aaaa-bbbb-cccc-000000000031"})
		if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
			t.Fatalf("EnsureIndexed unrelated %d: %v", i, err)
		}
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(index.ConsolidatedPath() + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove consolidated store: %v", err)
		}
	}

	called := false
	more := func() []view.Scope {
		called = true
		return scopes.All(context.Background(), "claude", false)
	}
	jsonIn := `[{"start_uuid":"` + firstUUID[:8] + `","topic":"catalog guard","summary":"deferred fold"}]`
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(jsonIn), sid[:8], nil, more, false, "", false); err != nil {
		t.Fatalf("runTagWriteCmd: %v", err)
	}
	if called {
		t.Fatal("tag-write built the eager all-project scope list despite a catalog hit")
	}
}

func TestGuardedSessionLookupDoesNotTreatForeignCatalogPathAsClaude(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	catalogDir := filepath.Join(configDir, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catalogDir)

	foreignDir := t.TempDir()
	const sid = "8c3d5e7f-0000-4000-8000-0000000000bb"
	transcriptPath := filepath.Join(foreignDir, sid+".jsonl")
	writeTranscript(t, foreignDir, sid, []string{"66666666-aaaa-bbbb-cccc-000000000041"})
	if err := paths.WriteCatalogEntry(catalogDir, paths.CatalogEntry{
		SessionID:      sid,
		TranscriptPath: transcriptPath,
		CWD:            foreignDir,
		Source:         "codex",
	}); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	called := false
	more := func() []view.Scope {
		called = true
		return nil
	}
	if _, _, err := agentproto.LocateSessionGuarded(sid[:8], nil, more); err == nil {
		t.Fatal("foreign catalog path resolved through the Claude scope")
	}
	if !called {
		t.Fatal("foreign catalog path bypassed the source-aware fallback")
	}
}

func TestGuardedSessionLookupPreservesMixedSourceAmbiguity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	catalogDir := filepath.Join(configDir, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catalogDir)

	const prefix = "8c3d5e7f"
	claudeDir := filepath.Join(configDir, "projects", "mixed-source")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudeSID := prefix + "-0000-4000-8000-0000000000aa"
	writeTranscript(t, claudeDir, claudeSID, []string{"66666666-aaaa-bbbb-cccc-000000000042"})
	if err := paths.WriteCatalogEntry(catalogDir, paths.CatalogEntry{
		SessionID:      claudeSID,
		TranscriptPath: filepath.Join(claudeDir, claudeSID+".jsonl"),
		CWD:            claudeDir,
		Source:         "claude",
	}); err != nil {
		t.Fatalf("write Claude catalog entry: %v", err)
	}

	foreignDir := t.TempDir()
	foreignSID := prefix + "-0000-4000-8000-0000000000bb"
	writeTranscript(t, foreignDir, foreignSID, []string{"66666666-aaaa-bbbb-cccc-000000000043"})
	if err := paths.WriteCatalogEntry(catalogDir, paths.CatalogEntry{
		SessionID:      foreignSID,
		TranscriptPath: filepath.Join(foreignDir, foreignSID+".jsonl"),
		CWD:            foreignDir,
		Source:         "codex",
	}); err != nil {
		t.Fatalf("write Codex catalog entry: %v", err)
	}

	claudeDB, claudeDBP := storetest.NewDB(t)
	storetest.InsertSession(t, claudeDB, storetest.Session{
		ID:         claudeSID,
		Project:    "mixed-source-claude",
		SourceTool: "claude",
	})
	codexDB, codexDBP := storetest.NewDB(t)
	storetest.InsertSession(t, codexDB, storetest.Session{
		ID:         foreignSID,
		Project:    "mixed-source-codex",
		SourceTool: "codex",
	})

	called := false
	more := func() []view.Scope {
		called = true
		return []view.Scope{
			{Project: "mixed-source-claude", DBP: claudeDBP, Source: "claude"},
			{Project: "mixed-source-codex", DBP: codexDBP, Source: "codex"},
		}
	}
	_, _, err := agentproto.LocateSessionGuarded(prefix, nil, more)
	var ambiguous *agentproto.ErrAmbiguousSession
	if !errors.As(err, &ambiguous) {
		t.Fatalf("LocateSessionGuarded error = %v, want ErrAmbiguousSession", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguous candidates = %d, want 2: %v", len(ambiguous.Candidates), ambiguous.Candidates)
	}
	gotIDs := map[string]bool{}
	for _, candidate := range ambiguous.Candidates {
		gotIDs[candidate.SessionID] = true
	}
	for _, wantID := range []string{claudeSID, foreignSID} {
		if !gotIDs[wantID] {
			t.Errorf("ambiguous candidates = %v, missing %q", gotIDs, wantID)
		}
	}
	if !called {
		t.Fatal("mixed-source prefix bypassed the source-aware fallback")
	}
}

func TestGuardedSessionLookupUsesForeignPreResolvedScope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	catalogDir := filepath.Join(configDir, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catalogDir)

	foreignDir := t.TempDir()
	const sid = "9d4e6f80-0000-4000-8000-0000000000cc"
	transcriptPath := filepath.Join(foreignDir, sid+".jsonl")
	writeTranscript(t, foreignDir, sid, []string{"77777777-aaaa-bbbb-cccc-000000000051"})
	if err := paths.WriteCatalogEntry(catalogDir, paths.CatalogEntry{
		SessionID:      sid,
		TranscriptPath: transcriptPath,
		CWD:            foreignDir,
		Source:         "codex",
	}); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	// Seed the source-aware fallback's pre-resolved Codex scope. If catalog
	// narrowing incorrectly treats the foreign path as Claude, it will resolve
	// a different db from foreignDir before this scope is consulted.
	wantDBP := index.DBPath("codex-pre-resolved")
	con, err := store.ConnectRW(wantDBP)
	if err != nil {
		t.Fatalf("ConnectRW: %v", err)
	}
	if err := index.EnsureSchema(con, "codex"); err != nil {
		con.Close()
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := con.Exec(
		`INSERT INTO sessions(id,message_count,is_subagent,source_tool,project,cwd)
		 VALUES(?,1,0,'codex',?,?)`, sid, filepath.Base(foreignDir), foreignDir,
	); err != nil {
		con.Close()
		t.Fatalf("seed Codex session: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close Codex db: %v", err)
	}

	called := false
	more := func() []view.Scope {
		called = true
		return []view.Scope{{
			Project: filepath.Base(foreignDir),
			DBP:     wantDBP,
			CWD:     foreignDir,
			Source:  "codex",
		}}
	}
	gotDBP, gotSID, err := agentproto.LocateSessionGuarded(sid[:8], nil, more)
	if err != nil {
		t.Fatalf("LocateSessionGuarded: %v", err)
	}
	if !called {
		t.Fatal("foreign catalog path did not reach the source-aware fallback")
	}
	if gotDBP != wantDBP || gotSID != sid {
		t.Fatalf("LocateSessionGuarded = (%q, %q), want (%q, %q)", gotDBP, gotSID, wantDBP, sid)
	}
}

// TestTagWriteLandsInTheOneStoreAndReadsBack walks the whole tag round trip the
// way a user does: write a tag with `tag-write`, then read it back through the
// normal read path (`outline`). Both ends resolve the session the same way, so
// the tag lands in the ONE store and is found there — there is no per-project
// copy for a write to fall into and never be read from again.
func TestTagWriteLandsInTheOneStoreAndReadsBack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	const sid = "3a7f21b0-0000-4000-8000-0000000000ab"
	const firstUUID = "11111111-aaaa-bbbb-cccc-000000000011"
	writeTranscript(t, proj, sid, []string{firstUUID, "22222222-aaaa-bbbb-cccc-000000000012"})

	projDB, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{projDB}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	const topic = "one store session lookup"
	jsonIn := `[{"start_uuid":"` + firstUUID[:8] + `","topic":"` + topic + `","summary":"the round trip under test"}]`

	var out strings.Builder
	publishDone := make(chan error, 1)
	oldPublish := spawnTagPublish
	spawnTagPublish = func(dbp, sessionID string) error {
		go func() { publishDone <- runTagPublishChild(context.Background(), io.Discard, dbp, sessionID) }()
		return nil
	}
	t.Cleanup(func() { spawnTagPublish = oldPublish })
	if err := runTagWriteCmd(&out, strings.NewReader(jsonIn), sid[:8], scope, nil, false, "", false); err != nil {
		t.Fatalf("runTagWriteCmd: %v", err)
	}
	select {
	case err := <-publishDone:
		if err != nil {
			t.Fatalf("tag publication: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consolidated publisher did not finish")
	}

	// The read path must see it after the deterministic detached publisher has
	// completed. This is the whole point: a tag written to a db the readers
	// never open is a tag that does not exist.
	res, err := agentproto.Outline(sid[:8], scope, false)
	if err != nil {
		t.Fatalf("Outline: %v", err)
	}
	found := false
	for _, got := range res.Topics {
		if got == topic {
			found = true
		}
	}
	if !found {
		t.Errorf("outline topics = %v, want the tag just written (%q)", res.Topics, topic)
	}

	// And it is physically in the one store, not stranded in the project db.
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()
	ids, err := store.TaggedSessionIDs(con)
	if err != nil {
		t.Fatalf("TaggedSessionIDs(consolidated): %v", err)
	}
	if len(ids) != 1 || ids[0] != sid {
		t.Errorf("tagged sessions in the consolidated store = %v, want [%s]", ids, sid)
	}
}

// TestLocalTagExportIncludesTheOneStore pins that a tag authored through the
// normal path still reaches the archive. Export enumerates local scopes, and
// the consolidated store is not one of them by discovery — it has to be listed
// deliberately, or every newly written tag would stay on this machine.
func TestLocalTagExportIncludesTheOneStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	const sid = "6d0c88f4-0000-4000-8000-0000000000cd"
	const firstUUID = "33333333-aaaa-bbbb-cccc-000000000021"
	writeTranscript(t, proj, sid, []string{firstUUID})

	projDB, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{projDB}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	jsonIn := `[{"start_uuid":"` + firstUUID[:8] + `","topic":"archive round trip","summary":"exported or lost"}]`
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(jsonIn), sid[:8], scope, nil, false, "", false); err != nil {
		t.Fatalf("runTagWriteCmd: %v", err)
	}

	files, err := localTagExporter()()
	if err != nil {
		t.Fatalf("localTagExporter: %v", err)
	}
	for _, f := range files {
		if f.SessionID == sid {
			return
		}
	}
	t.Errorf("the exporter offered %d tag files, none for the session just tagged (%s)", len(files), sid)
}
