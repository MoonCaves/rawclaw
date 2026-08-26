# Hostile Architecture & Deletion Review

**Desk**: Lenny Bruce / Architecture & Deletion Reviewer  
**Scope**: Target Commits `5c50c7c`, `b21852899cf4`, `c129eed`, `54bf2b03d3b3`, plus Targeted Prior Art Falsification  
**Methodology**: Deletion Test, Right-Sizing, Deep Module Analysis, Git Prior Art (`object-name.c`, `patch-id`), and Go Proverbs.

---

## 1. Executive Summary

Four commits across container, tombstone, and consolidation refactors were audited alongside upstream Git prior art (`object-name.c:update_candidates` at `c44beea485f0` and `patch-id --stable` at `f78ce2f7b6df`):

1. `5c50c7c` successfully removed duplicate `if rebuild` conditional branching in `internal/index/consolidated.go` and deduplicated `durable.Meta` initialization in `internal/index/containers.go` via `containerMeta` (net -11 lines).
2. `b218528` performed an arithmetic correction in documentation accounting from -13 to -11 lines.
3. `c129eed` recorded review decisions that correctly rejected pass-through wrappers (`RefreshFreshContainer`/`FoldFreshContainer`), but initially tolerated moving unsafe pruning rather than deleting it outright.
4. `54bf2b0` executed the true Deletion Test by completely removing `pruneStaleRefreshDBs` and its fragile test fixture (net **-159 lines**), eliminating an un-fenced filesystem race condition.
5. **Targeted Prior Art Audit (Resolver & Patch-ID)**: Evaluated resolver seams and candidate disambiguation against Git `object-name.c`. Ruled **EXACT NO-CHANGE** on product Go sources; all 6 invariant gates are verified and pass under the race detector.

---

## 2. Target Commit Audits & Detailed Findings

### Finding 1: Deduplication of Rebuild & Vault Guards (`5c50c7c`)
* **SHA**: `5c50c7c00ce748eb73d3c0454ce0027c9f90112c`
* **Path & Lines**:
  * `internal/index/consolidated.go:418-420`
  * `internal/index/containers.go:578-595`
* **Tag / Rung**: `shrink` / `yagni` (eliminating duplicate branch and struct initialization redundancy).
* **Reproducible Evidence**:
  1. *Duplicate Branch in `consolidated.go`*:
     ```go
     // Before 5c50c7c:
     	if rebuild {
     		if err != nil {
     			return st, fmt.Errorf("preserve consolidated tags: %w", err)
     		}
     	}
     	if rebuild {
     		prevLive = dst
     ```
     Two adjacent identical conditional checks with no state mutation between them.
  2. *Deduplicated `durable.Meta` in `containers.go`*:
     `vaultContainer` and `vaultContainerAll` duplicated 14 lines of struct construction and `backingFileState` calls. Commit `5c50c7c` extracted `containerMeta`:
     ```go
     func containerMeta(c source.Container, sourceID string, projectArg, cwdArg any) (durable.Meta, string) {
         m := durable.Meta{
             ID:         c.ID,
             Source:     sourceID,
             Project:    strOf(projectArg),
             CWD:        strOf(cwdArg),
             IsSubagent: c.IsSubagent,
             ParentID:   c.ParentID,
             SourcePath: realpath(c.Path),
         }
         rawPath := backingFilePath(c.Path)
         if mtime, size, fp, err := backingFileState(rawPath); err == nil {
             m.SourceMTime = mtime
             m.SourceSize = size
             m.SourceFP = fp
         }
         return m, rawPath
     }
     ```
* **Deletion Test**:
  * Deleting the redundant `if rebuild` check eliminates dead branching syntax.
  * Deleting `containerMeta` would force re-duplicating 14 lines of metadata construction across both vault entrypoints.
* **Exact Replacement**:
  * `consolidated.go`: Merge adjacent `if rebuild` checks into one block.
  * `containers.go`: Keep `containerMeta`, but replace `projectArg, cwdArg any` with `project, cwd string` to eliminate type erasure.
