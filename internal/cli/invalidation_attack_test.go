package cli

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
)

// TestRunTagPublishChildHonorsCancellationWhileWaitingForFence is an
// invalidation proof for the 25-second detached-child deadline. A canceled
// child must not remain blocked in a non-contextual fold while the fence is
// held.
func TestRunTagPublishChildHonorsCancellationWhileWaitingForFence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	holder := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated lock: locked=%t err=%v", locked, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		done <- runTagPublishChild(ctx, io.Discard, filepath.Join(t.TempDir(), "missing.db"))
	}()

	select {
	case err := <-done:
		_ = holder.Unlock()
		if err == nil {
			t.Fatal("canceled tag publisher returned nil")
		}
	case <-time.After(300 * time.Millisecond):
		_ = holder.Unlock()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("tag publisher remained blocked after fence release")
		}
		t.Fatal("canceled tag publisher did not return within 300ms")
	}
}

// TestOverlayAuthoritativeTopicsDropsDeletedDerivedRows is an invalidation
// proof for re-tag deletion: the authoritative per-project set is a complete
// replacement, so a derived row absent from it must not survive the overlay.
func TestOverlayAuthoritativeTopicsDropsDeletedDerivedRows(t *testing.T) {
	derived := []store.TopicSegment{
		{SessionID: "session", StartUUID: "keep", Topic: "newer derived copy"},
		{SessionID: "session", StartUUID: "deleted", Topic: "stale deleted topic"},
	}
	authoritative := []store.TopicSegment{
		{SessionID: "session", StartUUID: "keep", Topic: "authoritative topic"},
	}

	got := overlayAuthoritativeTopics(derived, authoritative)
	if len(got) != 1 || got[0].StartUUID != "keep" || got[0].Topic != "authoritative topic" {
		t.Fatalf("overlay = %#v, want only authoritative replacement", got)
	}
	for _, seg := range got {
		if seg.StartUUID == "deleted" {
			t.Fatal("overlay retained a derived row deleted from authoritative set")
		}
	}
}
