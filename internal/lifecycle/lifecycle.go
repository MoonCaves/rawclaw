// Package lifecycle implements USER-DRIVEN session lifecycle operations over
// Claude Code transcripts: archiving a session's .jsonl out of the active
// projects tree, and deleting sessions behind a filter gate.
//
// Two safety invariants drive the design (prior-art consensus, all LLM-free):
//   - Delete is FILTER-GATED and DRY-RUN-FIRST: it refuses to delete every
//     session (>=1 filter must be set), and a dry run reports a plan without
//     touching disk.
//   - A real delete writes a TOMBSTONE sidecar (a plain text file, one session
//     id per line) so a later index pass can skip a deleted session instead of
//     resurrecting it. The tombstone is NOT a schema change — it is a flat file
//     the indexer consults.
//
// This package is intentionally self-contained: it resolves sessions by walking
// .jsonl files directly and counts messages by JSONL line count, so it carries
// no dependency on the index/parse internals. The CLI layer wires the
// subcommands, flags, and the y/N confirm around these functions.
package lifecycle

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrNoFilter is returned by Delete when the caller supplies no filter. Deleting
// every session must be an explicit, narrowed act — never the default.
var ErrNoFilter = errors.New("refusing to delete all sessions: set at least one filter")

// DeleteOpts carries the filter gate and the dry-run switch for Delete.
//
// At least one of Before / Project / MaxMessages must be set or Delete returns
// ErrNoFilter. The filters are ANDed: a session must satisfy every set filter to
// match. A zero-value field is "unset" and does not constrain the match.
type DeleteOpts struct {
	// Before, when non-zero, matches sessions whose transcript file was last
	// modified strictly before this instant.
	Before time.Time
	// Project, when non-empty, matches sessions whose transcript directory path
	// contains this substring (case-sensitive).
	Project string
	// MaxMessages, when > 0, matches sessions with at most this many messages
	// (JSONL lines). Use it to prune short/low-signal sessions.
	MaxMessages int
	// DryRun, when true, computes and returns the plan WITHOUT deleting anything
	// or writing a tombstone.
	DryRun bool
}

// any reports whether at least one filter is set. A bare DeleteOpts (only DryRun
// set, or nothing) is "no filter" and is refused.
func (o DeleteOpts) any() bool {
	return !o.Before.IsZero() || o.Project != "" || o.MaxMessages > 0
}

// PlanItem is one session matched by a Delete pass.
type PlanItem struct {
	SessionID string // .jsonl stem == the claude --resume id
	Path      string // absolute path to the .jsonl
	Project   string // friendly project label (basename of the transcript dir)
	Bytes     int64  // file size in bytes
	Messages  int    // JSONL line count
}

// DeletePlan is the result of a Delete pass — what matched and how much disk it
// reclaims. For a dry run, Deleted is false and nothing on disk changed; for a
// real delete, Deleted is true and Matched lists the sessions that were removed
// and tombstoned.
type DeletePlan struct {
	Matched       []PlanItem
	TotalBytes    int64
	Deleted       bool   // false for a dry run
	TombstonePath string // where ids were (or would be) appended
}

// Archive moves the session identified by sessionPathOrID into archiveDir,
// returning the new path. sessionPathOrID may be an absolute/relative path to a
// .jsonl, or a bare session id resolved against the projects tree (top-level
// sessions only).
//
// If archiveDir is empty it defaults to ~/.claude/archive/. The move is
// idempotent: if the source is already inside archiveDir (or the destination
// already exists and the source is gone), Archive reports success with the
// archived path rather than erroring.
func Archive(sessionPathOrID, archiveDir string) (string, error) {
	if archiveDir == "" {
		archiveDir = defaultArchiveDir()
	}
	archiveDir = expandHome(archiveDir)

	src, err := resolveSessionPath(sessionPathOrID)
	if err != nil {
		// Idempotency: if it is not on disk but already sits in the archive,
		// treat that as done rather than an error.
		if dst := filepath.Join(archiveDir, sessionFileName(sessionPathOrID)); fileExists(dst) {
			return dst, nil
		}
		return "", err
	}

	dst := filepath.Join(archiveDir, filepath.Base(src))

	// Already archived: source IS the destination (same realpath) — done.
	if sameFile(src, dst) {
		return dst, nil
	}

	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", fmt.Errorf("create archive dir %q: %w", archiveDir, err)
	}

	if err := moveFile(src, dst); err != nil {
		return "", fmt.Errorf("archive %q -> %q: %w", src, dst, err)
	}
	return dst, nil
}

