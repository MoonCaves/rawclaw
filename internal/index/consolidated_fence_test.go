package index

import (
	"context"
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
