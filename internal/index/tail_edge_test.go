package index

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/antigravity"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestTailEdge_EmptyTail covers empty tail conditions:
// 1. readTailChunk with fromOffset == toOffset, fromOffset > toOffset, and fromOffset < 0.
// 2. parseTailMessages when fromOffset == toOffset.
// 3. Appending only blank newlines to an indexed transcript.
// 4. Initial empty (0-byte) file behavior.
func TestTailEdge_EmptyTail(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty_tail.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"Initial message"},"uuid":"u-empty-1","timestamp":"2026-08-25T10:00:00Z"}` + "\n"
	writeFile(t, f, msg1)
	fileSize := int64(len(msg1))

	// 1. Direct unit checks on readTailChunk boundaries
	chunk, newOffset, ok, pending := readTailChunk(f, fileSize, fileSize)
	if ok || pending || chunk != nil || newOffset != fileSize {
		t.Errorf("readTailChunk(from == to) = (chunk=%v, offset=%d, ok=%v, pending=%v), want (nil, %d, false, false)", chunk, newOffset, ok, pending, fileSize)
	}

	chunk, newOffset, ok, pending = readTailChunk(f, fileSize+10, fileSize)
	if ok || pending || chunk != nil || newOffset != fileSize+10 {
		t.Errorf("readTailChunk(from > to) = (chunk=%v, offset=%d, ok=%v, pending=%v), want (nil, %d, false, false)", chunk, newOffset, ok, pending, fileSize+10)
	}

	chunk, newOffset, ok, pending = readTailChunk(f, -1, fileSize)
	if ok || pending || chunk != nil || newOffset != -1 {
		t.Errorf("readTailChunk(from < 0) = (chunk=%v, offset=%d, ok=%v, pending=%v), want (nil, -1, false, false)", chunk, newOffset, ok, pending)
	}

	chunk, newOffset, ok, pending = readTailChunk(filepath.Join(dir, "nonexistent.jsonl"), 0, 10)
	if ok || pending || chunk != nil || newOffset != 0 {
		t.Errorf("readTailChunk(nonexistent) = (chunk=%v, offset=%d, ok=%v, pending=%v), want (nil, 0, false, false)", chunk, newOffset, ok, pending)
	}

	// 2. parseTailMessages when fromOffset == toOffset
	c := source.Container{ID: "sess-empty-tail", Path: f, CWD: "/work"}
	dbp := filepath.Join(dir, "empty_tail.db")
	msgsFn := claudeTailMsgsFn()

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("initial indexing: %v", err)
	}

	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	tailMsgs, retOffset, parseOk := parseTailMessages(con, c, "claude", f, fileSize, fileSize)
	if parseOk || len(tailMsgs) != 0 || retOffset != fileSize {
		t.Errorf("parseTailMessages(from == to) = (msgs=%v, offset=%d, ok=%v), want (nil, %d, false)", tailMsgs, retOffset, parseOk, fileSize)
	}

	// 3. Append only newlines (blank lines) after watermark
	appendFile(t, f, "\n\n\n")

	ResetIngestCountersForTesting()
	n, status, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", "")
	if err != nil {
		t.Fatalf("indexing after blank lines: %v", err)
	}
	if status != IndexFresh || n != 1 {
		t.Errorf("status = %v, n = %d, want IndexFresh, 1", status, n)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
		t.Errorf("IncrementalIngestCount after blank lines = %d, want 1", gotIncr)
	}
	if got := scalarInt(t, con, "SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID); got != 1 {
		t.Errorf("message count after blank lines = %d, want 1", got)
	}

	// 4. Initial 0-byte empty file behavior:
	// An empty file has prev.size = 0. Appending first message requires full reindex
	// because incremental path is guarded by `prev.size > 0`.
	emptyF := filepath.Join(dir, "zero_byte.jsonl")
	writeFile(t, emptyF, "")
	emptyC := source.Container{ID: "sess-zero-byte", Path: emptyF, CWD: "/work"}
	emptyDBP := filepath.Join(dir, "zero_byte.db")

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(emptyDBP, true, []source.Container{emptyC}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("initial zero byte indexing: %v", err)
	}
	if gotFull := FullReindexCount.Load(); gotFull != 1 {
		t.Errorf("FullReindexCount for 0-byte file = %d, want 1", gotFull)
	}

	// Write first message to the zero-byte file
	writeFile(t, emptyF, msg1)
	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(emptyDBP, false, []source.Container{emptyC}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("indexing after first content in 0-byte file: %v", err)
	}
	if gotFull := FullReindexCount.Load(); gotFull != 1 {
		t.Errorf("FullReindexCount after first content in 0-byte file = %d, want 1", gotFull)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 0 {
		t.Errorf("IncrementalIngestCount = %d, want 0", gotIncr)
	}
}

// TestTailEdge_WatermarkExactlyAtFileEnd verifies that when a watermark is exactly
// at the file end (file unchanged):
// 1. EnsureIndexedContainers performs a no-op without re-indexing.
// 2. EnsureFreshContainer verifies the container as fresh and returns the exact count.
// 3. checkPrefixFingerprint at file length matches provenance.FileFingerprint.
func TestTailEdge_WatermarkExactlyAtFileEnd(t *testing.T) {
	cfg := isolateCache(t)

	f := filepath.Join(cfg, "at_end.jsonl")
	msg1 := `{"type":"user","message":{"role":"user","content":"Watermark at end query"},"uuid":"u-end-1","timestamp":"2026-08-25T10:00:00Z"}` + "\n"
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Watermark at end response"},"uuid":"u-end-2","timestamp":"2026-08-25T10:00:05Z"}` + "\n"
	writeFile(t, f, msg1+msg2)

	c := source.Container{ID: "sess-at-end", Path: f, CWD: "/work"}
	dbp := RefreshDBPath("claude", c.ID, c.Path)
	msgsFn := claudeTailMsgsFn()

	// 1. Initial indexing
	ResetIngestCountersForTesting()
	n, err := EnsureFreshContainer(dbp, c, msgsFn, "claude")
	if err != nil {
		t.Fatalf("initial EnsureFreshContainer: %v", err)
	}
	if n != 2 {
		t.Fatalf("message count = %d, want 2", n)
	}

	// 2. Immediate re-check with watermark exactly at file end
	ResetIngestCountersForTesting()
	n2, status, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", "")
	if err != nil {
		t.Fatalf("re-index unchanged file: %v", err)
	}
	if status != IndexFresh || n2 != 1 {
		t.Errorf("status = %v, n = %d, want IndexFresh, 1", status, n2)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 0 {
		t.Errorf("IncrementalIngestCount = %d, want 0 on unchanged file", gotIncr)
	}
	if gotFull := FullReindexCount.Load(); gotFull != 0 {
		t.Errorf("FullReindexCount = %d, want 0 on unchanged file", gotFull)
	}

	// 3. EnsureFreshContainer on unchanged file
	nFresh, err := EnsureFreshContainer(dbp, c, msgsFn, "claude")
	if err != nil {
		t.Fatalf("EnsureFreshContainer on unchanged file: %v", err)
	}
	if nFresh != 2 {
		t.Errorf("EnsureFreshContainer count = %d, want 2", nFresh)
	}

	// 4. checkPrefixFingerprint vs provenance.FileFingerprint
	st, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	prefixFP := checkPrefixFingerprint(f, st.Size())
	fileFP := provenance.FileFingerprint(f, st.Size())
	if prefixFP != fileFP {
		t.Errorf("checkPrefixFingerprint(%d) = %q, want %q", st.Size(), prefixFP, fileFP)
	}

	// Edge checks on checkPrefixFingerprint
	if fp := checkPrefixFingerprint(f, 0); fp != "" {
		t.Errorf("checkPrefixFingerprint(0) = %q, want empty string", fp)
	}
	if fp := checkPrefixFingerprint(f, -5); fp != "" {
		t.Errorf("checkPrefixFingerprint(-5) = %q, want empty string", fp)
	}
	if fp := checkPrefixFingerprint(filepath.Join(cfg, "nonexistent.jsonl"), 100); fp != "" {
		t.Errorf("checkPrefixFingerprint(nonexistent) = %q, want empty string", fp)
	}
}

