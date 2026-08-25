package index

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestIncrementalIngest_AppendFastPath_Claude verifies that append-only growth
// takes the incremental fast path, increments the counter, and yields a database
// state byte-equal to a clean full reindex.
func TestIncrementalIngest_AppendFastPath_Claude(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "claude-incr.db")
	dbRef := filepath.Join(dir, "claude-ref.db")
	f := filepath.Join(dir, "session-c1.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"How do I configure redis cache pool?"},"uuid":"u-101","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Configure redis with PoolSize and MinIdleConns."},"uuid":"u-102","timestamp":"2026-08-20T10:00:05Z"}`

	initialContent := msg1 + "\n" + msg2 + "\n"
	if err := os.WriteFile(f, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "session-c1", Path: f, CWD: "/repo"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		data, err := os.ReadFile(got.Path)
		if err != nil {
			return nil, err
		}
		return parseClaudeTail(data)
	}

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

	// 2. Append a complete message followed by a half-written message. The
	// incremental watermark must anchor at the last complete newline, not EOF.
	msg3 := `{"type":"user","message":{"role":"user","content":"What about connection timeouts?"},"uuid":"u-103","timestamp":"2026-08-20T10:01:00Z"}`
	msg4Partial := `{"type":"assistant","message":{"role":"assistant","content":"Set DialTimeout to 5s`
	appendedContent := msg3 + "\n" + msg4Partial

	fHandle, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fHandle.WriteString(appendedContent); err != nil {
		t.Fatal(err)
	}
	_ = fHandle.Close()

	// 3. Second index pass -> MUST use incremental fast path and retain the
	// complete-lines prefix watermark before the partial record.
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

	// 4. Complete the pending record and ingest again. This must also use the
	// incremental path; otherwise the stored fingerprint was anchored at EOF.
	msg4Remainder := `s and ReadTimeout to 3s."},"uuid":"u-104","timestamp":"2026-08-20T10:01:05Z"}` + "\n"
	fHandle2, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fHandle2.WriteString(msg4Remainder); err != nil {
		t.Fatal(err)
	}
	_ = fHandle2.Close()

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

	var msgCount int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id='session-c1'").Scan(&msgCount); err != nil {
		t.Fatal(err)
	}
	if msgCount != 4 {
		t.Errorf("message_count = %d, want 4", msgCount)
	}

	var rowCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='session-c1'").Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 4 {
		t.Errorf("messages row count = %d, want 4", rowCount)
	}

	// 6. Compare against reference full reindex on dbRef: rows must be identical
	claudeFullMsgs := func(got source.Container) ([]model.Message, error) {
		data, err := os.ReadFile(got.Path)
		if err != nil {
			return nil, err
		}
		var out []model.Message
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var o map[string]any
			if err := json.Unmarshal([]byte(line), &o); err != nil {
				continue
			}
			if !indexable(o) {
				continue
			}
			text := parse.ExtractText(o)
			if text == "" {
				continue
			}
			iso, _ := o["timestamp"].(string)
			out = append(out, model.Message{
				Role:  parse.MsgRole(o),
				Text:  text,
				TS:    parse.ISOToEpoch(iso),
				TSISO: iso,
				UUID:  parse.MsgUUID(o),
			})
		}
		return out, nil
	}

	_, _, err = EnsureIndexedContainers(dbRef, true, []source.Container{c}, claudeFullMsgs, "claude", "")
	if err != nil {
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

	if err := os.WriteFile(f, []byte(header+"\n"+item1+"\n"+item2+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "session-codex-1", Path: f, CWD: "/workspace"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "Implement Raft consensus", TS: 1, TSISO: "2026-08-20T10:00:00Z", UUID: mintCodexUUID("session-codex-1", 0)},
			{Role: "assistant", Text: "Raft implementation starting with leader election.", TS: 2, TSISO: "2026-08-20T10:00:05Z", UUID: mintCodexUUID("session-codex-1", 1)},
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

	fHandle, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fHandle.WriteString(item3 + "\n" + item4 + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = fHandle.Close()

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

	var msgCount int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id='session-codex-1'").Scan(&msgCount); err != nil {
		t.Fatal(err)
	}
	if msgCount != 4 {
		t.Errorf("message_count = %d, want 4", msgCount)
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
		want := mintCodexUUID("session-codex-1", i)
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

	if err := os.WriteFile(f, []byte(step1+"\n"+step2+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "session-agy-1", Path: f, CWD: "/workspace"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "Fix memory leak", TS: 1, TSISO: "2026-08-20T10:00:00Z", UUID: mintAntigravityUUID("session-agy-1", 0, 0)},
			{Role: "assistant", Text: "[THINKING] Profile reveals unclosed channels.\nInvestigating heap profile.", TS: 2, TSISO: "2026-08-20T10:00:05Z", UUID: mintAntigravityUUID("session-agy-1", 1, 1)},
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

	fHandle, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fHandle.WriteString(step3 + "\n" + step4 + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = fHandle.Close()

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

	var msgCount int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id='session-agy-1'").Scan(&msgCount); err != nil {
		t.Fatal(err)
	}
	if msgCount != 4 {
		t.Errorf("message_count = %d, want 4", msgCount)
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
	if err := os.WriteFile(f, []byte(fullContent), 0o644); err != nil {
		t.Fatal(err)
	}

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
	if err := os.WriteFile(f, []byte(shortContent), 0o644); err != nil {
		t.Fatal(err)
	}

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

	var msgCount int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id='session-trunc'").Scan(&msgCount); err != nil {
		t.Fatal(err)
	}
	if msgCount != 2 {
		t.Errorf("message_count after truncation = %d, want 2", msgCount)
	}

	var rowCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='session-trunc'").Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 2 {
		t.Errorf("messages row count after truncation = %d, want 2 (no stale rows)", rowCount)
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

	initialContent := msg1 + "\n" + msg2 + "\n"
	if err := os.WriteFile(f, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "session-rewrite", Path: f, CWD: "/repo"}
	msgsFn1 := func(got source.Container) ([]model.Message, error) {
		return parseClaudeTail([]byte(initialContent))
	}

	ResetIngestCountersForTesting()

	// Initial index
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn1, "claude", ""); err != nil {
		t.Fatal(err)
	}

	// Rewrite line 1 in place and append line 3 (file is larger, but head is modified)
	msg1Rewritten := `{"type":"user","message":{"role":"user","content":"REWRITTEN line 1"},"uuid":"u-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg3 := `{"type":"user","message":{"role":"user","content":"Appended line 3"},"uuid":"u-3","timestamp":"2026-08-20T10:00:10Z"}`

	rewrittenContent := msg1Rewritten + "\n" + msg2 + "\n" + msg3 + "\n"
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(f, []byte(rewrittenContent), 0o644); err != nil {
		t.Fatal(err)
	}

	msgsFn2 := func(got source.Container) ([]model.Message, error) {
		return parseClaudeTail([]byte(rewrittenContent))
	}

	// Reindex: head fingerprint mismatch must fall back to full reindex
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn2, "claude", ""); err != nil {
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

	var firstContent string
	if err := con.QueryRow("SELECT content FROM messages WHERE session_id='session-rewrite' AND uuid='u-1'").Scan(&firstContent); err != nil {
		t.Fatal(err)
	}
	if firstContent != "REWRITTEN line 1" {
		t.Errorf("content = %q, want 'REWRITTEN line 1'", firstContent)
	}
}

func TestIncrementalIngest_FallbackOnMalformedCompleteTail(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "malformed.db")
	f := filepath.Join(dir, "session-malformed.jsonl")
	msg1 := `{"type":"user","message":{"role":"user","content":"Before malformed"},"uuid":"bad-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"After malformed"},"uuid":"bad-2","timestamp":"2026-08-20T10:00:05Z"}`
	initial := msg1 + "\n"
	if err := os.WriteFile(f, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	c := source.Container{ID: "session-malformed", Path: f, CWD: "/repo"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		data, err := os.ReadFile(got.Path)
		if err != nil {
			return nil, err
		}
		var out []model.Message
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var o map[string]any
			if err := json.Unmarshal([]byte(line), &o); err != nil || !indexable(o) {
				continue
			}
			text := parse.ExtractText(o)
			if text == "" {
				continue
			}
			iso, _ := o["timestamp"].(string)
			out = append(out, model.Message{Role: parse.MsgRole(o), Text: text, TS: parse.ISOToEpoch(iso), TSISO: iso, UUID: parse.MsgUUID(o)})
		}
		return out, nil
	}

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}
	malformed := `{"type":"user","message":{"role":"user","content":"broken"}` + "\n"
	if fHandle, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644); err != nil {
		t.Fatal(err)
	} else {
		if _, err := fHandle.WriteString(malformed + msg2 + "\n"); err != nil {
			t.Fatal(err)
		}
		_ = fHandle.Close()
	}

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
	var count int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("message count after malformed tail = %d, want 2", count)
	}
}

