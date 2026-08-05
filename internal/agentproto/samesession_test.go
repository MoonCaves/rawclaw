package agentproto

import (
	"errors"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// TestLocateSessionCoalescesSameIDAcrossProjects pins the rule that the same
// session id is ONE session, however many project databases hold a row
// for it.
//
// Real case: a session started in one working directory and continued in
// another. The agent kept the id and wrote a second transcript under the new
// project, so the sweep found two rows. locateSession reported that as an
// ambiguity and told the caller to "give a longer prefix" — impossible advice,
// since the full ids are byte-identical. The session became unreachable to
// read, outline and tag.
func TestLocateSessionCoalescesSameIDAcrossProjects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	oldProj := t.TempDir()
	newProj := t.TempDir()
	const sid = "7c1d40a2-0000-4000-8000-00000000c0de"
	const uuid = "9f3e1c20-aaaa-bbbb-cccc-000000000001"

	// Same session id, two projects — the shape a cross-directory continue makes.
	writeSession(t, oldProj, sid, uuid, "the earlier half of the conversation")
	writeSession(t, newProj, sid, uuid, "the continued half of the conversation")

	oldDB, _, _, err := index.EnsureIndexed(oldProj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(old): %v", err)
	}
	if _, _, _, err := index.EnsureIndexed(newProj, false); err != nil {
		t.Fatalf("EnsureIndexed(new): %v", err)
	}

	// Mark the older project's row as a retained stub: its source file is gone
	// (the worktree was deleted) but durable retention kept the row.
	con := openCacheRW(t, oldDB)
	if _, err := con.Exec("UPDATE sessions SET missing_since=? WHERE id=?", 1.0, sid); err != nil {
		t.Fatalf("mark retained: %v", err)
	}
	con.Close()

	scope := []view.Scope{
		{Project: paths.ProjectLabel(oldProj), TDir: oldProj},
		{Project: paths.ProjectLabel(newProj), TDir: newProj},
	}

	dbp, fullSID, _, err := locateSession(scope, sid[:8])
	if err != nil {
		var amb *ErrAmbiguousSession
		if errors.As(err, &amb) {
			t.Fatalf("one session reported as an ambiguity — the advice to lengthen the prefix cannot help, the ids are identical: %v", err)
		}
		t.Fatalf("locateSession: %v", err)
	}
	if fullSID != sid {
		t.Errorf("full session id = %q, want %q", fullSID, sid)
	}

	// It must resolve to the LIVE row, not the retained stub from the deleted dir.
	c := openCacheRW(t, dbp)
	defer c.Close()
	_, live, ok := store.SessionRowQuality(c, sid)
	if !ok {
		t.Fatalf("resolved db %q has no row for %s", dbp, sid)
	}
	if !live {
		t.Error("resolved to the retained stub; the live transcript should win")
	}
}

// TestLocateSessionStillFlagsDistinctIDCollision guards the other direction: two
// GENUINELY different sessions that merely share an 8-char prefix must still
// raise, because there a longer prefix really can disambiguate.
func TestLocateSessionStillFlagsDistinctIDCollision(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	projA := t.TempDir()
	projB := t.TempDir()
	// Same 8-char prefix, different sessions — Codex ids are UUIDv7, so a shared
	// time-ordered prefix is routine.
	const sidA = "019f0001-1111-7000-8000-0000000000a1"
	const sidB = "019f0001-2222-7000-8000-0000000000b2"

	writeSession(t, projA, sidA, "9f3e1c20-aaaa-bbbb-cccc-00000000000a", "one conversation")
	writeSession(t, projB, sidB, "9f3e1c20-aaaa-bbbb-cccc-00000000000b", "a different conversation")

	scope := []view.Scope{
		{Project: paths.ProjectLabel(projA), TDir: projA},
		{Project: paths.ProjectLabel(projB), TDir: projB},
	}

	if _, _, _, err := locateSession(scope, "019f0001"); err == nil {
		t.Fatal("two distinct sessions sharing a prefix must still raise an ambiguity")
	} else {
		var amb *ErrAmbiguousSession
		if !errors.As(err, &amb) {
			t.Fatalf("want *ErrAmbiguousSession, got %T: %v", err, err)
		}
	}
}

// TestCoalescePrefersLocalOverForeignReplica pins that a replicated (foreign)
// scope never wins the coalesce, even when its row is live and larger. Tag
// export skips foreign databases, so resolving there would send tag-write at a
// replica whose writes never sync back.
func TestCoalescePrefersLocalOverForeignReplica(t *testing.T) {
	local := sessionCand{SessionID: "s", Project: "local", dbp: "/local.db", foreign: false}
	remote := sessionCand{SessionID: "s", Project: "remote", dbp: "/remote.db", foreign: true}

	// Foreign swept FIRST so insertion order cannot be what saves us.
	got := coalesceSameSession([]sessionCand{remote, local})
	if len(got) != 1 {
		t.Fatalf("want 1 coalesced candidate, got %d", len(got))
	}
	if got[0].foreign {
		t.Errorf("resolved to the foreign replica %q; the local scope must win", got[0].Project)
	}
}
