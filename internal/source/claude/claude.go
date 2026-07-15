// Package claude is the Source adapter for Claude Code transcripts under
// ~/.claude/projects (or $CLAUDE_CONFIG_DIR/projects): one project directory per
// working dir, one *.jsonl per session, subagents under a subagents/ subdir.
//
// It is a thin lift of the reader that lived inline in internal/index: Discover
// reuses internal/paths discovery and internal/index.SessionIDFor for lineage;
// Messages reproduces the parseTranscript flatten byte-for-byte via
// internal/parse, so ingesting through this adapter yields identical rows.
package claude

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
)

// Adapter reads Claude Code transcripts. It holds no state; a zero value is
// usable, and New is provided for symmetry with other adapters.
type Adapter struct{}

// Compile-time proof the adapter satisfies the port.
var _ source.Source = (*Adapter)(nil)

// New returns a ready Claude adapter.
func New() *Adapter { return &Adapter{} }

// Registration wires the Claude adapter into the source registry. Call
// source.Register(claude.Registration()) explicitly at start-up.
func Registration() source.Registration {
	return source.Registration{
		ID:     "claude",
		Detect: detect,
		New:    func() source.Source { return New() },
	}
}

// detect reports whether path lives under a Claude Code projects tree.
func detect(path string) bool {
	return strings.Contains(path, "/.claude/projects") ||
		(os.Getenv("CLAUDE_CONFIG_DIR") != "" && strings.Contains(path, "/projects/"))
}

// Discover enumerates every Claude session: each *.jsonl (top-level or subagent)
// under every project dir, tagged with the session id, subagent flag, and parent
// that internal/index.SessionIDFor derives from the path. Returns (nil, nil) when
// no projects exist — an empty corpus is not an error.
func (a *Adapter) Discover() ([]source.Container, error) {
	var out []source.Container
	for _, dir := range paths.AllProjectDirs() {
		cwd := paths.ProjectCWD(dir)
		for _, f := range paths.ContainedJSONL(dir) {
			sid, isSub, parent := index.SessionIDFor(f, dir)
			out = append(out, source.Container{
				ID:         sid,
				Path:       f,
				CWD:        cwd,
				IsSubagent: isSub == 1,
				ParentID:   parent,
				ResumeArgv: []string{"claude", "--resume", resumeID(sid)},
			})
		}
	}
	return out, nil
}

// Messages flattens one Claude transcript into normalized messages in file order.
// It mirrors the former index.parseTranscript exactly: skip non-indexable and
// empty-text records; one malformed line is skipped, not fatal. Malformed lines
// are counted and logged once (grouped, low-cardinality) rather than swallowed.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return nil, fmt.Errorf("claude: read %s: %w", c.Path, err)
	}
	var out []model.Message
	var bad int
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var o map[string]any
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			bad++
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
	if bad > 0 {
		slog.Warn("claude: skipped malformed jsonl lines", "count", bad, "path", c.Path)
	}
	return out, nil
}

// indexable reports whether o's "type" is one internal/parse indexes.
func indexable(o map[string]any) bool {
	t, _ := o["type"].(string)
	for _, it := range parse.IndexableTypes {
		if t == it {
			return true
		}
	}
	return false
}

// resumeID returns the id `claude --resume` expects: a subagent session id is
// "<parent>/<stem>", so take the final segment; a top-level id is already the stem.
func resumeID(sid string) string {
	if i := strings.LastIndex(sid, "/"); i >= 0 {
		return sid[i+1:]
	}
	return sid
}
