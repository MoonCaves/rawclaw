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

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/claude"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
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

func runPrewarmTest(t *testing.T, sourceBody string, messages []model.Message) (string, *tagTestSource, source.Container) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeTagSourceFile(t, path, sourceBody)
	sid := "prewarm-session-0001"
	c := source.Container{ID: sid, Path: path, CWD: t.TempDir()}
	src := &tagTestSource{containers: []source.Container{c}, messages: messages}
	reg := tagTestRegistration("prewarm-test", src)
	var out strings.Builder
	if err := runPrewarmCmd(&out, sid, nil, nil, []source.Registration{reg}); err != nil {
		t.Fatalf("runPrewarmCmd: %v", err)
	}
	return index.PrewarmDumpPath(sid), src, c
}

func TestRunPrewarmExternalBehaviors(t *testing.T) {
	t.Run("folds absent session", func(t *testing.T) {
		dump, _, c := runPrewarmTest(t, "one", []model.Message{{Role: "user", Text: "one", UUID: "11111111-one"}})
		if _, err := os.Stat(dump); err != nil {
			t.Fatalf("dump missing: %v", err)
		}
		con, err := store.ConnectRO(index.ConsolidatedPath())
		if err != nil {
			t.Fatal(err)
		}
		defer con.Close()
		var n int
		if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id=?", c.ID).Scan(&n); err != nil || n != 1 {
			t.Fatalf("consolidated session count = %d, err=%v", n, err)
		}
	})

	t.Run("present session is not refolded", func(t *testing.T) {
		dump, src, c := runPrewarmTest(t, "one", []model.Message{{Role: "user", Text: "one", UUID: "11111111-one"}})
		before, err := os.ReadFile(dump)
		if err != nil {
			t.Fatal(err)
		}
		writeTagSourceFile(t, c.Path, "one\ntwo")
		src.messages = append(src.messages, model.Message{Role: "assistant", Text: "two", UUID: "22222222-two"})
		var out strings.Builder
		reg := tagTestRegistration("prewarm-test", src)
		if err := runPrewarmCmd(&out, c.ID, nil, nil, []source.Registration{reg}); err != nil {
			t.Fatal(err)
		}
		con, err := store.ConnectRO(index.ConsolidatedPath())
		if err != nil {
			t.Fatal(err)
		}
		var n int
		_ = con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID).Scan(&n)
		_ = con.Close()
		if n != 1 {
			t.Fatalf("consolidated message count = %d, want unchanged 1", n)
		}
		if string(before) == string(mustReadFile(t, dump)) {
			t.Fatalf("dump did not refresh private cache after transcript growth")
		}
	})

	t.Run("unchanged transcript keeps dump mtime", func(t *testing.T) {
		dump, src, c := runPrewarmTest(t, "one", []model.Message{{Role: "user", Text: "one", UUID: "11111111-one"}})
		first, err := os.Stat(dump)
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		reg := tagTestRegistration("prewarm-test", src)
		if err := runPrewarmCmd(&out, c.ID, nil, nil, []source.Registration{reg}); err != nil {
			t.Fatal(err)
		}
		second, _ := os.Stat(dump)
		if !second.ModTime().Equal(first.ModTime()) {
			t.Fatalf("unchanged dump mtime changed: %v -> %v", first.ModTime(), second.ModTime())
		}
	})

	t.Run("grown transcript regenerates dump", func(t *testing.T) {
		dump, src, c := runPrewarmTest(t, "one", []model.Message{{Role: "user", Text: "one", UUID: "11111111-one"}})
		first, err := os.Stat(dump)
		if err != nil {
			t.Fatal(err)
		}
		writeTagSourceFile(t, c.Path, "one\ntwo")
		_ = os.Chtimes(c.Path, time.Now().Add(2*time.Second), time.Now().Add(2*time.Second))
		src.messages = append(src.messages, model.Message{Role: "assistant", Text: "two", UUID: "22222222-two"})
		var out strings.Builder
		if err := runPrewarmCmd(&out, c.ID, nil, nil, []source.Registration{tagTestRegistration("prewarm-test", src)}); err != nil {
			t.Fatal(err)
		}
		second, _ := os.Stat(dump)
		if !second.ModTime().After(first.ModTime()) {
			t.Fatalf("grown dump mtime did not advance: %v -> %v", first.ModTime(), second.ModTime())
		}
	})
}

