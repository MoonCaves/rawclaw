package agentproto

import (
	"bytes"
	"strings"
	"testing"
)

// TestOutlineHeaderIsMarkedUTC pins the time-rendering policy for the outline
// header (an agent-parsed surface): the session instant renders as marked-UTC
// RFC3339 ("…Z"), never an unmarked local time. The fixture's transcript
// timestamp is 2026-06-01T10:00:00Z, so the header must show exactly that
// instant with the Z marker regardless of the host zone.
func TestOutlineHeaderIsMarkedUTC(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)
	writeSession(t, proj, "timesess", "33333333-aaaa-bbbb-cccc-000000000003", "an opening message")

	res, err := Outline("timesess", scope, false)
	if err != nil {
		t.Fatalf("Outline: %v", err)
	}
	if res.ISO != "2026-06-01T10:00:00Z" {
		t.Errorf("Outline ISO = %q, want %q (marked UTC)", res.ISO, "2026-06-01T10:00:00Z")
	}
	var buf bytes.Buffer
	renderOutline(&buf, res)
	if !strings.Contains(buf.String(), "━━ 2026-06-01T10:00:00Z ·") {
		t.Errorf("outline header missing marked-UTC instant:\n%s", buf.String())
	}
}

// TestRenderSearchTimestampsMarkedUTC pins the policy for human search
// results: a stored transcript ISO (fractional, Z) is normalized to the
// marked-UTC seconds form by the timefmt seam.
func TestRenderSearchTimestampsMarkedUTC(t *testing.T) {
	env := SearchEnvelope{
		Results: []SearchRef{{
			Project:   "proj",
			SessionID: "abcd1234-0000-0000-0000-000000000001",
			ISO:       "2026-06-01T10:00:00.123Z",
			Snippet:   "snippet",
			ReadRef:   "abcd1234:9f3e1c20",
		}},
		Count: 1, TotalMatches: 1,
	}
	var buf bytes.Buffer
	renderSearch(&buf, env, "q", "across all projects")
	if !strings.Contains(buf.String(), "━━ 2026-06-01T10:00:00Z ·") {
		t.Errorf("search result header not normalized to marked UTC:\n%s", buf.String())
	}
}
