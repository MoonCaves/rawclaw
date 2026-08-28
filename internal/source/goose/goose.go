// Package goose is the Source adapter for Goose AI agent session transcripts
// under ~/.local/share/goose/sessions/ (or ~/.config/goose/sessions/,
// $GOOSE_HOME/sessions/). Goose stores sessions either in a monolithic shared
// sessions.db SQLite database (with sessions and messages tables) or as
// standalone per-session .db files.
//
// The adapter discovers all sessions, extracts session metadata (ID, CWD,
// lineage), and flattens message rows from SQLite into normalized model.Message
// records in order.
package goose

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// ID is the stable source name (source_tool column, --source flag).
const ID = "goose"

// Adapter reads Goose session SQLite databases.
type Adapter struct {
	roots []string
}

// Compile-time proof the adapter satisfies the Source port.
var _ source.Source = (*Adapter)(nil)

// New returns a ready Goose adapter with default discovery roots.
func New() *Adapter {
	return &Adapter{roots: SessionsRoots()}
}

// NewRoot returns a Goose adapter configured with a single explicit root directory.
func NewRoot(root string) *Adapter {
	return &Adapter{roots: []string{root}}
}

// NewRoots returns a Goose adapter configured with explicit root directories.
func NewRoots(roots ...string) *Adapter {
	return &Adapter{roots: roots}
}

// Registration wires the Goose adapter into the source registry.
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
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".db") {
				return nil
			}
			if c, ok := lookupDatabase(path, id); ok {
				out = append(out, c)
			}
			return nil
		})
		if err != nil {
			return nil, err
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
	var exists int
	if err := db.QueryRow("SELECT 1 FROM sqlite_master WHERE type IN ('table','view') AND name='sessions'").Scan(&exists); err == nil && exists == 1 {
		cols, err := tableColumns(db, "sessions")
		if err != nil {
			return source.Container{}, false
		}
		idCol := findMatchingCol(cols, "id", "session_id", "name")
		if idCol == "" || !isSafeIdent(idCol) {
			return source.Container{}, false
		}
		cwdCol := findMatchingCol(cols, "working_dir", "cwd", "workdir", "directory", "project_path")
		parentCol := findMatchingCol(cols, "parent_id", "parent_session_id", "parent", "parent_thread_id")
		isSubCol := findMatchingCol(cols, "is_subagent", "subagent", "is_sub")
		selectCols := []string{idCol}
		if cwdCol != "" && isSafeIdent(cwdCol) {
			selectCols = append(selectCols, cwdCol)
		} else {
			selectCols = append(selectCols, "NULL AS cwd")
		}
		if parentCol != "" && isSafeIdent(parentCol) {
			selectCols = append(selectCols, parentCol)
		} else {
			selectCols = append(selectCols, "NULL AS parent_id")
		}
		if isSubCol != "" && isSafeIdent(isSubCol) {
			selectCols = append(selectCols, isSubCol)
		} else {
			selectCols = append(selectCols, "NULL AS is_sub")
		}
		var gotID, cwd, parent, isSub any
		if err := db.QueryRow(fmt.Sprintf("SELECT %s FROM sessions WHERE %s=? LIMIT 1", strings.Join(selectCols, ", "), idCol), id).Scan(&gotID, &cwd, &parent, &isSub); err != nil || parseString(gotID) != id {
			return source.Container{}, false
		}
		return source.Container{
			ID:         id,
			Path:       path + "#" + id,
			CWD:        parseString(cwd),
			IsSubagent: parseBool(isSub),
			ParentID:   parseString(parent),
			ResumeArgv: []string{"goose", "session", "--resume", "--session-id", id},
		}, true
	}
	if c, ok, err := standaloneContainer(db, path); err == nil && ok && c.ID == id {
		return c, true
	}
	return source.Container{}, false
}

// detect reports whether path lives under a Goose sessions tree.
func detect(path string) bool {
	if !strings.HasSuffix(path, ".db") && !strings.Contains(path, ".db#") {
		return false
	}
	if strings.Contains(path, "/goose/sessions") || strings.Contains(path, "/.goose/sessions") {
		return true
	}
	if gh := os.Getenv("GOOSE_HOME"); gh != "" && strings.Contains(path, gh) {
		return true
	}
	return false
}

