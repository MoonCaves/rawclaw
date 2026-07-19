package cli

import (
	"os"
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs this package's tests under goleak, failing on a leaked goroutine
// (the goroutine-leak class; complements -race). Test-only — never shipped.
//
// The package-wide autosync kill switch guards every test that drives a
// successful search/read/outline through the real command tree: on a dev
// machine with a real archive configured, the trigger would otherwise exec
// the TEST BINARY as a detached sync child against the real state dir.
// Autosync's own tests re-enable per-test via t.Setenv.
func TestMain(m *testing.M) {
	os.Setenv("RAWCLAW_ARCHIVE_AUTOSYNC", "off")
	goleak.VerifyTestMain(m)
}
