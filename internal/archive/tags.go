package archive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// tagsDirName is the per-machine tags subtree inside a machine dir, a SIBLING of
// the `claude/` and `codex/` transcript trees: `<machine>/tags/<session>.json`.
// It rides the same disjoint-machine-dir invariant as transcripts — a machine
// writes only its OWN `<machine>/tags/`, `stageMachineDir` stages only
// `-- <machine>`, and foreign machine dirs are never touched or pruned
// (push.go:syncTrees) — so a losing set's file survives on every machine that
// holds it. Tags are NON-derivable agent labor, so unlike embeddings they
// must live in the archive, not a local-only cache.
const tagsDirName = "tags"

// TagFile is one session's tagging, serialized as `<machine>/tags/<session>.json`.
// It is the whole authored unit for that session on the writing machine: the
// segment set plus the optional verdict, stamped with the origin machine id the
// cross-machine ingest resolves on. One file per session — a re-tag overwrites it
// (dedup by path), and it diffs cleanly in git.
type TagFile struct {
	SessionID     string       `json:"session_id"`
	OriginMachine string       `json:"origin_machine"`
	Segments      []TagSegment `json:"segments,omitempty"`
	Verdict       *TagVerdict  `json:"verdict,omitempty"`
}

// TagSegment is one topic segment in a TagFile.
type TagSegment struct {
	StartUUID string  `json:"start_uuid"`
	EndUUID   string  `json:"end_uuid,omitempty"`
	Topic     string  `json:"topic"`
	Summary   string  `json:"summary,omitempty"`
	TaggedAt  float64 `json:"tagged_at,omitempty"`
}

// TagVerdict is a session's verdict in a TagFile (e.g. routine + floor|agent).
type TagVerdict struct {
	Verdict  string  `json:"verdict"`
	Source   string  `json:"source"`
	TaggedAt float64 `json:"tagged_at,omitempty"`
}

// TagExporter returns THIS machine's locally-authored tag files. It is injected
// from the cli seam (SetTagExporter) because collecting local tags means
// enumerating local scopes + reading the store — deps the archive package
// deliberately does not carry (internal/scopes imports archive; the reverse would
// cycle). nil = the archive was built without the tag feature wired, so push
// simply skips tag export (transcripts still sync).
type TagExporter func() ([]TagFile, error)

// SetTagExporter wires the local-tag source used by PushLocal. Returns the
// Archive for call chaining at the cli seam.
func (a *Archive) SetTagExporter(fn TagExporter) *Archive {
	a.exportTags = fn
	return a
}

// tagsDir is this machine's tags subtree in the clone.
func (a *Archive) tagsDir() string {
	return filepath.Join(a.machineDir(), tagsDirName)
}

// exportOwnTags writes this machine's tag files into `<machine>/tags/`, one JSON
// per tagged session, via temp-file + rename so a kill mid-write can never leave
// a torn JSON where `git add` could stage it (the same atomicity transcript
// copies use). A nil exporter is a no-op. Returns how many files were written.
//
// Own-dir only: it writes exclusively under this machine's dir, so it can never
// touch — let alone prune — a foreign machine's tag files. Re-tagging a
// session overwrites its file in place; a session that lost all its tags leaves a
// stale file (rare; not pruned here — pruning own tag files is deferred).
func (a *Archive) exportOwnTags() (int, error) {
	if a.exportTags == nil {
		return 0, nil
	}
	files, err := a.exportTags()
	if err != nil {
		return 0, fmt.Errorf("collect local tags: %w", err)
	}
	if len(files) == 0 {
		return 0, nil
	}
	destDir := a.tagsDir()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, fmt.Errorf("create tags dir: %w", err)
	}
	// Stage temps OUTSIDE the tracked tree, under .git (same filesystem as the
	// clone, so the rename stays atomic) — a kill mid-write can then never leave a
	// `.tag-*` orphan where `git add -A -- <machine>` would commit it. This mirrors
	// syncTrees' `.git/rawclaw-tmp` staging for transcript copies (push.go). Not
	// cleared here: syncTrees already cleared it this push, and any leftover under
	// .git is untracked and harmless.
	tmpDir := filepath.Join(a.clone, ".git", "rawclaw-tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return 0, fmt.Errorf("create tags temp dir: %w", err)
	}
	written := 0
	for _, f := range files {
		if f.SessionID == "" {
			continue // a tag file must be keyed by a session
		}
		// Stamp our own id as the authoring origin — a locally-collected tag is
		// this machine's by construction (the store leaves origin NULL locally).
		f.OriginMachine = a.machineID
		if err := writeTagFileAtomic(tmpDir, destDir, f); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// writeTagFileAtomic marshals f, writes it to a temp file in tmpDir (which MUST
// be outside the tracked tree — e.g. under .git — so a torn temp is never
// stageable), then renames it to `<destDir>/<session>.json`. tmpDir and destDir
// must share a filesystem for the rename to be atomic. A session id may contain a
// path separator (subagent ids are "<parent>/<child>", provenance.go), so the
// destination's parent dir is created before the rename — otherwise the rename
// would fail into a missing subdir (B2).
func writeTagFileAtomic(tmpDir, destDir string, f TagFile) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tag file %s: %w", f.SessionID, err)
	}
	tmp, err := os.CreateTemp(tmpDir, ".tag-*")
	if err != nil {
		return fmt.Errorf("create tag temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write tag temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close tag temp: %w", err)
	}
	dst := filepath.Join(destDir, f.SessionID+".json")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("create tag dest dir for %s: %w", f.SessionID, err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("place tag file %s: %w", dst, err)
	}
	return nil
}
