package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// scopeOfSession reads the (project, cwd) a row carries, reporting NULL as a
// false ok so a test can tell "not known" apart from an empty string.
func scopeOfSession(t *testing.T, con *sql.DB, id string) (project, cwd string, projectOK, cwdOK bool) {
	t.Helper()
	var p, c sql.NullString
	if err := con.QueryRow("SELECT project, cwd FROM sessions WHERE id=?", id).Scan(&p, &c); err != nil {
		t.Fatalf("read scope of %s: %v", id, err)
	}
	return p.String, c.String, p.Valid, c.Valid
}

// TestIndex_StampsRecordedCWD is the base case: a session's scope comes from the
// cwd the transcript itself records, not from the enclosing directory's name.
// The project dir here is deliberately named something else, so a fallback to
// the directory name would be visible.
func TestIndex_StampsRecordedCWD(t *testing.T) {
	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "s.jsonl"),
		`{"type":"user","cwd":"/w/api-gateway","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	project, cwd, projectOK, cwdOK := scopeOfSession(t, con, "s")
	if !projectOK || !cwdOK {
		t.Fatalf("scope not stamped: project ok=%v cwd ok=%v", projectOK, cwdOK)
	}
	if cwd != "/w/api-gateway" {
		t.Errorf("cwd = %q, want %q", cwd, "/w/api-gateway")
	}
	if project != "api-gateway" {
		t.Errorf("project = %q, want %q", project, "api-gateway")
	}
}

// TestIndex_CWDFromNonIndexableLine guards the reason cwd is read before the
// indexable filter: Claude records cwd on attachment/meta lines too. A session
// whose only cwd-bearing line is not itself an indexable message must still
// land with a scope.
func TestIndex_CWDFromNonIndexableLine(t *testing.T) {
	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "s.jsonl"),
		`{"type":"attachment","cwd":"/w/billing"}`,
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	project, cwd, _, cwdOK := scopeOfSession(t, con, "s")
	if !cwdOK {
		t.Fatal("cwd is NULL; a non-indexable line's cwd was skipped")
	}
	if cwd != "/w/billing" || project != "billing" {
		t.Errorf("scope = (%q, %q), want (%q, %q)", project, cwd, "billing", "/w/billing")
	}
}

// TestIndex_NoRecordedCWDFallsBackToDirName covers a transcript that records no
// cwd at all: project falls back to the enclosing directory's name (what
// paths.ProjectLabel shows), and cwd stays NULL rather than claiming a
// directory the session never reported.
func TestIndex_NoRecordedCWDFallsBackToDirName(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "orphan-project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(proj, "s.jsonl"),
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	project, _, projectOK, cwdOK := scopeOfSession(t, con, "s")
	if cwdOK {
		t.Error("cwd is non-NULL; an unrecorded cwd must stay unknown, not be invented")
	}
	if !projectOK || project != "orphan-project" {
		t.Errorf("project = %q (ok=%v), want %q", project, projectOK, "orphan-project")
	}
}

// TestIndex_NestedThreadTakesProjectScope covers the majority of rows on a real
// index: subagent and workflow transcripts live in SUBDIRECTORIES of the project
// dir and record no cwd of their own. Their scope is the project's — reading the
// file's own parent directory instead would label them "subagents" or "wf_<id>",
// which are not projects.
func TestIndex_NestedThreadTakesProjectScope(t *testing.T) {
	proj := t.TempDir()
	writeJSONL(t, filepath.Join(proj, "top.jsonl"),
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"parent"}}`)
	for _, sub := range []string{"subagents", "wf_2f7c632c-fa7"} {
		if err := os.MkdirAll(filepath.Join(proj, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		writeJSONL(t, filepath.Join(proj, sub, "child.jsonl"),
			`{"type":"user","timestamp":"2026-06-01T10:05:00Z","message":{"role":"user","content":"nested thread"}}`)
	}

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	// Collect every row's scope in one read, then assert — the ids are derived
	// (a nested thread is "<parent>/<stem>"), so listing them is the check that
	// all three sessions landed AND that each carries the project's full scope,
	// cwd included. Asserting project alone would pass even with cwd inheritance
	// broken, because the label is recoverable from the directory by itself.
	rows, err := con.Query("SELECT id, COALESCE(project,'<NULL>'), COALESCE(cwd,'<NULL>') FROM sessions ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, project, cwd string
		if err := rows.Scan(&id, &project, &cwd); err != nil {
			t.Fatal(err)
		}
		n++
		if project != "ledger" || cwd != "/w/ledger" {
			t.Errorf("session %s scope = (%q, %q), want (%q, %q)", id, project, cwd, "ledger", "/w/ledger")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("indexed %d sessions, want 3 (one top-level + two nested)", n)
	}
}

// TestBackfillScope_NestedThreadTakesProjectScope is the same guarantee on the
// upgrade path, which cannot see a project dir handed to it and has to walk up
// to one from each row's stored source_path.
func TestBackfillScope_NestedThreadTakesProjectScope(t *testing.T) {
	cfg := t.TempDir()
	root := filepath.Join(cfg, "projects")
	proj := filepath.Join(root, "-w-ledger")
	if err := os.MkdirAll(filepath.Join(proj, "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	writeJSONL(t, filepath.Join(proj, "top.jsonl"),
		`{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"parent"}}`)

	con, _ := openTestDB(t)
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := con.Exec(
		`INSERT INTO sessions(id,message_count,source_path,origin_machine) VALUES('top/child',1,?,'m')`,
		filepath.Join(proj, "subagents", "child.jsonl"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("DELETE FROM meta WHERE key=?", scopeBackfillKey); err != nil {
		t.Fatal(err)
	}
	if err := migrateScopeColumns(con); err != nil {
		t.Fatalf("migrateScopeColumns: %v", err)
	}

	project, cwd, _, _ := scopeOfSession(t, con, "top/child")
	if project != "ledger" || cwd != "/w/ledger" {
		t.Errorf("nested thread backfilled to (%q, %q), want (%q, %q)", project, cwd, "ledger", "/w/ledger")
	}
}

// TestMigrateScopeColumns_UpgradesInPlace is the durability guarantee: a db that
// predates the scope columns gains them and backfills WITHOUT a rebuild, so a
// session whose source file was purged upstream — the only copy of which is this
// row — survives the upgrade. A schema-version bump would have re-walked the
// live tree and pruned it.
func TestMigrateScopeColumns_UpgradesInPlace(t *testing.T) {
	proj := t.TempDir()
	live := filepath.Join(proj, "live.jsonl")
	purged := filepath.Join(proj, "purged.jsonl")
	writeJSONL(t, live, `{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"still here"}}`)
	writeJSONL(t, purged, `{"type":"user","cwd":"/w/ledger","timestamp":"2026-06-01T11:00:00Z","message":{"role":"user","content":"gone upstream"}}`)

	con, _ := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(purged); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2 (after purge): %v", err)
	}

	// Rewind to the pre-migration shape: drop the columns and the completion
	// marker, so EnsureSchema faces exactly what a v0.8.0 db looks like.
	for _, c := range scopeColumns {
		if _, err := con.Exec("ALTER TABLE sessions DROP COLUMN " + c.name); err != nil {
			t.Fatalf("rewind: drop sessions.%s: %v", c.name, err)
		}
	}
	if _, err := con.Exec("DELETE FROM meta WHERE key=?", scopeBackfillKey); err != nil {
		t.Fatalf("rewind: clear marker: %v", err)
	}

	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("EnsureSchema (migration): %v", err)
	}

	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("session count = %d after migration, want 2 — the upgrade dropped rows", n)
	}
	var stillOnlyCopy int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id='purged' AND only_copy_since IS NOT NULL").Scan(&stillOnlyCopy); err != nil {
		t.Fatal(err)
	}
	if stillOnlyCopy != 1 {
		t.Error("the retained (purged) session lost its only_copy_since watermark in the upgrade")
	}

	// The live session's file is still readable, so the backfill recovers its
	// recorded cwd exactly.
	project, cwd, _, cwdOK := scopeOfSession(t, con, "live")
	if !cwdOK || cwd != "/w/ledger" || project != "ledger" {
		t.Errorf("live scope = (%q, %q, cwdOK=%v), want (%q, %q, true)", project, cwd, cwdOK, "ledger", "/w/ledger")
	}
	// The purged session shares the directory, so the memo answers for it too —
	// this is what makes the backfill one read per project, not one per session.
	project, cwd, _, cwdOK = scopeOfSession(t, con, "purged")
	if !cwdOK || cwd != "/w/ledger" || project != "ledger" {
		t.Errorf("purged scope = (%q, %q, cwdOK=%v), want (%q, %q, true)", project, cwd, cwdOK, "ledger", "/w/ledger")
	}
}

// TestBackfillScope_DateShardedSourceGetsNoLabel is the defect this gate exists
// for. A source that shards its transcripts by date has no project directory at
// all, so walking up from the file finds nothing and the parent's name is a day
// number. Labeling the row with it invents a project that never existed: it then
// answers a path filter and names itself in the scope footer. The row must come
// out unlabeled instead, because unlabeled is the truth.
func TestBackfillScope_DateShardedSourceGetsNoLabel(t *testing.T) {
	shard := filepath.Join(t.TempDir(), "2026", "07", "09")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(shard, "rollout-abc.jsonl")
	writeJSONL(t, rollout, `{"type":"user","timestamp":"2026-07-09T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, _ := openTestDB(t)
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	// Two rows on the same shard: one from another source, one from the
	// directory-walk ingest. Only the first is refused — an explicit --dir tree
	// is laid out project-per-directory, and the gate must not disturb it.
	if _, err := con.Exec(
		`INSERT INTO sessions(id,message_count,source_path,origin_machine,source_tool) VALUES('foreign',1,?,'m','codex')`,
		rollout); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec(
		`INSERT INTO sessions(id,message_count,source_path,origin_machine,source_tool) VALUES('walked',1,?,'m',?)`,
		rollout, sourceClaude); err != nil {
		t.Fatal(err)
	}
	if _, err := con.Exec("DELETE FROM meta WHERE key=?", scopeBackfillKey); err != nil {
		t.Fatal(err)
	}
	if err := migrateScopeColumns(con); err != nil {
		t.Fatalf("migrateScopeColumns: %v", err)
	}

	project, _, projectOK, cwdOK := scopeOfSession(t, con, "foreign")
	if projectOK || cwdOK {
		t.Errorf("date-sharded row scoped to (%q, cwdOK=%v); a day number is not a project", project, cwdOK)
	}
	if project, _, ok, _ := scopeOfSession(t, con, "walked"); !ok || project != "09" {
		t.Errorf("directory-walk row = %q (ok=%v); the gate must leave that path alone", project, ok)
	}
}

// TestMigrateScopeColumns_RepairsInventedLabels covers the other half: a db an
// earlier binary already mislabeled. That db carries the first pass's completion
// marker, so the repair has to be its own stamped pass or it never runs.
func TestMigrateScopeColumns_RepairsInventedLabels(t *testing.T) {
	con, _ := openTestDB(t)
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	// invented: a foreign-source row labeled with a day number and nothing behind
	// it. grounded: a foreign-source row whose cwd was actually recorded, so its
	// label is earned. walked: the directory-walk ingest's own label, untouchable.
	for _, r := range []struct{ id, source, project, cwd string }{
		{"invented", "codex", "09", ""},
		{"grounded", "codex", "ledger", "/w/ledger"},
		{"walked", sourceClaude, "orphan-project", ""},
	} {
		var cwd any
		if r.cwd != "" {
			cwd = r.cwd
		}
		if _, err := con.Exec(
			`INSERT INTO sessions(id,message_count,origin_machine,source_tool,project,cwd) VALUES(?,1,'m',?,?,?)`,
			r.id, r.source, r.project, cwd); err != nil {
			t.Fatal(err)
		}
	}
	// The db predates the repair but not the backfill — exactly the upgrade shape.
	if _, err := con.Exec("DELETE FROM meta WHERE key=?", inventedScopeRepairKey); err != nil {
		t.Fatal(err)
	}
	if err := migrateScopeColumns(con); err != nil {
		t.Fatalf("migrateScopeColumns: %v", err)
	}

	if project, _, ok, _ := scopeOfSession(t, con, "invented"); ok {
		t.Errorf("invented label %q survived the repair", project)
	}
	if project, cwd, _, _ := scopeOfSession(t, con, "grounded"); project != "ledger" || cwd != "/w/ledger" {
		t.Errorf("grounded row = (%q, %q); a label backed by a recorded cwd is not invented", project, cwd)
	}
	if project, _, ok, _ := scopeOfSession(t, con, "walked"); !ok || project != "orphan-project" {
		t.Errorf("directory-walk row = %q (ok=%v); the repair reached past its own source", project, ok)
	}

	var marker string
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", inventedScopeRepairKey).Scan(&marker); err != nil || marker != "1" {
		t.Errorf("repair marker = %q (%v), want %q", marker, err, "1")
	}
}

// TestMigrateScopeColumns_MarkerStopsRework proves the completion marker does
// its job: once the backfill has run, a row that resolved to nothing is not
// re-read on every later invocation. Without the marker, a db holding sessions
// whose files are gone would pay that scan forever.
func TestMigrateScopeColumns_MarkerStopsRework(t *testing.T) {
	con, _ := openTestDB(t)
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := con.Exec(
		`INSERT INTO sessions(id,message_count,source_path,origin_machine) VALUES('s',1,'/nonexistent/dir/s.jsonl','m')`,
	); err != nil {
		t.Fatal(err)
	}
	// A fresh db was already marked backfilled at creation, so clear the marker
	// to force one real pass over the row inserted above.
	if _, err := con.Exec("DELETE FROM meta WHERE key=?", scopeBackfillKey); err != nil {
		t.Fatal(err)
	}
	if err := migrateScopeColumns(con); err != nil {
		t.Fatalf("migrateScopeColumns: %v", err)
	}

	// The file does not exist, so cwd stays unknown while project still comes
	// from the recorded directory name.
	project, _, projectOK, cwdOK := scopeOfSession(t, con, "s")
	if cwdOK {
		t.Error("cwd is non-NULL for a session whose file is gone")
	}
	if !projectOK || project != "dir" {
		t.Errorf("project = %q (ok=%v), want %q from the source_path's directory", project, projectOK, "dir")
	}

	var marker string
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", scopeBackfillKey).Scan(&marker); err != nil {
		t.Fatalf("completion marker not written: %v", err)
	}
	if marker != "1" {
		t.Errorf("marker = %q, want %q", marker, "1")
	}
}
