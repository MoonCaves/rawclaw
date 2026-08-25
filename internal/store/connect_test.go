package store_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

func TestConnectROPragmas(t *testing.T) {
	_, dbp := storetest.NewDB(t)

	ro, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("ConnectRO: %v", err)
	}
	defer ro.Close()

	var mmapSize int64
	if err := ro.QueryRow("PRAGMA mmap_size").Scan(&mmapSize); err != nil {
		t.Fatalf("PRAGMA mmap_size: %v", err)
	}
	if mmapSize <= 0 {
		t.Errorf("PRAGMA mmap_size = %d, want > 0", mmapSize)
	}

	var busyTimeout int
	if err := ro.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("PRAGMA busy_timeout = %d, want 5000", busyTimeout)
	}
}

// TestConnectRO_ConcurrentWriteSafety verifies that a read-only connection with
// tuned pragmas (mmap_size, mode=ro) safely queries the database while a background
// writer writes transactions in WAL mode without errors or data corruption.
func TestConnectRO_ConcurrentWriteSafety(t *testing.T) {
	conRW, dbp := storetest.NewDB(t)

	stopWriters := make(chan struct{})
	var wg sync.WaitGroup

	// Background writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stopWriters:
				return
			default:
				i++
				sid := fmt.Sprintf("concurrent-sess-%d", i)
				storetest.InsertSession(t, conRW, storetest.Session{
					ID:           sid,
					StartedAt:    float64(1700000000 + i),
					LastTS:       float64(1700000000 + i),
					MessageCount: 1,
					Project:      "concurrent",
					CWD:          "/workspace/concurrent",
				})
				storetest.InsertMessage(t, conRW, storetest.Message{
					SessionID: sid,
					Role:      "user",
					Content:   fmt.Sprintf("concurrent message payload %d for testing read-write safety", i),
					TS:        float64(1700000000 + i),
					ISO:       "2026-08-25T10:00:00Z",
					UUID:      fmt.Sprintf("uuid-%d", i),
				})
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	// Concurrent readers
	numReaders := 4
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ro, err := store.ConnectRO(dbp)
				if err != nil {
					t.Errorf("reader %d: ConnectRO: %v", readerID, err)
					return
				}

				// Run search and browse queries
				_, sErr := store.SearchHits(ro, "payload", store.Filter{}, store.SortRelevance, 10)
				if sErr != nil {
					t.Errorf("reader %d: SearchHits: %v", readerID, sErr)
				}

				_, bErr := store.BrowseSessions(ro, "", "", 10)
				if bErr != nil {
					t.Errorf("reader %d: BrowseSessions: %v", readerID, bErr)
				}

				_ = ro.Close()
				time.Sleep(2 * time.Millisecond)
			}
		}(r)
	}

	// Wait for readers to finish
	time.Sleep(200 * time.Millisecond)
	close(stopWriters)
	wg.Wait()
}
