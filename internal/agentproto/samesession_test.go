package agentproto

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// writeSessionHalf writes ONE transcript holding several messages, so the two
// halves of a session continued in a second directory can differ in length and
// still overlap — the shape that makes a merged row provable.
func writeSessionHalf(t *testing.T, proj, stem string, uuids []string) {
	t.Helper()
	var b strings.Builder
	for _, uuid := range uuids {
		b.WriteString(`{"type":"user","uuid":"` + uuid + `","timestamp":"2026-06-01T10:00:00Z",` +
			`"message":{"role":"user","content":"half of the conversation"}}` + "\n")
	}
	path := filepath.Join(proj, stem+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestLocateSessionReturnsMergedRow pins that the same session id is ONE
// session, however many project databases hold a row for it — and that the row
// the lookup returns is the MERGED one from the consolidated store, not either
// half picked by a ranking.
//
// Real case: a session started in one working directory and continued in
// another. The agent kept the id and wrote a second transcript under the new
// project, so a per-project sweep found a row in each. That was reported as an
// ambiguity with the advice to "give a longer prefix" — impossible, since the
// full ids are byte-identical — leaving the session unreachable to read,
// outline and tag.
func TestLocateSessionReturnsMergedRow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	firstDir := t.TempDir()
	secondDir := t.TempDir()
	const sid = "7c1d40a2-0000-4000-8000-00000000c0de"
	const shared = "9f3e1c20-aaaa-bbbb-cccc-000000000001"

	// Overlapping, unequal halves: the union is 4 messages, so a row holding 2 or
	// 3 would prove the lookup returned a half rather than the merge.
	writeSessionHalf(t, firstDir, sid, []string{shared, "9f3e1c20-aaaa-bbbb-cccc-000000000004"})
	writeSessionHalf(t, secondDir, sid, []string{
		shared,
		"9f3e1c20-aaaa-bbbb-cccc-000000000002",
		"9f3e1c20-aaaa-bbbb-cccc-000000000003",
	})

	firstDB, _, _, err := index.EnsureIndexed(firstDir, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(first): %v", err)
	}
	secondDB, _, _, err := index.EnsureIndexed(secondDir, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(second): %v", err)
	}

	// The first directory was deleted afterwards: durable retention keeps its row
	// with a missing_since watermark, so only the second half is live.
	con := openCacheRW(t, firstDB)
	if _, err := con.Exec("UPDATE sessions SET missing_since=? WHERE id=?", 1.0, sid); err != nil {
		t.Fatalf("mark retained: %v", err)
	}
	con.Close()

	if _, err := index.ConsolidateFrom([]string{firstDB, secondDB}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	scope := []view.Scope{
		{Project: paths.ProjectLabel(firstDir), TDir: firstDir},
		{Project: paths.ProjectLabel(secondDir), TDir: secondDir},
	}

	dbp, fullSID, proj, err := locateSession(scope, nil, sid[:8])
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
	if dbp != index.ConsolidatedPath() {
		t.Errorf("resolved db = %q, want the consolidated store %q", dbp, index.ConsolidatedPath())
	}
	// Provenance follows the live half, not the retained stub of the deleted dir.
	if want := paths.ProjectLabel(secondDir); proj != want {
		t.Errorf("project = %q, want the live half's %q", proj, want)
	}

	// The row itself is the merge: every message from BOTH halves, counted once.
	c := openCacheRW(t, dbp)
	defer c.Close()
	var msgs int
	if err := c.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sid).Scan(&msgs); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if msgs != 4 {
		t.Errorf("resolved row holds %d messages, want the merged 4 (2 + 3 with one shared)", msgs)
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

	if _, _, _, err := locateSession(scope, nil, "019f0001"); err == nil {
		t.Fatal("two distinct sessions sharing a prefix must still raise an ambiguity")
	} else {
		var amb *ErrAmbiguousSession
		if !errors.As(err, &amb) {
			t.Fatalf("want *ErrAmbiguousSession, got %T: %v", err, err)
		}
	}
}

// TestLocateSessionHonorsProjectNarrowing pins that scope still means
// something once every project lives in one database: a lookup narrowed to one
// project must not reach a session belonging to another. In the one store that
// narrowing is a WHERE clause rather than a choice of which file to open, so it
// is easy to lose by accident and invisible when it is lost — every lookup
// still "works", just against the whole machine.
func TestLocateSessionHonorsProjectNarrowing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mine := t.TempDir()
	theirs := t.TempDir()
	const mySID = "2e5a77c1-0000-4000-8000-0000000000d1"
	const theirSID = "8b41c0e9-0000-4000-8000-0000000000d2"
	writeSession(t, mine, mySID, "9f3e1c20-aaaa-bbbb-cccc-0000000000d1", "a session in my project")
	writeSession(t, theirs, theirSID, "9f3e1c20-aaaa-bbbb-cccc-0000000000d2", "a session in another project")

	myDB, _, _, err := index.EnsureIndexed(mine, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(mine): %v", err)
	}
	theirDB, _, _, err := index.EnsureIndexed(theirs, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(theirs): %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{myDB, theirDB}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	narrow := []view.Scope{{Project: paths.ProjectLabel(mine), TDir: mine}}
	if _, _, _, err := locateSession(narrow, nil, theirSID[:8]); err == nil {
		t.Error("a lookup narrowed to one project resolved another project's session")
	} else {
		var nf *ErrSessionNotFound
		if !errors.As(err, &nf) {
			t.Errorf("want *ErrSessionNotFound, got %T: %v", err, err)
		}
	}
	// The same narrow scope still finds its OWN session, so the filter narrows
	// rather than simply blocking.
	if _, fullSID, _, err := locateSession(narrow, nil, mySID[:8]); err != nil {
		t.Errorf("narrowed lookup lost its own session: %v", err)
	} else if fullSID != mySID {
		t.Errorf("full session id = %q, want %q", fullSID, mySID)
	}
}

// TestLocateSessionFallsBackWhenOneStoreEmpty pins the promise that a machine
// which has never consolidated still resolves its sessions: with no
// consolidated store on disk, the lookup says so and sweeps the per-project
// indexes instead of reporting a session that exists as missing.
func TestLocateSessionFallsBackWhenOneStoreEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	const sid = "5b2c9d10-0000-4000-8000-0000000000fa"
	writeSession(t, proj, sid, "9f3e1c20-aaaa-bbbb-cccc-0000000000fa", "a session only the per-project index knows")

	// Index the project WITHOUT consolidating, then remove anything a write-through
	// may have left, so the one store genuinely cannot answer.
	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(index.ConsolidatedPath() + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove consolidated store: %v", err)
		}
	}

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	dbp, fullSID, _, err := locateSession(scope, nil, sid[:8])
	if err != nil {
		t.Fatalf("a session in a per-project index must still resolve without the one store: %v", err)
	}
	if fullSID != sid {
		t.Errorf("full session id = %q, want %q", fullSID, sid)
	}
	if dbp == index.ConsolidatedPath() {
		t.Errorf("resolved to the consolidated store %q, which does not exist", dbp)
	}
}

