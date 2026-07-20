package claudeweb

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/source"
)

// A synthetic mini-export: three conversations. c1 is a plain text turn pair;
// c2 carries thinking + tool_use + tool_result blocks (exercising the reused
// internal/parse block handling) and a duplicate message.uuid (exercising
// within-dump dedup); c3 is out of created_at order in the file (exercising the
// created_at sort). NOT a real personal export — every value here is invented.
const fixtureConversations = `[
  {
    "uuid": "c1111111-1111-1111-1111-111111111111",
    "name": "deploy auth decision",
    "created_at": "2026-07-15T10:00:00Z",
    "updated_at": "2026-07-15T10:05:00Z",
    "chat_messages": [
      {"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","text":"where did we land on auth",
       "content":[{"type":"text","text":"where did we land on auth"}]},
      {"uuid":"m2","sender":"assistant","created_at":"2026-07-15T10:00:30Z","text":"we chose short-lived tokens",
       "content":[{"type":"text","text":"we chose short-lived tokens"}]}
    ]
  },
  {
    "uuid": "c2222222-2222-2222-2222-222222222222",
    "name": "tool session",
    "created_at": "2026-07-16T09:00:00Z",
    "updated_at": "2026-07-16T09:10:00Z",
    "chat_messages": [
      {"uuid":"m3","sender":"human","created_at":"2026-07-16T09:00:00Z",
       "content":[{"type":"text","text":"run the migration please"}]},
      {"uuid":"m4","sender":"assistant","created_at":"2026-07-16T09:00:10Z",
       "content":[
         {"type":"thinking","thinking":"the user wants a schema migration"},
         {"type":"tool_use","name":"bash","input":{"cmd":"migrate up"}},
         {"type":"text","text":"running the migration now"}
       ]},
      {"uuid":"m5","sender":"assistant","created_at":"2026-07-16T09:00:20Z",
       "content":[{"type":"tool_result","content":"migration applied cleanly"}]},
      {"uuid":"m5","sender":"assistant","created_at":"2026-07-16T09:00:20Z",
       "content":[{"type":"tool_result","content":"migration applied cleanly"}]}
    ]
  },
  {
    "uuid": "c3333333-3333-3333-3333-333333333333",
    "name": "ordering",
    "created_at": "2026-07-17T08:00:00Z",
    "updated_at": "2026-07-17T08:10:00Z",
    "chat_messages": [
      {"uuid":"m7","sender":"assistant","created_at":"2026-07-17T08:00:30Z",
       "content":[{"type":"text","text":"second"}]},
      {"uuid":"m6","sender":"human","created_at":"2026-07-17T08:00:00Z",
       "content":[{"type":"text","text":"first"}]}
    ]
  }
]`

