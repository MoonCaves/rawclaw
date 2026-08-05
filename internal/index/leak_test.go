package index

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs this package's tests under goleak, failing on a leaked goroutine
// (the goroutine-leak class; complements -race). Test-only — never shipped.
//
// It also points HOME (and the XDG data dir under it) at a scratch directory
// first. The ingest paths under test write rawclaw's OWN state into the user's
// home — the consolidated store under ~/.cache, the durable transcript vault
// under ~/.local/share — and a test run must never leave fixture sessions
// sitting in the real one. goleak.VerifyTestMain is inlined rather than called,
// because it exits the process and a deferred cleanup would never run.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "rawclaw-index-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "scratch home:", err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	code := m.Run()
	leakErr := goleak.Find()
	os.RemoveAll(home)
	if leakErr != nil {
		fmt.Fprintln(os.Stderr, "goleak:", leakErr)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