func TestRunTagPrepCmdReadsCommittedTagBeforeConsolidatedFold(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("HOME", configDir)
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	projDir := filepath.Join(configDir, "projects", "-overlay-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const sid = "overlay-session-0001"
	path := filepath.Join(projDir, sid+".jsonl")
	writeTagSourceFile(t, path, `{"type":"user","uuid":"11112222-msg","timestamp":"2026-08-25T10:00:00Z","message":{"role":"user","content":"overlay message"}}`+"\n")
	c := source.Container{ID: sid, Path: path, CWD: projDir}
	src := &tagTestSource{containers: []source.Container{c}, messages: []model.Message{{Role: "user", Text: "overlay message", UUID: "11112222-msg"}}}
	reg := tagTestRegistration("overlay-test", src)
	dbp := index.RefreshDBPath(reg.ID, sid, path)
	if _, err := index.PrepareFreshContainer(dbp, c, src.Messages, reg.ID); err != nil {
		t.Fatal(err)
	}
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTopicSegment(con, sid, "11112222-msg", "11112222-msg", "authoritative topic", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := con.Close(); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, sid, nil, nil, []source.Registration{reg}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already fully tagged") {
		t.Fatalf("tag-prep ignored committed authoritative topic:\n%s", out.String())
	}
}

// TestTagWriteQueuesDerivedPublication proves the foreground command can finish
// after the authoritative project-db write without waiting for consolidated
// publication. The publisher seam is replaced so this remains deterministic.
func TestTagWriteQueuesDerivedPublication(t *testing.T) {
	root := newCfgRoot(t)
	sid := "5f3e1c20-aaaa-bbbb-cccc-0000000abcd1"
	dir := writeTaggableSession(t, root, "proj-tag-queued", sid,
		"11111111-aaaa-bbbb-cccc-000000000001", "22222222-aaaa-bbbb-cccc-000000000002")
	var published string
	old := spawnTagPublish
	spawnTagPublish = func(dbp, sessionID string) error {
		published = dbp
		return nil
	}
	t.Cleanup(func() { spawnTagPublish = old })

	var out strings.Builder
	err := runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"11111111","topic":"queued","summary":"deferred publication"}]`),
		sid[:8], []view.Scope{{Project: "proj-tag-queued", TDir: dir}}, nil, false, "", false)
	if err != nil {
		t.Fatalf("runTagWriteCmd: %v", err)
	}
	if published == "" {
		t.Fatal("tag-write did not request detached derived publication")
	}
	if !strings.Contains(out.String(), "publication queued") {
		t.Fatalf("output = %q, want queued publication receipt", out.String())
	}
}

func TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication(t *testing.T) {
	root := newCfgRoot(t)
	sid := "6f3e1c20-aaaa-bbbb-cccc-0000000abcd1"
	dir := writeTaggableSession(t, root, "proj-tag-delayed", sid,
		"33333333-aaaa-bbbb-cccc-000000000001", "44444444-aaaa-bbbb-cccc-000000000002")

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	finished := false
	var published string
	old := spawnTagPublish
	spawnTagPublish = func(dbp, sessionID string) error {
		published = dbp
		close(started)
		go func() {
			<-release
			done <- runTagPublishChild(context.Background(), io.Discard, dbp, sessionID)
		}()
		return nil
	}
	t.Cleanup(func() {
		spawnTagPublish = old
		select {
		case <-release:
		default:
			close(release)
		}
		if !finished {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("delayed publisher did not finish")
			}
		}
	})

	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(`[{
"start_uuid":"33333333","topic":"delayed authoritative","summary":"pending fold"
}]`), sid[:8], []view.Scope{{Project: "proj-tag-delayed", TDir: dir}}, nil, false, "", false); err != nil {
		t.Fatalf("runTagWriteCmd: %v", err)
	}
	select {
	case <-started:
	default:
		t.Fatal("publisher was not delayed")
	}
	auth, err := readAuthoritativeTagTopics(published, sid)
	if err != nil {
		t.Fatalf("read authoritative topics: %v", err)
	}
	if len(auth) != 1 || auth[0].Topic != "delayed authoritative" {
		t.Fatalf("authoritative topics while child blocked = %#v, want committed tag", auth)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("publish after release: %v", err)
	}
	finished = true
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated after release: %v", err)
	}
	defer con.Close()
	segs, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("read consolidated topics after release: %v", err)
	}
	if len(segs) != 1 || segs[0].Topic != "delayed authoritative" {
		t.Fatalf("consolidated topics after release = %#v, want eventual publication", segs)
	}
}

func TestRunTagPublishChildHonorsCanceledContext(t *testing.T) {
	root := newCfgRoot(t)
	sid := "7f3e1c20-aaaa-bbbb-cccc-0000000abcd1"
	dir := writeTaggableSession(t, root, "proj-tag-cancel", sid,
		"55555555-aaaa-bbbb-cccc-000000000001", "66666666-aaaa-bbbb-cccc-000000000002")
	dbPath := filepath.Join(dir, "tags.db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runTagPublishChild(ctx, io.Discard, dbPath, sid); !errors.Is(err, context.Canceled) {
		t.Fatalf("runTagPublishChild canceled error = %v, want context.Canceled", err)
	}
}

func TestRunPrewarmRegeneratesWhenDumpMissing(t *testing.T) {
	dump, src, c := runPrewarmTest(t, "one", []model.Message{{
		Role: "user", Text: "one", UUID: "11111111-one",
	}})
	if err := os.Remove(dump); err != nil {
		t.Fatalf("remove dump: %v", err)
	}
	var out strings.Builder
	if err := runPrewarmCmd(&out, c.ID, nil, nil, []source.Registration{tagTestRegistration("prewarm-test", src)}); err != nil {
		t.Fatalf("regenerate missing dump: %v", err)
	}
	if _, err := os.Stat(dump); err != nil {
		t.Fatalf("missing dump was not regenerated: %v", err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
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

func TestRunTagPrepCmdUsesFreshPrewarmDump(t *testing.T) {
	dump, src, c := runPrewarmTest(t, "prewarmed", []model.Message{{
		Role: "user", Text: "from prewarm", UUID: "aaaaaaaa-prewarm",
	}})
	src.messagesErr = errors.New("refresh should not run")

	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, c.ID, nil, nil, []source.Registration{
		tagTestRegistration("prewarm-test", src),
	}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}
	want := string(mustReadFile(t, dump))
	if out.String() != want {
		t.Fatalf("tag-prep output = %q, want prewarm dump %q", out.String(), want)
	}
}

func TestRunTagPrepCmdFallsBackForStalePrewarmDump(t *testing.T) {
	_, src, c := runPrewarmTest(t, "old", []model.Message{{
		Role: "user", Text: "old", UUID: "bbbbbbbb-old",
	}})
	writeTagSourceFile(t, c.Path, "old\nnew")
	src.messages = append(src.messages, model.Message{
		Role: "assistant", Text: "refreshed", UUID: "cccccccc-new",
	})

	var out strings.Builder
	if err := runTagPrepCmdWithSources(&out, c.ID, nil, nil, []source.Registration{
		tagTestRegistration("prewarm-test", src),
	}); err != nil {
		t.Fatalf("runTagPrepCmdWithSources: %v", err)
	}
	if !strings.Contains(out.String(), "cccccccc [assistant] refreshed") {
		t.Fatalf("tag-prep did not fall back to refreshed source:\n%s", out.String())
	}
	if src.messagesCalls < 2 {
		t.Fatalf("Messages called %d times, want prewarm plus fallback refresh", src.messagesCalls)
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

	// The fresh project DB is authoritative immediately; detached publication
	// may still be pending, so assert that the refresh DB was retained.
	dbp := index.RefreshDBPath(claude.Registration().ID, fullSID, transcriptPath)
	if _, err := os.Stat(dbp); err != nil {
		t.Fatalf("refresh db %s not found on disk, want retained: %v", dbp, err)
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
		Source:         "codex",
	}); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	var out strings.Builder
	opts := &Options{Resume: "resume01"}
	if err := runResume(&out, opts); err != nil {
		t.Fatalf("runResume: %v", err)
	}

	if !strings.Contains(out.String(), "codex resume resume01-catalog-session-uuid") {
		t.Fatalf("runResume output missing catalog source's Codex command:\n%s", out.String())
	}
}

func TestRunResumeUsesConsolidatedCatalogForExactID(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(configDir, "cache"))

	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("ConnectRW consolidated: %v", err)
	}
	if err := store.Rebuild(con); err != nil {
		con.Close()
		t.Fatalf("Rebuild consolidated: %v", err)
	}
	const fullSID = "resume02-consolidated-session-uuid"
	if _, err := con.Exec(`INSERT INTO sessions
		(id, message_count, source_tool, project, cwd, is_subagent)
		VALUES (?, 1, ?, ?, ?, 0)`, fullSID, "codex", "catalog-project", "/work/catalog"); err != nil {
		con.Close()
		t.Fatalf("insert consolidated session: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close consolidated: %v", err)
	}

	var out strings.Builder
	if err := runResume(&out, &Options{Resume: fullSID}); err != nil {
		t.Fatalf("runResume: %v", err)
	}
	if !strings.Contains(out.String(), "codex resume "+fullSID) {
		t.Fatalf("runResume output missing consolidated Codex command:\n%s", out.String())
	}
}

func TestRunResumeDeduplicatesCatalogAndConsolidatedHit(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("HOME", configDir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(configDir, "cache"))

	const fullSID = "resume03-shared-session-uuid"
	transcriptPath := filepath.Join(configDir, "transcripts", fullSID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	writeTagSourceFile(t, transcriptPath, `{"cwd":"/work/shared"}`+"\n")
	if err := paths.WriteCatalogEntry(filepath.Join(configDir, "catalog"), paths.CatalogEntry{
		SessionID: fullSID, TranscriptPath: transcriptPath, CWD: "/work/shared", Source: "claude",
	}); err != nil {
		t.Fatalf("WriteCatalogEntry: %v", err)
	}

	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("ConnectRW consolidated: %v", err)
	}
	if err := store.Rebuild(con); err != nil {
		con.Close()
		t.Fatalf("Rebuild consolidated: %v", err)
	}
	if _, err := con.Exec(`INSERT INTO sessions
		(id, message_count, source_tool, project, cwd, is_subagent)
		VALUES (?, 1, ?, ?, ?, 0)`, fullSID, "claude", "shared", "/work/shared"); err != nil {
		con.Close()
		t.Fatalf("insert consolidated session: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close consolidated: %v", err)
	}

	var out strings.Builder
	if err := runResume(&out, &Options{Resume: fullSID}); err != nil {
		t.Fatalf("runResume: %v", err)
	}
	if strings.Contains(out.String(), "sessions match") {
		t.Fatalf("same Claude session was reported ambiguous across lookup layers:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "claude --resume "+fullSID) {
		t.Fatalf("runResume output missing expected resume command:\n%s", out.String())
	}
}

func TestRunTagPrepCmd_ContentionSpawnsDetachedFoldAndFoldsOnNextTouch(t *testing.T) {
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

	var spawned string
	oldSpawn := spawnIngest
	spawnIngest = func(sessionArg string) { spawned = sessionArg }
	t.Cleanup(func() { spawnIngest = oldSpawn })

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

	if spawned != fullSID {
		t.Fatalf("detached ingest session = %q, want %q", spawned, fullSID)
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
	c := source.Container{ID: fullSID, Path: transcriptPath, CWD: projDir}
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

// TestRunTagPrepCmd_DetachedFoldDoesNotInspectConsolidatedFailures proves the
// dump does not synchronously touch the consolidated store after refresh.
func TestRunTagPrepCmd_DetachedFoldDoesNotInspectConsolidatedFailures(t *testing.T) {
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

	var spawned string
	oldSpawn := spawnIngest
	spawnIngest = func(sessionArg string) { spawned = sessionArg }
	t.Cleanup(func() { spawnIngest = oldSpawn })

	var out strings.Builder
	if err := runTagPrepCmd(&out, "foldfa", nil, nil); err != nil {
		t.Fatalf("runTagPrepCmd failed, want exit 0 (the dump itself must still succeed): %v", err)
	}

	if !strings.Contains(out.String(), "77778888 [user] fold failure message") {
		t.Fatalf("dump missing message; got:\n%s", out.String())
	}

	if spawned != fullSID {
		t.Fatalf("detached ingest session = %q, want %q", spawned, fullSID)
	}
}
