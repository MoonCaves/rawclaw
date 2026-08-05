package agentproto

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// liveSession is the shape the current-turn tests share: a session with real
// earlier history, then the caller's just-typed prompt at the tail, loaded with
// the query word so it OUTRANKS that history — which is exactly the displacement
// being fixed.
func liveSession(t *testing.T, proj string) {
	t.Helper()
	writeRichSession(t, proj, "livesess01", "2026-06-01", []msgSpec{
		{role: "user", uuid: "11111111-aaaa-bbbb-cccc-000000000001",
			content: "back then we wrote the runbook for rolling back a bad image"},
		{role: "assistant", uuid: "11111111-aaaa-bbbb-cccc-000000000002",
			content: "the runbook pins the digest before it swaps"},
		{role: "user", uuid: "11111111-aaaa-bbbb-cccc-000000000003",
			content: "runbook runbook runbook what did we ever decide about the runbook"},
		{role: "user", uuid: "11111111-aaaa-bbbb-cccc-000000000004",
			content: "[TOOL_RESULT] runbook grep exit 0"},
	})
}

// TestSearchWithoutCurrentSessionRanksTheJustTypedPrompt records the defect, so
// the fix below is measured against the real behavior rather than an assumption.
func TestSearchWithoutCurrentSessionRanksTheJustTypedPrompt(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)
	liveSession(t, proj)

	env := Search("runbook", scope, SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(env.Results))
	}
	if !strings.Contains(env.Results[0].Snippet, "what did we ever decide") {
		t.Fatalf("expected the just-typed prompt to win the slot, got %q", env.Results[0].Snippet)
	}
	if env.ExcludedCurrentTurn != 0 {
		t.Errorf("ExcludedCurrentTurn = %d with no caller session, want 0", env.ExcludedCurrentTurn)
	}
}

// TestSearchExcludesCurrentTurn: told where it is, search must not hand the
// caller back the prompt it just typed — and the slot must go to a real result
// instead of being lost.
func TestSearchExcludesCurrentTurn(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)
	liveSession(t, proj)

	env := Search("runbook", scope, SearchOpts{CurrentSession: "livesess01"}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("expected the conversation to survive, got %d results", len(env.Results))
	}
	if strings.Contains(env.Results[0].Snippet, "what did we ever decide") {
		t.Fatalf("the just-typed prompt came back as a result: %q", env.Results[0].Snippet)
	}
	if env.ExcludedCurrentTurn == 0 {
		t.Errorf("ExcludedCurrentTurn = 0, want the withheld count reported")
	}
}

// TestCurrentTurnKeepsEarlierHistoryOfSameSession is the scoping constraint,
// asserted on its own because it is the thing most likely to be widened by
// accident: in a long session the EARLIER parts of that same session are
// legitimate history — often the most relevant history there is — and must stay
// searchable. Only the turn in flight goes.
func TestCurrentTurnKeepsEarlierHistoryOfSameSession(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)
	liveSession(t, proj)

	env := Search("runbook", scope, SearchOpts{CurrentSession: "livesess01"}, nil)
	if len(env.Results) == 0 {
		t.Fatal("the live session's earlier history was excluded — the exclusion is too wide")
	}
	got := env.Results[0].Snippet
	if !strings.Contains(got, "rolling back a bad image") && !strings.Contains(got, "pins the digest") {
		t.Errorf("expected an earlier message of the live session, got %q", got)
	}
	// The read-ref must point INTO that session, not at some other conversation.
	if !strings.HasPrefix(env.Results[0].ReadRef, "livesess") {
		t.Errorf("ReadRef = %q, want a ref into the live session", env.Results[0].ReadRef)
	}
}

// TestCurrentTurnLeavesOtherSessionsAlone: the exclusion is keyed on one
// session. A different conversation that happens to be the caller's best match
// is untouched.
func TestCurrentTurnLeavesOtherSessionsAlone(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)
	liveSession(t, proj)
	writeRichSession(t, proj, "othersess1", "2026-05-01", []msgSpec{
		{role: "user", uuid: "22222222-aaaa-bbbb-cccc-000000000001",
			content: "runbook runbook runbook a different conversation entirely"},
	})

	env := Search("runbook", scope, SearchOpts{CurrentSession: "livesess01"}, nil)
	found := false
	for _, r := range env.Results {
		if strings.HasPrefix(r.ReadRef, "otherses:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the other conversation was dropped too: %+v", env.Results)
	}
}

