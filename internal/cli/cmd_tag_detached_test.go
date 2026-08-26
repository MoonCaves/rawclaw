package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/gofrs/flock"
)

// TestRunTagPublishChildHonorsContextTimeout is an immutable red proof for
// detached publication cancellation. Holding the real consolidated fence
// makes the child wait at the actual publication seam; a bounded context must
// return a terminal deadline error instead of waiting on Background forever.
func TestRunTagPublishChildHonorsContextTimeout(t *testing.T) {
	root := newCfgRoot(t)
	dbp := filepath.Join(root, "source.db")
	lock := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		t.Fatalf("lock consolidated store: %v", err)
	}
	if !locked {
		t.Fatal("consolidated store lock was already held")
	}
	defer lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	var out strings.Builder
	done := make(chan error, 1)
	go func() { done <- runTagPublishChild(ctx, &out, dbp) }()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("runTagPublishChild error = %v, want context deadline exceeded", err)
		}
		if !strings.Contains(out.String(), "context deadline exceeded") {
			t.Fatalf("terminal publication receipt = %q, want deadline evidence", out.String())
		}
	case <-time.After(500 * time.Millisecond):
		// Release the real fence so an implementation that ignored ctx cannot
		// outlive this test process; its non-contextual path then reports the
		// missing source and is the expected red result.
		_ = lock.Unlock()
		select {
		case err := <-done:
			t.Fatalf("runTagPublishChild ignored context deadline: %v; receipt=%q", err, out.String())
		case <-time.After(2 * time.Second):
			t.Fatal("runTagPublishChild remained blocked after releasing consolidated lock")
		}
	}
}

// TestRunTagWriteCommandSeamIsNotIsolated records why command-level latency
// cannot prove detached publication: guarded lookup refreshes the project
// index and its write-through waits on the same consolidated fence before the
// authoritative topic write is reached.
func TestRunTagWriteCommandSeamIsNotIsolated(t *testing.T) {
	root := newCfgRoot(t)
	sid := "a1b2c3d4-aaaa-bbbb-cccc-0000000abcd3"
	dir := writeTaggableSession(t, root, "proj-detached", sid,
		"11111111-aaaa-bbbb-cccc-000000000001", "22222222-aaaa-bbbb-cccc-000000000002")
	scope := []view.Scope{{Project: "proj-detached", TDir: dir}}

	consolidatedLock := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	locked, err := consolidatedLock.TryLock()
	if err != nil {
		t.Fatalf("lock consolidated store: %v", err)
	}
	if !locked {
		t.Fatal("consolidated store lock was already held")
	}
	defer consolidatedLock.Unlock()

	jsonIn := `[{"start_uuid":"11111111","topic":"detached","summary":"published after return"}]`
	result := make(chan error, 1)
	go func() {
		var out strings.Builder
		result <- runTagWriteCmd(&out, strings.NewReader(jsonIn), sid[:8], scope, nil, false, "", false)
	}()

	select {
	case err := <-result:
		t.Fatalf("runTagWriteCmd returned while guarded lookup was fenced: %v", err)
	case <-time.After(250 * time.Millisecond):
		// Inspect the authoritative project db while the command is blocked. An
		// empty result proves the wait is in guarded lookup, not publication.
		con, openErr := store.ConnectRO(index.DBPath(dir))
		if openErr != nil {
			t.Fatalf("open project store while lookup is fenced: %v", openErr)
		}
		segments, readErr := store.TopicsForSession(con, sid)
		_ = con.Close()
		if readErr != nil {
			t.Fatalf("read project topics while lookup is fenced: %v", readErr)
		}
		if len(segments) != 0 {
			t.Fatalf("authoritative topics while lookup is fenced = %+v, want none", segments)
		}
	}
	_ = consolidatedLock.Unlock()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("runTagWriteCmd after releasing lookup fence: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runTagWriteCmd remained blocked after releasing consolidated lock")
	}

	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()
	hits, err := store.MatchTopics(con, "detached", 8, nil)
	if err != nil {
		t.Fatalf("MatchTopics on consolidated store: %v", err)
	}
	if len(hits) != 1 || hits[0].Topic != "detached" {
		t.Fatalf("consolidated store topic hits = %+v, want detached publication", hits)
	}
}
