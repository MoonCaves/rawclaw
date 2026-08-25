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

	var (
		writerErrMu sync.Mutex
		writerErr   error
	)
	setWriterErr := func(err error) {
		if err != nil {
			writerErrMu.Lock()
			if writerErr == nil {
				writerErr = err
			}
			writerErrMu.Unlock()
		}
	}

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
				if _, err := conRW.Exec(
					"INSERT OR IGNORE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,project,cwd,source_tool) VALUES(?,?,?,?,?,?,?,?,?)",
					sid, float64(1700000000+i), float64(1700000000+i), 1, 0, nil, "concurrent", "/workspace/concurrent", nil,
				); err != nil {
					setWriterErr(fmt.Errorf("insert session %q: %w", sid, err))
					return
				}
				if _, err := conRW.Exec(
					"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
					sid, "user", fmt.Sprintf("concurrent message payload %d for testing read-write safety", i),
					float64(1700000000+i), "2026-08-25T10:00:00Z", fmt.Sprintf("uuid-%d", i),
				); err != nil {
					setWriterErr(fmt.Errorf("insert message: %w", err))
					return
				}
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

	if writerErr != nil {
		t.Errorf("writer error: %v", writerErr)
	}
}