// SessionsRoots returns all potential directories containing Goose session .db files.
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

	if h := os.Getenv("GOOSE_HOME"); h != "" {
		add(filepath.Join(h, "sessions"))
		add(h)
	}
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		add(filepath.Join(xdgData, "goose", "sessions"))
	}
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		add(filepath.Join(xdgConfig, "goose", "sessions"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add(filepath.Join(home, ".local", "share", "goose", "sessions"))
		add(filepath.Join(home, ".config", "goose", "sessions"))
		add(filepath.Join(home, ".goose", "sessions"))
	}
	return roots
}

// Discover enumerates every Goose session across configured session roots.
func (a *Adapter) Discover() ([]source.Container, error) {
	roots := a.roots
	if len(roots) == 0 {
		roots = SessionsRoots()
	}
	return a.DiscoverRoots(roots)
}

// DiscoverRoots enumerates Goose sessions under the given directory roots.
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
			// Skip temporary SQLite files
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
				return fmt.Errorf("read database %s: %w", cleanPath, err)
			}
			out = append(out, containers...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("goose: discover %s: %w", root, err)
		}
	}

	return out, nil
}

// discoverDatabaseContainers inspects a SQLite database file. If a sessions table
// is found, it yields one container per session (keyed with path#id). Otherwise,
// it treats the entire file as a standalone single-session database.
func discoverDatabaseContainers(dbPath string) ([]source.Container, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)&_pragma=mmap_size(%d)", dbPath, store.ROMmapSize))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	var sessionsTableExists int
	err = db.QueryRow("SELECT 1 FROM sqlite_master WHERE type IN ('table', 'view') AND name='sessions' LIMIT 1").Scan(&sessionsTableExists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check sessions table: %w", err)
	}

	if sessionsTableExists == 1 {
		cols, err := tableColumns(db, "sessions")
		if err != nil {
			return nil, fmt.Errorf("inspect sessions table: %w", err)
		}
		if len(cols) > 0 {
			idCol := findMatchingCol(cols, "id", "session_id", "name")
			if idCol != "" && isSafeIdent(idCol) {
				cwdCol := findMatchingCol(cols, "working_dir", "cwd", "workdir", "directory", "project_path")
				parentCol := findMatchingCol(cols, "parent_id", "parent_session_id", "parent")
				isSubCol := findMatchingCol(cols, "is_subagent", "subagent", "is_sub")

				selectCols := []string{idCol}
				if cwdCol != "" && isSafeIdent(cwdCol) {
					selectCols = append(selectCols, cwdCol)
				} else {
					selectCols = append(selectCols, "'' AS cwd")
				}
				if parentCol != "" && isSafeIdent(parentCol) {
					selectCols = append(selectCols, parentCol)
				} else {
					selectCols = append(selectCols, "'' AS parent_id")
				}
				if isSubCol != "" && isSafeIdent(isSubCol) {
					selectCols = append(selectCols, isSubCol)
				} else {
					selectCols = append(selectCols, "0 AS is_sub")
				}

				query := fmt.Sprintf("SELECT %s FROM sessions", strings.Join(selectCols, ", "))
				rows, qErr := db.Query(query)
				if qErr != nil {
					return nil, fmt.Errorf("query sessions: %w", qErr)
				}
				defer rows.Close()
				res, rowsErr := sessionContainersFromRows(rows, dbPath)
				if rowsErr != nil {
					return nil, fmt.Errorf("iterate sessions: %w", rowsErr)
				}
				return res, nil
			}
		}
	}

	// Standalone single-session database fallback.
	c, ok, err := standaloneContainer(db, dbPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return []source.Container{c}, nil
}