* **Safety Risk**: None; behavior and metadata fields are strictly preserved.
* **Net Production Lines**: **-11 lines** (-2 in `consolidated.go`, -9 in `containers.go`).

---

### Finding 2: Unfenced Refresh Cache Pruning Deletion (`54bf2b0`)
* **SHA**: `54bf2b03d3b32bf639924ff0a1f8f6885772eb81`
* **Path & Lines**:
  * `internal/index/containers.go:44-80, 134`
  * `internal/index/containers_test.go:591-709`
* **Tag / Rung**: `delete` (un-fenced background GC on synchronous ingest path).
* **Reproducible Evidence**:
  `pruneStaleRefreshDBs()` scanned `store.CacheDir()/refresh` and called `os.Remove(filepath.Join(dir, name))` during every `EnsureFreshContainer()` invocation.
  1. *Concurrency Hazard*: Concurrent processes writing to `-wal` and `-shm` sidecars could have files unlinked out from under active SQLite handles, triggering `SQLITE_IOERR` or silent corruption.
  2. *Orthogonal Concern*: A targeted single-container refresh operation was performing synchronous directory-wide disk garbage collection.
  3. *Unnecessary Complexity*: Temporary refresh databases are deterministically addressed by container ID hash (`RefreshDBPath`) and overwritten safely on refresh.
* **Deletion Test**:
  Deleting `pruneStaleRefreshDBs` completely removed 40 lines of dangerous un-fenced IO and 119 lines of mock mtime tests (`TestEnsureFreshContainer_PruneStaleLeftovers`) without breaking container refresh semantics or atomicity.
* **Exact Replacement**: Complete removal of `pruneStaleRefreshDBs()` and its test fixture.
* **Safety Risk**: Positive; eliminates a multi-process race condition and prevents unexpected database unlinks.
* **Net Production Lines**: **-159 lines** (-40 in `containers.go`, -119 in `containers_test.go`).

---

### Finding 3: Container Hostile Review Findings Accuracy (`c129eed`)
* **SHA**: `c129eedc0c55f5b572c57eef613e31ac3f0fda69`
* **Path & Lines**: `FINDINGS.md:1-73`
* **Tag / Rung**: `review-record`
* **Reproducible Evidence**:
  * Correctly rejected Luna's `RefreshFreshContainer`/`FoldFreshContainer` wrappers (`f3d03b0`) and redundant tail-loop extraction (C3, C5).
  * However, findings C1 and C4 approved shifting `pruneStaleRefreshDBs` into `EnsureFreshContainer` (`85cf480`) and grouping WAL/SHM mtimes (`ba60ca8`). This was a shallow review: it optimized an unsafe mechanism that should have been deleted outright, as subsequently accomplished in `54bf2b0`.
* **Deletion Test**: Documentation accurately reflects state prior to `54bf2b0`.
* **Exact Replacement**: Update review record to note that C1/C4 are superseded by complete deletion in `54bf2b0`.
* **Safety Risk**: Documentation accuracy only.
* **Net Production Lines**: 0 in source; documents net -159 lines achieved in `54bf2b0`.

---

### Finding 4: Tombstone Transplant Net Lines Accounting (`b218528`)
* **SHA**: `b21852899cf4904b7dff92bdae58f9ee60799af0`
* **Path & Lines**: `FINDINGS.md:14`
* **Tag / Rung**: `accounting-fix`
* **Reproducible Evidence**:
  Commit `5c50c7c` had 8 additions and 19 deletions ($19 - 8 = 11$). Corrected misstated `-13` to `-11`.
* **Deletion Test**: Verified arithmetic correction.
* **Exact Replacement**: Text fix.
* **Safety Risk**: None.
* **Net Production Lines**: 0.

---

## 3. Targeted Prior Art Falsification & Scorecard

