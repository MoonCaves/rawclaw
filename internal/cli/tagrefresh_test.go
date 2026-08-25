package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/claude"
	"github.com/MoonCaves/rawclaw/internal/store"
)

type tagTestSource struct {
	containers    []source.Container
	messages      []model.Message
	discoverErr   error
	messagesErr   error
	discoverCalls int
	messagesCalls int
}

func (s *tagTestSource) Discover() ([]source.Container, error) {
	s.discoverCalls++
	return append([]source.Container(nil), s.containers...), s.discoverErr
}

func (s *tagTestSource) Messages(source.Container) ([]model.Message, error) {
	s.messagesCalls++
	return append([]model.Message(nil), s.messages...), s.messagesErr
}

func tagTestRegistration(id string, src *tagTestSource) source.Registration {
	return source.Registration{
		ID:  id,
		New: func() source.Source { return src },
	}
}

func writeTagSourceFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
}

func seedTagSession(
	t *testing.T,
	reg source.Registration,
	c source.Container,
) {
	t.Helper()
	dbp := filepath.Join(store.CacheDir(), "tag-prep-seed.db")
	if _, _, err := index.EnsureIndexedContainers(
		dbp,
		false,
		[]source.Container{c},
		reg.New().Messages,
		reg.ID,
		"",
	); err != nil {
		t.Fatalf("seed stale session: %v", err)
	}
}

func TestRunTagPrepCmdRefreshesRegisteredSourceWithoutCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(t.TempDir(), "future-runtime-session.log")
	writeTagSourceFile(t, path, "old")
	const sid = "future-runtime-session-0001"
	c := source.Container{ID: sid, Path: path, CWD: "/not/the/process/cwd"}
	src := &tagTestSource{
		containers: []source.Container{c},
		messages: []model.Message{
			{Role: "user", Text: "old indexed message", UUID: "11111111-old"},
		},
	}
	reg := tagTestRegistration("future-runtime", src)
	seedTagSession(t, reg, c)

	writeTagSourceFile(t, path, "old\nnew")
	src.messages = append(src.messages, model.Message{
		Role: "assistant", Text: "new live message", UUID: "22222222-new",
	})

	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, sid, nil, nil, []source.Registration{reg}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}
	if !strings.Contains(out.String(), "22222222 [assistant] new live message") {
		t.Fatalf("tag-prep returned a stale dump:\n%s", out.String())
	}
	if src.discoverCalls != 0 {
		t.Fatalf("Discover called %d times for an already located session, want 0", src.discoverCalls)
	}
}

func TestRunTagPrepCmdDiscoversUnindexedRegisteredSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(t.TempDir(), "brand-new-runtime-session.log")
	writeTagSourceFile(t, path, "live")
	const sid = "brand-new-runtime-session-0001"
	c := source.Container{ID: sid, Path: path, CWD: "/some/other/project"}
	src := &tagTestSource{
		containers: []source.Container{c},
		messages: []model.Message{
			{Role: "user", Text: "found through the registry", UUID: "33333333-new"},
		},
	}
	reg := tagTestRegistration("brand-new-runtime", src)

	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, sid, nil, nil, []source.Registration{reg}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}
	if !strings.Contains(out.String(), "33333333 [user] found through the registry") {
		t.Fatalf("tag-prep did not index the new registered source:\n%s", out.String())
	}
	if src.discoverCalls != 1 {
		t.Fatalf("Discover called %d times, want 1", src.discoverCalls)
	}
}

func TestRunTagPrepCmdExactIDDoesNotRefreshPrefixedSubagents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	const sid = "exact-runtime-session-0001"
	parentPath := filepath.Join(dir, "parent.log")
	childPath := filepath.Join(dir, "child.log")
	writeTagSourceFile(t, parentPath, "parent")
	writeTagSourceFile(t, childPath, "child")
	src := &tagTestSource{
		containers: []source.Container{
			{ID: sid, Path: parentPath},
			{ID: sid + "/subagent", Path: childPath, IsSubagent: true, ParentID: sid},
		},
		messages: []model.Message{
			{Role: "user", Text: "only the exact session", UUID: "66666666-one"},
		},
	}
	reg := tagTestRegistration("exact-runtime", src)

	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, sid, nil, nil, []source.Registration{reg}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}
	if src.messagesCalls != 1 {
		t.Fatalf("Messages called %d times, want only the exact session", src.messagesCalls)
	}
}

