package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── Fix 1a/1b: delete confirmation wording is provenance-aware ──

// TestDeleteCmd_LivePromptWordingAndReceipt: a delete that would remove LIVE
// sessions (original transcript files still on disk) prompts with the
// removes-the-originals wording and, on 'y', prints the matching receipt.
func TestDeleteCmd_LivePromptWordingAndReceipt(t *testing.T) {
	root := newCfgRoot(t)
	live := writeSession(t, root, "proj-a", "gate0001-0000-0000-0000-000000000001", 2)

	out, err := runCmd(t, newDeleteCmd(), "y\n", "gate0001")
	if err != nil {
		t.Fatalf("live delete confirm-yes: %v\nout: %s", err, out)
	}
	wantPrompt := "This removes rawclaw's copy (index and archive) and the original session transcript files. Confirm with your user. [y/N]"
	if !strings.Contains(out, wantPrompt) {
		t.Errorf("live delete prompt missing the ratified wording:\nwant %q\nout: %s", wantPrompt, out)
	}
	wantReceipt := "Removed rawclaw's copy (index and archive) and the original session transcript files."
	if !strings.Contains(out, wantReceipt) {
		t.Errorf("live delete receipt missing:\nwant %q\nout: %s", wantReceipt, out)
	}
	if _, serr := os.Stat(live); !os.IsNotExist(serr) {
		t.Errorf("session still present after confirmed live delete (err=%v)", serr)
	}
}

