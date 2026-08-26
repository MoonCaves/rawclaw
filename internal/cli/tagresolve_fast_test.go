package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/gofrs/flock"
)

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
	spawnTagPublish = func(string) error { return nil }
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
	spawnTagPublish = func(string) error { return nil }
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
