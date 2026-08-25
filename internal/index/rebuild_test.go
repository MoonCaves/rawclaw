package index

import (
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestRestoreSession_RollbackOnFailure(t *testing.T) {
	con, dbp := openTestDB(t)

	// Seed an existing session and messages
	if _, err := con.Exec(
		"INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent,project,cwd) VALUES(?,?,?,?,?,?,?)",
		"sess-atomic-vault", 1.0, 2.0, 2, 0, "testproj", "/path",
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	if _, err := con.Exec(
		"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
		"sess-atomic-vault", "user", "original message 1", 1.0, "2026-08-15T00:00:00Z", "u1",
	); err != nil {
		t.Fatalf("seed message 1: %v", err)
	}
	if _, err := con.Exec(
		"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
		"sess-atomic-vault", "assistant", "original message 2", 2.0, "2026-08-15T00:00:01Z", "u2",
	); err != nil {
		t.Fatalf("seed message 2: %v", err)
	}

	// Install a trigger that aborts insertion on role='fail' to simulate mid-write failure
	if _, err := con.Exec("CREATE TRIGGER test_abort_restore BEFORE INSERT ON messages WHEN new.role = 'fail' BEGIN SELECT RAISE(ABORT, 'injected write failure'); END;"); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	v := durable.Session{
		Meta: durable.Meta{
			ID:         "sess-atomic-vault",
			Source:     "claude",
			SourcePath: filepath.Join(t.TempDir(), "s.jsonl"),
			CWD:        "/path",
		},
	}

	rows := []reindexRow{
		{role: "user", content: "replacement message 1", ts: 3.0, tsISO: "2026-08-15T00:00:02Z", uuid: "u-new-1"},
		{role: "fail", content: "replacement message 2", ts: 4.0, tsISO: "2026-08-15T00:00:03Z", uuid: "u-new-2"},
	}

	err := restoreSession(con, v, rows, 3.0, 4.0, "/path")
	if err == nil {
		t.Fatal("restoreSession should have returned error on injected failure")
	}

	// Verify original session and its messages are 100% intact after failure
	var sessionCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='sess-atomic-vault'").Scan(&sessionCount); err != nil {
		t.Fatalf("query session count: %v", err)
	}
	if sessionCount != 1 {
		t.Errorf("after rollback session count = %d, want 1 (original session preserved)", sessionCount)
	}

	var msgCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='sess-atomic-vault'").Scan(&msgCount); err != nil {
		t.Fatalf("query message count: %v", err)
	}
	if msgCount != 2 {
		t.Errorf("after rollback message count = %d, want 2 (original messages preserved)", msgCount)
	}

	var firstMsg string
	if err := con.QueryRow("SELECT content FROM messages WHERE session_id='sess-atomic-vault' AND uuid='u1'").Scan(&firstMsg); err != nil {
		t.Fatalf("query original message: %v", err)
	}
	if firstMsg != "original message 1" {
		t.Errorf("content = %q, want %q", firstMsg, "original message 1")
	}

	// Also verify read-only connection sees intact state
	rcon, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("connect RO: %v", err)
	}
	defer rcon.Close()

	if err := rcon.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='sess-atomic-vault'").Scan(&msgCount); err != nil {
		t.Fatalf("ro query message count: %v", err)
	}
	if msgCount != 2 {
		t.Errorf("ro after rollback message count = %d, want 2", msgCount)
	}
}
