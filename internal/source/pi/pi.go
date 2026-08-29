// Package pi is the Source adapter for Pi coding agent transcripts under
// ~/.pi/agent/sessions/ (or $PI_CODING_AGENT_DIR/sessions, $PI_CONFIG_DIR/agent/sessions).
//
// Pi persists session transcripts as date-stamped JSONL files grouped by project
// working directories (e.g. ~/.pi/agent/sessions/--Users-jay-m4-code-rawclaw--/*.jsonl).
//
// The first record in each session file is a session header containing metadata (ID, CWD,
// timestamp). Subsequent records represent user/assistant/tool messages, custom extension
// events, compactions, and branch summaries.
package pi

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/source"
)

// ID is the stable source name (source_tool column, --source flag).
const ID = "pi"

// Adapter reads Pi agent session transcripts.
type Adapter struct {
	roots []string
}

// Compile-time proof the adapter satisfies the Source port.
var _ source.Source = (*Adapter)(nil)

// New returns a ready Pi adapter with default discovery roots.
func New() *Adapter {
	return &Adapter{roots: SessionsRoots()}
}

// NewRoot returns a Pi adapter configured with a single explicit root directory.
func NewRoot(root string) *Adapter {
	return &Adapter{roots: []string{root}}
}

// NewRoots returns a Pi adapter configured with explicit root directories.
func NewRoots(roots ...string) *Adapter {
	return &Adapter{roots: roots}
}

// Registration wires the Pi adapter into the source registry.
func Registration() source.Registration {
	return source.Registration{
		ID:     ID,
		Detect: detect,
		New:    func() source.Source { return New() },
		Lookup: lookup,
	}
}

// SessionsRoots returns candidate directories where Pi session transcripts live.
func SessionsRoots() []string {
	var roots []string
	if d := os.Getenv("PI_CODING_AGENT_DIR"); d != "" {
		roots = append(roots, filepath.Join(d, "sessions"))
	}
	if d := os.Getenv("PI_CONFIG_DIR"); d != "" {
		roots = append(roots, filepath.Join(d, "agent", "sessions"))
	}
	if d := os.Getenv("PI_HOME"); d != "" {
		roots = append(roots, filepath.Join(d, "agent", "sessions"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, filepath.Join(home, ".pi", "agent", "sessions"))
	}
	return roots
}

// detect reports whether path belongs to a Pi agent sessions tree.
func detect(path string) bool {
	clean := filepath.ToSlash(path)
	return strings.Contains(clean, "/.pi/agent/sessions/") ||
		strings.Contains(clean, "/pi/agent/sessions/") ||
		(strings.Contains(clean, "/sessions/--") && strings.HasSuffix(clean, ".jsonl"))
}

// lookup resolves one full session id without discovering or indexing an entire corpus.
func lookup(id string) ([]source.Container, error) {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return nil, nil
	}
	a := New()
	containers, err := a.Discover()
	if err != nil {
		return nil, err
	}
	var out []source.Container
	for _, c := range containers {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out, nil
}

// HeaderRecord represents the first line of a Pi session JSONL transcript.
type HeaderRecord struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
}

// Discover enumerates every Pi session across all configured session roots.
func (a *Adapter) Discover() ([]source.Container, error) {
	var out []source.Container
	seen := make(map[string]bool)

	for _, root := range a.roots {
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("pi: read dir %s: %w", root, err)
		}

		for _, entry := range entries {
			subPath := filepath.Join(root, entry.Name())
			if entry.IsDir() {
				// Subdirectory corresponding to a project working directory
				files, err := os.ReadDir(subPath)
				if err != nil {
					continue
				}
				for _, f := range files {
					if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
						continue
					}
					filePath := filepath.Join(subPath, f.Name())
					if seen[filePath] {
						continue
					}
					seen[filePath] = true

					c, ok := a.readContainerMeta(filePath)
					if ok {
						out = append(out, c)
					}
				}
			} else if strings.HasSuffix(entry.Name(), ".jsonl") {
				// Direct JSONL session file under root
				if seen[subPath] {
					continue
				}
				seen[subPath] = true
				c, ok := a.readContainerMeta(subPath)
				if ok {
					out = append(out, c)
				}
			}
		}
	}
	return out, nil
}

// readContainerMeta inspects the first line of a session JSONL file to build its Container metadata.
func (a *Adapter) readContainerMeta(path string) (source.Container, bool) {
	f, err := os.Open(path)
	if err != nil {
		return source.Container{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return source.Container{}, false
	}

	var hdr HeaderRecord
	if err := json.Unmarshal(scanner.Bytes(), &hdr); err != nil {
		// Fallback: extract ID from filename
		base := filepath.Base(path)
		stem := strings.TrimSuffix(base, ".jsonl")
		id := stem
		if idx := strings.LastIndex(stem, "_"); idx >= 0 && idx < len(stem)-1 {
			id = stem[idx+1:]
		}
		return source.Container{
			ID:         id,
			Path:       path,
			CWD:        "",
			ResumeArgv: source.ResumeArgv(ID, id),
		}, true
	}

	id := hdr.ID
	if id == "" {
		base := filepath.Base(path)
		stem := strings.TrimSuffix(base, ".jsonl")
		id = stem
		if idx := strings.LastIndex(stem, "_"); idx >= 0 && idx < len(stem)-1 {
			id = stem[idx+1:]
		}
	}

	return source.Container{
		ID:         id,
		Path:       path,
		CWD:        hdr.CWD,
		ResumeArgv: source.ResumeArgv(ID, id),
	}, true
}

// MessageRecord models an individual event row inside a Pi transcript JSONL.
type MessageRecord struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	ParentID   *string         `json:"parentId"`
	Timestamp  string          `json:"timestamp"`
	CustomType string          `json:"customType,omitempty"`
	Content    json.RawMessage `json:"content,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	Message    *InnerMessage   `json:"message,omitempty"`
}

// InnerMessage models the nested payload of a "message" type event in Pi.
type InnerMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Timestamp int64           `json:"timestamp,omitempty"`
}

// ContentBlock models structured elements inside user/assistant content arrays.
type ContentBlock struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	Thinking   string          `json:"thinking,omitempty"`
	Name       string          `json:"name,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	Content    json.RawMessage `json:"content,omitempty"`
}

