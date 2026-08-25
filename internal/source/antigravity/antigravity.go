// Package antigravity is the Source adapter for Google Antigravity (AGY)
// session transcripts under ~/.gemini/antigravity-cli/brain/<uuid> (or
// $ANTIGRAVITY_HOME/brain/<uuid>): one brain directory per conversation,
// transcript in .system_generated/logs/transcript.jsonl (or transcript_full.jsonl),
// and workspace associations tracked in history.jsonl.
//
// Subagents spawned via invoke_subagent are lineage-tracked so the index hides
// them by default and collapses them to their parent session — the same treatment
// Claude and Codex subagents receive. Antigravity step records carry step indices,
// from which this adapter mints deterministic, source-stable message uuids.
package antigravity

import (
	"bufio"
	"bytes"
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

// ID is the stable source name (source_tool column, --source flag).
const ID = "antigravity"

// Adapter reads Antigravity transcripts.
type Adapter struct {
	root string
}

// Compile-time proof the adapter satisfies the ingest port.
var _ source.Source = (*Adapter)(nil)

// New returns a ready Antigravity adapter using the default config directory.
func New() *Adapter {
	return &Adapter{root: ConfigDir()}
}

// NewRoot returns an adapter over an explicit config root (useful in tests).
func NewRoot(root string) *Adapter {
	return &Adapter{root: root}
}

// Registration wires the Antigravity adapter into the source registry.
func Registration() source.Registration {
	return source.Registration{
		ID:     ID,
		Detect: detect,
		New:    func() source.Source { return New() },
	}
}

// detect reports whether path lives under an Antigravity tree.
func detect(path string) bool {
	return strings.Contains(path, "/.gemini/antigravity-cli") ||
		(os.Getenv("ANTIGRAVITY_HOME") != "" && strings.Contains(path, os.Getenv("ANTIGRAVITY_HOME")))
}

// ConfigDir returns the Antigravity config directory: $ANTIGRAVITY_HOME if set,
// else ~/.gemini/antigravity-cli.
func ConfigDir() string {
	if h := os.Getenv("ANTIGRAVITY_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".gemini", "antigravity-cli")
}

// GlobalConfigDir returns the Antigravity global customization config directory.
// On disk the config dir is a SIBLING of the CLI state dir, not a child:
// ~/.gemini/config sits beside ~/.gemini/antigravity-cli (verified against a live
// install's builtin hooks doc). A plain join under ANTIGRAVITY_HOME — the Codex
// shape — would therefore point at the wrong tree, so the basename sniff below
// maps whichever of the two dirs ANTIGRAVITY_HOME names to its config sibling.
func GlobalConfigDir() string {
	if h := os.Getenv("ANTIGRAVITY_HOME"); h != "" {
		if filepath.Base(h) == "antigravity-cli" {
			return filepath.Join(filepath.Dir(h), "config")
		}
		if filepath.Base(h) == "config" {
			return h
		}
		return filepath.Join(h, "config")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".gemini", "config")
}

// BrainRoot returns the brain directory under the given config dir.
func BrainRoot(cfgDir string) string {
	if cfgDir == "" {
		return ""
	}
	return filepath.Join(cfgDir, "brain")
}

// Discover enumerates every Antigravity conversation under BrainRoot(a.root).
// Returns (nil, nil) when the brain directory is absent.
func (a *Adapter) Discover() ([]source.Container, error) {
	if a.root == "" {
		return nil, nil
	}
	return a.DiscoverRoot(BrainRoot(a.root))
}

// DiscoverRoot enumerates Antigravity sessions under an explicit brain directory.
func (a *Adapter) DiscoverRoot(brainDir string) ([]source.Container, error) {
	if brainDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(brainDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("antigravity: read brain dir %s: %w", brainDir, err)
	}

	historyMap := loadHistory(filepath.Join(a.root, "history.jsonl"))

	type candSession struct {
		id   string
		path string
	}

	var mapped []source.Container
	var unmapped []candSession

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		sessionID := entry.Name()
		sessDir := filepath.Join(brainDir, sessionID)

		transcriptPath := findTranscriptFile(sessDir)
		if transcriptPath == "" {
			continue
		}

		if cwd, ok := historyMap[sessionID]; ok && cwd != "" {
			mapped = append(mapped, source.Container{
				ID:         sessionID,
				Path:       transcriptPath,
				CWD:        cwd,
				IsSubagent: false,
				ParentID:   "",
				ResumeArgv: []string{"agy", "--conversation", sessionID},
			})
		} else {
			unmapped = append(unmapped, candSession{id: sessionID, path: transcriptPath})
		}
	}

	// Fast path: if all discovered sessions have known history, zero transcript reads are needed.
	if len(unmapped) == 0 {
		return mapped, nil
	}

	// Process unmapped sessions (subagents or sessions lacking history.jsonl records).
	subagentParents := map[string]string{}
	unmappedHeaders := make(map[string]sessionHeader, len(unmapped))
	var needsParentScan bool

	for _, u := range unmapped {
		cwd, parentID, isSub := inspectSessionHeader(u.path)
		unmappedHeaders[u.id] = sessionHeader{cwd: cwd, parentID: parentID, isSub: isSub}
		if parentID != "" {
			subagentParents[u.id] = parentID
		} else {
			needsParentScan = true
		}
	}

	// If any unmapped session might be a spawned subagent whose parent wasn't in its header,
	// scan candidate transcripts for INVOKE_SUBAGENT records.
	if needsParentScan {
		var allCandidates []candSession
		for _, m := range mapped {
			allCandidates = append(allCandidates, candSession{id: m.ID, path: m.Path})
		}
		allCandidates = append(allCandidates, unmapped...)

		for _, cand := range allCandidates {
			children := scanSpawnedSubagents(cand.path)
			for _, childID := range children {
				if childID != "" {
					subagentParents[childID] = cand.id
				}
			}
		}
	}

	out := make([]source.Container, 0, len(mapped)+len(unmapped))
	out = append(out, mapped...)

	for _, u := range unmapped {
		hdr := unmappedHeaders[u.id]
		parentID := subagentParents[u.id]
		if parentID == "" {
			parentID = hdr.parentID
		}
		isSub := parentID != "" || hdr.isSub

		cwd := hdr.cwd
		if cwd == "" && parentID != "" {
			cwd = historyMap[parentID]
		}

		out = append(out, source.Container{
			ID:         u.id,
			Path:       u.path,
			CWD:        cwd,
			IsSubagent: isSub,
			ParentID:   parentID,
			ResumeArgv: []string{"agy", "--conversation", u.id},
		})
	}

	return out, nil
}

// findTranscriptFile finds the preferred transcript file in a session dir.
// Prefers transcript_full.jsonl to avoid truncated fields, falling back to transcript.jsonl.
func findTranscriptFile(sessDir string) string {
	logsDir := filepath.Join(sessDir, ".system_generated", "logs")
	pFull := filepath.Join(logsDir, "transcript_full.jsonl")
	if fi, err := os.Stat(pFull); err == nil && !fi.IsDir() {
		return pFull
	}
	p := filepath.Join(logsDir, "transcript.jsonl")
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}
	return ""
}