// TestIncrementalIngest_IncompleteTrailingLine verifies that an incomplete line
// (no trailing newline) is not partially ingested and completes on subsequent growth.
func TestIncrementalIngest_IncompleteTrailingLine(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "partial.db")
	f := filepath.Join(dir, "session-partial.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"Complete message 1"},"uuid":"u-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Complete message 2"},"uuid":"u-2","timestamp":"2026-08-20T10:00:05Z"}`

	initialContent := msg1 + "\n" + msg2 + "\n"
	if err := os.WriteFile(f, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "session-partial", Path: f, CWD: "/repo"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		data, _ := os.ReadFile(got.Path)
		return parseClaudeTail(data)
	}

	ResetIngestCountersForTesting()

	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	// Append a half-written message without newline
	partialLine := `{"type":"user","message":{"role":"user","content":"Half written message`
	fHandle, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fHandle.WriteString(partialLine); err != nil {
		t.Fatal(err)
	}
	_ = fHandle.Close()

	// Ingest during partial write -> readTailChunk finds no trailing newline, falls back cleanly
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	var msgCount int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id='session-partial'").Scan(&msgCount); err != nil {
		t.Fatal(err)
	}
	if msgCount != 2 {
		t.Errorf("message_count during partial write = %d, want 2", msgCount)
	}

	// Now complete the third message and add a fourth message
	remaining := ` 3"},"uuid":"u-3","timestamp":"2026-08-20T10:01:00Z"}` + "\n"
	msg4 := `{"type":"assistant","message":{"role":"assistant","content":"Complete message 4"},"uuid":"u-4","timestamp":"2026-08-20T10:01:05Z"}` + "\n"

	fHandle2, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fHandle2.WriteString(remaining + msg4); err != nil {
		t.Fatal(err)
	}
	_ = fHandle2.Close()

	// Ingest after completion
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	var msgCount2 int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id='session-partial'").Scan(&msgCount2); err != nil {
		t.Fatal(err)
	}
	if msgCount2 != 4 {
		t.Errorf("message_count after completion = %d, want 4", msgCount2)
	}
}

