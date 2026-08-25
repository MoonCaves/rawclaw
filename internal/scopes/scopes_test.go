package scopes

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// writeCodexRollout writes a minimal one-message Codex rollout file (a
// session_meta header + one user message) at path, for tests that seed a
// Codex index db directly via index.EnsureIndexedContainers rather than
// driving the real adapter's Discover().
func writeCodexRollout(t *testing.T, path, sessionID, cwd string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		fmt.Sprintf(`{"type":"session_meta","timestamp":"2026-07-15T10:00:00Z","payload":{"id":%q,"cwd":%q,"thread_source":"user"}}`, sessionID, cwd),
		`{"type":"response_item","timestamp":"2026-07-15T10:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"beacon token"}]}}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCodexDBPath_Injective guards the cross-prune contract (see
// index.EnsureIndexedContainers): each Codex cwd group is ingested into its OWN
// db and passed as the COMPLETE set for that db. If two DISTINCT cwds map to the
// same db path they share one db, and each call then prunes the other's sessions
// (or wipes them under reindex) — the exact contract violation the commit warns
// about, WITHIN Codex.
//
// encodeCWD's readable slug is lossy ('/', '.', literal '-' all fold to '-'), so
// codexDBPath appends a hash of the full cwd to stay injective. These pairs would
// collide on the slug alone; they must map to distinct db paths.
func TestCodexDBPath_Injective(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{"/Users/a/b", "/Users/a-b"}, // '/' vs literal '-'
		{"/Users/a.b", "/Users/a-b"}, // '.' vs literal '-'
		{"/x/y", "/x.y"},             // '/' vs '.'
	}
	for _, p := range pairs {
		if got0, got1 := codexDBPath(p[0]), codexDBPath(p[1]); got0 == got1 {
			t.Errorf("codexDBPath collision: %q and %q both -> %q; distinct cwds share a db and cross-prune", p[0], p[1], got0)
		}
	}
}

// TestCodexDBPath_PrefixedAwayFromClaude guards the property the "codex-" prefix
// exists to guarantee: a Codex cwd group can never land on the Claude project db
// for the same cwd.
func TestCodexDBPath_PrefixedAwayFromClaude(t *testing.T) {
	t.Parallel()
	base := filepath.Base(codexDBPath("/Users/octocat/proj"))
	if !strings.HasPrefix(base, "codex-") {
		t.Errorf("codexDBPath base %q lacks the codex- prefix that separates it from the Claude db", base)
	}
}

// TestCodexDBPath_Stable pins that a given cwd (including empty) maps to the SAME
// db across runs — the hash is deterministic, so a cwd group keeps its index.
func TestCodexDBPath_Stable(t *testing.T) {
	t.Parallel()
	for _, cwd := range []string{"", "/Users/octocat/proj"} {
		if a, b := codexDBPath(cwd), codexDBPath(cwd); a != b {
			t.Errorf("codexDBPath(%q) not stable: %q vs %q", cwd, a, b)
		}
	}
	if got := encodeCWD(""); got != "unknown" {
		t.Errorf("encodeCWD(%q) = %q, want %q", "", got, "unknown")
	}
}

// TestClaudeUnionsOrphanedDBs is the D8 scope-discovery guard: an index db whose
// source project dir was purged (emptied, so AllProjectDirs drops it) must still
// be discovered as an eager read-only scope — while a tombstoned-only db (a real
// delete) must NOT resurface, and a live project must not be double-listed. It
// exercises Claude() against a REAL projects root (the seam the injected-scope
// unit tests skipped, which is how the end-to-end gap hid).
func TestClaudeUnionsOrphanedDBs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))

	root := filepath.Join(home, ".claude", "projects")
	liveDir := filepath.Join(root, "-p-live")
	orphanDir := filepath.Join(root, "-p-orphan")
	deletedDir := filepath.Join(root, "-p-deleted")
	write := func(dir, stem string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		line := `{"type":"user","uuid":"aaaa1111-bbbb-cccc-dddd-eeeeeeeeeeee","timestamp":"2026-06-01T10:00:00Z","cwd":"/tmp/x","message":{"role":"user","content":"beacon token"}}`
		if err := os.WriteFile(filepath.Join(dir, stem+".jsonl"), []byte(line+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(liveDir, "livesess")
	write(orphanDir, "orphansess")
	write(deletedDir, "deadsess")
	for _, d := range []string{liveDir, orphanDir, deletedDir} {
		if _, _, _, err := index.EnsureIndexed(d, false); err != nil {
			t.Fatalf("index %s: %v", d, err)
		}
	}

	// Purge orphanDir + deletedDir (remove their jsonl → empty dirs that
	// AllProjectDirs no longer yields).
	if err := os.Remove(filepath.Join(orphanDir, "orphansess.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(deletedDir, "deadsess.jsonl")); err != nil {
		t.Fatal(err)
	}
	// deletedDir's session is ALSO tombstoned (a real `rawclaw delete`).
	tombDir := filepath.Join(home, ".cache", "session-search")
	if err := os.MkdirAll(tombDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tombDir, ".deleted"), []byte("deadsess\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scs := Claude()
	has := func(pred func(view.Scope) bool) bool {
		for _, s := range scs {
			if pred(s) {
				return true
			}
		}
		return false
	}

	// Live project: still a lazy scope (TDir set).
	if !has(func(s view.Scope) bool { return s.TDir == liveDir }) {
		t.Errorf("live project scope missing")
	}
	// Orphan-retained: eager read-only scope on its db (DBP set, TDir empty).
	orphanDBP := index.DBPath(orphanDir)
	if !has(func(s view.Scope) bool { return s.DBP == orphanDBP && s.TDir == "" && s.Source == "claude" }) {
		t.Errorf("orphaned-source db not discovered as an eager scope")
	}
	// Tombstoned-only: must NOT resurface — a real delete stays deleted.
	if has(func(s view.Scope) bool { return s.DBP == index.DBPath(deletedDir) }) {
		t.Errorf("tombstoned-only db resurfaced as a scope — a real delete must read as deleted")
	}
	// Dedup: the live db is never ALSO surfaced as an orphan (DBP) scope.
	if has(func(s view.Scope) bool { return s.DBP == index.DBPath(liveDir) }) {
		t.Errorf("live project db double-listed as an orphan scope")
	}

	// The retained orphan was stamped only_copy_since (own-source, source gone).
	con, err := store.ConnectRO(orphanDBP)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	var ms sql.NullFloat64
	if err := con.QueryRow("SELECT only_copy_since FROM sessions WHERE id='orphansess'").Scan(&ms); err != nil {
		t.Fatalf("read orphansess.only_copy_since: %v", err)
	}
	if !ms.Valid || ms.Float64 <= 0 {
		t.Errorf("orphan session not stamped only_copy_since, got %+v", ms)
	}
}

// TestCodexUnionsOrphanedDBs_NoLiveContainers is the D8 guard for Codex,
// mirroring TestClaudeUnionsOrphanedDBs: a Codex cwd whose rollouts are ALL
// purged (Discover() returns zero containers — CODEX_HOME has no sessions/
// dir at all) must still surface its retained db as an eager read-only scope.
// Before this fix, Codex() returned nil before ever scanning for orphans.
func TestCodexUnionsOrphanedDBs_NoLiveContainers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexHome := t.TempDir() // no sessions/ subdir -> Discover() sees zero containers
	t.Setenv("CODEX_HOME", codexHome)

	cwd := "/tmp/purged-codex-proj"
	dbp := codexDBPath(cwd)
	src := filepath.Join(home, "orphan-source", "rollout-orphansess.jsonl")
	writeCodexRollout(t, src, "orphansess", cwd)
	cs := []source.Container{{ID: "orphansess", Path: src, CWD: cwd}}
	if _, _, err := index.EnsureIndexedContainers(dbp, true, cs, codex.New().Messages, "codex", ""); err != nil {
		t.Fatalf("seed orphan db: %v", err)
	}

	scs := Codex(false)
	var found *view.Scope
	for i := range scs {
		if scs[i].DBP == dbp {
			found = &scs[i]
		}
	}
	if found == nil {
		t.Fatalf("purged-cwd codex db not discovered as an eager scope; got %+v", scs)
	}
	if found.Source != "codex" || found.TDir != "" {
		t.Errorf("orphan codex scope malformed: %+v", *found)
	}
}

// TestCodexUnionsOrphanedDBs_CoveredNotDuplicated guards the dedup half of D8
// for Codex: a cwd that IS still live (Discover() finds it) must be indexed
// exactly once — never ALSO surfaced a second time by the orphan scan.
func TestCodexUnionsOrphanedDBs_CoveredNotDuplicated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	cwd := "/tmp/live-codex-proj"
	day := filepath.Join(codexHome, "sessions", "2026", "07", "15")
	writeCodexRollout(t, filepath.Join(day, "rollout-livesess.jsonl"), "livesess", cwd)

	dbp := codexDBPath(cwd)
	scs := Codex(false)
	count := 0
	for _, s := range scs {
		if s.DBP == dbp {
			count++
		}
	}
	if count != 1 {
		t.Errorf("live codex cwd db listed %d times, want 1: %+v", count, scs)
	}
}

// TestCodexUnionsOrphanedDBs_TombstonedExcluded guards the delete-stays-deleted
// half of D8 for Codex: an orphaned db whose only session was explicitly
// tombstoned (a real `rawclaw delete`) must NOT resurface as a scope.
func TestCodexUnionsOrphanedDBs_TombstonedExcluded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexHome := t.TempDir() // no sessions/ subdir -> Discover() sees zero containers
	t.Setenv("CODEX_HOME", codexHome)

	cwd := "/tmp/deleted-codex-proj"
	dbp := codexDBPath(cwd)
	src := filepath.Join(home, "orphan-source", "rollout-deadsess.jsonl")
	writeCodexRollout(t, src, "deadsess", cwd)
	cs := []source.Container{{ID: "deadsess", Path: src, CWD: cwd}}
	if _, _, err := index.EnsureIndexedContainers(dbp, true, cs, codex.New().Messages, "codex", ""); err != nil {
		t.Fatalf("seed orphan db: %v", err)
	}

	if err := os.WriteFile(filepath.Join(store.CacheDir(), ".deleted"), []byte("deadsess\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scs := Codex(false)
	for _, s := range scs {
		if s.DBP == dbp {
			t.Errorf("tombstoned-only codex db resurfaced as a scope: %+v", s)
		}
	}
}

// TestCodexOrphanLabel checks the friendly-label derivation strips both the
// "codex-" prefix and the trailing injective cwd-hash segment, so the label
// reads as a basename rather than an opaque hex tag.
func TestCodexOrphanLabel(t *testing.T) {
	cwd := "/Users/octocat/proj"
	base := filepath.Base(codexDBPath(cwd))
	if got, want := codexOrphanLabel(base), codexLabel(cwd); got != want {
		t.Errorf("codexOrphanLabel(%q) = %q, want %q (matching codexLabel(%q))", base, got, want, cwd)
	}
}

// TestCodex_WriteLockContention proves that when a concurrent connection holds
// a write lock on a Codex cwd index db during discovery/indexing, the returned
// Scope is marked Stale=true, and Resolve on that scope returns IndexStale.
func TestCodex_WriteLockContention(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	cwd := "/tmp/lock-codex-proj"
	day := filepath.Join(codexHome, "sessions", "2026", "08", "25")
	rolloutPath := filepath.Join(day, "rollout-sess1.jsonl")
	writeCodexRollout(t, rolloutPath, "sess1", cwd)

	dbp := codexDBPath(cwd)

	// 1. Initial indexing
	scs := Codex(true)
	if len(scs) != 1 {
		t.Fatalf("initial Codex() returned %d scopes, want 1", len(scs))
	}
	if scs[0].Stale {
		t.Fatalf("initial scope.Stale = true, want false")
	}

	// 2. Open second connection and hold exclusive write lock
	con2, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}
	defer con2.Close()

	tx2, err := con2.Begin()
	if err != nil {
		t.Fatalf("begin con2 tx: %v", err)
	}
	defer tx2.Rollback()

	if _, err := tx2.Exec("INSERT INTO meta(key, value) VALUES('lock_probe', 'held')"); err != nil {
		t.Fatalf("hold write lock: %v", err)
	}

	// 3. Update rollout file so reindexing is triggered
	time.Sleep(10 * time.Millisecond)
	lines := []string{
		fmt.Sprintf(`{"type":"session_meta","timestamp":"2026-08-25T10:00:00Z","payload":{"id":%q,"cwd":%q,"thread_source":"user"}}`, "sess1", cwd),
		`{"type":"response_item","timestamp":"2026-08-25T10:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"updated token"}]}}`,
	}
	if err := os.WriteFile(rolloutPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Run Codex(): write lock held by con2 causes reindexing to fail with SQLITE_BUSY.
	// Codex() must mark the scope Stale.
	scs2 := Codex(false)
	if len(scs2) != 1 {
		t.Fatalf("Codex() returned %d scopes, want 1", len(scs2))
	}
	if !scs2[0].Stale {
		t.Errorf("scope.Stale = false, want true when index encountered write lock contention")
	}

	// 5. Resolve on the scope must return IndexStale
	resolvedDBP, status, err := Resolve(scs2[0], false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolvedDBP != dbp {
		t.Errorf("resolvedDBP = %q, want %q", resolvedDBP, dbp)
	}
	if status == index.IndexFresh {
		t.Errorf("Resolve status = %v, want not fresh (IndexStale)", status)
	}
	if status != index.IndexStale {
		t.Errorf("Resolve status = %v, want IndexStale", status)
	}
}

type testLogRecorder struct {
	records []slog.Record
	mu      sync.Mutex
}

func (r *testLogRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *testLogRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec)
	return nil
}
func (r *testLogRecorder) WithAttrs(_ []slog.Attr) slog.Handler { return r }
func (r *testLogRecorder) WithGroup(_ string) slog.Handler      { return r }

func (r *testLogRecorder) Warns() []slog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []slog.Record
	for _, rec := range r.records {
		if rec.Level >= slog.LevelWarn {
			out = append(out, rec)
		}
	}
	return out
}

