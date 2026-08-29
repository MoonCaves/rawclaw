package opencode

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/MoonCaves/rawclaw/internal/source"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/Users/jay-m4/.local/share/opencode/opencode.db", true},
		{"/Users/jay-m4/.local/share/opencode/opencode.db#ses_123", true},
		{"/data/opencode/sessions.db", true},
		{"/home/user/.claude/projects/proj/session.jsonl", false},
		{"/tmp/test.txt", false},
	}

	for _, tc := range cases {
		got := detect(tc.path)
		if got != tc.want {
			t.Errorf("detect(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func setupTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	schema := `
	CREATE TABLE session (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		parent_id TEXT,
		slug TEXT NOT NULL,
		directory TEXT NOT NULL,
		title TEXT NOT NULL,
		version TEXT NOT NULL,
		time_created INTEGER NOT NULL,
		time_updated INTEGER NOT NULL
	);
	CREATE TABLE message (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		time_created INTEGER NOT NULL,
		time_updated INTEGER NOT NULL,
		data TEXT NOT NULL
	);
	CREATE TABLE part (
		id TEXT PRIMARY KEY,
		message_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		time_created INTEGER NOT NULL,
		time_updated INTEGER NOT NULL,
		data TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	nowMs := time.Now().UnixMilli()

	// Insert root session
	_, err = db.Exec(`INSERT INTO session (id, project_id, parent_id, slug, directory, title, version, time_created, time_updated)
		VALUES ('ses_root1', 'proj1', NULL, 'root-slug', '/workspace/app', 'Root Session', '1.0', ?, ?)`, nowMs, nowMs)
	if err != nil {
		t.Fatalf("insert root session: %v", err)
	}

	// Insert subagent session
	_, err = db.Exec(`INSERT INTO session (id, project_id, parent_id, slug, directory, title, version, time_created, time_updated)
		VALUES ('ses_sub1', 'proj1', 'ses_root1', 'sub-slug', '/workspace/app', 'Subagent Session', '1.0', ?, ?)`, nowMs+1000, nowMs+1000)
	if err != nil {
		t.Fatalf("insert sub session: %v", err)
	}

	// Insert messages for root session
	_, err = db.Exec(`INSERT INTO message (id, session_id, time_created, time_updated, data)
		VALUES ('msg_1', 'ses_root1', ?, ?, '{"role":"user"}')`, nowMs+100, nowMs+100)
	if err != nil {
		t.Fatalf("insert msg 1: %v", err)
	}

	_, err = db.Exec(`INSERT INTO message (id, session_id, time_created, time_updated, data)
		VALUES ('msg_2', 'ses_root1', ?, ?, '{"role":"assistant"}')`, nowMs+200, nowMs+200)
	if err != nil {
		t.Fatalf("insert msg 2: %v", err)
	}

	// Insert parts for msg_1 (user text)
	_, err = db.Exec(`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
		VALUES ('prt_1', 'msg_1', 'ses_root1', ?, ?, '{"type":"text","text":"Please check the codebase"}')`, nowMs+110, nowMs+110)
	if err != nil {
		t.Fatalf("insert part 1: %v", err)
	}

	// Insert parts for msg_2 (reasoning + tool call + text)
	_, err = db.Exec(`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
		VALUES ('prt_2', 'msg_2', 'ses_root1', ?, ?, '{"type":"reasoning","text":"I should check the files."}')`, nowMs+210, nowMs+210)
	if err != nil {
		t.Fatalf("insert part 2: %v", err)
	}

	_, err = db.Exec(`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
		VALUES ('prt_3', 'msg_2', 'ses_root1', ?, ?, '{"type":"tool","tool":"read_file","state":{"status":"completed","input":{"path":"main.go"},"output":"package main"}}')`, nowMs+220, nowMs+220)
	if err != nil {
		t.Fatalf("insert part 3: %v", err)
	}

	_, err = db.Exec(`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
		VALUES ('prt_4', 'msg_2', 'ses_root1', ?, ?, '{"type":"text","text":"Found main.go with package main."}')`, nowMs+230, nowMs+230)
	if err != nil {
		t.Fatalf("insert part 4: %v", err)
	}

	return dbPath
}

func TestDiscoverAndMessages(t *testing.T) {
	dbPath := setupTestDB(t)
	dir := filepath.Dir(dbPath)

	adapter := NewRoots(dir)

	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover() err: %v", err)
	}

	if len(containers) != 2 {
		t.Fatalf("got %d containers, want 2", len(containers))
	}

	rootC := containers[0]
	if rootC.ID != "ses_root1" {
		t.Errorf("root container ID = %q, want ses_root1", rootC.ID)
	}
	if rootC.CWD != "/workspace/app" {
		t.Errorf("root container CWD = %q, want /workspace/app", rootC.CWD)
	}
	if rootC.IsSubagent {
		t.Errorf("root container should not be subagent")
	}
	if rootC.ParentID != "" {
		t.Errorf("root container ParentID = %q, want empty", rootC.ParentID)
	}

	subC := containers[1]
	if subC.ID != "ses_sub1" {
		t.Errorf("sub container ID = %q, want ses_sub1", subC.ID)
	}
	if !subC.IsSubagent {
		t.Errorf("sub container should be subagent")
	}
	if subC.ParentID != "ses_root1" {
		t.Errorf("sub container ParentID = %q, want ses_root1", subC.ParentID)
	}

	// Test Messages extraction
	msgs, err := adapter.Messages(rootC)
	if err != nil {
		t.Fatalf("Messages(rootC) err: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}

	if msgs[0].Role != "user" {
		t.Errorf("msg[0] role = %q, want user", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Text, "Please check the codebase") {
		t.Errorf("msg[0] content = %q, missing user prompt", msgs[0].Text)
	}

	if msgs[1].Role != "assistant" {
		t.Errorf("msg[1] role = %q, want assistant", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Text, "[THINKING] I should check the files.") {
		t.Errorf("msg[1] content missing thinking block: %q", msgs[1].Text)
	}
	if !strings.Contains(msgs[1].Text, "[tool_use: read_file]") || !strings.Contains(msgs[1].Text, "[tool_result: read_file] package main") {
		t.Errorf("msg[1] content missing tool block: %q", msgs[1].Text)
	}
	if !strings.Contains(msgs[1].Text, "Found main.go with package main.") {
		t.Errorf("msg[1] content missing assistant text: %q", msgs[1].Text)
	}

}

func TestLookup(t *testing.T) {
	dbPath := setupTestDB(t)
	c, ok := lookupSessionInDB(dbPath, "ses_root1")
	if !ok {
		t.Fatal("lookupSessionInDB failed for ses_root1")
	}
	if c.ID != "ses_root1" || c.CWD != "/workspace/app" {
		t.Errorf("lookup result = %+v", c)
	}

	_, ok = lookupSessionInDB(dbPath, "non_existent")
	if ok {
		t.Fatal("lookupSessionInDB succeeded for non_existent session")
	}
}

func TestResumeArgv(t *testing.T) {
	argv := source.ResumeArgv(ID, "ses_12345")
	if len(argv) != 3 || argv[0] != "opencode" || argv[1] != "--session" || argv[2] != "ses_12345" {
		t.Errorf("ResumeArgv = %v, want [opencode --session ses_12345]", argv)
	}
}
