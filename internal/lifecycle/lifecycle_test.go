package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// writeSession writes a .jsonl with `lines` JSON-ish lines under
// projectsRoot/<project>/<id>.jsonl and returns its path.
func writeSession(t *testing.T, projectsRoot, project, id string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(projectsRoot, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	p := filepath.Join(dir, id+".jsonl")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %q: %v", p, err)
	}
	return p
}

func ids(items []PlanItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.SessionID
	}
	sort.Strings(out)
	return out
}

func TestArchive_MovesFile(t *testing.T) {
	projects := t.TempDir()
	archive := t.TempDir()
	src := writeSession(t, projects, "proj-a", "sess-1", `{"cwd":"/x"}`, `{"role":"user"}`)

	newPath, err := Archive(src, archive)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	want := filepath.Join(archive, "sess-1.jsonl")
	if newPath != want {
		t.Fatalf("newPath = %q, want %q", newPath, want)
	}
	if fileExists(src) {
		t.Fatalf("source %q still present after archive", src)
	}
	if !fileExists(want) {
		t.Fatalf("archived file %q missing", want)
	}
}

func TestArchive_CreatesArchiveDir(t *testing.T) {
	projects := t.TempDir()
	archive := filepath.Join(t.TempDir(), "nested", "archive") // does not exist yet
	src := writeSession(t, projects, "proj-a", "sess-2", `{"role":"user"}`)

	newPath, err := Archive(src, archive)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if !fileExists(newPath) {
		t.Fatalf("archived file %q missing", newPath)
	}
}

func TestArchive_Idempotent(t *testing.T) {
	projects := t.TempDir()
	archive := t.TempDir()
	src := writeSession(t, projects, "proj-a", "sess-3", `{"role":"user"}`)

	first, err := Archive(src, archive)
	if err != nil {
		t.Fatalf("first Archive: %v", err)
	}

	// Second call: source is gone but the id already sits in the archive.
	second, err := Archive("sess-3", archive)
	if err != nil {
		t.Fatalf("second Archive (idempotent): %v", err)
	}
	if first != second {
		t.Fatalf("idempotent Archive returned %q then %q", first, second)
	}

	// And re-archiving the destination path itself is a no-op success.
	third, err := Archive(first, archive)
	if err != nil {
		t.Fatalf("re-archive dest: %v", err)
	}
	if third != first {
		t.Fatalf("re-archive dest returned %q, want %q", third, first)
	}
}

func TestArchive_MissingSessionErrors(t *testing.T) {
	archive := t.TempDir()
	_, err := Archive("does-not-exist", archive)
	if err == nil {
		t.Fatal("expected error archiving a missing session, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist in chain, got %v", err)
	}
}

func TestDelete_NoFilterErrors(t *testing.T) {
	projects := t.TempDir()
	cache := t.TempDir()
	writeSession(t, projects, "proj-a", "sess-1", `{"role":"user"}`)

	tests := []struct {
		name string
		opts DeleteOpts
	}{
		{name: "empty opts", opts: DeleteOpts{}},
		{name: "only dry-run set", opts: DeleteOpts{DryRun: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Delete(projects, cache, tt.opts)
			if !errors.Is(err, ErrNoFilter) {
				t.Fatalf("Delete(%v) error = %v, want ErrNoFilter", tt.opts, err)
			}
			// The gate must fire BEFORE any deletion.
			if !fileExists(filepath.Join(projects, "proj-a", "sess-1.jsonl")) {
				t.Fatal("session deleted despite ErrNoFilter gate")
			}
		})
	}
}

func TestDelete_DryRunDeletesNothingButReportsBytes(t *testing.T) {
	projects := t.TempDir()
	cache := t.TempDir()
	p1 := writeSession(t, projects, "proj-a", "short", `{"a":1}`)                // 1 msg
	p2 := writeSession(t, projects, "proj-a", "alsoShort", `{"a":1}`, `{"b":2}`) // 2 msgs

	info1, _ := os.Stat(p1)
	info2, _ := os.Stat(p2)
	wantBytes := info1.Size() + info2.Size()

	plan, err := Delete(projects, cache, DeleteOpts{MaxMessages: 5, DryRun: true})
	if err != nil {
		t.Fatalf("Delete dry-run: %v", err)
	}
	if plan.Deleted {
		t.Fatal("dry-run plan.Deleted = true, want false")
	}
	if len(plan.Matched) != 2 {
		t.Fatalf("matched %d sessions, want 2", len(plan.Matched))
	}
	if plan.TotalBytes != wantBytes {
		t.Fatalf("TotalBytes = %d, want %d", plan.TotalBytes, wantBytes)
	}
	// Nothing touched on disk.
	if !fileExists(p1) || !fileExists(p2) {
		t.Fatal("dry-run removed files")
	}
	if fileExists(plan.TombstonePath) {
		t.Fatal("dry-run wrote a tombstone")
	}
}