// TestCodex_IndexingFailure_SingleHandlingLog proves that a failure during container
// reindexing propagates up to the scope boundary and produces exactly ONE log record
// (at the scopes boundary) rather than duplicate logs below it, while marking the
// scope Stale.
func TestCodex_IndexingFailure_SingleHandlingLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	cwd := "/tmp/single-handling-proj"
	day := filepath.Join(codexHome, "sessions", "2026", "08", "25")
	rolloutPath := filepath.Join(day, "rollout-sess1.jsonl")
	writeCodexRollout(t, rolloutPath, "sess-single-handling", cwd)

	dbp := codexDBPath(cwd)

	// 1. Initial index
	scs := Codex(true)
	if len(scs) != 1 || scs[0].Stale {
		t.Fatalf("initial indexing failed or stale: %+v", scs)
	}

	// 2. Install a trigger on messages to force a non-busy failure during reindex
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := con.Exec("CREATE TRIGGER fail_messages BEFORE INSERT ON messages BEGIN SELECT RAISE(FAIL, 'forced write failure'); END;"); err != nil {
		con.Close()
		t.Fatalf("create trigger: %v", err)
	}
	con.Close()

	// 3. Update rollout file so reindexing runs
	time.Sleep(10 * time.Millisecond)
	lines := []string{
		fmt.Sprintf(`{"type":"session_meta","timestamp":"2026-08-25T10:00:00Z","payload":{"id":%q,"cwd":%q,"thread_source":"user"}}`, "sess-single-handling", cwd),
		`{"type":"response_item","timestamp":"2026-08-25T10:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"updated text"}]}}`,
	}
	if err := os.WriteFile(rolloutPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Capture slog records
	recorder := &testLogRecorder{}
	origLogger := slog.Default()
	slog.SetDefault(slog.New(recorder))
	defer slog.SetDefault(origLogger)

	// 5. Run Codex(): must produce exactly ONE slog.Warn record at the scopes boundary
	scs2 := Codex(false)
	if len(scs2) != 1 {
		t.Fatalf("Codex() returned %d scopes, want 1", len(scs2))
	}
	if !scs2[0].Stale {
		t.Errorf("scope.Stale = false, want true when indexing failed")
	}

	warns := recorder.Warns()
	if len(warns) != 1 {
		t.Fatalf("got %d slog.Warn records, want exactly 1 (single-handling rule)", len(warns))
	}
	if warns[0].Message != "scopes: codex index failed" {
		t.Errorf("log message = %q, want %q", warns[0].Message, "scopes: codex index failed")
	}

	// Check that the error logged at the boundary contains the session ID
	var loggedErr error
	warns[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "err" {
			if e, ok := a.Value.Any().(error); ok {
				loggedErr = e
			}
		}
		return true
	})
	if loggedErr == nil {
		t.Fatal("log record missing 'err' attribute")
	}
	if !strings.Contains(loggedErr.Error(), "sess-single-handling") {
		t.Errorf("logged error %q does not contain session ID %q", loggedErr.Error(), "sess-single-handling")
	}
}
