package claudeweb

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/source"
)

// A synthetic mini-export: three conversations (single account). c1 is a plain
// text turn pair; c2 carries thinking + tool_use + tool_result blocks and a
// duplicate message.uuid (within-dump dedup); c3 is out of created_at order.
// NOT a real export — every value is invented.
const fixtureConversations = `[
  {"uuid":"c1111111-1111-1111-1111-111111111111","name":"auth","created_at":"2026-07-15T10:00:00Z","updated_at":"2026-07-15T10:05:00Z","account":{"uuid":"acc00001-0000-0000-0000-000000000000"},
   "chat_messages":[
     {"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","text":"where did we land on auth","content":[{"type":"text","text":"where did we land on auth"}]},
     {"uuid":"m2","sender":"assistant","created_at":"2026-07-15T10:00:30Z","text":"we chose short-lived tokens","content":[{"type":"text","text":"we chose short-lived tokens"}]}
   ]},
  {"uuid":"c2222222-2222-2222-2222-222222222222","name":"tool","created_at":"2026-07-16T09:00:00Z","updated_at":"2026-07-16T09:10:00Z","account":{"uuid":"acc00001-0000-0000-0000-000000000000"},
   "chat_messages":[
     {"uuid":"m3","sender":"human","created_at":"2026-07-16T09:00:00Z","content":[{"type":"text","text":"run the migration please"}]},
     {"uuid":"m4","sender":"assistant","created_at":"2026-07-16T09:00:10Z","content":[{"type":"thinking","thinking":"the user wants a schema migration"},{"type":"tool_use","name":"bash","input":{"cmd":"migrate up"}},{"type":"text","text":"running the migration now"}]},
     {"uuid":"m5","sender":"assistant","created_at":"2026-07-16T09:00:20Z","content":[{"type":"tool_result","content":"migration applied cleanly"}]},
     {"uuid":"m5","sender":"assistant","created_at":"2026-07-16T09:00:20Z","content":[{"type":"tool_result","content":"migration applied cleanly"}]}
   ]},
  {"uuid":"c3333333-3333-3333-3333-333333333333","name":"order","created_at":"2026-07-17T08:00:00Z","updated_at":"2026-07-17T08:10:00Z","account":{"uuid":"acc00001-0000-0000-0000-000000000000"},
   "chat_messages":[
     {"uuid":"m7","sender":"assistant","created_at":"2026-07-17T08:00:30Z","content":[{"type":"text","text":"second"}]},
     {"uuid":"m6","sender":"human","created_at":"2026-07-17T08:00:00Z","content":[{"type":"text","text":"first"}]}
   ]}
]`

const fixtureAccount = "acc00001-0000-0000-0000-000000000000"

