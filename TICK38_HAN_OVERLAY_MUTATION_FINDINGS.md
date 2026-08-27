# Tick 38 Han overlay/publisher mutation findings

Audit base: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`. This is a report-only
mutation audit. The four advertised tests were exact-listed before mutation:

1. `TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication`
2. `TestOverlayAuthoritativeTopicsReplacesSessionSet`
3. `TestOverlayAuthoritativeTopicsRemovesDeletedTopics`
4. `TestRunTagPublishChildHonorsCanceledContext`

The focused command matched four tests and passed under the candidate:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli \
  -run 'TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication|TestOverlayAuthoritativeTopics(ReplacesSessionSet|RemovesDeletedTopics)|TestRunTagPublishChildHonorsCanceledContext'
ok github.com/MoonCaves/rawclaw/internal/cli 1.999s
```

## Findings

| Layer | Result | Evidence |
|---|---|---|
| Detached child/process exit | UNCERTAIN | Existing `TestConsolidate_RetryAfterAbruptPostMergeExit` passed (`0.15s`, retry `37.078ms`) and proves fold retry after an injected child exit. It does not launch the detached `tag-publish` child or prove publication survives parent exit, terminal receipt ownership, or child cleanup. No new process-exit reproduction was run after the freeze instruction. |
| Cancellation while consolidated fence is held | CONFIRMED for candidate; mutation caught | One-line mutation `SyncConsolidatedFromContext` → `SyncConsolidatedFrom`, patch ID `22ddc47772a70eacfe9c03365921a19ead947854`, made `TestRunTagPublishChildHonorsCancellationWhileWaitingForFence` fail after the 300ms bound (observed package failure at `0.918s`; log showed the non-contextual fold continued after fence release). The restored candidate passed the four-test gate. |
| Transaction cancellation | UNCERTAIN independently | The candidate uses `BeginTx(ctx, nil)`, but no dedicated live transaction-lock cancellation test exists in the advertised four. A temporary canceled-context reproduction was run only with a combined mutation; it returned nil under the mutation, proving the combined guard can be lost but not isolating transaction cancellation. |
| Watermark-query cancellation | UNCERTAIN independently | The candidate uses `QueryRowContext` for the sync watermark. No dedicated blocked watermark-query reproduction was run. The combined mutation replaced both `QueryRowContext` and `BeginTx(ctx,nil)` with non-context forms; temporary test `TestTemporaryConsolidateOneContextRejectsCanceledWork` failed with `error = <nil>` after about `0.08s`. Combined mutation patch ID: `3e319ed5953434260642a2f963d8a8fd130cd673`. |
| Deleted-topic replacement | CONFIRMED for candidate; mutation caught | One-line mutation changed `overlayAuthoritativeTopics` to append derived and authoritative rows, patch ID `a1484537f68b5812a558991271335e3652d9aaf8`. `TestOverlayAuthoritativeTopicsDropsDeletedDerivedRows` failed immediately with both stale and authoritative rows present. Candidate implementation returns a copy of the authoritative replacement set. |
| Co-contributor preservation | CONFIRMED narrowly; advertised test is mutation-blind for the tested SQL branch | The candidate’s `TestConsolidate_PreservesTopicsWhenCoContributorRemains` passed in the baseline (`0.25s`). Mutation `NOT EXISTS` → `EXISTS`, patch ID `c760ffcfd668fc431ed21285641bc073b2fd71d3`, also passed. The later merge from the co-contributor reinserted its topic, so this test does not prove the prune query itself cannot over-delete. The candidate’s behavior is confirmed by final state, but the specific guard is not independently pinned. |

## Ordering and restoration

The fence-cancellation mutation exceeded the 300ms deadline before the held
fence was released; after release, the fold continued and completed. The
overlay mutation failed immediately. The combined transaction/watermark
mutation completed the canceled temporary path and returned nil in about
0.08s. The co-contributor mutation completed in about 0.25s and remained
green, which is the semantic-blindness result above. The existing process-exit
test completed its retry in 37.078ms after the child exited with status 124.

All disposable edits were restored byte-for-byte: `git diff` is empty for
`internal/cli/tagpublish.go`, `internal/cli/tagrefresh.go`,
`internal/index/consolidated.go`, and all temporary test changes were removed.
No product file, shared harness, mailbox, scorecard, or Direction Lock was
changed. No adoption or score inference follows from this report; no merge
authorization is granted.

## Gate boundary

Observed green gates are the exact four-test candidate CLI gate and the
existing focused index tests for process-exit, co-contributor preservation,
and unchanged sole-source topics. The mutation gates were intentionally red or
false-green as recorded above. A full repository race gate was not run in this
freeze window. Therefore the untested detached-child process-exit,
independent transaction cancellation, and independent watermark-query
cancellation layers remain **UNCERTAIN**, not PASS.

Report-only commit is limited to this file. Required handoff checks are a
clean worktree, upstream divergence `0 0`, and this report’s SHA-256.