// standaloneContainer identifies a per-session database and extracts its
// optional metadata. Unrelated SQLite files are ignored unless they have a
// recognizable Goose message table or session metadata table.
func standaloneContainer(db *sql.DB, dbPath string) (source.Container, bool, error) {
	defaultID := strings.TrimSuffix(filepath.Base(dbPath), ".db")
	metaTable, idCol, valCol, err := findMetadataTable(db)
	if err != nil {
		return source.Container{}, false, err
	}
	cwd, parentID := "", ""
	isSub := false
	metadataID := false
	metadata := metaTable != ""
	if metadata {
		rows, qErr := db.Query(fmt.Sprintf("SELECT %s, %s FROM %s LIMIT 10", idCol, valCol, metaTable))
		if qErr != nil {
			return source.Container{}, false, fmt.Errorf("query %s: %w", metaTable, qErr)
		}
		for rows.Next() {
			var rawKey, rawValue any
			if scanErr := rows.Scan(&rawKey, &rawValue); scanErr != nil {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(parseString(rawKey)))
			value := parseString(rawValue)
			switch key {
			case "id", "session_id":
				if value != "" {
					defaultID = value
					metadataID = true
				}
			case "cwd", "working_dir", "workdir", "directory", "project_path":
				cwd = value
			case "parent_id", "parent_session_id", "parent", "parent_thread_id":
				parentID = value
			case "is_subagent", "subagent", "is_sub":
				isSub = parseBool(rawValue)
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return source.Container{}, false, fmt.Errorf("iterate %s: %w", metaTable, rowsErr)
		}
		rows.Close()
	}

	if !metadataID {
		msgTable, err := findMessageTable(db)
		if err != nil {
			return source.Container{}, false, fmt.Errorf("find message table: %w", err)
		}
		if msgTable == "" || !messageTableSupported(db, msgTable) {
			return source.Container{}, false, nil
		}
	}
	if defaultID == "" {
		return source.Container{}, false, nil
	}
	return source.Container{
		ID:         defaultID,
		Path:       dbPath,
		CWD:        cwd,
		IsSubagent: isSub,
		ParentID:   parentID,
		ResumeArgv: []string{"goose", "session", "--resume", "--session-id", defaultID},
	}, true, nil
}

func findMetadataTable(db *sql.DB) (table, idCol, valCol string, err error) {
	for _, candidate := range []string{"session_meta", "metadata"} {
		var exists int
		if qErr := db.QueryRow("SELECT 1 FROM sqlite_master WHERE type IN ('table', 'view') AND name=? LIMIT 1", candidate).Scan(&exists); qErr != nil {
			if errors.Is(qErr, sql.ErrNoRows) {
				continue
			}
			return "", "", "", qErr
		}
		if exists != 1 {
			continue
		}
		cols, qErr := tableColumns(db, candidate)
		if qErr != nil {
			return "", "", "", qErr
		}
		id := findMatchingCol(cols, "id", "session_id", "key")
		value := findMatchingCol(cols, "value", "val", "data")
		if id != "" && value != "" && isSafeIdent(id) && isSafeIdent(value) {
			return candidate, id, value, nil
		}
	}
	return "", "", "", nil
}

func messageTableSupported(db *sql.DB, table string) bool {
	cols, err := tableColumns(db, table)
	if err != nil {
		return false
	}
	return findMatchingCol(cols, "role", "sender", "author", "type") != "" &&
		findMatchingCol(cols, "content_json", "content", "text", "body", "message", "payload") != ""
}

type rowsIterator interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func sessionContainersFromRows(rows rowsIterator, dbPath string) ([]source.Container, error) {
	var res []source.Container
	for rows.Next() {
		var (
			sid, cwd, parent any
			isSub            any
		)
		if scanErr := rows.Scan(&sid, &cwd, &parent, &isSub); scanErr != nil {
			return nil, scanErr
		}
		sID := parseString(sid)
		if sID != "" {
			sub := parseBool(isSub)
			res = append(res, source.Container{
				ID:         sID,
				Path:       dbPath + "#" + sID,
				CWD:        parseString(cwd),
				IsSubagent: sub,
				ParentID:   parseString(parent),
				ResumeArgv: []string{"goose", "session", "--resume", "--session-id", sID},
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// Messages extracts and normalizes all messages for the given session container.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	backingPath := c.Path
	if idx := strings.IndexByte(backingPath, '#'); idx >= 0 {
		backingPath = backingPath[:idx]
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)&_pragma=mmap_size(%d)", backingPath, store.ROMmapSize))
	if err != nil {
		return nil, fmt.Errorf("goose: open database %s: %w", backingPath, err)
	}
	defer db.Close()

	// Find the message table
	msgTable, err := findMessageTable(db)
	if err != nil {
		return nil, fmt.Errorf("goose: find message table %s: %w", backingPath, err)
	}
	if msgTable == "" || !isSafeIdent(msgTable) {
		return nil, nil // empty / unsupported
	}

	cols, err := tableColumns(db, msgTable)
	if err != nil {
		return nil, fmt.Errorf("goose: inspect columns %s: %w", backingPath, err)
	}
	if len(cols) == 0 {
		return nil, nil
	}

	idCol := findMatchingCol(cols, "id", "message_id", "mid", "rowid")
	roleCol := findMatchingCol(cols, "role", "sender", "author", "type")
	contentCol := findMatchingCol(cols, "content_json", "content", "text", "body", "message", "payload")
	tsCol := findMatchingCol(cols, "created_timestamp", "created_at", "timestamp", "created", "ts", "time", "date")
	sessionCol := findMatchingCol(cols, "session_id", "session", "sess_id", "thread_id")

	if roleCol == "" || !isSafeIdent(roleCol) || contentCol == "" || !isSafeIdent(contentCol) {
		return nil, nil
	}

	selectCols := []string{roleCol, contentCol}
	if idCol != "" && isSafeIdent(idCol) {
		selectCols = append([]string{idCol}, selectCols...)
	} else {
		selectCols = append([]string{"rowid"}, selectCols...)
	}
	if tsCol != "" && isSafeIdent(tsCol) {
		selectCols = append(selectCols, tsCol)
	} else {
		selectCols = append(selectCols, "'' AS ts")
	}

	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectCols, ", "), msgTable)
	var args []any
	if sessionCol != "" && c.ID != "" && isSafeIdent(sessionCol) {
		query += fmt.Sprintf(" WHERE %s = ?", sessionCol)
		args = append(args, c.ID)
	}

	// Determine ordering
	orderCol := ""
	if tsCol != "" && isSafeIdent(tsCol) {
		orderCol = tsCol
	} else if idCol != "" && isSafeIdent(idCol) {
		orderCol = idCol
	} else {
		orderCol = "rowid"
	}
	if !isSafeIdent(orderCol) {
		orderCol = "rowid"
	}
	query += fmt.Sprintf(" ORDER BY %s ASC", orderCol)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("goose: query messages %s: %w", backingPath, err)
	}
	defer rows.Close()

	var messages []model.Message
	ordinal := 0

	for rows.Next() {
		ordinal++
		var (
			rawID      any
			rawRole    string
			rawContent string
			rawTS      any
		)

		if err := rows.Scan(&rawID, &rawRole, &rawContent, &rawTS); err != nil {
			continue
		}

		role := normalizeRole(rawRole)
		content := extractContent(rawContent)
		if strings.TrimSpace(content) == "" {
			continue
		}

		ts, tsISO := parseTimestamp(rawTS)
		uuid := normalizeUUID(rawID, c.ID, ordinal)

		messages = append(messages, model.Message{
			Role:  role,
			Text:  content,
			TS:    ts,
			TSISO: tsISO,
			UUID:  uuid,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("goose: iterate messages %s: %w", backingPath, err)
	}

	return messages, nil
}

func findMessageTable(db *sql.DB) (string, error) {
	candidates := []string{"messages", "chat", "chat_messages", "events", "conversation_history", "history"}
	for _, cand := range candidates {
		var exists int
		err := db.QueryRow("SELECT 1 FROM sqlite_master WHERE type IN ('table', 'view') AND name=? LIMIT 1", cand).Scan(&exists)
		if err == nil && exists == 1 {
			return cand, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("check message table %s: %w", cand, err)
		}
	}
	return "", nil
}

func tableColumns(db *sql.DB, tableName string) ([]string, error) {
	if !isSafeIdent(tableName) {
		return nil, nil
	}
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, fmt.Errorf("query table info: %w", err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err == nil {
			name = strings.ToLower(strings.TrimSpace(name))
			if isSafeIdent(name) {
				cols = append(cols, name)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table info: %w", err)
	}
	return cols, nil
}

// isSafeIdent reports whether s is a valid and safe unquoted SQL identifier
// ([a-zA-Z_][a-zA-Z0-9_]*).
func isSafeIdent(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, r := range s {
		// First char may not be a digit: the documented grammar is
		// [a-zA-Z_][a-zA-Z0-9_]*. HEAD enforced only the charset, so "9x"
		// passed. De Morgan form keeps staticcheck QF1001 quiet.
		if i == 0 && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '_' {
			return false
		}
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func findMatchingCol(cols []string, matches ...string) string {
	for _, m := range matches {
		for _, col := range cols {
			if col == strings.ToLower(m) {
				return col
			}
		}
	}
	return ""
}

func normalizeRole(r string) string {
	r = strings.ToLower(strings.TrimSpace(r))
	switch r {
	case "user", "human":
		return "user"
	case "assistant", "model", "ai", "bot", "goose":
		return "assistant"
	case "system":
		return "system"
	case "tool", "tool_call", "tool_result":
		return "assistant"
	default:
		return "assistant"
	}
}

func extractContent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// If JSON payload (e.g. array of content blocks or struct)
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		var obj any
		if err := json.Unmarshal([]byte(raw), &obj); err == nil {
			switch v := obj.(type) {
			case map[string]any:
				if text, ok := v["text"].(string); ok && text != "" {
					return text
				}
				if content, ok := v["content"].(string); ok && content != "" {
					return content
				}
				if msg, ok := v["message"].(string); ok && msg != "" {
					return msg
				}
			case []any:
				var sb strings.Builder
				for _, item := range v {
					if m, ok := item.(map[string]any); ok {
						if text, ok := m["text"].(string); ok {
							sb.WriteString(text)
							sb.WriteString("\n")
						} else if content, ok := m["content"].(string); ok {
							sb.WriteString(content)
							sb.WriteString("\n")
						}
					}
				}
				if sb.Len() > 0 {
					return strings.TrimSpace(sb.String())
				}
			}
		}
	}
	return raw
}

func parseTimestamp(v any) (float64, string) {
	if v == nil {
		return 0, ""
	}

	switch val := v.(type) {
	case time.Time:
		return float64(val.UnixNano()) / 1e9, val.UTC().Format(time.RFC3339)
	case string:
		val = strings.TrimSpace(val)
		if val == "" {
			return 0, ""
		}
		// Try RFC3339
		if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
			return float64(t.UnixNano()) / 1e9, val
		}
		if t, err := time.Parse("2006-01-02 15:04:05", val); err == nil {
			return float64(t.Unix()), t.Format(time.RFC3339)
		}
		if t, err := time.Parse("2006-01-02T15:04:05", val); err == nil {
			return float64(t.Unix()), t.Format(time.RFC3339)
		}
		// Try numeric epoch string
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, time.Unix(int64(f), 0).UTC().Format(time.RFC3339)
		}
		return 0, val
	case int64:
		// Epoch seconds vs milliseconds vs microseconds
		if val > 1e14 {
			sec := val / 1e6
			return float64(sec), time.Unix(sec, 0).UTC().Format(time.RFC3339)
		} else if val > 1e11 {
			sec := val / 1000
			return float64(sec), time.Unix(sec, 0).UTC().Format(time.RFC3339)
		}
		return float64(val), time.Unix(val, 0).UTC().Format(time.RFC3339)
	case float64:
		return val, time.Unix(int64(val), 0).UTC().Format(time.RFC3339)
	default:
		return 0, fmt.Sprint(v)
	}
}

func normalizeUUID(rawID any, sessionID string, ordinal int) string {
	if rawID == nil {
		return mintUUID(sessionID, ordinal)
	}

	str := fmt.Sprint(rawID)
	str = strings.TrimSpace(str)
	if str == "" {
		return mintUUID(sessionID, ordinal)
	}

	// If valid hex (UUID / sha1 / md5 / 8+ hex chars)
	cleanHex := strings.ReplaceAll(str, "-", "")
	if len(cleanHex) >= 8 && isAllHex(cleanHex) {
		return strings.ToLower(str)
	}

	return mintUUID(sessionID, ordinal)
}

func mintUUID(sessionID string, ordinal int) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s:%d", sessionID, ordinal)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func isAllHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func parseBool(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val != 0
	case string:
		val = strings.ToLower(strings.TrimSpace(val))
		return val == "true" || val == "1" || val == "yes" || val == "sub_agent" || val == "subagent"
	default:
		return false
	}
}

func parseString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprint(v)
	}
}
