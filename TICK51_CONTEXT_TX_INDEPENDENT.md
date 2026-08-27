# Tick51 context transaction independent adjudication

## Scope and identity

- Checkout: `rawclaw-furiosa-t51-context-tx-independent`
- Branch: `worker/furiosa-t51-context-tx-independent-20260827`
- Exact base and HEAD before work: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`
- Product integration: none. The only temporary product hypothesis changed the
  fold statements in `consolidateOneContext` from `tx.Exec` to `tx.ExecContext(ctx, ...)`.
- Temporary test: `internal/index/tick51_context_tx_independent_test.go`.
- Permanent file: this report only.

## Graphify and method preflight

Commands run before source inspection:

```text
mnemon --store rawclaw recall 'rawclaw consolidated SyncConsolidatedFrom context transaction cancellation Tick51' --limit 5
graphify reflect --if-stale
graphify explain 'SyncConsolidatedFrom'
graphify query 'SyncConsolidatedFrom ExecContext tx Exec fold transaction' --budget 4000
```

The first Graphify explain/query reported `error: graph file not found`. After
`graphify update .`, the retry succeeded. It located
`SyncConsolidatedFrom` at `internal/index/consolidated.go:554`, its call to
`SyncConsolidatedFromContext`, `AcquireConsolidatedFence`,
`consolidateOneContext`, `StampIngestWatermark`, and the Tick51 test node.

The required AGENTS.md chain and method files were read: graphify,
golang-how-to, golang-database, golang-context, golang-concurrency,
golang-testing, golang-troubleshooting, golang-error-handling, and
golang-security.

## Symmetric red/green experiment

The test-only hook was temporarily added to pause immediately after
`BeginTx(ctx, nil)` and before the first fold statement. The test then acquired
`BEGIN IMMEDIATE` on another connection, released the hook, canceled the
context, and required prompt `context.Canceled` plus unchanged consolidated
rows and watermark.

Red, baseline product behavior (baseline code plus the test harness hook):

```text
exact-one list:
TestTick51ContextTxIndependent
ok github.com/MoonCaves/rawclaw/internal/index 0.566s

CGO_ENABLED=0 go test -race -count=1 -timeout 2s -run '^TestTick51ContextTxIndependent$' ./internal/index
--- FAIL: TestTick51ContextTxIndependent (0.40s)
    ...: sync error = clear affected session set: sql: transaction has already been committed or rolled back, want context.Canceled
goleak: found unexpected goroutines:
... database/sql.(*DB).connectionOpener ...
FAIL
```

Green, one hypothesis only (`tx.Exec` -> `tx.ExecContext(ctx, ...)` for all
fold transaction statements):

```text
exact-one list:
TestTick51ContextTxIndependent
ok github.com/MoonCaves/rawclaw/internal/index 1.215s

CGO_ENABLED=0 go test -race -count=1 -timeout 5s -run '^TestTick51ContextTxIndependent$' ./internal/index
ok github.com/MoonCaves/rawclaw/internal/index 2.164s
```

Verdict: changing the fold transaction's `tx.Exec` calls to
`tx.ExecContext(ctx, ...)` is sufficient for the tested lock-held cancellation
case. It caused the blocked statement to return through the canceled context,
the test observed `context.Canceled`, and the preexisting session and watermark
remained unchanged.

## Hook and baseline judgment

The test-only hook is a deterministic synchronization seam, not production
behavior. It is acceptable as a focused harness aid because it is nil by
default and was restored with the product file; it does not prove that a
production caller naturally reaches the same exact interleaving. The test also
does not assert that the `ready` hook was observed before proceeding, so that
timing contract is weaker than ideal.

The red run is symmetric in behavior but not a literal clean-base test: exact
base `8e9c9b7` has neither the temporary test nor the temporary hook. The red
baseline therefore means base production logic plus the minimum test-only hook
needed to run the same harness. It must not be described as an exact-base
checkout test. The green run uses the identical harness and changes only the
transaction execution API, so the red/green delta isolates the hypothesis.

## Unproven layers and transaction boundary

The focused test begins after the context-aware fence and consolidated schema
setup. These remain outside the demonstrated cancellation boundary:

- Schema/healing/topic/session-source setup in
  `SyncConsolidatedFromContext` (`internal/index/consolidated.go:587-604`)
  uses non-context `DB` calls and is only followed by `ctx.Err()` checks.
- Source migration and `ATTACH DATABASE`/`DETACH DATABASE` in
  `consolidateOneContext` (`:693-716`) use non-context calls.
- The fold's pre-transaction source inspection and `migrateSessionSources`
  (`:698-803`) use non-context calls except the sync-watermark lookup.
- Tombstone pruning (`:619-624`, `:1188-1257`) starts its own `con.Begin()`;
  its transaction statements are non-context and were not tested here.
- `StampIngestWatermark` (`:625-629`, `:1357-1371`) uses `DB.Exec` after the
  fold transaction has committed. Its errors are deliberately best-effort,
  and cancellation is not propagated into it.
- Transaction-bound publication is not proven end-to-end. The tested fold
  transaction includes the merge and sync watermark (`:812-938` in the
  temporary mutation), but post-fold prune and watermark stamping are separate
  operations. The report does not claim atomic publication across those layers.

The existing context-aware fence wait and `BeginTx(ctx, nil)` were not changed.
The experiment proves only interruption of a blocked statement executed through
the fold transaction using the modernc SQLite driver's context path.

## Restoration and patch identity

Before restoration, the temporary combined tracked diff had:

```text
consolidated_worktree_sha256=8cac7138dfb263436ab287263bcb583e54328c2cf1870e72f74a594cc65056cc
tick51_test_sha256=9a364b513b5082667d849846cfb8c606e907eb5cd3552cd221dd0cbbcd4736b4
combined_worktree_diff_sha256=bb44372fc3fb89c70fdda52b830b4be7e0f6236b27c6f1a0313ae95b80f20c09
combined_patch_id=45454e5efb1fb2263b95c228f45cfcbaacfad5b9
```

Restoration commands and observed output:

```text
cmp -s <(git show 8e9c9b7:internal/index/consolidated.go) internal/index/consolidated.go
consolidated.go: byte-identical to base 8e9c9b7
tick51_context_tx_independent_test.go: absent (restored to base state)
consolidated.go: no tracked diff
```

The temporary Go files were formatted with `gofmt -w` before each Go test while
they existed. No Go file is included in the final commit.

## Final gate status

This lane's focused red and green gates are observed above. Full repository
race/build gates were not run because this lane is an independent temporary
mutation adjudication and does not integrate product code; any such unobserved
gate is `UNCERTAIN`, not green.

Final report commit and push receipts are recorded in the handoff message.
