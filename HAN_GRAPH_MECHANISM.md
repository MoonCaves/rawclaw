# Han Graphify mechanism report

## Scope and evidence boundary

This is a Graphify-only report against project path `/Users/jay-m4/code/rawclaw`.
The first orientation call was `graph_stats`, followed by repeated MCP calls with
that project path. Graphify reported 3,501 nodes, 10,364 edges, 194 communities,
64% EXTRACTED, 36% INFERRED, and 0% AMBIGUOUS. `graphify reflect --if-stale`
found no prior lessons. No source text, patch body, or candidate-branch graph was
read. Git was used only for metadata after the graph framed these questions.

The graph is current-tree/base evidence. It does not index unmerged candidate
commits. A query containing `61b7957` returned benchmark and tag-related nodes but
no commit node or candidate-only mechanism. Therefore no candidate behavior below
is attributed to Graphify.

## Query record

The exact-vocabulary orientation query was:

`catalog hook setup session ingest path`

It selected starts `Setup()`, `Session`, `catalog_hook_test.go`,
`hookresolve_test.go`, `ingestContainerWithRetry()`, and `paths.go`. The BFS
returned 102 nodes. The call-filtered DFS returned 65 nodes and was useful for
the consolidation call chain. The follow-up exact vocabulary query was:

`tag write vector topup autosync detach fence receipt read after write`

It returned 221 nodes; its starts included `detach()`, `Read()`, `write()`,
`autosyncTokenPath()`, `AcquireConsolidatedFence()`, `topup.go`, and `vectors.go`.
The `spawnIngestChild` query returned only five nodes.

## Current-base mechanism map

### Catalog and hook path

- `renderHookScript()` is `internal/cli/setup.go:L343`, community `NewDB`, degree
  16. Its extracted call is `renderHookScript() -> rawclawResolveHead()`.
- `ReadCatalogEntry()` is `internal/paths/paths.go:L91`, community `paths.go`,
  degree 9. It has an extracted reference to `CatalogEntry()` and an extracted
  caller `resolveSessionCatalog()`.
- `TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath()` is
  `internal/cli/catalog_hook_test.go:L19`, community `paths.go`, degree 6. Its
  exact neighbors are `readDetachedIngestCalls()` [calls, EXTRACTED],
  `renderHookScript()` [calls, INFERRED], and `ReadCatalogEntry()` [calls,
  INFERRED].
- `renderHookScript()` also has inferred callers for the Claude/Codex catalog
  tests and detached-ingest tests, plus extracted callers
  `installRawclawHookAt()` and `renderAntigravityPrimeScript()`.

The strongest exact two-hop path is:

`renderHookScript()` <-[calls, INFERRED]-
`TestCodexPrimeScript_CreatesSessionCatalogEntry_FullPayload()`
 -[calls, INFERRED]-> `ReadCatalogEntry()`.

The strongest catalog-to-ingest path returned was five hops:

`renderHookScript()` <-[calls, INFERRED]-
`TestCodexPrimeScript_CreatesPartialCatalogEntry()`
 -[calls, INFERRED]-> `ReadCatalogEntry()`
 <-[calls, INFERRED]- `catalogIngestSource()`
 -[references, EXTRACTED]-> `tagSourceMatch`
 <-[references, EXTRACTED]- `ingestContainerWithRetry()`.

These edges show the tested relationship between hook rendering, catalog lookup,
and ingest resolution. They do not prove a production hook's exact shell body or
its process-lifetime behavior.

### Tag write, fold, and fence

- `runTagWriteCmd()` is `internal/cli/cmd_tag.go:L475`, community
  `BenchmarkConnectionPragmas`, degree 19. Exact neighbors include
  `runTagWrite()` [calls, EXTRACTED], `runTagWriteRoutine()` [calls, EXTRACTED],
  `ConsolidatedPath()` [calls, INFERRED], `AcquireConsolidatedFence()` [calls,
  INFERRED], `SyncConsolidatedFrom()` [calls, INFERRED], and `ConnectRW()`
  [calls, INFERRED].
- `SyncConsolidatedFrom()` is `internal/index/consolidated.go:L553`, community
  `setup_test.go`, degree 34. It has extracted calls to
  `ConsolidatedPath()`, `IsConsolidatedDB()`, `beginConsolidatePhase()`,
  `consolidateOne()`, `healUpgradedConsolidatedStore()`,
  `migrateSessionSources()`, `pruneTombstoned()`, and
  `StampIngestWatermark()`. Its inferred call to `AcquireConsolidatedFence()`
  is also present.
