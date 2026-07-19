package archive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// SSHDestination resolves a machine name to the ssh destination `rawclaw live`
// dials: the config's ssh-map entry when one exists, else the name itself
// (letting ~/.ssh/config aliases carry user/port/key). Live peek works without
// a configured archive, so this reads the config file leniently — a missing
// file, or one without the archive's own remote/name fields, still resolves
// (an ssh-map-only config is valid for live). A config that EXISTS but cannot
// be parsed is an error: silently dropping a user's ssh map would dial the
// wrong destination and then blame the name.
func SSHDestination(machine string) (string, error) {
	b, err := os.ReadFile(configPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return machine, nil
		}
		return "", fmt.Errorf("read archive config %s: %w", configPath(), err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return "", fmt.Errorf("parse archive config %s (its ssh map decides where `live` dials): %w", configPath(), err)
	}
	if dest, ok := cfg.SSH[machine]; ok && dest != "" {
		return dest, nil
	}
	return machine, nil
}
