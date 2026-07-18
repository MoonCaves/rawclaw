package store_test

import (
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// localDate mirrors SQLite's date(ts,'unixepoch','localtime') so the
// BrowseSessions date-bound assertions hold in any test timezone.
func localDate(ts float64) string {
	return time.Unix(int64(ts), 0).Local().Format("2006-01-02")
}

func TestBrowseSessions(t *testing.T) {
	con, _ := storetest.NewDB(t)
	// Three top-level sessions on distinct days + one subagent thread.
	day := 24 * 3600.0
	base := 1750000000.0 // fixed epoch; date math is relative
	storetest.InsertSession(t, con, storetest.Session{ID: "old", LastTS: base, MessageCount: 2})
	storetest.InsertSession(t, con, storetest.Session{ID: "mid", LastTS: base + day, MessageCount: 3})
	storetest.InsertSession(t, con, storetest.Session{ID: "new", LastTS: base + 2*day, MessageCount: 4})
	storetest.InsertSession(t, con, storetest.Session{ID: "new/agent-x", LastTS: base + 3*day, MessageCount: 1, IsSubagent: true, ParentID: "new"})

	rows, err := store.BrowseSessions(con, "", "", 10)
	if err != nil {
		t.Fatalf("BrowseSessions: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("BrowseSessions = %d rows, want 3 (subagent excluded)", len(rows))
	}
	// Newest first by last_ts.
	if rows[0].SessionID != "new" || rows[1].SessionID != "mid" || rows[2].SessionID != "old" {
		t.Errorf("order = %s,%s,%s, want new,mid,old", rows[0].SessionID, rows[1].SessionID, rows[2].SessionID)
	}
	if rows[0].MessageCount != 4 || rows[0].LastTS != base+2*day {
		t.Errorf("row fields = %+v, want MessageCount 4 / LastTS %v", rows[0], base+2*day)
	}

	// limit caps the result.
	rows, err = store.BrowseSessions(con, "", "", 1)
	if err != nil || len(rows) != 1 || rows[0].SessionID != "new" {
		t.Errorf("BrowseSessions limit 1 = %v (%v), want [new]", rows, err)
	}

	// since: inclusive local-date lower bound drops "old".
	rows, err = store.BrowseSessions(con, localDate(base+day), "", 10)
	if err != nil || len(rows) != 2 {
		t.Errorf("BrowseSessions since = %d rows (%v), want 2", len(rows), err)
	}

	// before: inclusive local-date upper bound drops "new".
	rows, err = store.BrowseSessions(con, "", localDate(base+day), 10)
	if err != nil || len(rows) != 2 {
		t.Errorf("BrowseSessions before = %d rows (%v), want 2", len(rows), err)
	}

	// since+before combined pins the middle day.
	rows, err = store.BrowseSessions(con, localDate(base+day), localDate(base+day), 10)
	if err != nil || len(rows) != 1 || rows[0].SessionID != "mid" {
		t.Errorf("BrowseSessions since+before = %v (%v), want [mid]", rows, err)
	}

	// No match → empty, no error.
	rows, err = store.BrowseSessions(con, "2099-01-01", "", 10)
	if err != nil || len(rows) != 0 {
		t.Errorf("BrowseSessions empty = %v (%v), want none", rows, err)
	}
}

func TestSessionsByPrefix(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "run-a"})
	storetest.InsertSession(t, con, storetest.Session{ID: "run-b"})
	storetest.InsertSession(t, con, storetest.Session{ID: "run-b/agent-x", IsSubagent: true, ParentID: "run-b"})
	storetest.InsertSession(t, con, storetest.Session{ID: "other"})

	// Top-level only, ambiguity limit 2: both run-* sessions, id order.
	ids, err := store.SessionsByPrefix(con, "run", false, 2)
	if err != nil {
		t.Fatalf("SessionsByPrefix: %v", err)
	}
	if len(ids) != 2 || ids[0] != "run-a" || ids[1] != "run-b" {
		t.Errorf("SessionsByPrefix top-level = %v, want [run-a run-b]", ids)
	}

	// Including subagents with limit 3 surfaces the agent thread too.
	ids, err = store.SessionsByPrefix(con, "run", true, 3)
	if err != nil || len(ids) != 3 || ids[2] != "run-b/agent-x" {
		t.Errorf("SessionsByPrefix incl-sub = %v (%v), want 3 with agent last", ids, err)
	}

	// The limit is the ambiguity ceiling: 3 matches, limit 2 → exactly 2 rows.
	ids, err = store.SessionsByPrefix(con, "run", true, 2)
	if err != nil || len(ids) != 2 {
		t.Errorf("SessionsByPrefix limit 2 = %v (%v), want 2 rows", ids, err)
	}

	// Unique prefix resolves to one; no match resolves to none.
	if ids, _ := store.SessionsByPrefix(con, "oth", false, 2); len(ids) != 1 || ids[0] != "other" {
		t.Errorf("SessionsByPrefix unique = %v, want [other]", ids)
	}
	if ids, _ := store.SessionsByPrefix(con, "zzz", false, 2); len(ids) != 0 {
		t.Errorf("SessionsByPrefix miss = %v, want none", ids)
	}
}