// loadHistory parses history.jsonl into a map from conversationId to workspace.
func loadHistory(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, "conversationId") {
			continue
		}
		var rec struct {
			ConversationID string `json:"conversationId"`
			Workspace      string `json:"workspace"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err == nil {
			if rec.ConversationID != "" && rec.Workspace != "" {
				out[rec.ConversationID] = rec.Workspace
			}
		}
	}
	return out
}

type sessionHeader struct {
	cwd      string
	parentID string
	isSub    bool
}

// inspectSessionHeader reads the opening lines of a transcript to find a recorded CWD and subagent lineage.
func inspectSessionHeader(path string) (cwd, parentID string, isSub bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", false
	}
	defer f.Close()

	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	if n == 0 {
		return "", "", false
	}
	headerData := buf[:n]

	scanner := bufio.NewScanner(bytes.NewReader(headerData))
	count := 0
	for scanner.Scan() && count < 10 {
		count++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Quick check for subagent reminder in prompt
		if strings.Contains(line, "<subagent_reminder>") {
			isSub = true
			if pid := extractParentFromPrompt(line); pid != "" {
				parentID = pid
			}
		}
		// Quick check for user information CWD
		if cwd == "" && strings.Contains(line, "<user_information>") {
			if extracted := extractCWDFromUserInformation(line); extracted != "" {
				cwd = extracted
			}
		}
		// Quick check for tool call CWD
		if cwd == "" && strings.Contains(line, "Cwd") {
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err == nil {
				if tcList, ok := rec["tool_calls"].([]any); ok {
					for _, tc := range tcList {
						if tcMap, ok := tc.(map[string]any); ok {
							if args, ok := tcMap["args"].(map[string]any); ok {
								if c, ok := args["Cwd"].(string); ok && c != "" {
									c = strings.Trim(c, "\"")
									if isAbsPath(c) {
										cwd = c
										break
									}
								}
							}
						}
					}
				}
			}
		}
		if cwd != "" && parentID != "" {
			break
		}
	}
	return cwd, parentID, isSub
}

// scanSpawnedSubagents scans a transcript for INVOKE_SUBAGENT steps using fast byte matching.
func scanSpawnedSubagents(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	if !bytes.Contains(data, []byte("INVOKE_SUBAGENT")) {
		return nil
	}
	var children []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.Contains(line, []byte("INVOKE_SUBAGENT")) {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if stepType, _ := rec["type"].(string); stepType != "INVOKE_SUBAGENT" {
			continue
		}
		content, _ := rec["content"].(string)
		if content != "" && strings.Contains(content, "conversationId") {
			for _, l := range strings.Split(content, "\n") {
				if strings.Contains(l, "conversationId") {
					parts := strings.Split(l, ":")
					if len(parts) >= 2 {
						cid := strings.Trim(strings.TrimSpace(parts[1]), "\", \t\r")
						if cid != "" {
							children = append(children, cid)
						}
					}
				}
			}
		}
	}
	return children
}

// extractParentFromPrompt parses the caller parent ID from a <subagent_reminder> block.
func extractParentFromPrompt(s string) string {
	const startTag = "<subagent_reminder>"
	const endTag = "</subagent_reminder>"
	start := strings.Index(s, startTag)
	if start < 0 {
		return ""
	}
	sub := s[start+len(startTag):]
	if end := strings.Index(sub, endTag); end >= 0 {
		sub = sub[:end]
	}
	for _, line := range strings.Split(sub, "\n") {
		if strings.Contains(line, "caller agent") || strings.Contains(line, "id:") || strings.Contains(line, "id =") {
			if idx := strings.Index(line, "id:"); idx >= 0 {
				val := strings.TrimSpace(line[idx+len("id:"):])
				val = strings.Trim(val, "\", \t\r)")
				if val != "" && len(val) >= 8 {
					return val
				}
			}
			if idx := strings.Index(line, `id="`); idx >= 0 {
				val := line[idx+len(`id="`):]
				if q := strings.Index(val, `"`); q >= 0 {
					return val[:q]
				}
			}
		}
	}
	return ""
}

