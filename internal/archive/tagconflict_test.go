package archive

import (
	"os"
	"testing"
	"time"
)

func TestTagConflictState_RoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate CacheDir off the real ~/.cache

	// Sorted + de-duplicated on write; read back clean.
	writeTagConflicts([]string{"sess-b", "sess-a", "sess-a"})
	got := readTagConflicts()
	if len(got) != 2 || got[0] != "sess-a" || got[1] != "sess-b" {
		t.Fatalf("round-trip = %v, want [sess-a sess-b]", got)
	}

	// An empty set truncates the record so a resolved conflict stops reporting.
	writeTagConflicts(nil)
	if got := readTagConflicts(); len(got) != 0 {
		t.Errorf("after empty write, conflicts = %v, want none", got)
	}
}

func TestTagIngestDue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Never ingested: run once.
	if !tagIngestDue() {
		t.Error("no ingest stamp should be due (run once)")
	}
	stampTagIngest()

	// Ingested, never pulled: nothing foreign changed → skip. (This is the case a
	// tags-dir-mtime gate got right too; the next case is the one it missed.)
	if tagIngestDue() {
		t.Error("ingested with no pull should not be due")
	}

	// Pull OLDER than the last ingest: skip.
	stampPull()
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(pullStampPath(), past, past); err != nil {
		t.Fatal(err)
	}
	if tagIngestDue() {
		t.Error("pull older than ingest should not be due")
	}

	// Pull NEWER than the last ingest (a fresh pull may have rewritten a foreign
	// tag file IN PLACE — no dir mtime bump, which a naive dir-mtime gate would
	// miss): due.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(pullStampPath(), future, future); err != nil {
		t.Fatal(err)
	}
	if !tagIngestDue() {
		t.Error("pull newer than ingest should be due (in-place foreign tag update)")
	}
}
