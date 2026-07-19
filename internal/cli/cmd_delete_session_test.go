package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/archive/archivetest"
)

// readTombstone returns the tombstone contents under the isolated HOME (root
// is <cfg>/projects, the cache lives at <cfg>/.cache/session-search).
func readTombstone(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "..", ".cache", "session-search", ".deleted"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read tombstone: %v", err)
	}
	return string(b)
}

// TestDeleteCmd_PositionalPrefixYes is the README example, verbatim shape:
// `rawclaw delete --yes <session8>` deletes exactly that session — file gone,
// id tombstoned, same receipts as a filter delete.
func TestDeleteCmd_PositionalPrefixYes(t *testing.T) {
	root := newCfgRoot(t)
	target := writeSession(t, root, "proj-a", "cafe0001-0000-0000-0000-000000000001", 3)
	other := writeSession(t, root, "proj-a", "beef0002-0000-0000-0000-000000000002", 3)

	// Through the ROOT command tree — proving the README invocation verbatim.
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--yes", "cafe0001")
	if err != nil {
		t.Fatalf("delete --yes <session8>: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Deleted 1 session") {
		t.Errorf("want deletion summary; out: %s", out)
	}
	if _, serr := os.Stat(target); !os.IsNotExist(serr) {
		t.Errorf("target session still present (err=%v)", serr)
	}
	if _, serr := os.Stat(other); serr != nil {
		t.Errorf("unrelated session touched: %v", serr)
	}
	if tomb := readTombstone(t, root); !strings.Contains(tomb, "cafe0001-0000-0000-0000-000000000001") {
		t.Errorf("tombstone missing the deleted id; got %q", tomb)
	}
}

// TestDeleteCmd_PositionalFullID: the full session id works the same as the
// 8-char prefix.
func TestDeleteCmd_PositionalFullID(t *testing.T) {
	root := newCfgRoot(t)
	target := writeSession(t, root, "proj-a", "cafe0003-0000-0000-0000-000000000003", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "cafe0003-0000-0000-0000-000000000003")
	if err != nil {
		t.Fatalf("delete full id: %v\nout: %s", err, out)
	}
	if _, serr := os.Stat(target); !os.IsNotExist(serr) {
		t.Errorf("target session still present (err=%v)", serr)
	}
	if !strings.Contains(out, "Deleted 1 session") {
		t.Errorf("want deletion summary; out: %s", out)
	}
}

// TestDeleteCmd_PositionalPromptsAndAborts: without --yes the same y/N prompt
// runs; 'n' aborts with exit 1 and the file survives.
func TestDeleteCmd_PositionalPromptsAndAborts(t *testing.T) {
	root := newCfgRoot(t)
	target := writeSession(t, root, "proj-a", "cafe0004-0000-0000-0000-000000000004", 2)

	out, err := runCmd(t, newDeleteCmd(), "n\n", "cafe0004")
	if err == nil {
		t.Fatalf("aborted positional delete should exit non-zero; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) || ee.Code != 1 {
		t.Fatalf("want ExitError code 1, got %T: %v", err, err)
	}
	if !strings.Contains(out, "[y/N]") || !strings.Contains(out, "Aborted; nothing deleted.") {
		t.Errorf("positional delete should prompt and abort; out: %s", out)
	}
	if _, serr := os.Stat(target); serr != nil {
		t.Errorf("session deleted despite abort: %v", serr)
	}
}

// TestDeleteCmd_PositionalUnknownID: an id matching nothing is a clear error,
// exit 1 — not the filter path's quiet "Nothing to delete." exit 0.
func TestDeleteCmd_PositionalUnknownID(t *testing.T) {
	root := newCfgRoot(t)
	writeSession(t, root, "proj-a", "cafe0005-0000-0000-0000-000000000005", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "dead9999")
	if err == nil {
		t.Fatalf("unknown id should error; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) || ee.Code != 1 {
		t.Fatalf("want ExitError code 1, got %T: %v", err, err)
	}
	if !strings.Contains(ee.Msg, "dead9999") {
		t.Errorf("error should name the unmatched id; got %q", ee.Msg)
	}
}

// TestDeleteCmd_PositionalShortPrefixRefused: a sub-8-char prefix that is not
// an exact session id must not fuzzy-match — refused as unknown rather than
// deleting whatever happens to share four characters.
func TestDeleteCmd_PositionalShortPrefixRefused(t *testing.T) {
	root := newCfgRoot(t)
	target := writeSession(t, root, "proj-a", "cafe0006-0000-0000-0000-000000000006", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "cafe")
	if err == nil {
		t.Fatalf("4-char prefix should not match; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) || ee.Code != 1 {
		t.Fatalf("want ExitError code 1, got %T: %v", err, err)
	}
	if _, serr := os.Stat(target); serr != nil {
		t.Errorf("session deleted on short prefix: %v", serr)
	}
}

// TestDeleteCmd_PositionalAmbiguousRefused: an 8-char prefix matching two
// sessions deletes NEITHER — the command names the collision and exits 1, so
// --yes can never take out a second session by accident.
func TestDeleteCmd_PositionalAmbiguousRefused(t *testing.T) {
	root := newCfgRoot(t)
	one := writeSession(t, root, "proj-a", "cafe0007-0000-0000-0000-000000000007", 2)
	two := writeSession(t, root, "proj-b", "cafe0007-1111-0000-0000-000000000008", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "cafe0007")
	if err == nil {
		t.Fatalf("ambiguous prefix should refuse; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) || ee.Code != 1 {
		t.Fatalf("want ExitError code 1, got %T: %v", err, err)
	}
	if !strings.Contains(ee.Msg, "narrow") {
		t.Errorf("refusal should ask to narrow; got %q", ee.Msg)
	}
	for _, p := range []string{one, two} {
		if _, serr := os.Stat(p); serr != nil {
			t.Errorf("ambiguous refusal must not delete %s: %v", p, serr)
		}
	}
	if tomb := readTombstone(t, root); strings.Contains(tomb, "cafe0007") {
		t.Errorf("ambiguous refusal must not tombstone; got %q", tomb)
	}
}

// TestDeleteCmd_PositionalReachesRetained: a retained session (source file
// already purged) is deletable by id — tombstone-only, same as the filter path.
func TestDeleteCmd_PositionalReachesRetained(t *testing.T) {
	root := newCfgRoot(t)
	retainSession(t, root, "proj-retained", "cafe0008-0000-0000-0000-000000000009", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "cafe0008")
	if err != nil {
		t.Fatalf("positional delete of retained: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "(source file already gone)") {
		t.Errorf("plan should label the retained match; out: %s", out)
	}
	if tomb := readTombstone(t, root); !strings.Contains(tomb, "cafe0008-0000-0000-0000-000000000009") {
		t.Errorf("tombstone missing retained id; got %q", tomb)
	}
}

// TestDeleteCmd_PositionalForeignRefused: a session id that resolves only to
// another machine's archived transcripts is refused with the origin-machine
// pointer — foreign sessions are read-only from every box.
func TestDeleteCmd_PositionalForeignRefused(t *testing.T) {
	fx := archivetest.Setup(t, "")

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--yes", archivetest.ForeignSession[:8])
	if err == nil {
		t.Fatalf("foreign-only session delete should refuse; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) || ee.Code != 1 {
		t.Fatalf("want ExitError code 1, got %T: %v", err, err)
	}
	if !strings.Contains(ee.Msg, "read-only") || !strings.Contains(ee.Msg, archivetest.ForeignName) {
		t.Errorf("refusal should name the origin machine and the read-only rule; got %q", ee.Msg)
	}
	if _, serr := os.Stat(foreignClonePath(fx)); serr != nil {
		t.Errorf("foreign transcript touched by refused delete: %v", serr)
	}
}

// TestDeleteCmd_PositionalLivePlusStaleRetainedRow: a session whose file is
// back on disk while its retained row still carries missing_since (purged,
// then restored, not yet reconciled) is ONE session, not an ambiguous pair —
// the delete proceeds, removing the file and tombstoning the id once.
func TestDeleteCmd_PositionalLivePlusStaleRetainedRow(t *testing.T) {
	root := newCfgRoot(t)
	// Retain it (index → purge → reindex marks missing_since)…
	retainSession(t, root, "proj-a", "cafe0012-0000-0000-0000-000000000013", 2)
	// …then the file comes back, with no reconcile pass in between.
	restored := writeSession(t, root, "proj-a", "cafe0012-0000-0000-0000-000000000013", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "cafe0012")
	if err != nil {
		t.Fatalf("delete of live+stale-retained session: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Deleted 1 session(s) (0 retained)") {
		t.Errorf("want a single live delete, not a double-count; out: %s", out)
	}
	if _, serr := os.Stat(restored); !os.IsNotExist(serr) {
		t.Errorf("restored file still present after delete (err=%v)", serr)
	}
	if tomb := readTombstone(t, root); strings.Count(tomb, "cafe0012-0000-0000-0000-000000000013") != 1 {
		t.Errorf("id should be tombstoned exactly once; got %q", tomb)
	}
}

// TestBrowseThisProjectWinsOverAll: --this-project beats --all on bare browse,
// the same precedence --stats applies.
func TestBrowseThisProjectWinsOverAll(t *testing.T) {
	newCfgRoot(t)

	// A history-less --dir under --this-project must show the no-history hint
	// even with --all also set.
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--all", "--this-project", "--dir", t.TempDir())
	if err != nil {
		t.Fatalf("browse --all --this-project: %v\n%s", err, out)
	}
	if !strings.Contains(out, "No transcript history") {
		t.Errorf("--this-project should win over --all on browse:\n%s", out)
	}
}

// TestDeleteCmd_ProvenanceNoteLive: after a real delete of a live session the
// provenance note states what was removed — the transcript file plus
// rawclaw's copy.
func TestDeleteCmd_ProvenanceNoteLive(t *testing.T) {
	root := newCfgRoot(t)
	writeSession(t, root, "proj-a", "cafe0009-0000-0000-0000-000000000010", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "cafe0009")
	if err != nil {
		t.Fatalf("delete: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Removed the session transcript file(s) and rawclaw's copy (index + archive).") {
		t.Errorf("missing live-delete provenance note; out: %s", out)
	}
}

// TestDeleteCmd_ProvenanceNoteRetained: a retained-only delete removed no
// on-disk transcript — the note says so: only rawclaw's copy died, the source
// tools' files are untouched.
func TestDeleteCmd_ProvenanceNoteRetained(t *testing.T) {
	root := newCfgRoot(t)
	retainSession(t, root, "proj-retained", "cafe0010-0000-0000-0000-000000000011", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "cafe0010")
	if err != nil {
		t.Fatalf("delete retained: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Removed rawclaw's copy (index + archive). Claude Code / Codex transcript files are untouched.") {
		t.Errorf("missing retained-delete provenance note; out: %s", out)
	}
}

// TestDeleteCmd_HelpNamesProvenance: `delete --help` long text carries the
// provenance sentence.
func TestDeleteCmd_HelpNamesProvenance(t *testing.T) {
	newCfgRoot(t)
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--help")
	if err != nil {
		t.Fatalf("delete --help: %v", err)
	}
	if !strings.Contains(out, "rawclaw's copy (index + archive") ||
		!strings.Contains(out, "Claude Code / Codex transcript files are untouched") {
		t.Errorf("delete --help missing the provenance sentence:\n%s", out)
	}
}

// TestDeleteCmd_DryRunPositionalExitZero: --dry-run with a positional session
// stays exit 0 and touches nothing.
func TestDeleteCmd_DryRunPositionalExitZero(t *testing.T) {
	root := newCfgRoot(t)
	target := writeSession(t, root, "proj-a", "cafe0011-0000-0000-0000-000000000012", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--dry-run", "cafe0011")
	if err != nil {
		t.Fatalf("positional --dry-run should exit 0: %v\nout: %s", err, out)
	}
	if _, serr := os.Stat(target); serr != nil {
		t.Errorf("dry-run deleted the session: %v", serr)
	}
}