// TestTailEdge_TruncatedTailRecord covers truncated and malformed records:
// 1. Incomplete line without trailing newline for Codex and Antigravity.
// 2. Claude non-indexable record in tail (falls back to full reindex).
// 3. Unknown sourceID and SQLite DB backing path fallback.
func TestTailEdge_TruncatedTailRecord(t *testing.T) {
	dir := t.TempDir()

	// --- 1. Codex incomplete line without newline ---
	fCodex := filepath.Join(dir, "codex_trunc.jsonl")
	header := `{"type":"session_meta","payload":{"id":"sess-codex-trunc","cwd":"/work"}}` + "\n"
	item1 := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Codex prompt"}]},"timestamp":"2026-08-25T10:00:00Z"}` + "\n"
	writeFile(t, fCodex, header+item1)

	cCodex := source.Container{ID: "sess-codex-trunc", Path: fCodex, CWD: "/work"}
	dbCodex := filepath.Join(dir, "codex_trunc.db")
	msgsCodex := func(got source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "Codex prompt", TS: 1, TSISO: "2026-08-25T10:00:00Z", UUID: codex.MintUUID(got.ID, 0)},
		}, nil
	}

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbCodex, true, []source.Container{cCodex}, msgsCodex, "codex", ""); err != nil {
		t.Fatalf("codex initial index: %v", err)
	}

	// Append partial Codex line
	partialCodex := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Codex in-progress`
	appendFile(t, fCodex, partialCodex)

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbCodex, false, []source.Container{cCodex}, msgsCodex, "codex", ""); err != nil {
		t.Fatalf("codex partial ingest: %v", err)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 0 {
		t.Errorf("Codex IncrementalIngestCount on partial write = %d, want 0", gotIncr)
	}

	// Complete Codex line
	appendFile(t, fCodex, ` done"}]},"timestamp":"2026-08-25T10:00:05Z"}`+"\n")
	if _, _, err := EnsureIndexedContainers(dbCodex, false, []source.Container{cCodex}, msgsCodex, "codex", ""); err != nil {
		t.Fatalf("codex completed ingest: %v", err)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
		t.Errorf("Codex IncrementalIngestCount after completion = %d, want 1", gotIncr)
	}

	// --- 2. Antigravity incomplete line without newline ---
	fAgy := filepath.Join(dir, "agy_trunc.jsonl")
	agyStep1 := `{"step_index":0,"type":"USER_INPUT","source":"USER_EXPLICIT","content":"<USER_REQUEST>Agy prompt</USER_REQUEST>","created_at":"2026-08-25T10:00:00Z"}` + "\n"
	writeFile(t, fAgy, agyStep1)

	cAgy := source.Container{ID: "sess-agy-trunc", Path: fAgy, CWD: "/work"}
	dbAgy := filepath.Join(dir, "agy_trunc.db")
	msgsAgy := func(got source.Container) ([]model.Message, error) {
		return []model.Message{
			{Role: "user", Text: "Agy prompt", TS: 1, TSISO: "2026-08-25T10:00:00Z", UUID: antigravity.MintUUID(got.ID, 0, 0)},
		}, nil
	}

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbAgy, true, []source.Container{cAgy}, msgsAgy, "antigravity", ""); err != nil {
		t.Fatalf("agy initial index: %v", err)
	}

	// Append partial Antigravity step
	partialAgy := `{"step_index":1,"type":"PLANNER_RESPONSE","source":"MODEL","content":"Writing code...`
	appendFile(t, fAgy, partialAgy)

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbAgy, false, []source.Container{cAgy}, msgsAgy, "antigravity", ""); err != nil {
		t.Fatalf("agy partial ingest: %v", err)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 0 {
		t.Errorf("Antigravity IncrementalIngestCount on partial write = %d, want 0", gotIncr)
	}

	// Complete Antigravity line
	appendFile(t, fAgy, ` done","created_at":"2026-08-25T10:00:05Z"}`+"\n")
	if _, _, err := EnsureIndexedContainers(dbAgy, false, []source.Container{cAgy}, msgsAgy, "antigravity", ""); err != nil {
		t.Fatalf("agy completed ingest: %v", err)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
		t.Errorf("Antigravity IncrementalIngestCount after completion = %d, want 1", gotIncr)
	}

	// --- 3. Claude non-indexable record type in tail (falls back to full reindex) ---
	fNonIdx := filepath.Join(dir, "non_idx.jsonl")
	msgC1 := `{"type":"user","message":{"role":"user","content":"Claude query 1"},"uuid":"uc-1","timestamp":"2026-08-25T10:00:00Z"}` + "\n"
	writeFile(t, fNonIdx, msgC1)

	cNonIdx := source.Container{ID: "sess-non-idx", Path: fNonIdx, CWD: "/work"}
	dbNonIdx := filepath.Join(dir, "non_idx.db")
	corruptMsgsFn := claudeFullMsgsFn()

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbNonIdx, true, []source.Container{cNonIdx}, corruptMsgsFn, "claude", ""); err != nil {
		t.Fatalf("non-idx initial index: %v", err)
	}

	// Append non-indexable progress line
	appendFile(t, fNonIdx, `{"type":"progress","data":{"percent":50}}`+"\n")

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbNonIdx, false, []source.Container{cNonIdx}, corruptMsgsFn, "claude", ""); err != nil {
		t.Fatalf("ingest after non-indexable tail line: %v", err)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 0 {
		t.Errorf("IncrementalIngestCount on non-indexable tail = %d, want 0", gotIncr)
	}
	if gotFull := FullReindexCount.Load(); gotFull != 1 {
		t.Errorf("FullReindexCount on non-indexable tail = %d, want 1 (fallback)", gotFull)
	}

	// --- 4. Unsupported source and container path with '#' or '.db' ---
	conClaude, err := store.ConnectRO(dbNonIdx)
	if err != nil {
		t.Fatal(err)
	}
	defer conClaude.Close()

	stNonIdx, err := os.Stat(fNonIdx)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, ok := parseTailMessages(conClaude, cNonIdx, "unknown_source", fNonIdx, 0, stNonIdx.Size()); ok {
		t.Error("parseTailMessages with unknown source and complete chunk returned ok=true, want false")
	}
	if msgs, off, ok := parseTailMessages(conClaude, cNonIdx, "unknown_source", fNonIdx, 0, 50); !ok || len(msgs) != 0 || off != 0 {
		t.Errorf("parseTailMessages with pending chunk returned (msgs=%v, off=%d, ok=%v), want (nil, 0, true)", msgs, off, ok)
	}
	cFragment := source.Container{ID: "sess-frag", Path: fNonIdx + "#frag", CWD: "/work"}
	if _, _, ok := parseTailMessages(conClaude, cFragment, "claude", fNonIdx, 0, stNonIdx.Size()); ok {
		t.Error("parseTailMessages with # container path returned ok=true, want false")
	}
	cDB := source.Container{ID: "sess-db", Path: "/path/to/source.db", CWD: "/work"}
	if _, _, ok := parseTailMessages(conClaude, cDB, "claude", "/path/to/source.db", 0, stNonIdx.Size()); ok {
		t.Error("parseTailMessages with .db path returned ok=true, want false")
	}
}

