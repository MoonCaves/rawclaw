package agentproto

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// TestSearchHitCarriesLastActivity is the contract: a hit is a point in the
// MIDDLE of a conversation, so it must also say where that conversation ended
// up. The match here is the session's opening; its real conclusion is four
// messages later, behind the machinery a busy tail collects.
func TestSearchHitCarriesLastActivity(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)
	writeRichSession(t, proj, "sesslate", "2026-06-01", []msgSpec{
		{role: "user", uuid: "11111111-aaaa-bbbb-cccc-000000000001",
			content: "start by drafting the deployment rollback runbook"},
		{role: "assistant", uuid: "11111111-aaaa-bbbb-cccc-000000000002",
			content: "rollback now runs from the pinned digest"},
		{role: "user", uuid: "11111111-aaaa-bbbb-cccc-000000000003",
			content: "[TOOL_RESULT] exit 0"},
		{role: "user", uuid: "11111111-aaaa-bbbb-cccc-000000000004",
			content: "<system-reminder>keep going</system-reminder>"},
	})

	env := Search("runbook", scope, SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(env.Results))
	}
	r := env.Results[0]
	if !strings.Contains(r.Snippet, "runbook") {
		t.Fatalf("snippet %q should hold the match", r.Snippet)
	}
	if !strings.Contains(r.Last, "pinned digest") {
		t.Errorf("Last = %q, want the session's newest real message", r.Last)
	}
	for _, banned := range []string{"TOOL_RESULT", "system-reminder"} {
		if strings.Contains(r.Last, banned) {
			t.Errorf("Last = %q leaked machinery %q", r.Last, banned)
		}
	}

	// The line renders under the snippet, above the read-ref, in browse's "now →"
	// vocabulary — one hit reads the same on both surfaces.
	var buf bytes.Buffer
	renderSearch(&buf, env, "runbook", "on this project")
	out := buf.String()
	snipAt := strings.Index(out, "…"+r.Snippet)
	nowAt := strings.Index(out, "     now → ")
	refAt := strings.Index(out, "     read ref=")
	if nowAt < 0 {
		t.Fatalf("no last-activity line in output:\n%s", out)
	}
	if !(snipAt < nowAt && nowAt < refAt) {
		t.Errorf("last-activity line out of place (snippet %d, now %d, ref %d):\n%s",
			snipAt, nowAt, refAt, out)
	}
}

// TestSearchHitLastActivityEmptyWhenTailIsAllMachinery carries ba0c430's honesty
// rule onto the hit line: when the whole scanned tail is generated material the
// hit gets NO line, rather than being captioned with a tool result.
func TestSearchHitLastActivityEmptyWhenTailIsAllMachinery(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)
	writeRichSession(t, proj, "sessnoise", "2026-06-01", []msgSpec{
		{role: "user", uuid: "22222222-aaaa-bbbb-cccc-000000000001",
			content: "draft the deployment rollback runbook"},
		{role: "user", uuid: "22222222-aaaa-bbbb-cccc-000000000002",
			content: "[TOOL_RESULT] only machinery here"},
		{role: "user", uuid: "22222222-aaaa-bbbb-cccc-000000000003",
			content: "<task-notification>agent done</task-notification>"},
		{role: "user", uuid: "22222222-aaaa-bbbb-cccc-000000000004",
			content: "[Request interrupted by user for tool use]"},
	})

	env := Search("runbook", scope, SearchOpts{}, nil)
	if len(env.Results) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(env.Results))
	}
	// The opening IS the newest real message here, so the honest answer is the
	// match itself — what must never appear is the machinery below it.
	for _, banned := range []string{"TOOL_RESULT", "task-notification", "interrupted by user"} {
		if strings.Contains(env.Results[0].Last, banned) {
			t.Errorf("Last = %q leaked machinery %q", env.Results[0].Last, banned)
		}
	}

	// And a session whose ENTIRE tail is machinery gets no line at all.
	proj2 := t.TempDir()
	scope2 := []view.Scope{{Project: paths.ProjectLabel(proj2), TDir: proj2}}
	writeRichSession(t, proj2, "sessallnoise", "2026-06-01", []msgSpec{
		{role: "user", uuid: "33333333-aaaa-bbbb-cccc-000000000001",
			content: "[TOOL_RESULT] runbook exit 0"},
	})
	env2 := Search("runbook", scope2, SearchOpts{IncludeTools: true}, nil)
	if len(env2.Results) != 1 {
		t.Fatalf("expected 1 hit with --include-tools, got %d", len(env2.Results))
	}
	if env2.Results[0].Last != "" {
		t.Errorf("Last = %q, want empty (tail is all machinery)", env2.Results[0].Last)
	}
	var buf bytes.Buffer
	renderSearch(&buf, env2, "runbook", "on this project")
	if strings.Contains(buf.String(), "now →") {
		t.Errorf("empty Last must render no line:\n%s", buf.String())
	}
}

