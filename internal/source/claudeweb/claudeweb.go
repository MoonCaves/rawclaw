// Package claudeweb is the Source adapter for imported Claude cloud history —
// the "claude-web" source: conversations from claude.ai / the Desktop app /
// Cowork that live on the account, with no local transcript and no sync API.
//
// `rawclaw import <zip|dir>` MATERIALIZES each exported conversation as a raw
// JSONL transcript (Claude record shape) under
// paths.ClaudeWebRoot()/<account>/<conversation-uuid>.jsonl (see materialize.go
// + export.go). This adapter then READS those files exactly like the claude
// adapter reads ~/.claude/projects JSONL — so claude-web is a NORMAL source with
// a durable, rebuildable origin: the index db demotes to a disposable cache, and
// the raw files ride retention + the transcript archive like every other source.
//
// Prior art (the in-repo model): internal/source/claude — Discover enumerates
// per-directory *.jsonl into Containers; Messages flattens one file via
// internal/parse (ExtractText/MsgRole/MsgUUID), byte-for-byte, so ingesting
// through this adapter yields identical rows.
package claudeweb

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
)

// ID is the stable source name (source_tool column, --source flag).
const ID = "claude-web"

// transcriptExt is the materialized transcript file extension.
const transcriptExt = ".jsonl"

// Adapter reads materialized claude-web transcripts under a root
// (paths.ClaudeWebRoot() in production; an explicit root in tests / the import
// reindex). Stateless; the zero value with an empty root reads nothing.
type Adapter struct {
	root string
}

// Compile-time proof the adapter satisfies the ingest port.
var _ source.Source = (*Adapter)(nil)

// New returns an adapter over the production transcript root.
func New() *Adapter { return &Adapter{root: paths.ClaudeWebRoot()} }

// NewRoot returns an adapter over an explicit transcript root — used by the
// import reindex and by tests.
func NewRoot(root string) *Adapter { return &Adapter{root: root} }

// Discover enumerates every materialized conversation: <root>/<account>/*.jsonl.
// Each file is one conversation, its session id the filename stem (the
// conversation uuid). CWD is "" (the export drops the working dir). An absent
// root yields (nil, nil) — an empty corpus is not an error.
func (a *Adapter) Discover() ([]source.Container, error) {
	accountDirs, _ := filepath.Glob(filepath.Join(a.root, "*"))
	sort.Strings(accountDirs)
	var out []source.Container
	for _, accDir := range accountDirs {
		if !isDir(accDir) {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(accDir, "*"+transcriptExt))
		sort.Strings(files)
		for _, f := range files {
			out = append(out, source.Container{
				ID:   convUUIDFromFile(f),
				Path: f,
				CWD:  "",
			})
		}
	}
	return out, nil
}

// Messages flattens one materialized transcript into normalized messages in file
// order — the SAME loop as the claude adapter (skip non-indexable / empty-text
// records; a malformed line is skipped, not fatal), so a materialized record
// round-trips to an identical model.Message.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return nil, fmt.Errorf("claude-web: read %s: %w", c.Path, err)
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
		slog.Warn("claude-web: skipped malformed jsonl lines", "count", bad, "path", c.Path)
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

// convUUIDFromFile derives the conversation uuid (session id) from a transcript
// filename: its stem (<uuid>.jsonl -> <uuid>).
func convUUIDFromFile(path string) string {
	return strings.TrimSuffix(filepath.Base(path), transcriptExt)
}

// AccountDirName returns the account-directory segment of a materialized
// transcript path — the grouping key the scopes layer uses to split per-account
// cache dbs (parallel to how codex groups by cwd).
func AccountDirName(path string) string {
	return filepath.Base(filepath.Dir(path))
}

// isDir reports whether path is a directory.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
