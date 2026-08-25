package goose

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// UpstreamFixture describes the test sessions created by setupUpstreamGooseFixture.
type UpstreamFixture struct {
	DBPath      string
	SessionsDir string
	SessionIDs  []string
}

// setupUpstreamGooseFixture creates a hermetic SQLite database under dir matching
// the exact upstream Block/AAIF Goose v1.10.0+ SQLite schema:
//
//	CREATE TABLE sessions (
//	    id TEXT PRIMARY KEY,
//	    working_dir TEXT NOT NULL,
//	    name TEXT NOT NULL,
//	    user_set_name INTEGER DEFAULT 0,
//	    session_type TEXT DEFAULT 'user',
//	    created_at TIMESTAMP
//	);
//	CREATE TABLE messages (
//	    id TEXT PRIMARY KEY,
//	    session_id TEXT NOT NULL,
//	    role TEXT NOT NULL,
//	    content TEXT NOT NULL,
//	    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
//	);
//
// It populates realistic multi-session data with MCP JSON content structures,
// multi-block messages, tool results, plain-text fallback, and empty sessions.
func setupUpstreamGooseFixture(t *testing.T, baseDir string) UpstreamFixture {
	t.Helper()

	sessionsDir := filepath.Join(baseDir, "goose", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("setupUpstreamGooseFixture mkdir: %v", err)
	}

	dbPath := filepath.Join(sessionsDir, "sessions.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("setupUpstreamGooseFixture open db: %v", err)
	}
	defer db.Close()

	// DDL matching upstream schema
	ddl := `
		PRAGMA journal_mode = WAL;

		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			working_dir TEXT NOT NULL,
			name TEXT NOT NULL,
			user_set_name INTEGER DEFAULT 0,
			session_type TEXT DEFAULT 'user',
			created_at TIMESTAMP
		);

		CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("setupUpstreamGooseFixture exec DDL: %v", err)
	}

	// Insert realistic sessions
	sessionInserts := `
		INSERT INTO sessions (id, working_dir, name, user_set_name, session_type, created_at) VALUES
			('01918a3b-745a-7140-84a1-000000000001', '/Users/alice/projects/api-service', 'investigate-latency', 1, 'user', '2026-08-25 10:00:00'),
			('01918a3b-745a-7140-84a1-000000000002', '/Users/alice/projects/web-ui', 'fix-nav-header', 0, 'user', '2026-08-25 11:00:00'),
			('01918a3b-745a-7140-84a1-000000000003', '/Users/alice/projects/infra', 'wal-tuning', 0, 'system', '2026-08-25 12:00:00'),
			('01918a3b-745a-7140-84a1-000000000004', '/Users/alice/projects/empty', 'empty-session', 0, 'user', '2026-08-25 13:00:00');
	`
	if _, err := db.Exec(sessionInserts); err != nil {
		t.Fatalf("setupUpstreamGooseFixture insert sessions: %v", err)
	}

	// Insert realistic messages covering MCP JSON arrays, single blocks, tool results, plain text
	messageInserts := `
		INSERT INTO messages (id, session_id, role, content, timestamp) VALUES
			('msg-001', '01918a3b-745a-7140-84a1-000000000001', 'user', '[{"type":"text","text":"Why is /api/v1/checkout taking 800ms?"}]', '2026-08-25 10:00:01'),
			('msg-002', '01918a3b-745a-7140-84a1-000000000001', 'assistant', '[{"type":"text","text":"I analyzed the profile traces."},{"type":"text","text":"The database connection pool is exhausted on checkout calls."}]', '2026-08-25 10:00:15'),
			('msg-003', '01918a3b-745a-7140-84a1-000000000001', 'user', '[{"type":"text","text":"Can you increase the pool limit to 50?"}]', '2026-08-25 10:01:00'),
			('msg-004', '01918a3b-745a-7140-84a1-000000000001', 'assistant', '[{"type":"tool_result","content":"Updated config/db.yaml max_connections: 50"}]', '2026-08-25 10:01:05'),

			('msg-005', '01918a3b-745a-7140-84a1-000000000002', 'user', '[{"type":"text","text":"Fix the header padding on mobile screens"}]', '2026-08-25 11:00:01'),
			('msg-006', '01918a3b-745a-7140-84a1-000000000002', 'assistant', '[{"type":"text","text":"Added responsive media query for header navbar."}]', '2026-08-25 11:00:20'),

			('msg-007', '01918a3b-745a-7140-84a1-000000000003', 'system', 'System prompt initialized: You are a SQLite database optimization assistant.', '2026-08-25 12:00:01'),
			('msg-008', '01918a3b-745a-7140-84a1-000000000003', 'user', 'Explain WAL checkpoint behavior in SQLite.', '2026-08-25 12:00:05'),
			('msg-009', '01918a3b-745a-7140-84a1-000000000003', 'assistant', 'WAL mode buffers writes in a separate log file until checkpointed into the main database.', '2026-08-25 12:00:10');
	`
	if _, err := db.Exec(messageInserts); err != nil {
		t.Fatalf("setupUpstreamGooseFixture insert messages: %v", err)
	}

	return UpstreamFixture{
		DBPath:      dbPath,
		SessionsDir: sessionsDir,
		SessionIDs: []string{
			"01918a3b-745a-7140-84a1-000000000001",
			"01918a3b-745a-7140-84a1-000000000002",
			"01918a3b-745a-7140-84a1-000000000003",
			"01918a3b-745a-7140-84a1-000000000004",
		},
	}
}
