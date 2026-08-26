package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/gofrs/flock"
)

func TestRunTagWriteDefaultCatalogFastPathBeforeFence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	catalog := filepath.Join(config, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catalog)
	target := filepath.Join(config, "projects", "default-fast")
	os.MkdirAll(target, 0o755)
	sid := "abc12345-0000-4000-8000-000000000001"
	uuid := "11111111-aaaa-bbbb-cccc-000000000001"
	writeTranscript(t, target, sid, []string{uuid})
	dbp, _, _, err := index.EnsureIndexed(target, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCatalogEntry(catalog, paths.CatalogEntry{SessionID: sid, TranscriptPath: filepath.Join(target, sid+".jsonl"), CWD: target}); err != nil {
		t.Fatal(err)
	}
	lock := flock.New(filepath.Join(filepath.Dir(index.ConsolidatedPath()), "consolidated.lock"))
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("lock: %v", err)
	}
	defer lock.Unlock()
	old := spawnTagPublish
	spawnTagPublish = func(string, string) error { return nil }
	defer func() { spawnTagPublish = old }()
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"11111111","topic":"default-fast","summary":"x"}]`), sid[:8], nil, func() []view.Scope { t.Fatal("guarded lookup called"); return nil }, false, "", false); err != nil {
		t.Fatal(err)
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	segs, err := store.TopicsForSession(con, sid)
	if err != nil || len(segs) != 1 {
		t.Fatalf("source topics=%#v err=%v", segs, err)
	}
}

func TestLocateTagWriteFast_NilScopeAmbiguousCatalogFallsBack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	catalog := filepath.Join(config, "catalog")
	t.Setenv("RAWCLAW_CATALOG_DIR", catalog)
	for _, sid := range []string{"abc99999-0000-4000-8000-000000000001", "abc99999-0000-4000-8000-000000000002"} {
		proj := filepath.Join(config, "projects", sid)
		os.MkdirAll(proj, 0o755)
		path := filepath.Join(proj, sid+".jsonl")
		writeTranscript(t, proj, sid, []string{"22222222-aaaa-bbbb-cccc-000000000001"})
		if err := paths.WriteCatalogEntry(catalog, paths.CatalogEntry{SessionID: sid, TranscriptPath: path, CWD: proj}); err != nil {
			t.Fatal(err)
		}
	}
	if db, full, found := locateTagWriteFast("abc99999", nil); found || db != "" || full != "" {
		t.Fatalf("ambiguous catalog fast path=(%q,%q,%v)", db, full, found)
	}
}

func TestTagWriteFastPathAuthorsBeforeConsolidatedFence(t *testing.T) {
	root := newCfgRoot(t)
	sid := "6f3e1c20-aaaa-bbbb-cccc-0000000abcd4"
	dir := writeTaggableSession(t, root, "proj-fast", sid,
		"11111111-aaaa-bbbb-cccc-000000000001",
		"22222222-aaaa-bbbb-cccc-000000000002")
	dbPath, _, _, err := index.EnsureIndexed(dir, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	lock := flock.New(index.ConsolidatedPath()[:len(index.ConsolidatedPath())-len("consolidated.db")] + "consolidated.lock")
	locked, err := lock.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated fence: locked=%t err=%v", locked, err)
	}
	defer lock.Unlock()
	oldPublish := spawnTagPublish
	spawnTagPublish = func(string, string) error { return nil }
	defer func() { spawnTagPublish = oldPublish }()
	done := make(chan error, 1)
	go func() {
		var out strings.Builder
		done <- runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"11111111","topic":"fast-authoritative","summary":"written before fold"}]`), sid[:8], []view.Scope{{Project: "proj-fast", DBP: dbPath}}, nil, false, "", false)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fast tag-write: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fast tag-write blocked behind consolidated fence")
	}

	con, err := store.ConnectRO(dbPath)
	if err != nil {
		t.Fatalf("open authoritative db: %v", err)
	}
	defer con.Close()
	segs, err := store.TopicsForSession(con, sid)
	if err != nil || len(segs) != 1 || segs[0].Topic != "fast-authoritative" {
		t.Fatalf("authoritative topics = %#v, err=%v", segs, err)
	}
}

