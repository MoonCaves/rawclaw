package tagger

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs this package's tests under goleak, failing on a leaked goroutine.
// Like adapters, this package does network I/O (http.Client), so it guards
// against an undrained-body / keep-alive leak. Test-only — never shipped.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
