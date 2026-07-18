package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRetainedMatches_FindsRetainedSession is the base case: a session whose
// backing file is purged (missing_since set) is found by RetainedMatches even
// though lifecycle.Delete's live walk can never see it.
func TestRetainedMatches_FindsRetainedSession(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2 (after purge): %v", err)
	}

	got, err := RetainedMatches(filepath.Dir(dbp), "", time.Time{}, 0)
	if err != nil {
		t.Fatalf("RetainedMatches: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d retained matches, want 1: %+v", len(got), got)
	}
	if got[0].SessionID != "s" {
		t.Errorf("SessionID = %q, want %q", got[0].SessionID, "s")
	}
	if got[0].Label != filepath.Base(proj) {
		t.Errorf("Label = %q, want %q", got[0].Label, filepath.Base(proj))
	}
	if got[0].MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", got[0].MessageCount)
	}
}

// TestRetainedMatches_ExcludesLiveSession guards the boundary with the live
// delete path: a still-present session (missing_since NULL) must NOT show up
// as a retained match, or a plain `delete --project x` would double-count it
// once from the live walk and once from here.
func TestRetainedMatches_ExcludesLiveSession(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "live.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"still here"}}`)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex: %v", err)
	}

	got, err := RetainedMatches(filepath.Dir(dbp), "", time.Time{}, 0)
	if err != nil {
		t.Fatalf("RetainedMatches: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d retained matches for a still-live session, want 0: %+v", len(got), got)
	}
}

// TestRetainedMatches_ProjectFilter checks the "path contains" semantic on
// both source_path (the common case) and the db filename (the fallback for a
// row whose source_path was never backfilled).
func TestRetainedMatches_ProjectFilter(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "my-special-project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}

	cacheDir := filepath.Dir(dbp)

	if got, err := RetainedMatches(cacheDir, "my-special-project", time.Time{}, 0); err != nil {
		t.Fatalf("RetainedMatches (matching filter): %v", err)
	} else if len(got) != 1 {
		t.Errorf("project filter %q: got %d matches, want 1", "my-special-project", len(got))
	}

	if got, err := RetainedMatches(cacheDir, "no-such-project", time.Time{}, 0); err != nil {
		t.Fatalf("RetainedMatches (non-matching filter): %v", err)
	} else if len(got) != 0 {
		t.Errorf("project filter %q: got %d matches, want 0", "no-such-project", len(got))
	}

	// db-filename fallback: blank the recorded source_path (simulates a row
	// whose source_path predates migrateDurabilityColumns' backfill) and
	// confirm the substring match falls back to the db's own filename.
	if _, err := con.Exec(`UPDATE sessions SET source_path='' WHERE id='s'`); err != nil {
		t.Fatal(err)
	}
	base := filepath.Base(dbp) // e.g. "test.db"
	needle := base[:len(base)-len(filepath.Ext(base))]
	if got, err := RetainedMatches(cacheDir, needle, time.Time{}, 0); err != nil {
		t.Fatalf("RetainedMatches (db-name fallback): %v", err)
	} else if len(got) != 1 {
		t.Errorf("db-filename fallback filter %q: got %d matches, want 1", needle, len(got))
	}
}

// TestRetainedMatches_BeforeFilter checks the last_ts cutoff: a session whose
// last message is at/after the cutoff is excluded, matching lifecycle's
// strictly-before semantic for --before.
func TestRetainedMatches_BeforeFilter(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}

	cacheDir := filepath.Dir(dbp)

	// Cutoff before the session's last_ts: excluded.
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got, err := RetainedMatches(cacheDir, "", early, 0); err != nil {
		t.Fatalf("RetainedMatches (early cutoff): %v", err)
	} else if len(got) != 0 {
		t.Errorf("cutoff before last_ts: got %d matches, want 0", len(got))
	}

	// Cutoff after the session's last_ts: included.
	late := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	if got, err := RetainedMatches(cacheDir, "", late, 0); err != nil {
		t.Fatalf("RetainedMatches (late cutoff): %v", err)
	} else if len(got) != 1 {
		t.Errorf("cutoff after last_ts: got %d matches, want 1", len(got))
	}
}

// TestRetainedMatches_MaxMessagesFilter checks the "at most N messages" gate.
func TestRetainedMatches_MaxMessagesFilter(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f,
		`{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"one"}}`,
		`{"type":"user","timestamp":"2026-06-01T10:01:00Z","message":{"role":"user","content":"two"}}`,
		`{"type":"user","timestamp":"2026-06-01T10:02:00Z","message":{"role":"user","content":"three"}}`,
	)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}

	cacheDir := filepath.Dir(dbp)

	if got, err := RetainedMatches(cacheDir, "", time.Time{}, 2); err != nil {
		t.Fatalf("RetainedMatches (max 2): %v", err)
	} else if len(got) != 0 {
		t.Errorf("max-messages 2 on a 3-message session: got %d matches, want 0", len(got))
	}

	if got, err := RetainedMatches(cacheDir, "", time.Time{}, 3); err != nil {
		t.Fatalf("RetainedMatches (max 3): %v", err)
	} else if len(got) != 1 {
		t.Errorf("max-messages 3 on a 3-message session: got %d matches, want 1", len(got))
	}
}

// TestRetainedMatches_ExcludesSubagents guards against a delete plan
// vacuuming subagent transcript rows through the retained path — those are
// not independently addressable top-level sessions, same rule the live walk
// already applies by only globbing top-level *.jsonl.
func TestRetainedMatches_ExcludesSubagents(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}
	// Simulate a subagent row that also went missing — direct UPDATE since
	// normal indexing does not easily produce this state in a unit test.
	if _, err := con.Exec(`UPDATE sessions SET is_subagent=1 WHERE id='s'`); err != nil {
		t.Fatal(err)
	}

	got, err := RetainedMatches(filepath.Dir(dbp), "", time.Time{}, 0)
	if err != nil {
		t.Fatalf("RetainedMatches: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d retained matches for a subagent row, want 0: %+v", len(got), got)
	}
}

// TestRetainedMatches_SkipsUnreadableDB guards the best-effort tolerance: a
// corrupt/non-db file sitting in the cache dir must not fail the whole scan —
// the stated requirement ("skip unreadable DBs gracefully, like scopes
// discovery does").
func TestRetainedMatches_SkipsUnreadableDB(t *testing.T) {
	proj := t.TempDir()
	f := filepath.Join(proj, "s.jsonl")
	writeJSONL(t, f, `{"type":"user","timestamp":"2026-06-01T10:00:00Z","message":{"role":"user","content":"hi"}}`)

	con, dbp := openTestDB(t)
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 1: %v", err)
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if err := UpdateIndex(con, proj); err != nil {
		t.Fatalf("UpdateIndex pass 2: %v", err)
	}

	cacheDir := filepath.Dir(dbp)
	// A garbage .db file alongside the real one — glob picks it up, open must fail
	// gracefully rather than aborting the scan.
	junk := filepath.Join(cacheDir, "corrupt.db")
	if err := os.WriteFile(junk, []byte("not a sqlite file"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := RetainedMatches(cacheDir, "", time.Time{}, 0)
	if err != nil {
		t.Fatalf("RetainedMatches should tolerate an unreadable db, got error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d retained matches with one corrupt db present, want 1 (from the good db): %+v", len(got), got)
	}
}