// extractCWDFromUserInformation parses the workspace URI from <user_information>.
func extractCWDFromUserInformation(content string) string {
	start := strings.Index(content, "<user_information>")
	if start < 0 {
		return ""
	}
	sub := content[start+len("<user_information>"):]
	end := strings.Index(sub, "</user_information>")
	if end >= 0 {
		sub = sub[:end]
	}
	for _, line := range strings.Split(sub, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, " -> ") {
			parts := strings.Split(line, " -> ")
			if len(parts) >= 1 && isAbsPath(strings.TrimSpace(parts[0])) {
				return strings.TrimSpace(parts[0])
			}
		}
		if isAbsPath(line) {
			return line
		}
	}
	return ""
}

func isAbsPath(s string) bool {
	return strings.HasPrefix(s, "/") || (len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/'))
}

// Messages flattens one Antigravity transcript into normalized messages in file order.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return nil, fmt.Errorf("antigravity: read %s: %w", c.Path, err)
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
			UUID:  mintUUID(c.ID, stepIdx, ordinal),
		})
		ordinal++
	}

	if bad > 0 {
		slog.Warn("antigravity: skipped malformed jsonl lines", "count", bad, "path", c.Path)
	}

	return out, nil
}

// normalize maps one transcript step to (role, flattened-text, ok).
func normalize(rec map[string]any) (role, text string, ok bool) {
	stepType, _ := rec["type"].(string)
	sourceVal, _ := rec["source"].(string)

	switch stepType {
	case "USER_INPUT":
		content, _ := rec["content"].(string)
		cleanText := parseUserRequest(content)
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
					argsStr := formatToolArgs(tcMap["args"])
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
		// Tool execution outputs (RUN_COMMAND, VIEW_FILE, GREP_SEARCH, SEARCH_WEB, CALL_MCP_TOOL, etc.)
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

// parseUserRequest extracts the human request from an Antigravity USER_INPUT content.
func parseUserRequest(s string) string {
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

// formatToolArgs extracts clean arguments text from a tool call payload.
func formatToolArgs(v any) string {
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

// mintUUID derives a stable per-message id from the session id + step index + line ordinal.
func mintUUID(sessionID string, stepIndex, ordinal int) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s:%d:%d", sessionID, stepIndex, ordinal)))
	return hex.EncodeToString(h[:])[:16]
}
