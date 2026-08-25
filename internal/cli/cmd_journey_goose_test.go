package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/source/goose"
)

func TestCLIJourney_GooseEndToEnd(t *testing.T) {
	// The journey exercises goose end to end, so it opts in the way a real
	// goose user does.
	t.Setenv("RAWCLAW_GOOSE", "1")
	newCfgRoot(t)

	gooseDir := t.TempDir()
	t.Setenv("GOOSE_HOME", gooseDir)
	t.Setenv("HOME", gooseDir)

	sessionsDir := filepath.Join(gooseDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessionsDir: %v", err)
	}

	sessionsDB := filepath.Join(sessionsDir, "sessions.db")
	dbConn, err := sql.Open("sqlite", sessionsDB)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer dbConn.Close()

	_, err = dbConn.Exec(`
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
		INSERT INTO sessions VALUES
			('goose-session-alpha', '/workspace/goose-app', 'deploy-orchestrator', 1, 'user', '2026-08-21 10:00:00'),
			('goose-session-beta', '/workspace/goose-app', 'db-migration', 0, 'user', '2026-08-21 10:05:00');

		INSERT INTO messages VALUES
			('m1', 'goose-session-alpha', 'user', '[{"type":"text","text":"Deploying Goose pipeline orchestrator"}]', '2026-08-21 10:00:00'),
			('m2', 'goose-session-alpha', 'assistant', '[{"type":"text","text":"Pipeline deployment succeeded with zero warnings."}]', '2026-08-21 10:00:05'),
			('m3', 'goose-session-beta', 'user', '[{"type":"text","text":"Check database migration status"}]', '2026-08-21 10:05:00'),
			('m4', 'goose-session-beta', 'assistant', '[{"type":"text","text":"Migration completed."}]', '2026-08-21 10:05:10');
	`)
	if err != nil {
		t.Fatalf("setup goose db: %v", err)
	}

	// 1. Test Discovery
	ad := goose.New()
	containers, err := ad.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("containers count = %d, want 2", len(containers))
	}

	// 2. Test Scopes integration
	scs := scopes.Goose(false)
	if len(scs) == 0 {
		t.Fatalf("expected goose scopes, got 0")
	}

	// 3. Test Search via CLI
	outSearch, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "orchestrator", "--source", "goose", "--dir", "/workspace/goose-app")
	if err != nil {
		t.Fatalf("search goose: %v\n%s", err, outSearch)
	}
	if !strings.Contains(outSearch, "orchestrator") {
		t.Errorf("search output missing 'orchestrator':\n%s", outSearch)
	}
	if strings.Contains(outSearch, "migration") {
		t.Errorf("search output contains 'migration' from other session:\n%s", outSearch)
	}

	// 4. Test Resume command
	outResume, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--resume", "goose-session-alpha")
	if err != nil {
		t.Fatalf("resume: %v\n%s", err, outResume)
	}
	if !strings.Contains(outResume, "goose session --resume --session-id goose-session-alpha") {
		t.Errorf("resume output = %q, want 'goose session --resume --session-id goose-session-alpha'", outResume)
	}
}
