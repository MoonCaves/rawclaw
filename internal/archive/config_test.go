package archive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteConfig_ReplacesInsteadOfTruncating: the config write must be a
// replace (build the new bytes beside the file, then swap), never an in-place
// truncate — a process killed mid-truncate-write leaves a half-written config
// that Load rejects on every later run. os.SameFile is the observable: a
// truncate keeps the old file's identity, a swap does not.
func TestWriteConfig_ReplacesInsteadOfTruncating(t *testing.T) {
	newTestHome(t)

	if err := writeConfig(Config{Remote: "/tmp/r.git", Name: "machine-a"}); err != nil {
		t.Fatalf("first writeConfig: %v", err)
	}
	before, err := os.Stat(configPath())
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}

	if err := writeConfig(Config{Remote: "/tmp/r.git", Name: "machine-b"}); err != nil {
		t.Fatalf("second writeConfig: %v", err)
	}
	after, err := os.Stat(configPath())
	if err != nil {
		t.Fatalf("stat rewritten config: %v", err)
	}

	if os.SameFile(before, after) {
		t.Error("writeConfig truncated the config in place — a kill mid-write can leave a torn config; it must swap in a fully-written replacement")
	}

	got, err := readConfig()
	if err != nil {
		t.Fatalf("readConfig after rewrite: %v", err)
	}
	if got.Name != "machine-b" {
		t.Errorf("rewritten config Name = %q, want machine-b", got.Name)
	}
}

// TestWriteConfig_LeavesNoScratchFiles: the swap must not strand its scratch
// file beside the config — the state dir stays clean after every write.
func TestWriteConfig_LeavesNoScratchFiles(t *testing.T) {
	newTestHome(t)

	for _, name := range []string{"machine-a", "machine-b"} {
		if err := writeConfig(Config{Remote: "/tmp/r.git", Name: name}); err != nil {
			t.Fatalf("writeConfig(%s): %v", name, err)
		}
	}

	dir := filepath.Dir(configPath())
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(configPath()) && strings.Contains(e.Name(), "archive-config") {
			t.Errorf("stray scratch file %q left beside the config", e.Name())
		}
	}
}
