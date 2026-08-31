// Package opencode is the Source adapter for OpenCode / Crush AI coding agent
// session transcripts stored in SQLite database format (~/.local/share/opencode/opencode.db,
// $XDG_DATA_HOME/opencode/opencode.db, $OPENCODE_DATA_DIR/opencode.db).
//
// The adapter discovers all sessions in the SQLite database, extracts session
// metadata (ID, CWD, lineage, timestamps), and flattens message and part rows
// into normalized model.Message records in order.
package opencode

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// ID is the stable source name (source_tool column, --source flag).
const ID = "opencode"

// Adapter reads OpenCode SQLite session databases.
type Adapter struct {
	roots []string
}

// Compile-time proof the adapter satisfies the Source port.
var _ source.Source = (*Adapter)(nil)

// New returns a ready OpenCode adapter with default discovery roots.
func New() *Adapter {
	return &Adapter{roots: SessionsRoots()}
}

// NewRoot returns an OpenCode adapter configured with a single explicit root directory.
func NewRoot(root string) *Adapter {
	return &Adapter{roots: []string{root}}
}

// NewRoots returns an OpenCode adapter configured with explicit root directories.
func NewRoots(roots ...string) *Adapter {
	return &Adapter{roots: roots}
}

// Registration wires the OpenCode adapter into the source registry.
func Registration() source.Registration {
	return source.Registration{
		ID:     ID,
		Detect: detect,
		New:    func() source.Source { return New() },
		Lookup: lookup,
	}
}

// detect reports whether path belongs to an OpenCode database or session URI.
func detect(path string) bool {
	if strings.Contains(path, "opencode.db") || strings.Contains(path, "/opencode/") {
		return strings.HasSuffix(path, ".db") || strings.Contains(path, ".db#")
	}
	return false
}

// SessionsRoots returns all potential directories containing OpenCode database files.
func SessionsRoots() []string {
	var roots []string
	seen := make(map[string]bool)
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		roots = append(roots, p)
	}

	if dataDir := os.Getenv("OPENCODE_DATA_DIR"); dataDir != "" {
		add(dataDir)
	}
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		add(filepath.Join(xdgData, "opencode"))
	}
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		add(filepath.Join(xdgConfig, "opencode"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add(filepath.Join(home, ".local", "share", "opencode"))
		add(filepath.Join(home, ".config", "opencode"))
		add(filepath.Join(home, ".opencode"))
	}
	return roots
}

// Discover enumerates every OpenCode session across configured database roots.
func (a *Adapter) Discover() ([]source.Container, error) {
	roots := a.roots
	if len(roots) == 0 {
		roots = SessionsRoots()
	}
	return a.DiscoverRoots(roots)
}

// DiscoverRoots enumerates OpenCode sessions under the given directory roots.
func (a *Adapter) DiscoverRoots(roots []string) ([]source.Container, error) {
	if len(roots) == 0 {
		return nil, nil
	}

	var out []source.Container
	seenPaths := make(map[string]bool)

	for _, root := range roots {
		if root == "" {
			continue
		}
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			// If root itself is a direct SQLite file
			if err == nil && !fi.IsDir() && strings.HasSuffix(root, ".db") {
				containers, err := discoverDatabaseContainers(root)
				if err == nil {
					out = append(out, containers...)
				}
			}
			continue
		}

		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".db") {
				return nil
			}
			if strings.HasSuffix(name, ".db-wal") || strings.HasSuffix(name, ".db-shm") || strings.HasSuffix(name, ".db-journal") {
				return nil
			}

			cleanPath := filepath.Clean(path)
			if seenPaths[cleanPath] {
				return nil
			}
			seenPaths[cleanPath] = true

			containers, err := discoverDatabaseContainers(cleanPath)
			if err != nil {
				// Non-fatal skip if not a valid opencode database
				return nil
			}
			out = append(out, containers...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("opencode: discover %s: %w", root, err)
		}
	}

	return out, nil
}

// discoverDatabaseContainers reads session records from an OpenCode SQLite database.
func discoverDatabaseContainers(dbPath string) ([]source.Container, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)&_pragma=mmap_size(%d)", dbPath, store.ROMmapSize))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	var sessionTableExists int
	err = db.QueryRow("SELECT 1 FROM sqlite_master WHERE type IN ('table', 'view') AND name='session' LIMIT 1").Scan(&sessionTableExists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check session table: %w", err)
	}

	if sessionTableExists != 1 {
		// Try 'sessions' plural fallback
		err = db.QueryRow("SELECT 1 FROM sqlite_master WHERE type IN ('table', 'view') AND name='sessions' LIMIT 1").Scan(&sessionTableExists)
		if err != nil || sessionTableExists != 1 {
			return nil, nil
		}
		return discoverPluralSessions(db, dbPath)
	}

	rows, err := db.Query("SELECT id, directory, parent_id FROM session ORDER BY time_created ASC")
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}
	defer rows.Close()
	return scanSessionRows(rows, dbPath)
}

