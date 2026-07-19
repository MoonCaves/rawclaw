package archive

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// staleAfter is the possibly-out-of-date window for a foreign machine dir: a
// dir whose last archive commit is older than this is enumerated with Stale
// set, so the search layer reports its results MAY miss recent activity
// (the existing stale-fallback posture) while still serving them. An idle
// machine with nothing new to push looks identical to an off one from here
// (inherent to any commit-age window) — which is why `archive status` never
// renders this as a per-machine verdict, only as "last new content"; the
// same window instead bounds the own-sync overdue flags there, the one
// freshness fact this machine knows first-hand.
const staleAfter = 24 * time.Hour

// Scopes enumerates the clone's FOREIGN machine dirs as ready-to-search scopes:
// each foreign machine's Claude project dirs and Codex cwd-groups, ingested
// through the existing index paths into their own namespaced cache dbs, with
// origin_machine stamped from the dir's manifest. Our OWN dir is excluded — the
// live local tree is fresher and already indexed; that exclusion is what makes
// cross-machine dedup a non-event. A missing clone yields nil (enumeration
// never touches the network; `archive pull` is the refresh path). reindex
// forces a full rebuild of the scope dbs, mirroring the local scopes. ctx
// bounds the per-machine staleness git probes (dirStale), so they die with
// the caller's watchdog like every other git child.
func (a *Archive) Scopes(ctx context.Context, reindex bool) []view.Scope {
	if _, err := os.Stat(filepath.Join(a.clone, ".git")); err != nil {
		return nil // no clone yet: local-only until `archive pull`
	}
	now := time.Now()
	var out []view.Scope
	for _, m := range a.foreignMachines() {
		stale := a.dirStale(ctx, m.Name, now)
		out = append(out, a.claudeScopes(m, stale, reindex, true)...)
		out = append(out, a.codexScopes(m, stale, reindex, true)...)
	}
	return out
}

// LookupScopes enumerates the same foreign scopes as Scopes WITHOUT ingesting
// anything: each scope's DBP names the cache db a previous search-time ingest
// would have built; a scope never ingested simply fails to open read-only at
// the caller. This is the cheap path for point lookups (e.g. resolving a
// --resume prefix) where walking and indexing every foreign tree would be far
// too heavy. No staleness git probe runs either (Stale stays false — lookups
// don't report freshness), which keeps this path free of child processes, so
// it needs no watchdog ctx.
func (a *Archive) LookupScopes() []view.Scope {
	if _, err := os.Stat(filepath.Join(a.clone, ".git")); err != nil {
		return nil // no clone yet: local-only until `archive pull`
	}
	var out []view.Scope
	for _, m := range a.foreignMachines() {
		out = append(out, a.claudeScopes(m, false, false, false)...)
		out = append(out, a.codexScopes(m, false, false, false)...)
	}
	return out
}

