package archive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// gitEnvT runs git in dir with extra environment (pinned dates), failing the
// test on error — the env-carrying sibling of gitT.
func gitEnvT(t *testing.T, dir string, extraEnv []string, args ...string) {
	t.Helper()
	full := append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.invalid",
		"-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// pushForeignDir pushes a foreign machine dir into the remote with a pinned
// commit date, from its own scratch clone.
func pushForeignDir(t *testing.T, bare, name, date string) {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone-"+name)
	gitT(t, "", "clone", bare, clone)
	writeTranscript(t, clone, name+"/claude/-proj/sess-"+name+".jsonl", "{}\n")
	if err := writeManifest(filepath.Join(clone, name), manifest{
		MachineID: "beefbeefbeefbeefbeefbeefbeefbeef",
		Name:      name, Hostname: name + "-host", UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	gitT(t, clone, "add", "-A")
	var env []string
	if date != "" {
		env = []string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}
	}
	gitEnvT(t, clone, env, "commit", "-m", name+": sync transcripts")
	gitT(t, clone, "push", "origin", "HEAD")
}

// TestStatus_ReportsMachinesAndSyncStamps: after a push and a pull of a
// foreign dir with an old pinned commit date, Status reports the remote, the
// clone, both sync stamps (fresh → not overdue), and one entry per machine
// carrying its last-new-content commit time — the old foreign dir's pinned
// date verbatim, with no staleness verdict attached.
func TestStatus_ReportsMachinesAndSyncStamps(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push: %v", err)
	}

	oldDate := "2026-01-02T03:04:05Z"
	pushForeignDir(t, bare, "machine-b", oldDate)
	if _, err := a.Pull(context.Background(), false); err != nil {
		t.Fatalf("pull: %v", err)
	}

	st, err := a.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Remote != bare {
		t.Errorf("Remote = %q, want %q", st.Remote, bare)
	}
	if st.Clone != a.ClonePath() {
		t.Errorf("Clone = %q, want %q", st.Clone, a.ClonePath())
	}
	if !st.CloneOK {
		t.Error("CloneOK = false, want true (clone exists)")
	}
	if st.LastPush.IsZero() {
		t.Error("LastPush is zero, want the push stamp")
	}
	if st.LastPull.IsZero() {
		t.Error("LastPull is zero, want the pull stamp")
	}

	byName := map[string]MachineStatus{}
	for _, m := range st.Machines {
		byName[m.Name] = m
	}
	own, ok := byName["machine-a"]
	if !ok {
		t.Fatalf("Status missing own machine; got %+v", st.Machines)
	}
	if !own.Own {
		t.Error("own machine not flagged Own")
	}
	if own.LastCommit.IsZero() {
		t.Error("own LastCommit is zero, want the sync commit time")
	}
	if st.PushOverdue {
		t.Error("PushOverdue right after a successful push")
	}
	if st.PullOverdue {
		t.Error("PullOverdue right after a successful pull")
	}

	foreign, ok := byName["machine-b"]
	if !ok {
		t.Fatalf("Status missing foreign machine; got %+v", st.Machines)
	}
	if foreign.Own {
		t.Error("foreign machine flagged Own")
	}
	wantT, _ := time.Parse(time.RFC3339, oldDate)
	if !foreign.LastCommit.Equal(wantT) {
		t.Errorf("foreign LastCommit = %v, want the pinned fixture time %v", foreign.LastCommit, wantT)
	}
	if foreign.MachineID != "beefbeefbeefbeefbeefbeefbeefbeef" {
		t.Errorf("foreign MachineID = %q, want the manifest id", foreign.MachineID)
	}
}

// TestStatus_OverdueOwnSyncStamps: the only staleness status may assert is
// what this machine knows first-hand — its own last successful push/pull.
// Aged stamps read overdue; a stamp that never existed reads "never" (zero
// time) WITHOUT the overdue flag — "never pulled" is its own honest state,
// not a broken sync loop.
func TestStatus_OverdueOwnSyncStamps(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push: %v", err)
	}

	// Age the push stamp beyond the window; leave the pull stamp absent.
	old := time.Now().Add(-2 * staleAfter)
	if err := os.Chtimes(pushStampPath(), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(pullStampPath()); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	st, err := a.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.PushOverdue {
		t.Error("PushOverdue = false for a push stamp older than the window")
	}
	if !st.LastPull.IsZero() {
		t.Errorf("LastPull = %v, want zero with no stamp", st.LastPull)
	}
	if st.PullOverdue {
		t.Error("PullOverdue = true for a never-pulled machine; never is not overdue")
	}
}

// TestPushLocal_UpToDateStampsSync: a successful PushLocal that verified
// there is nothing to push is still a successful sync — the stamp must
// advance, or an idle-but-healthy machine's own sync reads as overdue/dead.
func TestPushLocal_UpToDateStampsSync(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	old := time.Now().Add(-2 * staleAfter)
	if err := os.Chtimes(pushStampPath(), old, old); err != nil {
		t.Fatal(err)
	}

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("no-op push: %v", err)
	}
	if rep.Committed || rep.Pushed {
		t.Fatalf("second push not a no-op: %+v", rep)
	}
	st, err := os.Stat(pushStampPath())
	if err != nil {
		t.Fatalf("push stamp missing after no-op sync: %v", err)
	}
	// Deterministic: without the refresh the mtime would still equal the aged
	// value; any refresh lands it far after old+staleAfter.
	if !st.ModTime().After(old.Add(staleAfter)) {
		t.Errorf("push stamp not refreshed by a verified-up-to-date sync (mtime %v)", st.ModTime())
	}
}

// TestStatus_NoCloneIsReportedNotFatal: a configured archive whose clone is
// missing (deleted, or never pulled on this box) reports CloneOK=false with
// zero machines — status is an OFFLINE read, it never clones or fetches.
func TestStatus_NoCloneIsReportedNotFatal(t *testing.T) {
	newTestHome(t)
	if err := writeConfig(Config{Remote: "/nonexistent-remote.git", Name: "machine-a"}); err != nil {
		t.Fatal(err)
	}
	a, err := Load()
	if err != nil || a == nil {
		t.Fatalf("Load: (%v, %v)", a, err)
	}

	st, err := a.Status(context.Background())
	if err != nil {
		t.Fatalf("Status without a clone: %v", err)
	}
	if st.CloneOK {
		t.Error("CloneOK = true, want false for a missing clone")
	}
	if len(st.Machines) != 0 {
		t.Errorf("Machines = %+v, want none without a clone", st.Machines)
	}
	if st.Remote != "/nonexistent-remote.git" {
		t.Errorf("Remote = %q, want the configured remote", st.Remote)
	}
}
