// Package claude is the Source adapter for Claude Code transcripts under
// ~/.claude/projects (or $CLAUDE_CONFIG_DIR/projects): one project directory per
// working dir, one *.jsonl per session, subagents under a subagents/ subdir.
//
// It is a thin lift of the reader that lived inline in internal/index: Discover
// reuses internal/paths discovery and internal/provenance.SessionIDFor for lineage;
// Messages reproduces the parseTranscript flatten byte-for-byte via
// internal/parse, so ingesting through this adapter yields identical rows.
package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/provenance"
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
		Lookup: lookup,
	}
}

func lookup(id string) ([]source.Container, error) {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return nil, nil
	}
	var out []source.Container
	if e, err := paths.ReadCatalogEntry(filepath.Join(paths.CatalogDir(), id)); err == nil && e.Source == "claude" && e.SessionID == id && e.TranscriptPath != "" {
		if fi, statErr := os.Stat(e.TranscriptPath); statErr == nil && fi.Mode().IsRegular() {
			cwd := e.CWD
			if cwd == "" {
				cwd = paths.FileCWD(e.TranscriptPath)
			}
			out = append(out, source.Container{ID: id, Path: e.TranscriptPath, CWD: cwd})
		}
	}
	for _, dir := range paths.AllProjectDirs() {
		p := filepath.Join(dir, id+".jsonl")
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			out = append(out, source.Container{ID: id, Path: p, CWD: paths.ProjectCWD(dir)})
		}
	}
	return out, nil
}

// detect reports whether path lives under a Claude Code projects tree.
func detect(path string) bool {
	return strings.Contains(path, "/.claude/projects") ||
		(os.Getenv("CLAUDE_CONFIG_DIR") != "" && strings.Contains(path, "/projects/"))
}

// Discover enumerates every Claude session: each *.jsonl (top-level or subagent)
// under every project dir, tagged with the session id, subagent flag, and parent
// that internal/provenance.SessionIDFor derives from the path. Returns (nil, nil) when
// no projects exist — an empty corpus is not an error.
func (a *Adapter) Discover() ([]source.Container, error) {
	var out []source.Container
	for _, dir := range paths.AllProjectDirs() {
		cwd := paths.ProjectCWD(dir)
		for _, f := range paths.ContainedJSONL(dir) {
			sid, isSub, parent := provenance.SessionIDFor(f, dir)
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

// TranscriptInfo holds the normalized messages and session metadata extracted
// from a Claude transcript.
type TranscriptInfo struct {
	Messages []model.Message
	Started  float64
	Last     float64
	CWD      string
}

// ParseTranscript parses a raw Claude Code JSONL transcript into normalized
// messages and session metadata, deduplicating records by message UUID.
func ParseTranscript(data []byte) (TranscriptInfo, error) {
	var info TranscriptInfo
	var bad int
	var startedSet, lastSet bool
	seenUUID := make(map[string]int)

	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var o map[string]any
		if err := json.Unmarshal(line, &o); err != nil {
			bad++
			continue
		}
		if info.CWD == "" {
			info.CWD = paths.LineCWD(o)
		}
		if !parse.IsIndexable(o) {
			continue
		}
		text := parse.ExtractText(o)
		if text == "" {
			continue
		}
		iso, _ := o["timestamp"].(string)
		ts := parse.ISOToEpoch(iso)
		u := parse.MsgUUID(o)
		msg := model.Message{
			Role:  parse.MsgRole(o),
			Text:  text,
			TS:    ts,
			TSISO: iso,
			UUID:  u,
		}
		realUUID, hasUUID := o["uuid"].(string)
		if hasUUID && realUUID != "" {
			if idx, ok := seenUUID[realUUID]; ok {
				info.Messages[idx] = msg
				continue
			}
			seenUUID[realUUID] = len(info.Messages)
		}
		info.Messages = append(info.Messages, msg)
		if ts != 0 {
			if !startedSet || ts < info.Started {
				info.Started, startedSet = ts, true
			}
			if !lastSet || ts > info.Last {
				info.Last, lastSet = ts, true
			}
		}
	}
	if bad > 0 {
		slog.Warn("claude: skipped malformed jsonl lines", "count", bad)
	}
	return info, nil
}

// Messages flattens one Claude transcript into normalized messages in file order.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return nil, fmt.Errorf("claude: read %s: %w", c.Path, err)
	}
	info, err := ParseTranscript(data)
	if err != nil {
		return nil, fmt.Errorf("claude: parse %s: %w", c.Path, err)
	}
	return info.Messages, nil
}

// resumeID returns the id `claude --resume` expects: a subagent session id is
// "<parent>/<stem>", so take the final segment; a top-level id is already the stem.
func resumeID(sid string) string {
	if i := strings.LastIndex(sid, "/"); i >= 0 {
		return sid[i+1:]
	}
	return sid
}
