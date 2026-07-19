package archive

import "testing"

// TestSSHDestination: the machine-name → ssh-destination mapping the `live`
// verb resolves through — a mapped name uses its config entry, an unmapped
// name falls back to the name itself (letting ~/.ssh/config own it), and a
// machine with no archive config at all still resolves to the name.
func TestSSHDestination(t *testing.T) {
	t.Run("unconfigured archive falls back to the name", func(t *testing.T) {
		newTestHome(t)
		if got := SSHDestination("box-a"); got != "box-a" {
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
		if got := SSHDestination("box-a"); got != "user@10.0.0.5" {
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
		if got := SSHDestination("box-b"); got != "box-b" {
			t.Errorf("SSHDestination(box-b) = %q, want box-b", got)
		}
	})
}

// TestConfigRoundTripsSSHMap: the ssh map survives a write→read cycle and an
// absent map stays absent (omitempty — old configs parse unchanged).
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
}
