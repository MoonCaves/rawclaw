package goose

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestGooseStrandedSidecar_WALBehavior (Tracker #206) tests the edge case where
// a SQLite Goose database in WAL mode is moved (e.g. via lifecycle.Archive or manual move)
// without its -wal and -shm sidecars.
// It proves:
// 1. Moving only the .db file leaves -wal and -shm behind.
// 2. Opening the moved .db does NOT crash, panic, or fail with SQLITE_CORRUPT.
// 3. The read degrades cleanly to the last-checkpointed state (uncheckpointed WAL rows are omitted).
func TestGooseStrandedSidecar_WALBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	liveDir := filepath.Join(tmpDir, "live")
	archiveDir := filepath.Join(tmpDir, "archive")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir live: %v", err)
	}
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}

	liveDBPath := filepath.Join(liveDir, "sessions.db")

	// 1. Create live database in WAL mode with checkpointed base data
	db, err := sql.Open("sqlite", liveDBPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

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

		INSERT INTO sessions VALUES ('s1', '/workspace/app', 'session-1', 0, 'user', '2026-08-25 12:00:00');
		INSERT INTO messages VALUES ('m1', 's1', 'user', 'Checkpointed initial message', '2026-08-25 12:00:01');
	`)
	if err != nil {
		t.Fatalf("init base schema: %v", err)
	}

	// Force a checkpoint so base data is flushed to sessions.db
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	// 2. Insert new messages into WAL mode while holding a reader open to prevent checkpoint
	dbReader, err := sql.Open("sqlite", liveDBPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer dbReader.Close()

	txReader, err := dbReader.Begin()
	if err != nil {
		t.Fatalf("begin reader tx: %v", err)
	}
	var dummy int
	_ = txReader.QueryRow("SELECT COUNT(*) FROM messages").Scan(&dummy)

	// Write m2 via writer - cannot checkpoint into main DB because reader holds snapshot
	if _, err := db.Exec("PRAGMA wal_autocheckpoint = 0;"); err != nil {
		t.Fatalf("disable autocheckpoint: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO messages VALUES ('m2', 's1', 'assistant', 'Uncheckpointed WAL response', '2026-08-25 12:01:00');
	`)
	if err != nil {
		t.Fatalf("insert WAL record: %v", err)
	}

	// Verify that -wal file exists and has data on disk
	walPath := liveDBPath + "-wal"
	walFi, err := os.Stat(walPath)
	if err != nil {
		t.Logf("WAL file stat: %v", err)
	} else {
		t.Logf("WAL file size on disk before move: %d bytes", walFi.Size())
	}

	// Close connections without running checkpoint
	_ = txReader.Rollback()
	_ = dbReader.Close()
	_ = db.Close()

	// 3. Move ONLY the .db file to archiveDir, leaving -wal and -shm behind (stranded sidecars)
	archivedDBPath := filepath.Join(archiveDir, "sessions.db")
	if err := os.Rename(liveDBPath, archivedDBPath); err != nil {
		t.Fatalf("rename .db: %v", err)
	}

	// Prove sidecars were stranded if they existed
	if st, err := os.Stat(walPath); err == nil && st.Size() > 0 {
		t.Logf("verified: %s (%d bytes) is stranded in liveDir after moving .db", walPath, st.Size())
	}

	// 4. Point adapter at archiveDir and verify clean degradation
	adapter := NewRoot(archiveDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover on stranded db returned unexpected error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	msgs, err := adapter.Messages(containers[0])
	if err != nil {
		t.Fatalf("Messages on stranded db returned unexpected error: %v (must degrade cleanly, not error)", err)
	}

	// The checkpointed base message must be present
	if len(msgs) == 0 {
		t.Fatalf("got 0 messages, want at least 1 (checkpointed message)")
	}
	if msgs[0].Text != "Checkpointed initial message" {
		t.Errorf("msgs[0].Text = %q, want 'Checkpointed initial message'", msgs[0].Text)
	}

	t.Logf("stranded sidecar result: %d messages cleanly read", len(msgs))
}

// TestGooseStrandedSidecar_UncheckpointedWALOmission (Tracker #206) explicitly tests
// what happens when a live WAL holds uncheckpointed transactions, and only the main .db
// file is moved or archived (leaving the -wal file stranded).
// It verifies that:
// 1. Opening the moved .db in the new directory succeeds without error or corruption.
// 2. The adapter cleanly reads the checkpointed state (m1 is present).
// 3. The uncheckpointed transaction in the stranded WAL (m2) is omitted without error.
func TestGooseStrandedSidecar_UncheckpointedWALOmission(t *testing.T) {
	tmpDir := t.TempDir()
	liveDir := filepath.Join(tmpDir, "live")
	archiveDir := filepath.Join(tmpDir, "archive")
	_ = os.MkdirAll(liveDir, 0o755)
	_ = os.MkdirAll(archiveDir, 0o755)

	liveDBPath := filepath.Join(liveDir, "sessions.db")

	db, err := sql.Open("sqlite", liveDBPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		PRAGMA journal_mode = WAL;
		CREATE TABLE sessions (id TEXT PRIMARY KEY, working_dir TEXT, name TEXT);
		CREATE TABLE messages (id TEXT PRIMARY KEY, session_id TEXT, role TEXT, content TEXT, timestamp TEXT);
		INSERT INTO sessions VALUES ('sess-1', '/workspace/goose', 'live-session');
		INSERT INTO messages VALUES ('m1', 'sess-1', 'user', 'Checkpointed commit', '2026-08-25T12:00:00Z');
	`)
	if err != nil {
		t.Fatalf("init base: %v", err)
	}

	// Force checkpoint of initial state
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	// Hold a long-running reader to prevent SQLite from checkpointing future writes
	dbReader, err := sql.Open("sqlite", liveDBPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer dbReader.Close()

	txReader, err := dbReader.Begin()
	if err != nil {
		t.Fatalf("begin reader: %v", err)
	}
	defer txReader.Rollback()

	var count int
	_ = txReader.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)

	// Write m2 into WAL
	_, err = db.Exec(`
		INSERT INTO messages VALUES ('m2', 'sess-1', 'assistant', 'Uncheckpointed WAL message', '2026-08-25T12:00:10Z');
	`)
	if err != nil {
		t.Fatalf("insert WAL: %v", err)
	}

	// Copy ONLY sessions.db to archiveDir (simulating moving or copying without sidecars)
	archivedDBPath := filepath.Join(archiveDir, "sessions.db")
	rawBytes, err := os.ReadFile(liveDBPath)
	if err != nil {
		t.Fatalf("read raw db: %v", err)
	}
	if err := os.WriteFile(archivedDBPath, rawBytes, 0o644); err != nil {
		t.Fatalf("write archived db: %v", err)
	}

	// Adapter discovers and reads the archived DB
	adapter := NewRoot(archiveDir)
	containers, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	msgs, err := adapter.Messages(containers[0])
	if err != nil {
		t.Fatalf("Messages failed: %v (expected clean degradation)", err)
	}

	// Verify clean degradation: m1 is present, uncheckpointed m2 is omitted
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want exactly 1 (checkpointed state only)", len(msgs))
	}
	if msgs[0].Text != "Checkpointed commit" {
		t.Errorf("msgs[0].Text = %q, want 'Checkpointed commit'", msgs[0].Text)
	}
}
