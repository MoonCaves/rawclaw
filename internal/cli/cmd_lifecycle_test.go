package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/spf13/cobra"
)

// newCfgRoot creates a fake CLAUDE_CONFIG_DIR with a projects/ subdir and points
// paths.ProjectsRoot() at it for the duration of the test (t.Setenv restores it).
// It returns the projects root path.
func newCfgRoot(t *testing.T) string {
	t.Helper()
	cfg := t.TempDir()
	root := filepath.Join(cfg, "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir projects root: %v", err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	// Isolate HOME too: a real `delete` writes its tombstone to the default
	// ~/.cache (cacheDir="" in runDelete). Without this, `go test` would pollute
	// the contributor's real ~/.cache/session-search/.deleted. Keep it hermetic.
	t.Setenv("HOME", cfg)
	return root
}

// writeSession writes a session .jsonl with nLines JSONL lines under
// <root>/<project>/<id>.jsonl and returns its absolute path.
func writeSession(t *testing.T, root, project, id string, nLines int) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	var b strings.Builder
	for i := 0; i < nLines; i++ {
		b.WriteString(`{"type":"user","text":"hi"}` + "\n")
	}
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return path
}

// runCmd executes cmd with args, capturing stdout+stderr and feeding stdin.
func runCmd(t *testing.T, cmd *cobra.Command, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// TestArchiveCmd_MovesFile: archive moves a session .jsonl into --archive-dir and
// prints the new path; the source is gone and the destination exists.
func TestArchiveCmd_MovesFile(t *testing.T) {
	root := newCfgRoot(t)
	// Pass the full .jsonl path: lifecycle.Archive accepts a path verbatim, which
	// keeps the test independent of the package's bare-id resolution root.
	src := writeSession(t, root, "proj-a", "a1b2c3d4e5f6", 3)
	archiveDir := t.TempDir()

	out, err := runCmd(t, newArchiveCmd(), "", src, "--archive-dir", archiveDir)
	if err != nil {
		t.Fatalf("archive: %v\nout: %s", err, out)
	}

	wantDst := filepath.Join(archiveDir, "a1b2c3d4e5f6.jsonl")
	if !strings.Contains(out, wantDst) {
		t.Errorf("archive output %q does not contain dest %q", out, wantDst)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source still present after archive (err=%v)", err)
	}
	if _, err := os.Stat(wantDst); err != nil {
		t.Errorf("dest missing after archive: %v", err)
	}
}

// TestArchiveCmd_NotFound: a missing session yields a friendly "not found"
// message and a non-zero ExitError (code 1).
func TestArchiveCmd_NotFound(t *testing.T) {
	newCfgRoot(t) // empty projects tree
	archiveDir := t.TempDir()

	out, err := runCmd(t, newArchiveCmd(), "", "deadbeefcafe", "--archive-dir", archiveDir)
	if err == nil {
		t.Fatalf("expected error for missing session; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) {
		t.Fatalf("want ExitError, got %T: %v", err, err)
	}
	if ee.Code == 0 {
		t.Errorf("want non-zero exit code, got %d", ee.Code)
	}
	if !strings.Contains(ee.Msg, "session not found") {
		t.Errorf("want friendly not-found message, got %q", ee.Msg)
	}
}

// TestDeleteCmd_DryRunReportsWithoutDeleting: --dry-run prints the plan but
// leaves every matched file on disk.
func TestDeleteCmd_DryRunReportsWithoutDeleting(t *testing.T) {
	root := newCfgRoot(t)
	thin := writeSession(t, root, "proj-a", "thin0000aaaa", 2)
	fat := writeSession(t, root, "proj-a", "fat00000bbbb", 50)

	// --max-messages 5 matches only the thin session.
	out, err := runCmd(t, newDeleteCmd(), "", "--max-messages", "5", "--dry-run")
	if err != nil {
		t.Fatalf("delete dry-run: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "thin0000") {
		t.Errorf("plan should list the thin session; out: %s", out)
	}
	if strings.Contains(out, "fat00000") {
		t.Errorf("plan should NOT list the fat session (>5 msgs); out: %s", out)
	}
	// Nothing deleted on a dry run.
	if _, err := os.Stat(thin); err != nil {
		t.Errorf("thin session deleted on dry-run: %v", err)
	}
	if _, err := os.Stat(fat); err != nil {
		t.Errorf("fat session vanished on dry-run: %v", err)
	}
}

// TestDeleteCmd_NoFilter: with no filter, delete refuses and returns a non-zero
// ExitError carrying the refusal message.
func TestDeleteCmd_NoFilter(t *testing.T) {
	newCfgRoot(t)

	out, err := runCmd(t, newDeleteCmd(), "")
	if err == nil {
		t.Fatalf("expected ErrNoFilter refusal; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) {
		t.Fatalf("want ExitError, got %T: %v", err, err)
	}
	if ee.Code == 0 {
		t.Errorf("want non-zero exit code, got %d", ee.Code)
	}
	if !strings.Contains(ee.Msg, "refusing to delete all sessions") {
		t.Errorf("want refusal message, got %q", ee.Msg)
	}
}

// TestDeleteCmd_ConfirmYesDeletes: without --dry-run, a 'y' on stdin triggers the
// real delete; the matched file is gone and a tombstone records its id.
func TestDeleteCmd_ConfirmYesDeletes(t *testing.T) {
	root := newCfgRoot(t)
	thin := writeSession(t, root, "proj-a", "thin1111cccc", 1)

	out, err := runCmd(t, newDeleteCmd(), "y\n", "--max-messages", "5")
	if err != nil {
		t.Fatalf("delete confirm-yes: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Deleted 1 session") {
		t.Errorf("want deletion summary; out: %s", out)
	}
	if _, err := os.Stat(thin); !os.IsNotExist(err) {
		t.Errorf("session still present after confirmed delete (err=%v)", err)
	}
}

// TestDeleteCmd_ConfirmNoAborts: a 'n' (or non-tty EOF) on stdin aborts; the file
// survives, the abort message prints, and the exit code is 1 (silent — the
// message already printed) so a script can distinguish "aborted" from "deleted".
func TestDeleteCmd_ConfirmNoAborts(t *testing.T) {
	root := newCfgRoot(t)
	thin := writeSession(t, root, "proj-a", "thin2222dddd", 1)

	out, err := runCmd(t, newDeleteCmd(), "n\n", "--max-messages", "5")
	if err == nil {
		t.Fatalf("aborted delete should exit non-zero; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) || ee.Code != 1 {
		t.Fatalf("want ExitError code 1, got %T: %v", err, err)
	}
	if ee.Msg != "" {
		t.Errorf("abort ExitError should carry no message (already printed), got %q", ee.Msg)
	}
	if !strings.Contains(out, "Aborted; nothing deleted.") {
		t.Errorf("want abort message; out: %s", out)
	}
	if _, err := os.Stat(thin); err != nil {
		t.Errorf("session deleted despite 'n' answer: %v", err)
	}
}

// TestDeleteCmd_EOFAbortExitsOne: the EOF abort (non-tty / closed stdin, no
// --yes) takes the same exit-1 path as an explicit 'n'.
func TestDeleteCmd_EOFAbortExitsOne(t *testing.T) {
	root := newCfgRoot(t)
	thin := writeSession(t, root, "proj-a", "thin5555aaaa", 1)

	out, err := runCmd(t, newDeleteCmd(), "", "--max-messages", "5")
	if err == nil {
		t.Fatalf("EOF-aborted delete should exit non-zero; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) || ee.Code != 1 {
		t.Fatalf("want ExitError code 1, got %T: %v", err, err)
	}
	if _, serr := os.Stat(thin); serr != nil {
		t.Errorf("session deleted despite EOF abort: %v", serr)
	}
	_ = out
}

// TestDeleteCmd_YesSkipsPrompt: --yes deletes without consulting stdin. With
// empty stdin (which would abort via EOF if the prompt ran), the matched session
// is still removed — proving the prompt was skipped for non-interactive use.
func TestDeleteCmd_YesSkipsPrompt(t *testing.T) {
	root := newCfgRoot(t)
	thin := writeSession(t, root, "proj-a", "thin3333eeee", 1)

	// Empty stdin: if the y/N prompt ran, EOF would abort and the file would
	// survive. With --yes the prompt is skipped and the delete proceeds.
	out, err := runCmd(t, newDeleteCmd(), "", "--max-messages", "5", "--yes")
	if err != nil {
		t.Fatalf("delete --yes: %v\nout: %s", err, out)
	}
	if strings.Contains(out, "[y/N]") {
		t.Errorf("--yes should not print the y/N prompt; out: %s", out)
	}
	if !strings.Contains(out, "Deleted 1 session") {
		t.Errorf("want deletion summary under --yes; out: %s", out)
	}
	if _, err := os.Stat(thin); !os.IsNotExist(err) {
		t.Errorf("session still present after --yes delete (err=%v)", err)
	}
}

// TestDeleteCmd_YesShortFlag: the -y short form behaves identically to --yes.
func TestDeleteCmd_YesShortFlag(t *testing.T) {
	root := newCfgRoot(t)
	thin := writeSession(t, root, "proj-a", "thin4444ffff", 1)

	out, err := runCmd(t, newDeleteCmd(), "", "--max-messages", "5", "-y")
	if err != nil {
		t.Fatalf("delete -y: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Deleted 1 session") {
		t.Errorf("want deletion summary under -y; out: %s", out)
	}
	if _, err := os.Stat(thin); !os.IsNotExist(err) {
		t.Errorf("session still present after -y delete (err=%v)", err)
	}
}

// retainSession indexes a session, then removes its backing file and
// reindexes so the store carries a RETAINED row (missing_since set) for it —
// the state durable retention leaves behind when a source tool purges a
// transcript, and the exact state `delete` must reach.
func retainSession(t *testing.T, root, project, id string, nLines int) {
	t.Helper()
	path := writeSession(t, root, project, id, nLines)
	dir := filepath.Dir(path)
	if _, _, _, err := index.EnsureIndexed(dir, false); err != nil {
		t.Fatalf("EnsureIndexed pass 1: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := index.EnsureIndexed(dir, false); err != nil {
		t.Fatalf("EnsureIndexed pass 2 (after purge): %v", err)
	}
}

// TestDeleteCmd_ReachesRetainedSession is the CLI-level proof of the union:
// a RETAINED session (source .jsonl already gone) has no live file for
// matchSessions to find, yet `delete --project <label> --yes` must still
// report it and tombstone it — the feature's contract is that explicit delete
// is the only way retained history dies, so delete has to actually reach it.
func TestDeleteCmd_ReachesRetainedSession(t *testing.T) {
	root := newCfgRoot(t)
	retainSession(t, root, "proj-retained", "abcret0001xyz", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--project", "proj-retained", "--yes")
	if err != nil {
		t.Fatalf("delete --project proj-retained --yes: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "(source file already gone)") {
		t.Errorf("plan should label the retained match; out: %s", out)
	}
	// The plan renders trunc8(sessionID), same convention as a live match's row.
	if !strings.Contains(out, trunc8("abcret0001xyz")) {
		t.Errorf("plan should list the retained session id; out: %s", out)
	}
	if !strings.Contains(out, "Deleted 1 session(s) (1 retained)") {
		t.Errorf("want a summary crediting the retained delete; out: %s", out)
	}

	// The tombstone sidecar (under the isolated HOME newCfgRoot set) now carries
	// the retained session's full id, so a later reindex would skip resurrecting it.
	tombPath := filepath.Join(root, "..", ".cache", "session-search", ".deleted")
	b, rerr := os.ReadFile(tombPath)
	if rerr != nil {
		t.Fatalf("read tombstone %q: %v", tombPath, rerr)
	}
	if !strings.Contains(string(b), "abcret0001xyz") {
		t.Errorf("tombstone %q missing retained session id; got %q", tombPath, string(b))
	}
}

// TestDeleteCmd_DryRunReportsRetainedWithoutTombstoning: --dry-run must show
// the retained match (so an operator can see what a real delete would reach)
// without writing anything — same invariant the live-file dry run already
// upholds.
func TestDeleteCmd_DryRunReportsRetainedWithoutTombstoning(t *testing.T) {
	root := newCfgRoot(t)
	retainSession(t, root, "proj-retained-dry", "dryret0001xyz", 1)

	out, err := runCmd(t, newDeleteCmd(), "", "--project", "proj-retained-dry", "--dry-run")
	if err != nil {
		t.Fatalf("delete --dry-run: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, trunc8("dryret0001xyz")) {
		t.Errorf("dry-run plan should list the retained session; out: %s", out)
	}
	if strings.Contains(out, "Deleted") {
		t.Errorf("dry-run must not report a deletion; out: %s", out)
	}

	tombPath := filepath.Join(root, "..", ".cache", "session-search", ".deleted")
	if b, rerr := os.ReadFile(tombPath); rerr == nil && strings.Contains(string(b), "dryret0001xyz") {
		t.Errorf("dry-run must not tombstone anything, but found the id in %q: %q", tombPath, string(b))
	}
}

// TestParseBefore covers the --before accepted formats and rejection.
func TestParseBefore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		wantErr bool
		wantSet bool
	}{
		{"empty unset", "", false, false},
		{"date only", "2026-01-02", false, true},
		{"rfc3339", "2026-01-02T15:04:05Z", false, true},
		{"garbage", "not-a-date", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseBefore(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseBefore(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got.IsZero() == tc.wantSet {
				t.Errorf("parseBefore(%q) set=%v, want set=%v", tc.in, !got.IsZero(), tc.wantSet)
			}
		})
	}
}

// TestHumanizeBytes covers the unit boundaries and the negative clamp.
func TestHumanizeBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"one kib", 1024, "1.0 KB"},
		{"mib", 1024 * 1024, "1.0 MB"},
		{"negative clamps", -5, "0 B"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := humanizeBytes(tc.in); got != tc.want {
				t.Errorf("humanizeBytes(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