func TestSessionMeta(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "s1", LastTS: 1234.5, MessageCount: 7})
	storetest.InsertSession(t, con, storetest.Session{ID: "s2"}) // zero last_ts

	lastTS, n, ok := store.SessionMeta(con, "s1")
	if !ok || lastTS != 1234.5 || n != 7 {
		t.Errorf("SessionMeta(s1) = (%v,%d,%v), want (1234.5,7,true)", lastTS, n, ok)
	}
	if lastTS, n, ok := store.SessionMeta(con, "s2"); !ok || lastTS != 0 || n != 0 {
		t.Errorf("SessionMeta(s2) = (%v,%d,%v), want (0,0,true)", lastTS, n, ok)
	}
	if _, _, ok := store.SessionMeta(con, "missing"); ok {
		t.Error("SessionMeta(missing) ok = true, want false")
	}
}

func TestParentOf(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "root"})
	storetest.InsertSession(t, con, storetest.Session{ID: "child", ParentID: "root"})

	if got := store.ParentOf(con, "child"); got != "root" {
		t.Errorf("ParentOf(child) = %q, want root", got)
	}
	// NULL parent and missing session both read as "" (root reached).
	if got := store.ParentOf(con, "root"); got != "" {
		t.Errorf("ParentOf(root) = %q, want empty", got)
	}
	if got := store.ParentOf(con, "nope"); got != "" {
		t.Errorf("ParentOf(nope) = %q, want empty", got)
	}
}

func TestCountsAndCorpusStats(t *testing.T) {
	con, dbp := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "top"})
	storetest.InsertSession(t, con, storetest.Session{ID: "top/agent-a", IsSubagent: true, ParentID: "top"})
	storetest.InsertMessage(t, con, storetest.Message{SessionID: "top", Role: "user", Content: "hello there", ISO: "2026-06-01T10:00:00Z", UUID: "u1"})
	storetest.InsertMessage(t, con, storetest.Message{SessionID: "top", Role: "assistant", Content: "hi back", ISO: "2026-06-02T10:00:00Z", UUID: "u2"})

	if got := store.CountSessions(dbp); got != 2 {
		t.Errorf("CountSessions = %d, want 2", got)
	}
	if got := store.CountTopLevelSessions(dbp); got != 1 {
		t.Errorf("CountTopLevelSessions = %d, want 1", got)
	}
	cs, err := store.GetCorpusStats(dbp)
	if err != nil {
		t.Fatalf("GetCorpusStats: %v", err)
	}
	want := store.CorpusStats{Sessions: 1, Subagents: 1, Messages: 2, User: 1, Assistant: 1, First: "2026-06-01", Last: "2026-06-02"}
	if cs != want {
		t.Errorf("GetCorpusStats = %+v, want %+v", cs, want)
	}
}