// TestCurrentTurnAcceptsASessionPrefix: agents copy the 8-char id off the hit
// line, so the flag must take that as readily as a full session id.
func TestCurrentTurnAcceptsASessionPrefix(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)
	liveSession(t, proj)

	env := Search("runbook", scope, SearchOpts{CurrentSession: "livesess"}, nil)
	if env.ExcludedCurrentTurn == 0 {
		t.Fatalf("an 8-char prefix did not resolve the caller's session")
	}
	if strings.Contains(env.Results[0].Snippet, "what did we ever decide") {
		t.Fatalf("the just-typed prompt came back: %q", env.Results[0].Snippet)
	}
}

// TestIsCurrentSession covers the matching rule directly, including the case the
// prefix form must refuse: a subagent thread shares its parent's entire id but is
// a different conversation, never the caller's own turn.
func TestIsCurrentSession(t *testing.T) {
	const parent = "11111111-aaaa-bbbb-cccc-000000000001"
	tests := []struct {
		name    string
		candSID string
		arg     string
		want    bool
	}{
		{"exact", parent, parent, true},
		{"8-char prefix", parent, "11111111", true},
		{"short prefix refused", parent, "1111", false},
		{"different session", parent, "22222222", false},
		{"subagent of the caller", parent + "/agent-x", parent, false},
		{"subagent by prefix", parent + "/agent-x", "11111111", false},
		{"exact subagent ref", parent + "/agent-x", parent + "/agent-x", true},
	}
	for _, tc := range tests {
		if got := isCurrentSession(tc.candSID, tc.arg); got != tc.want {
			t.Errorf("%s: isCurrentSession(%q, %q) = %v, want %v", tc.name, tc.candSID, tc.arg, got, tc.want)
		}
	}
}

// TestCurrentTurnStartSkipsThisTurnsMachinery: the boundary is the newest thing a
// PERSON typed, so this turn's tool results, injected envelopes and the runtime's
// interruption marker are all stepped over on the way back to the prompt — and
// then withheld along with it, because they are newer.
func TestCurrentTurnStartSkipsThisTurnsMachinery(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeRichSession(t, proj, "machsess01", "2026-06-01", []msgSpec{
		{role: "user", uuid: "33333333-aaaa-bbbb-cccc-000000000001",
			content: "old history about the runbook"},
		{role: "user", uuid: "33333333-aaaa-bbbb-cccc-000000000002",
			content: "the prompt I just typed about the runbook"},
		{role: "assistant", uuid: "33333333-aaaa-bbbb-cccc-000000000003",
			content: "[THINKING] runbook, let me search"},
		{role: "user", uuid: "33333333-aaaa-bbbb-cccc-000000000004",
			content: "[TOOL_RESULT] runbook exit 0"},
		{role: "user", uuid: "33333333-aaaa-bbbb-cccc-000000000005",
			content: "<system-reminder>runbook</system-reminder>"},
		{role: "user", uuid: "33333333-aaaa-bbbb-cccc-000000000006",
			content: "[Request interrupted by user for tool use]"},
	})
	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}

	// --include-tools puts the machinery back in the haystack, so if the boundary
	// landed on any of it those records would come back.
	env := Search("runbook", scope, SearchOpts{
		CurrentSession: "machsess01", IncludeTools: true,
	}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(env.Results))
	}
	if !strings.Contains(env.Results[0].Snippet, "old history") {
		t.Fatalf("expected the pre-turn history, got %q", env.Results[0].Snippet)
	}
	// The prompt plus everything after it that was a CANDIDATE: the prompt,
	// [THINKING], [TOOL_RESULT], <system-reminder>. The interruption marker holds
	// no query term, so it never entered the pool to be withheld from.
	if env.ExcludedCurrentTurn != 4 {
		t.Errorf("ExcludedCurrentTurn = %d, want 4 (the prompt and this turn's matching machinery)",
			env.ExcludedCurrentTurn)
	}
}

// deepMachineryTail is how many machinery records sit between the caller's prompt
// and the end of the session in the test below. It is deliberately far past any
// window a "scan back from the tail" implementation would plausibly use: the
// first version of this feature scanned 40, and 40 was already too small against
// the live corpus, where the newest human-typed record measured 65 back.
const deepMachineryTail = 120

