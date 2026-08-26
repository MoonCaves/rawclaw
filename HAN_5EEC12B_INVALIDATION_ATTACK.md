# Han invalidation attack against `5eec12b`

## Verdict

Both Furiosa claims are CONFIRMED on the exact candidate. Product readiness is
REOPENED / UNCERTAIN until the integration branch supplies context propagation
through the fold and a full authoritative replacement overlay.

## Claim 1: detached child cancellation

`internal/cli/tagpublish.go:68-78` accepts `ctx` but line 73 calls
`index.SyncConsolidatedFrom(dbp)`, which creates `context.Background()` before
waiting on the consolidated fence. The deterministic test
`TestRunTagPublishChildHonorsCancellationWhileWaitingForFence` holds the fence,
cancels the child before launch, and requires return within 300 ms. Candidate
result: FAIL after 300 ms; after releasing the fence the call continues into
the fold and returns only afterward. This proves the 25-second child deadline
cannot cancel an in-flight/non-contextual fold and gives no terminal canceled
receipt.

The independent evidence in `cc3e088` is narrower: it kills the fence-context
mutation at about 2.05 s, but `BeginTx(ctx)->Begin()` and
`QueryRowContext->QueryRow` mutations survive. Therefore a future fix must be
judged in layers: child timeout, fence cancellation, transaction cancellation,
and watermark-query cancellation are separate claims.

## Claim 2: deleted topic survives overlay

`internal/cli/tagrefresh.go:128-143` copies all derived rows, replaces matching
keys, and appends authoritative rows. It never removes derived keys absent from
the authoritative set. `TestOverlayAuthoritativeTopicsDropsDeletedDerivedRows`
seeds one retained and one deleted derived segment, then supplies only the
retained authoritative segment. Candidate result: FAIL because the deleted
segment remains. This is a direct stale-topic leak after delayed retag
publication.

## Mutation proof

The overlay assertion was mutated to expect two rows (replacement plus stale
derived row); focused race test passed, demonstrating a false-green assertion.
It was restored. The cancellation timeout failure was mutated from `Fatal` to
`Log`; focused race test passed despite the canceled child taking beyond the
300 ms bound. It was restored.

## Gates

Focused command:

```text
go test -race -count=1 ./internal/cli -run 'TestRunTagPublishChildHonorsCancellationWhileWaitingForFence|TestOverlayAuthoritativeTopicsDropsDeletedDerivedRows'
```

Observed candidate result: both tests FAIL, cancellation at ~0.39 s total
with the required 300 ms bound exceeded, overlay immediately with stale row.

Broader gate `go test -race -count=1 ./internal/cli ./internal/index` ran to
completion in about 103 seconds: `internal/index` PASS; `internal/cli` FAIL
only on these two intentional red proofs (the package reported about 101 s).

No production files were changed. New red fixture only:
`internal/cli/invalidation_attack_test.go`.
