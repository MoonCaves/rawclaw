# T85 hostile smoke: PR #40

Date: 2026-08-28 WITA  
Checkout: `029f60d77e7e03192bc966de3a835a4a32a00fe2`  
Public PR: `MoonCaves/rawclaw#40`  

## Verdict

**ACCEPT — checkout/main merge payload. HOLD — public release behavior.**

The merged checkout passes the observable hostile smoke below. This is not a
release-asset approval: the latest public release remains `v0.10.0`, published
before PR #40 (`gh release view` returned `v0.10.0`, 2026-08-25T22:59:04Z).

## Merge and CI receipts

- `git rev-parse HEAD` = `029f60d77e7e03192bc966de3a835a4a32a00fe2`.
- `git rev-parse origin/main` = the same SHA; `git rev-list --left-right --count origin/main...HEAD` = `0 0`.
- `git show --stat --summary 029f60d` identifies the merge payload as “tag-write closeout: instant authoring and safe detached publication (#40)” with 15 changed files, 1,465 insertions, and 93 deletions.
- `gh pr view 40 --repo MoonCaves/rawclaw` independently reports `MERGED`, merge commit `029f60d...`, and base `main`.
- `gh pr checks 40 --repo MoonCaves/rawclaw` independently reports PASS for `build (1.24.0)`, `build (stable)`, and `lint` in workflow run `33042376289`.
- `git diff --check 029f60d^ 029f60d` passed. Stable patch-id: `0a923027fd72d2225b5ca6aa637968adafe1e98f`.

## Hostile observable contracts

Focused command:

```sh
CGO_ENABLED=0 go test -race ./internal/cli -run '^TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication$' -count=1 -v
```

PASS (`4.45s` test; package `6.766s`). The authoritative project DB remains
usable while detached publication is delayed; publication later acquires the
consolidated fence and completes.

Focused lifecycle/side-effect command:

```sh
CGO_ENABLED=0 go test -race ./internal/cli -run \
  '^TestRunTagWrite(RejectsSegmentOutsideWindow|RetagReplaces|Routine_ReTagPreservesTopics)$|^TestTagWriteFastPath.*$|^TestRunTagPrepCmd_(ContentionSpawnsDetachedFoldAndFoldsOnNextTouch|DetachedFoldDoesNotInspectConsolidatedFailures)$' \
  -count=1 -v
```

PASS. This covered retag replacement, routine-topic preservation, detached
fold after contention, detached-fold failure isolation, and authoring before
the consolidated fence. The test output showed no duplicate/error side effect
and all selected tests passed.

Additional detached publication command:

```sh
CGO_ENABLED=0 go test -race ./internal/cli -run \
  '^TestRunTagWrite(LandsInTheOneStoreAndReadsBack|Routine_MarksRoutineAndFolds)$|^TestTagWriteQueuesDerivedPublication$|^TestRunTagPublishChildHonorsCanceledContext$' \
  -count=1 -v
```

PASS. The queue receipt, routine closeout, one-store readback, and canceled
publisher context contracts all passed.

## Exact cancellation API removal

The previously false-green API is **`SyncConsolidatedFromContext(ctx,
srcPath)`**, not the unrelated still-present vector test
`TestVecIndexCancellationStopsWork`.

Exact history:

```sh
git show 8a87724d5e0e4bea4cb5bcf0be80019ab56fe319
```

Commit `8a87724d` (“fix(index): drop unproven fold cancellation API”) removes
the exported `SyncConsolidatedFromContext` wrapper and the cancellation checks
from `internal/index/consolidated.go`, and deletes
`internal/index/consolidated_cancel_test.go`. The prior test's SHA-256 from its
parent is `060a13b07e4e5e58e5ac59a848e0eaca2c69c1847cea5081d863b95f2ac36ae4`.

Current negative reproduction:

```sh
go doc github.com/MoonCaves/rawclaw/internal/index.SyncConsolidatedFromContext
```

Exit `1`: cannot find that package/symbol. Current `go doc` exposes only
`SyncConsolidatedFrom(srcPath string) error`; `rg -n
'SyncConsolidatedFromContext' . --glob '!graphify-out/**'` returned no matches,
and `git cat-file -e 029f60d:internal/index/consolidated_cancel_test.go`
confirmed the deleted test path is absent.

The remaining `TestVecIndexCancellationStopsWork` was run independently:

```sh
CGO_ENABLED=0 go test -race ./internal/semantic -run \
  '^TestVecIndexCancellationStopsWork$' -count=1 -v
```

PASS (`1.54s` test; package `3.364s`). It is a different API and does not
reintroduce the removed consolidated-fold cancellation claim.

## Limits

This report validates the merged checkout and public-main CI metadata. It does
not validate a post-PR release artifact or installed binary; public release
behavior therefore remains HOLD.