func TestTagWriteTDirFastPathAuthorsBeforeConsolidatedFence(t *testing.T) {
	root := newCfgRoot(t)
	sid := "6f3e1c20-aaaa-bbbb-cccc-0000000abcd5"
	dir := writeTaggableSession(t, root, "proj-fast-tdir", sid,
		"33333333-aaaa-bbbb-cccc-000000000001",
		"44444444-aaaa-bbbb-cccc-000000000002")
	if _, _, _, err := index.EnsureIndexed(dir, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	lock := flock.New(filepath.Join(filepath.Dir(index.ConsolidatedPath()), "consolidated.lock"))
	locked, err := lock.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated fence: locked=%t err=%v", locked, err)
	}
	defer lock.Unlock()
	oldPublish := spawnTagPublish
	spawnTagPublish = func(string, string) error { return nil }
	defer func() { spawnTagPublish = oldPublish }()

	done := make(chan error, 1)
	go func() {
		var out strings.Builder
		done <- runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"33333333","topic":"tdir-authoritative","summary":"written before fold"}]`), sid[:8], []view.Scope{{Project: "proj-fast-tdir", TDir: dir}}, nil, false, "", false)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("TDir fast tag-write: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TDir fast tag-write blocked behind consolidated fence")
	}

	con, err := store.ConnectRO(index.RefreshDBPath("claude", sid, filepath.Join(dir, sid+".jsonl")))
	if err != nil {
		t.Fatalf("open authoritative db: %v", err)
	}
	defer con.Close()
	segs, err := store.TopicsForSession(con, sid)
	if err != nil || len(segs) != 1 || segs[0].Topic != "tdir-authoritative" {
		t.Fatalf("authoritative topics = %#v, err=%v", segs, err)
	}
}

func TestLocateTagWriteFast_ExplicitEmptyScopeDoesNotUseCatalog(t *testing.T) {
	if db, sid, found := locateTagWriteFast("anything", []view.Scope{}); found || db != "" || sid != "" {
		t.Fatalf("explicit empty scope fast path = (%q, %q, %v), want no lookup", db, sid, found)
	}
}

func TestTagWriteTDirRefreshesExactArbitrarySymlinkedContainer(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "arbitrary-live")
	alias := filepath.Join(root, "live-alias")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	sid := "dirrefresh-aaaa-4000-8000-000000000001"
	path := filepath.Join(alias, sid+".jsonl")
	if err := os.WriteFile(path, []byte("{}\n{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := source.Registration{
		ID: "test-dir-source",
		Detect: func(path string) bool {
			return strings.HasSuffix(path, sid+".jsonl")
		},
		New: func() source.Source { return testDirSource{} },
	}
	source.Register(reg)
	msgs := func(source.Container) ([]model.Message, error) {
		return []model.Message{{Role: "user", Text: "one", UUID: "aaaa1111"}, {Role: "assistant", Text: "two", UUID: "bbbb2222"}}, nil
	}
	db := index.RefreshDBPath(reg.ID, sid, path)
	if _, err := index.PrepareFreshContainer(db, source.Container{ID: sid, Path: path, CWD: "/arbitrary/project"}, msgs, reg.ID); err != nil {
		t.Fatalf("initial exact refresh: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}\n{}\n{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := flock.New(filepath.Join(filepath.Dir(index.ConsolidatedPath()), "consolidated.lock"))
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("hold consolidated fence: %v", err)
	}
	defer lock.Unlock()
	oldPublish := spawnTagPublish
	spawnTagPublish = func(string, string) error { return nil }
	defer func() { spawnTagPublish = oldPublish }()
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"aaaa1111","topic":"arbitrary","summary":"exact"}]`), sid[:8], []view.Scope{{TDir: alias}}, nil, false, "", false); err != nil {
		t.Fatalf("tag-write: %v", err)
	}
	con, err := store.ConnectRO(db)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	var sourceID, cwd string
	if err := con.QueryRow("SELECT source_tool,cwd FROM sessions WHERE id=?", sid).Scan(&sourceID, &cwd); err != nil {
		t.Fatal(err)
	}
	if sourceID != reg.ID || cwd != filepath.Base(target) {
		t.Fatalf("identity=(%q,%q), want (%q,%q)", sourceID, cwd, reg.ID, filepath.Base(target))
	}
}

func TestLocateTagWriteFast_TDirAmbiguousPrefixDoesNotRefresh(t *testing.T) {
	dir := t.TempDir()
	sourceID := "test-ambiguous-dir-source"
	for _, sid := range []string{"samepref-aaaa-4000-8000-000000000001", "samepref-bbbb-4000-8000-000000000002"} {
		if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	source.Register(source.Registration{
		ID:     sourceID,
		Detect: func(path string) bool { return pathInDir(path, dir) && strings.HasSuffix(path, ".jsonl") },
		New:    func() source.Source { return testDirSource{} },
	})
	if db, sid, found := locateTagWriteFast("samepref", []view.Scope{{TDir: dir}}); found || db != "" || sid != "" {
		t.Fatalf("ambiguous TDir fast path=(%q,%q,%v)", db, sid, found)
	}
}

type testDirSource struct{}

func (testDirSource) Discover() ([]source.Container, error) { return nil, nil }

func (testDirSource) Messages(source.Container) ([]model.Message, error) {
	return []model.Message{{Role: "user", Text: "one", UUID: "aaaa1111"}, {Role: "assistant", Text: "two", UUID: "bbbb2222"}}, nil
}
