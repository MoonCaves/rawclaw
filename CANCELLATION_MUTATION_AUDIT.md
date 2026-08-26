# Consolidated sync cancellation mutation audit

Commit under test: `047a6def4f21c3279563a7aef2cc331b4ecb6b6d`

The exact filter `-run '^TestSyncConsolidatedFromContext$'` matched one test.

## Mutation results

### Fence context: killed

Mutation: replace `AcquireConsolidatedFence(ctx)` with
`AcquireConsolidatedFence(context.Background())`.

Observed command:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestSyncConsolidatedFromContext$'
```

Observed result: exit 1. The test waited for its two-second bound and failed:

```text
--- FAIL: TestSyncConsolidatedFromContext (2.05s)
    consolidated_cancel_test.go:35: SyncConsolidatedFromContext did not stop after cancellation
FAIL
real 2.76
```

The held fence was released after `2.051348666s`; the test therefore detects
cancellation delayed past the fence wait. The mutation was restored.

### BeginTx context: survived

Mutation: replace `con.BeginTx(ctx, nil)` with `con.Begin()`.

Observed result: exit 0; the same filter reported `ok ... 1.708s` (wall time
`3.17s`). This test does not reach the merge transaction because cancellation
is injected while the fence is held, so it cannot independently pin `BeginTx`.
The mutation was restored.

### QueryRowContext: survived

Mutation: replace the sync-watermark `con.QueryRowContext(ctx, ...)` with
`con.QueryRow(...)`.

Observed result: exit 0; the same filter reported `ok ... 1.622s` (wall time
`2.85s`). For the same reason, this test does not independently pin the
watermark query's context propagation. The mutation was restored.

## Verdict

`TestSyncConsolidatedFromContext` immutably proves cancellation while waiting
for the consolidated fence, but it does not prove cancellation during the
watermark query or merge transaction. The implementation still carries the
context into both paths; stronger phase-controlled tests would be needed to
turn those two surviving mutations into killed mutations.
