package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTimerTool swaps the launchctl/systemctl seam for a recorder. failOn
// (optional) makes one specific invocation fail, so error paths are testable.
func fakeTimerTool(t *testing.T, failOn string) *[]string {
	t.Helper()
	var calls []string
	old := runTimerTool
	runTimerTool = func(ctx context.Context, name string, args ...string) (string, error) {
		line := name + " " + strings.Join(args, " ")
		calls = append(calls, line)
		if failOn != "" && strings.Contains(line, failOn) {
			return "boom", fmt.Errorf("%s: exit status 1", line)
		}
		return "", nil
	}
	t.Cleanup(func() { runTimerTool = old })
	return &calls
}

// fakeExe pins the self-binary path the timer artifacts embed.
func fakeExe(t *testing.T, path string) {
	t.Helper()
	old := selfExe
	selfExe = func() (string, error) { return path, nil }
	t.Cleanup(func() { selfExe = old })
}

// TestLaunchdPlist_Content: the agent runs `<exe> archive push` hourly with
// output to the state-dir log — and XML-escapes hostile path bytes.
func TestLaunchdPlist_Content(t *testing.T) {
	t.Parallel()
	got := launchdPlist("/opt/we&rd/rawclaw", "/logs/timer.log")
	for _, want := range []string{
		"<string>" + launchdLabel + "</string>",
		"<string>/opt/we&amp;rd/rawclaw</string>",
		"<string>archive</string>",
		"<string>push</string>",
		"<string>--timeout</string>",
		"<string>" + autosyncChildTimeoutArg + "</string>",
		"<integer>3600</integer>",
		"<key>RunAtLoad</key>\n\t<false/>",
		"<string>/logs/timer.log</string>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q:\n%s", want, got)
		}
	}
}

// TestSystemdUnits_Content: oneshot service + hourly persistent timer.
func TestSystemdUnits_Content(t *testing.T) {
	t.Parallel()
	svc := systemdServiceUnit("/opt/rawclaw")
	for _, want := range []string{"Type=oneshot", `ExecStart="/opt/rawclaw" archive push --timeout ` + autosyncChildTimeoutArg} {
		if !strings.Contains(svc, want) {
			t.Errorf("service unit missing %q:\n%s", want, svc)
		}
	}
	tm := systemdTimerUnit()
	for _, want := range []string{"OnCalendar=hourly", "Persistent=true", "WantedBy=timers.target"} {
		if !strings.Contains(tm, want) {
			t.Errorf("timer unit missing %q:\n%s", want, tm)
		}
	}
}

// TestSystemdServiceUnit_EscapesHostilePaths: %, ", and \ in the install path
// survive systemd's quoting AND specifier expansion.
func TestSystemdServiceUnit_EscapesHostilePaths(t *testing.T) {
	t.Parallel()
	svc := systemdServiceUnit(`/home/50%off/my "bin"\rawclaw`)
	if !strings.Contains(svc, `ExecStart="/home/50%%off/my \"bin\"\\rawclaw" archive push`) {
		t.Errorf("hostile path not escaped:\n%s", svc)
	}
}

// TestInstallTimer_Darwin: plist written under ~/Library/LaunchAgents, then
// bootout (stale registration) + bootstrap — in that order.
func TestInstallTimer_Darwin(t *testing.T) {
	home := t.TempDir()
	calls := fakeTimerTool(t, "")
	fakeExe(t, "/opt/rawclaw")

	var out bytes.Buffer
	if err := installTimer(context.Background(), &out, "darwin", home); err != nil {
		t.Fatalf("installTimer: %v", err)
	}

	p := launchdPlistPath(home)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if !strings.Contains(string(b), "/opt/rawclaw") {
		t.Errorf("plist lacks the exe path:\n%s", b)
	}
	want := []string{
		"launchctl bootout " + launchdDomain() + "/" + launchdLabel,
		"launchctl bootstrap " + launchdDomain() + " " + p,
	}
	if len(*calls) != 2 || (*calls)[0] != want[0] || (*calls)[1] != want[1] {
		t.Errorf("launchctl calls = %q, want %q", *calls, want)
	}
	if !strings.Contains(out.String(), "--eject") {
		t.Errorf("receipt should point at --eject:\n%s", out.String())
	}
}

// TestInstallTimer_DarwinBootstrapFailure: a failed bootstrap surfaces as an
// error AND takes the written plist back out — launchd loads everything under
// LaunchAgents at next login, so leaving it would be a delayed silent install.
func TestInstallTimer_DarwinBootstrapFailure(t *testing.T) {
	home := t.TempDir()
	fakeTimerTool(t, "bootstrap")
	fakeExe(t, "/opt/rawclaw")

	var out bytes.Buffer
	err := installTimer(context.Background(), &out, "darwin", home)
	if err == nil || !strings.Contains(err.Error(), launchdLabel) {
		t.Fatalf("err = %v, want a bootstrap failure naming the agent", err)
	}
	if _, serr := os.Stat(launchdPlistPath(home)); !os.IsNotExist(serr) {
		t.Errorf("plist left behind after failed bootstrap: %v", serr)
	}
}

// TestInstallTimer_Linux: both units written, then daemon-reload and
// enable --now on the timer.
func TestInstallTimer_Linux(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	calls := fakeTimerTool(t, "")
	fakeExe(t, "/opt/rawclaw")

	var out bytes.Buffer
	if err := installTimer(context.Background(), &out, "linux", home); err != nil {
		t.Fatalf("installTimer: %v", err)
	}

	servicePath, timerPath := systemdUnitPaths(home)
	for _, p := range []string{servicePath, timerPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("unit not written: %v", err)
		}
	}
	want := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now " + systemdUnitName + ".timer",
	}
	if len(*calls) != 2 || (*calls)[0] != want[0] || (*calls)[1] != want[1] {
		t.Errorf("systemctl calls = %q, want %q", *calls, want)
	}
}

