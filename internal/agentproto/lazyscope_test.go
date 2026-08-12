package agentproto

import (
	"os"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// TestLocateSessionSkipsScopeListWhenOneStoreAnswers pins the reason read,
// outline and the tag verbs take their all-projects list as a function instead
// of a value.
//
// Real case: a single `read` opened every per-project index on the machine —
// 126 database files — before asking the one store anything. Warm that was
// seconds of work for nothing; cold, right after a schema change, each of
// those files also ran its migration, which took about two minutes against a
// verb the watchdog stops in thirty seconds, so the verb was killed and read
// simply did not work. The one store holds every project as a column and can
// resolve an id with no list at all, so the list must not be built unless the
// store comes up empty.
func TestLocateSessionSkipsScopeListWhenOneStoreAnswers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	const sid = "3ae91b70-0000-4000-8000-0000000000ab"
	writeSession(t, proj, sid, "9f3e1c20-aaaa-bbbb-cccc-0000000000ab", "a session the one store knows about")

	db, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{db}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	called := false
	more := func() []view.Scope {
		called = true
		return []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	}

	// Nil scope = every project. The store narrows by nothing and answers.
	_, fullSID, _, err := locateSession(nil, more, sid[:8])
	if err != nil {
		t.Fatalf("locateSession: %v", err)
	}
	if fullSID != sid {
		t.Errorf("full session id = %q, want %q", fullSID, sid)
	}
	if called {
		t.Error("the per-project list was built even though the one store answered the id — that is the enumeration this seam exists to avoid")
	}
}

// TestLocateSessionBuildsScopeListWhenOneStoreCannotAnswer is the other half:
// deferring the list must not turn into never building it. A machine that has
// not consolidated still resolves its sessions by sweeping the per-project
// indexes.
func TestLocateSessionBuildsScopeListWhenOneStoreCannotAnswer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	const sid = "6dc4f281-0000-4000-8000-0000000000cd"
	writeSession(t, proj, sid, "9f3e1c20-aaaa-bbbb-cccc-0000000000cd", "a session only the per-project index knows")

	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	// Remove whatever a write-through left, so the one store genuinely cannot answer.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(index.ConsolidatedPath() + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove consolidated store: %v", err)
		}
	}

	called := false
	more := func() []view.Scope {
		called = true
		return []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	}

	_, fullSID, _, err := locateSession(nil, more, sid[:8])
	if err != nil {
		t.Fatalf("a session in a per-project index must still resolve without the one store: %v", err)
	}
	if fullSID != sid {
		t.Errorf("full session id = %q, want %q", fullSID, sid)
	}
	if !called {
		t.Error("the one store could not answer and the per-project list was never built — the session would be reported missing")
	}
}

// TestLocateSessionEmptyScopeResolvesNothing pins the difference between a nil
// scope and an empty one. Nil means nobody narrowed anything, so the lookup may
// go wide. EMPTY means a caller narrowed deliberately — --this-project in a
// directory with no transcript history — and it must resolve nothing rather
// than silently searching every project and returning a stranger's session.
func TestLocateSessionEmptyScopeResolvesNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	const sid = "81f0c3d9-0000-4000-8000-0000000000ef"
	writeSession(t, proj, sid, "9f3e1c20-aaaa-bbbb-cccc-0000000000ef", "a session in some other project")

	db, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{db}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	called := false
	more := func() []view.Scope {
		called = true
		return []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	}

	if _, _, _, err := locateSession([]view.Scope{}, more, sid[:8]); err == nil {
		t.Error("an empty scope resolved a session — --this-project in a directory with no history must report not-found, not go wide")
	}
	if called {
		t.Error("an empty scope built the all-projects list, which is the widening it exists to prevent")
	}
}