// Delete removes the sessions under projectsRoot that match opts, gated by the
// filter requirement. projectsRoot is the Claude Code projects root (the dir
// holding the per-project transcript dirs). cacheDir is where the tombstone
// sidecar lives (typically ~/.cache/session-search); if empty it defaults to
// that path.
//
// Behavior:
//   - No filter set -> ErrNoFilter (never deletes everything).
//   - opts.DryRun true -> returns the plan, touches nothing on disk.
//   - Otherwise -> deletes each matched .jsonl and appends its id to the
//     tombstone, then returns the plan with Deleted=true.
func Delete(projectsRoot, cacheDir string, opts DeleteOpts) (DeletePlan, error) {
	plan := DeletePlan{
		TombstonePath: TombstonePath(cacheDir),
	}
	if !opts.any() {
		return plan, ErrNoFilter
	}

	matched, err := matchSessions(projectsRoot, opts)
	if err != nil {
		return plan, err
	}
	plan.Matched = matched
	for _, it := range matched {
		plan.TotalBytes += it.Bytes
	}

	if opts.DryRun {
		return plan, nil
	}

	// Real delete: remove each file, then record the id in the tombstone so a
	// reindex skips it. We tombstone the id only after the file is gone, so a
	// failed unlink does not leave a phantom tombstone for a still-present file.
	ids := make([]string, 0, len(matched))
	for _, it := range matched {
		if err := os.Remove(it.Path); err != nil {
			return plan, fmt.Errorf("delete %q: %w", it.Path, err)
		}
		ids = append(ids, it.SessionID)
	}
	if err := appendTombstones(plan.TombstonePath, ids); err != nil {
		return plan, fmt.Errorf("write tombstone %q: %w", plan.TombstonePath, err)
	}

	plan.Deleted = true
	return plan, nil
}

// TombstonePath returns the path to the tombstone sidecar file:
// <cacheDir>/.deleted. If cacheDir is empty it defaults to
// ~/.cache/session-search.
func TombstonePath(cacheDir string) string {
	if cacheDir == "" {
		cacheDir = defaultCacheDir()
	}
	return filepath.Join(expandHome(cacheDir), ".deleted")
}

// TombstoneIDs appends each id in ids to the tombstone sidecar under cacheDir,
// wrapping appendTombstones (same atomicity: create-dir, open-append, write).
// Exported so the CLI can tombstone RETAINED sessions matched by
// index.RetainedMatches — those have no backing file to os.Remove, so
// Delete's normal remove-then-tombstone path does not apply; this is the
// tombstone-only half for that case.
func TombstoneIDs(cacheDir string, ids []string) error {
	path := TombstonePath(cacheDir)
	if err := appendTombstones(path, ids); err != nil {
		return fmt.Errorf("write tombstone %q: %w", path, err)
	}
	return nil
}

// LoadTombstones reads the tombstone at <cacheDir>/.deleted and returns the set
// of deleted session ids. A missing file is not an error — it yields an empty
// set. Blank lines and surrounding whitespace are ignored.
func LoadTombstones(cacheDir string) (map[string]struct{}, error) {
	set := make(map[string]struct{}) // never nil — safe to range / read
	path := TombstonePath(cacheDir)

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return set, nil
		}
		return set, fmt.Errorf("open tombstone %q: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 1024*1024)
	for sc.Scan() {
		id := strings.TrimSpace(sc.Text())
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		return set, fmt.Errorf("read tombstone %q: %w", path, err)
	}
	return set, nil
}

// IsTombstoned reports whether sessionID appears in the tombstone under cacheDir.
// A missing tombstone means nothing is tombstoned (returns false, nil).
func IsTombstoned(cacheDir, sessionID string) (bool, error) {
	set, err := LoadTombstones(cacheDir)
	if err != nil {
		return false, err
	}
	_, ok := set[sessionID]
	return ok, nil
}

