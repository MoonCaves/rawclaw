# Container Test Deletion Audit

Audit target: `d7106e9bd0cb6b4f98e5e8bfdedd82dde8dd9bd9` (`refactor(index): delete helper-coupled test and record hostile rulings in findings`). The commit deletes 99 lines from `internal/index/containers_test.go` and changes no production code. Audit branch: `norm/ozzy-spy`.

## Verdict: SAFE TO ADOPT, with one explicit lost edge contract

The deletion is safe as a production-preserving test cleanup. The removed test directly exercised the private `containerMeta` helper for three matrices; it did not exercise a user-visible command, index result, rebuild, or retention decision. Existing end-to-end tests still pin the important behavior. The one behavior no longer directly pinned is that calling `containerMeta` with a missing backing file returns zero `SourceMTime`, `SourceSize`, and `SourceFP`. Current production flow skips missing backing files in `updateContainers` at `internal/index/containers.go:325-331`, so that matrix is presently unreachable through `EnsureIndexedContainers`.

Ponytail ruling: `delete` the helper-coupled matrix; retain a small test only if a future caller deliberately allows `containerMeta` to process missing sources. No production replacement is needed.

## Deleted assertion trace

| Deleted assertion / case | Surviving contract evidence | Result |
|---|---|---|
| `m.ID`, `m.Source`, `m.Project`, `m.CWD` (`containers_test.go` deleted lines 884-895) | `TestRebuildRestoresContainerSession` at `internal/index/durable_test.go:477-491` verifies restored `source_tool=codexish` and `cwd=/w/billing`; production maps `sourceID`/scope in `internal/index/containers.go:469-475`. | Redundant for end-to-end behavior; helper wiring itself is intentionally unpinned. |
| `IsSubagent`, `ParentID` (`containers_test.go` deleted lines 896-900) | `TestEnsureIndexedContainers` at `internal/index/containers_test.go:69-81` queries persisted `is_subagent` and `parent_id`; independent Claude path coverage is `internal/index/index_test.go:591-609`. | Fully pinned by observable rows. |
| Existing-file `SourceSize` and non-empty `SourceFP` (`containers_test.go` deleted lines 902-907) | `TestIndexVaultsTranscriptVerbatim` at `internal/index/durable_test.go:47-64` checks sidecar source path and fingerprint; `TestEnsureIndexedContainers_SQLiteWALTrigger` at `internal/index/containers_test.go:182-316` covers changing backing state and reindex/no-op transitions; production computes size/fingerprint at `internal/index/containers.go:161-181`. | Size/fingerprint behavior is covered through vault and watermark flows, though not by a direct helper assertion. |
| Missing-file case expects zero source stats (`containers_test.go` deleted lines 864-877) | `updateContainers` calls `backingFileState` and `continue`s on stat error at `internal/index/containers.go:325-331`; no surviving test calls `containerMeta` directly with a missing path. | **Lost narrow edge contract; currently unreachable through the production container path.** |
| `rawPath == backingFilePath(c.Path)` (`containers_test.go` deleted lines 908-910) | `TestRebuildRestoresContainerSession` deletes the source and successfully rebuilds the container vault at `internal/index/durable_test.go:463-490`; `TestRebuiltStoreSurvivesTheNextLivePass` checks original source-path indexing at `internal/index/durable_test.go:350-352`. | Covered by round-trip behavior and source-path assertions. |

## Required contract audit

- **Bounded cleanup: PINNED.** `TestEnsureFreshContainer_PruneStaleLeftovers` remains at `internal/index/containers_test.go:594-708`, covering stale cleanup, fresh leftovers, mixed-age WAL groups, and failed-sync retention. The deleted test never touched this path.
- **Rebuild-failure preservation: PINNED.** `TestRebuildFailurePreservesExistingStore` remains at `internal/index/rebuild_test.go:13-45`; `TestConsolidateFrom_RebuildFailureLeavesLiveStoreIntact` remains at `internal/index/consolidated_test.go:2042-2092`; `TestConsolidate_RebuildPreservesStoreOnlyTags` remains at `internal/index/consolidated_test.go:465-494`. These assert the live store survives injected pre-swap/fold failure.
- **Live-generation safety: PINNED.** `TestEnsureIndexedContainers_SQLiteWALTrigger` remains at `internal/index/containers_test.go:182-316` and checks real WAL creation, WAL-triggered reindex, checkpoint transition, and unchanged-state no-op. `TestEnsureFreshContainer_IncrementalEndToEnd` remains at `internal/index/incremental_test.go:461-518`; tail seam coverage remains in `internal/index/tail_edge_test.go:26-940`.
- **No-op/retry contracts: PINNED.** `TestAppendContainer_StaleWatermarkIsNoOp` remains at `internal/index/incremental_test.go:413-459`; `TestTailEdge_ReingestIdempotency` remains at `internal/index/tail_edge_test.go:520-660`; `TestConsolidate_RetryAfterAbruptPostMergeExit` remains at `internal/index/consolidated_test.go:1824-1912`.

## Narrow observed gate

Command: `env CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'Test(EnsureIndexedContainers|RebuildRestoresContainerSession|IndexVaultsTranscriptVerbatim|RebuildFailurePreservesExistingStore|RestoreSession_RollbackOnFailure|EnsureFreshContainer_IncrementalEndToEnd)$'`

Result: **PASS**, `ok github.com/MoonCaves/rawclaw/internal/index 3.536s` (wall observed by the command runner: `3.536s`). This is a focused race gate, not a repository-wide green claim. No production files were changed; `gofmt -w` is N/A. `git diff --check` for this report is required before commit.

## Net lines and adoption

Target commit: production `0`; tests `-99` deleted lines; docs `+15/-6` in `FINDINGS.md` (net `+9`). The test deletion itself is net `-99` lines. **Adopt:** yes, because all four requested operational contracts remain pinned and the only lost assertion targets an unreachable private-helper fallback. **Caveat:** if `containerMeta` gains a caller that tolerates missing sources, restore a focused missing-file assertion rather than the deleted 99-line matrix.