func writeDirExport(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, conversationsFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeNamedZip(t *testing.T, dir, name, body string) string {
	t.Helper()
	zp := filepath.Join(dir, name)
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(conversationsFile)
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

// materialize writes body as a dir export, materializes into a fresh root, and
// returns (root, adapter over that root).
func materialize(t *testing.T, body string) (string, *Adapter) {
	t.Helper()
	root := t.TempDir()
	if _, err := Materialize(writeDirExport(t, body), root, false); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	return root, NewRoot(root)
}

func containerByID(cs []source.Container, id string) (source.Container, bool) {
	for _, c := range cs {
		if c.ID == id {
			return c, true
		}
	}
	return source.Container{}, false
}

func TestMaterializeAndRead_RoundTrip(t *testing.T) {
	t.Parallel()
	root, ad := materialize(t, fixtureConversations)

	// The tree: one account dir, three transcript files.
	accDir := filepath.Join(root, fixtureAccount)
	files, _ := filepath.Glob(filepath.Join(accDir, "*.jsonl"))
	if len(files) != 3 {
		t.Fatalf("want 3 transcript files under %s, got %d", accDir, len(files))
	}

	got, err := ad.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Discover: want 3 containers, got %d", len(got))
	}
	for _, id := range []string{"c1111111-1111-1111-1111-111111111111", "c2222222-2222-2222-2222-222222222222", "c3333333-3333-3333-3333-333333333333"} {
		c, ok := containerByID(got, id)
		if !ok {
			t.Errorf("missing container %q", id)
			continue
		}
		if c.CWD != "" || c.IsSubagent || c.ParentID != "" {
			t.Errorf("container %q malformed: %+v", id, c)
		}
	}

	// c1: role mapping + text.
	c1, _ := containerByID(got, "c1111111-1111-1111-1111-111111111111")
	m1, err := ad.Messages(c1)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(m1) != 2 || m1[0].Role != "user" || m1[0].Text != "where did we land on auth" || m1[1].Role != "assistant" {
		t.Errorf("c1 messages wrong: %+v", m1)
	}
	if m1[0].UUID != "m1" || m1[0].TSISO != "2026-07-15T10:00:00Z" || m1[0].TS == 0 {
		t.Errorf("c1 msg[0] identity/ts wrong: %+v", m1[0])
	}

	// c2: parse block reuse + uuid dedup (m5 twice → 3 messages).
	c2, _ := containerByID(got, "c2222222-2222-2222-2222-222222222222")
	m2, _ := ad.Messages(c2)
	if len(m2) != 3 {
		t.Fatalf("c2: want 3 messages after uuid dedup, got %d: %+v", len(m2), m2)
	}
	for _, marker := range []string{"[THINKING]", "[TOOL:bash]", "running the migration now"} {
		if !strings.Contains(m2[1].Text, marker) {
			t.Errorf("c2 assistant text %q missing %q (parse block reuse)", m2[1].Text, marker)
		}
	}
	if !strings.Contains(m2[2].Text, "[TOOL_RESULT]") {
		t.Errorf("c2 tool_result not flattened: %q", m2[2].Text)
	}

	// c3: created_at order.
	c3, _ := containerByID(got, "c3333333-3333-3333-3333-333333333333")
	m3, _ := ad.Messages(c3)
	if len(m3) != 2 || m3[0].Text != "first" || m3[1].Text != "second" {
		t.Errorf("c3 not in created_at order: %+v", m3)
	}
}

// TestContentBlocksVerbatim: the materialized file stores the export's content
// blocks VERBATIM (thinking/tool_use/tool_result), so the rebuilt index is
// byte-identical.
func TestContentBlocksVerbatim(t *testing.T) {
	t.Parallel()
	root, _ := materialize(t, fixtureConversations)
	data, err := os.ReadFile(filepath.Join(root, fixtureAccount, "c2222222-2222-2222-2222-222222222222.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, block := range []string{`"thinking"`, `"tool_use"`, `"tool_result"`, `"bash"`, "migrate up"} {
		if !strings.Contains(body, block) {
			t.Errorf("materialized transcript missing verbatim block content %q", block)
		}
	}
}

// TestPerAccountTree: a two-account export materializes into two account dirs,
// each conversation routed to its own account.
func TestPerAccountTree(t *testing.T) {
	t.Parallel()
	const twoAccounts = `[
	  {"uuid":"a1","account":{"uuid":"aaaa0000-1111-2222-3333-444444444444"},"chat_messages":[{"uuid":"ma","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"account A"}]}]},
	  {"uuid":"b1","account":{"uuid":"bbbb0000-5555-6666-7777-888888888888"},"chat_messages":[{"uuid":"mb","sender":"human","created_at":"2026-07-16T10:00:00Z","content":[{"type":"text","text":"account B"}]}]}
	]`
	root, ad := materialize(t, twoAccounts)

	for _, acc := range []string{"aaaa0000-1111-2222-3333-444444444444", "bbbb0000-5555-6666-7777-888888888888"} {
		if _, err := os.Stat(filepath.Join(root, acc)); err != nil {
			t.Errorf("account dir %q missing: %v", acc, err)
		}
	}
	got, _ := ad.Discover()
	if len(got) != 2 {
		t.Fatalf("want 2 containers across accounts, got %d", len(got))
	}
	// AccountDirName routes each container to the right account.
	for _, c := range got {
		acc := AccountDirName(c.Path)
		if (c.ID == "a1" && acc != "aaaa0000-1111-2222-3333-444444444444") ||
			(c.ID == "b1" && acc != "bbbb0000-5555-6666-7777-888888888888") {
			t.Errorf("container %q routed to wrong account dir %q", c.ID, acc)
		}
	}
}

// TestMultiBatchMaterialize: a "…-batch-NNNN.zip" import globs the batch set and
// materializes every conversation.
func TestMultiBatchMaterialize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeNamedZip(t, dir, "data-a-ts-h-batch-0000.zip",
		`[{"uuid":"c1","account":{"uuid":"aaaa0000-1111-2222-3333-444444444444"},"chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"batch 0"}]}]}]`)
	writeNamedZip(t, dir, "data-a-ts-h-batch-0001.zip",
		`[{"uuid":"c2","account":{"uuid":"aaaa0000-1111-2222-3333-444444444444"},"chat_messages":[{"uuid":"m2","sender":"human","created_at":"2026-07-16T10:00:00Z","content":[{"type":"text","text":"batch 1"}]}]}]`)
	writeNamedZip(t, dir, "unrelated.zip", `[{"uuid":"cX","chat_messages":[]}]`)

	root := t.TempDir()
	if _, err := Materialize(filepath.Join(dir, "data-a-ts-h-batch-0000.zip"), root, false); err != nil {
		t.Fatalf("Materialize(batch): %v", err)
	}
	got, _ := NewRoot(root).Discover()
	ids := map[string]bool{}
	for _, c := range got {
		ids[c.ID] = true
	}
	if len(got) != 2 || !ids["c1"] || !ids["c2"] {
		t.Fatalf("multi-batch materialized = %v, want {c1,c2}", ids)
	}
	if ids["cX"] {
		t.Error("unrelated sibling zip pulled into the batch import")
	}
}

// TestMalformedNoPartial: a malformed export errors and leaves the tree
// UNTOUCHED (staged writes never committed).
func TestMalformedNoPartial(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := Materialize(writeDirExport(t, `[{"uuid":"c1","account":{"uuid":"a"},"chat_messages":[]},{"uuid":`), root, false)
	if err == nil {
		t.Fatal("malformed export must error")
	}
	// No account dirs committed (only the temp staging, which is removed).
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".import-") {
			t.Errorf("malformed import left a committed entry %q (partial write)", e.Name())
		}
	}
}

