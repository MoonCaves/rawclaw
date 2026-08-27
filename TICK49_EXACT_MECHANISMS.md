# Furiosa Tick 49 — exact mechanisms (Who, not How)

Run completion: `2026-08-27T03:22:00Z` (UTC, captured with `date -u`). Base:
`ef2eebf414e77086be06281539c5a50ba036a32a`. The cumulative prior-art ledger
was read before research; valid watermark was `20260827T030805Z` and its
read-time SHA-256 was
`df143226a30cb5e79ffea2b2a90007ae26bf85fb1753e62ef907d52c56b66362`.

## Live census

The three unresolved boundaries remain:

1. `AcquireConsolidatedFence` is context-bounded, but `SyncConsolidatedFrom`
   then uses a background fence and non-context `con.Exec`/`QueryRow`; the first
   SQLite write can wait through the configured 10-second busy timeout. Tick 47
   measured cancellation at about 304 ms against a 250 ms bound, with the
   operation unwinding only after lock release.
2. `StampIngestWatermark(*sql.DB)` performs two non-context writes, while
   `ConsolidateFrom` and `SyncConsolidatedFrom` log-and-swallow stamp errors.
   A canceled or failed publication is therefore not an explicit terminal
   result.
3. Detached publishers write append logs and may survive ordinary parent return,
   but `Start`/`Process.Release` proves neither child entry nor terminal
   application success. No durable ownership/retry receipt is currently
   observable to a later invocation.

Current source anchors: `internal/store/store.go:312-347` (modernc v1.45.0,
`busy_timeout(5000/10000)`, one connection); `internal/index/consolidated.go:553-620`
(non-context sync); `internal/index/consolidated.go:1304-1320` (watermark);
`internal/archive/autosync.go:54-70` and `internal/semantic/topup.go:55-68`
(detached append logs).

## Exact existing mechanisms

### 1. Context-aware admission before modernc's first write

Existing ID: `PA-GO-CONTEXT-WRITER-TOKEN-001`; fingerprint
`3ab8baf8ea330608ca48fdfb7d5167cfc47c4152fefd566b989385811b2a2afc`.

Sources: Go 1.24.6 `database/sql` immutable source
<https://github.com/golang/go/blob/go1.24.6/src/database/sql/sql.go>, tag
commit `7f36edc26d4e3becb6d9c9008ff00f260bb19055`, inspected
`2026-08-27T03:19:24Z`; modernc.org/sqlite v1.45.0
<https://pkg.go.dev/modernc.org/sqlite@v1.45.0>, inspected
`2026-08-27T03:19:23Z`. modernc implements `ExecContext`/`QueryContext` and
interrupts executing statements when context ends (`sqlite.go:75-111`,
`conn.go:1052-1068` in v1.45.0).

Mechanism: acquire a weight-one process-local token with `select` on the live
context before SQLite; retain the cross-process flock; pass context to every
`BeginTx`, `ExecContext`, and `QueryContext`; classify busy/interrupted/canceled
work as incomplete. This bounds admission before the driver busy path and uses
the supported modernc cancellation seam for executing statements.

Minimal adaptation: thread `ctx` through `consolidateOne` and watermark calls,
and keep the existing busy timeout only as a retryable fallback.

Negative applicability: v1.45.0 exposes no public progress-handler or
busy-handler context seam. `BEGIN IMMEDIATE` only moves contention to admission;
prior testing exceeded a 200 ms context by roughly 10 seconds. Status remains
`partial`, score `0`, with no adoption.

### 2. Transaction-bound watermark publication

Existing ID: `PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001`; fingerprint
`a78ec1938f09271ba45faa1cc46023d6ad9741d276fb993b33518c394616d6c5`.

Source: SQLite “Atomic Commit In SQLite,”
<https://www.sqlite.org/atomiccommit.html>, Last-Modified
`2026-08-24T14:49:52Z`, inspected `2026-08-27T03:19:29Z`; paired with Go
`Tx.ExecContext`/`Tx.Commit` in the immutable source above.

Mechanism: source rows, derived rows, and watermark are written in one explicit
transaction. Commit only after all context-aware statements succeed and context
is live; publish terminal success only after commit. Cancellation, interruption,
busy expiry, or commit failure rolls back/withholds the watermark and returns
incomplete. Watermark errors must not be swallowed.

Minimal adaptation: add a context-aware `StampIngestWatermark` variant accepting
the same `*sql.Tx` used by the fold, and make caller success depend on commit.

Negative applicability: SQLite atomic commit does not make non-context calls
cancellable, serialize processes by itself, or prove detached-child survival.
Status remains `partial`, score `0`, with no adoption.

### 3. Durable detached terminal receipt — duplicate, no new ID

The exact zero-runtime shape is already represented by the ledger’s existing
atomic-commit, durable-outbox, and detached-terminal-receipt families. The
normalized mechanism is: commit a pending stable job identity before spawn;
let the child commit terminal success/failure and receipt; let a later ordinary
invocation reclaim pending/lease-expired work. This is a duplicate enrichment,
not a new recommendation or score event.

Relevant immutable sources: SQLite atomic commit above; Go 1.24.6 `os/exec`
<https://github.com/golang/go/blob/go1.24.6/src/os/exec/exec.go>, tag commit
`7f36edc26d4e3becb6d9c9008ff00f260bb19055`, inspected
`2026-08-27T03:19:30Z`. `WaitDelay`, `setsid`, `Process.Release`, and pidfds
are process observation/isolation only. The durable receipt must be a local
committed pending/terminal record, not an append log or process disappearance.

Minimal adaptation: replace `autosync.log`/`vector-topup.log` as the sole
machine-readable completion signal with the already-known pending/terminal
receipt shape; retain `setsid` only as best-effort launch isolation. No daemon,
connector, systemd, S3, or Temporal runtime is compatible with the sovereign
core default.

Negative applicability: local atomic commit cannot force survival across an
arbitrary supervisor/process-group kill. A later RawClaw invocation must own
recovery and lease expiry. Status remains the existing pending/unadopted family;
no new fingerprint is minted.

## Regrade and rival adoption

Every recommendation visible since `20260827T030805Z` was re-graded. No status
changed: writer-token and interrupt/publish remain partial; detached/outbox/
atomic-commit families remain pending or partial; sidecar-prune remains the
already-adopted technical lock. No immutable adoption receipt or score event was
found.

Local and `origin` Han/Ozzy refs were inspected without mailbox access. No direct
Han or Ozzy tip had a commit timestamp at or after `20260827T030805Z`; no
post-watermark product adoption, withdrawal, rebuttal, or current-base candidate
exists. Report-only Furiosa refs and scheduler/self receipts are not adoption
evidence.

Duplicates rejected: modernc `busy_timeout`, `BEGIN IMMEDIATE`, raw
`sqlite3_interrupt`, progress callbacks, Go `WaitDelay`/pidfd, Temporal,
Debezium outbox, Kafka/NATS acknowledgements, systemd state, S3 receipts, and
weighted semaphores are existing aliases or negative comparators. No fourth
recommendation is warranted.

Valid watermark remains `20260827T030805Z`; no future receipt was accepted.
Next leads: prove real modernc first-write cancellation/admission; add
transaction-layer entry tests for `BeginTx`/`ExecContext`/watermark publication;
and run parent-exit plus later-invocation recovery for pending/terminal receipts.

Direction Lock: `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically locked
under prior exact-base evidence. This report creates no lock, score,
implementation approval, or merge authorization. **NO MERGE AUTHORIZATION.**
