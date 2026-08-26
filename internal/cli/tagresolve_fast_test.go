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

func TestLocateTagWriteFast_ExplicitDirectoryDoesNotNeedGlobalDiscovery(t *testing.T) {
	newCfgRoot(t)
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(t.TempDir(), "missing-catalog"))

	tdir := filepath.Join(t.TempDir(), "arbitrary-transcripts")
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	sid := "arbitrary1-aaaa-bbbb-cccc-000000000001"
	writeTranscript(t, tdir, sid, []string{"aaaaaaaa-aaaa-bbbb-cccc-000000000001"})
	dbp, _, _, err := index.EnsureIndexed(tdir, false)
	if err != nil {
		t.Fatal(err)
	}

	if gotDB, gotSID, found := locateTagWriteFast(sid[:8], []view.Scope{{Project: "arbitrary", TDir: tdir}}); !found || gotDB != dbp || gotSID != sid {
		t.Fatalf("explicit arbitrary scope = (%q, %q, %v), want (%q, %q, true)", gotDB, gotSID, found, dbp, sid)
	}
}

func TestLocateTagWriteFast_ExplicitSymlinkAliasDoesNotNeedGlobalDiscovery(t *testing.T) {
	newCfgRoot(t)
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(t.TempDir(), "empty-catalog"))

	target := filepath.Join(t.TempDir(), "target-transcripts")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	sid := "symlink01-aaaa-bbbb-cccc-000000000001"
	writeTranscript(t, target, sid, []string{"bbbbbbbb-bbbb-cccc-dddd-000000000001"})
	if _, _, _, err := index.EnsureIndexed(target, false); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "project-alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if gotDB, gotSID, found := locateTagWriteFast(sid[:8], []view.Scope{{Project: "alias", TDir: alias}}); !found || gotDB != index.DBPath(alias) || gotSID != sid {
		t.Fatalf("explicit symlink scope = (%q, %q, %v), want (%q, %q, true)", gotDB, gotSID, found, index.DBPath(alias), sid)
	}
}

func TestLocateTagWriteFast_MissingOrEmptyCatalogFallsBackToProjectDiscovery(t *testing.T) {
	for _, catalogState := range []string{"missing", "empty"} {
		t.Run(catalogState, func(t *testing.T) {
			root := newCfgRoot(t)
			catalog := filepath.Join(t.TempDir(), "catalog")
			t.Setenv("RAWCLAW_CATALOG_DIR", catalog)
			if catalogState == "empty" {
				if err := os.MkdirAll(catalog, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			sid := "fallback1-aaaa-bbbb-cccc-000000000001"
			tdir := filepath.Join(root, "fallback-project")
			if err := os.MkdirAll(tdir, 0o755); err != nil {
				t.Fatal(err)
			}
			writeTranscript(t, tdir, sid, []string{"cccccccc-aaaa-bbbb-cccc-000000000001"})
			dbp, _, _, err := index.EnsureIndexed(tdir, false)
			if err != nil {
				t.Fatal(err)
			}

			if gotDB, gotSID, found := locateTagWriteFast(sid[:8], nil); !found || gotDB != dbp || gotSID != sid {
				t.Fatalf("%s catalog scope = (%q, %q, %v), want (%q, %q, true)", catalogState, gotDB, gotSID, found, dbp, sid)
			}
		})
	}
}

func TestLocateTagWriteFast_DistinctSourceAndProjectIdentityIsAmbiguous(t *testing.T) {
	root := newCfgRoot(t)
	sid := "collision-aaaa-bbbb-cccc-000000000001"
	dirA := writeTaggableSession(t, root, "collision-claude", sid, "dddddddd-aaaa-bbbb-cccc-000000000001")
	dirB := writeTaggableSession(t, root, "collision-codex", sid, "eeeeeeee-aaaa-bbbb-cccc-000000000001")

	scope := []view.Scope{
		{Project: "collision-claude", DBP: index.DBPath(dirA), Source: "claude"},
		{Project: "collision-codex", DBP: index.DBPath(dirB), Source: "codex"},
	}
	if db, fullSID, found := locateTagWriteFast(sid[:8], scope); found {
		t.Fatalf("same session id across source/project scopes = (%q, %q, %v), want ambiguous fallback", db, fullSID, found)
	}
}
