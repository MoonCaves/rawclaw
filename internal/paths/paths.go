// Package paths holds transcript-directory discovery: the projects root,
// cwd→project-dir resolution (by matching the cwd recorded inside transcripts),
// contained-JSONL enumeration with symlink-out containment, and session-id
// resolution.
package paths

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SessionHit is one result of ResolveSession: a top-level session whose id
// starts with the requested prefix. It carries the full session id, the working
// dir recorded in the transcript, and a friendly project label.
type SessionHit struct {
	SessionID string // full session id (the .jsonl stem == claude --resume id)
	Path      string // backing transcript file path
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
	return ExpandHome("~/.claude/projects")
}

// ConfigDir returns the Claude Code config dir: $CLAUDE_CONFIG_DIR if set, else
// ~/.claude. Unlike ProjectsRoot, this never requires the dir to already exist —
// a writer (e.g. `rawclaw setup`, which creates settings.json and the hooks dir
// on a fresh machine) needs the resolved path before anything is on disk yet.
func ConfigDir() string {
	if cc := os.Getenv("CLAUDE_CONFIG_DIR"); cc != "" {
		return cc
	}
	return ExpandHome("~/.claude")
}

func rawclawDataDir(subdir string) string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "rawclaw", subdir)
	}
	return ExpandHome(filepath.Join("~/.local/share/rawclaw", subdir))
}

// TranscriptsRoot is the durable home for rawclaw's OWN copy of every session
// it indexes — the transcript vault. It lives in the XDG DATA dir, NOT the
// disposable cache (which holds only rebuildable index dbs), because the vault
// is the truth the dbs are rebuilt FROM: deleting the cache must cost nothing.
// $XDG_DATA_HOME/rawclaw/transcripts, else ~/.local/share/rawclaw/transcripts.
// A vault file's mtime records when rawclaw last snapshotted that session,
// NOT when the session was last active: freshness comes from the live
// sources, which are re-indexed on every invoke.
func TranscriptsRoot() string {
	return rawclawDataDir("transcripts")
}

// CatalogDir is the durable home for rawclaw's session catalog: one flat
// directory under the rawclaw data home holding one entry per session id
// written at session birth. Honors $RAWCLAW_CATALOG_DIR if set, else
// $XDG_DATA_HOME/rawclaw/catalog (or ~/.local/share/rawclaw/catalog).
func CatalogDir() string {
	if c := os.Getenv("RAWCLAW_CATALOG_DIR"); c != "" {
		return c
	}
	return rawclawDataDir("catalog")
}

// TombstonePath returns the durable path for deleted session IDs:
// $XDG_DATA_HOME/rawclaw/.deleted, else ~/.local/share/rawclaw/.deleted.
func TombstonePath() string {
	return filepath.Join(rawclawDataDir(""), ".deleted")
}

// CatalogEntry is one entry in the durable session catalog.
type CatalogEntry struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	CWD            string `json:"cwd,omitempty"`
	Source         string `json:"source,omitempty"`
}

// ReadCatalogEntry reads and parses a catalog entry from disk. Readers must
// tolerate unparseable entries (which serve as pure dedup markers).
func ReadCatalogEntry(path string) (CatalogEntry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return CatalogEntry{}, err
	}
	var e CatalogEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return CatalogEntry{}, err
	}
	return e, nil
}