func TestRunTagPrepCmdPrefixPrefersRootOverSubagents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	const sid = "prefix-runtime-session-0001"
	parentPath := filepath.Join(dir, "parent.log")
	childPath := filepath.Join(dir, "child.log")
	writeTagSourceFile(t, parentPath, "parent")
	writeTagSourceFile(t, childPath, "child")
	src := &tagTestSource{
		containers: []source.Container{
			{ID: sid, Path: parentPath},
			{ID: sid + "/subagent", Path: childPath, IsSubagent: true, ParentID: sid},
		},
		messages: []model.Message{
			{Role: "user", Text: "root session message", UUID: "77777777-root"},
		},
	}
	reg := tagTestRegistration("prefix-runtime", src)

	var out strings.Builder
	// Pass an 8-char prefix rather than full session ID
	if err := runTagPrepCmdWithSources(&out, sid[:8], nil, nil, []source.Registration{reg}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}
	if src.messagesCalls != 1 {
		t.Fatalf("Messages called %d times, want only root session", src.messagesCalls)
	}
	if !strings.Contains(out.String(), "77777777 [user] root session message") {
		t.Fatalf("tag-prep output missing root session:\n%s", out.String())
	}
}

func TestRunTagPrepCmdDoesNotReturnStaleDataAfterRefreshFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(t.TempDir(), "failing-runtime-session.log")
	writeTagSourceFile(t, path, "old")
	const sid = "failing-runtime-session-0001"
	c := source.Container{ID: sid, Path: path}
	src := &tagTestSource{
		containers: []source.Container{c},
		messages: []model.Message{
			{Role: "user", Text: "stale data must not print", UUID: "44444444-old"},
		},
	}
	reg := tagTestRegistration("failing-runtime", src)
	seedTagSession(t, reg, c)

	writeTagSourceFile(t, path, "old\nnew")
	src.messagesErr = errors.New("live transcript unreadable")

	var out strings.Builder
	err := runTagPrepCmdWithSources(&out, sid, nil, nil, []source.Registration{reg})
	if err == nil {
		t.Fatal("expected live refresh failure")
	}
	if !strings.Contains(err.Error(), "live transcript unreadable") {
		t.Fatalf("error = %q, want the source failure", err)
	}
	if out.Len() != 0 {
		t.Fatalf("tag-prep printed stale data after refresh failed:\n%s", out.String())
	}
}

func TestRunTagPrepCmdReusesUnchangedRefreshWatermark(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(t.TempDir(), "unchanged-runtime-session.log")
	writeTagSourceFile(t, path, "unchanged")
	const sid = "unchanged-runtime-session-0001"
	c := source.Container{ID: sid, Path: path}
	src := &tagTestSource{
		containers: []source.Container{c},
		messages: []model.Message{
			{Role: "user", Text: "only parsed once", UUID: "55555555-one"},
		},
	}
	reg := tagTestRegistration("unchanged-runtime", src)
	seedTagSession(t, reg, c)
	seedCalls := src.messagesCalls

	for range 2 {
		var out strings.Builder
		if err := runTagPrepCmdWithSources(&out, sid, nil, nil, []source.Registration{reg}); err != nil {
			t.Fatalf("runTagPrepCmdWithSources: %v", err)
		}
	}
	if got := src.messagesCalls - seedCalls; got != 1 {
		t.Fatalf("refresh parsed unchanged source %d times, want 1", got)
	}
}

