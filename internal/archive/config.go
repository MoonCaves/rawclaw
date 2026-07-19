package archive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// Config is the archive configuration persisted in the state dir by
// `archive init` and read back by Load.
type Config struct {
	Remote string `json:"remote"` // git remote URL of the archive repository
	Name   string `json:"name"`   // this machine's top-level dir name in the repo
}

// configPath is <state-dir>/archive-config.json — beside the machine-id file,
// so one dir holds every piece of cross-session archive state.
func configPath() string {
	return filepath.Join(store.CacheDir(), "archive-config.json")
}

// cloneDir is <state-dir>/archive/clone — the managed local clone. It is a
// rebuildable artifact: deleting it only forces a re-clone on the next verb.
func cloneDir() string {
	return filepath.Join(store.CacheDir(), "archive", "clone")
}

// readConfig loads and validates the persisted config. A missing file surfaces
// as fs.ErrNotExist (Load's "feature off" signal); anything else is an error.
func readConfig() (Config, error) {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", configPath(), err)
	}
	if cfg.Remote == "" || cfg.Name == "" {
		return Config{}, fmt.Errorf("invalid archive config %s: remote and name are required", configPath())
	}
	return cfg, nil
}

// writeConfig persists cfg. One small single-shot write: a torn write cannot
// pass unnoticed — it surfaces as a parse error from Load, whose message
// points at the file.
func writeConfig(cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode archive config: %w", err)
	}
	p := configPath()
	if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}
	return nil
}