// TestLastActivityDoesNotAffectOrdering is the design constraint asserted
// directly, the same way TestTopicLabelDoesNotAffectOrdering holds it for the
// topic label: the last-activity lookup runs AFTER ranking, keyed on results
// already selected, so it cannot change which conversations are returned or in
// what order.
//
// Two corpora, identical in everything the ranking can see. Both are searched
// with --role user, so the assistant tails the second corpus adds are outside
// the haystack entirely — they cannot be matched, only READ by the after-ranking
// lookup. The tails are adversarial: the query word saturates the tail of the
// session that matches WEAKEST, the arrangement most likely to lift it if tail
// text leaked into ranking. Session stems and uuids are shared across the two
// corpora, so the read-refs are comparable byte-for-byte.
func TestLastActivityDoesNotAffectOrdering(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	strongMsg := msgSpec{role: "user", uuid: "44444444-aaaa-bbbb-cccc-000000000001",
		content: "deployment deployment deployment notes about the deployment"}
	weakMsg := msgSpec{role: "user", uuid: "55555555-aaaa-bbbb-cccc-000000000001",
		content: "mostly unrelated chatter that mentions deployment once in passing"}

	// Corpus 1: the two matching messages, nothing else.
	base := t.TempDir()
	writeRichSession(t, base, "sessstrong", "2026-06-01", []msgSpec{strongMsg})
	writeRichSession(t, base, "sessweak", "2026-06-01", []msgSpec{weakMsg})

	// Corpus 2: the same two messages plus assistant tails — the weak session's
	// loaded with the query word, the strong session's deliberately not.
	tailed := t.TempDir()
	writeRichSession(t, tailed, "sessstrong", "2026-06-01", []msgSpec{strongMsg,
		{role: "assistant", uuid: "44444444-aaaa-bbbb-cccc-000000000002",
			content: "wrapped up and closed the thread"}})
	writeRichSession(t, tailed, "sessweak", "2026-06-01", []msgSpec{weakMsg,
		{role: "assistant", uuid: "55555555-aaaa-bbbb-cccc-000000000002",
			content: "loadedtailsentinel deployment deployment deployment deployment"}})

	refsOf := func(dir string) ([]string, []SearchRef) {
		scope := []view.Scope{{Project: paths.ProjectLabel(dir), TDir: dir}}
		env := Search("deployment", scope, SearchOpts{Role: "user"}, nil)
		refs := make([]string, len(env.Results))
		for i, r := range env.Results {
			refs[i] = r.ReadRef
		}
		return refs, env.Results
	}

	baseOrder, baseResults := refsOf(base)
	if len(baseOrder) < 2 {
		t.Fatalf("expected both sessions to match, got %d", len(baseOrder))
	}
	for i, r := range baseResults {
		if strings.Contains(r.Last, "loadedtailsentinel") {
			t.Fatalf("base result %d already carries a loaded tail %q", i, r.Last)
		}
	}

	tailedOrder, tailedResults := refsOf(tailed)
	assertOrder(t, tailedOrder, baseOrder)

	// The lookup really did run on the adversarial material — otherwise the
	// unchanged order would prove nothing.
	found := false
	for _, r := range tailedResults {
		if strings.Contains(r.Last, "loadedtailsentinel") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no result carried the loaded tail; the lookup never saw it: %+v", tailedResults)
	}
}