// TestTailEdge_ContentAppendedAfterWatermark covers:
// 1. Multiple sequential appends (single and multi-message batches).
// 2. Preservation of session metadata (started_at preserved, last_ts advanced, message_count accurate).
// 3. Message order, content, and UUIDs across appends.
// 4. Large transcript append exceeding 8192 bytes (verifying head+tail dual fingerprint windowing).
func TestTailEdge_ContentAppendedAfterWatermark(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "sequential_appends.db")
	f := filepath.Join(dir, "session_seq.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"Question 1"},"uuid":"u-seq-1","timestamp":"2026-08-25T10:00:00Z"}` + "\n"
	writeFile(t, f, msg1)

	c := source.Container{ID: "sess-seq", Path: f, CWD: "/work"}
	msgsFn := claudeTailMsgsFn()

	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("initial indexing: %v", err)
	}

	steps := []struct {
		name      string
		content   string
		wantTotal int
		wantLast  string
	}{
		{
			name:      "single message append",
			content:   `{"type":"assistant","message":{"role":"assistant","content":"Answer 1"},"uuid":"u-seq-2","timestamp":"2026-08-25T10:00:05Z"}` + "\n",
			wantTotal: 2,
			wantLast:  "2026-08-25T10:00:05Z",
		},
		{
			name: "batch of two messages",
			content: `{"type":"user","message":{"role":"user","content":"Question 2"},"uuid":"u-seq-3","timestamp":"2026-08-25T10:01:00Z"}` + "\n" +
				`{"type":"assistant","message":{"role":"assistant","content":"Answer 2"},"uuid":"u-seq-4","timestamp":"2026-08-25T10:01:05Z"}` + "\n",
			wantTotal: 4,
			wantLast:  "2026-08-25T10:01:05Z",
		},
		{
			name:      "another single message",
			content:   `{"type":"user","message":{"role":"user","content":"Question 3"},"uuid":"u-seq-5","timestamp":"2026-08-25T10:02:00Z"}` + "\n",
			wantTotal: 5,
			wantLast:  "2026-08-25T10:02:00Z",
		},
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	for i, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			appendFile(t, f, step.content)

			ResetIngestCountersForTesting()
			n, status, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", "")
			if err != nil {
				t.Fatalf("indexing step %d: %v", i, err)
			}
			if status != IndexFresh || n != 1 {
				t.Errorf("status = %v, n = %d, want IndexFresh, 1", status, n)
			}
			if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
				t.Errorf("IncrementalIngestCount step %d = %d, want 1", i, gotIncr)
			}
			if gotFull := FullReindexCount.Load(); gotFull != 0 {
				t.Errorf("FullReindexCount step %d = %d, want 0", i, gotFull)
			}

			var startedAt, lastTS float64
			var messageCount int
			if err := con.QueryRow("SELECT started_at, last_ts, message_count FROM sessions WHERE id='sess-seq'").Scan(&startedAt, &lastTS, &messageCount); err != nil {
				t.Fatal(err)
			}
			wantStarted := parse.ISOToEpoch("2026-08-25T10:00:00Z")
			if startedAt != wantStarted {
				t.Errorf("started_at = %f, want %f (preserved across appends)", startedAt, wantStarted)
			}
			wantLastTS := parse.ISOToEpoch(step.wantLast)
			if lastTS != wantLastTS {
				t.Errorf("last_ts = %f, want %f", lastTS, wantLastTS)
			}
			if messageCount != step.wantTotal {
				t.Errorf("message_count = %d, want %d", messageCount, step.wantTotal)
			}
		})
	}

	// Verify all 5 messages are sequentially stored
	rows, err := con.Query("SELECT uuid FROM messages WHERE session_id='sess-seq' ORDER BY id ASC")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var gotUUIDs []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatal(err)
		}
		gotUUIDs = append(gotUUIDs, u)
	}
	wantUUIDs := []string{"u-seq-1", "u-seq-2", "u-seq-3", "u-seq-4", "u-seq-5"}
	if len(gotUUIDs) != len(wantUUIDs) {
		t.Fatalf("got %d uuids, want %d", len(gotUUIDs), len(wantUUIDs))
	}
	for i := range wantUUIDs {
		if gotUUIDs[i] != wantUUIDs[i] {
			t.Errorf("message %d uuid = %q, want %q", i, gotUUIDs[i], wantUUIDs[i])
		}
	}

	// --- Large transcript append exceeding 8192 bytes ---
	t.Run("large file >8KB dual window fingerprint", func(t *testing.T) {
		fLarge := filepath.Join(dir, "large_session.jsonl")
		dbLarge := filepath.Join(dir, "large_session.db")

		var b strings.Builder
		for i := 0; i < 20; i++ {
			pad := strings.Repeat("A", 450)
			line := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"Msg %d %s"},"uuid":"u-lg-%d","timestamp":"2026-08-25T10:00:00Z"}`+"\n", i, pad, i)
			b.WriteString(line)
		}
		initialLarge := b.String()
		if len(initialLarge) <= 8192 {
			t.Fatalf("initial large content size = %d, want > 8192 bytes", len(initialLarge))
		}
		writeFile(t, fLarge, initialLarge)

		cLarge := source.Container{ID: "sess-large", Path: fLarge, CWD: "/work"}
		msgsLarge := claudeTailMsgsFn()

		ResetIngestCountersForTesting()
		if _, _, err := EnsureIndexedContainers(dbLarge, true, []source.Container{cLarge}, msgsLarge, "claude", ""); err != nil {
			t.Fatalf("large initial index: %v", err)
		}

		// Append 5 more messages (~3KB)
		var appendB strings.Builder
		for i := 20; i < 25; i++ {
			pad := strings.Repeat("B", 450)
			line := fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":"Appended Msg %d %s"},"uuid":"u-lg-%d","timestamp":"2026-08-25T10:01:00Z"}`+"\n", i, pad, i)
			appendB.WriteString(line)
		}
		appendFile(t, fLarge, appendB.String())

		ResetIngestCountersForTesting()
		if _, _, err := EnsureIndexedContainers(dbLarge, false, []source.Container{cLarge}, msgsLarge, "claude", ""); err != nil {
			t.Fatalf("large incremental index: %v", err)
		}
		if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
			t.Errorf("IncrementalIngestCount on large file (>8KB) = %d, want 1", gotIncr)
		}
		if gotFull := FullReindexCount.Load(); gotFull != 0 {
			t.Errorf("FullReindexCount on large file (>8KB) = %d, want 0", gotFull)
		}

		conLg, err := store.ConnectRO(dbLarge)
		if err != nil {
			t.Fatal(err)
		}
		defer conLg.Close()

		if got := scalarInt(t, conLg, "SELECT COUNT(*) FROM messages WHERE session_id='sess-large'"); got != 25 {
			t.Errorf("large session message count = %d, want 25", got)
		}
	})
}

