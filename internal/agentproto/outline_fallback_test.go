package agentproto

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// TestOutline_UntaggedSessionTailFallback tests that an untagged session
// (len(Topics) == 0) surfaces the newest human directive when buried behind
// intermediate messages and machine outputs.
func TestOutline_UntaggedSessionTailFallback(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	sid := "untagged-session-123"
	// Write opening message
	writeSession(t, proj, sid, "11111111-aaaa-bbbb-cccc-000000000001", "Goal: initial opening command")

	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	dbp, fullSID, _, err := locateSession(scope, nil, sid)
	if err != nil {
		t.Fatalf("locateSession: %v", err)
	}

	con := openCacheRW(t, dbp)
	defer con.Close()

	// Insert enough intermediate messages so that opening window (cap 4) doesn't capture the later directive
	for i := 1; i <= 6; i++ {
		_, err = con.Exec(`INSERT INTO messages (session_id, role, content, uuid, ts) VALUES (?, 'assistant', ?, ?, ?)`,
			fullSID, fmt.Sprintf("intermediate step %d", i), fmt.Sprintf("uuid-mid-%d", i), 100+i)
		if err != nil {
			t.Fatalf("insert mid: %v", err)
		}
	}

	// Insert intermediate turns past the 4-message start bookend so the human directive is buried
	_, err = con.Exec(`INSERT INTO messages (session_id, role, content, uuid, ts) VALUES
		(?, 'assistant', 'working on initial command 1', 'uuid-ast-1', 100),
		(?, 'assistant', 'working on initial command 2', 'uuid-ast-1b', 110),
		(?, 'assistant', 'working on initial command 3', 'uuid-ast-1c', 120),
		(?, 'assistant', 'working on initial command 4', 'uuid-ast-1d', 130),
		(?, 'user', 'Wait, please change direction and refactor the auth middleware instead', 'uuid-hum-2', 200),
		(?, 'assistant', 'Refactoring auth middleware now...', 'uuid-ast-2', 300),
		(?, 'user', '[TOOL_RESULT] compilation succeeded', 'uuid-tool-1', 400),
		(?, 'assistant', 'All tests passing', 'uuid-ast-3', 500)`,
		fullSID, fullSID, fullSID, fullSID, fullSID, fullSID, fullSID, fullSID)
	if err != nil {
		t.Fatalf("insert messages: %v", err)
	}

	res, err := Outline(sid, scope, false)
	if err != nil {
		t.Fatalf("Outline: %v", err)
	}

	if len(res.Topics) != 0 {
		t.Fatalf("res.Topics expected empty, got %v", res.Topics)
	}

	if res.LastHumanDirective == nil {
		t.Fatalf("res.LastHumanDirective expected non-nil for untagged session with human turn")
	}

	if !strings.Contains(res.LastHumanDirective.Text, "refactor the auth middleware") {
		t.Errorf("res.LastHumanDirective.Text = %q, want containing 'refactor the auth middleware'", res.LastHumanDirective.Text)
	}

	var buf bytes.Buffer
	renderOutline(&buf, res)
	out := buf.String()

	if !strings.Contains(out, "── LAST HUMAN INSTRUCTION ──") {
		t.Errorf("renderOutline missing '── LAST HUMAN INSTRUCTION ──', got:\n%s", out)
	}
	if !strings.Contains(out, "refactor the auth middleware") {
		t.Errorf("renderOutline missing directive text, got:\n%s", out)
	}
}

// TestOutline_TaggedSessionSkipsFallback confirms that when topics are present,
// LastHumanDirective remains nil and the topic outline is preserved.
func TestOutline_TaggedSessionSkipsFallback(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	sid := "tagged-session-456"
	writeSession(t, proj, sid, "22222222-aaaa-bbbb-cccc-000000000002", "Goal: initial opening command")

	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	scope := []view.Scope{{Project: paths.ProjectLabel(proj), TDir: proj}}
	dbp, fullSID, _, err := locateSession(scope, nil, sid)
	if err != nil {
		t.Fatalf("locateSession: %v", err)
	}

	con := openCacheRW(t, dbp)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, fullSID,
		"22222222-aaaa-bbbb-cccc-000000000002", "", "Milestone A", "Summary A", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}
	con.Close()

	res, err := Outline(sid, scope, false)
	if err != nil {
		t.Fatalf("Outline: %v", err)
	}

	if len(res.Topics) != 1 || res.Topics[0] != "Milestone A" {
		t.Fatalf("res.Topics = %v, want [Milestone A]", res.Topics)
	}

	if res.LastHumanDirective != nil {
		t.Fatalf("res.LastHumanDirective should be nil when topics exist, got %v", res.LastHumanDirective)
	}

	var buf bytes.Buffer
	renderOutline(&buf, res)
	out := buf.String()

	if strings.Contains(out, "── LAST HUMAN INSTRUCTION ──") {
		t.Errorf("renderOutline should NOT contain '── LAST HUMAN INSTRUCTION ──' when topics exist, got:\n%s", out)
	}
	if !strings.Contains(out, "topics: Milestone A") {
		t.Errorf("renderOutline missing 'topics: Milestone A', got:\n%s", out)
	}
}