// TestOutlineCountsOnlyItsOwnMessagesInBetween pins the outline's middle count
// against the one store's row numbering. In a per-project index a session's
// rows are contiguous, so the gap between the two bookends could be read off
// the row ids by subtraction. In a store holding every project that is false:
// ids run across all of them, and a session continued in a second directory is
// folded in two pieces with whatever else the first source held sitting
// between them. Subtracting there counts strangers — the arc of a 10-message
// session was reported as tens of thousands of messages "in between".
func TestOutlineCountsOnlyItsOwnMessagesInBetween(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	firstDir := t.TempDir()
	secondDir := t.TempDir()
	// The split session sorts BEFORE the bystander, so folding the first source
	// lays down its opening half, then the bystander's rows, and the second
	// source appends the closing half after them.
	const splitSID = "1a0b40a2-0000-4000-8000-00000000ab01"
	const otherSID = "9b0b40a2-0000-4000-8000-00000000ab02"

	uuids := func(prefix string, n int) []string {
		var out []string
		for i := 1; i <= n; i++ {
			out = append(out, prefix+"-aaaa-bbbb-cccc-"+fmt.Sprintf("%012d", i))
		}
		return out
	}
	// Ten messages in the split session: more than the two four-message bookends,
	// so exactly two of them fall in the middle.
	writeSessionHalf(t, firstDir, splitSID, uuids("11110001", 5))
	writeSessionHalf(t, secondDir, splitSID, uuids("11110002", 5))
	writeSessionHalf(t, firstDir, otherSID, uuids("22220001", 20))

	firstDB, _, _, err := index.EnsureIndexed(firstDir, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(first): %v", err)
	}
	secondDB, _, _, err := index.EnsureIndexed(secondDir, false)
	if err != nil {
		t.Fatalf("EnsureIndexed(second): %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{firstDB, secondDB}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	// Fixture precondition: the halves really are split around the bystander's
	// rows. Without that this test would pass on contiguous ids and prove nothing.
	con := openCacheRW(t, index.ConsolidatedPath())
	var lo, hi, own int
	if err := con.QueryRow(
		"SELECT MIN(id), MAX(id), COUNT(*) FROM messages WHERE session_id=?", splitSID,
	).Scan(&lo, &hi, &own); err != nil {
		t.Fatalf("read split session rows: %v", err)
	}
	con.Close()
	if own != 10 {
		t.Fatalf("split session holds %d messages, want 10", own)
	}
	if hi-lo+1 == own {
		t.Fatalf("fixture did not interleave: the session's rows %d..%d are contiguous", lo, hi)
	}

	scope := []view.Scope{
		{Project: paths.ProjectLabel(firstDir), TDir: firstDir},
		{Project: paths.ProjectLabel(secondDir), TDir: secondDir},
	}
	res, err := Outline(splitSID[:8], scope, false)
	if err != nil {
		t.Fatalf("Outline: %v", err)
	}
	// Four messages open the arc and four close it, leaving two unshown.
	if res.MidCount != 2 {
		t.Errorf("MidCount = %d, want 2 — the count must cover this session's own rows, not the id gap", res.MidCount)
	}
	if res.MidCount > own {
		t.Errorf("MidCount = %d exceeds the whole session's %d messages", res.MidCount, own)
	}
}

// TestSweepFallbackKeepsOneRowPerSession pins the degraded path: when the one
// store cannot answer and the same session sits in two indexes, scope order
// decides which row answers — the earlier scope. Scope building already lists
// live project directories before orphans and local scopes before replicas of
// other machines, so the earlier scope is the one a write may safely go to.
func TestSweepFallbackKeepsOneRowPerSession(t *testing.T) {
	local := sessionCand{SessionID: "s", Project: "ledger", dbp: "/ledger.db"}
	replica := sessionCand{SessionID: "s", Project: "billing", dbp: "/billing.db"}

	got := firstRowPerSession([]sessionCand{local, replica})
	if len(got) != 1 {
		t.Fatalf("want 1 candidate for one session, got %d", len(got))
	}
	if got[0].Project != "ledger" {
		t.Errorf("resolved to %q; the earlier scope must win", got[0].Project)
	}
}
