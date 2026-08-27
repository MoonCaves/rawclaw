# Tick 47 database-cancellation referee

Target: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` (exact base and candidate).
Report commit: `worker/furiosa-t47-db-cancel-referee-20260827`.
Source report reviewed: `74b45d90924a25657842d5b1060fecb01dd1d0ca`, SHA-256
`e743df829e507b4b00eb9182b70ef992bb1d4d99d099f85785d4c43bb350e08b`.

## Verdict

**REPRODUCED.** Cancellation is honored at the external consolidated-fence
wait, but not bounded once sync has entered the SQLite path. A deterministic
test held a real SQLite write transaction, seeded a changed source, started
`SyncConsolidatedFromContext`, and canceled after 50 ms. The operation did not
return by the 250 ms target: the bounded observation completed at 303.837 ms.
After the lock was released, the operation unwound with `context canceled`.

The post-release assertions showed no new session and no changed `sync:*`
watermark. Thus this probe found no publication after cancellation, but the
bounded-return contract is still violated.

## Evidence

Disposable test: `internal/index/tick47_db_cancel_referee_test.go` (created,
gofmt'd, run, then removed before the report commit).

Focused preflight:

```text
filter: ^TestTick47(DBCancelReachesSQLiteWriteWait|StampIngestWatermarkHasNoContext)$
result: listed exactly 2 tests; exit 0
output SHA-256: ccaf1f4894675725eef2a620c49c08776edb522a36a68c269e6f402be86dd75d
```

Focused race probe:

```text
command: CGO_ENABLED=0 go test -race -count=1 -timeout=12s -run '^TestTick47DBCancelReachesSQLiteWriteWait$' ./internal/index
result: FAIL (expected adversarial failure)
observed: cancel result=timeout elapsed=303.837125ms
post-release result=context canceled
test/process output SHA-256: cf7aa356fa53c8b6cb18dbbd5173be5b6ca91bb6587e5fe024461ed7d5dcc7d
```

The test's session and watermark checks ran after lock release and passed: the
consolidated session count and `sync:<source identity>` value remained at the
pre-probe values. The lock was always rolled back for cleanup.

Existing fence-only test:

```text
filter: ^TestSyncConsolidatedFromContext$
list result: exactly 1 test; exit 0
list output SHA-256: 2cfe313b6a0d5aebae2ddddc6277c7099fd7aec025337794faa4a17fac8e4df3
race result: PASS; test time 1.577s
test output SHA-256: 6f16b50ddf21715f272041bcc20a93b1521cd03fd84642d1543c13c9aac218e9
```

Full gate after disposable-test removal:

```text
CGO_ENABLED=0 go test -race -count=1 ./...
result: PASS
output SHA-256: a4bea5535dc217f94fa779283dcfb4ab06d16a99be07b0f0f5ee4bed0ed6783c
```

## Layer and seam inspection

`SyncConsolidatedFromContext` checks the context before and after setup, then
calls `consolidateOneContext`. That function begins a context-bound transaction
with `con.BeginTx(ctx, nil)`, but every fold statement uses `tx.Exec`, not
`tx.ExecContext`. The first SQLite write/lock wait is therefore not coupled to
the cancellation context. The deterministic lock holder demonstrated this.

`StampIngestWatermark` has signature `func(*sql.DB) error` and performs two
non-context `con.Exec` calls. It has no context seam. Sync calls it after the
fold and swallows its error after debug logging. Current tests call the same
non-context function directly or exercise the existing fence-only cancellation
test; none enters a context-aware watermark layer.

## Mutation verdict

The report's mutation conclusions stand:

1. Making `BeginTx` use `context.Background()` is not a sufficient mutation
   for this lock scenario because deferred SQLite transactions do not wait for
   the write lock at `BEGIN`; the wait occurs in later non-context statements.
2. A context/non-context watermark mutation is vacuous while
   `StampIngestWatermark` itself has no context argument.
3. Moving publication before commit is not distinguishable without a barrier or
   a transaction/publication seam.
4. Swallowing the watermark error requires fault injection to assert terminal
   failure and withheld success.

No product fix was implemented. The final branch contains only this report.

## Reproducibility and tree state

- Base/candidate HEAD before report commit: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`.
- Product diff before report: empty.
- Disposable Go test: removed before commit.
- `git diff --check`: clean.
- Final worktree: clean.
- Upstream ahead/behind: `0/0` after push.
