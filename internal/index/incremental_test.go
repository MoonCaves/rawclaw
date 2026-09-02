package index

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/antigravity"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/MoonCaves/rawclaw/internal/store"
)

func TestIncrementalIngest_LargeAppendKeepsExistingRows(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "large.db")
	f := filepath.Join(dir, "large.jsonl")
	content := strings.Repeat("large transcript payload ", 40)
	var b strings.Builder
	for i := range 10000 {
		fmt.Fprintf(&b, `{"type":"user","message":{"role":"user","content":%q},"uuid":"large-%d","timestamp":"2026-08-20T10:00:00Z"}`+"\n", content, i)
	}
	writeFile(t, f, b.String())
	c := source.Container{ID: "large-session", Path: f, CWD: "/repo"}
	msgsFn := claudeTailMsgsFn()
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	var firstID int64
	if err := con.QueryRow("SELECT id FROM messages WHERE session_id=? ORDER BY id LIMIT 1", c.ID).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	appendFile(t, f, `{"type":"assistant","message":{"role":"assistant","content":"new one"},"uuid":"large-new-1","timestamp":"2026-08-20T11:00:00Z"}`+"\n")
	appendFile(t, f, `{"type":"user","message":{"role":"user","content":"new two"},"uuid":"large-new-2","timestamp":"2026-08-20T11:00:05Z"}`+"\n")
	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}
	if got := IncrementalIngestCount.Load(); got != 1 {
		t.Fatalf("incremental ingests = %d, want 1", got)
	}
	if got := FullReindexCount.Load(); got != 0 {
		t.Fatalf("full reindexes = %d, want 0", got)
	}
	var firstIDAfter int64
	if err := con.QueryRow("SELECT id FROM messages WHERE session_id=? ORDER BY id LIMIT 1", c.ID).Scan(&firstIDAfter); err != nil {
		t.Fatal(err)
	}
	if firstIDAfter != firstID {
		t.Fatalf("first message id changed from %d to %d", firstID, firstIDAfter)
	}
	if got := scalarInt(t, con, "SELECT message_count FROM sessions WHERE id=?", c.ID); got != 10002 {
		t.Fatalf("message_count = %d, want 10002", got)
	}
	var offset, size int64
	if err := con.QueryRow("SELECT byte_offset,size FROM file_index WHERE session_id=?", c.ID).Scan(&offset, &size); err != nil {
		t.Fatal(err)
	}
	if offset != size {
		t.Fatalf("byte_offset = %d, size = %d, want complete append", offset, size)
	}
}

