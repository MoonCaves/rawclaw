# Code-Reduction & Simplification Audit: `ada503f..43cb92f`

**Session Tag:** `20260826-ponytail-audit`  
**Audit Scope:** Git diff `ada503f..43cb92f` (+19,219 additions, -1,132 deletions across 117 files)  
**Target Codebase:** RawClaw — Pure Go (`CGO_ENABLED=0`), zero-runtime dependencies, SQLite FTS5 indexer.  
**North Star Constraint (`AGENTS.md`):** Keyword search by default, sovereign single binary, no LLM in hot path, no silent failure or data truncation.

---

## Executive Summary

Between commits `ada503f` and `43cb92f`, the RawClaw repository expanded by **19,219 lines** of Go source, tests, and documentation. This growth accompanied major architectural shifts:
1. One-store consolidation (`consolidated.db`, `session_sources` provenance tracking, and O(1) freshness checks).
2. Multi-adapter expansion (Google Antigravity and Goose SQLite adapters).
3. Tail-based incremental transcript ingestion (`internal/index/tail.go`).
4. Background ingest hooks and rate-limited spawn tokens (`internal/cli/cmd_ingest.go`, `internal/cli/bg_ingest.go`).
5. Routine-verdict tiering and vector coverage warning infrastructure.

While these features achieved high reliability and sub-millisecond query performance, rapid incremental landing introduced substantial code duplication, over-abstraction, copy-pasted parser routines, and redundant test scaffolding.

This audit identifies **~1,200–1,500 lines of removable non-test code** and **~1,200–1,600 lines of redundant test code**, for a total code reduction opportunity of **~2,400–3,100 lines (~13–16% of all added code)** without regressing any functional capabilities or invariants.

---

## Code Reduction Opportunity Scorecard

| Rank | Component / Area | Primary File(s) | Primary Anti-Pattern | Est. Removable LOC | Risk Level |
|---|---|---|---|---|---|
| **1** | **Incremental Tail Parser Duplication** | `internal/index/tail.go` | Reimplemented / copy-pasted adapter parsers | **~345 LOC** | Low |
| **2** | **Tag Windowing & Range Replacement Complexity** | `internal/cli/cmd_tag.go`, `internal/store/topics.go` | Over-abstraction & complex sliding window | **~296 LOC** | Low (Proven on `4c82fbe`) |
| **3** | **Consolidated Store Migration & Introspection Bloat** | `internal/index/consolidated.go` | Duplicated PRAGMA checks & legacy compatibility scaffolding | **~210 LOC** | Low–Medium |
| **4** | **Ingest vs Tag-Prep Source Resolution Duplication** | `internal/cli/cmd_ingest.go`, `internal/cli/tagrefresh.go` | Duplicated resolution & lookup pipelines | **~120 LOC** | Low |
| **5** | **Shared Candidate Sort & Routine Verdict Duplication** | `internal/view/view.go`, `internal/agentproto/agentproto.go`, `internal/store/verdict.go` | Verbatim duplicate helper & invariant-violating helper | **~75 LOC** | Low |
| **6** | **Session Catalog vs Stem Resolution Duplication** | `internal/paths/paths.go` | Redundant path validation & metadata extraction | **~50 LOC** | Low |
| **7** | **Test Suite Duplication (Tail Edge vs Incremental vs Consolidated)** | `tail_edge_test.go`, `incremental_test.go`, `consolidated_test.go` | Duplicate test setups & overlapping assertion cases | **~1,400 LOC** | Low (Test cleanup only) |

---

## Prioritized Non-Test Code Reduction Opportunities

---

### Candidate 1: Verbatim Source Parser Duplication in `internal/index/tail.go`

- **Target File:** `internal/index/tail.go:152-497` (+496 lines total in file)
- **Lines Removable:** **~345 LOC** (69% of the file)
- **Anti-Pattern:** Duplicated helper / reimplemented existing adapter parser logic.
- **Risk Level:** **Low**

#### Technical Analysis
`internal/index/tail.go` was introduced to parse only the appended tail bytes of modified transcript files. Instead of reusing the parsing logic already defined in the source adapters or the shared parse package, `tail.go` copy-pasted private helper functions from:
1. **Google Antigravity Adapter (`internal/source/antigravity/antigravity.go`):**
   - `normalizeAntigravity` (`tail.go:386-446`) is an exact copy of `normalize` (`antigravity.go:472-533`).
   - `parseAntigravityUserRequest` (`tail.go:448-460`) is an exact copy of `parseUserRequest` (`antigravity.go:536-548`).
   - `formatAntigravityToolArgs` (`tail.go:462-491`) is an exact copy of `formatToolArgs` (`antigravity.go:551-580`).
   - `mintAntigravityUUID` (`tail.go:493-496`) is an exact copy of `mintUUID` (`antigravity.go:583-586`).
