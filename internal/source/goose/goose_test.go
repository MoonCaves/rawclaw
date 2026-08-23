package goose

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/source"
)

// TestGooseSharedDatabase tests the single monolithic sessions.db schema containing
// multiple sessions in a sessions table and messages with session_id predicates.
func TestGooseSharedDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDB := filepath.Join(tmpDir, "sessions.db")

	db, err := sql.Open("sqlite", sessionsDB)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Create Goose shared schema
	_, err = db.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			working_dir TEXT,
			parent_id TEXT,
			is_subagent INTEGER
		);
		CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			role TEXT,
			content TEXT,
			created_at TEXT
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	// Insert 2 distinct sessions
	_, err = db.Exec(`
		INSERT INTO sessions (id, working_dir, parent_id, is_subagent) VALUES
			('sess-alpha-1234', '/workspace/alpha', '', 0),
			('sess-beta-5678', '/workspace/beta', 'sess-alpha-1234', 1);

		INSERT INTO messages (id, session_id, role, content, created_at) VALUES
			('m1', 'sess-alpha-1234', 'user', 'How to deploy to production?', '2026-08-21T10:00:00Z'),
			('m2', 'sess-alpha-1234', 'assistant', 'Use the release workflow.', '2026-08-21T10:00:05Z'),
			('m3', 'sess-beta-5678', 'user', 'Investigate alpha deployment logs', '2026-08-21T10:01:00Z'),
			('m4', 'sess-beta-5678', 'assistant', 'Found zero errors.', '2026-08-21T10:01:10Z');
	`)
	if err != nil {
		t.Fatalf("insert records: %v", err)
	}

	adapter := NewRoot(tmpDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(containers) != 2 {
		t.Fatalf("got %d containers, want 2", len(containers))
	}

	// Map containers by ID
	byID := make(map[string]source.Container)
	for _, c := range containers {
		byID[c.ID] = c
	}

	c1, ok1 := byID["sess-alpha-1234"]
	if !ok1 {
		t.Fatalf("missing container for sess-alpha-1234")
	}
	if c1.CWD != "/workspace/alpha" || c1.IsSubagent || c1.ParentID != "" {
		t.Errorf("unexpected c1 metadata: %+v", c1)
	}

	// Test Messages extraction filters strictly by session_id
	msgs1, err := adapter.Messages(c1)
	if err != nil {
		t.Fatalf("Messages c1 failed: %v", err)
	}
	if len(msgs1) != 2 {
		t.Fatalf("c1 messages count = %d, want 2 (must not bleed sess-beta messages)", len(msgs1))
	}
	if msgs1[0].Text != "How to deploy to production?" || msgs1[0].Role != "user" {
		t.Errorf("unexpected msgs1[0]: %+v", msgs1[0])
	}
	if msgs1[1].Text != "Use the release workflow." || msgs1[1].Role != "assistant" {
		t.Errorf("unexpected msgs1[1]: %+v", msgs1[1])
	}

	// Verify c2
	c2, ok2 := byID["sess-beta-5678"]
	if !ok2 {
		t.Fatalf("missing container for sess-beta-5678")
	}
	if c2.CWD != "/workspace/beta" || !c2.IsSubagent || c2.ParentID != "sess-alpha-1234" {
		t.Errorf("unexpected c2 metadata: %+v", c2)
	}

	msgs2, err := adapter.Messages(c2)
	if err != nil {
		t.Fatalf("Messages c2 failed: %v", err)
	}
	if len(msgs2) != 2 {
		t.Fatalf("c2 messages count = %d, want 2", len(msgs2))
	}
	if msgs2[0].Text != "Investigate alpha deployment logs" {
		t.Errorf("unexpected msgs2[0]: %+v", msgs2[0])
	}
}

// TestGooseStandaloneDatabase tests single-session standalone database files.
func TestGooseStandaloneDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	sessPath := filepath.Join(tmpDir, "standalone-session-1.db")

	db, err := sql.Open("sqlite", sessPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE session_meta (key TEXT PRIMARY KEY, value TEXT);
		INSERT INTO session_meta (key, value) VALUES
			('cwd', '/workspace/myproject'),
			('id', 'custom-sess-id-999');

		CREATE TABLE messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT,
			content TEXT,
			created_at TEXT
		);
		INSERT INTO messages (role, content, created_at) VALUES
			('user', 'Run unit tests', '2026-08-21T11:00:00Z'),
			('assistant', 'All 10 tests passed', '2026-08-21T11:00:05Z');
	`)
	if err != nil {
		t.Fatalf("setup standalone db: %v", err)
	}

	adapter := NewRoot(tmpDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("got %d containers, want 1", len(containers))
	}

	c := containers[0]
	if c.ID != "custom-sess-id-999" || c.CWD != "/workspace/myproject" {
		t.Errorf("unexpected container: %+v", c)
	}

	msgs, err := adapter.Messages(c)
	if err != nil {
		t.Fatalf("Messages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Text != "Run unit tests" {
		t.Errorf("unexpected msg 0: %+v", msgs[0])
	}
}

// TestGooseJSONContentExtraction tests parsing structured JSON content payloads.
func TestGooseJSONContentExtraction(t *testing.T) {
	rawJSON := `[{"text": "Hello world"}, {"text": "How can I help?"}]`
	extracted := extractContent(rawJSON)
	want := "Hello world\nHow can I help?"
	if extracted != want {
		t.Errorf("extractContent = %q, want %q", extracted, want)
	}

	rawObj := `{"content": "Single content payload"}`
	if got := extractContent(rawObj); got != "Single content payload" {
		t.Errorf("extractContent object = %q, want %q", got, "Single content payload")
	}
}

// TestGooseTimestampParsing tests various timestamp format parsings.
func TestGooseTimestampParsing(t *testing.T) {
	ts, iso := parseTimestamp("2026-08-21T10:00:00Z")
	if iso != "2026-08-21T10:00:00Z" || ts == 0 {
		t.Errorf("parseTimestamp RFC3339 failed: ts=%f iso=%s", ts, iso)
	}

	ts2, _ := parseTimestamp(int64(1750000000000)) // millis
	if ts2 != 1750000000 {
		t.Errorf("parseTimestamp millis = %f, want 1750000000", ts2)
	}
}

// TestGooseEndToEndIndexing tests indexing containers into a RawClaw cache database.
func TestGooseEndToEndIndexing(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDB := filepath.Join(tmpDir, "sessions.db")

	db, err := sql.Open("sqlite", sessionsDB)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, _ = db.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY, working_dir TEXT);
		CREATE TABLE messages (id TEXT PRIMARY KEY, session_id TEXT, role TEXT, content TEXT, created_at TEXT);
		INSERT INTO sessions VALUES ('s1', '/tmp/project');
		INSERT INTO messages VALUES ('m1', 's1', 'user', 'Search keyword alpha beta', '2026-08-21T12:00:00Z');
	`)

	adapter := NewRoot(tmpDir)
	containers, err := adapter.Discover()
	if err != nil || len(containers) != 1 {
		t.Fatalf("discover failed: %v, len=%d", err, len(containers))
	}

	// Verify MessagesFunc interface compatibility
	var fn source.Source = adapter
	msgs, err := fn.Messages(containers[0])
	if err != nil || len(msgs) != 1 {
		t.Fatalf("Messages failed: %v, len=%d", err, len(msgs))
	}
}