// TestIncrementalIngest_AppendFastPath_Claude verifies that append-only growth
// takes the incremental fast path, increments the counter, anchors at the last
// newline before a partial record, and yields a database byte-equal to a clean
// full reindex.
func TestIncrementalIngest_AppendFastPath_Claude(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "claude-incr.db")
	dbRef := filepath.Join(dir, "claude-ref.db")
	f := filepath.Join(dir, "session-c1.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"How do I configure redis cache pool?"},"uuid":"u-101","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Configure redis with PoolSize and MinIdleConns."},"uuid":"u-102","timestamp":"2026-08-20T10:00:05Z"}`
	writeFile(t, f, msg1+"\n"+msg2+"\n")

	c := source.Container{ID: "session-c1", Path: f, CWD: "/repo"}
	msgsFn := claudeTailMsgsFn()

	ResetIngestCountersForTesting()

	// 1. Initial full index pass
	n, status, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", "")
	if err != nil {
		t.Fatalf("initial indexing failed: %v", err)
	}
	if status != IndexFresh || n != 1 {
		t.Fatalf("initial index status=%v, n=%d", status, n)
	}
	if gotFull := FullReindexCount.Load(); gotFull != 1 {
		t.Errorf("FullReindexCount = %d, want 1", gotFull)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 0 {
		t.Errorf("IncrementalIngestCount = %d, want 0", gotIncr)
	}

	// 2. Append a complete message followed by a half-written message without newline.
	msg3 := `{"type":"user","message":{"role":"user","content":"What about connection timeouts?"},"uuid":"u-103","timestamp":"2026-08-20T10:01:00Z"}`
	msg4Partial := `{"type":"assistant","message":{"role":"assistant","content":"Set DialTimeout to 5s`
	appendFile(t, f, msg3+"\n"+msg4Partial)

	// 3. Second index pass -> MUST use incremental fast path and retain prefix watermark.
	n2, status2, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", "")
	if err != nil {
		t.Fatalf("second indexing failed: %v", err)
	}
	if status2 != IndexFresh || n2 != 1 {
		t.Fatalf("second index status=%v, n=%d", status2, n2)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
		t.Errorf("IncrementalIngestCount = %d, want 1 after append", gotIncr)
	}
	if gotFull := FullReindexCount.Load(); gotFull != 1 {
		t.Errorf("FullReindexCount = %d, want 1 (should not have triggered full reindex)", gotFull)
	}

	// 4. Complete the pending record and ingest again.
	msg4Remainder := `s and ReadTimeout to 3s."},"uuid":"u-104","timestamp":"2026-08-20T10:01:05Z"}` + "\n"
	appendFile(t, f, msg4Remainder)

	n3, status3, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", "")
	if err != nil {
		t.Fatalf("third indexing failed: %v", err)
	}
	if status3 != IndexFresh || n3 != 1 {
		t.Fatalf("third index status=%v, n=%d", status3, n3)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 2 {
		t.Errorf("IncrementalIngestCount = %d, want 2 after two appends", gotIncr)
	}

	// 5. Verify message count in database
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	if got := scalarInt(t, con, "SELECT message_count FROM sessions WHERE id='session-c1'"); got != 4 {
		t.Errorf("message_count = %d, want 4", got)
	}
	if got := scalarInt(t, con, "SELECT COUNT(*) FROM messages WHERE session_id='session-c1'"); got != 4 {
		t.Errorf("messages row count = %d, want 4", got)
	}

	// 6. Compare against reference full reindex on dbRef: rows must be identical
	claudeFullMsgs := claudeFullMsgsFn()
	if _, _, err := EnsureIndexedContainers(dbRef, true, []source.Container{c}, claudeFullMsgs, "claude", ""); err != nil {
		t.Fatalf("reference indexing failed: %v", err)
	}

	conRef, err := store.ConnectRO(dbRef)
	if err != nil {
		t.Fatal(err)
	}
	defer conRef.Close()

	assertMessagesEqual(t, con, conRef, "session-c1")
}

// TestIncrementalIngest_AppendFastPath_Codex verifies incremental tail ingest for Codex rollouts.
func TestIncrementalIngest_AppendFastPath_Codex(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "codex-incr.db")
	f := filepath.Join(dir, "rollout-2026-08-20-001.jsonl")

	header := `{"type":"session_meta","payload":{"id":"session-codex-1","cwd":"/workspace"}}`
	item1 := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Implement Raft consensus"}]},"timestamp":"2026-08-20T10:00:00Z"}`
	item2 := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Raft implementation starting with leader election."}]},"timestamp":"2026-08-20T10:00:05Z"}`
	writeFile(t, f, header+"\n"+item1+"\n"+item2+"\n")

	c := source.Container{ID: "session-codex-1", Path: f, CWD: "/workspace"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "Implement Raft consensus", TS: 1, TSISO: "2026-08-20T10:00:00Z", UUID: codex.MintUUID("session-codex-1", 0)},
			{Role: "assistant", Text: "Raft implementation starting with leader election.", TS: 2, TSISO: "2026-08-20T10:00:05Z", UUID: codex.MintUUID("session-codex-1", 1)},
		}, nil
	}

	ResetIngestCountersForTesting()

	// Initial index
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "codex", ""); err != nil {
		t.Fatalf("initial index: %v", err)
	}

	// Append 2 new response items
	item3 := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"How does log replication work?"}]},"timestamp":"2026-08-20T10:01:00Z"}`
	item4 := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Leader appends entries to log and broadcasts AppendEntries RPCs."}]},"timestamp":"2026-08-20T10:01:05Z"}`
	appendFile(t, f, item3+"\n"+item4+"\n")

	// Incremental ingest pass
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "codex", ""); err != nil {
		t.Fatalf("second index: %v", err)
	}

	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
		t.Errorf("IncrementalIngestCount = %d, want 1", gotIncr)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	if got := scalarInt(t, con, "SELECT message_count FROM sessions WHERE id='session-codex-1'"); got != 4 {
		t.Errorf("message_count = %d, want 4", got)
	}

	// Verify UUIDs are deterministic ordinals 0, 1, 2, 3
	rows, err := con.Query("SELECT uuid FROM messages WHERE session_id='session-codex-1' ORDER BY id ASC")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var uuids []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatal(err)
		}
		uuids = append(uuids, u)
	}

	for i := 0; i < 4; i++ {
		want := codex.MintUUID("session-codex-1", i)
		if uuids[i] != want {
			t.Errorf("message %d uuid = %q, want %q", i, uuids[i], want)
		}
	}
}