// TestDeleteCmd_RetainedOnlyPromptKeepsShape: a retained-only delete (originals
// already gone) keeps the existing prompt and receipt — rawclaw's copy only.
func TestDeleteCmd_RetainedOnlyPromptKeepsShape(t *testing.T) {
	root := newCfgRoot(t)
	retainSession(t, root, "proj-retained", "gate0002-0000-0000-0000-000000000002", 2)

	out, err := runCmd(t, newDeleteCmd(), "y\n", "gate0002")
	if err != nil {
		t.Fatalf("retained delete confirm-yes: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Delete 1 session(s)? This is irreversible. [y/N]") {
		t.Errorf("retained-only delete should keep the current prompt shape; out: %s", out)
	}
	if strings.Contains(out, "original session transcript files") {
		t.Errorf("retained-only delete must not claim original files are removed; out: %s", out)
	}
	if !strings.Contains(out, "Removed rawclaw's copy (index + archive). Claude Code / Codex transcript files are untouched.") {
		t.Errorf("retained-only receipt missing; out: %s", out)
	}
}

// ── Fix 1c: --yes alone no longer bypasses the gate for live files ──

// TestDeleteCmd_YesAloneRefusedWhenLiveFiles: --yes without --files on a delete
// that removes original transcript files is refused (exit 2) with the
// re-run-with---files pointer; nothing is deleted or tombstoned.
func TestDeleteCmd_YesAloneRefusedWhenLiveFiles(t *testing.T) {
	root := newCfgRoot(t)
	live := writeSession(t, root, "proj-a", "gate0003-0000-0000-0000-000000000003", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "gate0003")
	if err == nil {
		t.Fatalf("--yes on a live-file delete should refuse; out: %s", out)
	}
	var ee ExitError
	if !asExit(err, &ee) || ee.Code != 2 {
		t.Fatalf("want ExitError code 2, got %T: %v", err, err)
	}
	wantMsg := "This delete removes original transcript files. Confirm with your user, then re-run with --yes --files."
	if ee.Msg != wantMsg {
		t.Errorf("refusal message = %q, want %q", ee.Msg, wantMsg)
	}
	if _, serr := os.Stat(live); serr != nil {
		t.Errorf("session deleted despite the --yes refusal: %v", serr)
	}
	tombPath := filepath.Join(root, "..", ".cache", "session-search", ".deleted")
	if b, rerr := os.ReadFile(tombPath); rerr == nil && strings.Contains(string(b), "gate0003") {
		t.Errorf("refused delete tombstoned the session: %q", string(b))
	}
}

// TestDeleteCmd_YesFilesDeletesLiveNonInteractive: --yes --files authorizes the
// non-interactive delete of original files; no prompt, receipt printed.
func TestDeleteCmd_YesFilesDeletesLiveNonInteractive(t *testing.T) {
	root := newCfgRoot(t)
	live := writeSession(t, root, "proj-a", "gate0004-0000-0000-0000-000000000004", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "--files", "gate0004")
	if err != nil {
		t.Fatalf("delete --yes --files: %v\nout: %s", err, out)
	}
	if strings.Contains(out, "[y/N]") {
		t.Errorf("--yes --files should not prompt; out: %s", out)
	}
	if !strings.Contains(out, "Removed rawclaw's copy (index and archive) and the original session transcript files.") {
		t.Errorf("live receipt missing under --yes --files; out: %s", out)
	}
	if _, serr := os.Stat(live); !os.IsNotExist(serr) {
		t.Errorf("session still present after --yes --files delete (err=%v)", serr)
	}
}

// TestDeleteCmd_YesAloneStillWorksRetainedOnly: retained-only deletes remove no
// original files, so --yes alone keeps working non-interactively.
func TestDeleteCmd_YesAloneStillWorksRetainedOnly(t *testing.T) {
	root := newCfgRoot(t)
	retainSession(t, root, "proj-retained", "gate0005-0000-0000-0000-000000000005", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--yes", "gate0005")
	if err != nil {
		t.Fatalf("retained-only delete --yes: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "Deleted 1 session(s) (1 retained)") {
		t.Errorf("want retained delete summary; out: %s", out)
	}
}

// TestDeleteCmd_FilesWithoutYesStillPrompts: --files is only meaningful with
// --yes — alone it neither errors nor skips the prompt.
func TestDeleteCmd_FilesWithoutYesStillPrompts(t *testing.T) {
	root := newCfgRoot(t)
	live := writeSession(t, root, "proj-a", "gate0006-0000-0000-0000-000000000006", 2)

	out, err := runCmd(t, newDeleteCmd(), "n\n", "--files", "gate0006")
	if err == nil {
		t.Fatalf("'n' abort should exit non-zero; out: %s", out)
	}
	if !strings.Contains(out, "[y/N]") {
		t.Errorf("--files alone must still prompt; out: %s", out)
	}
	if _, serr := os.Stat(live); serr != nil {
		t.Errorf("session deleted despite 'n' answer: %v", serr)
	}
}

// TestDeleteCmd_DryRunUnaffectedByGate: --dry-run stays exit 0 and touches
// nothing, with or without --yes — the gate never fires on a plan-only run.
func TestDeleteCmd_DryRunUnaffectedByGate(t *testing.T) {
	root := newCfgRoot(t)
	live := writeSession(t, root, "proj-a", "gate0007-0000-0000-0000-000000000007", 2)

	out, err := runCmd(t, newDeleteCmd(), "", "--dry-run", "--yes", "gate0007")
	if err != nil {
		t.Fatalf("--dry-run --yes should stay exit 0: %v\nout: %s", err, out)
	}
	if _, serr := os.Stat(live); serr != nil {
		t.Errorf("dry-run deleted the session: %v", serr)
	}
}

// TestDeleteCmd_HelpNamesFilesFlag: `delete --help` documents --files and the
// --yes gate.
func TestDeleteCmd_HelpNamesFilesFlag(t *testing.T) {
	newCfgRoot(t)
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "delete", "--help")
	if err != nil {
		t.Fatalf("delete --help: %v", err)
	}
	if !strings.Contains(out, "--files") {
		t.Errorf("delete --help missing the --files flag:\n%s", out)
	}
}

// ── Fix 2: folder guard — implicit cwd discovery must not index a loose-jsonl dir ──

// TestBareBrowseFromLooseJSONLDirDoesNotIndex is the /tmp incident's
// regression test: a bare `rawclaw` run FROM a directory that happens to hold
// loose .jsonl files must NOT treat that directory as a transcripts dir — no
// browse rows from it, and no index db created for it in the cache.
func TestBareBrowseFromLooseJSONLDirDoesNotIndex(t *testing.T) {
	newCfgRoot(t) // empty projects tree; HOME isolated to cfg
	loose := t.TempDir()
	if err := os.WriteFile(filepath.Join(loose, "x.jsonl"),
		[]byte(`{"type":"user","text":"hi"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(loose)

	// Root built AFTER the chdir so --dir defaults to the loose dir — the exact
	// bare-run shape of the incident.
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "")
	if err != nil {
		t.Fatalf("bare browse: %v\nout: %s", err, out)
	}
	if !strings.Contains(out, "No transcript history") {
		t.Errorf("bare run from a loose-jsonl dir should resolve no history; out: %s", out)
	}
	home, _ := os.UserHomeDir()
	dbs, _ := filepath.Glob(filepath.Join(home, ".cache", "session-search", "*.db"))
	if len(dbs) != 0 {
		t.Errorf("bare run indexed the loose dir into the cache: %v", dbs)
	}
}

// TestExplicitDirLooseJSONLStillResolves: --dir is the explicit opt-in — a
// jsonl-bearing folder passed by flag still resolves as a transcripts dir.
func TestExplicitDirLooseJSONLStillResolves(t *testing.T) {
	newCfgRoot(t)
	loose := t.TempDir()
	if err := os.WriteFile(filepath.Join(loose, "sess0001-0000-0000-0000-000000000001.jsonl"),
		[]byte(`{"type":"user","message":{"role":"user","content":"hi"},"timestamp":"2026-01-02T03:04:05Z","uuid":"u1"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--dir", loose)
	if err != nil {
		t.Fatalf("browse --dir <loose>: %v\nout: %s", err, out)
	}
	if strings.Contains(out, "No transcript history") {
		t.Errorf("explicit --dir on a jsonl-bearing folder should resolve; out: %s", out)
	}
}

// ── Fix 3: syncing archive verbs run stall-bounded, not wall-clock-bounded ──

// TestResolveTimeoutFromArgsArchiveStallPosture: archive init/push/pull (and a
// manual autosync) sync with a git remote — no wall-clock cap fits both a hung
// transfer and a legitimate slow multi-GB first push, so their default is
// watchdog OFF (0); hangs die on stall via the git children's stall detection
// instead. An explicit --timeout / RAWCLAW_TIMEOUT still takes precedence.
func TestResolveTimeoutFromArgsArchiveStallPosture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		env  string
		want time.Duration
	}{
		{"archive push disables the watchdog", []string{"archive", "push"}, "", 0},
		{"archive pull disables the watchdog", []string{"archive", "pull"}, "", 0},
		{"archive init disables the watchdog", []string{"archive", "init", "git@example.com:o/r.git"}, "", 0},
		{"archive autosync disables the watchdog", []string{"archive", "autosync"}, "", 0},
		{"explicit --timeout takes precedence", []string{"archive", "push", "--timeout", "90s"}, "", 90 * time.Second},
		{"explicit --timeout=0 stays disabled", []string{"archive", "push", "--timeout", "0"}, "", 0},
		{"RAWCLAW_TIMEOUT takes precedence", []string{"archive", "push"}, "45s", 45 * time.Second},
		{"timeout flag before subcommand still wins", []string{"--timeout", "10s", "archive", "push"}, "", 10 * time.Second},
		{"archive status keeps default", []string{"archive", "status"}, "", defaultTimeout},
		{"archive <session> move keeps default", []string{"archive", "cafe0001"}, "", defaultTimeout},
		{"non-archive command keeps default", []string{"some", "query"}, "", defaultTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveTimeoutFromArgs(tc.args, tc.env); got != tc.want {
				t.Errorf("resolveTimeoutFromArgs(%q, %q) = %v, want %v", tc.args, tc.env, got, tc.want)
			}
		})
	}
}

func TestIsArchiveSyncInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"archive", "push"}, true},
		{[]string{"archive", "pull"}, true},
		{[]string{"archive", "init", "url"}, true},
		{[]string{"archive", "autosync"}, true},
		{[]string{"--timeout", "5s", "archive", "push"}, true},
		{[]string{"archive", "status"}, false},
		{[]string{"archive", "enable-timer"}, false},
		{[]string{"archive", "cafe0001"}, false},
		{[]string{"archive"}, false},
		{[]string{"upgrade"}, false},
		{[]string{"push"}, false},
		{nil, false},
	}
	for _, tc := range tests {
		if got := isArchiveSyncInvocation(tc.args); got != tc.want {
			t.Errorf("isArchiveSyncInvocation(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
