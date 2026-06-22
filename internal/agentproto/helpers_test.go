package agentproto

import (
	"testing"

	"github.com/MoonCaves/rawclaw/internal/retrieve"
)

// anchorLite is a compact spec for building retrieve.Anchor values in sort tests.
type anchorLite struct {
	sid   string
	iso   string
	cov   int
	rank  int
	fused float64
}

// anchors expands a slice of anchorLite specs into retrieve.Anchor values.
func anchors(specs []anchorLite) []retrieve.Anchor {
	out := make([]retrieve.Anchor, 0, len(specs))
	for _, s := range specs {
		out = append(out, retrieve.Anchor{
			SessionID: s.sid,
			ISO:       s.iso,
			Cov:       s.cov,
			Rank:      s.rank,
			Fused:     s.fused,
		})
	}
	return out
}

// sidsOf extracts the session ids from a candidate slice (post-sort order).
func sidsOf(cands []retrieve.Anchor) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.SessionID
	}
	return out
}

// assertOrder fails the test if got != want element-wise.
func assertOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order length = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
