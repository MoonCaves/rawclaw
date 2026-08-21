package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
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

// TestIngestForeignTags_CreatesMissingTopicSchema pins the production state that
// broke ingest: an archive-built foreign scope db has the base schema but never
// passed through a path that applies the topic sidecar (no topic_segment).
// Ingest must create the sidecar itself and land the tags, not warn and skip.
func TestIngestForeignTags_CreatesMissingTopicSchema(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clone := t.TempDir()

	// Foreign scope db WITHOUT EnsureTopicSchema — exactly how the archive
	// splice path builds them.
	dbp := filepath.Join(t.TempDir(), "box-b-scope.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("ConnectRW: %v", err)
	}
	if err := store.Rebuild(con); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	storetest.InsertSession(t, con, storetest.Session{ID: "S", MessageCount: 3})
	con.Close()

	writeTag(t, clone, "box-a", "box-a-id", TagFile{
		SessionID: "S", OriginMachine: "box-a-id",
		Segments: []TagSegment{{StartUUID: "u1", Topic: "gateway work"}},
	})

	a := &Archive{cfg: Config{Name: "box-a"}, clone: clone, machineID: "box-a-id"}
	a.ingestForeignTags([]view.Scope{{DBP: dbp, Origin: "box-b-id", Source: "claude"}}, true)

	ro, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("ConnectRO: %v", err)
	}
	defer ro.Close()
	got, err := store.TopicsForSession(ro, "S")
	if err != nil {
		t.Fatalf("TopicsForSession after ingest: %v", err)
	}
	if len(got) != 1 || got[0].Topic != "gateway work" {
		t.Fatalf("segments after ingest = %+v, want the one tagged segment", got)
	}
}

