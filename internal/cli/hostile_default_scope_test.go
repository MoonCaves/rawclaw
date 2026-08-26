package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
)

// TestTagWriteDefaultScopeConsolidatedOnlyWaitsForFence proves that a retained
// session answered only by consolidated.db waits behind the same fence as a
// concurrent rebuild. Direct writes must serialize with snapshot-and-replace
// publication or a committed tag can be discarded.
func TestTagWriteDefaultScopeConsolidatedOnlyWaitsForFence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "consolidated-only-0001"
	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := index.EnsureSchema(con, "claude"); err != nil {
		con.Close()
		t.Fatal(err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		con.Close()
		t.Fatal(err)
	}
	if _, err := con.Exec(`INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent,source_tool,project,cwd)
		VALUES(?,?,?,?,0,?,?,?)`, sid, 1.0, 1.0, 1, "claude", "retained", "/retained"); err != nil {
		con.Close()
		t.Fatal(err)
	}
	if _, err := con.Exec(`INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid)
		VALUES(?,?,?,?,?,?)`, sid, "user", "retained message", 1.0, "", "11111111-retained"); err != nil {
		con.Close()
		t.Fatal(err)
	}
	if err := con.Close(); err != nil {
		t.Fatal(err)
	}

	lock := flock.New(index.ConsolidatedPath()[:len(index.ConsolidatedPath())-len("consolidated.db")] + "consolidated.lock")
	ok, err := lock.TryLock()
	if err != nil || !ok {
		t.Fatalf("hold consolidated fence: locked=%t err=%v", ok, err)
	}

	done := make(chan error, 1)
	go func() {
		var out strings.Builder
		done <- runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"11111111","topic":"retained","summary":"author"}]`), sid[:8], nil, nil, false, "", false)
	}()
	select {
	case err := <-done:
		lock.Unlock()
		t.Fatalf("tag-write returned before fence release (err=%v); direct consolidated writes must be fenced", err)
	case <-time.After(300 * time.Millisecond):
		// Still blocked, as required for snapshot-and-replace safety.
	}
	if err := lock.Unlock(); err != nil {
		t.Fatalf("release held lock: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tag-write after fence release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tag-write did not complete after fence release")
	}
}
