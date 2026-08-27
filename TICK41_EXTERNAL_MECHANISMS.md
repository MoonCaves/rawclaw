# Tick 41 external-mechanism delta

- `run_timestamp`: `2026-08-27T02:06:10Z`
- `completion_utc`: `2026-08-27T02:08:42Z` (captured with `date -u` after commit)
- `base`: `ef2eebf414e77086be06281539c5a50ba036a32a`
- `prior_watermark`: `20260827T020554Z` (supervisor-confirmed processed Tick 42 boundary)
- `new_watermark`: `20260827T020554Z` (no later valid receipt read; mailbox intentionally untouched)
- report-only; no product, ledger, mailbox, cursor, or graph output changed

Graphify orientation found `SyncConsolidatedFrom -> AcquireConsolidatedFence -> consolidateOne -> StampIngestWatermark`.

## New exact mechanisms

### PA-GO-CONTEXT-WRITER-TOKEN-001

- URL/title: <https://github.com/jackc/pgx/blob/v5.7.6/pgxpool/pool.go> — `pgxpool.Pool.Acquire` / `AcquireFunc`
- immutable revision: tag `v5.7.6`, commit `a2fca037434a0a7096b095d4ed87cdffb03b626e`; date unavailable; accessed `2026-08-27T02:00:00Z`
- mechanism: `Acquire(ctx)` selects capacity versus `ctx.Done()`; cancellation is admission failure; `AcquireFunc` scopes release.
- fit/blocker: pure-Go shape before `BeginTx`; a one-token buffered channel preserves it without dependency and prevents modernc busy waiting after admission. It cannot replace the cross-process fence; every direct writer must use it.
- ruling: **NARROWED comparator, score 0**. Use a stdlib token `select` before every consolidated writer; retain `AcquireConsolidatedFence`; do not add pgx or `x/sync`.
- normalized text: `Place one context-aware admission gate before any SQLite write: pgxpool Acquire(ctx) blocks only until ctx cancellation, then return incomplete without entering SQLite; retain cross-process fencing.`
- fingerprint: `3ab8baf8ea330608ca48fdfb7d5167cfc47c4152fefd566b989385811b2a2afc`

### PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001

- URL/title: <https://sqlite.org/c3ref/interrupt.html> — `Interrupt A Long-Running Query`
- immutable revision: SQLite documentation page; Fossil revision unavailable; accessed `2026-08-27T02:00:00Z`
- mechanism: `sqlite3_interrupt()` returns `SQLITE_INTERRUPT`; interrupted INSERT/UPDATE/DELETE in an explicit transaction rolls back that transaction. It may race completion.
- fit/blocker: exact detached-maintenance rule: cancellation is incomplete and watermark/receipt advances only after commit. modernc v1.45.0 exposes no supported public raw-connection interrupt seam; interrupt does not cancel busy admission or create retry ownership.
- ruling: **CONFIRMED transaction semantics, score 0**. Treat cancellation, `SQLITE_INTERRUPT`, and pre-commit death as incomplete; only commit publishes. No new implementation until a public modernc seam exists.
- normalized text: `Use SQLite sqlite3_interrupt as a cancellation boundary for detached maintenance: interruption returns SQLITE_INTERRUPT and rolls back transaction writes; publish watermark or terminal receipt only after commit.`
- fingerprint: `a78ec1938f09271ba45faa1cc46023d6ad9741d276fb993b33518c394616d6c5`

### SQLite benchmark verification (duplicate of PA-SEMANTIC-BENCH-COUNTER-001)

- URL/title: <https://sqlite.org/src/raw/test/speedtest1.c?ci=trunk> — SQLite `speedtest1.c`
- immutable revision: Fossil trunk endpoint; revision unavailable; accessed `2026-08-27T02:01:00Z`
- mechanism: verification mode (`bVerify`) records deterministic result bytes/hash and calls `fatal_error` on mismatch, separating semantic correctness from timing.
- fit/blocker: seed a fixture, assert expected post-prune aggregate/non-zero work, then report latency. C source and result hashing do not alone prove minimum work.
- ruling: **CONFIRMED validity precedent, duplicate, score 0**. Enrich `PA-SEMANTIC-BENCH-COUNTER-001`; no second ID.
- source-derived fingerprint (not scored): `4f71c3671391e21c59bd3f94801797f5eba78ab829b449633f1d64399e278eea`

## Cumulative re-grade and scoring

No post-watermark adoption receipt was read. Existing statuses remain: BEGIN IMMEDIATE narrowed 0; interrupt-BEGIN rebutted/narrowed 0 (statement cancellation only); progress budget narrowed 0; busy timeout duplicate 0; busy-handler deadline narrowed comparator 0; semantic benchmark confirmed validity-only/unadopted 0; WAL, FTS5 deletemerge, singleflight, atomic commit, UPSERT, CAS, outbox, Kafka/NATS/Git/systemd/S3/Temporal terminal families pending/partial 0; weighted semaphore rejected 0; sidecar-prune externally adopted and technically locked.

Score delta `0`; totals Furiosa `+9`, Han `+2`, Ozzy `+3`. External precedent alone scores zero. pgxpool is not RawClaw adoption; SQLite C docs are not modernc API evidence. The benchmark source is duplicate by semantic mechanism/adopter target. No score event.

## Direction Lock and next leads

`PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically `LOCKED` on its recorded base/candidates/selector/gates/adopter receipt. No invalidation and **NO MERGE AUTHORIZATION**.

1. Mutation-test every consolidated writer bypass against a stdlib token; prove cancellation before SQLite entry under 200 ms.
2. Separate lock-admission cancellation from executing-query cancellation; busy timeout expiry is not context-bounded admission.
3. Make the semantic aggregate guard permanent against no-op, zero-live-ID, and skipped-`session_verdict` mutations.
4. Test process death before/after commit and watermark-query cancellation; publish terminal state only from committed transaction state.

## Validation

Report-only; Go formatting N/A; no product gates or green build claimed. No mailbox/cursor read or modified. No merge authorization.
