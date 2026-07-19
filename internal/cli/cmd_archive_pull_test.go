package cli

import (
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/archive/archivetest"
)

// TestArchivePullCmd_Refreshes: explicit `archive pull` refreshes the clone
// and says so — even right after a pull (explicit always bypasses the
// throttle).
func TestArchivePullCmd_Refreshes(t *testing.T) {
	archivetest.Setup(t, "") // leaves a fresh throttle stamp behind

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "pull")
	if err != nil {
		t.Fatalf("archive pull: %v\n%s", err, out)
	}
	if !strings.Contains(out, "refreshed") {
		t.Errorf("explicit pull output = %q, want a refresh report (throttle bypassed)", out)
	}
}

// TestArchivePullCmd_ThrottleSkips: `archive pull --throttle` right after a
// pull honors the stamp and reports the skip.
func TestArchivePullCmd_ThrottleSkips(t *testing.T) {
	archivetest.Setup(t, "") // fixture's Pull wrote a fresh stamp

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "pull", "--throttle")
	if err != nil {
		t.Fatalf("archive pull --throttle: %v\n%s", err, out)
	}
	if !strings.Contains(out, "skipped") {
		t.Errorf("throttled pull output = %q, want a skip report", out)
	}
}

// TestArchivePullCmd_UnconfiguredNoOp: pull without init is a clean no-op with
// a pointer at init — the same feature-off contract as push.
func TestArchivePullCmd_UnconfiguredNoOp(t *testing.T) {
	newArchiveHome(t)

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "pull")
	if err != nil {
		t.Fatalf("unconfigured pull errored: %v", err)
	}
	if !strings.Contains(out, "archive init") {
		t.Errorf("no-op output should point at init, got %q", out)
	}
}

// TestResume_ForeignSessionDegrades: --resume on a session that lives on
// another machine can't hand back a runnable local command — it degrades with
// a clear message naming the machine and the command to run THERE.
func TestResume_ForeignSessionDegrades(t *testing.T) {
	archivetest.Setup(t, "")

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "--resume", archivetest.ForeignSession[:8])
	if err != nil {
		t.Fatalf("--resume foreign: %v\n%s", err, out)
	}
	if !strings.Contains(out, archivetest.ForeignName) {
		t.Errorf("degrade message does not name the machine:\n%s", out)
	}
	if !strings.Contains(out, "claude --resume "+archivetest.ForeignSession) {
		t.Errorf("degrade message missing the remote-side resume command:\n%s", out)
	}
	if !strings.Contains(out, "another machine") {
		t.Errorf("degrade message should say the session lives on another machine:\n%s", out)
	}
}

// TestResume_LocalStillWins: a local session resolves exactly as before — the
// foreign fallback only fires when nothing local matches.
func TestResume_LocalStillWins(t *testing.T) {
	archivetest.Setup(t, "")

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "--resume", "localses")
	if err != nil {
		t.Fatalf("--resume local: %v\n%s", err, out)
	}
	if !strings.Contains(out, "claude --resume localsess") {
		t.Errorf("local resume broken:\n%s", out)
	}
	if strings.Contains(out, "another machine") {
		t.Errorf("local resume wrongly degraded:\n%s", out)
	}
}
