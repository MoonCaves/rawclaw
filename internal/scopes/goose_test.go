package scopes

import (
	"context"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source/goose"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestGooseOptedIn pins the opt-in gate: goose discovery is OFF by default,
// on when the user sets RAWCLAW_GOOSE, and on when they explicitly ask with
// --source goose. (The expensive walk must never run un-asked; see GooseOptedIn.)
func TestGooseOptedIn(t *testing.T) {
	t.Setenv("RAWCLAW_GOOSE", "")
	if GooseOptedIn("") {
		t.Error("goose discovery ran with no opt-in — the default must be off")
	}
	if !GooseOptedIn("goose") {
		t.Error("--source goose must count as opt-in")
	}
	t.Setenv("RAWCLAW_GOOSE", "1")
	if !GooseOptedIn("") {
		t.Error("RAWCLAW_GOOSE=1 must enable goose discovery")
	}
	t.Setenv("RAWCLAW_GOOSE", "off")
	if GooseOptedIn("") {
		t.Error("RAWCLAW_GOOSE=off must disable goose discovery")
	}
}

// TestGooseOptedOut_StillServesAlreadyIndexedHistory pins the release-blocking
// fix: opting out must skip the eager filesystem walk but NEVER hide history
// goose already indexed on a prior opted-in run — an archive must never hide
// what it already holds (GooseOptedIn's own doc comment promises this; the
// original All() gate broke that promise by skipping containerScopes' orphan
// half along with its eager half).
func TestGooseOptedOut_StillServesAlreadyIndexedHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RAWCLAW_GOOSE", "")

	dbp := containerDBPath(goose.ID, "/w/an-old-goose-project")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open goose fixture db: %v", err)
	}
	if err := index.EnsureSchema(con, "goose"); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := con.Exec(
		`INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent,project,cwd)
		 VALUES('goose-sess-1',100,100,1,0,'an-old-goose-project','/w/an-old-goose-project')`,
	); err != nil {
		t.Fatalf("seed goose session: %v", err)
	}
	if _, err := con.Exec(
		`INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES('goose-sess-1','user','hi',100,'',  'u-1')`,
	); err != nil {
		t.Fatalf("seed goose message: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}

	if GooseOptedIn("") {
		t.Fatal("test setup: expected goose to be opted OUT")
	}

	scopes := All(context.Background(), "", false)
	var found bool
	for _, sc := range scopes {
		if sc.Source == goose.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("already-indexed goose history is missing from All() while opted out — the archive promise is broken")
	}
}