// WriteCatalogEntry writes a catalog entry to dir/<session_id> atomically.
func WriteCatalogEntry(catalogDir string, entry CatalogEntry) error {
	cleanSessionID := filepath.Clean(entry.SessionID)
	isDot := cleanSessionID == "."
	isClean := cleanSessionID == entry.SessionID
	isFlat := filepath.Base(cleanSessionID) == cleanSessionID
	isLocal := filepath.IsLocal(cleanSessionID)
	if isDot || !isClean || !isFlat || !isLocal {
		return fmt.Errorf("invalid session id: %q", entry.SessionID)
	}
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(catalogDir, entry.SessionID)
	tmp := filepath.Join(catalogDir, fmt.Sprintf(".tmp.%s.%d", entry.SessionID, os.Getpid()))
	defer func() { _ = os.Remove(tmp) }()
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	// fsync before rename: without it, os.Rename can make the temp file's
	// NAME durable while its CONTENT is still only in the page cache, so a
	// crash between write and rename can leave target as a 0-byte or
	// truncated file instead of the old or new content.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// FindTranscriptDir resolves the projects subdir for `cwd` by matching the cwd
// recorded inside transcripts (authoritative), falling back to path encoding.
// Returns "" if none found.
//
// Discovery is location-based only: the known projects root and the cwds its
// transcripts record. A directory that merely holds loose *.jsonl files is NOT
// treated as a transcripts dir here — that fallback once let a bare run from a
// folder like /tmp index the folder itself into the cache. Arbitrary-folder
// indexing is the explicit --dir opt-in: FindTranscriptDirExplicit.
func FindTranscriptDir(cwd string) string {
	target := Realpath(ExpandHome(cwd))
	root := ProjectsRoot()

	// Footgun guard: if the caller already passed a transcripts dir (a child of
	// projects/), use it verbatim — don't re-encode an already-encoded path
	// into nothing.
	if isDir(target) && Realpath(filepath.Dir(target)) == Realpath(root) {
		return target
	}

	if isDir(root) {
		entries, _ := filepath.Glob(filepath.Join(root, "*"))
		slices.Sort(entries)
		for _, d := range entries {
			if !isDir(d) {
				continue
			}
			files, _ := filepath.Glob(filepath.Join(d, "*.jsonl"))
			slices.Sort(files)
			for _, f := range files { // check ALL top-level files, not just first
				rec := firstCWD(f)
				if rec != "" && Realpath(rec) == target {
					return d
				}
			}
		}

		cand := filepath.Join(root, encodePath(target))
		if isDir(cand) {
			return cand
		}

		// Worktree fallback: if this working dir has no transcripts of its own,
		// resolve its parent repository root if it is a Git worktree.
		if gitRoot := GitCommonRoot(target); gitRoot != "" && gitRoot != target {
			for _, d := range entries {
				if !isDir(d) {
					continue
				}
				files, _ := filepath.Glob(filepath.Join(d, "*.jsonl"))
				slices.Sort(files)
				for _, f := range files {
					rec := firstCWD(f)
					if rec != "" && Realpath(rec) == gitRoot {
						return d
					}
				}
			}
			candGit := filepath.Join(root, encodePath(gitRoot))
			if isDir(candGit) {
				return candGit
			}
		}
	}

	return ""
}

// GitCommonRoot returns the root directory of the repository enclosing dir,
// resolving Git worktree pointers (where .git is a file containing "gitdir: ...")
// to the parent repository root. Returns "" if dir is not inside a git repo.
func GitCommonRoot(dir string) string {
	dir = Realpath(ExpandHome(dir))
	curr := dir
	for {
		gitPath := filepath.Join(curr, ".git")
		if fi, err := os.Stat(gitPath); err == nil {
			if fi.IsDir() {
				return curr
			}
			// Worktree: .git is a file
			data, err := os.ReadFile(gitPath)
			if err == nil {
				content := strings.TrimSpace(string(data))
				if gitDir, ok := strings.CutPrefix(content, "gitdir:"); ok {
					gitDir = strings.TrimSpace(gitDir)
					if !filepath.IsAbs(gitDir) {
						gitDir = filepath.Join(curr, gitDir)
					}
					gitDir = Realpath(gitDir)
					// If commondir exists inside gitDir, follow it
					commonFile := filepath.Join(gitDir, "commondir")
					if cdata, err := os.ReadFile(commonFile); err == nil {
						commonRel := strings.TrimSpace(string(cdata))
						commonDir := filepath.Clean(filepath.Join(gitDir, commonRel))
						return filepath.Dir(commonDir)
					}
					// Fallback: gitDir is <main>/.git/worktrees/<name>
					return filepath.Clean(filepath.Join(gitDir, "../../.."))
				}
			}
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}

// FindTranscriptDirExplicit resolves `dir` like FindTranscriptDir, additionally
// accepting ANY directory that directly holds *.jsonl files. This is the
// explicit --dir opt-in for indexing an arbitrary transcript folder; the
// ordinary resolution runs first, so a working dir that happens to carry stray
// .jsonl data files still resolves to its recorded transcripts dir.
func FindTranscriptDirExplicit(dir string) string {
	if td := FindTranscriptDir(dir); td != "" {
		return td
	}
	target := Realpath(ExpandHome(dir))
	if isDir(target) {
		if hits, _ := filepath.Glob(filepath.Join(target, "*.jsonl")); len(hits) > 0 {
			return target
		}
	}
	return ""
}

// ContainedJSONL returns the recursive *.jsonl under transcriptDir, EXCLUDING
// any whose realpath escapes the root (symlink-out containment).
func ContainedJSONL(transcriptDir string) []string {
	rootRP := strings.TrimRight(Realpath(transcriptDir), string(os.PathSeparator))
	out := []string{}

	matches := globRecursiveJSONL(transcriptDir)
	for _, f := range matches {
		rp := Realpath(f)
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
	if rec := DirCWD(tdir); rec != "" {
		// basename of the recorded cwd (trailing slash stripped), else enc.
		if base := baseName(strings.TrimRight(rec, "/")); base != "" {
			return base
		}
	}
	return enc
}

// DirCWD returns the working directory this project dir's transcripts record,
// or "" when none does. It samples ONE top-level transcript because a project
// dir maps 1:1 to a working directory by construction — the dir's name IS that
// path, encoded — so one sample answers for every session in the dir.
//
// Unlike ProjectCWD it never substitutes the directory's own name for a real
// path: a caller storing a cwd needs to tell "the sessions here ran in /x/y"
// apart from "nothing here records where it ran."
func DirCWD(tdir string) string {
	if f := firstTopLevelJSONL(tdir); f != "" {
		return firstCWD(f)
	}
	return ""
}

// ProjectDirOf returns the project dir a transcript file belongs to: the
// ancestor sitting directly under the projects root. Returns "" when the file
// is not under the projects root at all (an explicit --dir scope, or a source
// that shards by date rather than by project).
//
// The file's own parent directory is NOT the answer in general: subagent and
// workflow transcripts live in SUBDIRECTORIES of their project dir, so reading
// the parent labels them "subagents" or "wf_<id>" instead of the project they
// actually ran in. Resolution is lexical, so it still answers for a transcript
// that has since been purged from disk.
func ProjectDirOf(jsonlPath string) string {
	sep := string(os.PathSeparator)
	root := strings.TrimRight(Realpath(ProjectsRoot()), sep)
	dir := Realpath(filepath.Dir(jsonlPath))
	for {
		parent := filepath.Dir(dir)
		if strings.TrimRight(parent, sep) == root {
			return dir
		}
		if parent == dir { // reached the filesystem root without meeting it
			return ""
		}
		dir = parent
	}
}

// AllProjectDirs returns every project dir under the projects root that holds
// at least one top-level *.jsonl.
func AllProjectDirs() []string {
	root := ProjectsRoot()
	entries, _ := filepath.Glob(filepath.Join(root, "*"))
	slices.Sort(entries)

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
	if c := DirCWD(tdir); c != "" {
		return c
	}
	return filepath.Base(filepath.Clean(tdir))
}

// ResolveSession finds the TOP-LEVEL session(s) whose id starts with `prefix`
// (the 8-char label printed in search output). Subagent threads are skipped.
// It checks the durable session catalog first for O(1) direct or flat-dir prefix
// resolution, falling back to project-dir stem resolution if the catalog misses.
func ResolveSession(prefix string) []SessionHit {
	if hits := resolveSessionCatalog(prefix); len(hits) > 0 {
		return hits
	}
	return resolveSessionStem(prefix)
}

func resolveSessionCatalog(prefix string) []SessionHit {
	catDir := CatalogDir()
	if catDir == "" {
		return nil
	}

	// 1. Direct O(1) lookup if prefix is an exact session id / file name.
	if prefix != "" && !strings.ContainsRune(prefix, os.PathSeparator) && !strings.ContainsRune(prefix, '/') {
		exactPath := filepath.Join(catDir, prefix)
		if entry, err := ReadCatalogEntry(exactPath); err == nil {
			if hit, ok := validateCatalogHit(entry, prefix); ok {
				return []SessionHit{hit}
			}
		}
	}

	// 2. Prefix scan: readdir the flat catalog directory.
	entries, err := os.ReadDir(catDir)
	if err != nil {
		return nil
	}

	var hits []SessionHit
	for _, de := range entries {
		if de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		if !strings.HasPrefix(de.Name(), prefix) {
			continue
		}
		entryPath := filepath.Join(catDir, de.Name())
		entry, err := ReadCatalogEntry(entryPath)
		if err != nil {
			continue
		}
		if hit, ok := validateCatalogHit(entry, prefix); ok {
			hits = append(hits, hit)
		}
	}
	return hits
}

func validateCatalogHit(entry CatalogEntry, prefix string) (SessionHit, bool) {
	if entry.SessionID == "" || !strings.HasPrefix(entry.SessionID, prefix) {
		return SessionHit{}, false
	}
	if entry.TranscriptPath == "" {
		return SessionHit{}, false
	}
	fi, err := os.Stat(entry.TranscriptPath)
	if err != nil || fi.IsDir() {
		return SessionHit{}, false
	}
	return sessionHitFromCatalog(entry), true
}

func sessionHitFromCatalog(entry CatalogEntry) SessionHit {
	cwd := entry.CWD
	if cwd == "" {
		cwd = firstCWD(entry.TranscriptPath)
	}
	var proj string
	if pdir := ProjectDirOf(entry.TranscriptPath); pdir != "" {
		proj = ProjectLabel(pdir)
	} else if cwd != "" {
		if base := baseName(strings.TrimRight(cwd, "/")); base != "" {
			proj = base
		} else {
			proj = cwd
		}
	}
	if proj == "" {
		proj = ProjectLabel(filepath.Dir(entry.TranscriptPath))
	}
	return SessionHit{
		SessionID: entry.SessionID,
		Path:      entry.TranscriptPath,
		CWD:       cwd,
		Project:   proj,
	}
}

func resolveSessionStem(prefix string) []SessionHit {
	hits := []SessionHit{}
	for _, d := range AllProjectDirs() {
		files, _ := filepath.Glob(filepath.Join(d, "*.jsonl")) // top-level only (no subagents/ recursion)
		slices.Sort(files)
		for _, f := range files {
			stem := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			if strings.HasPrefix(stem, prefix) {
				hits = append(hits, SessionHit{
					SessionID: stem,
					Path:      f,
					CWD:       firstCWD(f),
					Project:   ProjectLabel(d),
				})
			}
		}
	}
	return hits
}

// FileCWD returns the working directory a single transcript file records, or ""
// when the file is unreadable or records none. It is firstCWD's exported form:
// the scope helpers above answer "what cwd does this PROJECT DIR have", while a
// per-session backfill needs the cwd of one named file.
func FileCWD(jsonlPath string) string { return firstCWD(jsonlPath) }

// LineCWD returns the working dir one transcript line records — top level, else
// nested under "message" — or "" when it records none.
func LineCWD(o map[string]any) string {
	if cwd, ok := o["cwd"].(string); ok && cwd != "" {
		return cwd
	}
	if msg, ok := o["message"].(map[string]any); ok {
		if cwd, ok := msg["cwd"].(string); ok && cwd != "" {
			return cwd
		}
	}
	return ""
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
		if cwd := LineCWD(o); cwd != "" {
			return cwd
		}
	}
	return ""
}

// firstTopLevelJSONL returns the first (sorted) top-level *.jsonl in tdir, or
// "". Used by project_label / project_cwd to sample one transcript.
func firstTopLevelJSONL(tdir string) string {
	files, _ := filepath.Glob(filepath.Join(tdir, "*.jsonl"))
	if len(files) == 0 {
		return ""
	}
	slices.Sort(files)
	return files[0]
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
	slices.Sort(out)
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

// ExpandHome replaces a leading "~" with the user's home directory, handling
// the "~" and "~/..." forms. Other forms are returned unchanged (we never see
// "~user" here).
func ExpandHome(path string) string {
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
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

// Realpath canonicalizes a path, resolving symlinks for the components that
// exist and lexically normalizing the rest.
// It never errors — a path that does not exist is returned cleaned/absolute.
// filepath.EvalSymlinks errors on missing components, so we resolve the longest
// existing prefix and re-append the non-existent tail.
func Realpath(path string) string {
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
