package cli

import (
	"fmt"
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
//
// It also pins the XDG data dir to a scratch directory. Tests here already
// point HOME at a fixture home, but the durable transcript vault reads
// XDG_DATA_HOME first — so an inherited value from the developer's shell would
// let a delete test reach the REAL vault. goleak.VerifyTestMain is inlined
// rather than called, because it exits the process and a deferred cleanup
// would never run.
func TestMain(m *testing.M) {
	os.Setenv("RAWCLAW_ARCHIVE_AUTOSYNC", "off")
	_ = os.Unsetenv("ANTIGRAVITY_CONVERSATION_ID")
	_ = os.Unsetenv("CLAUDE_CODE_SESSION_ID")
	_ = os.Unsetenv("CODEX_SESSION_ID")

	// Same hazard, second spawner: with RAWCLAW_EMBED_ENDPOINT set, a search
	// through the real command tree fires a detached vector top-up of
	// os.Executable() — the TEST BINARY. Go's flag parsing stops at the first
	// non-flag arg, so `<exe> vector-topup --dbp X` re-runs the whole suite
	// instead of erroring, and forks again. Stub the seam package-wide; the
	// spawn-counting tests swap in their own per-test.
	spawnVectorTopup = func(string) {}

	// A test binary is not a RawClaw CLI binary. If a test accidentally reaches
	// the real detached-ingest seam, os.Executable() re-execs cli.test with the
	// `ingest` argument, which starts another full test suite and can recurse
	// while contending on the consolidated-store lock. Individual tests that
	// need to exercise spawn decisions replace this seam locally.
	spawnIngest = func(string) {}

	data, err := os.MkdirTemp("", "rawclaw-cli-data-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "scratch data dir:", err)
		os.Exit(1)
	}
	os.Setenv("XDG_DATA_HOME", data)

	code := m.Run()
	leakErr := goleak.Find()
	os.RemoveAll(data)
	if leakErr != nil {
		fmt.Fprintln(os.Stderr, "goleak:", leakErr)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