func TestEnsureFreshContainer_IncompleteTrailingLine(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	f := filepath.Join(cfg, "session.jsonl")
	msg1 := `{"type":"user","message":{"role":"user","content":"Complete message"},"uuid":"strict-1","timestamp":"2026-08-20T10:00:00Z"}`
	if err := os.WriteFile(f, []byte(msg1+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: "strict-partial", Path: f, CWD: "/work"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		data, err := os.ReadFile(got.Path)
		if err != nil {
			return nil, err
		}
		return parseClaudeTail(data)
	}
	dbp := RefreshDBPath("claude", c.ID, c.Path)

	if n, err := EnsureFreshContainer(dbp, c, msgsFn, "claude"); err != nil || n != 1 {
		t.Fatalf("initial refresh n=%d err=%v, want one message and no error", n, err)
	}

	partial := `{"type":"assistant","message":{"role":"assistant","content":"still writing`
	fHandle, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fHandle.WriteString(partial); err != nil {
		t.Fatal(err)
	}
	_ = fHandle.Close()

	if n, err := EnsureFreshContainer(dbp, c, msgsFn, "claude"); err != nil || n != 1 {
		t.Fatalf("partial refresh n=%d err=%v, want one message and no error", n, err)
	}
}

func TestAppendContainer_StaleWatermarkIsNoOp(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	dbp := filepath.Join(cfg, "stale-append.db")
	f := filepath.Join(cfg, "session.jsonl")
	msg1 := `{"type":"user","message":{"role":"user","content":"First"},"uuid":"cas-1","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Second"},"uuid":"cas-2","timestamp":"2026-08-20T10:00:05Z"}`
	initial := msg1 + "\n" + msg2 + "\n"
	if err := os.WriteFile(f, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	c := source.Container{ID: "cas-session", Path: f, CWD: "/work"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		data, err := os.ReadFile(got.Path)
		if err != nil {
			return nil, err
		}
		return parseClaudeTail(data)
	}
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatal(err)
	}

	msg3 := `{"type":"user","message":{"role":"user","content":"Third"},"uuid":"cas-3","timestamp":"2026-08-20T10:01:00Z"}` + "\n"
	if fHandle, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644); err != nil {
		t.Fatal(err)
	} else {
		if _, err := fHandle.WriteString(msg3); err != nil {
			t.Fatal(err)
		}
		_ = fHandle.Close()
	}

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

	var count int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("message count after stale append = %d, want 3", count)
	}
}

// TestEnsureFreshContainer_IncrementalEndToEnd proves that EnsureFreshContainer
// refreshes the private cache incrementally and publishes the updated messages to consolidated.db.
func TestEnsureFreshContainer_IncrementalEndToEnd(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cfg, ".cache"))

	sessID := "fresh-sess-1234"
	f := filepath.Join(cfg, "session.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"First query"},"uuid":"u-101","timestamp":"2026-08-20T10:00:00Z"}`
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"First reply"},"uuid":"u-102","timestamp":"2026-08-20T10:00:05Z"}`
	if err := os.WriteFile(f, []byte(msg1+"\n"+msg2+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := source.Container{ID: sessID, Path: f, CWD: "/work"}
	msgsFn := func(got source.Container) ([]model.Message, error) {
		data, _ := os.ReadFile(got.Path)
		return parseClaudeTail(data)
	}

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
	fHandle, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fHandle.WriteString(msg3 + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = fHandle.Close()

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
	con, err := store.ConnectRO(ConsolidatedPath())
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	var msgCount int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id=?", sessID).Scan(&msgCount); err != nil {
		t.Fatal(err)
	}
	if msgCount != 3 {
		t.Errorf("consolidated message_count = %d, want 3", msgCount)
	}

	var rowCount int
	if err := con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", sessID).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 3 {
		t.Errorf("consolidated messages row count = %d, want 3", rowCount)
	}
}