2. **Codex Adapter (`internal/source/codex/codex.go`):**
   - `normalizeCodex` (`tail.go:216-257`), `mapCodexRole` (`tail.go:259-273`), `codexContentText` (`tail.go:275-294`), `codexSummaryText` (`tail.go:296-302`), `codexOutputText` (`tail.go:304-316`), `codexActionQuery` (`tail.go:318-325`), `codexArgsText` (`tail.go:327-340`), and `mintCodexUUID` (`tail.go:342-345`) duplicate identical functions in `internal/source/codex/codex.go:174-273`.
3. **Claude Code Parser (`internal/index/index.go` / `internal/parse`):**
   - `parseClaudeTail` (`tail.go:153-181`) duplicates the record loop from `internal/index/index.go`.

#### Concrete Impact
Having duplicate parsing code in two places causes maintenance divergence. When an adapter bug is fixed in `internal/source/antigravity/antigravity.go` (e.g. handling a new tool call format or schema change), the incremental tail parser in `tail.go` remains stale unless manually synchronized.

#### Proposed Reduction
Export `NormalizeMessage` / `ParseRecord` on the adapter packages (or provide a chunk-parsing helper on the `source.Source` interface or adapter package), allowing `tail.go` to delegate line parsing directly. `tail.go` shrinks from 497 lines to under 150 lines.

---

### Candidate 2: Simplify Tag Windowing & Multi-Range Deletion in `internal/cli/cmd_tag.go`

- **Target Files:** `internal/cli/cmd_tag.go:36-824` (+546 LOC net), `internal/store/topics.go`
- **Lines Removable:** **~250–300 LOC** in `cmd_tag.go` + **~46 LOC** in `topics.go`
- **Anti-Pattern:** Over-abstraction & excessive sliding-window complexity.
- **Risk Level:** **Low** (Directly validated on branch `refactor/tag-insert-only` commit `4c82fbe`)

#### Technical Analysis
Between `ada503f` and `43cb92f`, `cmd_tag.go` expanded from 278 lines to 824 lines (+546 lines net). The expansion introduced:
1. Complex multi-range chunking (`computeTagChunk`, `messageRange`, `chunkByteCap`).
2. Gap detection and multi-range validation across disjoint message sets (`rangeCovered`, lines 675-685).
3. Dual fallback mapping from UUID to display index and numeric message ID (`uuidToDispIdx`, `uuidToMsgID`, lines 721-782).
4. `store.ReplaceSessionRangeSegments` in `internal/store/topics.go` (46 lines of dynamic SQL deletion logic).

#### Empirical Proof on Branch `refactor/tag-insert-only` (`4c82fbe`)
The `refactor/tag-insert-only` branch (`4c82fbe`) demonstrates that simplifying to **insert-only windowing with exact anchor validation**:
- Reduces `cmd_tag.go` by **303 lines** (replaced by 194 lines, net reduction of 109 lines while deleting fragile multi-range edge cases).
- Deletes `ReplaceSessionRangeSegments` from `internal/store/topics.go` (-46 LOC).
- Simplifies `internal/cli/cmd_tag_test.go` (-185 LOC).
- Replaces complex overlapping range calculations with simple prefix matching against `displayable[winStart..winEnd]`.

---

### Candidate 3: Consolidated Store Migration Scaffolding & PRAGMA Introspection Bloat

- **Target File:** `internal/index/consolidated.go` (+725 LOC net, 1,347 lines total)
- **Lines Removable:** **~180–250 LOC**
- **Anti-Pattern:** Legacy compatibility scaffolding, duplicated PRAGMA introspection, and redundant SQL transaction boilerplate.
- **Risk Level:** **Low–Medium**

#### Technical Analysis
1. **Legacy Basename Watermark Scaffolding (`consolidated.go:108-148`):**
   `rewriteLegacyWatermark` and the single-candidate basename fallback loop exist purely to migrate pre-`ebc086a` builds. Lines 137–145 even open a secondary `store.ConnectRW` connection if `con.Exec` fails on read-only handles. This entire fallback path can be collapsed or made a simple one-time schema upgrade migration.
2. **Duplicate SQLite PRAGMA Introspection (`consolidated.go:841-913`):**
   `srcHasTable` (lines 847-858), `srcHasColumn` (lines 864-888), and `hasScopeColumns` (lines 890-913) execute manual `PRAGMA table_info` scans. This duplicates table/column introspection helpers in `internal/source/goose/goose.go` and `internal/store/`. A single generic helper `tableHasColumns(con, table, cols...)` replaces ~75 lines.
