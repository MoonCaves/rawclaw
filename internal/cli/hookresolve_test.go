package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestShellSingleQuote covers the POSIX single-quote escaping that makes a baked
// path a single safe shell word even with spaces or an embedded apostrophe.
func TestShellSingleQuote(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{`/usr/local/bin/rawclaw`, `'/usr/local/bin/rawclaw'`},
		{`/Users/a b/.local/bin/rawclaw`, `'/Users/a b/.local/bin/rawclaw'`},
		{`/Users/o'brien/rawclaw`, `'/Users/o'\''brien/rawclaw'`},
	} {
		if got := shellSingleQuote(tc.in); got != tc.want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestRawclawBinQuoted_ErrorFallsBackToEmptyWord: an unresolvable self path must
// yield the empty shell word """ (never a setup failure) — the hook's PATH
// fallback then covers it.
func TestRawclawBinQuoted_ErrorFallsBackToEmptyWord(t *testing.T) {
	old := selfExe
	t.Cleanup(func() { selfExe = old })

	selfExe = func() (string, error) { return "", errors.New("boom") }
	if got := rawclawBinQuoted(); got != "''" {
		t.Errorf("rawclawBinQuoted() on error = %q, want \"''\"", got)
	}

	selfExe = func() (string, error) { return "/opt/rawclaw/bin/rawclaw", nil }
	if got := rawclawBinQuoted(); got != `'/opt/rawclaw/bin/rawclaw'` {
		t.Errorf("rawclawBinQuoted() = %q, want the quoted abs path", got)
	}
}

// runPrime renders the Claude prime template with quotedBin, writes it, and runs
// it under sh with the given PATH — returning stdout and any run error. TMPDIR is
// a fresh dir so the once-per-session marker never suppresses the banner.
func runPrime(t *testing.T, quotedBin, path string) (string, error) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	scriptPath := filepath.Join(t.TempDir(), "prime.sh")
	if err := os.WriteFile(scriptPath, []byte(renderHookScript(rawclawPrimeScript, quotedBin)), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, scriptPath)
	cmd.Env = []string{"PATH=" + path, "TMPDIR=" + t.TempDir()}
	cmd.Stdin = strings.NewReader(`{"session_id":"resolve-test"}`)
	out, err := cmd.Output()
	return string(out), err
}

// stubRawclaw writes an executable `rawclaw` (exit 0, no output) into dir and
// returns its full path.
func stubRawclaw(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "rawclaw")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestPrimeHook_BakedPathFiresOffPATH is the core regression: the binary is NOT
// on the hook's PATH (the real bug — ~/.local/bin absent from a non-login PATH),
// yet the baked absolute path makes the banner fire anyway.
func TestPrimeHook_BakedPathFiresOffPATH(t *testing.T) {
	bin := stubRawclaw(t, filepath.Join(t.TempDir(), "offpath"))
	out, err := runPrime(t, shellSingleQuote(bin), "/usr/bin:/bin") // PATH excludes bin's dir
	if err != nil {
		t.Fatalf("hook errored: %v", err)
	}
	if !strings.Contains(out, "Raw transcript history for context") {
		t.Errorf("banner did not fire via baked path; out=%q", out)
	}
}

// TestPrimeHook_SpaceInBakedPath: a baked path containing a space still resolves
// (proves the shell-quoting), off PATH.
func TestPrimeHook_SpaceInBakedPath(t *testing.T) {
	bin := stubRawclaw(t, filepath.Join(t.TempDir(), "bin dir"))
	out, err := runPrime(t, shellSingleQuote(bin), "/usr/bin:/bin")
	if err != nil {
		t.Fatalf("hook errored: %v", err)
	}
	if !strings.Contains(out, "Raw transcript history for context") {
		t.Errorf("banner did not fire with a spaced baked path; out=%q", out)
	}
}

// TestPrimeHook_FallsBackToPATH: the baked path is gone (binary moved/upgraded),
// but rawclaw is on PATH — the command -v fallback fires the banner.
func TestPrimeHook_FallsBackToPATH(t *testing.T) {
	onPath := filepath.Join(t.TempDir(), "onpath")
	stubRawclaw(t, onPath)
	out, err := runPrime(t, `'/nonexistent/rawclaw'`, onPath+":/usr/bin:/bin")
	if err != nil {
		t.Fatalf("hook errored: %v", err)
	}
	if !strings.Contains(out, "Raw transcript history for context") {
		t.Errorf("banner did not fire via PATH fallback; out=%q", out)
	}
}

// TestPrimeHook_SilentNoOpWhenUnresolvable: baked path gone AND nothing on PATH →
// no banner, clean exit 0 (never a hook error).
func TestPrimeHook_SilentNoOpWhenUnresolvable(t *testing.T) {
	out, err := runPrime(t, `'/nonexistent/rawclaw'`, "/usr/bin:/bin")
	if err != nil {
		t.Fatalf("hook must exit 0 when rawclaw is unresolvable, got: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected silent no-op, got output: %q", out)
	}
}
