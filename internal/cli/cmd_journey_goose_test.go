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
		INSERT INTO sessions VALUES
			('goose-session-alpha', '/workspace/goose-app', '', 0),
			('goose-session-beta', '/workspace/goose-app', '', 0);

		INSERT INTO messages VALUES
			('m1', 'goose-session-alpha', 'user', 'Deploying Goose pipeline orchestrator', '2026-08-21T10:00:00Z'),
			('m2', 'goose-session-alpha', 'assistant', 'Pipeline deployment succeeded with zero warnings.', '2026-08-21T10:00:05Z'),
			('m3', 'goose-session-beta', 'user', 'Check database migration status', '2026-08-21T10:05:00Z'),
			('m4', 'goose-session-beta', 'assistant', 'Migration completed.', '2026-08-21T10:05:10Z');
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
	if !strings.Contains(outResume, "goose session --resume goose-session-alpha") {
		t.Errorf("resume output = %q, want 'goose session --resume goose-session-alpha'", outResume)
	}
}