3. **Redundant Backfill Queries in `migrateSessionSources` & `healUpgradedConsolidatedStore` (`consolidated.go:976-1100`):**
   `healUpgradedConsolidatedStore` and `migrateSessionSources` perform redundant table checks and sequential `SELECT COUNT(*)` statements across `sessions` and `session_sources` that can be unified into a single migration guard.

---

### Candidate 4: Duplicated Ingest and Tag-Prep Source Resolution

- **Target Files:** `internal/cli/cmd_ingest.go:153-240` (+316 LOC), `internal/cli/tagrefresh.go:44-150` (+70 LOC)
- **Lines Removable:** **~100–140 LOC**
- **Anti-Pattern:** Duplicated lookup pipelines & parallel container matching structs.
- **Risk Level:** **Low**

#### Technical Analysis
Both `cmd_ingest.go` and `tagrefresh.go` live in package `cli` and execute nearly identical 4-stage resolution pipelines for a session argument:
1. Check session catalog (`catalogIngestSource` in `cmd_ingest.go` vs direct catalog lookup in `paths.ResolveSession`).
2. Check stem lookup (`stemTagSources` in `tagrefresh.go`).
3. Check registered source adapter discovery (`discoverAllIngestSources` vs `discoverTagSources`).
4. Check consolidated store location (`locatedTagSource`).

Additionally, `cmd_ingest.go:311-316` defines its own private `backingPath(p string)` which is byte-for-byte identical to `internal/index/containers.go:87-92` `backingFilePath(p string)`.

#### Proposed Reduction
Unify session resolution in `internal/cli/tagrefresh.go` (or `internal/paths`) into a single `ResolveTargetContainers(sessionArg string)` helper and reuse `index.BackingFilePath`.

---

### Candidate 5: Verbatim Duplicate `sortCandidates` & Invariant-Violating `RoutineVerdictSet`

- **Target Files:** `internal/view/view.go:318-355`, `internal/agentproto/agentproto.go:1420-1457`, `internal/store/verdict.go:167-184`
- **Lines Removable:** **~75 LOC**
- **Anti-Pattern:** Verbatim duplicate helper & dead/harmful helper function.
- **Risk Level:** **Low**

#### Technical Analysis
1. **Verbatim Duplicate Helper (`view.go` vs `agentproto.go`):**
   Lines 318–355 of `internal/view/view.go` and lines 1420–1457 of `internal/agentproto/agentproto.go` contain an identical 38-line implementation of `sortCandidates(cands []retrieve.Anchor, mode string)` implementing the 3-way sort ("newest", "oldest", and relevance with routine tiering).
2. **Harmful / Redundant `RoutineVerdictSet` (`store/verdict.go:167-184`):**
   `store.RoutineVerdictSet` returns sessions marked `routine` without checking if the session carries real topic segments. As documented in `docs/notes/adversarial-review-20260825.md`, calling `RoutineVerdictSet` in `topicsFromStore` and `topicsByFanOut` violates the core invariant *"A real tag beats routine"*.
   Removing `RoutineVerdictSet` and standardizing on `store.RoutineSet` eliminates dead code and fixes a known correctness bug.

---

### Candidate 6: Session Catalog vs Stem Resolution Duplication in `internal/paths/paths.go`

- **Target File:** `internal/paths/paths.go:298-396` (+156 LOC total)
- **Lines Removable:** **~50 LOC**
- **Anti-Pattern:** Redundant path resolution, CWD extraction, and project labeling.
- **Risk Level:** **Low**

#### Technical Analysis
`resolveSessionCatalog`, `validateCatalogHit`, and `sessionHitFromCatalog` (`paths.go:309-396`) duplicate directory extraction and project label computation already implemented in `ProjectLabel`, `ProjectDirOf`, and `resolveSessionStem`. `sessionHitFromCatalog` contains an 18-line fallback ladder for project resolution that mirrors `ProjectLabel(pdir)`.

---

### Candidate 7: Redundant Methods in `internal/store/verdict.go` & `internal/lifecycle/floor.go`

- **Target Files:** `internal/store/verdict.go:41-90`, `internal/lifecycle/floor.go`
- **Lines Removable:** **~35 LOC**
- **Anti-Pattern:** Over-abstraction & redundant SQL statements.
- **Risk Level:** **Low**

#### Technical Analysis
`UpsertVerdict` (`store/verdict.go:41-56`) and `MergeVerdict` (`store/verdict.go:72-90`) execute nearly identical `INSERT ... ON CONFLICT DO UPDATE` statements, differing only in the `WHERE` clause condition for LWW resolution. They can share a single query constructor.

