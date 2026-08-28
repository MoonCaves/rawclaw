package goose

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestGooseSharedDatabase tests the single monolithic sessions.db schema containing
// multiple sessions in a sessions table and messages with session_id predicates.

// isolateHome points HOME (and the XDG dirs) at a temp dir so nothing in these
// tests can reach the real user cache. EnsureIndexedContainers write-through
// opens the REAL consolidated store otherwise — a gate run was caught holding
// a 2.6GB production store open for the length of the goose suite.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", home+"/.cache")
	t.Setenv("XDG_DATA_HOME", home+"/.local/share")
}

func TestGooseSharedDatabase(t *testing.T) {
	isolateHome(t)
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
	isolateHome(t)
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

func TestGooseStandaloneMetadataAndLookup(t *testing.T) {
	isolateHome(t)
	tmpDir := t.TempDir()
	sessPath := filepath.Join(tmpDir, "on-disk-name.db")
	db, err := sql.Open("sqlite", sessPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE metadata (key TEXT PRIMARY KEY, val TEXT);
		INSERT INTO metadata (key, val) VALUES
			('id', 'metadata-session'),
			('cwd', NULL),
			('parent_id', 'parent-session'),
			('is_subagent', 'true');
		CREATE TABLE messages (id INTEGER PRIMARY KEY, role TEXT, content TEXT);
		INSERT INTO messages (role, content) VALUES ('user', 'hello');
	`)
	if err != nil {
		db.Close()
		t.Fatalf("setup database: %v", err)
	}
	db.Close()

	adapter := NewRoot(tmpDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("Discover returned %d containers, want 1: %+v", len(containers), containers)
	}
	c := containers[0]
	if c.ID != "metadata-session" || c.CWD != "" || c.ParentID != "parent-session" || !c.IsSubagent {
		t.Fatalf("metadata was not preserved: %+v", c)
	}

	t.Setenv("GOOSE_HOME", tmpDir)
	got, err := lookup("metadata-session")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 1 || got[0].ID != "metadata-session" || got[0].ParentID != "parent-session" || !got[0].IsSubagent {
		t.Fatalf("lookup metadata mismatch: %+v", got)
	}
	if got, err := lookup("on-disk-name"); err != nil || len(got) != 0 {
		t.Fatalf("lookup returned ghost filename identity: got=%+v err=%v", got, err)
	}
}

func TestGooseLookupPreservesParentWithoutSubagentFlag(t *testing.T) {
	isolateHome(t)
	tmpDir := t.TempDir()
	sessPath := filepath.Join(tmpDir, "sessions.db")
	db, err := sql.Open("sqlite", sessPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY, working_dir TEXT, parent_id TEXT, is_subagent INTEGER);
		INSERT INTO sessions VALUES ('child-session', NULL, 'parent-session', 0);
	`)
	db.Close()
	if err != nil {
		t.Fatalf("setup database: %v", err)
	}

	t.Setenv("GOOSE_HOME", tmpDir)
	got, err := lookup("child-session")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("lookup returned %d containers, want 1: %+v", len(got), got)
	}
	if got[0].ParentID != "parent-session" || got[0].IsSubagent {
		t.Fatalf("lookup did not preserve parent-only metadata: %+v", got[0])
	}
}

func TestGooseDiscoverySkipsUnrelatedDatabase(t *testing.T) {
	isolateHome(t)
	tmpDir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "unrelated.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE unrelated (value TEXT)"); err != nil {
		db.Close()
		t.Fatalf("create unrelated table: %v", err)
	}
	db.Close()

	got, err := NewRoot(tmpDir).Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("unrelated database produced ghost containers: %+v", got)
	}
}