// TestEmptyAccountRefusedUnderMirror: an account-less export is refused under
// mirror (no write) and allowed under keep (materialized into the "unknown"
// account dir).
func TestEmptyAccountRefusedUnderMirror(t *testing.T) {
	t.Parallel()
	body := `[{"uuid":"c1","chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"no account"}]}]}]`

	t.Run("mirror refuses", func(t *testing.T) {
		root := t.TempDir()
		if _, err := Materialize(writeDirExport(t, body), root, true); err == nil {
			t.Fatal("account-less export under mirror must be refused")
		}
		if entries, _ := os.ReadDir(root); hasCommitted(entries) {
			t.Error("refused import left committed files")
		}
	})
	t.Run("keep allows into unknown", func(t *testing.T) {
		root := t.TempDir()
		if _, err := Materialize(writeDirExport(t, body), root, false); err != nil {
			t.Fatalf("account-less under keep must be allowed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "unknown", "c1.jsonl")); err != nil {
			t.Errorf("account-less conversation not materialized under 'unknown': %v", err)
		}
	})
}

func hasCommitted(entries []os.DirEntry) bool {
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".import-") {
			return true
		}
	}
	return false
}

// TestUsersJSONNeverRead: a users.json beside conversations.json is never read,
// so its PII never reaches a materialized transcript.
func TestUsersJSONNeverRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.json"),
		[]byte(`[{"email_address":"secret@example.com","full_name":"Jane Secret","phone_number":"+15550001111"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, conversationsFile),
		[]byte(`[{"uuid":"c1","account":{"uuid":"acc00009-0000-0000-0000-000000000000"},"chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"hello"}]}]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := Materialize(dir, root, false); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "acc00009-0000-0000-0000-000000000000", "c1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pii := range []string{"secret@example.com", "Jane Secret", "+15550001111"} {
		if strings.Contains(string(data), pii) {
			t.Errorf("PII %q leaked into a materialized transcript", pii)
		}
	}
}

// TestBranchedCreatedAtOrder: a branched conversation (parent_message_uuid) is
// materialized + read with ALL messages in created_at order (no tree walk).
func TestBranchedCreatedAtOrder(t *testing.T) {
	t.Parallel()
	body := `[{"uuid":"c1","account":{"uuid":"acc00003-0000-0000-0000-000000000000"},"chat_messages":[
	  {"uuid":"m3","sender":"assistant","created_at":"2026-07-15T10:00:30Z","parent_message_uuid":"m1","content":[{"type":"text","text":"branch two"}]},
	  {"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","parent_message_uuid":"root","content":[{"type":"text","text":"root turn"}]},
	  {"uuid":"m2","sender":"assistant","created_at":"2026-07-15T10:00:15Z","parent_message_uuid":"m1","content":[{"type":"text","text":"branch one"}]}
	]}]`
	_, ad := materialize(t, body)
	got, _ := ad.Discover()
	msgs, _ := ad.Messages(got[0])
	want := []string{"root turn", "branch one", "branch two"}
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d: %+v", len(msgs), msgs)
	}
	for i, w := range want {
		if msgs[i].Text != w {
			t.Errorf("message[%d] = %q, want %q (created_at order)", i, msgs[i].Text, w)
		}
	}
}

// TestDiscoverEmptyRootNotError: an absent transcript root yields no containers,
// not an error.
func TestDiscoverEmptyRootNotError(t *testing.T) {
	t.Parallel()
	got, err := NewRoot(filepath.Join(t.TempDir(), "nope")).Discover()
	if err != nil || len(got) != 0 {
		t.Errorf("empty root: got (%d, %v), want (0, nil)", len(got), err)
	}
}

// TestMergeNeverDrops is the raw-archive keep-everything guarantee: re-importing
// a SMALLER export of the same conversation must NOT drop a message a prior
// import wrote — the transcript is MERGED (union by identity), never overwritten.
func TestMergeNeverDrops(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	acc := `"account":{"uuid":"acc00007-0000-0000-0000-000000000000"}`
	// Import 1: c1 has m1 + m2.
	full := `[{"uuid":"c1",` + acc + `,"chat_messages":[` +
		`{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"first turn"}]},` +
		`{"uuid":"m2","sender":"assistant","created_at":"2026-07-15T10:00:30Z","content":[{"type":"text","text":"second turn"}]}]}]`
	if _, err := Materialize(writeDirExport(t, full), root, false); err != nil {
		t.Fatalf("import 1: %v", err)
	}
	// Import 2: a SMALLER export of c1 with only m1.
	smaller := `[{"uuid":"c1",` + acc + `,"chat_messages":[` +
		`{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"first turn"}]}]}]`
	if _, err := Materialize(writeDirExport(t, smaller), root, false); err != nil {
		t.Fatalf("import 2: %v", err)
	}
	// The transcript still holds BOTH messages.
	got, _ := NewRoot(root).Discover()
	if len(got) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(got))
	}
	msgs, _ := NewRoot(root).Messages(got[0])
	if len(msgs) != 2 {
		t.Errorf("merge dropped a message: got %d, want 2 (m1+m2 preserved): %+v", len(msgs), msgs)
	}
}

