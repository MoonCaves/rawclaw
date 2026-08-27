package index

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
)

func TestConsolidatedFence_ReportsHolderOnceAfterThreshold(t *testing.T) {
	isolateCache(t)
	path := filepath.Join(store.CacheDir(), "consolidated.db")
	if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("create consolidated placeholder: %v", err)
	}

	holder := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated lock: locked=%t err=%v", locked, err)
	}
	defer holder.Unlock()

	oldThreshold := consolidatedLockWaitThreshold
	oldLsof := runConsolidatedLsof
	consolidatedLockWaitThreshold = time.Millisecond
	calls := 0
	runConsolidatedLsof = func(paths []string) ([]byte, error) {
		calls++
		if len(paths) != 1 || paths[0] != path {
			t.Errorf("lsof paths = %v, want [%q]", paths, path)
		}
		return []byte("cgoose.test\ncrawclaw\n"), nil
	}
	defer func() {
		consolidatedLockWaitThreshold = oldThreshold
		runConsolidatedLsof = oldLsof
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := AcquireConsolidatedFence(ctx); err == nil {
		t.Fatal("acquireConsolidatedFence succeeded while another writer held the lock")
	}
	if calls != 1 {
		t.Fatalf("lsof calls = %d, want 1", calls)
	}
	if got := lsofCommands([]byte("cgoose.test\ncrawclaw\n")); !strings.Contains(got, "goose.test") {
		t.Fatalf("holder processes = %q, want goose.test", got)
	}
}

// TestIsBusy_RecognizesConsolidatedFenceTimeout proves isBusy treats a real
// AcquireConsolidatedFence timeout the same as a SQLite busy/locked error —
// tag-prep's fold-deferral (internal/cli/tagrefresh.go) depends on this to
// tell "store contended, try later" apart from a genuine fold failure.
func TestIsBusy_RecognizesConsolidatedFenceTimeout(t *testing.T) {
	isolateCache(t)

	holder := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated lock: locked=%t err=%v", locked, err)
	}
	defer holder.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, fenceErr := AcquireConsolidatedFence(ctx)
	if fenceErr == nil {
		t.Fatal("AcquireConsolidatedFence succeeded while another writer held the lock")
	}
	if !isBusy(fenceErr) {
		t.Fatalf("isBusy(%v) = false, want true (a fence-wait timeout is a busy/contended store)", fenceErr)
	}
}

func TestConsolidatedFence_LogsAcquireDurationOnTimeout(t *testing.T) {
	isolateCache(t)
	holder := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated lock: locked=%t err=%v", locked, err)
	}
	defer holder.Unlock()

	recorder := &testLogRecorder{}
	original := slog.Default()
	slog.SetDefault(slog.New(recorder))
	defer slog.SetDefault(original)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := AcquireConsolidatedFence(ctx); err == nil {
		t.Fatal("AcquireConsolidatedFence succeeded while another writer held the lock")
	}

	recorder.mu.Lock()
	records := append([]slog.Record(nil), recorder.records...)
	recorder.mu.Unlock()
	var started, completed bool
	for _, rec := range records {
		if rec.Message != "consolidated fence phase" {
			continue
		}
		var phase, event string
		var duration slog.Value
		rec.Attrs(func(attr slog.Attr) bool {
			switch attr.Key {
			case "phase":
				phase = attr.Value.String()
			case "event":
				event = attr.Value.String()
			case "duration":
				duration = attr.Value
			}
			return true
		})
		if phase == "acquire" {
			started = started || event == "start"
			completed = completed || duration.Kind() == slog.KindDuration
		}
	}
	if !started || !completed {
		t.Fatalf("failed acquire logs: started=%t completed=%t, want both", started, completed)
	}
}
