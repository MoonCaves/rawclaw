# Tick 52 prior-art cancellation delta

Run timestamp: `2026-08-27T04:04:14Z`  
Run completion UTC: `2026-08-27T04:04:14Z`  
Prior watermark: `20260827T032519Z`  
New watermark: `20260827T032519Z`

The watermark is unchanged. This worker did not access the supervisor mailbox or cursor, and no
later conforming receipt was independently available from the read-only prior-art log. No future
dated watermark is accepted.

## Last-launch grade

The last launch, Tick 49's durable-receipt source addendum, is **rejected as duplicate enrichment**.
`khepin/liteq` and `mikestefanello/backlite` supply useful SQLite pending/claim/retry/terminal
examples, but they are the same durable-outbox, atomic-commit, detached-terminal-receipt, and lease
families already in the ledger. They have no independent RawClaw adopter, immutable RawClaw green
receipt, or score-eligible event. Same-family and self adoption score zero. This is not
supersession and does not alter Direction Lock.

## Independently inspected external mechanisms

### PA-GO-CONTEXT-WRITER-TOKEN-001

Normalized recommendation: `Place one context-aware admission gate before any SQLite write: process-local capacity admission returns on context cancellation before entering SQLite, while retaining the cross-process fence.`

Fingerprint SHA-256: `3ab8baf8ea330608ca48fdfb7d5167cfc47c4152fefd566b989385811b2a2afc`  
Status: `partial`, score `0`.

Source: [Go database/sql BeginTx](https://github.com/golang/go/blob/go1.24.6/src/database/sql/sql.go),
Go 1.24.6 source tag commit `7f36edc26d4e3becb6d9c9008ff00f260bb19055`, inspected
`2026-08-27T04:03:09Z`. The `BeginTx` path passes the context to the driver and starts
`Tx.awaitDone`; cancellation rolls the transaction back. This confirms that cancellation must enter
the transaction boundary, but it does not create a caller-context-aware SQLite lock-admission hook.

Limitation: driver cancellation support is conditional. The current modernc SQLite v1.45.0 audit
(`b8975b7dcf269f7c09929073c1feb64701066f41`) found execution interruption but no supported public
busy-handler, progress-handler, unlock-notify, or raw interrupt seam for first-write admission.
`AcquireConsolidatedFence` therefore remains a separate cross-process fence; a process-local token
would be an additional unadopted design, not evidence of adoption.

### PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001

Normalized recommendation: `Treat SQLite interruption or pre-commit cancellation as incomplete: explicit transaction writes roll back and watermark or terminal receipt advances only after commit.`

Fingerprint SHA-256: `a78ec1938f09271ba45faa1cc46023d6ad9741d276fb993b33518c394616d6c5`  
Status: `partial`, score `0`.

Source: [SQLite unlock notification API](https://www.sqlite.org/c3ref/unlock_notify.html),
official SQLite C API page, HTTP Last-Modified `2026-08-24T14:49:52Z`, inspected
`2026-08-27T04:03:09Z`. The API registers a callback for the connection holding a required lock;
the callback fires when that transaction commits or rolls back. The companion
[unlock-notify example](https://www.sqlite.org/unlock_notify.html) retries after notification and
rolls back immediately when SQLite detects a deadlock. This is the exact wait-then-retry versus
rollback boundary relevant to writer admission.

Immutable anchor: the official documentation has no Git commit/tag; the recorded HTTP
Last-Modified value and canonical API URL are the available upstream identity. The page also states
that the feature requires `SQLITE_ENABLE_UNLOCK_NOTIFY` and shared-cache mode.

Limitation: RawClaw's modernc driver does not expose this C hook publicly, and the mechanism is not
a durable publication receipt. A notification or interrupted statement cannot advance
`StampIngestWatermark`; source merge, derived rows, and watermark still need one transaction whose
commit is the publication boundary.

### PA-TEMPORAL-DURABLE-PUBLISH-001 / PA-SQLITE-ATOMIC-COMMIT-001

Canonical normalized recommendations and fingerprints:

- `PA-TEMPORAL-DURABLE-PUBLISH-001` — `Use Temporal Workflow-style durable asynchronous publication: persist execution history and expose terminal workflow result or failure independently of the worker process lifetime.` Fingerprint SHA-256: `8bbacec11c2a18c9ce58db8292816b4caa3a816973695aaaabf50e861bd97936`.
- `PA-SQLITE-ATOMIC-COMMIT-001` — `Use SQLite atomic-commit protocol: write journal-backed transaction, commit only after durable sync, and treat pre-commit process death as retryable rather than success; publish terminal receipt in the same transaction.` Fingerprint SHA-256: `c063e5ece27bfbb90105f159565a752711ed6dd67fefa8c3ba7d6032b02c4901`.

Status: both remain `pending`, score `0`; no new ID is minted for River because it is duplicate
family evidence.

Source: [River v0.23.0](https://github.com/riverqueue/river/tree/99e45ec023354d8670f3314fe1909b85e860ecd0),
`riverqueue/river` immutable commit `99e45ec023354d8670f3314fe1909b85e860ecd0`, title
“Prepare version v0.23.0 (#943)”, committed `2025-06-04T08:42:46-07:00`, inspected
`2026-08-27T04:02:58Z`. In `internal/jobexecutor/job_executor.go`, execution keeps the live
context through work and reports cancellation, discard, retryable, or available state through a
state-if-running completion update. In the SQLite generated driver, `JobGetAvailable` atomically
changes an available job to running and increments its attempt; `JobSetStateIfRunning` conditionally
records terminal/error state; `JobGetStuck` identifies running jobs older than a cutoff; unique-key
insert uses conflict handling. All database calls use context-aware methods.

Relevance: this is a mature, SQLite-capable example of durable identity, conditional terminal
commit, retry scheduling, stuck-work reclamation, and idempotent uniqueness. It directly supports a
RawClaw design where pending work is durable before detachment and terminal success is written only
after the application result is known.

Limitations: River is an external job system, not a RawClaw adopter. Its database model and worker
process are larger than the sovereign zero-dependency core, and this inspected commit does not prove
that a parent-exit RawClaw child can publish a terminal receipt. No score or merge authorization
follows.

## Adoption, deduplication, and Direction Lock

- New external adoption: none.
- Partial adoption: none newly evidenced. Existing RawClaw partial statuses remain unchanged.
- Rejected: Tick 49 durable-receipt addendum as duplicate family enrichment; River is also rejected
  as a new recommendation ID for the same reason.
- Superseded: none.
- Score event: zero. Totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`.
- Dedupe rule applied: fingerprint + adopter + immutable evidence. External source code is not a
  RawClaw adopter receipt; self-family evidence cannot score twice.
- Direction Lock: `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically `LOCKED` on the existing
  base/candidate/selector/gates/adopter record. This report grants no merge authorization.

## Validation and next leads

The report is the only intended file change. The pre-existing untracked `.t52-prior-art-worker.log`
was not touched. Next leads are a real modernc first-write cancellation/admission test, a
transaction-layer entry test proving rollback and watermark suppression, and a parent-exit harness
that observes durable pending-to-terminal retry behavior. None is claimed complete here.
