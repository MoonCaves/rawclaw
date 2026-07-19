package archive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// manifestName is the per-machine manifest file at the root of a machine dir
// in the archive repo. It maps the human-readable dir name to the stable
// machine_id, so identity survives hostname changes and the compound row key
// (origin_machine, source_tool, session_id) stays collision-free even if two
// machines ever claim the same display name.
const manifestName = ".rawclaw-machine.json"

// manifest is the registration record committed inside a machine dir.
type manifest struct {
	MachineID string `json:"machine_id"` // stable self-minted id (identity)
	Name      string `json:"name"`       // human-readable dir name (display)
	Hostname  string `json:"hostname"`   // hostname at registration (informational)
	UpdatedAt string `json:"updated_at"` // RFC3339 UTC of the last manifest write
}

// readManifest loads the manifest from a machine dir. A missing file surfaces
// as fs.ErrNotExist (the "dir unclaimed" signal).
func readManifest(machineDir string) (manifest, error) {
	p := filepath.Join(machineDir, manifestName)
	b, err := os.ReadFile(p)
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return manifest{}, fmt.Errorf("parse %s: %w", p, err)
	}
	return m, nil
}

// writeManifest writes the manifest into a machine dir, creating the dir.
func writeManifest(machineDir string, m manifest) error {
	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		return fmt.Errorf("create machine dir: %w", err)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	p := filepath.Join(machineDir, manifestName)
	if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}
	return nil
}
