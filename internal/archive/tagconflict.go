package archive

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// tagConflictPath is <state-dir>/archive/tag-conflicts — the recorded set of
// sessions whose cross-machine tags did not agree (≥2 distinct real segment
// sets). Each ingest pass rewrites it with the current conflict set; `archive
// status` reads it. One session id per line.
func tagConflictPath() string {
	return filepath.Join(store.CacheDir(), "archive", "tag-conflicts")
}

// writeTagConflicts records the conflicted session ids (sorted, unique) for
// `archive status`. Best-effort + atomic (temp+rename): a failed write only
// under-reports conflicts in status, never corrupts. An empty set truncates the
// file so a resolved conflict stops being reported.
func writeTagConflicts(sids []string) {
	dir := filepath.Join(store.CacheDir(), "archive")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	uniq := map[string]struct{}{}
	for _, s := range sids {
		uniq[s] = struct{}{}
	}
	sorted := make([]string, 0, len(uniq))
	for s := range uniq {
		sorted = append(sorted, s)
	}
	sort.Strings(sorted)
	body := strings.Join(sorted, "\n")
	if body != "" {
		body += "\n"
	}
	tmp, err := os.CreateTemp(dir, ".tag-conflicts-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, tagConflictPath())
}

// readTagConflicts returns the recorded conflicted session ids (empty when none
// or unreadable). Offline read for `archive status`.
func readTagConflicts() []string {
	b, err := os.ReadFile(tagConflictPath())
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// tagIngestStampPath is the mtime gate for the tag-ingest pass: it advances after
// every pass, and the pass is skipped when no machine's tags dir is newer (cheap
// steady-state — a handful of stats instead of walking every session on every
// search).
func tagIngestStampPath() string {
	return filepath.Join(store.CacheDir(), "archive", "last-tag-ingest")
}

// tagIngestDue reports whether the tag-ingest pass should run. It gates on the
// PULL stamp, not the tags-dir mtime: a `git pull` that updates a foreign tag
// file IN PLACE (rewriting existing `<sid>.json` bytes) need not bump the parent
// directory's mtime, so a dir-mtime gate would silently miss a changed foreign
// tagging and a newly-arrived conflict. A pull is exactly when foreign tags can
// change, and stampPull advances on every successful pull — so "pull newer than
// the last ingest" is the correct, O(1) trigger. Never ingested → run; never
// pulled → nothing foreign to ingest. (Own re-tags don't need this pass: local
// authoring writes straight to the local db; this pass only feeds foreign scope
// dbs.)
func tagIngestDue() bool {
	ingest, err := os.Stat(tagIngestStampPath())
	if err != nil {
		return true // never ingested (or unreadable stamp): run once
	}
	pull, err := os.Stat(pullStampPath())
	if err != nil {
		return false // never pulled: no foreign tag change since the last ingest
	}
	return pull.ModTime().After(ingest.ModTime())
}

// stampTagIngest records a completed tag-ingest pass (mtime = now).
func stampTagIngest() {
	p := tagIngestStampPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(p, nil, 0o644)
	_ = os.Chtimes(p, time.Time{}, time.Now())
}

// allMachineDirs lists every machine dir name in the clone — this machine plus
// every foreign one — the set gatherTagFiles unions a session's tags across.
func (a *Archive) allMachineDirs() []string {
	out := []string{a.cfg.Name}
	for _, m := range a.foreignMachines() {
		out = append(out, m.Name)
	}
	return out
}
