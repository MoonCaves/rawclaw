package archive

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// newTestHome isolates every path the archive touches under a temp HOME:
// the state dir (config, machine-id, clone), the Claude projects root, and
// the Codex sessions root. Returns the temp home.
func newTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir reads USERPROFILE on Windows (HOME everywhere else) —
	// set both so the isolation holds on every OS instead of touching real
	// user state.
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("RAWCLAW_ARCHIVE", "")
	return home
}

// TestLoad_Unconfigured: no config file → (nil, nil), the feature-off zero
// state every caller treats as one nil-check.
func TestLoad_Unconfigured(t *testing.T) {
	newTestHome(t)

	a, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v, want nil", err)
	}
	if a != nil {
		t.Fatalf("Load() = %+v, want nil (feature off)", a)
	}
}

// TestLoad_Configured: a written config round-trips through Load.
func TestLoad_Configured(t *testing.T) {
	newTestHome(t)

	cfg := Config{Remote: "/tmp/some-remote.git", Name: "machine-a"}
	if err := writeConfig(cfg); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	a, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}
	if a == nil {
		t.Fatal("Load() = nil, want configured archive")
	}
	if !reflect.DeepEqual(a.cfg, cfg) {
		t.Errorf("Load() cfg = %+v, want %+v", a.cfg, cfg)
	}
	if a.machineID == "" {
		t.Error("Load() machineID empty, want stable id")
	}
	if a.clone == "" {
		t.Error("Load() clone path empty")
	}
	if a.run == nil {
		t.Error("Load() run seam nil, want the exec adapter")
	}
}

// TestLoad_KillSwitch: RAWCLAW_ARCHIVE=off disables the feature even when
// configured.
func TestLoad_KillSwitch(t *testing.T) {
	newTestHome(t)

	if err := writeConfig(Config{Remote: "/tmp/r.git", Name: "m"}); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	t.Setenv("RAWCLAW_ARCHIVE", "off")

	a, err := Load()
	if err != nil || a != nil {
		t.Fatalf("Load() = (%v, %v), want (nil, nil) with kill switch", a, err)
	}
}

// TestLoad_CorruptConfig: a malformed config file is an error, not silent
// feature-off — the user configured the archive and deserves to know it broke.
func TestLoad_CorruptConfig(t *testing.T) {
	newTestHome(t)

	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := Load()
	if err == nil {
		t.Fatalf("Load() = (%v, nil), want error for corrupt config", a)
	}
}
