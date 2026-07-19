package archive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// seedForeignMachine writes a foreign machine dir (manifest + one Claude
// transcript + one Codex rollout) into a clone-shaped tree and returns the
// machine dir.
func seedForeignMachine(t *testing.T, cloneRoot, name, machineID string) string {
	t.Helper()
	dir := filepath.Join(cloneRoot, name)
	if err := writeManifest(dir, manifest{
		MachineID: machineID,
		Name:      name,
		Hostname:  name + "-host",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, dir, "claude/-remote-proj/sess-bbbb.jsonl",
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","cwd":"/remote/proj","message":{"role":"user","content":"foreign hello"}}`+"\n")
	writeTranscript(t, dir, "codex/2026/07/rollout-ffff.jsonl",
		`{"type":"session_meta","timestamp":"2026-06-01T10:00:00Z","payload":{"id":"rollout-ffff","cwd":"/remote/proj"}}`+"\n"+
			`{"type":"response_item","timestamp":"2026-06-01T10:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"foreign codex hello"}]}}`+"\n")
	return dir
}

// initArchiveWithForeign stands up a full archive: init against a bare remote,
// push machine-a's own transcripts, then land a foreign machine dir on the
// remote and pull it into the clone. Returns the loaded archive.
func initArchiveWithForeign(t *testing.T, foreignName, foreignID string) *Archive {
	t.Helper()
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("PushLocal: %v", err)
	}

	// The foreign machine pushes from its own clone of the same remote.
	cloneB := filepath.Join(t.TempDir(), "clone-b")
	gitT(t, "", "clone", bare, cloneB)
	seedForeignMachine(t, cloneB, foreignName, foreignID)
	gitT(t, cloneB, "add", "-A")
	gitT(t, cloneB, "commit", "-m", foreignName+": sync transcripts")
	gitT(t, cloneB, "push", "origin", "HEAD")

	if _, err := a.Pull(context.Background(), false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	return a
}

// TestScopes_ForeignDirsOnly: the clone holds our own dir plus one foreign
// machine dir → Scopes returns ONLY the foreign machine's scopes (claude +
// codex), labeled with the machine name and carrying the manifest's machine id
// as Origin. Our own dir is excluded — the live local tree is fresher.
func TestScopes_ForeignDirsOnly(t *testing.T) {
	const foreignID = "beefbeefbeefbeefbeefbeefbeefbeef"
	a := initArchiveWithForeign(t, "machine-b", foreignID)

	scopes := a.Scopes(false)
	if len(scopes) != 2 {
		t.Fatalf("Scopes() = %d scopes (%+v), want 2 (foreign claude + codex)", len(scopes), scopes)
	}
	sources := map[string]view.Scope{}
	for _, sc := range scopes {
		sources[sc.Source] = sc
		if sc.Origin != foreignID {
			t.Errorf("scope %q Origin = %q, want manifest machine id %q", sc.Project, sc.Origin, foreignID)
		}
		if sc.OriginName != "machine-b" {
			t.Errorf("scope %q OriginName = %q, want machine-b", sc.Project, sc.OriginName)
		}
		if !strings.HasPrefix(sc.Project, "machine-b/") {
			t.Errorf("scope Project = %q, want machine-name prefix", sc.Project)
		}
		if sc.DBP == "" {
			t.Errorf("scope %q DBP empty, want a pre-ensured db", sc.Project)
		}
		if sc.Stale {
			t.Errorf("scope %q Stale = true for a just-pushed dir", sc.Project)
		}
		if strings.Contains(sc.Project, "machine-a") {
			t.Errorf("own machine dir leaked into scopes: %q", sc.Project)
		}
	}
	if _, ok := sources["claude"]; !ok {
		t.Error("no claude scope for the foreign machine")
	}
	if _, ok := sources["codex"]; !ok {
		t.Error("no codex scope for the foreign machine")
	}
}

// TestScopes_StampsForeignOrigin: the ingested rows carry the FOREIGN machine
// id from the dir's manifest as origin_machine — not this machine's id.
func TestScopes_StampsForeignOrigin(t *testing.T) {
	const foreignID = "beefbeefbeefbeefbeefbeefbeefbeef"
	a := initArchiveWithForeign(t, "machine-b", foreignID)

	for _, sc := range a.Scopes(false) {
		if n := checkAllOrigins(t, sc.DBP, foreignID); n == 0 {
			t.Errorf("scope %q ingested zero sessions", sc.Project)
		}
	}
}

// checkAllOrigins asserts every session row in dbp carries origin_machine ==
// want and returns the row count. Connections close via defer so a fatal
// assertion cannot leak them.
func checkAllOrigins(t *testing.T, dbp, want string) int {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", dbp, err)
	}
	defer con.Close()
	rows, err := con.Query("SELECT id, origin_machine FROM sessions")
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, origin string
		if err := rows.Scan(&id, &origin); err != nil {
			t.Fatal(err)
		}
		n++
		if origin != want {
			t.Errorf("session %s origin_machine = %q, want %q", id, origin, want)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestScopes_RenamedOwnDirExcluded: a dir whose manifest carries OUR machine id
// under a different name (this machine before a rename) is still our own data —
// it must not re-enter as a foreign scope and duplicate local hits.
func TestScopes_RenamedOwnDirExcluded(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	// A dir claimed by OUR id under an old name, present in the clone.
	old := filepath.Join(a.ClonePath(), "old-name")
	if err := writeManifest(old, manifest{
		MachineID: a.machineID, Name: "old-name", Hostname: "h", UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, old, "claude/-tmp-proj/sess-old.jsonl", `{"type":"user","message":{"role":"user","content":"old"}}`+"\n")
	gitT(t, a.ClonePath(), "add", "-A")
	gitT(t, a.ClonePath(), "commit", "-m", "old-name: stale self dir")

	for _, sc := range a.Scopes(false) {
		if strings.HasPrefix(sc.Project, "old-name/") {
			t.Errorf("renamed own dir enumerated as foreign: %q", sc.Project)
		}
	}
}

// TestScopes_ManifestlessDirSkipped: a top-level dir without a manifest is not
// a machine dir (a stray README dir, a half-pushed registration) — skipped.
func TestScopes_ManifestlessDirSkipped(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	stray := filepath.Join(a.ClonePath(), "stray")
	writeTranscript(t, stray, "claude/-x/sess-s.jsonl", "{}\n")

	for _, sc := range a.Scopes(false) {
		if strings.HasPrefix(sc.Project, "stray/") {
			t.Errorf("manifest-less dir enumerated: %q", sc.Project)
		}
	}
}

// TestScopes_NoCloneNoScopes: a configured archive whose clone is absent (never
// pulled, or deleted for recovery) yields zero scopes — enumeration never does
// network; `archive pull` is the refresh path.
func TestScopes_NoCloneNoScopes(t *testing.T) {
	newTestHome(t)
	if err := writeConfig(Config{Remote: "/tmp/nowhere.git", Name: "machine-a"}); err != nil {
		t.Fatal(err)
	}
	a, err := Load()
	if err != nil || a == nil {
		t.Fatalf("Load: (%v, %v)", a, err)
	}
	if scopes := a.Scopes(false); len(scopes) != 0 {
		t.Errorf("Scopes() with no clone = %d scopes, want 0", len(scopes))
	}
}

// TestScopes_StaleDirFlagged: a foreign dir whose last commit is older than the
// staleness window is still enumerated (results must be served) but flagged
// Stale — the carrier into the existing stale-fallback search posture.
func TestScopes_StaleDirFlagged(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	// Age machine-b's history: rewrite its commit with an old date via a fresh
	// commit stamped in the past on a new foreign dir.
	old := filepath.Join(a.ClonePath(), "machine-c")
	if err := writeManifest(old, manifest{
		MachineID: "c0dec0dec0dec0dec0dec0dec0dec0de", Name: "machine-c", Hostname: "c-host",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, old, "claude/-c-proj/sess-cccc.jsonl", `{"type":"user","message":{"role":"user","content":"old machine"}}`+"\n")
	gitEnvCommit(t, a.ClonePath(), "2020-01-01T00:00:00Z", "machine-c: ancient sync")

	var sawFresh, sawStale bool
	for _, sc := range a.Scopes(false) {
		switch {
		case strings.HasPrefix(sc.Project, "machine-b/"):
			sawFresh = true
			if sc.Stale {
				t.Errorf("fresh dir flagged stale: %q", sc.Project)
			}
		case strings.HasPrefix(sc.Project, "machine-c/"):
			sawStale = true
			if !sc.Stale {
				t.Errorf("stale dir NOT flagged: %q", sc.Project)
			}
		}
	}
	if !sawFresh || !sawStale {
		t.Errorf("expected both machines enumerated (fresh=%v stale=%v)", sawFresh, sawStale)
	}
}

// gitEnvCommit stages everything and commits with a pinned author+committer
// date, so a test can plant history at an arbitrary age.
func gitEnvCommit(t *testing.T, dir, isoDate, msg string) {
	t.Helper()
	gitT(t, dir, "add", "-A")
	cmd := exec.Command("git",
		"-c", "user.name=test", "-c", "user.email=test@example.invalid",
		"commit", "-m", msg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+isoDate, "GIT_COMMITTER_DATE="+isoDate)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit (dated): %v\n%s", err, out)
	}
}
