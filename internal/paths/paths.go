// Package paths holds transcript-directory discovery: the projects root,
// cwd→project-dir resolution (by matching the cwd recorded inside transcripts),
// contained-JSONL enumeration with symlink-out containment, and session-id
// resolution.
package paths

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SessionHit is one result of ResolveSession: a top-level session whose id
// starts with the requested prefix. It carries the full session id, the working
// dir recorded in the transcript, and a friendly project label.
type SessionHit struct {
	SessionID string // full session id (the .jsonl stem == claude --resume id)
	CWD       string // working dir recorded in the transcript (may be "")
	Project   string // friendly project label
}

// ProjectsRoot returns the Claude Code projects root: $CLAUDE_CONFIG_DIR/projects
// if it exists, else ~/.claude/projects.
func ProjectsRoot() string {
	cc := os.Getenv("CLAUDE_CONFIG_DIR")
	if cc != "" {
		candidate := filepath.Join(cc, "projects")
		if isDir(candidate) {
			return candidate
		}
	}
	return expandHome("~/.claude/projects")
}

// ConfigDir returns the Claude Code config dir: $CLAUDE_CONFIG_DIR if set, else
// ~/.claude. Unlike ProjectsRoot, this never requires the dir to already exist —
// a writer (e.g. `rawclaw setup`, which creates settings.json and the hooks dir
// on a fresh machine) needs the resolved path before anything is on disk yet.
func ConfigDir() string {
	if cc := os.Getenv("CLAUDE_CONFIG_DIR"); cc != "" {
		return cc
	}
	return expandHome("~/.claude")
}

// FindTranscriptDir resolves the projects subdir for `cwd` by matching the cwd
// recorded inside transcripts (authoritative), falling back to path encoding.
// Returns "" if none found.
func FindTranscriptDir(cwd string) string {
	target := realpath(expandHome(cwd))
	root := ProjectsRoot()

	// Footgun guard: if the caller already passed a transcripts dir (a child of
	// projects/, or any dir that directly holds *.jsonl), use it verbatim — don't
	// re-encode an already-encoded path into nothing.
	if isDir(target) {
		if realpath(filepath.Dir(target)) == realpath(root) {
			return target
		}
		if hits, _ := filepath.Glob(filepath.Join(target, "*.jsonl")); len(hits) > 0 {
			return target
		}
	}

	if isDir(root) {
		entries, _ := filepath.Glob(filepath.Join(root, "*"))
		sort.Strings(entries)
		for _, d := range entries {
			if !isDir(d) {
				continue
			}
			files, _ := filepath.Glob(filepath.Join(d, "*.jsonl"))
			sort.Strings(files)
			for _, f := range files { // check ALL top-level files, not just first
				rec := firstCWD(f)
				if rec != "" && realpath(rec) == target {
					return d
				}
			}
		}
	}

	cand := filepath.Join(root, encodePath(target))
	if isDir(cand) {
		return cand
	}
	return ""
}

// ContainedJSONL returns the recursive *.jsonl under transcriptDir, EXCLUDING
// any whose realpath escapes the root (symlink-out containment).
func ContainedJSONL(transcriptDir string) []string {
	rootRP := strings.TrimRight(realpath(transcriptDir), string(os.PathSeparator))
	out := []string{}

	matches := globRecursiveJSONL(transcriptDir)
	for _, f := range matches {
		rp := realpath(f)
		// Containment check: rp == join(root, relpath(rp, root)) && rp startswith root+sep.
		// The first clause holds whenever rp is lexically under rootRP; combined with
		// the prefix check it rejects anything whose realpath escapes the root.
		rel, err := filepath.Rel(rootRP, rp)
		if err != nil {
			continue
		}
		reconstructed := filepath.Join(rootRP, rel)
		if rp == reconstructed && strings.HasPrefix(rp, rootRP+string(os.PathSeparator)) {
			out = append(out, f)
		}
	}
	return out
}

// ProjectLabel returns a friendly project name = basename of the cwd recorded
// in a transcript, else the encoded dir basename.
func ProjectLabel(tdir string) string {
	enc := filepath.Base(filepath.Clean(tdir))
	for _, f := range firstTopLevelJSONL(tdir) {
		rec := firstCWD(f)
		if rec != "" {
			// basename of the recorded cwd (trailing slash stripped), else enc.
			if base := baseName(strings.TrimRight(rec, "/")); base != "" {
				return base
			}
			return enc
		}
	}
	return enc
}

