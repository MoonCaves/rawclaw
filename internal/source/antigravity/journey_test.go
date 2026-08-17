package antigravity

import (
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestAntigravityRecallJourney exercises a complete end-to-end user recall journey:
// 1. Discovering transcripts from a custom root
// 2. Extracting normalized messages and tool summaries
// 3. Indexing into a local SQLite store
// 4. Performing incremental append and verifying watermark updates
func TestAntigravityRecallJourney(t *testing.T) {
	tmp := t.TempDir()
	dbp := filepath.Join(tmp, "test_antigravity.db")

	// Seed history.jsonl
	writeJSONL(t, filepath.Join(tmp, "history.jsonl"),
		`{"conversationId":"session-alpha","workspace":"/workspace/app"}`,
	)

	// Seed session-alpha transcript
	tAlpha := filepath.Join(tmp, "brain", "session-alpha", ".system_generated", "logs", "transcript_full.jsonl")
	writeJSONL(t, tAlpha,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>optimize the postgres query</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:05Z","thinking":"analyzing index","content":"Running explain analyze","tool_calls":[{"name":"run_command","args":{"CommandLine":"psql -c 'EXPLAIN ANALYZE SELECT * FROM users;'"}}]}`,
		`{"step_index":2,"source":"MODEL","type":"RUN_COMMAND","created_at":"2026-08-15T10:00:10Z","content":"Seq Scan on users  (cost=0.00..35.50 rows=2550 width=128)"}`,
	)

	ad := NewRoot(tmp)
	containers, err := ad.Discover()
	if err != nil {
		t.Fatalf("Discover() failed: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("Discover() returned %d containers, want 1", len(containers))
	}

	// 1. Initial Indexing
	nSessions, status, err := index.EnsureIndexedContainers(dbp, false, containers, ad.Messages, ID, "")
	if err != nil {
		t.Fatalf("EnsureIndexedContainers failed: %v", err)
	}
	if nSessions != 1 || status != index.IndexFresh {
		t.Errorf("EnsureIndexedContainers = (%d, %v), want (1, %v)", nSessions, status, index.IndexFresh)
	}

	// Verify database content
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("ConnectRO failed: %v", err)
	}
	defer con.Close()

	var msgCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='session-alpha'").Scan(&msgCount); err != nil {
		t.Fatalf("count messages failed: %v", err)
	}
	if msgCount != 3 {
		t.Errorf("messages count = %d, want 3", msgCount)
	}

	// 2. Incremental Append: append a new step to the transcript
	writeJSONL(t, tAlpha,
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":"2026-08-15T10:00:00Z","content":"<USER_REQUEST>optimize the postgres query</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:05Z","thinking":"analyzing index","content":"Running explain analyze","tool_calls":[{"name":"run_command","args":{"CommandLine":"psql -c 'EXPLAIN ANALYZE SELECT * FROM users;'"}}]}`,
		`{"step_index":2,"source":"MODEL","type":"RUN_COMMAND","created_at":"2026-08-15T10:00:10Z","content":"Seq Scan on users  (cost=0.00..35.50 rows=2550 width=128)"}`,
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-08-15T10:00:15Z","thinking":"creating btree index","content":"Created index on users(email)"}`,
	)

	// Re-run indexing with new step
	containers2, err := ad.Discover()
	if err != nil {
		t.Fatalf("Discover() 2 failed: %v", err)
	}
	_, _, err = index.EnsureIndexedContainers(dbp, false, containers2, ad.Messages, ID, "")
	if err != nil {
		t.Fatalf("EnsureIndexedContainers 2 failed: %v", err)
	}

	// Verify message count is now 4 and no duplicates
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='session-alpha'").Scan(&msgCount); err != nil {
		t.Fatalf("count messages 2 failed: %v", err)
	}
	if msgCount != 4 {
		t.Errorf("messages count after append = %d, want 4", msgCount)
	}
}
