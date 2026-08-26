package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
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

func TestLocateTagWriteFast_TDirScopesAmbiguousPrefix(t *testing.T) {
	root := newCfgRoot(t)
	const prefix = "samepref"
	sidA := prefix + "-aaaa-4000-8000-000000000001"
	sidB := prefix + "-bbbb-4000-8000-000000000002"
	uuidA := "aaaaaaaa-aaaa-bbbb-cccc-000000000001"
	uuidB := "bbbbbbbb-bbbb-cccc-dddd-000000000002"
	dirA := writeTaggableSession(t, root, "proj-a", sidA, uuidA)
	dirB := writeTaggableSession(t, root, "proj-b", sidB, uuidB)
	for _, path := range []string{filepath.Join(dirA, sidA+".jsonl")} {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(`{"type":"user","uuid":"` + map[string]string{path: uuidA + "-new", filepath.Join(dirB, sidB+".jsonl"): uuidB + "-new"}[path] + `","timestamp":"2026-06-01T10:00:01Z","message":{"role":"user","content":"new"}}
`); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	old := spawnTagPublish
	spawnTagPublish = func(string, string) error { return nil }
	t.Cleanup(func() { spawnTagPublish = old })
	var out strings.Builder
	if err := runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"`+uuidB[:8]+`","topic":"scoped","summary":"scoped"}]`), prefix, []view.Scope{{Project: "proj-b", TDir: dirB}}, nil, false, "", false); err != nil {
		t.Fatalf("scoped tag-write: %v", err)
	}
	for _, tc := range []struct {
		name, db, sid string
		want          int
	}{{"unscoped project", index.DBPath(dirA), sidA, 0}, {"scoped project", index.DBPath(dirB), sidB, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			con, err := store.ConnectRO(tc.db)
			if err != nil {
				t.Fatal(err)
			}
			defer con.Close()
			segs, err := store.TopicsForSession(con, tc.sid)
			if err != nil {
				t.Fatal(err)
			}
			if len(segs) != tc.want {
				t.Fatalf("topics = %#v, want %d", segs, tc.want)
			}
		})
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

	con, err := store.ConnectRO(index.DBPath(dir))
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
