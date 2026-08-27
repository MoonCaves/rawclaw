# T53 current-base cancellation referee

Date: 2026-08-27 WITA  
Base: `c818ea1212bb1f1110cefa65472f658b844840ef` (`origin/main`)  
Branch: `worker/furiosa-t53-current-base-cancel-20260827`

## Verdict

**CONFIRMED: the current base has a first-write cancellation gap.**

`SyncConsolidatedFrom` has no context parameter and calls `consolidateOne` after
`AcquireConsolidatedFence(context.Background())`. `consolidateOne` uses deferred
`con.Begin()`, so transaction admission returns before its first write. The first
`tx.Exec` waits on SQLite's configured 10-second busy timeout when an independent
SQLite writer holds the database lock. No caller cancellation can reach this
wait on the current-base API.

The bounded-cancellation error contract (`context.Canceled` or
`context.DeadlineExceeded`) is therefore **not testable against the untouched
base**: `SyncConsolidatedFromContext` does not exist at this base. A context-aware
candidate was identified independently as `0cd0b9ce77e362b5bd4e973f948eb9981cdbf452`
(`fix(index): bound consolidated writer cancellation`), but it was not merged or
used as current-base evidence.

## Candidate interleaving and implementation verdict

The candidate's test does not exercise cancellation after SQLite admission. Its
ordered interleaving is:

1. The test acquires `consolidatedWriterGate` itself.
2. The test starts `SyncConsolidatedFromContext` with the gate already occupied.
3. `SyncConsolidatedFromContext` waits in `acquireConsolidatedWriter(ctx)` and
   cancellation returns `context.Canceled` from that process-local admission
   wait.
4. Only separately, the test's independent SQLite transaction holds the
   consolidated database writer lock; the canceled fold has not reached SQLite.
5. Releasing the gate and SQLite transaction permits the retry to fold and stamp
   its watermark.

Commit `7d1ca1c643795a145db1e33d657192993ff8fd78` removes the unsupported direct
modernc busy-wait cancellation probe and records this admission boundary. Thus
`0cd0b9c`/`7d1ca1c` do not establish cancellation of an already-admitted SQLite
write or driver busy wait. **Implementation direction: UNCERTAIN.** The candidate
is not recommended on this evidence; a context-aware mutation must independently
gate entry past admission before it can validate that claim.

The current-base reproduction below intentionally bypasses that candidate gate:
the untouched base has no `consolidatedWriterGate` and no context-aware API. It
holds only an independent real SQLite writer lock, starts the non-context-aware
`SyncConsolidatedFrom`, and observes the first-write wait, non-publication, then
release-and-retry commit. That validates the current-base first-write gap, but it
does not validate the candidate's gate-bypassing or SQLite-cancellation behavior.

## Deterministic lock-held reproduction

Temporary test: `internal/index/tick53_current_base_cancel_test.go`  
Test: `TestTick53_CurrentBase_FirstWriteWaitsPastCancellationTarget`

The test seeded and initially folded one source, added one changed source row,
then held an independent write transaction on the consolidated database. It
started `SyncConsolidatedFrom` and used a 250 ms cancellation target as an
observation boundary. The test then released the real SQLite lock and required
the same call to finish, distinguishing first-write wait from admission deadlock
or a leaked goroutine.

Observed run:

```text
go test ./internal/index -list '^TestTick53_CurrentBase_FirstWriteWaitsPastCancellationTarget$'
TestTick53_CurrentBase_FirstWriteWaitsPastCancellationTarget
ok  github.com/MoonCaves/rawclaw/internal/index  0.467s
exact_one_count=1

time go test -race -count=1 ./internal/index -run '^TestTick53_CurrentBase_FirstWriteWaitsPastCancellationTarget$' -v
... sync remained blocked for 251.044792ms while SQLite write lock was held
--- PASS: TestTick53_CurrentBase_FirstWriteWaitsPastCancellationTarget (0.50s)
PASS
ok  github.com/MoonCaves/rawclaw/internal/index  2.029s
real 2.89s
```

While blocked, the consolidated store showed zero `after` sessions and zero
`after` messages. The prior `sync:%` watermark remained present. After lock
release, the call returned successfully; exactly one `after` session and message
were visible, and both the sync watermark and `last_ingest_time` advanced.

This is an honest first-write result, not a cancellation pass: the current-base
function cannot return a context error because it cannot receive a context.

## False-green and asymmetry checks

- The holder was a real SQLite transaction obtained through a separate
  `store.ConnectRW` connection, not `consolidated.lock` or a test-only writer
  gate. This enters the database lock layer.
- The test changed the source watermark before the second fold, forcing the
  fold past source reads and into the transaction.
- The test observed the blocked store through a separate read-only connection
  and checked rows plus the existing watermark before releasing the holder.
- Releasing the holder and requiring completion proves the wait was not merely
  transaction admission and that the operation can commit normally.
- No production hook, sleep-based production seam, or asymmetric baseline was
  added. The 250 ms timer is only the bounded observation target; it does not
  cause the operation to return.
- The candidate `0cd0b9c` and correction `7d1ca1c` were inspected exactly. Their
  test cancels while `consolidatedWriterGate` is occupied, before SQLite; no
  candidate behavior is reported as current-base behavior.

## Source and command receipts

Relevant current-base signatures:

```text
internal/index/consolidated.go:534  func SyncConsolidatedFrom(srcPath string) error
internal/index/consolidated.go:612  func consolidateOne(con *sql.DB, src string) (...)
internal/index/consolidated.go:1235 func StampIngestWatermark(con *sql.DB) error
internal/index/consolidated_fence.go:34 func AcquireConsolidatedFence(ctx context.Context) (...)
```

Graphify was refreshed with `graphify update .` and reported 3,408 nodes,
10,157 edges, and 160 communities. `graphify explain` connected
`SyncConsolidatedFrom` to `consolidateOne`, `AcquireConsolidatedFence`, and
`StampIngestWatermark`; the vocabulary query showed the fold's `Begin`, writes,
commit, and watermark path. The local graph initially did not exist, so the
AST-only refresh was required before the symbol queries.

Exact candidate identity check:

```text
0cd0b9c fix(index): bound consolidated writer cancellation
commit 0cd0b9ce77e362b5bd4e973f948eb9981cdbf452
```

Temporary Go-file restoration:

```text
base consolidated blob: 9ee0376652d12fec9b7099fd25f5024dd6eb75f1
working consolidated blob: 9ee0376652d12fec9b7099fd25f5024dd6eb75f1
temporary test blob: fa01d42df2434e9e6004099ce44cd678747daaa4
```

The temporary test is removed before commit. No product integration or merge
authorization is implied by this report.
