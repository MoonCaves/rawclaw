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

func holdConsolidatedFence(t *testing.T) *flock.Flock {
	t.Helper()
	holder := flock.New(filepath.Join(store.CacheDir(), "consolidated.lock"))
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold consolidated lock: locked=%t err=%v", locked, err)
	}
	return holder
}

func TestConsolidatedFence_ReportsHolderOnceAfterThreshold(t *testing.T) {
	isolateCache(t)
	path := filepath.Join(store.CacheDir(), "consolidated.db")
	if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("create consolidated placeholder: %v", err)
	}

	holder := holdConsolidatedFence(t)
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

	holder := holdConsolidatedFence(t)
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

// TestConsolidatedFence_LogsAcquireTimeoutDuration proves that a timed-out
// AcquireConsolidatedFence emits an initial event=start marker and a terminal
// duration log recording elapsed wait time (>= timeout), in strict order.
func TestConsolidatedFence_LogsAcquireTimeoutDuration(t *testing.T) {
	isolateCache(t)
	holder := holdConsolidatedFence(t)
	defer holder.Unlock()

	recorder := &testLogRecorder{}
	orig := slog.Default()
	slog.SetDefault(slog.New(recorder))
	defer slog.SetDefault(orig)

	timeout := 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if _, err := AcquireConsolidatedFence(ctx); err == nil {
		t.Fatal("AcquireConsolidatedFence succeeded while another writer held the lock")
	}

	recorder.mu.Lock()
	records := append([]slog.Record(nil), recorder.records...)
	recorder.mu.Unlock()

	startIdx, durationIdx := -1, -1
	var recordedDuration time.Duration
	for i, rec := range records {
		if rec.Message != "consolidated fence phase" {
			continue
		}
		var phase, event string
		var dur slog.Value
		rec.Attrs(func(attr slog.Attr) bool {
			switch attr.Key {
			case "phase":
				phase = attr.Value.String()
			case "event":
				event = attr.Value.String()
			case "duration":
				dur = attr.Value
			}
			return true
		})
		if phase != "acquire" {
			continue
		}
		if event == "start" && startIdx == -1 {
			startIdx = i
		}
		if dur.Kind() == slog.KindDuration && durationIdx == -1 {
			durationIdx = i
			recordedDuration = dur.Duration()
		}
	}
	if startIdx == -1 {
		t.Error("timed-out fence acquisition missing phase=acquire event=start log")
	}
	if durationIdx == -1 {
		t.Error("timed-out fence acquisition missing phase=acquire duration log")
	}
	if startIdx != -1 && durationIdx != -1 && startIdx >= durationIdx {
		t.Errorf("start log (idx=%d) did not precede duration log (idx=%d)", startIdx, durationIdx)
	}
	if recordedDuration < timeout {
		t.Errorf("recorded duration %v is less than wait timeout %v", recordedDuration, timeout)
	}
}
