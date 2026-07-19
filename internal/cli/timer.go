package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// The hourly-push timer artifacts. One name each per platform, so eject can
// remove exactly what enable-timer added and nothing else.
const (
	// launchdLabel is the macOS launchd job label; its plist lives at
	// ~/Library/LaunchAgents/<label>.plist.
	launchdLabel = "com.rawclaw.archive-push"

	// systemdUnitName names the Linux systemd user units
	// (<name>.service + <name>.timer under ~/.config/systemd/user).
	systemdUnitName = "rawclaw-archive-push"

	// timerIntervalSec is the launchd push cadence (hourly). The systemd timer
	// says the same thing as OnCalendar=hourly.
	timerIntervalSec = 3600
)

// runTimerTool executes a system tool (launchctl / systemctl) and returns its
// combined output — a seam faked by unit tests, which assert the exact
// invocation sequence without touching the live service manager.
var runTimerTool = func(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// timerLogPath is <state-dir>/archive/timer.log — where launchd sends the
// hourly push's output (systemd's runs land in the user journal instead).
func timerLogPath() string {
	return filepath.Join(store.CacheDir(), "archive", "timer.log")
}

// launchdPlistPath is the one file enable-timer owns on macOS.
func launchdPlistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

// launchdDomain is the per-user GUI domain launchctl bootstrap/bootout target.
func launchdDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// xmlEscape escapes the five XML special characters for plist string values —
// install paths are user-controlled and may hold any of them.
var xmlEscape = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
).Replace

// launchdPlist renders the launchd agent: run `<exe> archive push` hourly,
// output to the timer log. RunAtLoad is off — installing the timer must not
// fire an immediate push; the first run comes on the first interval tick.
func launchdPlist(exe, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>archive</string>
		<string>push</string>
		<string>--timeout</string>
		<string>%s</string>
	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>RunAtLoad</key>
	<false/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, launchdLabel, xmlEscape(exe), autosyncChildTimeout, timerIntervalSec, xmlEscape(logPath), xmlEscape(logPath))
}

// systemdUserDir resolves the user-unit dir per the freedesktop spec:
// $XDG_CONFIG_HOME/systemd/user, else ~/.config/systemd/user.
func systemdUserDir(home string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "systemd", "user")
	}
	return filepath.Join(home, ".config", "systemd", "user")
}

// systemdUnitPaths returns the two files enable-timer owns on Linux.
func systemdUnitPaths(home string) (service, timer string) {
	dir := systemdUserDir(home)
	return filepath.Join(dir, systemdUnitName+".service"),
		filepath.Join(dir, systemdUnitName+".timer")
}

// systemdServiceUnit renders the oneshot service the timer fires. The exe path
// is double-quoted (systemd's own quoting) so a path with spaces survives.
func systemdServiceUnit(exe string) string {
	return fmt.Sprintf(`[Unit]
Description=rawclaw archive push

[Service]
Type=oneshot
ExecStart="%s" archive push --timeout %s
`, exe, autosyncChildTimeout)
}

// systemdTimerUnit renders the hourly timer. Persistent=true catches up after
// sleep/downtime — the hourly push runs on wake instead of silently skipping.
func systemdTimerUnit() string {
	return `[Unit]
Description=Hourly rawclaw archive push

[Timer]
OnCalendar=hourly
Persistent=true

[Install]
WantedBy=timers.target
`
}

// installTimer installs the hourly push for goos, writing the platform's
// artifacts and registering them with the live service manager. Idempotent:
// re-running replaces rawclaw's own artifacts in place.
func installTimer(w io.Writer, goos, home string) error {
	exe, err := autosyncExe()
	if err != nil {
		return fmt.Errorf("resolve rawclaw binary path: %w", err)
	}
	switch goos {
	case "darwin":
		p := launchdPlistPath(home)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(launchdPlist(exe, timerLogPath())), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", p, err)
		}
		// Unload any previous registration first (ignore "not loaded"), so a
		// re-run picks up the fresh plist instead of erroring on a live job.
		_, _ = runTimerTool("launchctl", "bootout", launchdDomain()+"/"+launchdLabel)
		if _, err := runTimerTool("launchctl", "bootstrap", launchdDomain(), p); err != nil {
			return fmt.Errorf("register launchd agent (plist written at %s): %w", p, err)
		}
		fmt.Fprintf(w, "Hourly archive push installed.\n  launchd agent: %s\n  log:           %s\nRemove it any time with `rawclaw archive enable-timer --eject`.\n",
			p, timerLogPath())
		return nil
	case "linux":
		servicePath, timerPath := systemdUnitPaths(home)
		if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(servicePath), err)
		}
		if err := os.WriteFile(servicePath, []byte(systemdServiceUnit(exe)), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", servicePath, err)
		}
		if err := os.WriteFile(timerPath, []byte(systemdTimerUnit()), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", timerPath, err)
		}
		if _, err := runTimerTool("systemctl", "--user", "daemon-reload"); err != nil {
			return fmt.Errorf("reload systemd user units (units written at %s): %w", filepath.Dir(servicePath), err)
		}
		if _, err := runTimerTool("systemctl", "--user", "enable", "--now", systemdUnitName+".timer"); err != nil {
			return fmt.Errorf("enable systemd user timer: %w", err)
		}
		fmt.Fprintf(w, "Hourly archive push installed.\n  systemd user units: %s + %s\n  logs:               journalctl --user -u %s\nRemove it any time with `rawclaw archive enable-timer --eject`.\n",
			servicePath, timerPath, systemdUnitName)
		return nil
	default:
		return fmt.Errorf("no timer backend for %s (launchd on macOS, systemd user timers on Linux); schedule `rawclaw archive push` with your platform's scheduler instead", goos)
	}
}

// ejectTimer removes exactly what installTimer added — the registration and
// rawclaw's own files, nothing else. Every step tolerates the thing already
// being gone: ejecting twice, or with nothing installed, is a clean no-op.
func ejectTimer(w io.Writer, goos, home string) error {
	switch goos {
	case "darwin":
		// Ignore bootout failure: "not loaded" is the normal already-ejected
		// state, and a stale registration with no plist must not block eject.
		_, _ = runTimerTool("launchctl", "bootout", launchdDomain()+"/"+launchdLabel)
		p := launchdPlistPath(home)
		removed, err := removeOwn(p)
		if err != nil {
			return err
		}
		if removed {
			fmt.Fprintf(w, "Hourly archive push removed (unloaded %s, deleted %s).\n", launchdLabel, p)
		} else {
			fmt.Fprintln(w, "No archive timer installed; nothing to remove.")
		}
		return nil
	case "linux":
		_, _ = runTimerTool("systemctl", "--user", "disable", "--now", systemdUnitName+".timer")
		servicePath, timerPath := systemdUnitPaths(home)
		removedTimer, err := removeOwn(timerPath)
		if err != nil {
			return err
		}
		removedService, err := removeOwn(servicePath)
		if err != nil {
			return err
		}
		_, _ = runTimerTool("systemctl", "--user", "daemon-reload")
		if removedTimer || removedService {
			fmt.Fprintf(w, "Hourly archive push removed (disabled %s.timer, deleted its units).\n", systemdUnitName)
		} else {
			fmt.Fprintln(w, "No archive timer installed; nothing to remove.")
		}
		return nil
	default:
		return fmt.Errorf("no timer backend for %s; nothing rawclaw could have installed here", goos)
	}
}

// removeOwn removes one rawclaw-owned file, reporting whether it existed.
// Missing is a clean no-op, not an error.
func removeOwn(p string) (bool, error) {
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("remove %s: %w", p, err)
	}
	return true, nil
}
