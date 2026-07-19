package archive

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// staleAfter is the staleness window for a foreign machine dir: a dir whose
// last archive commit is older than this is enumerated with Stale set, so the
// search layer reports it through the existing stale-fallback posture while
// still serving its results. Machines sync at least hourly when the timer is
// on, so a day of silence means genuinely off/asleep — though an idle machine
// with nothing new to push looks identical from here (inherent to any window).
const staleAfter = 24 * time.Hour

// Scopes enumerates the clone's FOREIGN machine dirs as ready-to-search scopes:
// each foreign machine's Claude project dirs and Codex cwd-groups, ingested
// through the existing index paths into their own namespaced cache dbs, with
// origin_machine stamped from the dir's manifest. Our OWN dir is excluded — the
// live local tree is fresher and already indexed; that exclusion is what makes
// cross-machine dedup a non-event. A missing clone yields nil (enumeration
// never touches the network; `archive pull` is the refresh path). reindex
// forces a full rebuild of the scope dbs, mirroring the local scopes.
func (a *Archive) Scopes(reindex bool) []view.Scope {
	if _, err := os.Stat(filepath.Join(a.clone, ".git")); err != nil {
		return nil // no clone yet: local-only until `archive pull`
	}
	now := time.Now()
	var out []view.Scope
	for _, m := range a.foreignMachines() {
		stale := a.dirStale(m.Name, now)
		out = append(out, a.claudeScopes(m, stale, reindex)...)
		out = append(out, a.codexScopes(m, stale, reindex)...)
	}
	return out
}

// foreignMachines lists the clone's top-level dirs that are OTHER machines'
// registered dirs: hidden dirs and our own dir (by name) are skipped, a dir
// without a readable manifest is not a machine dir (warned, skipped), and a
// manifest claiming OUR machine id is this machine's data under an old name —
// skipped, or every pre-rename session would come back as a duplicate foreign
// hit. The returned manifests carry the DIR name as Name (the layout truth;
// a manifest's recorded name could lag a rename).
func (a *Archive) foreignMachines() []manifest {
	entries, err := os.ReadDir(a.clone)
	if err != nil {
		return nil
	}
	var out []manifest
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == a.cfg.Name {
			continue
		}
		m, merr := readManifest(filepath.Join(a.clone, e.Name()))
		if merr != nil {
			slog.Warn("archive: skipping dir without a readable machine manifest",
				"dir", e.Name(), "err", merr)
			continue
		}
		if m.MachineID == a.machineID {
			continue // our own data under a previous name — the live tree wins
		}
		m.Name = e.Name()
		out = append(out, m)
	}
	return out
}

// dirStale reports whether name's history in the clone is older than
// staleAfter, judged by its last commit time — the local clone's knowledge of
// that machine IS what search serves, so an un-pulled clone and a silent
// machine both (correctly) read as stale. An unreadable probe counts as stale:
// unknown freshness is reported, never silently passed off as fresh.
func (a *Archive) dirStale(name string, now time.Time) bool {
	out, err := a.run(context.Background(), a.clone, "log", "-1", "--format=%ct", "--", name)
	if err != nil {
		return true
	}
	ct, perr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if perr != nil {
		return true
	}
	return now.Sub(time.Unix(ct, 0)) > staleAfter
}

// claudeScopes enumerates one foreign machine's Claude project dirs
// (<machine>/claude/<project-dir> holding top-level *.jsonl — the same shape
// paths.AllProjectDirs requires locally) and ingests each into its namespaced
// db with the machine's identity stamped as origin. An ingest failure keeps the
// scope: search opens the db read-only and reports the degradation itself.
func (a *Archive) claudeScopes(m manifest, stale, reindex bool) []view.Scope {
	root := filepath.Join(a.clone, m.Name, "claude")
	entries, _ := filepath.Glob(filepath.Join(root, "*"))
	sort.Strings(entries)

	var out []view.Scope
	for _, d := range entries {
		if !isDir(d) {
			continue
		}
		if hits, _ := filepath.Glob(filepath.Join(d, "*.jsonl")); len(hits) == 0 {
			continue
		}
		dbp := archiveScopeDBPath(m.Name, "claude", filepath.Base(d))
		if _, _, err := index.EnsureIndexedTree(dbp, d, reindex, m.MachineID); err != nil {
			slog.Warn("archive: foreign claude scope index failed",
				"machine", m.Name, "dir", d, "err", err)
		}
		out = append(out, view.Scope{
			Project:    m.Name + "/" + paths.ProjectLabel(d),
			DBP:        dbp,
			CWD:        paths.ProjectCWD(d),
			Source:     "claude",
			Origin:     m.MachineID,
			OriginName: m.Name,
			Stale:      stale,
		})
	}
	return out
}

