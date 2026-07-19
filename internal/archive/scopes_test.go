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

	scopes := a.Scopes(t.Context(), false)
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

	for _, sc := range a.Scopes(t.Context(), false) {
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

	for _, sc := range a.Scopes(t.Context(), false) {
		if strings.HasPrefix(sc.Project, "old-name/") {
			t.Errorf("renamed own dir enumerated as foreign: %q", sc.Project)
		}
	}
}

// TestScopes_RenamedForeignDirDeduped: a FOREIGN machine that re-registered
// under a new name leaves its old dir in the archive forever (the archive
// never prunes) — both dirs claim the same machine id and hold the identical
// pre-rename sessions. Only ONE of them (the newer registration) may be
// enumerated, or every pre-rename session would double-list under an
// identical, never-disambiguable session id.
func TestScopes_RenamedForeignDirDeduped(t *testing.T) {
	const foreignID = "beefbeefbeefbeefbeefbeefbeefbeef"
	a := initArchiveWithForeign(t, "machine-b", foreignID) // UpdatedAt 2026-01-01

	// The same machine's OLD dir: same id, older registration, same session.
	old := filepath.Join(a.ClonePath(), "machine-b-old")
	if err := writeManifest(old, manifest{
		MachineID: foreignID, Name: "machine-b-old", Hostname: "b-host",
		UpdatedAt: "2025-06-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, old, "claude/-remote-proj/sess-bbbb.jsonl",
		`{"type":"user","message":{"role":"user","content":"foreign hello"}}`+"\n")
	gitT(t, a.ClonePath(), "add", "-A")
	gitT(t, a.ClonePath(), "commit", "-m", "machine-b-old: leftover pre-rename dir")

	var names []string
	for _, sc := range a.Scopes(t.Context(), false) {
		if sc.Origin == foreignID {
			names = append(names, sc.Project)
		}
		if strings.HasPrefix(sc.Project, "machine-b-old/") {
			t.Errorf("stale pre-rename dir enumerated: %q", sc.Project)
		}
	}
	if len(names) != 2 { // claude + codex scopes of the CURRENT dir only
		t.Errorf("foreign scopes = %v, want exactly the current dir's two", names)
	}
}

// TestScopes_EmptyMachineIDSkipped: a manifest without a machine_id is
// malformed — enumerating it would stamp its rows with the LOCAL machine id
// (empty origin falls through to the local stamp), corrupting provenance.
func TestScopes_EmptyMachineIDSkipped(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	bad := filepath.Join(a.ClonePath(), "machine-x")
	if err := writeManifest(bad, manifest{
		MachineID: "", Name: "machine-x", Hostname: "x-host", UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, bad, "claude/-x-proj/sess-x.jsonl",
		`{"type":"user","message":{"role":"user","content":"x"}}`+"\n")
	gitT(t, a.ClonePath(), "add", "-A")
	gitT(t, a.ClonePath(), "commit", "-m", "machine-x: malformed manifest")

	for _, sc := range a.Scopes(t.Context(), false) {
		if strings.HasPrefix(sc.Project, "machine-x/") {
			t.Errorf("empty-machine_id dir enumerated: %q", sc.Project)
		}
	}
}

// TestScopes_ManifestlessDirSkipped: a top-level dir without a manifest is not
// a machine dir (a stray README dir, a half-pushed registration) — skipped.
func TestScopes_ManifestlessDirSkipped(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	stray := filepath.Join(a.ClonePath(), "stray")
	writeTranscript(t, stray, "claude/-x/sess-s.jsonl", "{}\n")

	for _, sc := range a.Scopes(t.Context(), false) {
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
	if scopes := a.Scopes(t.Context(), false); len(scopes) != 0 {
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
	for _, sc := range a.Scopes(t.Context(), false) {
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

// countSessionRows returns how many rows in dbp's sessions (and messages)
// tables carry the given session id.
func countSessionRows(t *testing.T, dbp, sessionID string) (sessions, messages int) {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", dbp, err)
	}
	defer con.Close()
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions WHERE id=?", sessionID).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sessionID).Scan(&messages); err != nil {
		t.Fatal(err)
	}
	return sessions, messages
}

// TestScopes_OwnerDeletePropagatesToReplicaIndex: machine B deletes one of its
// own sessions and pushes (E5 — the file leaves the archive); after this
// machine pulls and reindexes, the replica scope db must drop the session too.
// For ARCHIVE scopes the clone IS the source of truth and the replica db is a
// rebuildable cache — absence from the clone is authoritative. Durable
// retention (D1/D2) protects LOCAL sources from upstream purges; it must not
// resurrect a session its owner explicitly deleted.
func TestScopes_OwnerDeletePropagatesToReplicaIndex(t *testing.T) {
	const foreignID = "beefbeefbeefbeefbeefbeefbeefbeef"
	a := initArchiveWithForeign(t, "machine-b", foreignID)

	// Machine B pushes a SECOND session per source into the same project /
	// cwd group, so each scope survives the later single-session delete (a
	// scope with zero sessions left stops being enumerated at all).
	cloneB := filepath.Join(t.TempDir(), "clone-b2")
	gitT(t, "", "clone", a.cfg.Remote, cloneB)
	writeTranscript(t, cloneB, "machine-b/claude/-remote-proj/sess-keep.jsonl",
		`{"type":"user","timestamp":"2026-06-03T10:00:00Z","cwd":"/remote/proj","message":{"role":"user","content":"foreign keeper"}}`+"\n")
	writeTranscript(t, cloneB, "machine-b/codex/2026/07/rollout-keep.jsonl",
		`{"type":"session_meta","timestamp":"2026-06-03T10:00:00Z","payload":{"id":"rollout-keep","cwd":"/remote/proj"}}`+"\n"+
			`{"type":"response_item","timestamp":"2026-06-03T10:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"foreign codex keeper"}]}}`+"\n")
	gitT(t, cloneB, "add", "-A")
	gitT(t, cloneB, "commit", "-m", "machine-b: sync transcripts")
	gitT(t, cloneB, "push", "origin", "HEAD")
	if _, err := a.Pull(context.Background(), false); err != nil {
		t.Fatalf("pull second foreign sessions: %v", err)
	}

	// First search-time ingest: all four foreign sessions land in the replica dbs.
	dbps := map[string]string{} // source → db path
	scs := a.Scopes(t.Context(), false)
	if len(scs) != 2 {
		t.Fatalf("Scopes = %d, want exactly 2 (one per source; the source→db map below assumes it)", len(scs))
	}
	for _, sc := range scs {
		dbps[sc.Source] = sc.DBP
	}
	for src, sids := range map[string][]string{
		"claude": {"sess-bbbb", "sess-keep"},
		"codex":  {"rollout-ffff", "rollout-keep"},
	} {
		for _, sid := range sids {
			if s, m := countSessionRows(t, dbps[src], sid); s == 0 || m == 0 {
				t.Fatalf("pre-delete %s session %s not ingested (sessions=%d messages=%d)", src, sid, s, m)
			}
		}
	}

	// Machine B deletes one session per source and pushes the removal (delete
	// propagation is git-level: the files leave the archive).
	for _, rel := range []string{
		"machine-b/claude/-remote-proj/sess-bbbb.jsonl",
		"machine-b/codex/2026/07/rollout-ffff.jsonl",
	} {
		if err := os.Remove(filepath.Join(cloneB, rel)); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, cloneB, "add", "-A")
	gitT(t, cloneB, "commit", "-m", "machine-b: sync transcripts")
	gitT(t, cloneB, "push", "origin", "HEAD")

	if _, err := a.Pull(context.Background(), false); err != nil {
		t.Fatalf("pull after owner delete: %v", err)
	}

	// Reindex via the normal search-time path: the deleted sessions must be
	// gone from the replica dbs — not retained as searchable ghosts — while
	// the surviving sessions stay indexed.
	a.Scopes(t.Context(), false)
	for src, sid := range map[string]string{"claude": "sess-bbbb", "codex": "rollout-ffff"} {
		if s, m := countSessionRows(t, dbps[src], sid); s != 0 || m != 0 {
			t.Errorf("owner-deleted %s session %s resurrected by the replica index (sessions=%d messages=%d)",
				src, sid, s, m)
		}
	}
	for src, sid := range map[string]string{"claude": "sess-keep", "codex": "rollout-keep"} {
		if s, m := countSessionRows(t, dbps[src], sid); s == 0 || m == 0 {
			t.Errorf("surviving %s session %s lost by the replica reconcile (sessions=%d messages=%d)",
				src, sid, s, m)
		}
	}
}

// TestScopes_BusySyncSkipsIngest: while a sibling sync holds the machine-wide
// lock (a pull may be mid-rebase, tearing the worktree down file-by-file),
// scope enumeration must NOT ingest — replica reconciliation would read the
// half-rewritten tree as authoritative absence and prune live foreign
// sessions. Scopes still enumerates (existing dbs keep serving); once the
// lock frees, the next pass ingests normally.
func TestScopes_BusySyncSkipsIngest(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	release, ok := tryAcquireSyncLock()
	if !ok {
		t.Fatal("could not take the sync lock in an idle fixture")
	}
	scs := a.Scopes(t.Context(), false)
	if len(scs) != 2 {
		t.Fatalf("Scopes under a held sync lock = %d scopes, want 2 (enumeration must survive)", len(scs))
	}
	for _, sc := range scs {
		if _, err := os.Stat(sc.DBP); err == nil {
			t.Errorf("scope %q ingested %s while a sync held the lock", sc.Project, sc.DBP)
		}
	}
	release()

	a.Scopes(t.Context(), false)
	for _, sc := range scs {
		if _, err := os.Stat(sc.DBP); err != nil {
			t.Errorf("scope %q not ingested after the lock freed: %v", sc.Project, err)
		}
	}
}

// TestScopes_UnverifiedCloneNotEnumerated: a clone without the completed-
// clone sentinel (torn mid-clone, or pre-sentinel and not yet adopted) is
// not enumerated by Scopes OR LookupScopes — its partial tree must never
// feed replica reconciliation, where absence is authoritative.
func TestScopes_UnverifiedCloneNotEnumerated(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	if err := os.Remove(filepath.Join(a.ClonePath(), ".git", cloneSentinel)); err != nil {
		t.Fatal(err)
	}
	if scs := a.Scopes(t.Context(), false); len(scs) != 0 {
		t.Errorf("Scopes on an unverified clone = %d scopes, want 0", len(scs))
	}
	if scs := a.LookupScopes(); len(scs) != 0 {
		t.Errorf("LookupScopes on an unverified clone = %d scopes, want 0", len(scs))
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

// TestScopes_CanceledCtxProbeDegradesToStale: the staleness git probes run
// under the CALLER's context — a dead ctx (the CLI watchdog firing) kills the
// probe instead of letting it run unbounded, and the unknown freshness reads
// as stale (never silently fresh). Enumeration itself still succeeds.
func TestScopes_CanceledCtxProbeDegradesToStale(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scs := a.Scopes(ctx, false)
	if len(scs) != 2 {
		t.Fatalf("Scopes(dead ctx) = %d scopes, want 2 (enumeration must survive a dead probe)", len(scs))
	}
	for _, sc := range scs {
		if !sc.Stale {
			t.Errorf("scope %q Stale = false under a dead ctx; unknown freshness must read stale", sc.Project)
		}
	}
}

// TestLookupScopes_NeverIngests: LookupScopes enumerates the same foreign
// scopes as Scopes but builds NOTHING — before any search-time ingest the
// scope dbs must not exist; after a real Scopes pass, LookupScopes points at
// the exact same, now-openable dbs.
func TestLookupScopes_NeverIngests(t *testing.T) {
	a := initArchiveWithForeign(t, "machine-b", "beefbeefbeefbeefbeefbeefbeefbeef")

	look := a.LookupScopes()
	if len(look) != 2 {
		t.Fatalf("LookupScopes() = %d scopes, want 2 (foreign claude + codex)", len(look))
	}
	for _, sc := range look {
		if sc.DBP == "" {
			t.Fatalf("lookup scope %q has empty DBP", sc.Project)
		}
		if _, err := os.Stat(sc.DBP); err == nil {
			t.Errorf("LookupScopes built %s — the lookup path must never ingest", sc.DBP)
		}
	}

	ingested := map[string]bool{}
	for _, sc := range a.Scopes(t.Context(), false) {
		ingested[sc.DBP] = true
	}
	for _, sc := range look {
		if !ingested[sc.DBP] {
			t.Errorf("lookup scope db %s does not match any ingested scope db", sc.DBP)
		}
		if _, err := os.Stat(sc.DBP); err != nil {
			t.Errorf("after Scopes ingest, lookup db %s still absent: %v", sc.DBP, err)
		}
	}
}
