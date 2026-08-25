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
			continue
		}
		role, text, ok := normalizeCodex(rec)
		if !ok || text == "" {
			continue
		}
		iso, _ := rec["timestamp"].(string)
		out = append(out, model.Message{
			Role:  role,
			Text:  text,
			TS:    parse.ISOToEpoch(iso),
			TSISO: iso,
			UUID:  mintCodexUUID(sessionID, ordinal),
		})
		ordinal++
	}
	return out, nil
}

// normalizeCodex mirrors codex.normalize for response_item records.
func normalizeCodex(rec map[string]any) (role, text string, ok bool) {
	if t, _ := rec["type"].(string); t != "response_item" {
		return "", "", false
	}
	p, ok := rec["payload"].(map[string]any)
	if !ok {
		return "", "", false
	}
	switch pt, _ := p["type"].(string); pt {
	case "message":
		r, _ := p["role"].(string)
		return mapCodexRole(r), codexContentText(p["content"]), true
	case "reasoning":
		s := codexSummaryText(p["summary"])
		if s == "" {
			return "", "", false
		}
		return "assistant", "[THINKING] " + s, true
	case "function_call":
		name, _ := p["name"].(string)
		args, _ := p["arguments"].(string)
		return "assistant", strings.TrimSpace(fmt.Sprintf("[TOOL:%s] %s", name, args)), true
	case "function_call_output", "custom_tool_call_output":
		return "tool", "[TOOL_RESULT] " + codexOutputText(p["output"]), true
	case "custom_tool_call":
		name, _ := p["name"].(string)
		input, _ := p["input"].(string)
		return "assistant", strings.TrimSpace(fmt.Sprintf("[TOOL:%s] %s", name, input)), true
	case "web_search_call":
		return "assistant", strings.TrimSpace("[TOOL:web_search] " + codexActionQuery(p["action"])), true
	case "tool_search_call":
		return "assistant", strings.TrimSpace("[TOOL:tool_search] " + codexArgsText(p["arguments"])), true
	case "tool_search_output":
		return "tool", "[TOOL_RESULT] " + codexOutputText(p["output"]), true
	case "image_generation_call":
		prompt, _ := p["prompt"].(string)
		return "assistant", strings.TrimSpace("[TOOL:image_generation] " + prompt), true
	default:
		return "", "", false
	}
}

func mapCodexRole(r string) string {
	switch r {
	case "developer", "system":
		return "system"
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	default:
		if r == "" {
			return "assistant"
		}
		return r
	}
}

func codexContentText(v any) string {
	blocks, ok := v.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, blk := range blocks {
		m, ok := blk.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := m["text"].(string); ok && t != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(t)
		}
	}
	return b.String()
}

func codexSummaryText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return codexContentText(v)
}

func codexOutputText(v any) string {
	switch o := v.(type) {
	case string:
		return o
	case map[string]any:
		if s, ok := o["output"].(string); ok {
			return s
		}
		if s := codexContentText(o["content"]); s != "" {
			return s
		}
	}
	return ""
}

func codexActionQuery(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	q, _ := m["query"].(string)
	return q
}

func codexArgsText(v any) string {
	switch a := v.(type) {
	case string:
		return a
	case map[string]any:
		if q, ok := a["query"].(string); ok && q != "" {
			return q
		}
		if b, err := json.Marshal(a); err == nil {
			return string(b)
		}
	}
	return ""
}

func mintCodexUUID(sessionID string, ordinal int) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s:%d", sessionID, ordinal)))
	return hex.EncodeToString(h[:])[:16]
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
			continue
		}
		role, text, ok := normalizeAntigravity(rec)
		if !ok || text == "" {
			continue
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
			UUID:  mintAntigravityUUID(sessionID, stepIdx, ordinal),
		})
		ordinal++
	}
	return out, nil
}

func normalizeAntigravity(rec map[string]any) (role, text string, ok bool) {
	stepType, _ := rec["type"].(string)
	sourceVal, _ := rec["source"].(string)

	switch stepType {
	case "USER_INPUT":
		content, _ := rec["content"].(string)
		cleanText := parseAntigravityUserRequest(content)
		if cleanText == "" {
			return "", "", false
		}
		return "user", cleanText, true

	case "PLANNER_RESPONSE":
		var parts []string
		if thinking, _ := rec["thinking"].(string); strings.TrimSpace(thinking) != "" {
			parts = append(parts, "[THINKING] "+strings.TrimSpace(thinking))
		}
		if content, _ := rec["content"].(string); strings.TrimSpace(content) != "" {
			parts = append(parts, strings.TrimSpace(content))
		}
		if toolCalls, ok := rec["tool_calls"].([]any); ok {
			for _, tc := range toolCalls {
				if tcMap, ok := tc.(map[string]any); ok {
					name, _ := tcMap["name"].(string)
					argsStr := formatAntigravityToolArgs(tcMap["args"])
					if argsStr != "" {
						parts = append(parts, strings.TrimSpace(fmt.Sprintf("[TOOL:%s] %s", name, argsStr)))
					} else {
						parts = append(parts, fmt.Sprintf("[TOOL:%s]", name))
					}
				}
			}
		}
		if len(parts) == 0 {
			return "", "", false
		}
		return "assistant", strings.Join(parts, "\n"), true

	case "SYSTEM_MESSAGE":
		content, _ := rec["content"].(string)
		if strings.TrimSpace(content) == "" {
			return "", "", false
		}
		return "system", strings.TrimSpace(content), true

	case "CONVERSATION_HISTORY", "CHECKPOINT":
		return "", "", false

	default:
		if content, _ := rec["content"].(string); strings.TrimSpace(content) != "" {
			return "tool", "[TOOL_RESULT] " + strings.TrimSpace(content), true
		}
		if sourceVal == "MODEL" || sourceVal == "SYSTEM" {
			if content, _ := rec["content"].(string); strings.TrimSpace(content) != "" {
				return "tool", "[TOOL_RESULT] " + strings.TrimSpace(content), true
			}
		}
		return "", "", false
	}
}

func parseAntigravityUserRequest(s string) string {
	const startTag = "<USER_REQUEST>"
	const endTag = "</USER_REQUEST>"
	start := strings.Index(s, startTag)
	if start >= 0 {
		sub := s[start+len(startTag):]
		if end := strings.Index(sub, endTag); end >= 0 {
			return strings.TrimSpace(sub[:end])
		}
		return strings.TrimSpace(sub)
	}
	return strings.TrimSpace(s)
}

func formatAntigravityToolArgs(v any) string {
	switch a := v.(type) {
	case string:
		var unq string
		if err := json.Unmarshal([]byte(a), &unq); err == nil && unq != "" {
			return unq
		}
		return strings.TrimSpace(a)
	case map[string]any:
		if cmd, ok := a["CommandLine"].(string); ok && cmd != "" {
			return cmd
		}
		if q, ok := a["query"].(string); ok && q != "" {
			return q
		}
		if q, ok := a["Query"].(string); ok && q != "" {
			return q
		}
		if p, ok := a["AbsolutePath"].(string); ok && p != "" {
			return p
		}
		if p, ok := a["TargetFile"].(string); ok && p != "" {
			return p
		}
		if b, err := json.Marshal(a); err == nil {
			return string(b)
		}
	}
	return ""
}

func mintAntigravityUUID(sessionID string, stepIndex, ordinal int) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s:%d:%d", sessionID, stepIndex, ordinal)))
	return hex.EncodeToString(h[:])[:16]
}