// codexScopes enumerates one foreign machine's Codex rollouts
// (<machine>/codex/...), groups them by recorded cwd, and ingests each group
// into its own namespaced db with the machine's identity as origin — the same
// per-cwd-group shape the local Codex enumeration builds.
func (a *Archive) codexScopes(m manifest, stale, reindex bool) []view.Scope {
	root := filepath.Join(a.clone, m.Name, "codex")
	if !isDir(root) {
		return nil
	}
	ad := codex.New()
	containers, err := ad.DiscoverRoot(root)
	if err != nil {
		slog.Warn("archive: foreign codex discover failed", "machine", m.Name, "err", err)
		return nil
	}

	byCWD := map[string][]source.Container{}
	for _, c := range containers {
		byCWD[c.CWD] = append(byCWD[c.CWD], c)
	}
	cwds := make([]string, 0, len(byCWD))
	for k := range byCWD {
		cwds = append(cwds, k)
	}
	sort.Strings(cwds)

	out := make([]view.Scope, 0, len(cwds))
	for _, cwd := range cwds {
		dbp := archiveScopeDBPath(m.Name, "codex", cwd)
		if _, _, ierr := index.EnsureIndexedContainers(
			dbp, reindex, byCWD[cwd], ad.Messages, codex.Registration().ID, m.MachineID,
		); ierr != nil {
			slog.Warn("archive: foreign codex scope index failed",
				"machine", m.Name, "cwd", cwd, "err", ierr)
		}
		out = append(out, view.Scope{
			Project:    m.Name + "/" + codexGroupLabel(cwd),
			DBP:        dbp,
			CWD:        cwd,
			Source:     "codex",
			Origin:     m.MachineID,
			OriginName: m.Name,
			Stale:      stale,
		})
	}
	return out
}

// codexGroupLabel is the friendly label for a Codex cwd group (basename of the
// recorded cwd, "codex" when unknown) — the same label the local Codex scopes
// wear, prefixed by the machine name at the call site.
func codexGroupLabel(cwd string) string {
	if cwd == "" {
		return "codex"
	}
	return filepath.Base(strings.TrimRight(cwd, "/"))
}

// archiveScopeDBPath namespaces one archive scope's cache db:
// "archive-<machine>-<source>-<readable>-<hash8>.db" in the shared cache dir.
// The "archive-" prefix keeps these out of the local orphan-db discovery scans;
// the trailing hash of the full (machine, source, key) triple keeps the name
// injective even though the readable segment is lossy and capped.
func archiveScopeDBPath(machine, sourceID, key string) string {
	sum := sha1.Sum([]byte(machine + "\x00" + sourceID + "\x00" + key))
	name := "archive-" + machine + "-" + sourceID + "-" + sanitizeDBSegment(key) +
		"-" + hex.EncodeToString(sum[:])[:8]
	return index.DBPath(name)
}

// dbSegmentCap bounds the readable segment of an archive db name so a deep
// foreign cwd can't push the filename past filesystem limits — identity comes
// from the hash, the segment is only for humans browsing the cache dir.
const dbSegmentCap = 80

// sanitizeDBSegment folds a layout key (project dir base, recorded cwd) into a
// safe filename segment: [a-zA-Z0-9_-] kept, everything else folded to "-",
// capped, "unknown" when nothing survives.
func sanitizeDBSegment(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > dbSegmentCap {
		out = out[:dbSegmentCap]
	}
	if out == "" {
		return "unknown"
	}
	return out
}