// matchSessions walks projectsRoot for top-level *.jsonl sessions and returns
// those satisfying every set filter, sorted by path for deterministic plans.
func matchSessions(projectsRoot string, opts DeleteOpts) ([]PlanItem, error) {
	root := expandHome(projectsRoot)
	dirs, err := projectDirs(root)
	if err != nil {
		return nil, err
	}

	out := []PlanItem{}
	for _, dir := range dirs {
		// Project filter is a substring on the transcript dir path.
		if opts.Project != "" && !strings.Contains(dir, opts.Project) {
			continue
		}
		items, err := matchInDir(dir, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// matchInDir evaluates the time/size filters for the top-level *.jsonl in one
// transcript dir. Project filtering is handled by the caller.
func matchInDir(dir string, opts DeleteOpts) ([]PlanItem, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", dir, err)
	}
	sort.Strings(files)

	label := filepath.Base(filepath.Clean(dir))
	out := []PlanItem{}
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil || info.IsDir() {
			continue // skip vanished / non-regular entries
		}
		if !opts.Before.IsZero() && !info.ModTime().Before(opts.Before) {
			continue
		}
		n, err := countLines(f)
		if err != nil {
			return nil, fmt.Errorf("count messages %q: %w", f, err)
		}
		if opts.MaxMessages > 0 && n > opts.MaxMessages {
			continue
		}
		out = append(out, PlanItem{
			SessionID: strings.TrimSuffix(filepath.Base(f), ".jsonl"),
			Path:      f,
			Project:   label,
			Bytes:     info.Size(),
			Messages:  n,
		})
	}
	return out, nil
}

// resolveSessionPath turns a path-or-id into an existing .jsonl path. A value
// that exists on disk (after ~ expansion) is used verbatim; otherwise it is
// treated as a session id and looked up among top-level sessions in the
// projects tree.
func resolveSessionPath(pathOrID string) (string, error) {
	cand := expandHome(pathOrID)
	if fileExists(cand) {
		return cand, nil
	}
	// Treat as a bare id: search the projects tree for "<id>.jsonl".
	root := defaultProjectsRoot()
	dirs, err := projectDirs(root)
	if err != nil {
		return "", err
	}
	want := sessionFileName(pathOrID)
	for _, dir := range dirs {
		p := filepath.Join(dir, want)
		if fileExists(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("session %q: %w", pathOrID, os.ErrNotExist)
}

// projectDirs returns the immediate subdirectories of root that hold at least
// one top-level *.jsonl. A missing root is not an error — it yields no dirs.
func projectDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read projects root %q: %w", root, err)
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		hits, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		if len(hits) > 0 {
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out, nil
}

// countLines counts newline-terminated and trailing-non-empty lines in a file —
// the JSONL message count. Empty file => 0.
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // transcript lines can be large
	n := 0
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			n++
		}
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("scan %q: %w", path, err)
	}
	return n, nil
}

// appendTombstones appends each id (one per line) to the tombstone file,
// creating the parent dir and file as needed. A nil/empty list is a no-op.
func appendTombstones(path string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create tombstone dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open tombstone: %w", err)
	}
	defer f.Close()

	var b strings.Builder
	for _, id := range ids {
		b.WriteString(id)
		b.WriteByte('\n')
	}
	if _, err := f.WriteString(b.String()); err != nil {
		return fmt.Errorf("append tombstone: %w", err)
	}
	return nil
}

// moveFile moves src to dst, falling back to copy+remove when os.Rename fails
// across filesystems (EXDEV).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device or other rename failure: copy then remove.
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close dest: %w", err)
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove source after copy: %w", err)
	}
	return nil
}

// --- small fs/path helpers (kept local so the package carries no deps) ---

func sessionFileName(pathOrID string) string {
	base := filepath.Base(pathOrID)
	if strings.HasSuffix(base, ".jsonl") {
		return base
	}
	return base + ".jsonl"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// sameFile reports whether src and dst refer to the same on-disk file (after
// symlink resolution), so an already-archived session is a no-op.
func sameFile(a, b string) bool {
	ra, ea := filepath.EvalSymlinks(a)
	rb, eb := filepath.EvalSymlinks(b)
	if ea != nil || eb != nil {
		return false
	}
	return ra == rb
}

func defaultArchiveDir() string   { return expandHome("~/.claude/archive") }
func defaultProjectsRoot() string { return expandHome("~/.claude/projects") }
func defaultCacheDir() string     { return expandHome("~/.cache/session-search") }

// expandHome replaces a leading "~" with the user's home dir ("~" and "~/..."
// forms only). Other inputs are returned unchanged.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}
