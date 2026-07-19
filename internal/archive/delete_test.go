package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
)

// TestPushLocal_DeletePropagates: a session deleted locally (file removed +
// id tombstoned, the v0.3.0 delete semantics) is removed from the REMOTE by
// the next push — an explicit delete is never resurrected by the archive.
// Untouched sessions and the machine manifest survive.
func TestPushLocal_DeletePropagates(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	writeTranscript(t, filepath.Join(home, ".claude", "projects"),
		"-tmp-proj/sess-keep.jsonl", `{"type":"user","text":"keep"}`+"\n")
	doomed := writeTranscript(t, filepath.Join(home, ".claude", "projects"),
		"-tmp-proj/sess-doomed.jsonl", `{"type":"user","text":"doomed"}`+"\n")

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// v0.3.0 delete: the local file goes away and the id is tombstoned.
	if err := os.Remove(doomed); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.TombstoneIDs("", []string{"sess-doomed"}); err != nil {
		t.Fatal(err)
	}

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("push after delete: %v", err)
	}
	if rep.Removed != 1 {
		t.Errorf("Removed = %d, want 1", rep.Removed)
	}
	if !rep.Pushed {
		t.Error("deletion was not pushed")
	}

	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-doomed.jsonl")); !os.IsNotExist(err) {
		t.Error("deleted session still in the archive — delete did not propagate")
	}
	for _, want := range []string{
		"machine-a/claude/-tmp-proj/sess-keep.jsonl",
		"machine-a/.rawclaw-machine.json",
	} {
		if _, err := os.Stat(filepath.Join(verify, want)); err != nil {
			t.Errorf("remote missing %s after delete propagation: %v", want, err)
		}
	}

	// The local tombstone is untouched: delete semantics stay v0.3.0.
	set, err := lifecycle.LoadTombstones("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set["sess-doomed"]; !ok {
		t.Error("local tombstone lost the deleted id")
	}
}

// TestPushLocal_DeletePropagatesCodex: codex rollouts are addressed by their
// session_meta id, not the file stem — a tombstoned codex id removes the
// rollout file from the archive.
func TestPushLocal_DeletePropagatesCodex(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	rollout := writeTranscript(t, filepath.Join(home, ".codex", "sessions"),
		"2026/07/rollout-2026-07-19-abc.jsonl",
		`{"type":"session_meta","payload":{"id":"codex-sess-1","cwd":"/repo","thread_source":"user"}}`+"\n")

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	if err := os.Remove(rollout); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.TombstoneIDs("", []string{"codex-sess-1"}); err != nil {
		t.Fatal(err)
	}

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("push after codex delete: %v", err)
	}
	if rep.Removed != 1 {
		t.Errorf("Removed = %d, want 1", rep.Removed)
	}

	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/codex/2026/07/rollout-2026-07-19-abc.jsonl")); !os.IsNotExist(err) {
		t.Error("deleted codex rollout still in the archive")
	}
}

// TestPushLocal_TombstonedFileNotResurrected: a tombstoned session whose local
// file is still on disk (restored from a backup, race with the delete) must
// NOT reach the archive — the tombstone outranks presence, or a deleted
// session would resurrect through the push path.
func TestPushLocal_TombstonedFileNotResurrected(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	writeTranscript(t, filepath.Join(home, ".claude", "projects"),
		"-tmp-proj/sess-zombie.jsonl", `{"type":"user","text":"zombie"}`+"\n")
	if err := lifecycle.TombstoneIDs("", []string{"sess-zombie"}); err != nil {
		t.Fatal(err)
	}

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("push: %v", err)
	}

	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-zombie.jsonl")); !os.IsNotExist(err) {
		t.Error("tombstoned session resurrected into the archive")
	}
}

// TestPushLocal_DeleteNeverTouchesForeignDirs: a tombstoned id that matches a
// FOREIGN machine's session must not remove that machine's file — foreign
// dirs are read-only from every box (no cross-machine delete in v1).
func TestPushLocal_DeleteNeverTouchesForeignDirs(t *testing.T) {
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

	// Machine B pushes a session whose id we will (wrongly) tombstone here.
	cloneB := filepath.Join(t.TempDir(), "clone-b")
	gitT(t, "", "clone", bare, cloneB)
	writeTranscript(t, cloneB, "machine-b/claude/-proj/sess-foreign.jsonl", "{}\n")
	if err := writeManifest(filepath.Join(cloneB, "machine-b"), manifest{
		MachineID: "beefbeefbeefbeefbeefbeefbeefbeef",
		Name:      "machine-b", Hostname: "b-host", UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	gitT(t, cloneB, "add", "-A")
	gitT(t, cloneB, "commit", "-m", "machine-b: sync transcripts")
	gitT(t, cloneB, "push", "origin", "HEAD")

	if _, err := a.Pull(context.Background(), false); err != nil {
		t.Fatalf("pull foreign dir: %v", err)
	}
	if err := lifecycle.TombstoneIDs("", []string{"sess-foreign"}); err != nil {
		t.Fatal(err)
	}

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("push with foreign-matching tombstone: %v", err)
	}
	if rep.Removed != 0 {
		t.Errorf("Removed = %d, want 0 (foreign sessions are read-only)", rep.Removed)
	}

	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-b/claude/-proj/sess-foreign.jsonl")); err != nil {
		t.Errorf("foreign session removed by a local tombstone: %v", err)
	}
}

