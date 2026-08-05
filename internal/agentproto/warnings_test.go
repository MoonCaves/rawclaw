package agentproto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// codes lists the warning codes in a built slice, in order — the shape most of
// these assertions are about.
func codes(ws []Warning) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

// hasCode reports whether code is present.
func hasCode(ws []Warning, code string) bool { return findWarning(ws, code) != nil }

// oneResult is the minimum result set that counts as "a search returned
// something", used by the cases whose subject is a warning other than the
// result set itself.
var oneResult = []SearchRef{{Project: "p", SessionID: "aaaa", ISO: "2026-06-18T10:00:00Z", ReadRef: "aaaa:9f"}}

// TestCleanSearchCarriesOnlyTheStandingCaveat is the case the whole ticket is
// for: a narrow query with clean hits from one complete project should not
// arrive wearing paragraphs of coaching. The only entry it earns is the standing
// reminder that these are raw transcripts.
func TestCleanSearchCarriesOnlyTheStandingCaveat(t *testing.T) {
	ws := buildWarnings(warningInputs{
		results: oneResult,
		reports: []ScopeReport{{Project: "p", Status: ScopeSearched}},
		total:   1,
	})
	if got := codes(ws); len(got) != 1 || got[0] != WarnRawHistory {
		t.Fatalf("clean search carried %v, want just [%s]", got, WarnRawHistory)
	}
}

// TestEachWarningFiresOnlyOnItsCondition drives one condition at a time and
// asserts the warning it should produce AND that it produced no other advisory.
// Firing on the right input is half the contract; not firing on everything else
// is the half that makes the output worth reading.
func TestEachWarningFiresOnlyOnItsCondition(t *testing.T) {
	clean := warningInputs{
		results: oneResult,
		reports: []ScopeReport{{Project: "p", Status: ScopeSearched}},
		total:   1,
	}

	tests := []struct {
		name string
		in   func(warningInputs) warningInputs
		want string
	}{
		{
			name: "recency skew: freshest match well newer than the top hit",
			in: func(in warningInputs) warningInputs {
				in.newestISO = "2026-08-01T10:00:00Z" // top hit is 2026-06-18
				return in
			},
			want: WarnRecencySkew,
		},
		{
			name: "broad query: many distinct matches",
			in: func(in warningInputs) warningInputs {
				in.total = broadQueryMatches
				return in
			},
			want: WarnBroadQuery,
		},
		{
			name: "broad query: the fetch ceiling was hit",
			in: func(in warningInputs) warningInputs {
				in.hitCeiling = true
				return in
			},
			want: WarnBroadQuery,
		},
		{
			name: "current turn: candidates withheld",
			in: func(in warningInputs) warningInputs {
				in.droppedTurn = 3
				return in
			},
			want: WarnCurrentTurnExcluded,
		},
		{
			name: "scope incomplete: a project failed to index",
			in: func(in warningInputs) warningInputs {
				in.reports = append(in.reports, ScopeReport{Project: "q", Status: ScopeSkippedError})
				return in
			},
			want: WarnScopeIncomplete,
		},
		{
			name: "scope incomplete: a project was served stale",
			in: func(in warningInputs) warningInputs {
				in.reports = append(in.reports, ScopeReport{Project: "q", Status: ScopeStaleFallback})
				return in
			},
			want: WarnScopeIncomplete,
		},
		{
			name: "project spread: hits cross a project boundary",
			in: func(in warningInputs) warningInputs {
				in.results = append(in.results, SearchRef{Project: "q", SessionID: "bbbb", ISO: "2026-06-18T10:00:00Z"})
				return in
			},
			want: WarnProjectSpread,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := buildWarnings(tc.in(clean))
			if !hasCode(ws, tc.want) {
				t.Fatalf("condition did not raise %s; got %v", tc.want, codes(ws))
			}
			// Exactly the triggered warning plus the standing caveat — nothing else
			// rode along on an unrelated condition.
			if got := codes(ws); len(got) != 2 {
				t.Errorf("raised %v, want only [%s %s]", got, tc.want, WarnRawHistory)
			}
		})
	}
}

// TestIncompletenessIsUnconditional: every other warning is a hint an agent may
// ignore. This one is a correctness claim — a result set that silently omits a
// project reads as complete, and acting on it is acting on a lie. It fires no
// matter what else is true of the search, including when there are no results at
// all to attach it to.
func TestIncompletenessIsUnconditional(t *testing.T) {
	broken := []ScopeReport{
		{Project: "p", Status: ScopeSearched},
		{Project: "q", Status: ScopeSkippedError, Detail: "boom"},
	}

	cases := map[string]warningInputs{
		"with results":    {results: oneResult, reports: broken, total: 1},
		"with no results": {results: nil, reports: broken},
		"sorted by newest, which suppresses the recency warning": {
			results: oneResult, reports: broken, sort: "newest", newestISO: "2026-08-01T10:00:00Z",
		},
		"already broad and already excluding the current turn": {
			results: oneResult, reports: broken, total: 500, hitCeiling: true, droppedTurn: 9,
		},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if ws := buildWarnings(in); !hasCode(ws, WarnScopeIncomplete) {
				t.Fatalf("incompleteness was dropped; got %v", codes(ws))
			}
		})
	}
}