func discoverPluralSessions(db *sql.DB, dbPath string) ([]source.Container, error) {
	rows, err := db.Query("SELECT id, directory, parent_id FROM sessions ORDER BY time_created ASC")
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()
	return scanSessionRows(rows, dbPath)
}

func scanSessionRows(rows *sql.Rows, dbPath string) ([]source.Container, error) {
	var containers []source.Container
	for rows.Next() {
		var id, directory string
		var parentID sql.NullString
		if err := rows.Scan(&id, &directory, &parentID); err != nil {
			continue
		}
		if id == "" {
			continue
		}

		isSubagent := parentID.Valid && strings.TrimSpace(parentID.String) != ""
		parent := ""
		if isSubagent {
			parent = strings.TrimSpace(parentID.String)
		}

		containers = append(containers, source.Container{
			ID:         id,
			Path:       fmt.Sprintf("%s#%s", dbPath, id),
			CWD:        directory,
			IsSubagent: isSubagent,
			ParentID:   parent,
			ResumeArgv: source.ResumeArgv(ID, id),
		})
	}
	return containers, nil
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
		dbPath := filepath.Join(root, "opencode.db")
		if fi, err := os.Stat(dbPath); err == nil && !fi.IsDir() {
			if c, ok := lookupSessionInDB(dbPath, id); ok {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func lookupSessionInDB(dbPath, id string) (source.Container, bool) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)", dbPath))
	if err != nil {
		return source.Container{}, false
	}
	defer db.Close()

	var foundID, directory string
	var parentID sql.NullString

	err = db.QueryRow("SELECT id, directory, parent_id FROM session WHERE id = ? OR id LIKE ? LIMIT 1", id, id+"%").Scan(&foundID, &directory, &parentID)
	if err != nil {
		err = db.QueryRow("SELECT id, directory, parent_id FROM sessions WHERE id = ? OR id LIKE ? LIMIT 1", id, id+"%").Scan(&foundID, &directory, &parentID)
		if err != nil {
			return source.Container{}, false
		}
	}

	isSubagent := parentID.Valid && strings.TrimSpace(parentID.String) != ""
	parent := ""
	if isSubagent {
		parent = strings.TrimSpace(parentID.String)
	}

	return source.Container{
		ID:         foundID,
		Path:       fmt.Sprintf("%s#%s", dbPath, foundID),
		CWD:        directory,
		IsSubagent: isSubagent,
		ParentID:   parent,
		ResumeArgv: source.ResumeArgv(ID, foundID),
	}, true
}

// Messages flattens one OpenCode session into normalized model.Message records.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	dbPath := c.Path
	sessionID := c.ID
	if idx := strings.Index(dbPath, "#"); idx >= 0 {
		dbPath = dbPath[:idx]
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)&_pragma=mmap_size(%d)", dbPath, store.ROMmapSize))
	if err != nil {
		return nil, fmt.Errorf("opencode: open %s: %w", dbPath, err)
	}
	defer db.Close()

	// Check if part table exists
	var partTableExists int
	_ = db.QueryRow("SELECT 1 FROM sqlite_master WHERE type IN ('table', 'view') AND name='part' LIMIT 1").Scan(&partTableExists)

	var msgs []model.Message
	if partTableExists == 1 {
		msgs, err = loadMessagesWithParts(db, sessionID)
	} else {
		msgs, err = loadMessagesDirect(db, sessionID)
	}

	if err != nil {
		return nil, fmt.Errorf("opencode: load messages for %s: %w", sessionID, err)
	}

	return msgs, nil
}

type messageRow struct {
	id          string
	timeCreated int64
	dataJSON    string
}

type partRow struct {
	id          string
	messageID   string
	timeCreated int64
	dataJSON    string
}

type messageData struct {
	Role string `json:"role"`
}