// TestCurrentTurnFindsAPromptBuriedUnderADeepTail is the regression test for the
// way this feature first shipped BROKEN, and it is written to fail against that
// implementation rather than to describe the fix.
//
// The original currentTurnStart walked back a fixed 40 records from the tail
// looking for the boundary. Every test above passes under that implementation,
// because every session they build is a handful of records long — so the suite
// was green while the feature did nothing at all on a real session. Live, roughly
// 85% of role=user rows are machinery, so a busy turn buries its own prompt far
// deeper than 40; the scan found nothing, returned 0, and 0 silently means "do
// not exclude".
//
// A fixed window is a guess about a ratio that varies per turn, so the property
// asserted here is that there is no window: the prompt is found however deep the
// tail is. Bumping the constant must not be able to make this pass again.
func TestCurrentTurnFindsAPromptBuriedUnderADeepTail(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	msgs := []msgSpec{
		{role: "user", uuid: "44444444-aaaa-bbbb-cccc-000000000001",
			content: "old history about the runbook"},
		// Loaded with the query term so it OUTRANKS the history above, the same way
		// liveSession does — a prompt that loses on relevance would never reach the
		// top slot, and then the exclusion would have nothing to prove.
		{role: "user", uuid: "44444444-aaaa-bbbb-cccc-000000000002",
			content: "runbook runbook runbook the prompt I just typed about the runbook"},
	}
	// Bury the prompt under a tail of this turn's own machinery. Alternating tool
	// results and injected envelopes is what a real working turn writes, and none
	// of it is something a person typed.
	for i := 0; i < deepMachineryTail; i++ {
		body := fmt.Sprintf("[TOOL_RESULT] runbook step %d exit 0", i)
		if i%2 == 1 {
			body = fmt.Sprintf("<system-reminder>runbook note %d</system-reminder>", i)
		}
		msgs = append(msgs, msgSpec{
			role:    "user",
			uuid:    fmt.Sprintf("44444444-aaaa-bbbb-cccc-%012d", i+3),
			content: body,
		})
	}
	writeRichSession(t, proj, "deepsess01", "2026-06-01", msgs)
	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}

	env := Search("runbook", scope, SearchOpts{CurrentSession: "deepsess01"}, nil)

	if env.ExcludedCurrentTurn == 0 {
		t.Fatalf("ExcludedCurrentTurn = 0 — the prompt sits %d records from the tail and was "+
			"not found, so the exclusion is inert (this is the shipped-broken behavior)",
			deepMachineryTail)
	}
	// The exclusion has to be real, not just counted: the prompt itself must be
	// gone from the results.
	for _, r := range env.Results {
		if strings.Contains(r.Snippet, "the prompt I just typed") {
			t.Errorf("the just-typed prompt came back in the results: %q", r.Snippet)
		}
	}
	// ...and the scoping constraint still holds at depth: earlier history of the
	// same session is not collateral damage.
	var keptHistory bool
	for _, r := range env.Results {
		if strings.Contains(r.Snippet, "old history") {
			keptHistory = true
		}
	}
	if !keptHistory {
		t.Errorf("the session's earlier history was withheld too; only the turn in flight should go")
	}
}

// TestEmptyResultSaysWhatItWithheld: when the only matches were the caller's own
// turn, "No matches" plus the standard rephrase advice is wrong twice over. The
// query DID match, so telling the caller to rewrite it sends them after a wording
// problem that does not exist — and reporting nothing over a set we chose not to
// return is a lie of omission. The empty path has to say what happened and name
// the flag that shows the withheld rows.
func TestEmptyResultSaysWhatItWithheld(t *testing.T) {
	var b strings.Builder
	renderSearch(&b, SearchEnvelope{
		Results:             nil,
		ExcludedCurrentTurn: 2,
	}, "runbook", "across all projects")
	got := b.String()

	if strings.Contains(got, "or rephrase") {
		t.Errorf("empty-with-withheld printed the rephrase advice, which is wrong here:\n%s", got)
	}
	for _, want := range []string{"turn you are in now", "--current-session off"} {
		if !strings.Contains(got, want) {
			t.Errorf("empty-with-withheld output missing %q:\n%s", want, got)
		}
	}

	// The ordinary empty case is untouched — it still gets the rephrase advice,
	// because there a wording problem is the likely cause.
	var plain strings.Builder
	renderSearch(&plain, SearchEnvelope{Results: nil}, "runbook", "across all projects")
	if !strings.Contains(plain.String(), "or rephrase") {
		t.Errorf("a genuinely empty result lost its rephrase advice:\n%s", plain.String())
	}
}
