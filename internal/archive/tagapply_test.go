package archive

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// newTopicDB builds a real store db with the v2 topic schema for apply tests.
func newTopicDB(t *testing.T) *sql.DB {
	t.Helper()
	dbp := filepath.Join(t.TempDir(), "scope.db")
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("ConnectRW: %v", err)
	}
	t.Cleanup(func() { con.Close() })
	if err := store.Rebuild(con); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	return con
}

func TestApplyResolvedTags_ConflictWinnerLandsIdempotently(t *testing.T) {
	con := newTopicDB(t)
	sid := "sess-x"
	files := []TagFile{
		{SessionID: sid, OriginMachine: "box-a", Segments: []TagSegment{{StartUUID: "u1", Topic: "thin"}}},
		{SessionID: sid, OriginMachine: "box-z", Segments: []TagSegment{{StartUUID: "u1", Topic: "rich"}, {StartUUID: "u2", Topic: "more"}}},
	}

	conflict, err := applyResolvedTags(con, sid, files)
	if err != nil {
		t.Fatalf("applyResolvedTags: %v", err)
	}
	if !conflict {
		t.Error("two distinct real sets should report a conflict")
	}

	got, _ := store.TopicsForSession(con, sid)
	if len(got) != 2 || got[0].Topic != "rich" || got[0].OriginMachine != "box-z" {
		t.Fatalf("winning set = %+v, want box-z's {rich,more}", got)
	}

	// Idempotent: re-applying the identical files leaves the same rows and does
	// not error (the content-hash guard skips the rewrite).
	if _, err := applyResolvedTags(con, sid, files); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	again, _ := store.TopicsForSession(con, sid)
	if len(again) != 2 || again[0].Topic != "rich" {
		t.Errorf("re-apply changed the rows: %+v", again)
	}
}

func TestApplyResolvedTags_IdenticalContentDifferentOrigin_ConvergesAttribution(t *testing.T) {
	// The apply-layer idempotent guard must rewrite when the winning ORIGIN
	// changes even at identical content, else attribution diverges from a machine
	// that ingested both files at once.
	con := newTopicDB(t)
	sid := "s-attr"

	// First: only box-a's tagging present.
	if _, err := applyResolvedTags(con, sid, []TagFile{
		{SessionID: sid, OriginMachine: "box-a", Segments: []TagSegment{{StartUUID: "u1", Topic: "same"}}},
	}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if got, _ := store.TopicsForSession(con, sid); len(got) != 1 || got[0].OriginMachine != "box-a" {
		t.Fatalf("first ingest attribution = %+v, want box-a", got)
	}

	// Later: box-z's identical-content file arrives; box-z > box-a is the
	// deterministic winner. Content hash is unchanged, so a content-only guard
	// would skip — attribution must still converge to box-z.
	if _, err := applyResolvedTags(con, sid, []TagFile{
		{SessionID: sid, OriginMachine: "box-a", Segments: []TagSegment{{StartUUID: "u1", Topic: "same"}}},
		{SessionID: sid, OriginMachine: "box-z", Segments: []TagSegment{{StartUUID: "u1", Topic: "same"}}},
	}); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if got, _ := store.TopicsForSession(con, sid); len(got) != 1 || got[0].OriginMachine != "box-z" {
		t.Errorf("converged attribution = %+v, want box-z (R1 apply guard must rewrite on origin change)", got)
	}
}

func TestApplyResolvedTags_RealBeatsRoutineAtReadTime(t *testing.T) {
	con := newTopicDB(t)
	sid := "sess-y"

	// Only a routine verdict so far.
	if _, err := applyResolvedTags(con, sid, []TagFile{
		{SessionID: sid, OriginMachine: "box-a", Verdict: &TagVerdict{Verdict: "routine", Source: "floor", TaggedAt: 1}},
	}); err != nil {
		t.Fatalf("apply verdict: %v", err)
	}
	if r, _ := store.IsEffectivelyRoutine(con, sid); !r {
		t.Error("routine verdict with no real segment should be effectively routine")
	}

	// A real tagging from another machine arrives — real beats routine.
	if _, err := applyResolvedTags(con, sid, []TagFile{
		{SessionID: sid, OriginMachine: "box-a", Verdict: &TagVerdict{Verdict: "routine", Source: "floor", TaggedAt: 1}},
		{SessionID: sid, OriginMachine: "box-b", Segments: []TagSegment{{StartUUID: "u1", Topic: "actually real"}}},
	}); err != nil {
		t.Fatalf("apply real: %v", err)
	}
	if r, _ := store.IsEffectivelyRoutine(con, sid); r {
		t.Error("a real segment must demote the routine verdict")
	}
}

func TestGatherTagFiles(t *testing.T) {
	clone := t.TempDir()
	sid := "sess-g"
	// Two machines each hold a tag file for the same session; one is absent.
	tmp := filepath.Join(clone, ".git", "rawclaw-tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, md := range []string{"box-a", "box-b"} {
		dir := filepath.Join(clone, md, tagsDirName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeTagFileAtomic(tmp, dir, TagFile{SessionID: sid, OriginMachine: md, Segments: []TagSegment{{StartUUID: "u1", Topic: md}}}); err != nil {
			t.Fatalf("seed %s: %v", md, err)
		}
	}
	files := gatherTagFiles(clone, []string{"box-a", "box-b", "box-absent"}, sid)
	if len(files) != 2 {
		t.Fatalf("gathered %d files, want 2 (absent machine skipped)", len(files))
	}
}