// TestIncrementalIngest_AppendFastPath_Antigravity verifies incremental tail ingest for Antigravity.
func TestIncrementalIngest_AppendFastPath_Antigravity(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "agy-incr.db")
	f := filepath.Join(dir, "transcript.jsonl")

	step1 := `{"step_index":0,"type":"USER_INPUT","source":"USER_EXPLICIT","content":"<USER_REQUEST>Fix memory leak</USER_REQUEST>","created_at":"2026-08-20T10:00:00Z"}`
	step2 := `{"step_index":1,"type":"PLANNER_RESPONSE","source":"MODEL","content":"Investigating heap profile.","thinking":"Profile reveals unclosed channels.","created_at":"2026-08-20T10:00:05Z"}`
	writeFile(t, f, step1+"\n"+step2+"\n")

	c := source.Container{ID: "session-agy-1", Path: f, CWD: "/workspace"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "Fix memory leak", TS: 1, TSISO: "2026-08-20T10:00:00Z", UUID: antigravity.MintUUID("session-agy-1", 0, 0)},
			{Role: "assistant", Text: "[THINKING] Profile reveals unclosed channels.\nInvestigating heap profile.", TS: 2, TSISO: "2026-08-20T10:00:05Z", UUID: antigravity.MintUUID("session-agy-1", 1, 1)},
		}, nil
	}

	ResetIngestCountersForTesting()

	// Initial index
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "antigravity", ""); err != nil {
		t.Fatalf("initial index: %v", err)
	}

	// Append new step records
	step3 := `{"step_index":2,"type":"USER_INPUT","source":"USER_EXPLICIT","content":"<USER_REQUEST>Apply patch</USER_REQUEST>","created_at":"2026-08-20T10:01:00Z"}`
	step4 := `{"step_index":3,"type":"PLANNER_RESPONSE","source":"MODEL","content":"Patch applied successfully.","created_at":"2026-08-20T10:01:05Z"}`
	appendFile(t, f, step3+"\n"+step4+"\n")

	// Incremental ingest pass
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "antigravity", ""); err != nil {
		t.Fatalf("second index: %v", err)
	}

	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
		t.Errorf("IncrementalIngestCount = %d, want 1", gotIncr)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	if got := scalarInt(t, con, "SELECT message_count FROM sessions WHERE id='session-agy-1'"); got != 4 {
		t.Errorf("message_count = %d, want 4", got)
	}
}