// TestPushLocal_MirrorModeNeverPrunesArchive: RAWCLAW_RETENTION=mirror governs
// the LOCAL index only — a push under mirror mode with locally-purged (but not
// tombstoned) sessions leaves every archive copy in place. The archive IS the
// durable mirror (absence never deletes; only an explicit tombstone does).
func TestPushLocal_MirrorModeNeverPrunesArchive(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	claudeFile, codexFile := seedTranscripts(t, home)

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// The source tool purges both transcripts locally; the user runs mirror
	// mode. NO tombstone exists — nothing was explicitly deleted.
	t.Setenv("RAWCLAW_RETENTION", "mirror")
	if err := os.Remove(claudeFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(codexFile); err != nil {
		t.Fatal(err)
	}

	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("push under mirror mode: %v", err)
	}
	if rep.Removed != 0 {
		t.Errorf("Removed = %d, want 0 (absence never deletes from the archive)", rep.Removed)
	}

	verify := checkoutRemote(t, bare)
	for _, want := range []string{
		"machine-a/claude/-tmp-proj/sess-1111.jsonl",
		"machine-a/codex/2026/07/rollout-abc.jsonl",
	} {
		if _, err := os.Stat(filepath.Join(verify, want)); err != nil {
			t.Errorf("archive copy pruned under mirror mode: %s: %v", want, err)
		}
	}
}

// TestPushLocal_DeleteReportsOnce: the archive-side removal is reported by
// the push that performs it — not by every later push. A tombstoned session
// whose local file still exists is skipped at the copy, so it can neither
// resurrect nor be re-copied-and-re-removed forever.
func TestPushLocal_DeleteReportsOnce(t *testing.T) {
	home := newTestHome(t)
	bare := initBareRepo(t)
	writeTranscript(t, filepath.Join(home, ".claude", "projects"),
		"-tmp-proj/sess-zombie.jsonl", `{"type":"user","text":"zombie"}`+"\n")
	writeTranscript(t, filepath.Join(home, ".claude", "projects"),
		"-tmp-proj/sess-live.jsonl", `{"type":"user","text":"live"}`+"\n")

	a, err := Init(context.Background(), bare, "machine-a")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := a.PushLocal(context.Background()); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Tombstone the zombie but leave its local file in place (restored
	// backup / delete race). The next push removes its archive copy once.
	if err := lifecycle.TombstoneIDs("", []string{"sess-zombie"}); err != nil {
		t.Fatal(err)
	}
	rep, err := a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("propagation push: %v", err)
	}
	if rep.Removed != 1 {
		t.Errorf("propagation push Removed = %d, want 1", rep.Removed)
	}

	// A later push with unrelated new content: the zombie must not be
	// re-copied or re-counted.
	writeTranscript(t, filepath.Join(home, ".claude", "projects"),
		"-tmp-proj/sess-new.jsonl", `{"type":"user","text":"new"}`+"\n")
	rep, err = a.PushLocal(context.Background())
	if err != nil {
		t.Fatalf("later push: %v", err)
	}
	if rep.Removed != 0 {
		t.Errorf("later push Removed = %d, want 0 (removal already propagated)", rep.Removed)
	}
	if rep.Copied != 1 {
		t.Errorf("later push Copied = %d, want 1 (only the new session)", rep.Copied)
	}

	verify := checkoutRemote(t, bare)
	if _, err := os.Stat(filepath.Join(verify, "machine-a/claude/-tmp-proj/sess-zombie.jsonl")); !os.IsNotExist(err) {
		t.Error("tombstoned session present in the archive")
	}
	for _, want := range []string{
		"machine-a/claude/-tmp-proj/sess-live.jsonl",
		"machine-a/claude/-tmp-proj/sess-new.jsonl",
	} {
		if _, err := os.Stat(filepath.Join(verify, want)); err != nil {
			t.Errorf("remote missing %s: %v", want, err)
		}
	}
}
