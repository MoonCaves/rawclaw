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
	t.Cleanup(func() { spawnTagPublish = old })
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
	t.Cleanup(func() { spawnTagPublish = oldPublish })

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
	t.Cleanup(func() { spawnTagPublish = oldPublish })

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

func TestTagWriteTDirFastPathIsExactAndCatalogIndependent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		catalog   bool
		alias     bool
		ambiguous bool
	}{
		{name: "missing catalog"},
		{name: "empty catalog", catalog: true},
		{name: "symlink alias", alias: true},
		{name: "ambiguous prefix", ambiguous: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newCfgRoot(t)
			dir := t.TempDir()
			if tc.catalog {
				catalog := filepath.Join(t.TempDir(), "catalog")
				t.Setenv("RAWCLAW_CATALOG_DIR", catalog)
				if err := os.MkdirAll(catalog, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			const prefix = "arbitrary1"
			sid := prefix + "-0000-4000-8000-000000000001"
			writeTranscriptWithCWD(t, dir, sid, "/workspace/exact-project")
			if tc.ambiguous {
				writeTranscriptWithCWD(t, dir, prefix+"-0000-4000-8000-000000000002", "/workspace/exact-project")
			}
			lookupDir := dir
			if tc.alias {
				lookupDir = filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(dir, lookupDir); err != nil {
					t.Fatal(err)
				}
			}
			if tc.ambiguous {
				if db, gotSID, found := locateTagWriteFast(prefix, []view.Scope{{TDir: lookupDir}}); found || db != "" || gotSID != "" {
					t.Fatalf("ambiguous lookup = (%q, %q, %v)", db, gotSID, found)
				}
				return
			}
			oldPublish := spawnTagPublish
			spawnTagPublish = func(string, string) error { return nil }
			t.Cleanup(func() { spawnTagPublish = oldPublish })
			var out strings.Builder
			if err := runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"11111111","topic":"exact","summary":"exact"}]`), prefix, []view.Scope{{TDir: lookupDir}}, nil, false, "", false); err != nil {
				t.Fatal(err)
			}
			con, err := store.ConnectRO(index.RefreshDBPath("claude", sid, filepath.Join(lookupDir, sid+".jsonl")))
			if err != nil {
				t.Fatal(err)
			}
			defer con.Close()
			var project, cwd, sourceTool string
			if err := con.QueryRow("SELECT project, cwd, source_tool FROM sessions WHERE id=?", sid).Scan(&project, &cwd, &sourceTool); err != nil {
				t.Fatal(err)
			}
			if project != "exact-project" || cwd != "/workspace/exact-project" || sourceTool != "claude" {
				t.Fatalf("identity = (%q, %q, %q), want (exact-project, /workspace/exact-project, claude)", project, cwd, sourceTool)
			}
		})
	}
}

func writeTranscriptWithCWD(t *testing.T, dir, sid, cwd string) {
	t.Helper()
	body := `{"type":"user","cwd":"` + cwd + `","uuid":"11111111-aaaa-bbbb-cccc-000000000001","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"a message worth tagging"}}
`
	if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