// TestInstallTimer_LinuxHonorsXDG: units land under $XDG_CONFIG_HOME when set.
func TestInstallTimer_LinuxHonorsXDG(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	fakeTimerTool(t, "")
	fakeExe(t, "/opt/rawclaw")

	var out bytes.Buffer
	if err := installTimer(context.Background(), &out, "linux", home); err != nil {
		t.Fatalf("installTimer: %v", err)
	}
	if _, err := os.Stat(filepath.Join(xdg, "systemd", "user", systemdUnitName+".timer")); err != nil {
		t.Errorf("timer unit not under XDG_CONFIG_HOME: %v", err)
	}
}

// TestEjectTimer_DarwinRoundTrip: eject unloads and removes exactly the plist
// enable-timer wrote — a sibling agent in the same dir is untouched.
func TestEjectTimer_DarwinRoundTrip(t *testing.T) {
	home := t.TempDir()
	fakeTimerTool(t, "")
	fakeExe(t, "/opt/rawclaw")

	var out bytes.Buffer
	if err := installTimer(context.Background(), &out, "darwin", home); err != nil {
		t.Fatalf("installTimer: %v", err)
	}
	sibling := filepath.Join(home, "Library", "LaunchAgents", "com.example.other.plist")
	if err := os.WriteFile(sibling, []byte("not ours"), 0o644); err != nil {
		t.Fatal(err)
	}

	calls := fakeTimerTool(t, "") // fresh recorder for the eject half
	if err := ejectTimer(context.Background(), &out, "darwin", home); err != nil {
		t.Fatalf("ejectTimer: %v", err)
	}
	if _, err := os.Stat(launchdPlistPath(home)); !os.IsNotExist(err) {
		t.Errorf("plist still present after eject: %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("sibling plist touched by eject: %v", err)
	}
	if len(*calls) != 1 || !strings.HasPrefix((*calls)[0], "launchctl bootout ") {
		t.Errorf("eject calls = %q, want one bootout", *calls)
	}
}

// TestEjectTimer_LinuxRoundTrip: disable --now, both units removed, reload.
func TestEjectTimer_LinuxRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	fakeTimerTool(t, "")
	fakeExe(t, "/opt/rawclaw")

	var out bytes.Buffer
	if err := installTimer(context.Background(), &out, "linux", home); err != nil {
		t.Fatalf("installTimer: %v", err)
	}
	calls := fakeTimerTool(t, "")
	if err := ejectTimer(context.Background(), &out, "linux", home); err != nil {
		t.Fatalf("ejectTimer: %v", err)
	}
	servicePath, timerPath := systemdUnitPaths(home)
	for _, p := range []string{servicePath, timerPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still present after eject: %v", p, err)
		}
	}
	want := []string{
		"systemctl --user disable --now " + systemdUnitName + ".timer",
		"systemctl --user daemon-reload",
	}
	if len(*calls) != 2 || (*calls)[0] != want[0] || (*calls)[1] != want[1] {
		t.Errorf("eject calls = %q, want %q", *calls, want)
	}
}

// TestEjectTimer_NothingInstalledIsCleanNoOp: ejecting twice (or never having
// installed) reports the no-op and errors nothing.
func TestEjectTimer_NothingInstalledIsCleanNoOp(t *testing.T) {
	home := t.TempDir()
	fakeTimerTool(t, "")

	for _, goos := range []string{"darwin", "linux"} {
		var out bytes.Buffer
		if err := ejectTimer(context.Background(), &out, goos, home); err != nil {
			t.Errorf("%s eject on clean machine: %v", goos, err)
		}
		if !strings.Contains(out.String(), "nothing to remove") {
			t.Errorf("%s eject output = %q, want the no-op line", goos, out.String())
		}
	}
}

// TestInstallTimer_UnsupportedGOOS: a platform with no backend gets a clear
// error, not a silent success.
func TestInstallTimer_UnsupportedGOOS(t *testing.T) {
	fakeTimerTool(t, "")
	fakeExe(t, "/opt/rawclaw")
	var out bytes.Buffer
	if err := installTimer(context.Background(), &out, "plan9", t.TempDir()); err == nil {
		t.Fatal("installTimer on plan9 succeeded; want a no-backend error")
	}
}

// TestEnableTimerCmd_RequiresConfiguredArchive: enable-timer without an
// archive refuses with the init pointer and calls no system tool.
func TestEnableTimerCmd_RequiresConfiguredArchive(t *testing.T) {
	newArchiveHome(t)
	calls := fakeTimerTool(t, "")

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "enable-timer")
	if err != nil {
		t.Fatalf("enable-timer unconfigured: %v", err)
	}
	if !strings.Contains(out, "archive init") {
		t.Errorf("output = %q, want the init pointer", out)
	}
	if len(*calls) != 0 {
		t.Errorf("system tools invoked while unconfigured: %q", *calls)
	}
}

// TestEnableTimerCmd_EjectWorksUnconfigured: symmetric removal never blocks —
// eject runs even after the archive config is gone.
func TestEnableTimerCmd_EjectWorksUnconfigured(t *testing.T) {
	newArchiveHome(t)
	fakeTimerTool(t, "")

	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "archive", "enable-timer", "--eject")
	if err != nil {
		t.Fatalf("eject unconfigured: %v", err)
	}
	if !strings.Contains(out, "nothing to remove") {
		t.Errorf("output = %q, want the clean no-op line", out)
	}
}