// TestIncrementalIngest_FallbackOnTruncation verifies that truncating a file
// falls back to full reindex without leaving stale or duplicate rows.
func TestIncrementalIngest_FallbackOnTruncation(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "trunc.db")
	f := filepath.Join(dir, "session-trunc.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"Line 1"},"uuid":"u-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Line 2"},"uuid":"u-2","timestamp":"2026-08-20T10:00:05Z"}`
	msg3 := `{"type":"user","message":{"role":"user","content":"Line 3"},"uuid":"u-3","timestamp":"2026-08-20T10:00:10Z"}`
	msg4 := `{"type":"assistant","message":{"role":"assistant","content":"Line 4"},"uuid":"u-4","timestamp":"2026-08-20T10:00:15Z"}`

	fullContent := strings.Join([]string{msg1, msg2, msg3, msg4}, "\n") + "\n"
	writeFile(t, f, fullContent)

	c := source.Container{ID: "session-trunc", Path: f, CWD: "/repo"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		return parseClaudeTail([]byte(fullContent))
	}

	ResetIngestCountersForTesting()

	// Initial index with 4 messages
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	// Now truncate file to only 2 messages
	shortContent := strings.Join([]string{msg1, msg2}, "\n") + "\n"
	writeFile(t, f, shortContent)

	shortMsgsFn := func(got source.Container) ([]model.Message, error) {
		return parseClaudeTail([]byte(shortContent))
	}

	// Reindex: must detect size < prev.size and fall back to full reindex
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, shortMsgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	if gotFull := FullReindexCount.Load(); gotFull != 2 {
		t.Errorf("FullReindexCount = %d, want 2 (fallback triggered on truncation)", gotFull)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	if got := scalarInt(t, con, "SELECT message_count FROM sessions WHERE id='session-trunc'"); got != 2 {
		t.Errorf("message_count after truncation = %d, want 2", got)
	}
	if got := scalarInt(t, con, "SELECT COUNT(*) FROM messages WHERE session_id='session-trunc'"); got != 2 {
		t.Errorf("messages row count after truncation = %d, want 2 (no stale rows)", got)
	}
}

// TestIncrementalIngest_FallbackOnHeadRewrite verifies that modifying the head of a file
// (e.g. rewrite in-place) is detected via fingerprint mismatch and fully re-indexed.
func TestIncrementalIngest_FallbackOnHeadRewrite(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "rewrite.db")
	f := filepath.Join(dir, "session-rewrite.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"Initial line 1"},"uuid":"u-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Initial line 2"},"uuid":"u-2","timestamp":"2026-08-20T10:00:05Z"}`
	writeFile(t, f, msg1+"\n"+msg2+"\n")

	c := source.Container{ID: "session-rewrite", Path: f, CWD: "/repo"}
	msgsFn := claudeFullMsgsFn()

	ResetIngestCountersForTesting()

	// Initial index
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	// Rewrite line 1 in place with the same length and append line 3.
	msg1Rewritten := `{"type":"user","message":{"role":"user","content":"Updated line 1"},"uuid":"u-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg3 := `{"type":"user","message":{"role":"user","content":"Appended line 3"},"uuid":"u-3","timestamp":"2026-08-20T10:00:10Z"}`

	time.Sleep(10 * time.Millisecond)
	writeFile(t, f, msg1Rewritten+"\n"+msg2+"\n"+msg3+"\n")

	// Reindex: head fingerprint mismatch must fall back to full reindex
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	if gotFull := FullReindexCount.Load(); gotFull != 2 {
		t.Errorf("FullReindexCount = %d, want 2 (fallback on head fingerprint mismatch)", gotFull)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	if got := scalar(t, con, "SELECT content FROM messages WHERE session_id='session-rewrite' AND uuid='u-1'"); got != "Updated line 1" {
		t.Errorf("content = %q, want 'Updated line 1'", got)
	}
}

// TestIncrementalIngest_FallbackOnMalformedCompleteTail verifies that appending
// corrupt JSON ending in a newline falls back cleanly to full reindex.
func TestIncrementalIngest_FallbackOnMalformedCompleteTail(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "malformed.db")
	f := filepath.Join(dir, "session-malformed.jsonl")
	msg1 := `{"type":"user","message":{"role":"user","content":"Before malformed"},"uuid":"bad-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"After malformed"},"uuid":"bad-2","timestamp":"2026-08-20T10:00:05Z"}`
	writeFile(t, f, msg1+"\n")

	c := source.Container{ID: "session-malformed", Path: f, CWD: "/repo"}
	msgsFn := claudeFullMsgsFn()

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	malformed := `{"type":"user","message":{"role":"user","content":"broken"}` + "\n"
	appendFile(t, f, malformed+msg2+"\n")

	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}
	if got := IncrementalIngestCount.Load(); got != 0 {
		t.Errorf("IncrementalIngestCount = %d, want 0 after malformed complete tail", got)
	}
	if got := FullReindexCount.Load(); got != 2 {
		t.Errorf("FullReindexCount = %d, want 2 after malformed complete tail", got)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	if got := scalarInt(t, con, "SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID); got != 2 {
		t.Errorf("message count after malformed tail = %d, want 2", got)
	}
}

// TestEnsureFreshContainer_IncompleteTrailingLine verifies that EnsureFreshContainer
// ignores uncompleted trailing lines without crashing or polluting state.
func TestEnsureFreshContainer_IncompleteTrailingLine(t *testing.T) {
	isolateCache(t)
	f := filepath.Join(t.TempDir(), "session.jsonl")
	msg1 := `{"type":"user","message":{"role":"user","content":"Complete message"},"uuid":"strict-1","timestamp":"2026-08-20T10:00:00Z"}`
	writeFile(t, f, msg1+"\n")

	c := source.Container{ID: "strict-partial", Path: f, CWD: "/work"}
	msgsFn := claudeTailMsgsFn()
	dbp := RefreshDBPath("claude", c.ID, c.Path)

	if n, err := EnsureFreshContainer(dbp, c, msgsFn, "claude"); err != nil || n != 1 {
		t.Fatalf("initial refresh n=%d err=%v, want one message and no error", n, err)
	}

	partial := `{"type":"assistant","message":{"role":"assistant","content":"still writing`
	appendFile(t, f, partial)

	if n, err := EnsureFreshContainer(dbp, c, msgsFn, "claude"); err != nil || n != 1 {
		t.Fatalf("partial refresh n=%d err=%v, want one message and no error", n, err)
	}
}

// TestAppendContainer_StaleWatermarkIsNoOp tests concurrency protection on appendContainer.
func TestAppendContainer_StaleWatermarkIsNoOp(t *testing.T) {
	isolateCache(t)
	dir := t.TempDir()
	dbp := filepath.Join(dir, "stale-append.db")
	f := filepath.Join(dir, "session.jsonl")
	msg1 := `{"type":"user","message":{"role":"user","content":"First"},"uuid":"cas-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Second"},"uuid":"cas-2","timestamp":"2026-08-20T10:00:05Z"}`
	initial := msg1 + "\n" + msg2 + "\n"
	writeFile(t, f, initial)

	c := source.Container{ID: "cas-session", Path: f, CWD: "/work"}
	msgsFn := claudeTailMsgsFn()
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	msg3 := `{"type":"user","message":{"role":"user","content":"Third"},"uuid":"cas-3","timestamp":"2026-08-20T10:01:00Z"}` + "\n"
	appendFile(t, f, msg3)

	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	oldSize := int64(len(initial))
	st, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	tailMs, newOffset, ok := parseTailMessages(con, c, "claude", f, oldSize, st.Size())
	if !ok || len(tailMs) != 1 {
		t.Fatalf("tail parse ok=%v messages=%d, want one message", ok, len(tailMs))
	}
	newFP := checkPrefixFingerprint(f, newOffset)
	if err := appendContainer(con, c, tailMs, "claude", "", realpath(f), oldSize, mtimeOf(st), newOffset, newFP); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := appendContainer(con, c, tailMs, "claude", "", realpath(f), oldSize, mtimeOf(st), newOffset, newFP); !errors.Is(err, errAppendStale) {
		t.Fatalf("second stale append error=%v, want errAppendStale", err)
	}

	if got := scalarInt(t, con, "SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID); got != 3 {
		t.Errorf("message count after stale append = %d, want 3", got)
	}
}

// TestEnsureFreshContainer_IncrementalEndToEnd proves that EnsureFreshContainer
// refreshes the private cache incrementally and publishes the updated messages to consolidated.db.
func TestEnsureFreshContainer_IncrementalEndToEnd(t *testing.T) {
	isolateCache(t)
	dir := t.TempDir()
	sessID := "fresh-sess-1234"
	f := filepath.Join(dir, "session.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"First query"},"uuid":"u-101","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"First reply"},"uuid":"u-102","timestamp":"2026-08-20T10:00:05Z"}`
	writeFile(t, f, msg1+"\n"+msg2+"\n")

	c := source.Container{ID: sessID, Path: f, CWD: "/work"}
	msgsFn := claudeTailMsgsFn()
	dbp := RefreshDBPath("claude", c.ID, c.Path)

	ResetIngestCountersForTesting()

	// 1. Initial EnsureFreshContainer
	n1, err := EnsureFreshContainer(dbp, c, msgsFn, "claude")
	if err != nil {
		t.Fatalf("initial EnsureFreshContainer: %v", err)
	}
	if n1 != 2 {
		t.Fatalf("initial message count = %d, want 2", n1)
	}

	// 2. Append new message
	msg3 := `{"type":"user","message":{"role":"user","content":"Second query appended"},"uuid":"u-103","timestamp":"2026-08-20T10:01:00Z"}`
	appendFile(t, f, msg3+"\n")

	// 3. Second EnsureFreshContainer -> MUST take incremental path
	n2, err := EnsureFreshContainer(dbp, c, msgsFn, "claude")
	if err != nil {
		t.Fatalf("second EnsureFreshContainer: %v", err)
	}
	if n2 != 3 {
		t.Fatalf("second message count = %d, want 3", n2)
	}

	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
		t.Errorf("IncrementalIngestCount = %d, want 1", gotIncr)
	}

	// 4. Verify consolidated store has all 3 messages
	con := openConsolidated(t)
	if got := scalarInt(t, con, "SELECT message_count FROM sessions WHERE id=?", sessID); got != 3 {
		t.Errorf("consolidated message_count = %d, want 3", got)
	}
	if got := scalarInt(t, con, "SELECT COUNT(*) FROM messages WHERE session_id=?", sessID); got != 3 {
		t.Errorf("consolidated messages row count = %d, want 3", got)
	}
}
