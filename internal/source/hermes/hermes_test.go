package hermes

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func createTestDB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	schema := `
	CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		cwd TEXT,
		parent_session_id TEXT,
		started_at REAL NOT NULL
	);
	CREATE TABLE messages (
		id INTEGER PRIMARY KEY,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT,
		tool_name TEXT,
		tool_calls TEXT,
		timestamp REAL NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	// Insert test session
	now := float64(time.Now().Unix())
	if _, err := db.Exec("INSERT INTO sessions(id, source, cwd, parent_session_id, started_at) VALUES(?,?,?,?,?)",
		"sess-001", "cli", "/test/project", nil, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO sessions(id, source, cwd, parent_session_id, started_at) VALUES(?,?,?,?,?)",
		"sess-sub-001", "cli", "/test/project", "sess-001", now+1); err != nil {
		t.Fatal(err)
	}

	// Insert test messages
	if _, err := db.Exec("INSERT INTO messages(id, session_id, role, content, tool_name, tool_calls, timestamp) VALUES(?,?,?,?,?,?,?)",
		1, "sess-001", "user", "Hello Hermes", nil, nil, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO messages(id, session_id, role, content, tool_name, tool_calls, timestamp) VALUES(?,?,?,?,?,?,?)",
		2, "sess-001", "assistant", "Hello! How can I help you?", nil, nil, now+0.5); err != nil {
		t.Fatal(err)
	}
}

func TestHermesAdapter_DiscoverAndMessages(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "state.db")
	createTestDB(t, dbPath)

	adapter := NewRoot(tmp)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}

	rootSess := containers[0]
	if rootSess.ID != "sess-001" || rootSess.IsSubagent {
		t.Errorf("unexpected root session: %+v", rootSess)
	}

	subSess := containers[1]
	if subSess.ID != "sess-sub-001" || !subSess.IsSubagent || subSess.ParentID != "sess-001" {
		t.Errorf("unexpected sub session: %+v", subSess)
	}

	msgs, err := adapter.Messages(rootSess)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Text != "Hello Hermes" {
		t.Errorf("unexpected message 0: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Text != "Hello! How can I help you?" {
		t.Errorf("unexpected message 1: %+v", msgs[1])
	}
}

func TestHermesAdapter_Lookup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HERMES_HOME", tmp)
	dbPath := filepath.Join(tmp, "state.db")
	createTestDB(t, dbPath)

	containers, err := lookup("sess-001")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(containers) != 1 || containers[0].ID != "sess-001" {
		t.Fatalf("unexpected lookup result: %+v", containers)
	}
}