// TestIngestForeignTags_WriteFailure_SkipsAndPreservesConflicts pins Defect 4:
// if applyResolvedTags fails (e.g. SQLite write failure / locked db), ingest must
// set skipped=true so that existing conflict records are preserved (unioned) and
// the ingest stamp is NOT advanced, allowing a future pass to retry.
func TestIngestForeignTags_WriteFailure_SkipsAndPreservesConflicts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clone := t.TempDir()

	// Seed an existing conflict from a prior run.
	writeTagConflicts([]string{"prior-conflict"})

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

	// Install a trigger that causes writes to topic_segment to fail.
	if _, err := con.Exec(`CREATE TRIGGER fail_topic_insert BEFORE INSERT ON topic_segment BEGIN SELECT RAISE(FAIL, 'simulated write error'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	con.Close()

	writeTag(t, clone, "box-a", "box-a-id", TagFile{
		SessionID: "S", OriginMachine: "box-a-id",
		Segments: []TagSegment{{StartUUID: "u1", Topic: "gateway work"}},
	})

	a := &Archive{cfg: Config{Name: "box-a"}, clone: clone, machineID: "box-a-id"}
	a.ingestForeignTags([]view.Scope{{DBP: dbp, Origin: "box-b-id", Source: "claude"}}, true)

	// 1. Existing conflicts must be preserved (not overwritten/wiped by the failed pass).
	conflicts := readTagConflicts()
	if len(conflicts) != 1 || conflicts[0] != "prior-conflict" {
		t.Errorf("conflicts after write failure = %v, want [prior-conflict]", conflicts)
	}

	// 2. Ingest stamp must NOT be created/advanced on skipped pass.
	if _, err := os.Stat(tagIngestStampPath()); !os.IsNotExist(err) {
		t.Errorf("tag ingest stamp exists after failed write pass; want stamp NOT advanced (os.IsNotExist)")
	}

	// Now remove the failing trigger and re-run to verify retry succeeds.
	con, err = store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("ConnectRW: %v", err)
	}
	if _, err := con.Exec("DROP TRIGGER fail_topic_insert"); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	con.Close()

	a.ingestForeignTags([]view.Scope{{DBP: dbp, Origin: "box-b-id", Source: "claude"}}, true)

	// Ingest should have succeeded on retry.
	ro, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("ConnectRO: %v", err)
	}
	defer ro.Close()
	got, err := store.TopicsForSession(ro, "S")
	if err != nil {
		t.Fatalf("TopicsForSession after retry: %v", err)
	}
	if len(got) != 1 || got[0].Topic != "gateway work" {
		t.Fatalf("segments after retry = %+v, want the one tagged segment", got)
	}

	// Stamp should now exist after clean pass.
	if _, err := os.Stat(tagIngestStampPath()); err != nil {
		t.Errorf("tag ingest stamp missing after successful pass: %v", err)
	}
}

// TestScopes_ForeignTagsPublishedToConsolidatedSamePass pins Defect 1 from Task 195:
// foreign tags freshly ingested during Scopes() must be published to consolidated.db
// on the same pass, without requiring a second fold or subsequent Scopes() run.
func TestScopes_ForeignTagsPublishedToConsolidatedSamePass(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the cache dir and consolidated store
	clone := t.TempDir()

	// Stamp verified clone sentinel so Scopes() considers the clone usable.
	if err := os.MkdirAll(filepath.Join(clone, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clone, ".git", cloneSentinel), nil, 0o644); err != nil {
		t.Fatalf("write clone sentinel: %v", err)
	}

	// Foreign machine box-b manifest.
	const foreignMachine = "box-b"
	const foreignID = "box-b-machine-id"
	if err := writeManifest(filepath.Join(clone, foreignMachine), manifest{
		MachineID: foreignID,
		Name:      foreignMachine,
		UpdatedAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("write box-b manifest: %v", err)
	}

	// Foreign transcript with two messages.
	const sid = "sess-foreign-1234"
	const firstUUID = "11111111-2222-3333-4444-555555555555"
	const secondUUID = "66666666-7777-8888-9999-000000000000"
	projDir := filepath.Join(clone, foreignMachine, "claude", "-remote-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}
	transcript := `{"type":"user","uuid":"` + firstUUID + `","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"foreign question"}}` + "\n" +
		`{"type":"assistant","uuid":"` + secondUUID + `","timestamp":"2026-06-01T10:00:05Z","message":{"role":"assistant","content":"foreign answer"}}` + "\n"
	if err := os.WriteFile(filepath.Join(projDir, sid+".jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	// Foreign tag file for session sid.
	const topicName = "distributed indexing defect"
	writeTag(t, clone, foreignMachine, foreignID, TagFile{
		SessionID:     sid,
		OriginMachine: foreignID,
		Segments: []TagSegment{
			{StartUUID: firstUUID, Topic: topicName, Summary: "verifying same pass publication", TaggedAt: 1710000000},
		},
	})

	a := &Archive{
		cfg:       Config{Name: "box-a"},
		clone:     clone,
		machineID: "box-a-id",
		run: func(ctx context.Context, dir string, args ...string) (string, error) {
			return "", nil
		},
	}

	// Run Scopes() once.
	scopes := a.Scopes(context.Background(), false)
	if len(scopes) == 0 {
		t.Fatalf("Scopes() returned no scopes")
	}

	// Verify consolidated.db immediately has the topic segment from the foreign tag.
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated store: %v", err)
	}
	defer con.Close()

	topics, err := store.TopicsForSession(con, sid)
	if err != nil {
		t.Fatalf("TopicsForSession on consolidated.db: %v", err)
	}
	if len(topics) != 1 {
		t.Fatalf("consolidated.db topics for %s = %d, want 1 (freshly ingested tag missed same-pass consolidation)", sid, len(topics))
	}
	if topics[0].Topic != topicName {
		t.Errorf("consolidated.db topic = %q, want %q", topics[0].Topic, topicName)
	}
	if topics[0].OriginMachine != foreignID {
		t.Errorf("consolidated.db topic OriginMachine = %q, want %q", topics[0].OriginMachine, foreignID)
	}
}


