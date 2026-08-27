# Tick 42 Han cancellation-layer mutation report

## Identity and scope

- Candidate: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` (`fix(index): prune only missing source topics`).
- Parent/base: `4119698525e806025ec36d00e0c85a5b1b3574a7`; verified with `git rev-parse`.
- Worktree/branch: `rawclaw-furiosa-t42-han-cancel-layers-20260827` / `worker/furiosa-t42-han-cancel-layers-20260827`.
- This is report-only. No product file is changed or proposed.

## Exact-one preflights and baseline

Each preflight used `go test -list '^TestName$'` and returned exactly one test:

```text
internal/index: TestSyncConsolidatedFromContext
internal/cli:   TestRunTagPublishChildHonorsCancellationWhileWaitingForFence
internal/cli:   TestRunTagPublishChildHonorsCanceledContext
internal/cli:   TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication
```

Focused `CGO_ENABLED=0 go test -race -count=1` passed: index `1.558s`, CLI `2.094s` (logs SHA-256 `bc1777...` and `a63fb7...`).

## One-fault mutations

| Layer | Mutation | Result | Verdict |
|---|---|---|---|
| Consolidated-fence wait | `AcquireConsolidatedFence(ctx)` -> `AcquireConsolidatedFence(context.Background())` | `TestSyncConsolidatedFromContext` red after `2.06s` timeout; lock holder released; no SQLite error/rows/topic/watermark writes | CONFIRMED: fence cancellation is genuinely covered |
| Child fold context | `SyncConsolidatedFromContext(ctx, dbp)` -> `SyncConsolidatedFrom(dbp)` | `TestRunTagPublishChildHonorsCancellationWhileWaitingForFence` red at `0.33s`; child continued through fence release instead of honoring canceled context | CONFIRMED: child passes cancellation to the fold |
| Transaction admission | `con.BeginTx(ctx, nil)` -> `con.Begin()` | Existing cancellation test stayed green (`0.511s`) because it cancels while blocked at the fence and never reaches BeginTx | NO SCORE CLAIM: transaction admission is untested; supported bounded behavior is UNCERTAIN |
| Source/watermark query | `QueryRowContext(ctx, ...)` -> `QueryRow(...)` | Existing cancellation test stayed green (`0.521s`) for the same pre-fence reason | NO SCORE CLAIM: watermark-query cancellation is untested; supported bounded behavior is UNCERTAIN |
| Detached process | `Setsid: true` -> `Setsid: false` | Existing watchdog, fence-cancel, and delayed-overlay tests stayed green; no test asserts parent-exit survival | UNCERTAIN: detached survival has no proving test |

The watermark and transaction mutations are intentionally reported false-green, not treated as evidence. No public modernc SQLite raw-interrupt seam was invented. A canceled context does not prove SQLite interruption after a transaction has begun.

## Detached publication receipt

`spawnTagPublishChild` uses bare `exec.Command`, `setsid`, redirected `tag-publish.log`, `Start`, and `Process.Release`. The only terminal line is emitted by the child (`tag-publish: published ...`); the parent receives only `publication queued`. `TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication` uses an in-process fake goroutine and therefore does not prove a detached child survives parent exit or that a terminal receipt is durably observed. Final process exit/status and receipt: **not observed**. Verdict: **UNCERTAIN / NO SCORE CLAIM**.

## State and timing evidence

- Fence mutation: context cancellation returned only after restoring test cleanup; no fold transaction was admitted, and no final row/topic/watermark state changed.
- Child-context mutation: the canceled child reached schema/open/close after the fence was released; no merge rows or watermark were committed for the missing source.
- Begin/query mutations: no layer entry was reached, so SQLite result codes and final row/topic/watermark state are unavailable, not inferred.
- Race count: zero race reports in focused tests and full gate.

## Patch/base accounting

Candidate diff against parent: `+100/-5` across `internal/index/consolidated.go` and `internal/index/consolidated_test.go`; patch-id `4aef91de56b2e0c4756103ebedeae821f1570dec`.

Inherited cancellation patch-ids for comparison: `332af5d` = `56d606d49b2b4e1a727b1e42d352e31e2c9f37dd`; `00e587d` = `645d0cea4f622e854a26618606352c5c8697852d`. No range-diff/adoption is implied.

## Gates and readiness

`CGO_ENABLED=0 go test -race -count=1 ./...` passed all packages (elapsed about `77.696s` at `internal/index`, full command completed successfully; log SHA-256 `d9cb38...`). `git diff --check` passed. `gofmt -l internal/` was empty. Current-base readiness for existing tests: **GREEN**. Integrated cancellation-layer readiness: **UNCERTAIN**, with no score or merge authorization because transaction admission, watermark interruption, and detached terminal receipt remain unproven.

## Required response

- Direction Lock impact: technical evidence only; preserve the current candidate fence/child cancellation behavior, but do not score the untested layers.
- Challenge: add separate supported tests that first reach transaction admission and watermark statement execution, record `ctx.Err`, elapsed time, SQLite code, and final state; add a real parent-exit/child-terminal-receipt harness.
- Action: keep these claims marked UNCERTAIN/NO SCORE CLAIM until those tests exist and each one-fault mutation is red.
- Required response: owner must provide the exact bounded mechanism or explicitly accept the uncertainty; no combined cancellation test may be used as proof.

## Evidence hashes

- `mutation_fence.log`: `0d665d211bfd50503df5c046b836eb5ba3665bea182e2c6125b9d05924db125f`
- `mutation_begin.log`: `b16b7a8e310b7a3b8738899a47fa22c8b32e53d774f0a6be7300de3fa00d4cfc`
- `mutation_watermark.log`: `e778f5b0d686e14d70b8e67d80266ea53ba0d5ae313e930d5facb799e59b1286`
- `mutation_child_context.log`: `3ef2e36cd2ceb0c96f5fbb58a54defe19101a1bc4fc76abd36e722ccacdaf5db`
- `mutation_detach.log`: `da6603b8c8707a92159f69babeac116f98dc0ce71857cfc62b54915a9233d0b6`