- `AcquireConsolidatedFence()` is `internal/index/consolidated_fence.go:L35`,
  community `locateSession`, degree 13. Its extracted call is
  `logConsolidatedLockHolder()`; inferred callers include `runTagWriteCmd()`,
  `SyncConsolidatedFrom()`, `ConsolidateFrom()`, and `RebuildFromTranscripts()`.
- The tag-write read-back contract is represented by the inferred caller
  `TestTagWriteLandsInTheOneStoreAndReadsBack()` of `runTagWriteCmd()`.

The exact shortest path from `TagFile` to `SyncConsolidatedFrom()` is:

`TagFile` <-[references, EXTRACTED]- `gatherTagFiles()`
 <-[calls, EXTRACTED]- `.ingestForeignTags()`
 -[calls, INFERRED]-> `SyncConsolidatedFrom()`.

`TagFile` itself was resolved through its references to `TagSegment` and
`TagVerdict`, and references from `writeTag()`, `applyResolvedTags()`,
`writeTagFileAtomic()`, and related tag readers/writers. This supports a durable
tag-file seam as a candidate mechanism, but Graphify does not establish the
transactional or crash guarantees of those functions.

### Detached seams and dead end

- Detached hook behavior is represented by
  `TestClaudePrimeScript_ExecutesDetachedIngest()` and
  `TestPrimeScripts_EmitDetachedIngest()`, both inferred callers of
  `renderHookScript()`.
- `detach()` resolves only to `Cmd` [references, EXTRACTED] and `detach_unix.go`
  [contains, EXTRACTED]. This is a narrow platform seam, not a complete
  publisher contract.
- `runAutosyncChild()` is `internal/cli/cmd_archive_autosync.go:L40`, community
  `ServeSession`, degree 7. Its exact neighbors are `Load()` [calls, INFERRED],
  `autosyncLogLine()` [calls, EXTRACTED], `localTagExporter()` [calls,
  INFERRED], and `cmd_archive_autosync.go` [contains, EXTRACTED].
- The shortest-path request `runAutosyncChild()` -> `detach()` found no path.
- No node matching the literal `vector-topup` exists. The broader query did find
  `topup.go`, `MaybeVectorTopup()`, `SetSpawnVectorTopup()`,
  `VectorTopupLogPath()`, and `AcquireTopupLock()`, but these are not proof of a
  detached tag-publication implementation.
- `spawnIngestChild()`'s exact BFS has only
  `spawnIngestChild() -> openIngestLog()` [calls, EXTRACTED] and
  `openIngestLog() -> CacheDir()` [calls, INFERRED], then `cacheHome()`.
  This graph-backed shape is a logging/cache seam, not evidence that it is a
  suitable derived-store publisher. Reusing it as one is therefore a dead end
  for this report.

### Setup and benchmark context

`Setup()` is an archive-test helper with extracted calls to `git()` and
`writeFile()` and an inferred call to `Init()`. `BenchmarkFTS5Search()` is
`internal/index/index_bench_test.go:L54`, community `runAutosyncChild`; its
neighbors include `EnsureSchema()`, `UpdateIndex()`, `seedBenchmarkCorpus()`,
`SearchHits()`, `ConnectRO()`, and `ConnectRW()`. The graph contains no
candidate-specific benchmark node for commit `61b7957`.

## What Graphify proves

1. The current tree has a direct inferred tag-write relationship to both
   `AcquireConsolidatedFence()` and `SyncConsolidatedFrom()`.
2. The consolidation function fans into schema, merge, migration, tombstone,
   watermark, and connection operations, with the fence in that mechanism map.
3. Catalog safety tests connect `renderHookScript()` and `ReadCatalogEntry()`;
   the special-path test also connects detached-ingest inspection.
4. Existing detached behavior is represented around hook tests and a separate
   autosync child. The graph does not connect autosync child execution to the
   platform `detach()` node.
5. `spawnIngestChild()` is connected to log opening and cache lookup only.

Confidence tags are preserved above: extracted edges are structural graph facts;
inferred edges are graph inference and weaker.

## What Graphify cannot prove

- It cannot see or evaluate unindexed candidate commits or branch-only code.
- It cannot prove that a tag write is durable before publication, that a detached
  child survives CLI exit, or that publication is idempotent.
