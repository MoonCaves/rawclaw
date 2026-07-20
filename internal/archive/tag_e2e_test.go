package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// TestIngestForeignTags_EndToEndConflict exercises the whole ingest path in an
// isolated temp clone: two machines hold conflicting real tags for one foreign
// session; ingest must land the deterministic winner in the scope db AND surface
// the conflict, while both tag files stay on disk.
func TestIngestForeignTags_EndToEndConflict(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the conflict state file + ingest stamp
	clone := t.TempDir()

	// box-b is a registered foreign machine (own machine is box-a).
	if err := writeManifest(filepath.Join(clone, "box-b"), manifest{
		MachineID: "box-b-id", Name: "box-b", UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("write box-b manifest: %v", err)
	}

	// A foreign scope db holding session S's messages (box-b's transcript home).
	dbp := filepath.Join(t.TempDir(), "box-b-scope.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("ConnectRW: %v", err)
	}
	if err := store.Rebuild(con); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	storetest.InsertSession(t, con, storetest.Session{ID: "S", MessageCount: 3})
	con.Close() // release the single writer before ingest reopens it

	// Two machines tag S differently: box-a thin, box-b rich → a real conflict.
	writeTag(t, clone, "box-a", "box-a-id", TagFile{
		SessionID: "S", OriginMachine: "box-a-id", Segments: []TagSegment{{StartUUID: "u1", Topic: "thin"}},
	})
	writeTag(t, clone, "box-b", "box-b-id", TagFile{
		SessionID: "S", OriginMachine: "box-b-id",
		Segments: []TagSegment{{StartUUID: "u1", Topic: "rich"}, {StartUUID: "u2", Topic: "more"}},
	})

	a := &Archive{cfg: Config{Name: "box-a"}, clone: clone, machineID: "box-a-id"}
	a.ingestForeignTags([]view.Scope{{DBP: dbp, Origin: "box-b-id", Source: "claude"}}, true)

	// Winner landed: box-b's rich set (box-b-id > box-a-id).
	ro, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("ConnectRO: %v", err)
	}
	defer ro.Close()
	got, _ := store.TopicsForSession(ro, "S")
	if len(got) != 2 || got[0].Topic != "rich" || got[0].OriginMachine != "box-b-id" {
		t.Fatalf("winning set = %+v, want box-b's {rich,more}", got)
	}

	// Conflict surfaced.
	if c := readTagConflicts(); len(c) != 1 || c[0] != "S" {
		t.Errorf("recorded conflicts = %v, want [S]", c)
	}

	// Both tag files still on disk (loser retained).
	for _, md := range []string{"box-a", "box-b"} {
		p := filepath.Join(clone, md, tagsDirName, "S.json")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("tag file %s missing after ingest (loser must be retained): %v", p, err)
		}
	}
}

// writeTag drops one machine's tag file into the clone (temp staged under .git,
// like production).
func writeTag(t *testing.T, clone, machine, _ string, tf TagFile) {
	t.Helper()
	dir := filepath.Join(clone, machine, tagsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(clone, ".git", "rawclaw-tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTagFileAtomic(tmp, dir, tf); err != nil {
		t.Fatalf("write tag %s/%s: %v", machine, tf.SessionID, err)
	}
}
