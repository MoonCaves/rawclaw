// Package live implements the live peek: "what is the agent on that machine
// doing right now". It has two halves behind one small surface — a serving
// half that enumerates and renders THIS machine's in-progress sessions
// straight from the transcript files (filesystem-fresh, the index and the
// archive are never touched), and a client half that SSHes a named machine
// and invokes the remote rawclaw's serving half over the pipe. One hop,
// seconds-fresh, raw bytes rendered without interpretation.
package live

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/MoonCaves/rawclaw/internal/source/claude"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
)

// DefaultListLimit caps the session list when the caller passes no limit.
const DefaultListLimit = 10

// Session is one row of the live session list — the JSON contract between the
// serving half (remote) and the client half (local). Age is computed on the
// serving machine against its own clock, so cross-machine clock skew never
// distorts the "how fresh is this" story.
type Session struct {
	SessionID    string `json:"session_id"`
	Source       string `json:"source"`  // "claude" | "codex"
	Project      string `json:"project"` // basename of the working dir ("" if unknown)
	CWD          string `json:"cwd,omitempty"`
	LastActivity string `json:"last_activity"` // marked-UTC RFC3339 (timefmt seam), from the file mtime
	AgeSeconds   int64  `json:"age_seconds"`   // now - mtime on the serving machine
	SizeBytes    int64  `json:"size_bytes"`
}

// ServeList writes the machine's top-level sessions as a JSON array ordered by
// last activity (file mtime) descending, capped at limit (<=0 = default). An
// empty corpus is an empty array, not an error — the client renders the
// "nothing recent" story. This is the pipe format `rawclaw live --serve`
// speaks; keep it append-only compatible.
func ServeList(w io.Writer, limit int) error {
	if limit <= 0 {
		limit = DefaultListLimit
	}
	rows := localSessions()
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].mtime.After(rows[j].mtime) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Session)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("emit session list: %w", err)
	}
	return nil
}

// timedSession pairs a Session with its raw mtime so ordering keeps
// sub-second precision the JSON contract rounds away.
type timedSession struct {
	Session
	mtime time.Time
}

// sources returns the transcript readers the serving half enumerates —
// the same adapters ingest uses, so live sees exactly what search would.
func sources() []source.Registration {
	return []source.Registration{claude.Registration(), codex.Registration()}
}

// localSessions enumerates every top-level session on this machine straight
// from the transcript trees. Subagent threads are excluded (they belong to
// their parent's story); a file that vanishes mid-walk is skipped.
func localSessions() []timedSession {
	now := time.Now()
	var out []timedSession
	for _, reg := range sources() {
		containers, err := reg.New().Discover()
		if err != nil {
			slog.Warn("live: source enumeration failed", "source", reg.ID, "err", err)
			continue
		}
		for _, c := range containers {
			if c.IsSubagent {
				continue
			}
			info, err := os.Stat(c.Path)
			if err != nil {
				continue // vanished mid-walk
			}
			age := int64(now.Sub(info.ModTime()).Seconds())
			if age < 0 {
				age = 0 // future mtime (clock jitter): clamp, don't invent negatives
			}
			out = append(out, timedSession{
				Session: Session{
					SessionID:    c.ID,
					Source:       reg.ID,
					Project:      projectName(c.CWD),
					CWD:          c.CWD,
					LastActivity: timefmt.UTC(info.ModTime()),
					AgeSeconds:   age,
					SizeBytes:    info.Size(),
				},
				mtime: info.ModTime(),
			})
		}
	}
	return out
}

// projectName is the friendly label for a working dir: its basename.
func projectName(cwd string) string {
	cwd = strings.TrimRight(cwd, "/")
	if cwd == "" {
		return ""
	}
	if i := strings.LastIndex(cwd, "/"); i >= 0 {
		return cwd[i+1:]
	}
	return cwd
}
