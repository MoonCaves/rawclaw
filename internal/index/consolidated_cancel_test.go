package index

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSyncConsolidatedFromContext(t *testing.T) {
	isolateCache(t)
	holder, err := AcquireConsolidatedFence(context.Background())
	if err != nil {
		t.Fatalf("AcquireConsolidatedFence: %v", err)
	}
	defer func() { _ = holder.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- SyncConsolidatedFromContext(ctx, "source.db")
	}()

	// Let the sync reach the fence poll, then cancel it. The held fence makes
	// this independent of source files and proves cancellation during waiting.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SyncConsolidatedFromContext error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SyncConsolidatedFromContext did not stop after cancellation")
	}
}
