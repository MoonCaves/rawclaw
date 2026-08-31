// Package hermes is the Source adapter for Hermes AI agent session transcripts
// under ~/.hermes/ (or $HERMES_HOME, $XDG_CONFIG_HOME/hermes, $XDG_DATA_HOME/hermes).
// Hermes stores sessions and messages in a state.db SQLite database.
package hermes

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
)

// ID is the stable source name (source_tool column, --source flag).
const ID = "hermes"

// Adapter reads Hermes session SQLite databases.
type Adapter struct {
	roots []string
}

// Compile-time proof the adapter satisfies the Source port.
var _ source.Source = (*Adapter)(nil)

// New returns a ready Hermes adapter with default discovery roots.
func New() *Adapter {
	return &Adapter{roots: SessionsRoots()}
}

// NewRoot returns a Hermes adapter configured with a single explicit root directory.
func NewRoot(root string) *Adapter {
	return &Adapter{roots: []string{root}}
}

// NewRoots returns a Hermes adapter configured with explicit root directories.
func NewRoots(roots ...string) *Adapter {
	return &Adapter{roots: roots}
}

// Registration wires the Hermes adapter into the source registry.
func Registration() source.Registration {
	return source.Registration{
		ID:     ID,
		Detect: detect,
		New:    func() source.Source { return New() },
		Lookup: lookup,
	}
}

func lookup(id string) ([]source.Container, error) {
	if id == "" {
		return nil, nil
	}
	a := New()
	var out []source.Container
	for _, root := range a.roots {
		if root == "" {
			continue
		}
		dbPath := filepath.Join(root, "state.db")
		if st, err := os.Stat(dbPath); err == nil && !st.IsDir() {
			if c, ok := lookupDatabase(dbPath, id); ok {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func lookupDatabase(path, id string) (source.Container, bool) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)", path))
	if err != nil {
		return source.Container{}, false
	}
	defer db.Close()

	var rowID, cwd string
	var parentSessionID sql.NullString
	err = db.QueryRow("SELECT id, cwd, parent_session_id FROM sessions WHERE id = ?", id).Scan(&rowID, &cwd, &parentSessionID)
	if err != nil {
		return source.Container{}, false
	}
	return source.Container{
		ID:         rowID,
		Path:       fmt.Sprintf("%s#%s", path, rowID),
		CWD:        cwd,
		ParentID:   parentSessionID.String,
		IsSubagent: parentSessionID.Valid && parentSessionID.String != "",
		ResumeArgv: []string{"hermes", "resume", rowID},
	}, true
}

func detect(path string) bool {
	clean := filepath.Clean(path)
	return strings.Contains(clean, ".hermes") && (strings.HasSuffix(clean, "state.db") || strings.Contains(clean, "state.db#"))
}

// SessionsRoots returns candidate directories where Hermes state.db may live.
func SessionsRoots() []string {
	var roots []string
	if h := os.Getenv("HERMES_HOME"); h != "" {
		roots = append(roots, h)
	}
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		roots = append(roots, filepath.Join(xdgData, "hermes"))
	}
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		roots = append(roots, filepath.Join(xdgConfig, "hermes"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, filepath.Join(home, ".hermes"))
	}
	return roots
}

// Discover enumerates all Hermes sessions from state.db.
func (a *Adapter) Discover() ([]source.Container, error) {
	var out []source.Container
	seen := make(map[string]bool)

	for _, root := range a.roots {
		if root == "" {
			continue
		}
		dbPath := filepath.Join(root, "state.db")
		if st, err := os.Stat(dbPath); err != nil || st.IsDir() {
			continue
		}
		containers, err := a.discoverDatabase(dbPath)
		if err != nil {
			continue
		}
		for _, c := range containers {
			if !seen[c.ID] {
				seen[c.ID] = true
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func (a *Adapter) discoverDatabase(dbPath string) ([]source.Container, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)", dbPath))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, cwd, parent_session_id FROM sessions ORDER BY started_at ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []source.Container
	for rows.Next() {
		var id, cwd string
		var parentSessionID sql.NullString
		if err := rows.Scan(&id, &cwd, &parentSessionID); err != nil {
			continue
		}
		out = append(out, source.Container{
			ID:         id,
			Path:       fmt.Sprintf("%s#%s", dbPath, id),
			CWD:        cwd,
			ParentID:   parentSessionID.String,
			IsSubagent: parentSessionID.Valid && parentSessionID.String != "",
			ResumeArgv: []string{"hermes", "resume", id},
		})
	}
	return out, rows.Err()
}

// Messages extracts all normalized messages for a session.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	dbPath, sessionID := splitDBPath(c.Path)
	if sessionID == "" {
		sessionID = c.ID
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)", dbPath))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, role, content, tool_name, tool_calls, timestamp FROM messages WHERE session_id = ? ORDER BY id ASC", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Message
	for rows.Next() {
		var msgID int64
		var role string
		var content, toolName, toolCalls sql.NullString
		var ts float64
		if err := rows.Scan(&msgID, &role, &content, &toolName, &toolCalls, &ts); err != nil {
			continue
		}

		text := content.String
		if text == "" && toolCalls.Valid && toolCalls.String != "" {
			text = formatToolCalls(toolCalls.String)
		} else if text == "" && toolName.Valid && toolName.String != "" {
			text = fmt.Sprintf("[tool: %s]", toolName.String)
		}

		if strings.TrimSpace(text) == "" {
			continue
		}

		var tsISO string
		if ts > 0 {
			tsISO = time.Unix(int64(ts), int64((ts-float64(int64(ts)))*1e9)).UTC().Format(time.RFC3339Nano)
		}
		uuid := deterministicUUID(sessionID, msgID, role, text)

		msgRole := role
		if msgRole != "user" && msgRole != "assistant" && msgRole != "system" {
			msgRole = "assistant"
		}

		out = append(out, model.Message{
			Role:  msgRole,
			Text:  text,
			TS:    ts,
			TSISO: tsISO,
			UUID:  uuid,
		})
	}
	return out, rows.Err()
}

func formatToolCalls(toolCallsJSON string) string {
	var calls []struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(toolCallsJSON), &calls); err != nil {
		return toolCallsJSON
	}
	var sb strings.Builder
	for i, call := range calls {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("[call: %s(%s)]", call.Name, string(call.Arguments)))
	}
	return sb.String()
}

func deterministicUUID(sessionID string, msgID int64, role, content string) string {
	h := sha1.New()
	h.Write([]byte(sessionID))
	h.Write([]byte(fmt.Sprintf(":%d:", msgID)))
	h.Write([]byte(role))
	h.Write([]byte(":"))
	h.Write([]byte(content))
	sum := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("%s-%s-%s-%s-%s", sum[0:8], sum[8:12], sum[12:16], sum[16:20], sum[20:32])
}

func splitDBPath(path string) (string, string) {
	if idx := strings.Index(path, "#"); idx != -1 {
		return path[:idx], path[idx+1:]
	}
	return path, ""
}