// writeDirExport writes conversations.json into a fresh directory and returns
// the directory path (the "already-extracted export" import shape).
func writeDirExport(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, conversationsFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeZipExport writes a .zip holding conversations.json (optionally nested
// under a top-level directory, matching a real export's layout) and returns the
// zip path.
func writeZipExport(t *testing.T, body, member string) string {
	t.Helper()
	dir := t.TempDir()
	zp := filepath.Join(dir, "data-export.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(member)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return zp
}

func TestDiscoverDirExport(t *testing.T) {
	t.Parallel()
	dir := writeDirExport(t, fixtureConversations)
	got, err := New(dir).Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 containers, got %d: %+v", len(got), got)
	}
	wantIDs := map[string]bool{
		"c1111111-1111-1111-1111-111111111111": true,
		"c2222222-2222-2222-2222-222222222222": true,
		"c3333333-3333-3333-3333-333333333333": true,
	}
	for _, c := range got {
		if !wantIDs[c.ID] {
			t.Errorf("unexpected container id %q (want the bare conversation uuid, no source prefix)", c.ID)
		}
		if c.CWD != "" {
			t.Errorf("container %q CWD = %q, want empty (export drops the working dir)", c.ID, c.CWD)
		}
		if c.IsSubagent || c.ParentID != "" {
			t.Errorf("container %q must be a root session, got IsSubagent=%v ParentID=%q", c.ID, c.IsSubagent, c.ParentID)
		}
		if _, err := os.Stat(c.Path); err != nil {
			t.Errorf("container %q Path %q must be a real, stat-able file: %v", c.ID, c.Path, err)
		}
	}
}

func TestDiscoverZipExport(t *testing.T) {
	t.Parallel()
	// Nested under a top-level dir, as a real export ZIP lays it out.
	zp := writeZipExport(t, fixtureConversations, "data-export/"+conversationsFile)
	got, err := New(zp).Discover()
	if err != nil {
		t.Fatalf("Discover(zip): %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 containers from zip, got %d", len(got))
	}
	for _, c := range got {
		if c.Path != zp {
			t.Errorf("zip-backed container Path = %q, want the zip path %q", c.Path, zp)
		}
	}
}

func TestMessagesRoleMappingAndText(t *testing.T) {
	t.Parallel()
	ad := New(writeDirExport(t, fixtureConversations))
	got, err := ad.Messages(source.Container{ID: "c1111111-1111-1111-1111-111111111111"})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 messages, got %d: %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Text != "where did we land on auth" {
		t.Errorf("msg[0] = (%q,%q), want (user, where did we land on auth)", got[0].Role, got[0].Text)
	}
	if got[1].Role != "assistant" || got[1].Text != "we chose short-lived tokens" {
		t.Errorf("msg[1] = (%q,%q), want (assistant, we chose short-lived tokens)", got[1].Role, got[1].Text)
	}
	if got[0].UUID != "m1" || got[0].TSISO != "2026-07-15T10:00:00Z" || got[0].TS == 0 {
		t.Errorf("msg[0] identity/timestamp wrong: %+v", got[0])
	}
}

// The typed content blocks must flow through the SHARED internal/parse handling,
// producing the same [THINKING]/[TOOL:...]/[TOOL_RESULT] markers the CLI source
// gets — and a duplicate message.uuid must be collapsed.
func TestMessagesReuseParseBlocksAndDedup(t *testing.T) {
	t.Parallel()
	ad := New(writeDirExport(t, fixtureConversations))
	got, err := ad.Messages(source.Container{ID: "c2222222-2222-2222-2222-222222222222"})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	// m3, m4, m5 — the duplicate m5 is deduped away.
	if len(got) != 3 {
		t.Fatalf("want 3 messages after uuid dedup, got %d: %+v", len(got), got)
	}
	assistant := got[1]
	for _, marker := range []string{"[THINKING]", "[TOOL:bash]", "running the migration now"} {
		if !contains(assistant.Text, marker) {
			t.Errorf("assistant text %q missing %q (parse block reuse)", assistant.Text, marker)
		}
	}
	if !contains(got[2].Text, "[TOOL_RESULT]") || !contains(got[2].Text, "migration applied cleanly") {
		t.Errorf("tool_result text not flattened via parse: %q", got[2].Text)
	}
}

// Messages must come out in created_at order even when the export lists them
// out of order.
func TestMessagesOrderedByCreatedAt(t *testing.T) {
	t.Parallel()
	ad := New(writeDirExport(t, fixtureConversations))
	got, err := ad.Messages(source.Container{ID: "c3333333-3333-3333-3333-333333333333"})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(got) != 2 || got[0].Text != "first" || got[1].Text != "second" {
		t.Fatalf("messages not ordered by created_at: %+v", got)
	}
}

func TestMapSender(t *testing.T) {
	t.Parallel()
	cases := map[string]string{"human": "user", "assistant": "assistant", "": "user", "tool": "tool"}
	for in, want := range cases {
		if got := mapSender(in); got != want {
			t.Errorf("mapSender(%q) = %q, want %q", in, got, want)
		}
	}
}

// A directory with no conversations.json is a real error (a user pointed import
// at the wrong place) — never a silent empty import.
func TestMissingConversationsIsError(t *testing.T) {
	t.Parallel()
	empty := t.TempDir()
	if _, err := New(empty).Discover(); err == nil {
		t.Fatal("Discover on a dir without conversations.json must error")
	}
}

// A ZIP that is not a Claude export (no conversations.json member) is a clear
// error, not a no-op.
func TestNonClaudeZipIsError(t *testing.T) {
	t.Parallel()
	zp := writeZipExport(t, "irrelevant", "some/other-file.json")
	if _, err := New(zp).Discover(); err == nil {
		t.Fatal("Discover on a zip without conversations.json must error")
	}
}

// A conversations.json body that is not a JSON array is a clear error.
func TestMalformedConversationsIsError(t *testing.T) {
	t.Parallel()
	dir := writeDirExport(t, `{"not":"an array"}`)
	if _, err := New(dir).Discover(); err == nil {
		t.Fatal("Discover on a non-array conversations.json must error")
	}
}

// An empty conversation array is NOT an error — it is a valid (if empty) export.
func TestEmptyArrayIsNotError(t *testing.T) {
	t.Parallel()
	dir := writeDirExport(t, `[]`)
	got, err := New(dir).Discover()
	if err != nil {
		t.Fatalf("empty array must not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 containers for an empty export, got %d", len(got))
	}
}

// PII guard: nothing from a users.json / account block is read into a message.
// Here the export carries an account object on the conversation; the indexed
// messages must contain only the transcript text, never the account email.
func TestNoAccountPIILeaksIntoMessages(t *testing.T) {
	t.Parallel()
	const withAccount = `[
  {"uuid":"c9","name":"n","created_at":"2026-07-15T10:00:00Z",
   "account":{"uuid":"acct-1","email_address":"secret@example.com"},
   "chat_messages":[
     {"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z",
      "content":[{"type":"text","text":"hello world"}]}
   ]}
]`
	ad := New(writeDirExport(t, withAccount))
	got, err := ad.Messages(source.Container{ID: "c9"})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	for _, m := range got {
		if contains(m.Text, "secret@example.com") {
			t.Errorf("account email leaked into an indexed message: %q", m.Text)
		}
	}
}

// contains is a tiny substring helper (avoids importing strings just for this).
func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// Guard the fixture stays deterministic: the discovery order equals the file
// order, independent of map iteration.
func TestDiscoverOrderIsFileOrder(t *testing.T) {
	t.Parallel()
	ad := New(writeDirExport(t, fixtureConversations))
	got, err := ad.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	ids := make([]string, len(got))
	for i, c := range got {
		ids[i] = c.ID
	}
	want := []string{
		"c1111111-1111-1111-1111-111111111111",
		"c2222222-2222-2222-2222-222222222222",
		"c3333333-3333-3333-3333-333333333333",
	}
	if !sort.StringsAreSorted(ids) { // fixture uuids are ascending, so file order == sorted
		t.Errorf("discovery order not file order: %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("discovery order[%d] = %q, want %q", i, ids[i], want[i])
		}
	}
}
