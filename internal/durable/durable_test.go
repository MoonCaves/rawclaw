package durable

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
)

// isolate points the vault at a scratch dir for one test.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	return filepath.Join(dir, "rawclaw", "transcripts")
}

// TestStoreFileKeepsBytesVerbatim is the whole point of the byte-copy path: the
// index is rebuilt by re-parsing the vaulted file, so any normalization here
// would show up as a diff between the live index and the rebuilt one.
func TestStoreFileKeepsBytesVerbatim(t *testing.T) {
	root := isolate(t)
	src := filepath.Join(t.TempDir(), "s.jsonl")
	// Deliberately ragged: no trailing newline, an unusual key order, a blank line.
	body := `{"message":{"role":"user","content":[{"type":"text","text":"hi"}]},"type":"user","uuid":"u1"}` + "\n\n" +
		`{"type":"assistant","uuid":"u2","message":{"role":"assistant","content":[{"type":"text","text":"yo"}]}}`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := StoreFile(Meta{ID: "s", Source: "claude", SourcePath: src}, src); err != nil {
		t.Fatalf("StoreFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "s.jsonl"))
	if err != nil {
		t.Fatalf("read vaulted transcript: %v", err)
	}
	if string(got) != body {
		t.Errorf("vaulted bytes differ from source:\n got: %q\nwant: %q", got, body)
	}
}

// TestStoreMessagesIsClaudeShape checks the OTHER producer lands in the same
// format the byte-copy path does — one vault, one reader. It asserts through
// the real parser rather than the struct, because the parser is what the
// rebuild actually uses.
func TestStoreMessagesIsClaudeShape(t *testing.T) {
	root := isolate(t)
	msgs := []model.Message{
		{Role: "user", Text: "where is the ledger", TS: 1780000000, TSISO: "2026-06-01T10:00:00Z", UUID: "m1"},
		{Role: "assistant", Text: "in billing", TS: 1780000060, TSISO: "2026-06-01T10:01:00Z", UUID: "m2"},
	}
	if err := StoreMessages(Meta{ID: "c1", Source: "codex", CWD: "/w/ledger"}, msgs); err != nil {
		t.Fatalf("StoreMessages: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "c1.jsonl"))
	if err != nil {
		t.Fatalf("read vaulted transcript: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), data)
	}
	for i, line := range lines {
		var o map[string]any
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			t.Fatalf("line %d is not JSON: %v", i, err)
		}
		if got := parse.MsgRole(o); got != msgs[i].Role {
			t.Errorf("line %d role = %q, want %q", i, got, msgs[i].Role)
		}
		if got := parse.ExtractText(o); got != msgs[i].Text {
			t.Errorf("line %d text = %q, want %q", i, got, msgs[i].Text)
		}
		if got := parse.MsgUUID(o); got != msgs[i].UUID {
			t.Errorf("line %d uuid = %q, want %q", i, got, msgs[i].UUID)
		}
		if got, _ := o["timestamp"].(string); got != msgs[i].TSISO {
			t.Errorf("line %d timestamp = %q, want %q", i, got, msgs[i].TSISO)
		}
		if got, _ := o["cwd"].(string); got != "/w/ledger" {
			t.Errorf("line %d cwd = %q, want /w/ledger", i, got)
		}
	}
}

// TestStoreMessagesKeepsUnindexableRoleSearchable guards the record-type mapping:
// a role the indexer does not recognize would be dropped on rebuild if it were
// written as the record type verbatim.
func TestStoreMessagesKeepsUnindexableRoleSearchable(t *testing.T) {
	root := isolate(t)
	if err := StoreMessages(Meta{ID: "c2", Source: "codex"}, []model.Message{
		{Role: "tool", Text: "ran the billing report", TSISO: "2026-06-01T10:00:00Z"},
	}); err != nil {
		t.Fatalf("StoreMessages: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "c2.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var o map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &o); err != nil {
		t.Fatal(err)
	}
	typ, _ := o["type"].(string)
	indexable := false
	for _, it := range parse.IndexableTypes {
		if typ == it {
			indexable = true
		}
	}
	if !indexable {
		t.Errorf("record type %q is not indexable — this message would vanish on rebuild", typ)
	}
	if got := parse.MsgRole(o); got != "tool" {
		t.Errorf("role = %q, want the true role %q preserved", got, "tool")
	}
	if got := parse.ExtractText(o); got != "ran the billing report" {
		t.Errorf("text = %q, want it preserved", got)
	}
}

// TestPathForStaysInsideRoot: a session id is not a trusted path component. A
// ".." segment must land as a file in the vault, never above it, and every
// remaining character must be one a path component can portably hold — a
// backslash is a separator on the other platform this has to work on.
func TestPathForStaysInsideRoot(t *testing.T) {
	root := isolate(t)
	for _, id := range []string{"../escape", "..", "a/../../b", "with space/x", `w\in\dows`, "nul\x00byte", "sess:1|2"} {
		p, err := PathFor(id)
		if err != nil {
			t.Fatalf("PathFor(%q): %v", id, err)
		}
		if !strings.HasPrefix(filepath.Clean(p), filepath.Clean(root)+string(filepath.Separator)) {
			t.Errorf("PathFor(%q) = %q, escapes the vault root %q", id, p, root)
		}
		rel := strings.TrimPrefix(filepath.Clean(p), filepath.Clean(root)+string(filepath.Separator))
		for _, seg := range strings.Split(rel, string(filepath.Separator)) {
			if strings.ContainsFunc(seg, unportable) {
				t.Errorf("PathFor(%q) = %q: segment %q holds a character a path component cannot portably carry", id, p, seg)
			}
		}
	}
}

// unportable reports a rune that has no business in a vault path component.
func unportable(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	case r == '.' || r == '_' || r == '-':
		return false
	}
	return true
}

// TestSubagentIDNestsAsDirectories: a lineage-namespaced id carries a slash, and
// nesting it keeps the vault browsable the way the source trees are.
func TestSubagentIDNestsAsDirectories(t *testing.T) {
	root := isolate(t)
	if err := StoreMessages(Meta{ID: "parent/child", Source: "claude"}, []model.Message{
		{Role: "user", Text: "sub", TSISO: "2026-06-01T10:00:00Z"},
	}); err != nil {
		t.Fatalf("StoreMessages: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "parent", "child.jsonl")); err != nil {
		t.Errorf("subagent transcript not nested under its parent: %v", err)
	}
	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != "parent/child" {
		t.Errorf("List = %+v, want one entry with id parent/child", list)
	}
}

// TestMissingSinceRoundTrips is the watermark surviving outside the db — the
// property that lets a rebuild label a retained session correctly.
func TestMissingSinceRoundTrips(t *testing.T) {
	isolate(t)
	if err := StoreMessages(Meta{ID: "s", Source: "claude"}, []model.Message{
		{Role: "user", Text: "hi", TSISO: "2026-06-01T10:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SetMissingSince("s", 1780000000); err != nil {
		t.Fatalf("SetMissingSince: %v", err)
	}
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].MissingSince != 1780000000 {
		t.Fatalf("MissingSince not persisted: %+v", list)
	}
	if err := SetMissingSince("s", 0); err != nil {
		t.Fatalf("SetMissingSince(clear): %v", err)
	}
	list, _ = List()
	if list[0].MissingSince != 0 {
		t.Errorf("MissingSince = %v after clear, want 0", list[0].MissingSince)
	}
}

// TestSetMissingSinceOnUnvaultedSessionIsNoOp: retention reports ids for
// sessions indexed before the vault existed. Those must not error, and must not
// conjure a sidecar with no transcript beside it.
func TestSetMissingSinceOnUnvaultedSessionIsNoOp(t *testing.T) {
	root := isolate(t)
	if err := SetMissingSince("never-vaulted", 1780000000); err != nil {
		t.Fatalf("SetMissingSince on an unvaulted session: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "never-vaulted.meta.json")); !os.IsNotExist(err) {
		t.Errorf("a sidecar was written with no transcript beside it (err=%v)", err)
	}
}

// TestRemoveDeletesBothFilesAndEmptyDirs: a user delete has to really delete, or
// the next rebuild resurrects the session out of the vault.
func TestRemoveDeletesBothFilesAndEmptyDirs(t *testing.T) {
	root := isolate(t)
	if err := StoreMessages(Meta{ID: "parent/child", Source: "claude"}, []model.Message{
		{Role: "user", Text: "hi", TSISO: "2026-06-01T10:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := Remove("parent/child"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "parent", "child.jsonl")); !os.IsNotExist(err) {
		t.Errorf("transcript still present after Remove (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(root, "parent", "child.meta.json")); !os.IsNotExist(err) {
		t.Errorf("sidecar still present after Remove (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(root, "parent")); !os.IsNotExist(err) {
		t.Errorf("empty lineage dir left behind (err=%v)", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("Remove pruned past the vault root: %v", err)
	}
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("List = %+v after Remove, want empty", list)
	}
}

// TestStoreFileSkipsRewriteOfUnchangedSource: a full reindex re-offers every
// file, and re-copying an unchanged corpus is pure cost. Detected by mtime,
// since the content would be identical either way.
func TestStoreFileSkipsRewriteOfUnchangedSource(t *testing.T) {
	root := isolate(t)
	src := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(src, []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Meta{ID: "s", Source: "claude", SourcePath: src, SourceSize: 55, SourceFP: "abc123"}
	if err := StoreFile(m, src); err != nil {
		t.Fatal(err)
	}
	vp := filepath.Join(root, "s.jsonl")
	before, err := os.Stat(vp)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate so a rewrite is unmistakable.
	old := before.ModTime().Add(-time.Hour)
	if err := os.Chtimes(vp, old, old); err != nil {
		t.Fatal(err)
	}

	if err := StoreFile(m, src); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(vp)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(old) {
		t.Errorf("unchanged source was re-copied (mtime moved %v -> %v)", old, after.ModTime())
	}

	// A CHANGED fingerprint must still rewrite.
	m.SourceFP = "def456"
	if err := StoreFile(m, src); err != nil {
		t.Fatal(err)
	}
	after2, err := os.Stat(vp)
	if err != nil {
		t.Fatal(err)
	}
	if after2.ModTime().Equal(old) {
		t.Errorf("changed source was NOT re-copied (mtime still %v)", old)
	}
}

// TestStoreFileClearsStaleMissingFlagOnSkip is the trap in the skip path: the
// source was just read off disk, so a leftover "its source is gone" watermark
// would mislabel a live session after the next rebuild.
func TestStoreFileClearsStaleMissingFlagOnSkip(t *testing.T) {
	isolate(t)
	src := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(src, []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Meta{ID: "s", Source: "claude", SourcePath: src, SourceSize: 55, SourceFP: "abc123"}
	if err := StoreFile(m, src); err != nil {
		t.Fatal(err)
	}
	if err := SetMissingSince("s", 1780000000); err != nil {
		t.Fatal(err)
	}

	if err := StoreFile(m, src); err != nil { // same fingerprint -> skip path
		t.Fatal(err)
	}
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("List = %+v, want one entry", list)
	}
	if list[0].MissingSince != 0 {
		t.Errorf("MissingSince = %v after re-indexing a present file, want 0", list[0].MissingSince)
	}
}

// TestListRecoversStrandedTranscript: a crash between the two renames leaves a
// transcript with no sidecar. That is still history, so it must be enumerated
// rather than silently skipped.
func TestListRecoversStrandedTranscript(t *testing.T) {
	root := isolate(t)
	if err := StoreMessages(Meta{ID: "s", Source: "claude"}, []model.Message{
		{Role: "user", Text: "hi", TSISO: "2026-06-01T10:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "s.meta.json")); err != nil {
		t.Fatal(err)
	}
	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != "s" {
		t.Fatalf("List = %+v, want the stranded transcript recovered as id s", list)
	}
}

// TestListOnMissingRootIsEmpty: a machine that has never indexed anything has no
// vault dir, and that is not an error.
func TestListOnMissingRootIsEmpty(t *testing.T) {
	isolate(t)
	list, err := List()
	if err != nil {
		t.Fatalf("List on a missing vault: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List = %+v, want empty", list)
	}
}