// TestTailEdge_ReingestIdempotency proves that:
// 1. Repeated calls to EnsureIndexedContainers on unchanged file do not re-index.
// 2. Repeated calls after an append do not re-index the tail.
// 3. Repeated calls to EnsureFreshContainer return identical results.
// 4. Ingest safely recovers if a previous transaction was rolled back.
func TestTailEdge_ReingestIdempotency(t *testing.T) {
	cfg := isolateCache(t)
	dbp := filepath.Join(cfg, "idempotent.db")
	f := filepath.Join(cfg, "session_idem.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"Idempotency query 1"},"uuid":"u-id-1","timestamp":"2026-08-25T10:00:00Z"}` + "\n"
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Idempotency answer 1"},"uuid":"u-id-2","timestamp":"2026-08-25T10:00:05Z"}` + "\n"
	writeFile(t, f, msg1+msg2)

	c := source.Container{ID: "sess-idem", Path: f, CWD: "/work"}
	msgsFn := claudeTailMsgsFn()

	// 1. Initial indexing
	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("initial indexing: %v", err)
	}
	if gotFull := FullReindexCount.Load(); gotFull != 1 {
		t.Fatalf("initial FullReindexCount = %d, want 1", gotFull)
	}

	// 2. Five consecutive re-ingest calls on unchanged file -> all zero
	for i := 0; i < 5; i++ {
		ResetIngestCountersForTesting()
		n, status, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", "")
		if err != nil {
			t.Fatalf("re-ingest %d failed: %v", i, err)
		}
		if status != IndexFresh || n != 1 {
			t.Errorf("re-ingest %d status=%v n=%d, want IndexFresh, 1", i, status, n)
		}
		if gotIncr := IncrementalIngestCount.Load(); gotIncr != 0 {
			t.Errorf("re-ingest %d IncrementalIngestCount = %d, want 0", i, gotIncr)
		}
		if gotFull := FullReindexCount.Load(); gotFull != 0 {
			t.Errorf("re-ingest %d FullReindexCount = %d, want 0", i, gotFull)
		}
	}

	// 3. Append msg 3
	msg3 := `{"type":"user","message":{"role":"user","content":"Idempotency query 2"},"uuid":"u-id-3","timestamp":"2026-08-25T10:01:00Z"}` + "\n"
	appendFile(t, f, msg3)

	// Ingest once (incremental fast path)
	ResetIngestCountersForTesting()
	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("first ingest after append: %v", err)
	}
	if gotIncr := IncrementalIngestCount.Load(); gotIncr != 1 {
		t.Errorf("IncrementalIngestCount = %d, want 1", gotIncr)
	}

	// Four subsequent calls on unchanged appended file -> all zero
	for i := 0; i < 4; i++ {
		ResetIngestCountersForTesting()
		n, status, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", "")
		if err != nil {
			t.Fatalf("subsequent re-ingest %d failed: %v", i, err)
		}
		if status != IndexFresh || n != 1 {
			t.Errorf("subsequent re-ingest %d status=%v n=%d, want IndexFresh, 1", i, status, n)
		}
		if gotIncr := IncrementalIngestCount.Load(); gotIncr != 0 {
			t.Errorf("subsequent re-ingest %d IncrementalIngestCount = %d, want 0", i, gotIncr)
		}
		if gotFull := FullReindexCount.Load(); gotFull != 0 {
			t.Errorf("subsequent re-ingest %d FullReindexCount = %d, want 0", i, gotFull)
		}
	}

	// 4. EnsureFreshContainer idempotency
	freshDBP := RefreshDBPath("claude", c.ID, c.Path)
	for i := 0; i < 3; i++ {
		n, err := EnsureFreshContainer(freshDBP, c, msgsFn, "claude")
		if err != nil {
			t.Fatalf("EnsureFreshContainer iteration %d: %v", i, err)
		}
		if n != 3 {
			t.Errorf("EnsureFreshContainer iteration %d count = %d, want 3", i, n)
		}
	}

	// 5. Recovery after simulated failure / rollback during append
	conRW, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conRW.Exec("CREATE TRIGGER test_append_fail BEFORE INSERT ON messages WHEN new.uuid = 'u-fail' BEGIN SELECT RAISE(ABORT, 'forced append error'); END;"); err != nil {
		t.Fatal(err)
	}
	conRW.Close()

	// Append failing message
	msgFail := `{"type":"assistant","message":{"role":"assistant","content":"Fail msg"},"uuid":"u-fail","timestamp":"2026-08-25T10:02:00Z"}` + "\n"
	appendFile(t, f, msgFail)

	// Ingest must fail and rollback cleanly
	_, _, err = EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", "")
	if err == nil {
		t.Fatal("EnsureIndexedContainers should have failed on injected trigger error")
	}

	conRO, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	if got := scalarInt(t, conRO, "SELECT COUNT(*) FROM messages WHERE session_id='sess-idem'"); got != 3 {
		t.Errorf("message count after rollback = %d, want 3", got)
	}
	conRO.Close()

	// Drop trigger and re-ingest: should recover and complete with 4 messages
	conRW, err = store.ConnectRW(dbp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conRW.Exec("DROP TRIGGER test_append_fail"); err != nil {
		t.Fatal(err)
	}
	conRW.Close()

	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("recovery indexing failed: %v", err)
	}

	conRO, err = store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer conRO.Close()
	if got := scalarInt(t, conRO, "SELECT COUNT(*) FROM messages WHERE session_id='sess-idem'"); got != 4 {
		t.Errorf("message count after recovery = %d, want 4", got)
	}
}

