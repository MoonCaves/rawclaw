# Tick 45 detached terminal-receipt prior art

Run completed: `2026-08-27T02:41:58Z` (UTC; WITA `2026-08-27 10:41:58`).

Base: `ef2eebf414e77086be06281539c5a50ba036a32a`.

Prior-art ledger watermark read before research: `20260827T022635Z`. The ledger was not
edited by this report-only lane. Since that watermark, all recorded recommendations remain
unchanged: sidecar prune is technically locked/external-adopted; SQLite admission and interrupt
families are narrowed/partial; the semantic counter is validity-only and unadopted; WAL,
FTS5-deletemerge, singleflight, atomic commit, outbox, Kafka/NATS/Git, systemd, Kubernetes,
S3, Temporal, and PostgreSQL retry families remain pending/partial/rejected as already recorded.
No post-watermark immutable adopter receipt or score event was found. Silence is zero.

## RawClaw boundary

At this base, `spawnIngestChild` starts a `setsid` child, redirects output to an append log, and
returns; the production path starts a best-effort goroutine calling `Cmd.Wait`, while one-shot
detached paths call `Process.Release`. Graphify's current-tree traversal ties the relevant
publication path to `runTagWriteCmd → SyncConsolidatedFrom`, and identifies `spawnIngestChild`
as a log/cache seam rather than a durable publisher. The existing receipt attack remains the
decisive local fact: successful `Start` does not prove child entry or a terminal receipt.

## Exact external mechanisms inspected

### 1. Go `os/exec.Cmd.WaitDelay` and `Cancel`

- Immutable source: <https://github.com/golang/go/blob/go1.24.6/src/os/exec/exec.go>, Go tag
  commit `7f36edc26d4e3becb6d9c9008ff00f260bb19055`; source fetched `2026-08-27`.
- Exact mechanism: `CommandContext` invokes `Cancel` when the context ends; `WaitDelay` bounds
  waiting for a child that ignores cancellation and for inherited pipes that never close. If
  the child exits successfully but pipes remain open, `Wait` returns `ErrWaitDelay`; if the
  child does not exit, Go kills it after the delay. `Wait` is therefore an attached-process
  observation and cleanup primitive, not a durable result publication protocol.
- Transferable invariant: a successful process exit and closed output streams are separate
  observations; cancellation/timeout must be represented as non-success, and callers must not
  infer a terminal application result from `Start` or from process disappearance.
- Minimal RawClaw adaptation: use `WaitDelay` only in a foreground/reaping path that owns the
  child. Do not use it to claim detached survival or to synthesize a durable receipt.
- Failure modes: parent exit before `Wait`; child disappears before application receipt; child
  descendants retain pipes; `WaitDelay` kills a still-useful child; platform-specific detached
  process behavior remains outside `os/exec`'s contract.
- Duplicate decision: no new PA ID. This sharpens the already logged detached best-effort and
  terminal-receipt gap, and overlaps existing cancellation/terminal publication families.
- Evidence quality: A (official Go source at an immutable release tag; mechanism is explicit),
  but applicability to detached RawClaw publication is negative.

### 2. Linux `pidfd_open` polling and `waitid`

- Immutable source: <https://github.com/mkerrisk/man-pages/blob/ae6b221882ce71ba82fcdbe02419a225111502f0/man2/pidfd_open.2>,
  man-pages repository `master` HEAD `ae6b221882ce71ba82fcdbe02419a225111502f0`; source
  fetched `2026-08-27` (HTTP source date `2026-08-27`).
- Exact mechanism: a PID file descriptor names a process without PID-reuse ambiguity; `poll`/
  `epoll` reports `EPOLLIN` when the task terminates and becomes a zombie, `EPOLLHUP` after it
  is reaped, and `waitid` can reap a child. The man page explicitly limits `waitid` to a PIDFD
  that is a child of the caller.
- Transferable invariant: process identity and process termination observation should use a
  stable kernel handle, not a recycled numeric PID or a guessed log state; reaping is distinct
  from observing termination.