// foreignMachines lists the clone's top-level dirs that are OTHER machines'
// registered dirs: hidden dirs and our own dir (by name) are skipped, a dir
// without a readable manifest is not a machine dir (warned, skipped), a
// manifest with no machine id is malformed (warned, skipped — an empty id
// would fall through to the local stamp and corrupt provenance), and a
// manifest claiming OUR machine id is this machine's data under an old name —
// skipped, or every pre-rename session would come back as a duplicate foreign
// hit. The same rename can happen to a FOREIGN machine (the archive never
// prunes, so its old dir lingers with the identical machine id and identical
// session files): dirs are deduped by machine id, newest manifest wins —
// otherwise every pre-rename session would list twice and its unchanged
// session id could never be disambiguated by any prefix. The returned
// manifests carry the DIR name as Name (the layout truth; a manifest's
// recorded name could lag a rename).
func (a *Archive) foreignMachines() []manifest {
	entries, err := os.ReadDir(a.clone)
	if err != nil {
		slog.Warn("archive: cannot list clone dirs; searching local scopes only",
			"clone", a.clone, "err", err)
		return nil
	}
	byID := map[string]manifest{} // machine id → winning manifest
	var order []string            // ids in first-seen (alphabetical-dir) order
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
		if m.MachineID == "" {
			slog.Warn("archive: skipping machine dir with an empty machine_id", "dir", e.Name())
			continue
		}
		if m.MachineID == a.machineID {
			continue // our own data under a previous name — the live tree wins
		}
		m.Name = e.Name()
		cur, seen := byID[m.MachineID]
		if !seen {
			byID[m.MachineID] = m
			order = append(order, m.MachineID)
			continue
		}
		// Duplicate claim: keep the newer registration (UpdatedAt is RFC3339
		// UTC, so lexical compare orders correctly); ties keep the first dir
		// (ReadDir is sorted), so the choice is deterministic either way.
		if m.UpdatedAt > cur.UpdatedAt {
			byID[m.MachineID] = m
		}
	}
	out := make([]manifest, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

// dirStale reports whether name's history in the clone is older than
// staleAfter — the same probe and window `archive status` reports, so search's
// stale-fallback and status can never disagree on what "stale" means.
func (a *Archive) dirStale(ctx context.Context, name string, now time.Time) bool {
	return staleAt(a.dirLastCommit(ctx, name), now)
}

// claudeScopes enumerates one foreign machine's Claude project dirs
// (<machine>/claude/<project-dir> holding top-level *.jsonl — the same shape
// paths.AllProjectDirs requires locally) and ingests each into its namespaced
// db with the machine's identity stamped as origin. Listing uses os.ReadDir,
// not Glob: the path segments come from another machine's push, and a glob
// metachar in a dir name must not silently vanish that machine's scopes. An
// ingest failure keeps the scope: search opens the db read-only and reports
// the degradation itself. ingest=false (LookupScopes) enumerates without
// touching the dbs at all.
func (a *Archive) claudeScopes(m manifest, stale, reindex, ingest bool) []view.Scope {
	root := filepath.Join(a.clone, m.Name, "claude")
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("archive: cannot list foreign claude tree", "machine", m.Name, "err", err)
		}
		return nil // absent tree: the machine pushed no Claude transcripts
	}

	var out []view.Scope
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		d := filepath.Join(root, e.Name())
		if !hasTopLevelJSONL(d) {
			continue
		}
		dbp := archiveScopeDBPath(m.Name, "claude", e.Name())
		if ingest {
			if _, _, err := index.EnsureIndexedTree(dbp, d, reindex, m.MachineID); err != nil {
				slog.Warn("archive: foreign claude scope index failed",
					"machine", m.Name, "dir", d, "err", err)
			}
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
// per-cwd-group shape the local Codex enumeration builds. ingest=false
// (LookupScopes) still discovers the cwd groups (that read is layout-only)
// but never writes their dbs.
func (a *Archive) codexScopes(m manifest, stale, reindex, ingest bool) []view.Scope {
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
		if ingest {
			if _, _, ierr := index.EnsureIndexedContainers(
				dbp, reindex, byCWD[cwd], ad.Messages, codex.Registration().ID, m.MachineID,
			); ierr != nil {
				slog.Warn("archive: foreign codex scope index failed",
					"machine", m.Name, "cwd", cwd, "err", ierr)
			}
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

// hasTopLevelJSONL reports whether dir directly holds at least one .jsonl —
// the ReadDir counterpart of the local scopes' top-level glob check.
func hasTopLevelJSONL(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			return true
		}
	}
	return false
}

// codexGroupLabel is the friendly label for a Codex cwd group (basename of the
// recorded cwd, "codex" when unknown) — the same label the local Codex scopes
// wear (scopes.codexLabel; duplicated here because scopes imports archive, so
// the reverse import would cycle), prefixed by the machine name at the call
// site.
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
	name := index.ArchiveDBPrefix + machine + "-" + sourceID + "-" + sanitizeDBSegment(key) +
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