// AllProjectDirs returns every project dir under the projects root that holds
// at least one top-level *.jsonl.
func AllProjectDirs() []string {
	root := ProjectsRoot()
	entries, _ := filepath.Glob(filepath.Join(root, "*"))
	sort.Strings(entries)

	out := []string{}
	for _, d := range entries {
		if !isDir(d) {
			continue
		}
		if hits, _ := filepath.Glob(filepath.Join(d, "*.jsonl")); len(hits) > 0 {
			out = append(out, d)
		}
	}
	return out
}

// ProjectCWD returns the working directory recorded in this project's
// transcripts (for path filtering), falling back to the encoded dir name.
func ProjectCWD(tdir string) string {
	for _, f := range firstTopLevelJSONL(tdir) {
		if c := firstCWD(f); c != "" {
			return c
		}
	}
	return filepath.Base(filepath.Clean(tdir))
}

// ResolveSession finds the TOP-LEVEL session(s) whose id starts with `prefix`
// (the 8-char label printed in search output). Subagent threads are skipped.
func ResolveSession(prefix string) []SessionHit {
	hits := []SessionHit{}
	for _, d := range AllProjectDirs() {
		files, _ := filepath.Glob(filepath.Join(d, "*.jsonl")) // top-level only (no subagents/ recursion)
		sort.Strings(files)
		for _, f := range files {
			stem := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			if strings.HasPrefix(stem, prefix) {
				hits = append(hits, SessionHit{
					SessionID: stem,
					CWD:       firstCWD(f),
					Project:   ProjectLabel(d),
				})
			}
		}
	}
	return hits
}

// firstCWD reads jsonlPath line by line and returns the first non-empty string
// `cwd` (top-level, else nested under "message"). Returns "" on any failure.
func firstCWD(jsonlPath string) string {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // transcript lines can be large
	for sc.Scan() {
		var o map[string]any
		if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
			continue
		}
		if cwd, ok := o["cwd"].(string); ok && cwd != "" {
			return cwd
		}
		if msg, ok := o["message"].(map[string]any); ok {
			if cwd, ok := msg["cwd"].(string); ok && cwd != "" {
				return cwd
			}
		}
	}
	return ""
}

// firstTopLevelJSONL returns the first (sorted) top-level *.jsonl in tdir, or an
// empty slice. Used by project_label / project_cwd to sample one transcript.
func firstTopLevelJSONL(tdir string) []string {
	files, _ := filepath.Glob(filepath.Join(tdir, "*.jsonl"))
	if len(files) == 0 {
		return nil
	}
	sort.Strings(files)
	return files[:1]
}

// globRecursiveJSONL returns every *.jsonl at any depth under root (including
// directly in root). Symlinked directories are NOT followed (filepath.WalkDir
// does not descend symlinks).
func globRecursiveJSONL(root string) []string {
	out := []string{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep walking
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// encodePath is Claude Code's project-dir encoding: every "/" and "." becomes
// "-".
func encodePath(p string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(p)
}

// baseName returns the substring after the final "/" with NO normalization.
// Unlike filepath.Base it returns "" for "" and for a trailing-slash path, and
// never collapses to ".".
func baseName(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// isDir reports whether path exists and is a directory (symlinks followed via
// os.Stat).
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// expandHome replaces a leading "~" with the user's home directory, handling
// the "~" and "~/..." forms. Other forms are returned unchanged (we never see
// "~user" here).
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path // leave it untouched when HOME is unknown
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// realpath canonicalizes a path, resolving symlinks for the components that
// exist and lexically normalizing the rest.
// It never errors — a path that does not exist is returned cleaned/absolute.
// filepath.EvalSymlinks errors on missing components, so we resolve the longest
// existing prefix and re-append the non-existent tail.
func realpath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}

	// Walk up to the longest existing prefix, EvalSymlinks it, re-append the tail.
	tail := []string{}
	cur := abs
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			parts := append([]string{resolved}, tail...)
			return filepath.Join(parts...)
		}
		parent := filepath.Dir(cur)
		if parent == cur { // reached the root, nothing resolved
			return abs
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}