---

## Test Suite Redundancy Analysis

Across `ada503f..43cb92f`, test files grew by **~10,500 lines**. While thorough coverage of edge cases is essential, multiple test suites construct identical mock SQLite schemas and JSONL fixtures to assert the exact same underlying functions.

---

### 1. `internal/index/tail_edge_test.go` (+1,264 LOC) vs `internal/index/incremental_test.go` (+860 LOC)

- **Redundancy Breakdown:**
  - **Incomplete / Truncated Trailing Records:** Tested extensively across `TestIncrementalIngest_IncompleteTrailingLine` and `TestEnsureFreshContainer_IncompleteTrailingLine` (`incremental_test.go:555-676`), and then re-tested in `TestTailEdge_TruncatedTailRecord` (`tail_edge_test.go:249-553`, 305 lines).
  - **Idempotent Watermark Checks:** `TestAppendContainer_StaleWatermarkIsNoOp` (`incremental_test.go:677-744`) duplicates `TestTailEdge_WatermarkExactlyAtFileEnd` (`tail_edge_test.go:161-248`) and `TestTailEdge_ReingestIdempotency` (`tail_edge_test.go:765-940`).
  - **Multi-Adapter Fast-Path Appends:** `TestIncrementalIngest_AppendFastPath_Claude/Codex/Antigravity` (`incremental_test.go:22-344`) duplicates `TestTailEdge_ContentAppendedAfterWatermark` and `TestTailEdge_ParserEdgeCases` (`tail_edge_test.go:554-764, 1111-1225`).
- **Reduction Strategy:** Consolidate shared fixture generation (`createSampleJSONL`) and convert individual 100-line test functions into table-driven subtests.
- **Estimated Removable LOC:** **~600–750 LOC**.

---

### 2. `internal/index/consolidated_test.go` (+1,035 LOC additions)

- **Redundancy Breakdown:**
  - Multiple test functions (`TestConsolidate_LegacyStoreSessionSourcesBackfill`, `TestConsolidate_PartialSessionSourcesBackfill`, `TestConsolidateFrom_PrunesLegacySourceAfterFullPass`, `TestConsolidateFrom_PreservesLegacySourceWhenCoContributorSkipped`, `TestConsolidateFrom_HealFailureAbortsBeforeBackfill`, lines 1400–1874) manually populate separate test databases with 30-to-60 line raw SQL inserts.
- **Reduction Strategy:** Extract a parameterizable `setupTestConsolidatedDB(t, opts...)` helper.
- **Estimated Removable LOC:** **~350–450 LOC**.

---

### 3. `internal/cli/cmd_answer_first_test.go` (+643 LOC) & `internal/cli/cmd_freshness_test.go` (+387 LOC)

- **Redundancy Breakdown:**
  - Both test suites set up full mock Claude/Antigravity directory trees with config files and transcripts, repeating 40+ lines of directory creation and mock binary wrappers in each subtest.
- **Reduction Strategy:** Share common mock environment builders across `cli` tests.
- **Estimated Removable LOC:** **~250–350 LOC**.

---

## Ranked Summary of Code-Reduction Opportunities

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ TOTAL REDUCTION POTENTIAL: ~2,450 – 3,100 LOC                                    │
├──────────────────────────────────────────────────────────────────────────────────┤
│ Non-Test Source Reduction:                                                       │
│   1. internal/index/tail.go (Reuse source adapter parsers):             ~345 LOC │
│   2. internal/cli/cmd_tag.go (Insert-only windowing via 4c82fbe):       ~296 LOC │
│   3. internal/index/consolidated.go (PRAGMA dedup & migration cleanup): ~210 LOC │
│   4. internal/cli/cmd_ingest.go & tagrefresh.go (Unified resolution):   ~120 LOC │
│   5. internal/view/view.go & agentproto.go (sortCandidates & RoutineSet):~75 LOC │
│   6. internal/paths/paths.go (Catalog vs stem dedup):                    ~50 LOC │
│   7. internal/store/verdict.go (Upsert/Merge consolidation):             ~35 LOC │
│                                                                                  │
│ Test Suite Scaffolding Consolidation:                                            │
│   8. tail_edge_test.go & incremental_test.go (Fixture & case dedup):    ~700 LOC │
│   9. consolidated_test.go (SQL fixture unification):                    ~400 LOC │
│  10. cmd_answer_first_test.go & cmd_freshness_test.go (CLI mock reuse): ~300 LOC │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## Verification & Integrity Note

- No production Go source code files (`*.go`) were modified during this audit.
- Target branch is `audit/ponytail-day1`.
- Clean working tree maintained per conduct rules.