// TestTailEdge_InterleavedAppendThenRead verifies interleaved read and write operations:
// 1. Initial write -> Search matches initial keywords.
// 2. Append 1 -> Search matches newly appended keywords immediately.
// 3. Partial write (no newline) -> Search still returns previous messages; partial content not visible.
// 4. Completion -> Search matches newly completed content.
// 5. Concurrent reads during incremental append.
func TestTailEdge_InterleavedAppendThenRead(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "interleaved.db")
	f := filepath.Join(dir, "interleaved.jsonl")

	msg1 := `{"type":"user","message":{"role":"user","content":"How do I configure distributed raft consensus?"},"uuid":"u-int-1","timestamp":"2026-08-25T10:00:00Z"}` + "\n"
	writeFile(t, f, msg1)

	c := source.Container{ID: "sess-interleaved", Path: f, CWD: "/work"}
	msgsFn := claudeTailMsgsFn()

	// 1. Initial indexing
	if _, _, err := EnsureIndexedContainers(dbp, true, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("initial indexing: %v", err)
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()

	assertFTSMatchCount(t, con, "distributed", 1)
	assertFTSMatchCount(t, con, "raft", 1)
	assertFTSMatchCount(t, con, "snapshotting", 0)

	// 2. Append reply mentioning 'snapshotting'
	msg2 := `{"type":"assistant","message":{"role":"assistant","content":"Configure raft with log compaction and state machine snapshotting."},"uuid":"u-int-2","timestamp":"2026-08-25T10:00:05Z"}` + "\n"
	appendFile(t, f, msg2)

	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("second indexing: %v", err)
	}

	assertFTSMatchCount(t, con, "distributed", 1)
	assertFTSMatchCount(t, con, "snapshotting", 1)
	assertFTSMatchCount(t, con, "heartbeat", 0)

	// 3. Append partial query mentioning 'heartbeat' without newline
	partialMsg3 := `{"type":"user","message":{"role":"user","content":"What is the recommended heartbeat interval?`
	appendFile(t, f, partialMsg3)

	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("partial indexing: %v", err)
	}

	assertFTSMatchCount(t, con, "snapshotting", 1)
	assertFTSMatchCount(t, con, "heartbeat", 0)

	// 4. Complete message 3 with newline and append message 4
	msg3Remainder := `"},"uuid":"u-int-3","timestamp":"2026-08-25T10:01:00Z"}` + "\n"
	msg4 := `{"type":"assistant","message":{"role":"assistant","content":"Set heartbeat interval to 150ms and election timeout to 1000ms."},"uuid":"u-int-4","timestamp":"2026-08-25T10:01:05Z"}` + "\n"
	appendFile(t, f, msg3Remainder+msg4)

	if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
		t.Fatalf("completed indexing: %v", err)
	}

	assertFTSMatchCount(t, con, "heartbeat", 2)
	assertFTSMatchCount(t, con, "election", 1)

	// 5. Concurrent read during incremental append
	var wg sync.WaitGroup
	readDone := make(chan struct{})

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rCon, err := store.ConnectRO(dbp)
			if err != nil {
				t.Errorf("concurrent reader connect: %v", err)
				return
			}
			defer rCon.Close()
			for {
				select {
				case <-readDone:
					return
				default:
					var cnt int
					_ = rCon.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='sess-interleaved'").Scan(&cnt)
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	for i := 5; i <= 9; i++ {
		time.Sleep(5 * time.Millisecond)
		line := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"Concurrent message %d"},"uuid":"u-int-%d","timestamp":"2026-08-25T10:02:%02dZ"}`+"\n", i, i, i)
		appendFile(t, f, line)

		if _, _, err := EnsureIndexedContainers(dbp, false, []source.Container{c}, msgsFn, "claude", ""); err != nil {
			t.Fatalf("concurrent ingest step %d: %v", i, err)
		}
	}

	close(readDone)
	wg.Wait()

	if got := scalarInt(t, con, "SELECT COUNT(*) FROM messages WHERE session_id='sess-interleaved'"); got != 9 {
		t.Errorf("final message count after concurrent operations = %d, want 9", got)
	}
}

// TestTailEdge_ParserEdgeCases directly tests edge cases across Claude, Codex,
// and Antigravity tail parsing functions.
func TestTailEdge_ParserEdgeCases(t *testing.T) {
	t.Run("Claude tail parser edge cases", func(t *testing.T) {
		chunk := []byte("\n  \n" +
			`{"type":"user","message":{"role":"user","content":"Line 1"},"uuid":"u-1","timestamp":"2026-08-25T10:00:00Z"}` + "\n" +
			"\t\n" +
			`{"type":"assistant","message":{"role":"assistant","content":"Line 2"},"uuid":"u-2","timestamp":"2026-08-25T10:00:05Z"}` + "\n\n")

		msgs, err := parseClaudeTail(chunk)
		if err != nil {
			t.Fatalf("parseClaudeTail failed on whitespace lines: %v", err)
		}
		if len(msgs) != 2 {
			t.Fatalf("got %d messages, want 2", len(msgs))
		}

		if _, err := parseClaudeTail([]byte(`{"type":"user", broken` + "\n")); err == nil {
			t.Error("parseClaudeTail should fail on malformed JSON")
		}
		if _, err := parseClaudeTail([]byte(`{"type":"file-history-viewer","data":{}}` + "\n")); err == nil {
			t.Error("parseClaudeTail should fail on non-indexable record type")
		}
		if _, err := parseClaudeTail([]byte(`{"type":"user","message":{"role":"user","content":""},"uuid":"u-0"}` + "\n")); err == nil {
			t.Error("parseClaudeTail should fail on empty text")
		}
	})

	t.Run("Codex tail parser edge cases", func(t *testing.T) {
		items := []string{
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"User msg"}]},"timestamp":"2026-08-25T10:00:00Z"}`,
			`{"type":"response_item","payload":{"type":"reasoning","summary":"Thinking process"},"timestamp":"2026-08-25T10:00:01Z"}`,
			`{"type":"response_item","payload":{"type":"function_call","name":"exec","arguments":"ls -la"},"timestamp":"2026-08-25T10:00:02Z"}`,
			`{"type":"response_item","payload":{"type":"function_call_output","output":"total 0"},"timestamp":"2026-08-25T10:00:03Z"}`,
			`{"type":"response_item","payload":{"type":"custom_tool_call","name":"custom","input":"payload"},"timestamp":"2026-08-25T10:00:04Z"}`,
			`{"type":"response_item","payload":{"type":"web_search_call","action":{"query":"sqlite wal"}},"timestamp":"2026-08-25T10:00:05Z"}`,
			`{"type":"response_item","payload":{"type":"tool_search_call","arguments":{"query":"tool_query"}},"timestamp":"2026-08-25T10:00:06Z"}`,
			`{"type":"response_item","payload":{"type":"tool_search_output","output":"found"},"timestamp":"2026-08-25T10:00:07Z"}`,
			`{"type":"response_item","payload":{"type":"image_generation_call","prompt":"sunset over mountains"},"timestamp":"2026-08-25T10:00:08Z"}`,
		}
		chunk := []byte(strings.Join(items, "\n") + "\n")
		msgs, err := parseCodexTail(chunk, "sess-codex-all", 0)
		if err != nil {
			t.Fatalf("parseCodexTail failed on standard items: %v", err)
		}
		if len(msgs) != len(items) {
			t.Fatalf("got %d messages, want %d", len(msgs), len(items))
		}

		if !strings.HasPrefix(msgs[1].Text, "[THINKING]") {
			t.Errorf("reasoning text = %q, want prefix '[THINKING]'", msgs[1].Text)
		}

		metaChunk := []byte(`{"type":"session_meta","payload":{"id":"sess-codex-all"}}` + "\n")
		if _, err := parseCodexTail(metaChunk, "sess-codex-all", 0); err == nil {
			t.Error("parseCodexTail should fail on session_meta in tail")
		}
		if _, err := parseCodexTail([]byte(`{"type":"response_item", broken`+"\n"), "sess-codex-all", 0); err == nil {
			t.Error("parseCodexTail should fail on malformed JSON")
		}
	})

	t.Run("Antigravity tail parser edge cases", func(t *testing.T) {
		steps := []string{
			`{"step_index":0,"type":"USER_INPUT","source":"USER_EXPLICIT","content":"<USER_REQUEST>User prompt here</USER_REQUEST>","created_at":"2026-08-25T10:00:00Z"}`,
			`{"step_index":1,"type":"PLANNER_RESPONSE","source":"MODEL","thinking":"Planning step","content":"Executing command","tool_calls":[{"name":"run_command","args":{"CommandLine":"go test"}}],"created_at":"2026-08-25T10:00:01Z"}`,
			`{"step_index":2,"type":"SYSTEM_MESSAGE","content":"System alert","created_at":"2026-08-25T10:00:02Z"}`,
			`{"step_index":3,"type":"TOOL_OUTPUT","source":"MODEL","content":"PASS","created_at":"2026-08-25T10:00:03Z"}`,
		}
		chunk := []byte(strings.Join(steps, "\n") + "\n")
		msgs, err := parseAntigravityTail(chunk, "sess-agy-all", 0)
		if err != nil {
			t.Fatalf("parseAntigravityTail failed on standard steps: %v", err)
		}
		if len(msgs) != len(steps) {
			t.Fatalf("got %d messages, want %d", len(msgs), len(steps))
		}

		if msgs[0].Text != "User prompt here" {
			t.Errorf("extracted user request = %q, want 'User prompt here'", msgs[0].Text)
		}
		if !strings.Contains(msgs[1].Text, "[THINKING] Planning step") ||
			!strings.Contains(msgs[1].Text, "Executing command") ||
			!strings.Contains(msgs[1].Text, "[TOOL:run_command] go test") {
			t.Errorf("planner response text = %q, want thinking, content, and tool call", msgs[1].Text)
		}

		histChunk := []byte(`{"step_index":4,"type":"CONVERSATION_HISTORY","created_at":"2026-08-25T10:00:04Z"}` + "\n")
		if _, err := parseAntigravityTail(histChunk, "sess-agy-all", 4); err == nil {
			t.Error("parseAntigravityTail should fail on CONVERSATION_HISTORY in tail")
		}
		if _, err := parseAntigravityTail([]byte(`{"step_index":5, broken`+"\n"), "sess-agy-all", 5); err == nil {
			t.Error("parseAntigravityTail should fail on malformed JSON")
		}
	})
}

