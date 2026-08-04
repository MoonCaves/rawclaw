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
// results: a stored transcript ISO (fractional, Z) is normalized by the timefmt
// seam to a UTC stamp that always carries its marker.
//
// The rendered shape changed when the hit header gained the topic label: search
// now uses the COMPACT marked-UTC form ("2026-06-01 10:00Z") instead of the full
// RFC3339 instant, trading the seconds for line width. The policy itself did not
// change and is asserted in two parts below — the instant is UTC (not the host's
// local time) and it is marked. Outline keeps the full instant; see
// TestOutlineHeaderIsMarkedUTC.
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
	got := buf.String()
	// Part 1: the instant is the UTC one, rendered compactly.
	if !strings.Contains(got, "━━ 2026-06-01 10:00Z ·") {
		t.Errorf("search header not the compact marked-UTC instant:\n%s", got)
	}
	// Part 2: no unmarked stamp. A bare "2026-06-01 10:00 " with no Z would be
	// the ambiguity this policy exists to prevent — catch it explicitly rather
	// than relying on the positive match above.
	if strings.Contains(got, "2026-06-01 10:00 ") {
		t.Errorf("search header carries an unmarked timestamp:\n%s", got)
	}
}