func TestGooseLookupMissingRootIsEmpty(t *testing.T) {
	t.Setenv("GOOSE_HOME", filepath.Join(t.TempDir(), "missing"))
	got, err := lookup("missing-session")
	if err != nil {
		t.Fatalf("lookup missing root: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("lookup missing root returned containers: %+v", got)
	}
}

// TestGooseJSONContentExtraction tests parsing structured JSON content payloads.
func TestGooseJSONContentExtraction(t *testing.T) {
	isolateHome(t)
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
	isolateHome(t)
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
	isolateHome(t)
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

// TestGooseRealSchema_DiscoveredAndIndexed tests the exact upstream Block/AAIF Goose v1.10.0+
// SQLite schema (sessions: id, working_dir, name, user_set_name, session_type, created_at;
// messages: id, session_id, role, content JSON array, timestamp) and verifies discovery,
// message ordering, MCP block text extraction, CWD mapping, subagent defaults, and resume argv.
func TestGooseRealSchema_DiscoveredAndIndexed(t *testing.T) {
	isolateHome(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sessionsDB := filepath.Join(sessionsDir, "sessions.db")

	db, err := sql.Open("sqlite", sessionsDB)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Upstream Block/AAIF Goose v1.10.0+ schema
	_, err = db.Exec(`
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
	`)
	if err != nil {
		t.Fatalf("create real goose schema: %v", err)
	}

	// Insert realistic sessions with MCP JSON content
	_, err = db.Exec(`
		INSERT INTO sessions (id, working_dir, name, user_set_name, session_type, created_at) VALUES
			('01918a3b-745a-7140-84a1-000000000001', '/Users/alice/projects/api-service', 'investigate-latency', 1, 'user', '2026-08-25 10:00:00'),
			('01918a3b-745a-7140-84a1-000000000002', '/Users/alice/projects/web-ui', 'fix-nav-header', 0, 'user', '2026-08-25 11:00:00');

		INSERT INTO messages (id, session_id, role, content, timestamp) VALUES
			('msg-001', '01918a3b-745a-7140-84a1-000000000001', 'user', '[{"type":"text","text":"Why is /api/v1/checkout taking 800ms?"}]', '2026-08-25 10:00:01'),
			('msg-002', '01918a3b-745a-7140-84a1-000000000001', 'assistant', '[{"type":"text","text":"I analyzed the profile traces."},{"type":"text","text":"The database connection pool is exhausted on checkout calls."}]', '2026-08-25 10:00:15'),
			('msg-003', '01918a3b-745a-7140-84a1-000000000001', 'user', '[{"type":"text","text":"Can you increase the pool limit to 50?"}]', '2026-08-25 10:01:00'),
			('msg-004', '01918a3b-745a-7140-84a1-000000000001', 'assistant', '[{"type":"tool_result","content":"Updated config/db.yaml max_connections: 50"}]', '2026-08-25 10:01:05'),
			('msg-005', '01918a3b-745a-7140-84a1-000000000002', 'user', '[{"type":"text","text":"Fix the header padding on mobile screens"}]', '2026-08-25 11:00:01'),
			('msg-006', '01918a3b-745a-7140-84a1-000000000002', 'assistant', '[{"type":"text","text":"Added responsive media query for header navbar."}]', '2026-08-25 11:00:20');
	`)
	if err != nil {
		t.Fatalf("insert realistic records: %v", err)
	}

	adapter := NewRoot(sessionsDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("got %d containers, want 2", len(containers))
	}

	byID := make(map[string]source.Container)
	for _, c := range containers {
		byID[c.ID] = c
	}

	// Verify Session 1
	c1, ok1 := byID["01918a3b-745a-7140-84a1-000000000001"]
	if !ok1 {
		t.Fatalf("missing container for session 1")
	}
	if c1.CWD != "/Users/alice/projects/api-service" {
		t.Errorf("c1.CWD = %q, want /Users/alice/projects/api-service", c1.CWD)
	}
	if c1.IsSubagent {
		t.Errorf("c1.IsSubagent = true, want false (default)")
	}
	if c1.ParentID != "" {
		t.Errorf("c1.ParentID = %q, want empty", c1.ParentID)
	}
	wantResume := []string{"goose", "session", "--resume", "--session-id", "01918a3b-745a-7140-84a1-000000000001"}
	if len(c1.ResumeArgv) != len(wantResume) || c1.ResumeArgv[3] != "--session-id" || c1.ResumeArgv[4] != "01918a3b-745a-7140-84a1-000000000001" {
		t.Errorf("c1.ResumeArgv = %v, want %v", c1.ResumeArgv, wantResume)
	}

	// Verify Messages extraction for Session 1
	msgs1, err := adapter.Messages(c1)
	if err != nil {
		t.Fatalf("Messages(c1) failed: %v", err)
	}
	if len(msgs1) != 4 {
		t.Fatalf("got %d messages for session 1, want 4", len(msgs1))
	}

	// Check roles and parsed content
	if msgs1[0].Role != "user" || msgs1[0].Text != "Why is /api/v1/checkout taking 800ms?" {
		t.Errorf("unexpected msgs1[0]: role=%q text=%q", msgs1[0].Role, msgs1[0].Text)
	}
	wantMsg2 := "I analyzed the profile traces.\nThe database connection pool is exhausted on checkout calls."
	if msgs1[1].Role != "assistant" || msgs1[1].Text != wantMsg2 {
		t.Errorf("unexpected msgs1[1]: role=%q text=%q", msgs1[1].Role, msgs1[1].Text)
	}
	if msgs1[2].Role != "user" || msgs1[2].Text != "Can you increase the pool limit to 50?" {
		t.Errorf("unexpected msgs1[2]: role=%q text=%q", msgs1[2].Role, msgs1[2].Text)
	}
	if msgs1[3].Role != "assistant" || msgs1[3].Text != "Updated config/db.yaml max_connections: 50" {
		t.Errorf("unexpected msgs1[3]: role=%q text=%q", msgs1[3].Role, msgs1[3].Text)
	}

	// Verify ordering is strictly ascending by timestamp
	for i := 1; i < len(msgs1); i++ {
		if msgs1[i].TS < msgs1[i-1].TS {
			t.Errorf("messages not in ascending timestamp order: msgs[%d].TS (%f) < msgs[%d].TS (%f)", i, msgs1[i].TS, i-1, msgs1[i-1].TS)
		}
	}

	// Verify Session 2 isolation
	c2, ok2 := byID["01918a3b-745a-7140-84a1-000000000002"]
	if !ok2 {
		t.Fatalf("missing container for session 2")
	}
	if c2.CWD != "/Users/alice/projects/web-ui" {
		t.Errorf("c2.CWD = %q, want /Users/alice/projects/web-ui", c2.CWD)
	}
	msgs2, err := adapter.Messages(c2)
	if err != nil {
		t.Fatalf("Messages(c2) failed: %v", err)
	}
	if len(msgs2) != 2 {
		t.Fatalf("got %d messages for session 2, want 2", len(msgs2))
	}
}

// TestGooseSQLInjectionPrevention verifies that dynamic table and column identifiers
// are validated against SQL injection patterns before formatting into queries.
func TestGooseSQLInjectionPrevention(t *testing.T) {
	isolateHome(t)
	// 1. Validate isSafeIdent behavior
	unsafeIdents := []string{
		"messages; DROP TABLE messages; --",
		"session_meta WHERE 1=1",
		"id, (SELECT password FROM users)",
		"name\" OR '1'='1",
		"' OR 1=1 --",
		"table-name",
		"table name",
		"col;SELECT 1",
		"",
		"id`",
		"id[",
		"id]",
		"id\x00",
		"123",
		"1abc",
	}
	for _, ident := range unsafeIdents {
		if isSafeIdent(ident) {
			t.Errorf("isSafeIdent(%q) = true, want false", ident)
		}
	}

	safeIdents := []string{
		"sessions",
		"messages",
		"created_at",
		"session_id",
		"working_dir",
		"id",
		"rowid",
		"_id",
		"session123",
	}
	for _, ident := range safeIdents {
		if !isSafeIdent(ident) {
			t.Errorf("isSafeIdent(%q) = false, want true", ident)
		}
	}

	// 2. Test tableColumns with malicious table name
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE messages (id TEXT PRIMARY KEY, content TEXT);`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	// An unsanitized table name must be rejected and return nil without executing PRAGMA
	cols, err := tableColumns(db, "messages; DROP TABLE messages; --")
	if err != nil {
		t.Fatalf("unexpected error for unsafe table name: %v", err)
	}
	if cols != nil {
		t.Errorf("tableColumns with injected SQL returned %v, want nil", cols)
	}

	// 3. Test with a table containing an injected column name
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS chat ("role" TEXT, "content" TEXT, "sess; DROP TABLE foo; --" TEXT);`); err != nil {
		t.Fatalf("create chat table: %v", err)
	}
	chatCols, err := tableColumns(db, "chat")
	if err != nil {
		t.Fatalf("tableColumns(chat) failed: %v", err)
	}
	for _, col := range chatCols {
		if !isSafeIdent(col) {
			t.Errorf("tableColumns returned unsanitized column %q", col)
		}
	}
}

// TestGooseMessages_RowsErr_TruncationError verifies that an iteration failure
// during rows.Next() in Messages() returns an error rather than silently returning
// a truncated list of messages.
func TestGooseMessages_RowsErr_TruncationError(t *testing.T) {
	isolateHome(t)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sessions.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Create tables with an index so SQLite streams row 1 before encountering the error on row 2.
	_, err = db.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			working_dir TEXT
		);
		CREATE TABLE raw_messages (
			id INTEGER PRIMARY KEY,
			session_id TEXT,
			role TEXT,
			raw_payload TEXT,
			created_at TEXT
		);
		CREATE INDEX idx_sess_time ON raw_messages(session_id, created_at);

		INSERT INTO sessions (id, working_dir) VALUES
			('sess-corrupt', '/workspace/corrupt');
		INSERT INTO raw_messages (id, session_id, role, raw_payload, created_at) VALUES
			(1, 'sess-corrupt', 'user', 'valid message 1', '2026-08-25T10:00:00Z'),
			(2, 'sess-corrupt', 'assistant', '{"bad json', '2026-08-25T10:00:05Z');

		CREATE VIEW messages AS
		SELECT
			id,
			session_id,
			role,
			CASE WHEN id = 2 THEN json_extract(raw_payload, '$.valid') ELSE raw_payload END AS content,
			created_at
		FROM raw_messages;
	`)
	if err != nil {
		t.Fatalf("setup database: %v", err)
	}

	adapter := NewRoot(tmpDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("got %d containers, want 1", len(containers))
	}

	msgs, err := adapter.Messages(containers[0])
	if err == nil {
		t.Fatalf("expected error on mid-iteration rows error, got nil error with %d messages (silent truncation)", len(msgs))
	}
	if msgs != nil {
		t.Fatalf("expected nil messages on error, got %d messages", len(msgs))
	}
}

type gooseRowsStub struct {
	rows   [][]any
	index  int
	rowErr error
}

func (s *gooseRowsStub) Next() bool {
	if s.index >= len(s.rows) {
		return false
	}
	s.index++
	return true
}

func (s *gooseRowsStub) Scan(dest ...any) error {
	row := s.rows[s.index-1]
	for i := range dest {
		*(dest[i].(*any)) = row[i]
	}
	return nil
}

func (s *gooseRowsStub) Err() error { return s.rowErr }

func TestSessionContainersFromRows_DoesNotFallThroughOnRowsError(t *testing.T) {
	isolateHome(t)
	rowsErr := errors.New("simulated sqlite iteration failure")
	rows := &gooseRowsStub{
		rows:   [][]any{{"partial-session", "/workspace", "", false}},
		rowErr: rowsErr,
	}

	containers, gotErr := sessionContainersFromRows(rows, "/tmp/sessions.db")
	if gotErr == nil {
		t.Fatal("sessionContainersFromRows returned success after rows.Err")
	}
	if !errors.Is(gotErr, rowsErr) {
		t.Fatalf("sessionContainersFromRows error = %v, want %v", gotErr, rowsErr)
	}
	if containers != nil {
		t.Fatalf("sessionContainersFromRows returned %d partial containers, want nil", len(containers))
	}
}

// TestGooseDiscovery_RowsErr_DoesNotDeleteOmittedDatabaseSessions (Issue #16)
// verifies that when discovery encounters a rows.Err() mid-iteration on one database,
// it returns an error rather than silently omitting that database and returning a partial
// set as success — preventing a caller running --reindex from treating the partial set as
// authoritative and deleting the omitted database's live sessions.
func TestGooseDiscovery_RowsErr_DoesNotDeleteOmittedDatabaseSessions(t *testing.T) {
	isolateHome(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// 1. Setup DB 1 with session sess-healthy-1 sharing CWD /workspace/shared
	db1Path := filepath.Join(sessionsDir, "db1.db")
	db1, err := sql.Open("sqlite", db1Path)
	if err != nil {
		t.Fatalf("open db1: %v", err)
	}
	if _, err := db1.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY, working_dir TEXT);
		CREATE TABLE messages (id TEXT PRIMARY KEY, session_id TEXT, role TEXT, content TEXT, created_at TEXT);
		INSERT INTO sessions VALUES ('sess-healthy-1', '/workspace/shared');
		INSERT INTO messages VALUES ('m1', 'sess-healthy-1', 'user', 'Healthy session msg', '2026-08-25T10:00:00Z');
	`); err != nil {
		db1.Close()
		t.Fatalf("init db1: %v", err)
	}
	db1.Close()

	// 2. Setup DB 2 initially with valid session sess-db2-1 sharing CWD /workspace/shared
	db2Path := filepath.Join(sessionsDir, "db2.db")
	db2, err := sql.Open("sqlite", db2Path)
	if err != nil {
		t.Fatalf("open db2: %v", err)
	}
	if _, err := db2.Exec(`
		CREATE TABLE raw_sessions (id TEXT PRIMARY KEY, working_dir TEXT, payload TEXT);
		CREATE TABLE messages (id TEXT PRIMARY KEY, session_id TEXT, role TEXT, content TEXT, created_at TEXT);
		INSERT INTO raw_sessions VALUES ('sess-db2-1', '/workspace/shared', '{"valid":true}');
		INSERT INTO messages VALUES ('m2', 'sess-db2-1', 'user', 'DB2 session msg', '2026-08-25T10:00:00Z');
		CREATE VIEW sessions AS SELECT id, working_dir FROM raw_sessions;
	`); err != nil {
		db2.Close()
		t.Fatalf("init db2: %v", err)
	}
	db2.Close()

	// 3. Initial indexing: discover both databases and index into RawClaw store
	adapter := NewRoot(sessionsDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("initial Discover failed: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("initial Discover got %d containers, want 2", len(containers))
	}

	storeDB := filepath.Join(tmpDir, "index_store.db")
	nIndexed, status, err := index.EnsureIndexedContainers(storeDB, false, containers, adapter.Messages, ID, "")
	if err != nil || status != index.IndexFresh || nIndexed != 2 {
		t.Fatalf("initial indexing failed: n=%d status=%v err=%v", nIndexed, status, err)
	}

	// Verify both sessions exist in storeDB
	con, err := store.ConnectRO(storeDB)
	if err != nil {
		t.Fatalf("connect storeDB: %v", err)
	}
	var count1, count2 int
	_ = con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='sess-healthy-1'").Scan(&count1)
	_ = con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='sess-db2-1'").Scan(&count2)
	con.Close()
	if count1 != 1 || count2 != 1 {
		t.Fatalf("initial store state wrong: count1=%d count2=%d", count1, count2)
	}

	// 4. Force a rows.Err() mid-iteration during DB2 session discovery
	db2Corrupt, err := sql.Open("sqlite", db2Path)
	if err != nil {
		t.Fatalf("open db2 for corruption: %v", err)
	}
	if _, err := db2Corrupt.Exec(`
		DROP VIEW sessions;
		INSERT INTO raw_sessions VALUES ('sess-corrupt-row', '/workspace/shared', '{"bad json');
		CREATE VIEW sessions AS
		SELECT
			id,
			CASE WHEN id = 'sess-corrupt-row' THEN json_extract(payload, '$.valid') ELSE working_dir END AS working_dir
		FROM raw_sessions
		ORDER BY id ASC;
	`); err != nil {
		db2Corrupt.Close()
		t.Fatalf("corrupt db2 view: %v", err)
	}
	db2Corrupt.Close()

	// 5. Run discovery against the directory with the failing DB2
	reindexAdapter := NewRoot(sessionsDir)
	discovered, discErr := reindexAdapter.Discover()

	// If discovery falsely succeeds, the caller (e.g. scopes.Goose or cli) proceeds with reindex
	if discErr == nil {
		// Simulating what the caller does on reindex when Discover() returns success:
		// Rebuilds storeDB from the discovered container set
		_, _, _ = index.EnsureIndexedContainers(storeDB, true, discovered, reindexAdapter.Messages, ID, "")
	}

	// 6. Assertions:
	// A: Discover MUST return an error when a database query fails mid-iteration
	if discErr == nil {
		t.Fatalf("Discover returned nil error after rows.Err() mid-iteration in db2 (partial discovery returned as success)")
	}
	if discovered != nil {
		t.Fatalf("Discover returned %d partial containers on error, want nil", len(discovered))
	}

	// B: DB2's session must NOT have been deleted from storeDB
	conAfter, err := store.ConnectRO(storeDB)
	if err != nil {
		t.Fatalf("connect storeDB after: %v", err)
	}
	defer conAfter.Close()

	var countDB2After int
	if err := conAfter.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='sess-db2-1'").Scan(&countDB2After); err != nil {
		t.Fatalf("count db2 sessions after: %v", err)
	}
	if countDB2After == 0 {
		t.Fatalf("DATA LOSS (Issue #16): sess-db2-1 was deleted from index store because discovery silently omitted db2 on rows.Err")
	}
}
