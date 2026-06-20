package semantic

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs this package's tests under goleak, failing on a leaked goroutine
// (the goroutine-leak class; complements -race). Test-only — never shipped.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
