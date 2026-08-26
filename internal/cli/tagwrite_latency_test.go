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

// TestTagWriteForegroundWaitsAfterDurableCommit proves that tag-write's
// authoring write commits before its consolidated publication wait. The held
// fence blocks only SyncConsolidatedFrom; the per-project rows must remain
// readable while runTagWriteCmd is still in flight.
func TestTagWriteForegroundWaitsAfterDurableCommit(t *testing.T) {
	root := newCfgRoot(t)
	sid := "7f3e1c20-aaaa-bbbb-cccc-0000000abcd3"
	dir := writeTaggableSession(t, root, "proj-latency", sid,
		"11111111-aaaa-bbbb-cccc-000000000001",
		"22222222-aaaa-bbbb-cccc-000000000002")
	dbPath, _, _, err := index.EnsureIndexed(dir, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	// EnsureIndexed may eagerly publish this fixture. Remove only the derived
	// copy so LocateSessionGuarded must exercise the per-project source DB.
	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	for _, query := range []string{
		"DELETE FROM topic_segment WHERE session_id=?",
		"DELETE FROM messages WHERE session_id=?",
		"DELETE FROM sessions WHERE id=?",
	} {
		if _, err := con.Exec(query, sid); err != nil {
			t.Fatalf("purge consolidated fixture with %q: %v", query, err)
		}
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close consolidated store: %v", err)
	}

	fence := flock.New(filepath.Join(filepath.Dir(index.ConsolidatedPath()), "consolidated.lock"))
	locked, err := fence.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated fence: locked=%t err=%v", locked, err)
	}

	scope := []view.Scope{{Project: "proj-latency", TDir: dir, DBP: dbPath}}
	jsonIn := `[{"start_uuid":"11111111","topic":"durable-before-publish","summary":"authoring committed"}]`
	done := make(chan error, 1)
	go func() {
		var out strings.Builder
		done <- runTagWriteCmd(&out, strings.NewReader(jsonIn), sid[:8], scope, nil, false, "", false)
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	var localErr error
	for {
		con, openErr := store.ConnectRO(dbPath)
		if openErr == nil {
			var topic string
			readErr := con.QueryRow("SELECT topic FROM topic_segment WHERE session_id=?", sid).Scan(&topic)
			_ = con.Close()
			if readErr == nil && topic == "durable-before-publish" {
				break
			}
			localErr = readErr
		}
		select {
		case err := <-done:
			fence.Unlock()
			t.Fatalf("runTagWriteCmd returned before durable local row was readable: %v (read err: %v)", err, localErr)
		case <-deadline.C:
			fence.Unlock()
			err := <-done
			t.Fatalf("timed out waiting for durable local row (runTagWriteCmd err: %v, read err: %v)", err, localErr)
		default:
			select {
			case <-time.After(time.Millisecond):
			case <-deadline.C:
				fence.Unlock()
				err := <-done
				t.Fatalf("timed out waiting for durable local row (runTagWriteCmd err: %v, read err: %v)", err, localErr)
			}
		}
	}

	select {
	case err := <-done:
		fence.Unlock()
		t.Fatalf("runTagWriteCmd returned while publication fence was held: %v", err)
	default:
	}
	if err := fence.Unlock(); err != nil {
		t.Fatalf("release publication fence: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runTagWriteCmd after fence release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runTagWriteCmd did not return after publication fence release")
	}
}
