package index

import (
	"bytes"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/antigravity"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
)

// Ingest tracing counters for verification and test assertions.
var (
	IncrementalIngestCount atomic.Int64
	FullReindexCount       atomic.Int64
)

// ResetIngestCountersForTesting resets the incremental and full reindex counters.
func ResetIngestCountersForTesting() {
	IncrementalIngestCount.Store(0)
	FullReindexCount.Store(0)
}

// readTailChunk reads bytes from fromOffset to toOffset in path.
// It verifies that fromOffset < toOffset and slices up to the last complete newline.
func readTailChunk(path string, fromOffset, toOffset int64) ([]byte, int64, bool, bool) {
	if fromOffset < 0 || toOffset <= fromOffset {
		return nil, fromOffset, false, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fromOffset, false, false
	}
	defer f.Close()

	if _, err := f.Seek(fromOffset, io.SeekStart); err != nil {
		return nil, fromOffset, false, false
	}

	length := toOffset - fromOffset
	buf := make([]byte, length)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fromOffset, false, false
	}
	buf = buf[:n]
	if len(buf) == 0 {
		return nil, fromOffset, false, false
	}

	lastNL := bytes.LastIndexByte(buf, '\n')
	if lastNL < 0 {
		// The writer has not completed a record yet. Keep the old watermark so
		// the next ingest retries the whole partial line after it grows.
		return nil, fromOffset, false, true
	}

	completeChunk := buf[:lastNL+1]
	newOffset := fromOffset + int64(lastNL+1)
	return completeChunk, newOffset, true, false
}

// checkPrefixFingerprint computes the FileFingerprint of path as it was at offset,
// reading at most min(offset, 4096) from offset 0, and if offset > 8192, reading 4096 bytes from offset-4096.
func checkPrefixFingerprint(path string, offset int64) string {
	if offset <= 0 {
		return ""
	}
	fh, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer fh.Close()

	headLen := 4096
	if offset < 4096 {
		headLen = int(offset)
	}
	head := make([]byte, headLen)
	n, err := io.ReadFull(fh, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return ""
	}
	head = head[:n]

	var tail []byte
	if offset > 8192 {
		if _, err := fh.Seek(offset-4096, io.SeekStart); err != nil {
			return ""
		}
		tail = make([]byte, 4096)
		m, err := io.ReadFull(fh, tail)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return ""
		}
		tail = tail[:m]
	}

	h := sha1.New()
	h.Write(head)
	h.Write([]byte("|"))
	h.Write(tail)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// parseTailMessages parses only newly appended transcript messages starting at fromOffset.
// Returns the parsed messages, the valid newOffset, and whether the tail parse succeeded.
func parseTailMessages(con *sql.DB, c source.Container, sourceID, rawPath string, fromOffset, toOffset int64) ([]model.Message, int64, bool) {
	if strings.Contains(c.Path, "#") || strings.HasSuffix(rawPath, ".db") {
		return nil, fromOffset, false
	}
	chunk, newOffset, ok, pending := readTailChunk(rawPath, fromOffset, toOffset)
	if !ok {
		if pending {
			return nil, fromOffset, true
		}
		return nil, fromOffset, false
	}

	var count int
	_ = con.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id=?", c.ID).Scan(&count)

	var msgs []model.Message
	var err error

	switch sourceID {
	case sourceClaude, "":
		msgs, err = parseClaudeTail(chunk)
	case "codex":
		msgs, err = parseCodexTail(chunk, c.ID, count)
	case "antigravity":
		msgs, err = parseAntigravityTail(chunk, c.ID, count)
	default:
		return nil, fromOffset, false
	}

	if err != nil {
		return nil, fromOffset, false
	}
	return msgs, newOffset, true
}

// parseClaudeTail parses newly appended Claude Code JSONL messages from chunk.
func parseClaudeTail(chunk []byte) ([]model.Message, error) {
	var out []model.Message
	for _, line := range splitLines(chunk) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var o map[string]any
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			return nil, fmt.Errorf("malformed claude tail record: %w", err)
		}
		if !indexable(o) {
			return nil, fmt.Errorf("unrecognized claude tail record type")
		}
		text := parse.ExtractText(o)
		if text == "" {
			return nil, fmt.Errorf("empty claude tail record")
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

// parseCodexTail parses newly appended Codex rollout response_item messages from chunk.
func parseCodexTail(chunk []byte, sessionID string, startOrdinal int) ([]model.Message, error) {
	var out []model.Message
	ordinal := startOrdinal
	for _, line := range splitLines(chunk) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("malformed codex tail record: %w", err)
		}
		role, text, ok := codex.NormalizeRecord(rec)
		if !ok {
			return nil, fmt.Errorf("unrecognized codex tail record")
		}
		if text == "" {
			return nil, fmt.Errorf("empty codex tail record")
		}
		iso, _ := rec["timestamp"].(string)
		out = append(out, model.Message{
			Role:  role,
			Text:  text,
			TS:    parse.ISOToEpoch(iso),
			TSISO: iso,
			UUID:  codex.MintUUID(sessionID, ordinal),
		})
		ordinal++
	}
	return out, nil
}

// parseAntigravityTail parses newly appended Antigravity step records from chunk.
func parseAntigravityTail(chunk []byte, sessionID string, startOrdinal int) ([]model.Message, error) {
	var out []model.Message
	ordinal := startOrdinal
	for _, line := range splitLines(chunk) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("malformed antigravity tail record: %w", err)
		}
		role, text, ok := antigravity.NormalizeRecord(rec)
		if !ok {
			return nil, fmt.Errorf("unrecognized antigravity tail record")
		}
		if text == "" {
			return nil, fmt.Errorf("empty antigravity tail record")
		}
		iso, _ := rec["created_at"].(string)

		stepIdx := ordinal
		if idx, ok := rec["step_index"].(float64); ok {
			stepIdx = int(idx)
		}

		out = append(out, model.Message{
			Role:  role,
			Text:  text,
			TS:    parse.ISOToEpoch(iso),
			TSISO: iso,
			UUID:  antigravity.MintUUID(sessionID, stepIdx, ordinal),
		})
		ordinal++
	}
	return out, nil
}