// TestRecencyWarningRespectsAnExplicitSort: the warning exists to offer --sort
// newest. Telling a caller who already passed it to pass it is noise.
func TestRecencyWarningRespectsAnExplicitSort(t *testing.T) {
	in := warningInputs{
		results:   oneResult,
		reports:   []ScopeReport{{Project: "p", Status: ScopeSearched}},
		newestISO: "2026-08-01T10:00:00Z",
		total:     1,
	}
	if ws := buildWarnings(in); !hasCode(ws, WarnRecencySkew) {
		t.Fatalf("default order did not raise %s: %v", WarnRecencySkew, codes(ws))
	}
	in.sort = "newest"
	if ws := buildWarnings(in); hasCode(ws, WarnRecencySkew) {
		t.Errorf("--sort newest still got told to add --sort newest: %v", codes(ws))
	}
}

// TestNoResultsCarriesNoFreshnessCaveat: the standing caveat is about the
// history that came back. With nothing returned there is nothing to verify
// against current state, and printing the reminder anyway would be filler.
func TestNoResultsCarriesNoFreshnessCaveat(t *testing.T) {
	ws := buildWarnings(warningInputs{
		reports: []ScopeReport{{Project: "p", Status: ScopeSearched}},
	})
	if hasCode(ws, WarnRawHistory) {
		t.Errorf("empty result set still carried the freshness caveat: %v", codes(ws))
	}
}

// TestWarningOrderPutsActionFirstAndCaveatLast locks the emitted order, because
// it IS the reading order in the terminal: what changes the next command comes
// first, what qualifies the set comes next, and the standing caveat closes.
func TestWarningOrderPutsActionFirstAndCaveatLast(t *testing.T) {
	ws := buildWarnings(warningInputs{
		results: append(oneResult, SearchRef{Project: "q", SessionID: "bbbb", ISO: "2026-06-18T10:00:00Z"}),
		reports: []ScopeReport{
			{Project: "p", Status: ScopeSearched},
			{Project: "q", Status: ScopeStaleFallback},
		},
		newestISO:   "2026-08-01T10:00:00Z",
		total:       99,
		hitCeiling:  true,
		droppedTurn: 2,
	})

	want := []string{
		WarnRecencySkew, WarnBroadQuery, WarnCurrentTurnExcluded,
		WarnScopeIncomplete, WarnProjectSpread, WarnRawHistory,
	}
	got := codes(ws)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("warning order = %v, want %v", got, want)
	}
}

// TestTextAndJSONCarryTheSameWarnings is the anti-drift assertion. The text
// footer used to be assembled from conditions the renderer evaluated itself,
// which is how --json came to report a withheld count while the text said "no
// matches". Now there is one source: the renderer prints the envelope's
// warnings, so the two surfaces cannot disagree without this failing.
func TestTextAndJSONCarryTheSameWarnings(t *testing.T) {
	env := SearchEnvelope{
		Results: oneResult,
		Scopes: []ScopeReport{
			{Project: "p", Status: ScopeSearched},
			{Project: "q", Status: ScopeSkippedError},
		},
	}
	env.Warnings = buildWarnings(warningInputs{
		results:     env.Results,
		reports:     env.Scopes,
		newestISO:   "2026-08-01T10:00:00Z",
		total:       1,
		droppedTurn: 4,
	})

	var buf bytes.Buffer
	renderSearch(&buf, env, "kw", "across all projects")

	var noteLines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "note: ") {
			noteLines = append(noteLines, strings.TrimPrefix(line, "note: "))
		}
	}

	// Round-trip the envelope so the comparison is against what a --json caller
	// actually receives, not against the in-memory struct.
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var decoded SearchEnvelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if len(noteLines) != len(decoded.Warnings) {
		t.Fatalf("text printed %d notes, json carries %d warnings:\n%s", len(noteLines), len(decoded.Warnings), buf.String())
	}
	for i, w := range decoded.Warnings {
		if noteLines[i] != w.Message {
			t.Errorf("note %d: text %q, json %q", i, noteLines[i], w.Message)
		}
		if w.Code == "" {
			t.Errorf("warning %d (%q) has no code — an agent has nothing to branch on", i, w.Message)
		}
	}
}

// TestEmptyResultDoesNotRepeatTheCurrentTurnWarning: on the empty path the
// exclusion is the headline, stated in full. Printing it again underneath as a
// footnote reads as two separate findings about two separate things.
func TestEmptyResultDoesNotRepeatTheCurrentTurnWarning(t *testing.T) {
	env := SearchEnvelope{ExcludedCurrentTurn: 2}
	env.Warnings = buildWarnings(warningInputs{droppedTurn: 2})

	var buf bytes.Buffer
	renderSearch(&buf, env, "kw", "across all projects")
	out := buf.String()

	if n := strings.Count(out, "turn you are in now"); n != 1 {
		t.Errorf("the exclusion was stated %d times, want once:\n%s", n, out)
	}
	if !strings.Contains(out, "--current-session off") {
		t.Errorf("the empty path did not name the flag that shows the withheld rows:\n%s", out)
	}
}

// TestFreshnessNoteHasNoBakedPrefix: the constant is a warning Message now and a
// directly-printed line in the read renderer. If it carried its own "note: " the
// search path would print "note: note: ...".
func TestFreshnessNoteHasNoBakedPrefix(t *testing.T) {
	if strings.HasPrefix(freshnessNote, "note:") {
		t.Fatalf("freshnessNote carries its own prefix: %q", freshnessNote)
	}
	var buf bytes.Buffer
	renderSearch(&buf, SearchEnvelope{
		Results:  oneResult,
		Warnings: buildWarnings(warningInputs{results: oneResult, total: 1}),
	}, "kw", "x")
	if strings.Contains(buf.String(), "note: note:") {
		t.Errorf("doubled prefix in output:\n%s", buf.String())
	}
}