// TestTailEdge_FingerprintWindowBoundaries tests checkPrefixFingerprint across various
// file offset boundaries: <4KB, exactly 4KB, between 4KB and 8KB, and >8KB.
func TestTailEdge_FingerprintWindowBoundaries(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "boundary_test.dat")

	fullSize := 12288
	data := make([]byte, fullSize)
	for i := range data {
		data[i] = byte((i*31 + 17) % 256)
	}
	writeFile(t, f, string(data))

	testOffsets := []int64{
		100,   // < 4096 (small)
		4096,  // exactly 4096
		6000,  // between 4096 and 8192
		8192,  // exactly 8192
		10000, // > 8192 (dual window: head + tail)
		12288, // file end
	}

	for _, off := range testOffsets {
		prefixFP := checkPrefixFingerprint(f, off)
		if prefixFP == "" {
			t.Errorf("checkPrefixFingerprint(offset=%d) returned empty string", off)
		}
		tmpF := filepath.Join(dir, fmt.Sprintf("prefix_%d.dat", off))
		writeFile(t, tmpF, string(data[:off]))
		wantFP := provenance.FileFingerprint(tmpF, off)
		if prefixFP != wantFP {
			t.Errorf("offset %d: checkPrefixFingerprint = %q, want FileFingerprint = %q", off, prefixFP, wantFP)
		}
	}
}