func TestDelete_RealDeleteRemovesFileAndWritesTombstone(t *testing.T) {
	projects := t.TempDir()
	cache := t.TempDir()
	pDel := writeSession(t, projects, "proj-a", "victim", `{"a":1}`) // 1 msg -> matches
	pKeep := writeSession(t, projects, "proj-a", "survivor",
		`{"a":1}`, `{"b":2}`, `{"c":3}`, `{"d":4}`) // 4 msgs -> excluded by MaxMessages=1

	plan, err := Delete(projects, cache, DeleteOpts{MaxMessages: 1})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !plan.Deleted {
		t.Fatal("plan.Deleted = false, want true")
	}
	if got := ids(plan.Matched); len(got) != 1 || got[0] != "victim" {
		t.Fatalf("matched ids = %v, want [victim]", got)
	}
	if fileExists(pDel) {
		t.Fatalf("victim %q not deleted", pDel)
	}
	if !fileExists(pKeep) {
		t.Fatalf("survivor %q wrongly deleted", pKeep)
	}

	// Tombstone written and consultable.
	set, err := LoadTombstones(cache)
	if err != nil {
		t.Fatalf("LoadTombstones: %v", err)
	}
	if _, ok := set["victim"]; !ok {
		t.Fatalf("victim id missing from tombstone set %v", set)
	}
	tomb, err := IsTombstoned(cache, "victim")
	if err != nil {
		t.Fatalf("IsTombstoned: %v", err)
	}
	if !tomb {
		t.Fatal("IsTombstoned(victim) = false, want true")
	}
	if tomb, _ := IsTombstoned(cache, "survivor"); tomb {
		t.Fatal("IsTombstoned(survivor) = true, want false")
	}
}

func TestDelete_FiltersAreANDed(t *testing.T) {
	projects := t.TempDir()
	cache := t.TempDir()
	// In proj-keep: short session that also matches the project substring.
	writeSession(t, projects, "alpha-keep", "hit", `{"a":1}`)
	// In proj-other: short session but WRONG project substring.
	writeSession(t, projects, "beta-other", "miss-project", `{"a":1}`)
	// In proj-keep: right project but too many messages.
	writeSession(t, projects, "alpha-keep", "miss-size",
		`{"a":1}`, `{"b":2}`, `{"c":3}`)

	plan, err := Delete(projects, cache, DeleteOpts{Project: "alpha-keep", MaxMessages: 1, DryRun: true})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got := ids(plan.Matched)
	if len(got) != 1 || got[0] != "hit" {
		t.Fatalf("AND-filtered matches = %v, want [hit]", got)
	}
}

func TestDelete_BeforeFilter(t *testing.T) {
	projects := t.TempDir()
	cache := t.TempDir()
	old := writeSession(t, projects, "proj-a", "old", `{"a":1}`)
	recent := writeSession(t, projects, "proj-a", "recent", `{"a":1}`)

	// Make "old" clearly older than the cutoff, "recent" clearly newer.
	cutoff := time.Now().Add(-24 * time.Hour)
	older := cutoff.Add(-48 * time.Hour)
	newer := cutoff.Add(48 * time.Hour)
	if err := os.Chtimes(old, older, older); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recent, newer, newer); err != nil {
		t.Fatal(err)
	}

	plan, err := Delete(projects, cache, DeleteOpts{Before: cutoff, DryRun: true})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got := ids(plan.Matched)
	if len(got) != 1 || got[0] != "old" {
		t.Fatalf("Before-filtered matches = %v, want [old]", got)
	}
}

func TestTombstone_RoundTrips(t *testing.T) {
	cache := t.TempDir()

	// Empty / missing tombstone -> empty set, no error.
	set, err := LoadTombstones(cache)
	if err != nil {
		t.Fatalf("LoadTombstones (missing): %v", err)
	}
	if len(set) != 0 {
		t.Fatalf("missing tombstone gave %d ids, want 0", len(set))
	}

	// Append twice across two calls; blanks ignored.
	if err := appendTombstones(TombstonePath(cache), []string{"id-a", "id-b"}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := appendTombstones(TombstonePath(cache), []string{"  ", "id-c"}); err != nil {
		t.Fatalf("append 2: %v", err)
	}

	set, err = LoadTombstones(cache)
	if err != nil {
		t.Fatalf("LoadTombstones: %v", err)
	}
	for _, id := range []string{"id-a", "id-b", "id-c"} {
		if _, ok := set[id]; !ok {
			t.Fatalf("id %q missing from round-tripped set %v", id, set)
		}
	}
	if _, ok := set[""]; ok {
		t.Fatal("blank line leaked into tombstone set")
	}
	if len(set) != 3 {
		t.Fatalf("set has %d ids, want 3 (%v)", len(set), set)
	}
}

func TestTombstonePath_Default(t *testing.T) {
	got := TombstonePath("")
	if !strings.HasSuffix(got, filepath.Join(".cache", "session-search", ".deleted")) {
		t.Fatalf("default TombstonePath = %q, want suffix .cache/session-search/.deleted", got)
	}
}

func TestLoadTombstones_AlwaysNonNil(t *testing.T) {
	// Even on a fresh cache with no file, the set must be safe to read/range.
	set, err := LoadTombstones(t.TempDir())
	if err != nil {
		t.Fatalf("LoadTombstones: %v", err)
	}
	if set == nil {
		t.Fatal("LoadTombstones returned a nil map")
	}
	if _, ok := set["anything"]; ok {
		t.Fatal("empty set reported a membership")
	}
}
