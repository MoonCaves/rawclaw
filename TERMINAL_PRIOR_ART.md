# Terminal receipt prior art

Run completion captured with `date -u`: `2026-08-26T21:42:55Z`. Base:
`8c8216e25e22496b2e3e919fce836be49d692e25`. Valid ledger watermark read before this run:
`20260826T202709Z`. No production code is changed.

## Prior-art ruling

The current detached publisher is an acknowledged best-effort launch, not a durable handoff.
`runTagWriteCmd` writes the authoritative per-project database, then `spawnTagPublishChild` calls
`exec.Cmd.Start`, redirects the child's output to `tag-publish.log`, and immediately calls
`Process.Release`. The foreground `publication queued` line therefore means only that launch was
accepted. The 20/20 immediate `Start + Kill + Release` reproduction in immutable reports `987c6a3`
and `0b39b82`, plus Furiosa's `1ddf6ba` ruling, establishes that zero child receipt bytes can remain.

The smallest mechanism that actually closes the loss window is a transactional outbox/local spool:
commit a publication intent and its immutable source identity in the same SQLite transaction as the
authoritative tag write; later drain it with lease, retry, and explicit terminal state. The
foreground acknowledgement remains `queued`, while `completed` or `failed` is read from durable
state. This is a design candidate, not an adoption or implementation recommendation in this lane.
It preserves a single static binary and no daemon, but requires a new on-disk schema and a defined
drain trigger.

Systemd and Temporal demonstrate stronger external owners, but they add a service/runtime dependency
and do not fit the sovereign default. S3's status vocabulary is useful but is not a local handoff.
SQLite queue precedents validate persistence, lease timeout, redelivery, and retry, but do not by
themselves supply a parent-owned terminal receipt. No cosmetic log line, parent `Wait`, or
`Process.Release` error handling closes the post-`Start` death window.

## External sources

