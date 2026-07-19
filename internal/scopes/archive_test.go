package scopes

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/archive/archivetest"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// TestAll_SplicesArchiveScopes: with an archive configured, All() returns the
// local scopes PLUS the foreign machine's scopes — origin-stamped, labeled with
// the machine name — and never the own-machine dir from the clone.
func TestAll_SplicesArchiveScopes(t *testing.T) {
	archivetest.Setup(t, "")

	scs := All("", false)
	var local, foreign *view.Scope
	for i := range scs {
		sc := &scs[i]
		switch {
		case sc.TDir != "" && strings.HasSuffix(sc.TDir, "-local-proj"):
			local = sc
		case sc.Origin != "":
			foreign = sc
		}
		if strings.Contains(sc.Project, archivetest.LocalName+"/") {
			t.Errorf("own machine dir enumerated from the clone: %q", sc.Project)
		}
	}
	if local == nil {
		t.Error("local project scope missing after the splice")
	}
	if foreign == nil {
		t.Fatalf("foreign archive scope missing; got %+v", scs)
	}
	if foreign.Origin != archivetest.ForeignID {
		t.Errorf("foreign Origin = %q, want the manifest machine id", foreign.Origin)
	}
	if !strings.HasPrefix(foreign.Project, archivetest.ForeignName+"/") {
		t.Errorf("foreign Project = %q, want %s/ prefix", foreign.Project, archivetest.ForeignName)
	}
}

// TestAll_SourceFilterAppliesToArchiveScopes: --source claude keeps foreign
// claude scopes; --source codex drops them (the fixture machine pushed no
// codex tree).
func TestAll_SourceFilterAppliesToArchiveScopes(t *testing.T) {
	archivetest.Setup(t, "")

	foreignCount := func(scs []view.Scope) int {
		n := 0
		for _, sc := range scs {
			if sc.Origin != "" {
				n++
			}
		}
		return n
	}
	if n := foreignCount(All("claude", false)); n != 1 {
		t.Errorf("--source claude foreign scopes = %d, want 1", n)
	}
	if n := foreignCount(All("codex", false)); n != 0 {
		t.Errorf("--source codex foreign scopes = %d, want 0 (no foreign codex tree)", n)
	}
}

// TestResolve_StaleArchiveScope: a Stale replica scope resolves to its db with
// IndexStale — the existing stale-fallback posture picks it up from there.
func TestResolve_StaleArchiveScope(t *testing.T) {
	want := filepath.Join(t.TempDir(), "some.db")
	dbp, status, err := Resolve(view.Scope{DBP: want, Stale: true}, false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dbp != want {
		t.Errorf("dbp = %q, want the scope's DBP", dbp)
	}
	if status != index.IndexStale {
		t.Errorf("status = %v, want IndexStale for a stale replica", status)
	}
}

// TestOrphanScanSkipsArchiveDBs: an archive-namespaced cache db must never be
// picked up by the Claude orphan-db scan — its machine dir in the clone is the
// live source; double-listing it would duplicate every foreign hit.
func TestOrphanScanSkipsArchiveDBs(t *testing.T) {
	archivetest.Setup(t, "")
	_ = All("", false) // the splice creates archive-*.db files in the cache dir

	// Claude() (live + orphan scan, NO archive splice) must not surface them.
	for _, sc := range Claude() {
		if strings.Contains(filepath.Base(sc.DBP), "archive-") {
			t.Errorf("orphan scan surfaced an archive db as a Claude scope: %+v", sc)
		}
	}
}
