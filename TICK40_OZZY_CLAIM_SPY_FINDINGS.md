# Tick 40 Ozzy claim-spy findings

## Scope and evidence fence

- Required base `ef2eebf414e77086be06281539c5a50ba036a32a` was verified before creating the isolated worktree.
- Worktree: `/Users/jay-m4/code/rawclaw-furiosa-t40-ozzy-claimspy-20260827`; branch: `worker/furiosa-t40-ozzy-claimspy-20260827`.
- Evidence fence: Ozzy checkout `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec`, checked-out branch `codex/tag-closeout-instant-spec-20260826`, and Ozzy-named refs only. Checkout was clean and tracked its origin branch; Ozzy refs were remote-only (no local tracking branches).
- Graphify ran `reflect --if-stale`, the requested consolidated/fence/prune/benchmark query, `explain AcquireConsolidatedFence`, and `path AcquireConsolidatedFence SyncConsolidatedFrom`; the path shows `SyncConsolidatedFrom` calls the fence.
- Tick 36 immutable evidence already counted c38 external adoption as Ozzy +3. No duplicate credit is awarded.

## Ancestry, payload, and patch identity

Each listed commit resolved and had merge-base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` with the required base. Stable whole/path patch IDs were distinct:

| commit | parent | payload | net | stable patch ID |
|---|---|---|---:|---|
| c38f79a sidecar pruning | 96aa522 | 2 production/test files | +48 | 6a62ff59b1b20a5873006b17ce72cd64229f65a6 |
| 75d1656 removed-session sidecars | 857dc62 | 2 production/test files | +99 | 88512d6e9bddea3f848b235a72ea9dc823c0197f |
| 537641b topic deletion | 593b16e | 2 production/test files | +52 | b7aaaee70fe88073287bb0fecc0c9b81beb80368 |
| 4f8ea6c narrow publisher fence | 48ef14f | 2 production files | +17 | fdf6b91cda7b2204274781303b335ec12c59d55a |
| 7dad56d fast-path refresh | 9110351 | 2 production files | +10 | eeca83456293ccf24fef13b1aa4e34183da61163 |
| 292284a no-fold explicit dirs | 96aa522 | 4 production/test files | +157 | 81eeddee3bde760245ab930199d9149f65a53080 |

`integrate/tagwrite-closeout-wave1` is `0d1da19c`; its remote is newer at `a33ab023eae0ca324956a66cf17b7ffa5b16c39d` (one 8-line benchmark-test deletion). No merge authorization exists.

## Per-claim verdicts

1. **c38f79a sidecar pruning — CONFIRMED behavior, NO NEW SCORE.** Focused source-without-sidecar-table behavior is real. It is distinct from 75d1656 and Furiosa `0cd00e4`, but prior immutable evidence already counted Ozzy +3.

2. **75d1656 removed-session sidecars — CONFIRMED narrow behavior, NO SCORE CLAIM.** The +48 production/+51 test payload is real, but does not establish partial-source/co-contributor or whole closeout correctness.

3. **386ec9d/73171fd tombstone-prune speed — REBUTTED as performance proof; correctness beyond focused tests UNCERTAIN.** Ozzy's `FINDINGS_PRUNE.md` reports only-new implementation samples (15.90–22.70 ms/op), no before/after baseline or benchstat, and a correlated `EXISTS` shape that may lose the old indexed missing-ID path. Required: identical parent/new fixture, >=6 samples each, benchstat, 1-existing and 600-missing cases, query plans, allocation/pragma details.

4. **8a62bf5 overlay/cancellation review — CONFIRMED as a challenge, not a fix.** Its stale same-session resurrection, SQL-phase cancellation, and detached queued-work durability findings are material; it provides no current-base implementation or end-to-end gate.

5. **4f8ea6c/551c143 narrow publication — UNCERTAIN.** Fence and eventual-publication tests are seam evidence only; they do not prove child failure, timeout, retry ownership, or whole-session authority.

6. **7dad56d, 74d4ee9, 292284a/f58a4c no-fold/fast-path claims — UNCERTAIN.** Sibling experiments do not show composition of refresh, no-fold, explicit-directory, and consolidated-closeout contracts on `ef2eebf`. No score.

## Unique challenge and required response

The strongest challenge is the tombstone speed claim: a new-only benchmark cannot prove a win and may replace six cheap indexed probes with six correlated scans. Ozzy must provide immutable receipt SHA, exact base/parent, raw samples, benchstat against parent, both fixture shapes, `EXPLAIN QUERY PLAN`, exact `CGO_ENABLED=0 go test -race` output, and dirty/upstream state. Without these fields: **NO SCORE CLAIM**.

## Readiness, scoring, and gates

- Current-base readiness against `ef2eebf`: **NOT READY** for merge or score uplift.
- Score recommendation: **Ozzy +0 new**; preserve existing +3 only. Reject duplicate c38 credit.
- Net-line evidence: c38 +48, 75d +99, 537 +52, 4f8 +17, 7dad +10, 292284a +157. Payload size is not a quality score.
- Exact bounded command attempted on the c38 referee checkout: `CGO_ENABLED=0 go test -race ./internal/index -run 'Test(Consolidate_(DeletesSidecarsWhenSourceRemovesWholeSession|PrunesExistingSidecarsWhenSourceHasNoSidecarTables)|PruneTombstonedIDs)' -count=1`. The tool returned no observable output; verdict is **UNCERTAIN**, not PASS.
- No Go files changed; `gofmt -w` was **N/A**.

Furiosa line: **A new-only stopwatch cannot turn a correlated delete into a proven speedup.**