type partData struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Name      string          `json:"name,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	CallID    string          `json:"callID,omitempty"`
	Synthetic bool            `json:"synthetic,omitempty"`
	State     *partState      `json:"state,omitempty"`
	Time      *partTime       `json:"time,omitempty"`
	Extra     json.RawMessage `json:"-"`
}

type partState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output string          `json:"output,omitempty"`
	Title  string          `json:"title,omitempty"`
}

type partTime struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

func loadMessagesWithParts(db *sql.DB, sessionID string) ([]model.Message, error) {
	// Query messages in chronological order
	mRows, err := db.Query("SELECT id, time_created, data FROM message WHERE session_id = ? ORDER BY time_created ASC, id ASC", sessionID)
	if err != nil {
		return nil, err
	}
	defer mRows.Close()

	var messages []messageRow
	for mRows.Next() {
		var m messageRow
		if err := mRows.Scan(&m.id, &m.timeCreated, &m.dataJSON); err == nil {
			messages = append(messages, m)
		}
	}
	if len(messages) == 0 {
		return nil, nil
	}

	// Query all parts for this session
	pRows, err := db.Query("SELECT id, message_id, time_created, data FROM part WHERE session_id = ? ORDER BY time_created ASC, id ASC", sessionID)
	if err != nil {
		return nil, err
	}
	defer pRows.Close()

	partsByMsg := make(map[string][]partRow)
	for pRows.Next() {
		var p partRow
		if err := pRows.Scan(&p.id, &p.messageID, &p.timeCreated, &p.dataJSON); err == nil {
			partsByMsg[p.messageID] = append(partsByMsg[p.messageID], p)
		}
	}

	var out []model.Message
	for ordinal, m := range messages {
		var mData messageData
		_ = json.Unmarshal([]byte(m.dataJSON), &mData)
		role := mData.Role
		if role == "" {
			role = "assistant"
		}

		parts := partsByMsg[m.id]
		var sb strings.Builder

		for _, p := range parts {
			var pData partData
			if err := json.Unmarshal([]byte(p.dataJSON), &pData); err != nil {
				continue
			}

			switch pData.Type {
			case "text":
				if pData.Text != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(pData.Text)
				}
			case "reasoning":
				if pData.Text != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString("[THINKING] ")
					sb.WriteString(pData.Text)
				}
			case "agent":
				if pData.Name != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString("[agent: ")
					sb.WriteString(pData.Name)
					sb.WriteString("]")
				}
			case "tool":
				toolName := pData.Tool
				if toolName == "" {
					toolName = pData.Name
				}
				if toolName != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					inputStr := ""
					outputStr := ""
					if pData.State != nil {
						if len(pData.State.Input) > 0 {
							inputStr = string(pData.State.Input)
						}
						outputStr = pData.State.Output
					}
					if inputStr != "" {
						sb.WriteString("[tool_use: ")
						sb.WriteString(toolName)
						sb.WriteString("] ")
						sb.WriteString(inputStr)
						sb.WriteString("\n")
					} else {
						sb.WriteString("[tool_use: ")
						sb.WriteString(toolName)
						sb.WriteString("]\n")
					}
					if outputStr != "" {
						sb.WriteString("[tool_result: ")
						sb.WriteString(toolName)
						sb.WriteString("] ")
						sb.WriteString(outputStr)
					}
				}
			}
		}

		body := strings.TrimSpace(sb.String())
		if body == "" {
			// If parts produced empty text, fallback to message data JSON
			body = m.dataJSON
		}

		t := time.UnixMilli(m.timeCreated).UTC()
		out = append(out, model.Message{
			UUID:  mintUUID(sessionID, ordinal),
			Role:  role,
			Text:  body,
			TS:    float64(m.timeCreated) / 1000.0,
			TSISO: t.Format(time.RFC3339),
		})
	}

	return out, nil
}

func loadMessagesDirect(db *sql.DB, sessionID string) ([]model.Message, error) {
	rows, err := db.Query("SELECT id, time_created, data FROM message WHERE session_id = ? ORDER BY time_created ASC, id ASC", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Message
	ordinal := 0
	for rows.Next() {
		var id string
		var timeCreated int64
		var dataJSON string
		if err := rows.Scan(&id, &timeCreated, &dataJSON); err != nil {
			continue
		}

		var mData messageData
		_ = json.Unmarshal([]byte(dataJSON), &mData)
		role := mData.Role
		if role == "" {
			role = "assistant"
		}

		t := time.UnixMilli(timeCreated).UTC()
		out = append(out, model.Message{
			UUID:  mintUUID(sessionID, ordinal),
			Role:  role,
			Text:  strings.TrimSpace(dataJSON),
			TS:    float64(timeCreated) / 1000.0,
			TSISO: t.Format(time.RFC3339),
		})
		ordinal++
	}
	return out, nil
}

func mintUUID(sessionID string, ordinal int) string {
	h := sha1.New()
	fmt.Fprintf(h, "opencode:%s:%d", sessionID, ordinal)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
