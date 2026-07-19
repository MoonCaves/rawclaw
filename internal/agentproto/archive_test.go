package agentproto

import (
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/archive/archivetest"
)

// foreignRef is the read-ref of the fixture's foreign message.
func foreignRef() string {
	return archivetest.ForeignSession[:8] + ":" + archivetest.ForeignUUID[:8]
}

// TestSearch_FindsForeignSession: with a second machine's dir in the archive,
// a plain search returns its session like any local hit — labeled with the
// machine name — and the envelope is complete (nothing stale, nothing skipped).
func TestSearch_FindsForeignSession(t *testing.T) {
	archivetest.Setup(t, "")

	env := Search(archivetest.ForeignBeacon, nil, SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("Search(%q) = %d results, want 1: %+v", archivetest.ForeignBeacon, len(env.Results), env.Results)
	}
	r := env.Results[0]
	if r.SessionID != archivetest.ForeignSession {
		t.Errorf("SessionID = %q, want %q", r.SessionID, archivetest.ForeignSession)
	}
	if !strings.HasPrefix(r.Project, archivetest.ForeignName+"/") {
		t.Errorf("Project = %q, want %s/ prefix", r.Project, archivetest.ForeignName)
	}
	if r.ReadRef != foreignRef() {
		t.Errorf("ReadRef = %q, want %q", r.ReadRef, foreignRef())
	}
	if !env.Complete {
		t.Errorf("envelope incomplete for a fresh archive: %+v", env.Scopes)
	}
}

// TestSearch_NoDuplicateLocalHits: the local session lives in the live tree
// AND in the clone's own-machine dir — excluding the own dir from enumeration
// must keep it a single hit.
func TestSearch_NoDuplicateLocalHits(t *testing.T) {
	archivetest.Setup(t, "")

	env := Search(archivetest.LocalBeacon, nil, SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("Search(%q) = %d results, want exactly 1 (no clone duplicate): %+v",
			archivetest.LocalBeacon, len(env.Results), env.Results)
	}
	if strings.Contains(env.Results[0].Project, archivetest.LocalName+"/") {
		t.Errorf("local hit surfaced under the clone label %q", env.Results[0].Project)
	}
}

// TestRead_ForeignSessionRenders: a foreign ref reads like any local ref.
func TestRead_ForeignSessionRenders(t *testing.T) {
	archivetest.Setup(t, "")

	res, err := Read(foreignRef(), nil, ReadOpts{})
	if err != nil {
		t.Fatalf("Read(%s): %v", foreignRef(), err)
	}
	if res.SessionID != archivetest.ForeignSession {
		t.Errorf("SessionID = %q, want %q", res.SessionID, archivetest.ForeignSession)
	}
	found := false
	for _, m := range res.Window {
		if strings.Contains(m.Text, archivetest.ForeignBeacon) {
			found = true
		}
	}
	if !found {
		t.Errorf("foreign message text missing from the window: %+v", res.Window)
	}
}

// TestOutline_ForeignSessionRenders: a foreign session outlines like any local
// session.
func TestOutline_ForeignSessionRenders(t *testing.T) {
	archivetest.Setup(t, "")

	res, err := Outline(archivetest.ForeignSession[:8], nil, false)
	if err != nil {
		t.Fatalf("Outline: %v", err)
	}
	if res.SessionID != archivetest.ForeignSession {
		t.Errorf("SessionID = %q, want %q", res.SessionID, archivetest.ForeignSession)
	}
	if res.MessageCount != 1 || len(res.Start) != 1 {
		t.Errorf("outline shape = count %d / start %d, want 1/1", res.MessageCount, len(res.Start))
	}
}

// TestSearch_StaleForeignDirReportedAndServed: a foreign dir whose last push
// is ancient (the machine is off/asleep) is REPORTED through the existing
// stale-fallback posture — and its results are still served, never dropped.
func TestSearch_StaleForeignDirReportedAndServed(t *testing.T) {
	archivetest.Setup(t, "2020-01-01T00:00:00Z")

	env := Search(archivetest.ForeignBeacon, nil, SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("stale dir's results not served: %d results", len(env.Results))
	}
	if env.Complete {
		t.Error("envelope claims complete despite a stale foreign dir")
	}
	staleReported := false
	for _, s := range env.Scopes {
		if strings.HasPrefix(s.Project, archivetest.ForeignName+"/") && s.Status == ScopeStaleFallback {
			staleReported = true
		}
	}
	if !staleReported {
		t.Errorf("stale foreign dir not reported via %s: %+v", ScopeStaleFallback, env.Scopes)
	}
}