- It cannot prove read-after-write visibility, overlay/union semantics, receipt
  contents, timeout bounds, or crash recovery.
- It cannot prove that the absence of `vector-topup` as a literal node means the
  implementation is absent; only that this graph has no such node.
- It cannot turn inferred test-call edges into production call guarantees.

All empirical experiments not run (tests, race tests, process-exit tests,
receipt inspection, and candidate runtime behavior) are **UNCERTAIN**.

## Metadata-only candidate comparison

Compared with base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`:

| Candidate | Merge base | commits ahead | paths / net lines | stable patch-id | dry cherry-pick |
|---|---|---:|---|---|---|
| `bd8346c` | `0d1da19c` | 4 | 4 paths, +283/-147 (net +136) | `51623b492a55e9366b33c0bc5821597f1bd51df3` | CLEAN |
| `37ec96b` | `5b9756b` | 8 (`0d1da19c...37ec96b`: 2 left / 8 right) | 8 paths, +666/-116 (net +550) | `e2492d10a547b703c62496a0e2a046b2c22952cf` | CONFLICT (`internal/cli/setup.go`) |
| `61b7957` | `0d1da19c` | 3 | 2 paths, +44/-73 (net -29) | `81143e1ed9deee1c34eb705e5909a8790e991e3c6` | CLEAN |

Affected names were, respectively: `bd8346c` — `internal/cli/cmd_ingest_test.go`,
`internal/cli/cmd_tag.go`, `internal/cli/setup.go`,
`internal/store/connect_bench_test.go`; `37ec96b` — those areas plus
`AGENTS.md`, `PRIOR_ART_SOURCES.md`, `WORKER_PROBLEM_PRIOR_ART.md`,
`internal/cli/setup_test.go`, and `internal/index/consolidated_test.go`; and
`61b7957` — `internal/cli/cmd_tag.go` and
`internal/store/connect_bench_test.go`.

Selected blob identities reinforce overlap without reading content:

- `internal/cli/cmd_tag.go`: base `c2c3510`; `bd8346c` and `61b7957`
  `d20c3bf`; `37ec96b` `115a14c`.
- `internal/cli/setup.go`: base `7d4e1cc`; `bd8346c` `c577bdc`;
  `37ec96b` `317fa26b`; `61b7957` remains base.
- `internal/cli/cmd_ingest_test.go`: base `daab6de`; `bd8346c` and `37ec96b`
  `c7915a3`; `61b7957` remains base.
- `internal/store/connect_bench_test.go`: base `6face08`; `bd8346c` and
  `61b7957` `dad90d1`; `37ec96b` remains base.

Graph PR context was also checked: PR #35 reports CI FAILURE and graph impact
550 nodes / 38 communities across 25 files; PR #37 reports CI FAILURE and 34
nodes / 5 communities across one file. This is PR metadata, not proof of the
candidate commits' behavior.

## Adoption claim and falsifier

Strongest graph-backed adoption claim: the smallest recognizable current-base
seam is the existing `runTagWriteCmd()` -> `AcquireConsolidatedFence()` /
`SyncConsolidatedFrom()` path, with durable tag-file operations upstream through
`gatherTagFiles()` and `.ingestForeignTags()`. Any detached publication design
should preserve that fence boundary and make read-after-write behavior explicit.

Strongest falsifier: a focused runtime test showing that foreground tag-write
completion followed by CLI exit can leave the authoritative tag unread or the
derived store permanently stale, or that a detached child races the consolidated
fence. Graphify cannot run or falsify that experiment; status is **UNCERTAIN**.

## Rival-claim scorecard

| Rival claim | Graph score | Reason |
|---|---|---|
| “A detached tag publisher already exists via vector-topup.” | Overreach | No `vector-topup` node and no `runAutosyncChild()` -> `detach()` path; only generic topup symbols appear. |
| “`spawnIngestChild()` is the reusable publisher seam.” | Overreach | Its graph neighborhood stops at `openIngestLog()` and cache lookup. |
| “Removing synchronous fold is safe because tag write is committed.” | Overreach | Graph shows tag-write -> fence/sync and read-back tests, but proves neither commit ordering nor eventual visibility. |
| “Catalog special-path protection proves all hook paths are safe.” | Overreach | The relevant edge is an inferred test relationship; shell/process semantics are outside graph proof. |
| “The candidate benchmark/commit is represented in the current graph.” | Overreach | Querying `61b7957` produced no candidate commit node; graph is base/current-tree only. |
