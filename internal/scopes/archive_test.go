package scopes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// gitS runs git in dir with a pinned identity, failing the test on error.
func gitS(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.invalid",
		"-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.Command("git", full...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// setupArchiveHome stands up an isolated HOME with a configured archive whose
// clone holds one FOREIGN machine dir ("machine-b", one Claude project with a
// distinctive beacon), plus one LOCAL Claude project. Returns the home dir.
func setupArchiveHome(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("system git not available")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("RAWCLAW_ARCHIVE", "")

	// Local Claude project.
	proj := filepath.Join(home, ".claude", "projects", "-local-proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","uuid":"aaaa1111-bbbb-cccc-dddd-eeeeeeeeeeee","timestamp":"2026-06-01T10:00:00Z","cwd":"/local/proj","message":{"role":"user","content":"local beacon"}}`
	if err := os.WriteFile(filepath.Join(proj, "localsess.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Archive: init against a bare remote, push our dir.
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitS(t, "", "init", "--bare", "--initial-branch=main", bare)
	a, err := archive.Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("archive.Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("PushLocal: %v", err)
	}

	// Foreign machine-b pushes its dir from its own clone.
	cloneB := filepath.Join(t.TempDir(), "clone-b")
	gitS(t, "", "clone", bare, cloneB)
	bdir := filepath.Join(cloneB, "machine-b")
	if err := os.MkdirAll(filepath.Join(bdir, "claude", "-remote-proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"machine_id":"beefbeefbeefbeefbeefbeefbeefbeef","name":"machine-b","hostname":"b-host","updated_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(bdir, ".rawclaw-machine.json"), []byte(manifest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bline := `{"type":"user","uuid":"bbbb2222-cccc-dddd-eeee-ffffffffffff","timestamp":"2026-06-02T10:00:00Z","cwd":"/remote/proj","message":{"role":"user","content":"foreign beacon"}}`
	if err := os.WriteFile(filepath.Join(bdir, "claude", "-remote-proj", "sess-bbbb.jsonl"), []byte(bline+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitS(t, cloneB, "add", "-A")
	gitS(t, cloneB, "commit", "-m", "machine-b: sync transcripts")
	gitS(t, cloneB, "push", "origin", "HEAD")

	if _, err := a.Pull(context.Background(), false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	return home
}

// TestAll_SplicesArchiveScopes: with an archive configured, All() returns the
// local scopes PLUS the foreign machine's scopes — origin-stamped, labeled with
// the machine name — and never the own-machine dir from the clone.
func TestAll_SplicesArchiveScopes(t *testing.T) {
	setupArchiveHome(t)

	scs := All("", false)
	var local, foreign *view.Scope
	for i := range scs {
		sc := &scs[i]
		switch {
		case sc.TDir != "" && strings.HasSuffix(sc.TDir, "-local-proj"):
			local = sc
		case sc.Origin != "":
			foreign = sc
		}
		if strings.Contains(sc.Project, "machine-a/") {
			t.Errorf("own machine dir enumerated from the clone: %q", sc.Project)
		}
	}
	if local == nil {
		t.Error("local project scope missing after the splice")
	}
	if foreign == nil {
		t.Fatalf("foreign archive scope missing; got %+v", scs)
	}
	if foreign.Origin != "beefbeefbeefbeefbeefbeefbeefbeef" {
		t.Errorf("foreign Origin = %q, want the manifest machine id", foreign.Origin)
	}
	if !strings.HasPrefix(foreign.Project, "machine-b/") {
		t.Errorf("foreign Project = %q, want machine-b/ prefix", foreign.Project)
	}
}

// TestAll_SourceFilterAppliesToArchiveScopes: --source claude keeps foreign
// claude scopes; --source codex drops them (machine-b pushed no codex tree).
func TestAll_SourceFilterAppliesToArchiveScopes(t *testing.T) {
	setupArchiveHome(t)

	foreignCount := func(scs []view.Scope) int {
		n := 0
		for _, sc := range scs {
			if sc.Origin != "" {
				n++
			}
		}
		return n
	}
	if n := foreignCount(All("claude", false)); n != 1 {
		t.Errorf("--source claude foreign scopes = %d, want 1", n)
	}
	if n := foreignCount(All("codex", false)); n != 0 {
		t.Errorf("--source codex foreign scopes = %d, want 0 (machine-b has no codex tree)", n)
	}
}

// TestResolve_StaleArchiveScope: a Stale replica scope resolves to its db with
// IndexStale — the existing stale-fallback posture picks it up from there.
func TestResolve_StaleArchiveScope(t *testing.T) {
	dbp, status, err := Resolve(view.Scope{DBP: "/tmp/some.db", Stale: true}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dbp != "/tmp/some.db" {
		t.Errorf("dbp = %q, want the scope's DBP", dbp)
	}
	if status != index.IndexStale {
		t.Errorf("status = %v, want IndexStale for a stale replica", status)
	}
}

// TestOrphanScanSkipsArchiveDBs: an archive-namespaced cache db must never be
// picked up by the Claude orphan-db scan — its machine dir in the clone is the
// live source; double-listing it would duplicate every foreign hit.
func TestOrphanScanSkipsArchiveDBs(t *testing.T) {
	setupArchiveHome(t)
	_ = All("", false) // the splice creates archive-*.db files in the cache dir

	// Claude() (live + orphan scan, NO archive splice) must not surface them.
	for _, sc := range Claude() {
		if strings.Contains(filepath.Base(sc.DBP), "archive-") {
			t.Errorf("orphan scan surfaced an archive db as a Claude scope: %+v", sc)
		}
	}
}