// Messages flattens one Pi session transcript into normalized model.Message records.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	f, err := os.Open(c.Path)
	if err != nil {
		return nil, fmt.Errorf("pi: open %s: %w", c.Path, err)
	}
	defer f.Close()

	var out []model.Message
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var bad int
	var ordinal int

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rec MessageRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			bad++
			continue
		}

		// Skip header line
		if rec.Type == "session" {
			continue
		}

		msg, ok := a.convertRecord(c.ID, rec, ordinal)
		if ok {
			out = append(out, msg)
			ordinal++
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("pi: scanner error", "path", c.Path, "err", err)
	}
	if bad > 0 {
		slog.Warn("pi: skipped malformed jsonl lines", "count", bad, "path", c.Path)
	}

	return out, nil
}

// convertRecord maps a decoded Pi transcript line to a normalized model.Message.
func (a *Adapter) convertRecord(sessionID string, rec MessageRecord, ordinal int) (model.Message, bool) {
	tsEpoch := parseISOOrEpoch(rec.Timestamp)

	uuid := rec.ID
	if uuid == "" {
		uuid = mintUUID(sessionID, ordinal)
	}

	switch rec.Type {
	case "message":
		if rec.Message == nil {
			return model.Message{}, false
		}
		role := rec.Message.Role
		if role == "" {
			role = "user"
		} else if role == "toolResult" {
			role = "assistant"
		}

		text := extractPiContentText(rec.Message.Content)
		if text == "" {
			return model.Message{}, false
		}

		if rec.Message.Timestamp > 0 {
			tsEpoch = float64(rec.Message.Timestamp) / 1000.0
		}

		return model.Message{
			Role:  role,
			Text:  text,
			TS:    tsEpoch,
			TSISO: rec.Timestamp,
			UUID:  uuid,
		}, true

	case "custom_message":
		var text string
		if len(rec.Content) > 0 {
			var rawStr string
			if err := json.Unmarshal(rec.Content, &rawStr); err == nil {
				text = rawStr
			} else {
				text = string(rec.Content)
			}
		}
		if text == "" {
			return model.Message{}, false
		}

		prefix := "[CUSTOM]"
		if rec.CustomType != "" {
			prefix = fmt.Sprintf("[%s]", strings.ToUpper(rec.CustomType))
		}

		return model.Message{
			Role:  "system",
			Text:  prefix + " " + text,
			TS:    tsEpoch,
			TSISO: rec.Timestamp,
			UUID:  uuid,
		}, true

	case "compaction":
		if rec.Summary == "" {
			return model.Message{}, false
		}
		return model.Message{
			Role:  "summary",
			Text:  "[SUMMARY] " + rec.Summary,
			TS:    tsEpoch,
			TSISO: rec.Timestamp,
			UUID:  uuid,
		}, true

	case "branch_summary":
		if rec.Summary == "" {
			return model.Message{}, false
		}
		return model.Message{
			Role:  "summary",
			Text:  "[BRANCH SUMMARY] " + rec.Summary,
			TS:    tsEpoch,
			TSISO: rec.Timestamp,
			UUID:  uuid,
		}, true

	default:
		// Other events (model_change, thinking_level_change, etc.) are non-textual events
		return model.Message{}, false
	}
}

// extractPiContentText flattens Pi's polymorphic content (string or array of blocks) into text.
func extractPiContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Case 1: Raw string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}

	// Case 2: Array of ContentBlocks
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if t := strings.TrimSpace(b.Text); t != "" {
					parts = append(parts, t)
				}
			case "thinking":
				if t := strings.TrimSpace(b.Thinking); t != "" {
					parts = append(parts, "[THINKING] "+t)
				}
			case "toolCall":
				var argsStr string
				if len(b.Arguments) > 0 {
					argsStr = string(b.Arguments)
				}
				parts = append(parts, fmt.Sprintf("[tool_use: %s(%s)]", b.Name, argsStr))
			case "toolResult":
				inner := extractPiContentText(b.Content)
				if inner != "" {
					parts = append(parts, fmt.Sprintf("[tool_result: %s]", inner))
				}
			}
		}
		return strings.Join(parts, "\n")
	}

	// Case 3: Fallback raw JSON
	return strings.TrimSpace(string(raw))
}

// parseISOOrEpoch parses an ISO8601 string to unix seconds as float64, defaulting to 0.
func parseISOOrEpoch(iso string) float64 {
	if iso == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339Nano, iso); err == nil {
		return float64(t.UnixNano()) / 1e9
	}
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return float64(t.Unix())
	}
	return parse.ISOToEpoch(iso)
}

// mintUUID constructs a deterministic, stable 16-hex UUID from session id and message index.
func mintUUID(sessionID string, ordinal int) string {
	h := sha1.New()
	fmt.Fprintf(h, "pi:%s:%d", sessionID, ordinal)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