### A. Resolver Attack: Git `object-name.c:update_candidates` at `c44beea485f0` (Receipt `c4d67bd`)
* **Upstream Invariant**: Abbreviated object lookup accumulates candidate OIDs, deduplicates identical full IDs, and returns explicit ambiguity when 2+ distinct IDs survive. No silent source ranking; exact-first, unique prefix second.
* **RawClaw Resolver Mapping**:
  * `internal/agentproto/agentproto.go:1761-1833` (`LocateSession`, `LocateSessionGuarded`, `oneStoreCands`, `catalogCands`, `sweepScopes`, `decideSession`).
  * `internal/paths/paths.go:298-348` (`ResolveSession`, `resolveSessionCatalog`).
* **Evaluation of 6 Semantic Gates**:
  1. *Exact full ID*: Resolved via exact catalog probe and indexed lookup.
  2. *Unique prefix*: Resolved via candidate count `len(cands) == 1`.
  3. *Two distinct full IDs*: `decideSession` returns `*ErrAmbiguousSession` with candidate list.
  4. *Same full ID across scopes/sources*: `firstRowPerSession` dedupes multi-directory continued sessions into a single candidate without false ambiguity.
  5. *Foreign catalog hit*: In `54afa70`, `catalogCands` skips foreign paths when `paths.ProjectDirOf(hit.Path) == ""` so fallback `more` resolves the pre-resolved foreign scope (`TestGuardedSessionLookupUsesForeignPreResolvedScope`).
  6. *Missing / empty store*: `oneStoreCands` logs a warning and falls back to catalog/sweep without failing lookup.
* **Ruling**: **EXACT NO-CHANGE RULING ON GO CODE** (`CLEAN & VALIDATED`).

### B. Patch-ID Attack: Git `patch-id --stable` and `range-diff` at `f78ce2f7b6df` (Receipt `dd655e7`)
* **Verified Series Patch-IDs**:
  * `54afa70`: `4b310ec5516b651c43cfecef4dca4124d061b8bf` (Preserve source semantics in catalog fallback)
  * `bf7cdd0`: `c943b45299eb099ee864b4798e6b11d5d1988086` (Prove guarded codex catalog fallback resolution)
* **Placebo Rejections**:
  * `21b8011` (`slices.Backward` iterator rewrite in `cmd_tag.go:284`): Net 0 lines, adds closure overhead over native reverse loop → **REJECTED**.
  * `d82d9de` (Range-over-int downgrade & test deletion in `tagrefresh_test.go`) → **REJECTED**.

---

## 4. Skill Scorecard & Evaluation

| Skill | Grade | Actionable Deletion Signal | Correctness & Safety | Noise / False Positives | Verdict |
|---|:---:|---|---|---|---|
| **`codebase-design`** | **A** | Exceptional | High | None | The Deletion Test cleanly identified `pruneStaleRefreshDBs` as pass-through bloat (-159 lines) and rejected wrapper sprawl (`RefreshFreshContainer` / `FoldFreshContainer`). |
| **`golang-code-style`** | **A** | High | High | None | Caught duplicate `if rebuild` branches in `consolidated.go`, enforced early returns, and flattened control flow. |
| **`golang-structs-interfaces`** | **A-** | High | High | Low | Enforces "no premature interfaces" and "avoid `any`"; correctly identified untyped arguments in `containerMeta`. |
| **`modular-refactor`** | **B+** | High | High | Low | Right-sizing rule prevented splitting 10-line loops into artificial layers, though initial phase reviews favored moving `pruneStaleRefreshDBs` rather than deleting it. |
| **`golang-design-patterns`** | **B+** | High | High | Low | Strong on avoiding implicit side-effects, though slightly less tailored to pure line-deletion metrics. |
| **`golang-modernize`** | **B** | Moderate | High | Low | Focuses on language feature adoption (Go 1.21–1.26 stdlib) rather than architectural and wrapper deletion. |

---

## 5. Overall Net Production Lines & Verification

* `5c50c7c`: **-11 lines** (Source code deduplication in `consolidated.go` and `containers.go`)
* `54bf2b0`: **-159 lines** (Deleted `pruneStaleRefreshDBs` source and tests)
* **Total Net Reduction**: **-170 lines**
* **Verification Gate**: Full race test pass across `agentproto` (249.7s), `cli` (280.8s), `paths` (5.6s), and `index` (339.0s) — `PASS`.