| Source | Publication/update date | Inspected mechanism | Concrete relevance | Status |
|---|---|---|---|---|
| [Transactional outbox](https://microservices.io/patterns/data/transactional-outbox.html) | page `Last-Modified: 2026-08-26 03:59:46 GMT`; original publication not stated; accessed 2026-08-26 | Write the event to an OUTBOX table in the same transaction as business state, then asynchronously publish; relay may duplicate, so consumers need idempotence. | Direct precedent for atomically coupling the tag mutation to a durable publication intent and immutable identity. | **ADOPTED AS PRIOR ART; partial fit** — broker relay is not proposed. |
| [SQLite transactions](https://www.sqlite.org/lang_transaction.html) | page date not exposed in response; accessed 2026-08-26 | Explicit `BEGIN`/`COMMIT`/`ROLLBACK`; write transactions serialize and `BEGIN IMMEDIATE` acquires the write immediately; cancellation/close must not be confused with commit. | Supplies the atomic commit primitive for an embedded outbox row plus the tag mutation. | **ADOPTED AS PRIOR ART; partial fit** |
| [goqite](https://github.com/maragudk/goqite) | created 2024-01-15; updated 2026-08-22; pushed 2026-07-10; accessed 2026-08-26 | Mature Go queue on SQLite: persisted messages, receive timeout, no redelivery before timeout, extendable visibility, and a job runner; 536 stars at inspection. | Closest language/runtime precedent for a single-binary local spool with lease timeout and redelivery. | **ADOPTED AS PRIOR ART; candidate fit** — dependency is not proposed for core. |
| [Backlite](https://github.com/mikestefanello/backlite) | created 2024-06-22; updated 2026-08-08; pushed 2026-07-21; accessed 2026-08-26 | Embedded SQLite task queues with persistence, retry/backoff, scheduled work, task status, and graceful shutdown; 151 stars. | Confirms durable task status and retries are ordinary SQLite queue concerns, not a terminal-log trick. | **ADOPTED AS PRIOR ART; partial fit** — extra worker/UI features are out of scope. |
| [Delayed Job](https://github.com/collectiveidea/delayed_job) | created 2008-09-13; updated 2026-08-01; accessed 2026-08-26 | Database-backed jobs reserve rows with an exclusive lock, recover stale locks, and record failed attempts; worker names survive restarts. | Mature lease/lock/recovery precedent for a retry owner and explicit failure state. | **ADOPTED AS PRIOR ART; partial fit** — Ruby implementation is not a dependency. |
| [systemd-run](https://www.freedesktop.org/software/systemd/man/latest/systemd-run.html) | systemd 261.2 man page; page date not stated; accessed 2026-08-26 | A transient service is managed by systemd as parent; `--wait` waits for completion, and `--remain-after-exit` retains status after process exit. | Proves an external supervisor can own detached work and preserve status beyond the caller. | **REJECTED for sovereign core** — requires systemd and cannot be the default cross-platform owner. |
| [Temporal Workflows](https://docs.temporal.io/workflows) | page `Last-Modified: 2026-08-26 21:29:11 GMT`; accessed 2026-08-26 | Durable ordered event history, replay, and recorded activity results let execution resume after application failure. | Strong precedent for durable execution history and terminal result independent of worker lifetime. | **REJECTED for sovereign core** — requires a Temporal service/runtime. |
| [S3 replication status](https://docs.aws.amazon.com/AmazonS3/latest/userguide/replication-status.html) | page date not stated; accessed 2026-08-26 | Explicit `PENDING`, `COMPLETED`, `FAILED`, and `REPLICA` states; temporary failures remain pending and can resume; failed objects need a retry path. | Supplies honest terminal-status vocabulary and the rule that disappearance is not completion. | **ADOPTED AS VOCABULARY ONLY** — remote replication semantics are not a local queue. |

## Recommendation status and fingerprints

The valid watermark was `20260826T202709Z`; all prior recommendations below were re-graded before
new proposals. `pending` means no external adopter plus immutable green receipt and therefore zero
score. The new fingerprints are SHA-256 of the normalized text shown after each ID.

| ID | Re-grade/status | Normalized recommendation text | Fingerprint |
|---|---|---|---|
| `PA-PG-MERGE-SCOPED-001` | **partial / superseded for this lane** — valid for source-scoped deletion, not terminal handoff. | Use PostgreSQL MERGE-style source-scoped reconciliation: match authoritative source rows, update or insert present rows, and delete only target rows NOT MATCHED BY SOURCE within the same source scope. | `46f16a453f6ab78d2cb3b8a5cb27c730d5433ecea8cf169a99d248daa9aa2cbf` |
| `PA-S3-TERMINAL-RECEIPT-001` | **partial / narrowed** — retain status vocabulary; no local durability claim. | Use Amazon S3 replication-status-style terminal receipts: asynchronous publication exposes explicit COMPLETED, FAILED, or REPLICA state plus retryable metrics, so disappearance or process exit is never treated as success. | `80561798a88b288ce4197d0c137c31075dd00f25cd1732695bf6acf9cd53f97d` |
| `PA-TEMPORAL-DURABLE-PUBLISH-001` | **rejected for core / external comparator** — durable history requires an external service. | Use Temporal Workflow-style durable asynchronous publication: persist execution history and expose terminal workflow result or failure independently of the worker process lifetime. | `8bbacec11c2a18c9ce58db8292816b4caa3a816973695aaaabf50e861bd97936` |
| `PA-TX-OUTBOX-TERMINAL-001` | **unadopted proposal** — smallest fit; zero score. | Commit authoritative mutation and durable pending publication intent in one SQLite transaction; drain pending intents on a later invocation with immutable job identity and explicit terminal state. | `fe9cb982dba957a8388de6696e90f587f02ae5d7b5ca4d919f16b8092b1c19de` |
| `PA-SPOOL-RETRY-TERMINAL-001` | **unadopted proposal** — corroborated by goqite/Backlite/Delayed Job; zero score. | Use a local SQLite-backed leased queue with pending/running/completed/failed states, timeout-based redelivery, and retry count; treat foreground output as enqueue acknowledgement only. | `56917e624cc7b79d7d3e171454e3fd3a395ad588487a20152b5ca50eaf237894` |
| `PA-SYSTEMD-OWNER-001` | **rejected** — external dependency violates core constraints. | Use systemd transient service state and --wait/RemainAfterExit as an external surviving owner and status store. | `af1c49b2eaf4242bf74591ec04c527b745740de77590fd8b1d8bf5b0a7383c47` |
| `PA-TEMPORAL-HISTORY-001` | **rejected** — external service dependency. | Use durable event history and replay to reconstruct execution and expose terminal result independent of worker lifetime. | `85e9d27b4f0999538a22bb94ef25dcfe5d4c4352aa2bcd0dc0f2b780953a3f53` |
| `PA-S3-STATUS-001` | **partial / narrowed** — vocabulary only, no implementation. | Expose pending/completed/failed/replica-style publication status and retry signal as the receipt vocabulary. | `cc3c1a33b705b8341e3e709cd3de6a17c84229d16376cb9801225021fd5a1203` |
| `PA-SQLITE-QUEUE-001` | **unadopted proposal** — covered and narrowed by `PA-SPOOL-RETRY-TERMINAL-001`; do not score twice. | Use SQLite-persisted messages with lease timeout and redelivery for a local queue; acknowledge only after durable completion state. | `1cb31235386ea8e937e64de7ff9332527da13072cc6846464869f76d3069e7c8` |

## Adoption, duplicates, and direction lock

- Adoption evidence: none for the new outbox/spool proposals. `987c6a3`, `0b39b82`, and `1ddf6ba`
  are independent red/ruling evidence, not adopters or green receipts. No score change: Han remains
  `+2`; Furiosa remains `+9`.
- Duplicate rejection: `PA-SQLITE-QUEUE-001` is a narrowed duplicate of the spool proposal; the
  existing PostgreSQL, S3, and Temporal recommendations are re-graded rather than rescored. No
  recommendation earns points from this report or from self-adoption.
- Direction Lock: **NO LOCK**. There is no same-base red/green winner, external adopter, or complete
  gate packet for a durable queue. The current candidate remains best-effort and terminal-incomplete.

## Process attack matrix

The exact matrix is in [`FINDINGS.md`](FINDINGS.md). Its decisive cases are T1/T2/T7: parent return,
child death, or parent death can occur after the authoritative write but before durable publication
intent or child terminal output. T3/T4 show that even completed work can lack a durable terminal
receipt. These are the reasons the outbox and spool proposals require persisted state, not more log
lines.

## Graphify and method record

`graphify reflect --if-stale` ran in this worktree and found no local graph memories. The isolated
checkout has no `graphify-out/graph.json`, so local graph queries were unavailable. Read-only use of
the repository graph at `/Users/jay-m4/code/rawclaw/graphify-out/graph.json` confirmed the
`runTagWriteCmd` path and its `spawnIngestChild`-style detached-child analogue; its lessons mark
direct child reuse as a dead end. No graph result was used as proof of external prior art.

## Validation and cleanliness

- Files touched by this lane: `FINDINGS.md`, `TERMINAL_PRIOR_ART.md` only; immutable production file
  fence is `internal/index/consolidated.go`, unchanged.
- `FINDINGS.md` commit: `bc077e0f63bc07f958bc47df734e565807df1d4f`.
- External push could not be confirmed: SSH `git push` and `git ls-remote` returned no remote ref
  within the bounded command window. The local branch is clean, but upstream equality is **UNCERTAIN**.
- No production edit, code formatting, or green claim is made from this report-only lane.
