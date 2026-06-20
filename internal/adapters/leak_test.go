package adapters

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs this package's tests under goleak, failing on a leaked goroutine
// (the goroutine-leak class; complements -race). This package does network I/O
// (http.Client), the likeliest source of an undrained-body / keep-alive leak.
// Test-only — never shipped.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