// TestReconcileMirrorPrunesAbsentFresh + staleness: under mirror, a FRESHER
// export that omits a conversation deletes its transcript; a STALE export never
// prunes.
func TestReconcileMirror(t *testing.T) {
	t.Parallel()
	acc := "acc00008-0000-0000-0000-000000000000"

	twoConvs := `[` +
		`{"uuid":"c1","updated_at":"2026-07-15T10:00:00Z","account":{"uuid":"` + acc + `"},"chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"keep"}]}]},` +
		`{"uuid":"c2","updated_at":"2026-07-15T10:00:00Z","account":{"uuid":"` + acc + `"},"chat_messages":[{"uuid":"m2","sender":"human","created_at":"2026-07-15T10:00:00Z","content":[{"type":"text","text":"maybe pruned"}]}]}]`
	oneConvFresh := `[{"uuid":"c1","updated_at":"2026-07-20T10:00:00Z","account":{"uuid":"` + acc + `"},"chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-20T10:00:00Z","content":[{"type":"text","text":"keep"}]}]}]`
	oneConvStale := `[{"uuid":"c1","updated_at":"2026-07-10T10:00:00Z","account":{"uuid":"` + acc + `"},"chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-07-10T10:00:00Z","content":[{"type":"text","text":"keep"}]}]}]`

	reconcile := func(t *testing.T, root, body string, mirror bool) {
		res, err := Materialize(writeDirExport(t, body), root, mirror)
		if err != nil {
			t.Fatalf("materialize: %v", err)
		}
		for _, ai := range res.Accounts {
			if err := Reconcile(ai, mirror); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
		}
	}
	c2File := func(root string) string { return filepath.Join(root, acc, "c2.jsonl") }

	t.Run("mirror + fresher export prunes the absent conversation", func(t *testing.T) {
		root := t.TempDir()
		reconcile(t, root, twoConvs, true)     // establishes c1,c2 (watermark 07-15)
		reconcile(t, root, oneConvFresh, true) // fresher (07-20), c2 absent → pruned
		if _, err := os.Stat(c2File(root)); !os.IsNotExist(err) {
			t.Error("c2 not pruned under mirror by a fresher export")
		}
	})
	t.Run("mirror + stale export does not prune", func(t *testing.T) {
		root := t.TempDir()
		reconcile(t, root, twoConvs, true)     // watermark 07-15
		reconcile(t, root, oneConvStale, true) // STALE (07-10), c2 absent → must KEEP
		if _, err := os.Stat(c2File(root)); err != nil {
			t.Errorf("c2 pruned by a STALE export (staleness guard failed): %v", err)
		}
	})
	t.Run("keep never prunes an absent conversation", func(t *testing.T) {
		root := t.TempDir()
		reconcile(t, root, twoConvs, false)     // c1,c2 under keep
		reconcile(t, root, oneConvFresh, false) // fresher, c2 absent — but keep retains it
		if _, err := os.Stat(c2File(root)); err != nil {
			t.Errorf("c2 pruned under keep (keep must retain absent conversations): %v", err)
		}
	})
}

// TestDiscoverSkipsStagingDir: a leftover ".import-*" staging dir (from a crash
// mid-import) must NOT surface as a bogus account scope — Discover skips
// dot-prefixed entries.
func TestDiscoverSkipsStagingDir(t *testing.T) {
	t.Parallel()
	root, ad := materialize(t, fixtureConversations)
	// Simulate a crash-leftover staging dir with a staged transcript inside.
	staging := filepath.Join(root, ".import-leftover")
	if err := os.MkdirAll(filepath.Join(staging, "acc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "acc", "ghost.jsonl"),
		[]byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"ghost"}]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := ad.Discover()
	for _, c := range got {
		if strings.HasPrefix(AccountDirName(c.Path), ".") {
			t.Errorf("staging dir surfaced as a scope: %+v", c)
		}
		if c.ID == "ghost" {
			t.Errorf("staged ghost transcript discovered: %+v", c)
		}
	}
	if len(got) != 3 { // only the 3 real fixture conversations
		t.Errorf("Discover returned %d containers, want 3 (staging excluded)", len(got))
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
