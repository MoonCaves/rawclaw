# Tick 29 WHO NOT HOW prior-art findings

Run completion: `2026-08-27T00:02:30Z` (UTC). Scope was limited to the
corrected watermark `20260826T232815Z`; no mailbox, cursor, or mailbox helper
was accessed.

## Mandatory re-grade

The cumulative ledger records each of these as `pending`, score `0`, with no
immutable external adoption after the corrected watermark:

| ID | Re-grade | Evidence |
|---|---|---|
| `PA-SQLITE-BEGIN-IMMEDIATE-001` | pending; 0 | Ledger line 719; no adopter receipt or current-base implementation recorded. |
| `PA-FTS5-DELETEMERGE-001` | pending; 0 | Ledger line 720; no adopter receipt or measured closeout result recorded. |
| `PA-GO-SINGLEFLIGHT-FALLBACK-001` | pending; 0 | Ledger line 721; no adopter receipt; generation/freshness concern remains open. |

The prior ledger explicitly says all three remain pending and that no new
adoption evidence changed status. Existing entries for SQLite atomic commit,
UPSERT, PostgreSQL `SKIP LOCKED`, transactional outbox, Temporal, NATS, Kafka,
Git refs, systemd, and scoped reconciliation were treated as duplicates or
already-covered mechanisms, not new recommendations.

## Additional exact mechanisms

### `PA-SQLITE-WAL-IDLE-CHECKPOINT-001`

**Normalized text:**

> Use SQLite WAL automatic checkpoint thresholds plus application-initiated
> `wal_checkpoint(PASSIVE)` during idle maintenance, keeping checkpoint work
> outside the closeout critical path while retaining explicit progress and
> incomplete-checkpoint handling.

**Fingerprint SHA-256:**
`efe829449d40df34663d26fa33920d98f25d65fac768b66bc2e1218d8c201e91`

**Source:** [SQLite Write-Ahead Logging](https://www.sqlite.org/wal.html),
title “Write-Ahead Logging”, Last-Modified `2026-08-24T14:49:52Z`, accessed
`2026-08-27T00:01:11Z`.

**Inspected mechanism:** SQLite documents that automatic checkpointing occurs
at a default 1000-page threshold and can be adjusted or disabled; disabled
checkpoints may instead run during idle moments or in a separate thread/process
(page source lines 256-263). It also documents that checkpoints can run
concurrently with readers but stop at readers’ end marks (lines 299-305), while
only one writer appends to the WAL (lines 293-297).

**Relevance:** RawClaw’s closeout risk includes long foreground consolidation,
WAL/SQLite lock contention, and maintenance work competing with tag publication.
An idle/application maintenance checkpoint is an exact scheduling primitive to
measure against foreground checkpoint behavior. It does not replace
`BEGIN IMMEDIATE`, FTS5 `deletemerge`, or durable publication ownership.

**Status:** `new`, pending, score 0. No adoption evidence was observed.

**Not a duplicate:** `PA-FTS5-DELETEMERGE-001` concerns FTS5 segment deletion
and merge budgeting; this recommendation concerns SQLite WAL-to-main-file
checkpoint scheduling. `PA-SQLITE-BEGIN-IMMEDIATE-001` concerns writer
admission, not checkpoint timing.

### `PA-GO-WEIGHTED-SEMAPHORE-WRITER-001`

**Normalized text:**

> Use `golang.org/x/sync/semaphore.Weighted` with context-aware `Acquire` and
> `Release` to bound concurrent RawClaw writer/fallback operations; use a
> weight of one for writer admission and retain separate durable ownership,
> fencing, and request coalescing.

**Fingerprint SHA-256:**
`3be536e7d5aa2e34267b8b0b334b81165311f124ce38d5bfd45ac57676593c40`

**Source:** [golang/sync semaphore.go](https://raw.githubusercontent.com/golang/sync/master/semaphore/semaphore.go),
title “Package semaphore”, inspected current source at accessed
`2026-08-27T00:01:11Z` (HTTP ETag
`dd23afe7d65054b977cdb7650b23b9cd146ec7d00f92857f1602a19a1f9dc862`).

**Inspected mechanism:** `NewWeighted` constructs a bounded resource; `Acquire`
checks cancellation before admission, queues waiters, returns `ctx.Err()` on
cancellation, and rolls back an acquisition if cancellation races readiness.
`Release` returns capacity and wakes FIFO waiters. The implementation also
documents that queue fairness prevents a large writer request from starving
behind readers.

**Relevance:** RawClaw has multiple in-process fallback and writer triggers.
Weighted admission can bound SQLite writer pressure and make waiting
cancellable without pretending that duplicate calls are coalesced or that a
process crash leaves durable ownership. It is suitable as an optional
coordination primitive only after measuring lock/busy behavior.

**Status:** `new`, pending, score 0. No adoption evidence was observed.

**Not a duplicate:** `PA-GO-SINGLEFLIGHT-FALLBACK-001` coalesces identical
in-process calls and shares one result; a weighted semaphore limits aggregate
concurrency and preserves distinct queued calls. It does not provide
singleflight semantics, durable CAS, or a publication queue. `BEGIN IMMEDIATE`
is SQLite transaction-level admission; this is Go process-level cancellation-
aware admission and can protect calls before opening SQLite.

## Adoption, duplicates, watermark, and next leads

- `changed_statuses`: none. The three mandatory recommendations remain
  pending/score 0; the two additions above are pending/score 0.
- `adoption_evidence`: none. No score-eligible event and no Direction Lock.
- `duplicates_rejected`: no new recommendation was made for already-ledgered
  atomic commit, UPSERT, FTS5 deletemerge, `BEGIN IMMEDIATE`, singleflight,
  outbox, Temporal, Kafka/NATS acknowledgements, Git ref transactions, systemd,
  or PostgreSQL `SKIP LOCKED`.
- `valid_candidate_new_watermark`: `20260826T232815Z` unchanged. Because the
  mailbox was prohibited, no later receipt can be claimed as processed.
- `next_leads`: benchmark idle/application WAL checkpointing against foreground
  checkpoint work under the real consolidated store; benchmark weighted writer
  admission against `SQLITE_BUSY` with cancellation; then seek an independent
  adopter receipt before assigning any score or technical Direction Lock.

