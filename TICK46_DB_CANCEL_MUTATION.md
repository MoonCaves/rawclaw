# Tick46 database-cancellation mutation adversary

Target: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` (`fix(index): prune only missing source topics`).
Branch: `worker/furiosa-t46-db-cancel-mutation-20260827`.

## Verdict

| layer | verdict | evidence |
| --- | --- | --- |
| fence admission | GREEN for the existing fence-only test | `TestSyncConsolidatedFromContext` cancels while `AcquireConsolidatedFence` polls and returns `context.Canceled` in bounded time |
| DB transaction admission / first write | RED / missing invariant | a real SQLite writer lock held the consolidated DB; cancellation after sync entered the DB path did not return within 250 ms while the first write waited. It unwound only after the lock was released |
| active watermark statement | UNCERTAIN as a cancellable operation; seam absent | `StampIngestWatermark(*sql.DB)` has no context argument and uses `con.Exec`; canceling an unrelated context cannot interrupt its active statement |
| rollback | GREEN only for ordinary statement errors | existing `TestConsolidate_RollsBackAMidFoldFailure` proves trigger-induced error rollback, not cancellation during a blocked DB statement |
| publish-after-commit | RED risk / not proven | `SyncConsolidatedFromContext` calls `StampIngestWatermark(con)` after commit, but swallows its error; there is no context-aware publication contract |

## Exact probes

I created and then removed disposable `internal/index/t46_db_cancel_mutation_test.go`.
It contained exactly two tests:

* `TestT46DBAdmissionCancelMutation`: seed a source, hold a write transaction on the consolidated DB, start `SyncConsolidatedFromContext`, cancel after the fold reaches the DB path, require bounded `context.Canceled`, and assert no session or `sync:*` watermark commit.
* `TestT46DBActiveWatermarkCancelMutation`: hold a SQLite write lock while `StampIngestWatermark` is the active operation, cancel a context, then release the lock and observe that the non-context watermark operation completes and publishes.

`go test ./internal/index -list '^TestT46DB(AdmissionCancelMutation|ActiveWatermarkCancelMutation)$'` listed exactly two matches and exited 0. The disposable tests were gofmt'd before execution.

Observed focused runs (task-local `HOME` and `GOCACHE`, `CGO_ENABLED=0`, `-race -count=1`):

* admission probe: FAIL after about 1.19 s process time; `cancellation did not bound return while first write was lock-waiting`; log SHA-256 `7e2ecbaca9e82d56fd709abe19ecab908f5da4ea7e489a6900c35898dfcb2b50`.
* watermark probe did not produce a completed race result before the disposable process was stopped during dependency compilation; its partial log SHA-256 was `844039526899f2b12ea640c3773d96e892a939ed131142d9a9f93fdcf04083eb`.

Official existing test: `go test ./internal/index -list '^TestSyncConsolidatedFromContext$'` listed exactly one match (SHA-256 `cf766a5404c7f1b70191ce7f4be75b45c12ad05da0d2df6079a6a7829f12b6ce`). Focused `CGO_ENABLED=0 go test -race -count=1 -run '^TestSyncConsolidatedFromContext$' ./internal/index` passed in 1.55 s test time / 26.95 s wall process time (dependency compilation included); log SHA-256 `fa345842ccc24e991ab74d6ca6592c1b05ed4f70fff52f4a22b4328564afb5ca`.

## Mutation analysis

The requested mutations cannot be made into valid red/green tests with the current public seams:

1. Replacing `con.BeginTx(ctx, nil)` with `con.BeginTx(context.Background(), nil)` is not observable at transaction admission because SQLite's default deferred `BEGIN` does not wait for a write lock. The first lock wait occurs later in `tx.Exec`, and those calls are non-context (`tx.Exec`, not `tx.ExecContext`). A test that cancels before `BeginTx` only exercises the already-covered pre-layer guard and gives a false green for this mutation.
2. Replacing watermark `Exec`/`QueryContext` with background/non-context is vacuous today: `StampIngestWatermark` already has no context and uses non-context `Exec`. A deterministic active-statement test therefore needs a context-bearing publication seam.
3. Publishing the watermark before commit cannot be distinguished by a test until publication is represented as part of the same context-aware transaction or a fault barrier exists between publication and commit.
4. Swallowing cancellation/error in the post-fold watermark call is currently observable only as silent success, because the caller logs at debug and returns nil. This requires a fault-injection seam for the watermark operation plus an assertion that sync returns an error and withholds terminal success.

Smallest required production seams (report-only, not implemented):

* inject a test-only barrier immediately after `BeginTx(ctx, nil)` and use `tx.ExecContext` for every fold statement that can wait on the database;
* change watermark stamping to `StampIngestWatermarkContext(ctx, con)` using `ExecContext`, return its error from sync, and place its publication inside the transaction whose commit establishes success; or add an explicit context-aware post-commit publication contract with no terminal-success claim on cancellation.

Do not claim modernc raw SQLite interrupt support: this run found no such supported seam. The evidence is limited to `database/sql` context admission plus observed SQLite lock behavior.

## Candidate payload and ancestry

Direct candidate parent-to-candidate payload: 2 files, 100 insertions and 5 deletions (`internal/index/consolidated.go`, `internal/index/consolidated_test.go`); stable patch-id `4aef91de56b2e0c4756103ebedeae821f1570dec`.

Whole ancestry `0d1da19c..8e9c9b7`: 12 files, 742 insertions and 17 deletions; stable patch-id `489776deb8f3f3f0df46d9b3998109af3c54c264`.

Path ancestry stable patch-ids:

* `internal/index/consolidated.go`: `47c95d16d69178ee2607acf99f695df534ba08c6`
* `internal/index/consolidated_test.go`: `a7e8b0567a4f530c7473bf00b6447f50e82b54d4`

`git range-diff 0d1da19c..8e9c9b7 0d1da19c..8e9c9b7` is identity: all 16 commits map 1:1. This is ancestry context, not evidence that cancellation layers are covered.

## False-green risk

HIGH. The current green test cancels before source existence/schema/transaction admission because it holds the external consolidated fence. It proves fence polling only. A cancellation before `BeginTx`, or cancellation after a non-context `tx.Exec` has already begun, can leave the test green while a lock wait, watermark publication, or terminal-success path remains uncancelable. The candidate must not receive a cancellation-complete verdict from the existing green.

No product code or disposable Go test remains modified. Only this report is intended for commit.
