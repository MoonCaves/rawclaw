// Package codex is the Source adapter for OpenAI Codex CLI transcripts under
// $CODEX_HOME/sessions (default ~/.codex/sessions): date-partitioned
// rollout-<ts>-<uuid>.jsonl files, each a self-contained session whose first
// record is a session_meta header.
//
// Every JSONL line is a {type,timestamp,payload} wrapper. The header's lineage
// fields (thread_source, parent_thread_id, forked_from_id) tag subagent/forked
// threads so the index hides them by default and collapses fork families to
// their root — the same treatment Claude subagents get. Codex message records
// carry NO per-message id, so this adapter mints a deterministic uuid from the
// session id + ordinal, stable across reindex.
package codex

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/source"
)

// Adapter reads Codex CLI transcripts. Stateless; the zero value is usable.
type Adapter struct{}

// Compile-time proof the adapter satisfies the port.
var _ source.Source = (*Adapter)(nil)

// New returns a ready Codex adapter.
func New() *Adapter { return &Adapter{} }

// Registration wires the Codex adapter into the source registry. Call
// source.Register(codex.Registration()) explicitly at start-up.
func Registration() source.Registration {
	return source.Registration{
		ID:     "codex",
		Detect: detect,
		New:    func() source.Source { return New() },
	}
}

// detect reports whether path lives under a Codex sessions tree.
func detect(path string) bool {
	return strings.Contains(path, "/.codex/sessions") ||
		(os.Getenv("CODEX_HOME") != "" && strings.Contains(path, "/sessions/"))
}

// SessionsRoot is $CODEX_HOME/sessions, or ~/.codex/sessions when CODEX_HOME is
// unset. Returns "" only when the home directory cannot be resolved.
func SessionsRoot() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return filepath.Join(h, "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

// Discover walks the sessions tree and returns one Container per rollout file,
// reading only each file's session_meta header for lineage + cwd. A file whose
// header is unreadable is skipped (logged, grouped). Returns (nil, nil) when the
// tree is absent — an empty corpus is not an error.
func (a *Adapter) Discover() ([]source.Container, error) {
	root := SessionsRoot()
	if root == "" {
		return nil, nil
	}
	var out []source.Container
	var bad int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep walking
		}
		if d.IsDir() || !isRollout(d.Name()) {
			return nil
		}
		m, ok := readMeta(path)
		if !ok {
			bad++
			return nil
		}
		out = append(out, source.Container{
			ID:         m.id,
			Path:       path,
			CWD:        m.cwd,
			IsSubagent: m.isChild(),
			ParentID:   m.parent(),
			ResumeArgv: []string{"codex", "resume", m.id},
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("codex: walk %s: %w", root, err)
	}
	if bad > 0 {
		slog.Warn("codex: skipped rollouts with unreadable session_meta", "count", bad, "root", root)
	}
	return out, nil
}

// Messages flattens one rollout into normalized messages in file order. It maps
// the response_item records (message / reasoning / function_call[_output]) and
// skips headers, telemetry (event_msg, token_count), and control records
// (turn_context, world_state, compacted, …). Message records carry no id, so a
// deterministic uuid is minted from the session id + ordinal.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return nil, fmt.Errorf("codex: read %s: %w", c.Path, err)
	}
	var out []model.Message
	var bad, ordinal int
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			bad++
			continue
		}
		role, text, ok := normalize(rec)
		if !ok || text == "" {
			continue
		}
		iso, _ := rec["timestamp"].(string)
		out = append(out, model.Message{
			Role:  role,
			Text:  text,
			TS:    parse.ISOToEpoch(iso),
			TSISO: iso,
			UUID:  mintUUID(c.ID, ordinal),
		})
		ordinal++
	}
	if bad > 0 {
		slog.Warn("codex: skipped malformed jsonl lines", "count", bad, "path", c.Path)
	}
	return out, nil
}

// normalize maps one rollout record to (role, flattened-text). ok=false means
// "not an indexable record" (a header, telemetry, or control line). Tool and
// reasoning content is kept with the same [TOOL:…]/[THINKING] markers Claude
// uses, so the snippet layer hides them identically.
func normalize(rec map[string]any) (role, text string, ok bool) {
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
		return mapRole(r), contentText(p["content"]), true
	case "reasoning":
		s := summaryText(p["summary"])
		if s == "" {
			return "", "", false
		}
		return "assistant", "[THINKING] " + s, true
	case "function_call":
		name, _ := p["name"].(string)
		args, _ := p["arguments"].(string)
		return "assistant", strings.TrimSpace(fmt.Sprintf("[TOOL:%s] %s", name, args)), true
	case "function_call_output", "custom_tool_call_output":
		return "tool", "[TOOL_RESULT] " + outputText(p["output"]), true
	case "custom_tool_call":
		name, _ := p["name"].(string)
		input, _ := p["input"].(string)
		return "assistant", strings.TrimSpace(fmt.Sprintf("[TOOL:%s] %s", name, input)), true
	case "web_search_call":
		return "assistant", strings.TrimSpace("[TOOL:web_search] " + actionQuery(p["action"])), true
	case "tool_search_call":
		return "assistant", strings.TrimSpace("[TOOL:tool_search] " + argsText(p["arguments"])), true
	case "tool_search_output":
		return "tool", "[TOOL_RESULT] " + outputText(p["output"]), true
	case "image_generation_call":
		prompt, _ := p["prompt"].(string)
		return "assistant", strings.TrimSpace("[TOOL:image_generation] " + prompt), true
	default:
		return "", "", false
	}
}

// actionQuery extracts a web_search_call's query text (payload.action.query).
func actionQuery(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	q, _ := m["query"].(string)
	return q
}

// argsText renders a tool call's arguments, which may be a JSON string (like
// function_call) OR an already-decoded object (like tool_search_call). Prefers a
// "query" field, else the compact JSON.
func argsText(v any) string {
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

// mapRole maps Codex roles onto the messages-table role vocabulary. "developer"
// is Codex's system role.
func mapRole(r string) string {
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

// contentText joins the text of a content-block array ([{type:input_text|
// output_text, text:…}]). Non-array or block-less content yields "".
func contentText(v any) string {
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

// summaryText flattens a reasoning summary, which may be a string or an array of
// {type:summary_text, text:…} blocks.
func summaryText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return contentText(v)
}

// outputText renders a function_call_output payload, which may be a string or a
// {output:…} / content-block shape.
func outputText(v any) string {
	switch o := v.(type) {
	case string:
		return o
	case map[string]any:
		if s, ok := o["output"].(string); ok {
			return s
		}
		if s := contentText(o["content"]); s != "" {
			return s
		}
	}
	return ""
}

// mintUUID derives a stable per-message id from the session id + ordinal. Codex
// message records carry no id; the ordinal is deterministic because the whole
// file is reparsed in order on any change.
//
// FOOTGUN: the ordinal counts only records normalize() accepts, so adding or
// removing a handled record type shifts every later ordinal in a session,
// changing its minted uuids — invalidating existing <session8>:<uuid8> refs
// (bookmarks, --around windows, prior citations) until a full reindex. If you
// change the normalize() switch, treat it as a ref-breaking change.
func mintUUID(sessionID string, ordinal int) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s:%d", sessionID, ordinal)))
	return hex.EncodeToString(h[:])[:16]
}

// isRollout reports whether name is a Codex rollout transcript file.
func isRollout(name string) bool {
	return strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, ".jsonl")
}
