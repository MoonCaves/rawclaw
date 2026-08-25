package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/model"
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