func TestRunTagPrepCmdResolvesTopLevelClaudeByFilenameStemWithoutDiscovery(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)

	projDir := filepath.Join(configDir, "projects", "-test-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}

	const fullSID = "a1b2c3d4-full-session-uuid-0001"
	transcriptPath := filepath.Join(projDir, fullSID+".jsonl")
	transcriptContent := `{"type":"user","uuid":"11111111-uuid","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"stem resolved message"}}` + "\n" +
		`{"type":"assistant","uuid":"22222222-uuid","timestamp":"2026-06-01T10:00:01Z","message":{"role":"assistant","content":"stem resolved response"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	otherSrc := &tagTestSource{
		containers: []source.Container{},
		messages:   []model.Message{},
	}
	otherReg := tagTestRegistration("other-runtime", otherSrc)
	claudeReg := claude.Registration()

	var out strings.Builder
	// Pass 8-character prefix "a1b2c3d4"
	if err := runTagPrepCmdWithSources(&out, "a1b2c3d4", nil, nil, []source.Registration{claudeReg, otherReg}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}

	if otherSrc.discoverCalls != 0 {
		t.Fatalf("other runtime Discover called %d times, want 0 (should resolve via stem)", otherSrc.discoverCalls)
	}
	if !strings.Contains(out.String(), "11111111 [user] stem resolved message") {
		t.Fatalf("tag-prep output missing expected user message:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "22222222 [assistant] stem resolved response") {
		t.Fatalf("tag-prep output missing expected assistant message:\n%s", out.String())
	}
}

func TestRunTagPrepCmdSubagentFallsThroughToDiscovery(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)

	projDir := filepath.Join(configDir, "projects", "-test-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}
	parentPath := filepath.Join(projDir, "parent-session-0001.jsonl")
	writeTagSourceFile(t, parentPath, `{"type":"user","uuid":"11111111-uuid","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"parent"}}`+"\n")

	subagentDir := filepath.Join(projDir, "subagents")
	if err := os.MkdirAll(subagentDir, 0o755); err != nil {
		t.Fatalf("mkdir subagentDir: %v", err)
	}

	const subSID = "c1d2e3f4-subagent-uuid"
	transcriptPath := filepath.Join(subagentDir, subSID+".jsonl")
	transcriptContent := `{"type":"user","uuid":"33333333-uuid","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"subagent message"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	otherSrc := &tagTestSource{
		containers: []source.Container{},
		messages:   []model.Message{},
	}
	otherReg := tagTestRegistration("other-runtime", otherSrc)
	claudeReg := claude.Registration()

	var out strings.Builder
	// Subagent session ID is "subagents/c1d2e3f4-subagent-uuid"
	if err := runTagPrepCmdWithSources(&out, "subagents/c1d2e3f4", nil, nil, []source.Registration{claudeReg, otherReg}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}

	// Subagent is skipped by stem resolver and falls through to full discovery
	if otherSrc.discoverCalls != 1 {
		t.Fatalf("other runtime Discover called %d times, want 1 (subagent falls through to discovery)", otherSrc.discoverCalls)
	}
	if !strings.Contains(out.String(), "33333333 [user] subagent message") {
		t.Fatalf("tag-prep output missing subagent message:\n%s", out.String())
	}
}

func TestRunTagPrepCmdNeverIndexedSessionEndToEnd(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)

	projDir := filepath.Join(configDir, "projects", "-e2e-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}

	const fullSID = "e2e00001-unindexed-session-uuid"
	transcriptPath := filepath.Join(projDir, fullSID+".jsonl")
	transcriptContent := `{"type":"user","uuid":"44444444-uuid","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"unindexed session message"}}` + "\n" +
		`{"type":"assistant","uuid":"55555555-uuid","timestamp":"2026-06-01T10:00:01Z","message":{"role":"assistant","content":"unindexed session reply"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	var out strings.Builder
	// Call default runTagPrepCmd which uses default sources.Registered()
	if err := runTagPrepCmd(&out, "e2e00001", nil, nil); err != nil {
		t.Fatalf("runTagPrepCmd failed for never-indexed session: %v", err)
	}

	if !strings.Contains(out.String(), "44444444 [user] unindexed session message") {
		t.Fatalf("tag-prep output missing user message:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "55555555 [assistant] unindexed session reply") {
		t.Fatalf("tag-prep output missing assistant message:\n%s", out.String())
	}

	// Verify session is now indexed in consolidated store
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("connect consolidated store: %v", err)
	}
	defer con.Close()

	var count int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", fullSID).Scan(&count); err != nil {
		t.Fatalf("query consolidated store: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row in consolidated store for %s, got %d", fullSID, count)
	}
}

func TestRunTagPrepCmdResolvesCatalogSessionWithoutDiscovery(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)

	catDir := filepath.Join(configDir, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)

	transcriptDir := filepath.Join(configDir, "transcripts")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatalf("mkdir transcriptDir: %v", err)
	}

	const fullSID = "b1c2d3e4-catalog-session-uuid"
	transcriptPath := filepath.Join(transcriptDir, fullSID+".jsonl")
	transcriptContent := `{"type":"user","uuid":"aaaa1111-uuid","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"catalog born message"}}` + "\n" +
		`{"type":"assistant","uuid":"bbbb2222-uuid","timestamp":"2026-06-01T10:00:01Z","message":{"role":"assistant","content":"catalog born reply"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	// Write catalog entry simulating hook birth
	if err := paths.WriteCatalogEntry(catDir, paths.CatalogEntry{
		SessionID:      fullSID,
		TranscriptPath: transcriptPath,
		CWD:            "/home/user/catalog-proj",
		Source:         "claude",
	}); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	otherSrc := &tagTestSource{
		containers: []source.Container{},
		messages:   []model.Message{},
	}
	otherReg := tagTestRegistration("other-runtime", otherSrc)
	claudeReg := claude.Registration()

	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, "b1c2d3e4", nil, nil, []source.Registration{claudeReg, otherReg}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}

	if otherSrc.discoverCalls != 0 {
		t.Fatalf("other runtime Discover called %d times, want 0 (should resolve via catalog)", otherSrc.discoverCalls)
	}
	if !strings.Contains(out.String(), "aaaa1111 [user] catalog born message") {
		t.Fatalf("tag-prep output missing user message:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "bbbb2222 [assistant] catalog born reply") {
		t.Fatalf("tag-prep output missing assistant message:\n%s", out.String())
	}
}

func TestRunTagPrepCmdStaleCatalogEntryFallsThrough(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)

	catDir := filepath.Join(configDir, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)

	// Write a stale catalog entry pointing to non-existent transcript
	_ = paths.WriteCatalogEntry(catDir, paths.CatalogEntry{
		SessionID:      "stale001-deleted-session-uuid",
		TranscriptPath: filepath.Join(configDir, "does-not-exist.jsonl"),
		CWD:            "/home/user/stale",
		Source:         "claude",
	})

	// Put actual session in Claude projects dir (stem fallback)
	projDir := filepath.Join(configDir, "projects", "-fallback-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}
	const fullSID = "stale001-fallback-session-uuid"
	transcriptPath := filepath.Join(projDir, fullSID+".jsonl")
	transcriptContent := `{"type":"user","uuid":"cccc3333-uuid","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"fallback session message"}}` + "\n" +
		`{"type":"assistant","uuid":"dddd4444-uuid","timestamp":"2026-06-01T10:00:01Z","message":{"role":"assistant","content":"fallback session reply"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	var out strings.Builder
	if err := runTagPrepCmd(&out, "stale001", nil, nil); err != nil {
		t.Fatalf("runTagPrepCmd failed with stale catalog entry: %v", err)
	}
	if !strings.Contains(out.String(), "cccc3333 [user] fallback session message") {
		t.Fatalf("tag-prep output missing user message:\n%s", out.String())
	}
}

func TestRunResumeResolvesCatalogSession(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)

	catDir := filepath.Join(configDir, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catDir)

	transcriptDir := filepath.Join(configDir, "transcripts")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatalf("mkdir transcriptDir: %v", err)
	}

	const fullSID = "resume01-catalog-session-uuid"
	transcriptPath := filepath.Join(transcriptDir, fullSID+".jsonl")
	writeTagSourceFile(t, transcriptPath, `{"cwd":"/home/user/resume-proj"}`+"\n")

	if err := paths.WriteCatalogEntry(catDir, paths.CatalogEntry{
		SessionID:      fullSID,
		TranscriptPath: transcriptPath,
		CWD:            "/home/user/resume-proj",
		Source:         "claude",
	}); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	var out strings.Builder
	opts := &Options{Resume: "resume01"}
	if err := runResume(&out, opts); err != nil {
		t.Fatalf("runResume: %v", err)
	}

	if !strings.Contains(out.String(), "claude --resume resume01-catalog-session-uuid") {
		t.Fatalf("runResume output missing expected resume command:\n%s", out.String())
	}
}

func TestRunTagPrepCmd_ContentionDefersFoldAndFoldsOnNextTouch(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)

	projDir := filepath.Join(configDir, "projects", "-contention-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}

	const fullSID = "contend1-session-uuid"
	transcriptPath := filepath.Join(projDir, fullSID+".jsonl")
	transcriptContent := `{"type":"user","uuid":"11112222-uuid","timestamp":"2026-08-25T10:00:00Z","message":{"role":"user","content":"busy lock contention message"}}` + "\n" +
		`{"type":"assistant","uuid":"33334444-uuid","timestamp":"2026-08-25T10:00:01Z","message":{"role":"assistant","content":"busy lock response"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	// 1. Ensure the consolidated store exists and hold BEGIN IMMEDIATE on it
	consolidatedPath := index.ConsolidatedPath()
	_ = os.MkdirAll(filepath.Dir(consolidatedPath), 0o755)
	conConsolidated, err := store.ConnectRW(consolidatedPath)
	if err != nil {
		t.Fatalf("ConnectRW consolidated: %v", err)
	}
	if err := store.Rebuild(conConsolidated); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if err := store.EnsureTopicSchema(conConsolidated); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if _, err := conConsolidated.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("BEGIN IMMEDIATE on consolidated store: %v", err)
	}

	// Capture stderr
	var errOut strings.Builder
	oldStderr := tagPrepStderr
	tagPrepStderr = &errOut
	defer func() { tagPrepStderr = oldStderr }()

	// (a) Run tag-prep while consolidated store is locked
	var out strings.Builder
	err = runTagPrepCmd(&out, "contend1", nil, nil)
	if err != nil {
		t.Fatalf("runTagPrepCmd under held lock failed, want exit 0: %v", err)
	}

	// Dump must succeed and contain messages
	if !strings.Contains(out.String(), "11112222 [user] busy lock contention message") {
		t.Fatalf("dump missing user message; got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "33334444 [assistant] busy lock response") {
		t.Fatalf("dump missing assistant reply; got:\n%s", out.String())
	}

	// Stderr must contain the deferred note
	wantNote := "# fold deferred (store busy); refresh db retained, will fold on next ingest"
	if !strings.Contains(errOut.String(), wantNote) {
		t.Fatalf("stderr = %q, want containing %q", errOut.String(), wantNote)
	}

	// Refresh DB must be retained on disk
	reg := claude.Registration()
	dbp := index.RefreshDBPath(reg.ID, fullSID, transcriptPath)
	if _, err := os.Stat(dbp); err != nil {
		t.Fatalf("refresh db %s not found on disk, want retained: %v", dbp, err)
	}

	// Release lock on consolidated store
	if _, err := conConsolidated.Exec("ROLLBACK"); err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}
	conConsolidated.Close()

	// Verify session is not yet in consolidated store
	roCon, err := store.ConnectRO(consolidatedPath)
	if err != nil {
		t.Fatalf("ConnectRO: %v", err)
	}
	var count int
	_ = roCon.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", fullSID).Scan(&count)
	roCon.Close()
	if count != 0 {
		t.Fatalf("session already in consolidated store before fold, count = %d, want 0", count)
	}

	// (b) Deferred refresh db folds on the next EnsureFreshContainer run
	c := source.Container{
		ID:   fullSID,
		Path: transcriptPath,
		CWD:  projDir,
	}
	adapter := reg.New()
	n, err := index.EnsureFreshContainer(dbp, c, adapter.Messages, reg.ID)
	if err != nil {
		t.Fatalf("EnsureFreshContainer on next touch: %v", err)
	}
	if n != 2 {
		t.Fatalf("EnsureFreshContainer n = %d, want 2", n)
	}

	// Verify session is now folded in consolidated store
	roCon, err = store.ConnectRO(consolidatedPath)
	if err != nil {
		t.Fatalf("ConnectRO: %v", err)
	}
	defer roCon.Close()
	if err := roCon.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", fullSID).Scan(&count); err != nil {
		t.Fatalf("query consolidated store: %v", err)
	}
	if count != 1 {
		t.Fatalf("session not folded into consolidated store, count = %d, want 1", count)
	}
}

func TestRunTagPrepCmdGooseOptedOutServesIndexedCopy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("RAWCLAW_GOOSE", "")

	path := filepath.Join(t.TempDir(), "goose", "sessions.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir goose path: %v", err)
	}
	writeTagSourceFile(t, path, "indexed")
	const sid = "goose-optout-session-0001"
	c := source.Container{ID: sid, Path: path}
	src := &tagTestSource{
		containers: []source.Container{c},
		messages:   []model.Message{{Role: "user", Text: "indexed copy", UUID: "55555555-copy"}},
	}
	reg := tagTestRegistration("goose", src)
	seedTagSession(t, reg, c)
	src.messages = []model.Message{{Role: "user", Text: "live should not be read", UUID: "66666666-live"}}

	var errOut strings.Builder
	oldStderr := tagPrepStderr
	tagPrepStderr = &errOut
	defer func() { tagPrepStderr = oldStderr }()

	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, sid, nil, nil, []source.Registration{reg}); err != nil {
		t.Fatalf("tag-prep returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "goose is opted out — serving indexed copy; set RAWCLAW_GOOSE=1 to refresh") {
		t.Fatalf("stderr = %q, want goose opt-out message", errOut.String())
	}
	if !strings.Contains(out.String(), "55555555 [user] indexed copy") {
		t.Fatalf("output did not serve indexed copy:\n%s", out.String())
	}
	if strings.Contains(out.String(), "66666666-live") {
		t.Fatalf("output refreshed opted-out goose session:\n%s", out.String())
	}
}

// TestRunTagPrepCmd_NonBusyFoldFailureIsReportedNotSwallowed proves a fold
// error that is NOT store-contention (index.IsBusy) still surfaces on
// stderr instead of vanishing silently — the dump itself already succeeded
// and stdout is honest, but a genuine fold failure (as opposed to the
// expected "try again later" busy case) must not print nothing.
func TestRunTagPrepCmd_NonBusyFoldFailureIsReportedNotSwallowed(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)

	projDir := filepath.Join(configDir, "projects", "-foldfail-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}

	const fullSID = "foldfail-session-uuid"
	transcriptPath := filepath.Join(projDir, fullSID+".jsonl")
	transcriptContent := `{"type":"user","uuid":"77778888-uuid","timestamp":"2026-08-25T10:00:00Z","message":{"role":"user","content":"fold failure message"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	// Make the consolidated store path a DIRECTORY: a real, unmocked way to
	// force store.ConnectRW(ConsolidatedPath()) to fail with something that is
	// neither a SQLite busy/locked string nor a fence-timeout — a genuine
	// non-busy fold error.
	consolidatedPath := index.ConsolidatedPath()
	if err := os.MkdirAll(consolidatedPath, 0o755); err != nil {
		t.Fatalf("mkdir consolidatedPath (as a directory, not a file): %v", err)
	}

	var errOut strings.Builder
	oldStderr := tagPrepStderr
	tagPrepStderr = &errOut
	defer func() { tagPrepStderr = oldStderr }()

	var out strings.Builder
	if err := runTagPrepCmd(&out, "foldfa", nil, nil); err != nil {
		t.Fatalf("runTagPrepCmd failed, want exit 0 (the dump itself must still succeed): %v", err)
	}

	if !strings.Contains(out.String(), "77778888 [user] fold failure message") {
		t.Fatalf("dump missing message; got:\n%s", out.String())
	}

	if strings.Contains(errOut.String(), "fold deferred (store busy)") {
		t.Fatalf("stderr wrongly reported a busy-deferral for a non-busy error:\n%s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "fold failed") {
		t.Fatalf("stderr = %q, want a non-silent \"fold failed\" note for the non-busy error", errOut.String())
	}
}
