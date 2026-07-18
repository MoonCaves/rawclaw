// Package storetest provides interface-built test fixtures for the store
// schema (D7): a production-schema db via store.Rebuild, plus row inserters.
// Fixture SQL is allowed HERE and nowhere else — storetest is colocated with
// the store, so schema knowledge stays in one directory tree. (Migration tests
// that deliberately hand-write LEGACY schemas keep their own raw DDL — they
// test the migration itself.)
package storetest

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// NewDB creates a fresh production-schema db (base schema + FTS + version
// stamp, via store.ConnectRW + store.Rebuild) in a temp dir and returns the
// open connection plus the db path. The connection is closed on test cleanup.
func NewDB(t testing.TB) (*sql.DB, string) {
	t.Helper()
	dbp := filepath.Join(t.TempDir(), "test.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("storetest: ConnectRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })
	if err := store.Rebuild(con); err != nil {
		t.Fatalf("storetest: Rebuild: %v", err)
	}
	return con, dbp
}

// Session is one sessions-row fixture. ParentID "" inserts SQL NULL (matching
// the indexer's missing-parent behavior).
type Session struct {
	ID           string
	StartedAt    float64
	LastTS       float64
	MessageCount int
	IsSubagent   bool
	ParentID     string
}

// InsertSession inserts one session row (INSERT OR IGNORE, so repeated inserts
// of the same id are a no-op — matching the add-message-then-session flow).
func InsertSession(t testing.TB, con *sql.DB, s Session) {
	t.Helper()
	var parent any
	if s.ParentID != "" {
		parent = s.ParentID
	}
	isSub := 0
	if s.IsSubagent {
		isSub = 1
	}
	if _, err := con.Exec(
		"INSERT OR IGNORE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id) VALUES(?,?,?,?,?,?)",
		s.ID, s.StartedAt, s.LastTS, s.MessageCount, isSub, parent); err != nil {
		t.Fatalf("storetest: insert session %q: %v", s.ID, err)
	}
}

// Message is one messages-row fixture. The FTS sync triggers index Content
// into messages_fts automatically on insert.
type Message struct {
	SessionID string
	Role      string
	Content   string
	TS        float64
	ISO       string
	UUID      string
}

// InsertMessage inserts one message row (uuid included) and returns its rowid.
func InsertMessage(t testing.TB, con *sql.DB, m Message) int {
	t.Helper()
	res, err := con.Exec(
		"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
		m.SessionID, m.Role, m.Content, m.TS, m.ISO, m.UUID)
	if err != nil {
		t.Fatalf("storetest: insert message: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("storetest: last insert id: %v", err)
	}
	return int(id)
}

// settableSessionFields is the allowlist for SetSessionField — the fixture
// mutations the durable-retention and provenance tests need.
var settableSessionFields = map[string]bool{
	"missing_since":  true,
	"origin_machine": true,
	"is_subagent":    true,
	"source_path":    true,
}

// SetSessionField sets one allowlisted sessions column
// (missing_since / origin_machine / is_subagent / source_path) on one session.
// The column name is allowlisted, never interpolated from arbitrary input.
func SetSessionField(t testing.TB, con *sql.DB, sid, field string, value any) {
	t.Helper()
	if !settableSessionFields[field] {
		t.Fatalf("storetest: SetSessionField: field %q not allowlisted", field)
	}
	if _, err := con.Exec("UPDATE sessions SET "+field+"=? WHERE id=?", value, sid); err != nil {
		t.Fatalf("storetest: set sessions.%s: %v", field, err)
	}
}
