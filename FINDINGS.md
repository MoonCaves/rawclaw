# Raid Containers: Hostile Review Findings

Scope: `internal/index/containers.go`, `internal/index/containers_test.go`, and caller integration in `internal/cli/tagrefresh.go` and `internal/cli/cmd_ingest.go`.

Base SHA: `479d14c` (`lenny/raid-containers-20260826`)

---

## Executive Summary

All proposed rival changes touching the container lifecycle and indexing paths have been audited against today's base. The current branch `lenny/raid-containers-20260826` already integrates the correct container isolation fixes (`85cf480`, `7f8e115`, `ba60ca8`). Proposed excess abstractions (e.g. `RefreshFreshContainer`/`FoldFreshContainer` in `f3d03b0`) are rejected under Ponytail ladder simplicity rules.

---

## Candidate Audits & Rulings

### C1 — Defer Refresh Pruning to Ingest Path (`85cf480` / `cc6b4cd`)
- **Exact Rival SHA**: `85cf4804257b61e516df3a60b1dac6982e908466` (and `cc6b4cd82911104356d61d05bbc930bf35a10255`)
- **File / Lines**: `internal/index/containers.go:91-135`, `internal/index/containers_test.go:723-745`
- **Evidence**: `PrepareFreshContainer` is used on the fast closeout path (`tagrefresh.go`). Executing `pruneStaleRefreshDBs` in `PrepareFreshContainer` conducted un-fenced `os.ReadDir` sweeps on every tag-prep invocation, causing contention and potential deletion of concurrently active staging databases. Moving pruning to `EnsureFreshContainer` confines directory sweeps to the ingest/publish path.
- **Verification**: Covered by `TestPrepareFreshContainer_ProvesFreshnessWithoutConsolidatedSync`.
- **RULING**: `CLEAN` (already incorporated into current base `479d14c`).

---

### C2 — Container Store Freshness Watermark Stamping (`7f8e115` / `4b4d94d`)
- **Exact Rival SHA**: `7f8e115b4497228e173d1102f479ceef9073759c` (and `4b4d94d4b04b596917ca24f62f28ff4d6c9b5137`)
- **File / Lines**: `internal/index/containers.go:277-279`, `internal/index/containers_test.go:771-817`
- **Evidence**: `ensureIndexedContainers` was missing `StampIngestWatermark(con)`, causing non-Claude container stores (Codex, Antigravity, Goose) to permanently report `Reason == no_ingest_watermark` in `CheckIndexFreshness`. Stamping watermarks during container tree indexing allows honest freshness detection across all adapters.
- **Verification**: Covered by `TestEnsureIndexedContainers_StampsFreshnessWatermark`.
- **RULING**: `CLEAN` (already incorporated into current base `479d14c`).

---

### C3 — Luna Dump-Before-Fold Container Wrappers (`f3d03b0`)
- **Exact Rival SHA**: `f3d03b0bf1a3908d9d97421e5ab95233cdf34d9b`
- **File / Lines**: `internal/index/containers.go:74-139`
- **Evidence**: Proposes splitting `EnsureFreshContainer` into `RefreshFreshContainer` + `FoldFreshContainer` and exporting `IsSQLiteBusy`.
- **Ponytail Judgment**: YAGNI / redundant abstraction. `PrepareFreshContainer` already executes the non-folding refresh and verification step. `tagrefresh.go` in the merged architecture calls `PrepareFreshContainer` directly to dump the tag chunk, followed by standard `SyncConsolidatedFrom(dbp)`. Extra wrapper functions and exported helpers add boilerplate without changing semantics.
- **RULING**: `REJECT` (superseded by standard `PrepareFreshContainer` flow).

---

### C4 — Sidecar Grouping in Refresh Prune (`ba60ca8`)
- **Exact Rival SHA**: `ba60ca8aaad26c15bcaf3ef30874f5d9a88de2ab`
- **File / Lines**: `internal/index/containers.go:44-78`, `internal/index/containers_test.go:680-705`
- **Evidence**: `pruneStaleRefreshDBs` groups `.db` files with companion `-wal` and `-shm` sidecars, evaluating age by the maximum mtime of the group. This prevents unlinking an older database file while a transaction is actively writing to its fresh WAL.
- **Verification**: Covered by `TestEnsureFreshContainer_PruneStaleLeftovers` (mixed-age case).
- **RULING**: `CLEAN` (already incorporated into current base `479d14c`).

---

### C5 — `dupl` Clone Finder: Incremental Tail Loop Body
- **Exact Location**: `internal/index/containers.go:350-363` vs `internal/index/index.go:951-964`
- **Evidence**: 14 lines of loop-body dispatch calling `parseTailMessages`, `checkPrefixFingerprint`, `appendContainer`.
- **Ponytail Judgment**: The shared logic (`parseTailMessages` and `appendContainer`) is already extracted into `internal/index/tail.go`. The 10-line calling sequence in the respective loops operates over distinct iteration structures (`source.Container` vs raw filesystem paths). Adding an extra layer of abstraction for 10 lines of loop control violates Ponytail rule 1 (no unrequested abstractions).
- **RULING**: `REJECT` (reuse in `tail.go` is sufficient; no change).

---

### C6 — `golangci-lint` Static Analysis Finder
- **Tool**: `/Users/jay-m4/go/bin/golangci-lint run ./internal/index/... ./internal/scopes/... ./internal/lifecycle/...`
- **Result**: `0 issues`.
- **RULING**: `CLEAN`.

---

## Verification Plan

1. Ensure tests in `internal/index` pass cleanly with race detection:
   `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...`
2. Ensure formatting:
   `gofmt -l internal/index/`
