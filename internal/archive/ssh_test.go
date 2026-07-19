package archive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSSHDestination: the machine-name → ssh-destination mapping the `live`
// verb resolves through — a mapped name uses its config entry, an unmapped
// name falls back to the name itself (letting ~/.ssh/config own it), and a
// machine with no archive config at all still resolves to the name.
func TestSSHDestination(t *testing.T) {
	t.Run("unconfigured archive falls back to the name", func(t *testing.T) {
		newTestHome(t)
		got, err := SSHDestination("box-a")
		if err != nil {
			t.Fatalf("SSHDestination: %v", err)
		}
		if got != "box-a" {
			t.Errorf("SSHDestination(box-a) = %q, want box-a", got)
		}
	})

	t.Run("mapped name resolves through the config", func(t *testing.T) {
		newTestHome(t)
		cfg := Config{
			Remote: "/tmp/r.git",
			Name:   "machine-a",
			SSH:    map[string]string{"box-a": "user@10.0.0.5"},
		}
		if err := writeConfig(cfg); err != nil {
			t.Fatal(err)
		}
		got, err := SSHDestination("box-a")
		if err != nil {
			t.Fatalf("SSHDestination: %v", err)
		}
		if got != "user@10.0.0.5" {
			t.Errorf("SSHDestination(box-a) = %q, want user@10.0.0.5", got)
		}
	})

	t.Run("unmapped name falls back to the name", func(t *testing.T) {
		newTestHome(t)
		cfg := Config{
			Remote: "/tmp/r.git",
			Name:   "machine-a",
			SSH:    map[string]string{"box-a": "user@10.0.0.5"},
		}
		if err := writeConfig(cfg); err != nil {
			t.Fatal(err)
		}
		got, err := SSHDestination("box-b")
		if err != nil {
			t.Fatalf("SSHDestination: %v", err)
		}
		if got != "box-b" {
			t.Errorf("SSHDestination(box-b) = %q, want box-b", got)
		}
	})

	t.Run("ssh-map-only config works without an archive", func(t *testing.T) {
		// Live peek is documented as usable WITHOUT the archive: a config
		// holding only the ssh map (no remote/name) must still resolve.
		newTestHome(t)
		if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath(),
			[]byte(`{"ssh": {"box-a": "user@10.0.0.5"}}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := SSHDestination("box-a")
		if err != nil {
			t.Fatalf("SSHDestination: %v", err)
		}
		if got != "user@10.0.0.5" {
			t.Errorf("SSHDestination(box-a) = %q, want user@10.0.0.5 (map-only config)", got)
		}
	})

	t.Run("corrupt config surfaces, never silently ignored", func(t *testing.T) {
		newTestHome(t)
		if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath(), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := SSHDestination("box-a"); err == nil {
			t.Error("corrupt config should error (a silently dropped ssh map sends the dial to the wrong host)")
		}
	})
}

// TestConfigRoundTripsSSHMap: the ssh map survives a write→read cycle, and an
// absent map stays absent (omitempty — a pre-map config parses unchanged).
func TestConfigRoundTripsSSHMap(t *testing.T) {
	newTestHome(t)
	in := Config{
		Remote: "/tmp/r.git",
		Name:   "machine-a",
		SSH:    map[string]string{"box-a": "agent@203.0.113.7"},
	}
	if err := writeConfig(in); err != nil {
		t.Fatal(err)
	}
	out, err := readConfig()
	if err != nil {
		t.Fatal(err)
	}
	if out.SSH["box-a"] != "agent@203.0.113.7" {
		t.Errorf("ssh map lost in round-trip: %+v", out.SSH)
	}

	// Absent map: a config written before the field existed round-trips to a
	// nil map and re-serializes without an "ssh" key.
	if err := writeConfig(Config{Remote: "/tmp/r.git", Name: "machine-a"}); err != nil {
		t.Fatal(err)
	}
	out, err = readConfig()
	if err != nil {
		t.Fatal(err)
	}
	if out.SSH != nil {
		t.Errorf("absent ssh map should stay nil, got %+v", out.SSH)
	}
	raw, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"ssh"`) {
		t.Errorf("omitempty violated — serialized config grew an ssh key:\n%s", raw)
	}
}