- Minimal RawClaw adaptation: none in the sovereign core. A Linux-only optional helper could
  poll a pidfd while the parent remains alive, but it cannot make a detached child survive the
  parent or publish an application-level result after the parent is gone.
- Failure modes: Linux-only (not macOS/Windows); parent must retain the pidfd and remain able to
  reap; a child can terminate before writing its receipt; `EPOLLIN` proves process death, not
  successful publication; a released/orphaned child is not made durable by pidfd.
- Duplicate decision: no new PA ID. This is a process-observation/reaping comparator for the
  existing detached receipt gap, not durable retry, outbox ownership, or terminal publication.
- Evidence quality: A for kernel semantics, C for RawClaw transfer because the default binary
  must remain cross-platform and zero-dependency.

### 3. Debezium transactional outbox / event router

- Immutable source: <https://github.com/debezium/debezium/blob/v2.7.3.Final/documentation/modules/ROOT/pages/transformations/outbox-event-router.adoc>,
  tag commit `922deeb8b212b331db1d41ef8b9fc7e6750fd608`; source fetched `2026-08-27`.
- Exact mechanism: application state and an outbox row are committed together; a connector
  captures changes from the outbox table and routes only those committed rows. The event carries
  a stable aggregate/message key and headers, allowing downstream delivery to be retried and
  deduplicated by identity. It requires a running connector/CDC service.
- Transferable invariant: durable handoff must be recorded before derived/asynchronous
  publication; a committed immutable identity is the retry/reconciliation anchor; missing
  derived output is pending/failure, never success.
- Minimal RawClaw adaptation: the already-recorded SQLite local spool/outbox proposal: commit a
  pending job and its identity in the local database, then let a later RawClaw invocation drain
  it with leases and terminal states. This is not a new mechanism for Tick 45.
- Failure modes: connector/service outage; row-claim/lease expiry and duplicate delivery;
  database commit does not prove downstream publication; no connector exists in RawClaw's
  single-static-binary default.
- Duplicate decision: exact duplicate of `PA-DEBEZIUM-OUTBOX-001`, and semantically overlaps
  `PA-SQLITE-ATOMIC-COMMIT-001`, `PA-PG-SKIP-LOCKED-RETRY-001`, and the prior detached outbox
  report. No new fingerprint or recommendation ID is warranted.
- Evidence quality: A for the outbox contract (official Debezium documentation at an immutable
  release), D for default-core applicability because it adds a service/runtime.

## Capability proof matrix

| Mechanism | Detached survival | Terminal success receipt | Durable retry | Duplicate-child suppression | Publish only after commit |
|---|---|---|---|---|---|
| Go `WaitDelay`/`Cancel` | No | No; only process/pipe status | No | No | No; caller must implement it |
| Linux pidfd | No | No; termination only | No | No | No |
| Debezium outbox | Worker/service can outlive caller, but not guaranteed by the row alone | Yes, only if downstream state/receipt is separately modeled | Yes, with external connector and replay | Identity supports it, but dedupe is consumer policy | Yes for the handoff row, not necessarily final publication |

No inspected source proves all five properties within RawClaw's constraints. In particular,
neither Go `Wait`, PIDFD polling, `setsid`, nor `Process.Release` proves detached survival plus
terminal success after parent death. Only the outbox family provides a durable retry anchor, and
that family is already present in the ledger and remains unadopted.

## Regrade and verdict

Every recommendation visible since watermark `20260827T022635Z` was re-graded from immutable
ledger evidence: no external adopter, no immutable green adoption receipt, no status transition,
and no score. Existing detached-receipt findings remain open/best-effort; durable ownership and
retry remain a separate unimplemented feature. The three inspected mechanisms produce no new
normalized text/fingerprint because each is either a direct sharpening of the existing receipt
gap or a duplicate of an existing outbox/atomic-commit/retry family.

**Compact verdict: NO NEW ID.** No merge authorization, score, or implementation recommendation.

