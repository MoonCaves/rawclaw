# T52 terminal prior-art raid

Run start UTC: `2026-08-27T04:01:52Z`.
Exact current base: `48661f403f880e2c1dac7615f39bbb8264eeafe7`.
Prior valid watermark: `20260827T032519Z`.
Run type: report-only. No production/test code, mailbox, helper, cursor, or supervisor state was
changed. No score claim or merge authorization is made.

## Verdict

The current base still fails the required trap. `runTagWriteCmd` performs the authoritative per-project
tag transaction, closes that database, then calls `spawnTagPublishChild`; `spawnTagPublishChild` calls
`exec.Cmd.Start` and `Process.Release`, while the child writes an append-only log. A parent death after
the authoritative commit and before `Start` leaves no pending publication identity. A child death before
its first byte leaves no queryable state. The foreground “publication queued” line means launch
acceptance only.

The smallest proven RawClaw-shaped mechanism remains a transactional outbox/local spool in SQLite:
persist immutable publication identity and `pending` state in the same transaction as the authoritative
tag mutation; a later invocation leases pending work, publishes it, and commits `completed` or `failed`
with retry metadata. This preserves the static binary/no-daemon/no-LLM default. It is still an
unadopted proposal, not a Direction Lock.

## Live problem census

| Area | Current-base evidence | Exact gap |
|---|---|---|
| Authoritative tag path | `internal/cli/cmd_tag.go:L473-L522` | Intent is created only after the write returns; parent death before child start loses handoff. |
| Detached tag publisher | `internal/cli/tagpublish.go:L36-L70` | `Start` + `Release` observe neither terminal application result nor retry ownership. |
| Other detached work | `internal/cli/bg_ingest.go:L99-L101`, `vectortopup.go:L43-L46`, `autosync.go:L84-L92` | A `Wait` goroutine or `Release` is process lifecycle handling, not durable publication state. |
| Existing safety evidence | `987c6a3`, `0b39b82`, `1ddf6ba` | Detached best-effort incompleteness is proven; no durable fix is present on `48661f4`. |

## External mechanisms inspected

| Source | Publication/update date | Exact mechanism | Relevance and fit |
|---|---|---|---|
| [Transactional outbox](https://microservices.io/patterns/data/transactional-outbox.html) | `Last-Modified: 2026-08-26T03:59:47Z`; original date not stated; accessed `2026-08-27` | Insert the event in an OUTBOX table in the same transaction as business state; an asynchronous relay later publishes it; consumers must tolerate duplicate delivery. | Direct precedent for atomically coupling tag state to a durable publication intent. Relay/broker is rejected for the sovereign core, but the local row is the exact reusable invariant. |
| [Debezium Outbox Event Router](https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html) | `Last-Modified: 2026-08-26T05:46:08Z`; accessed `2026-08-27` | A committed outbox row is transformed and routed by a connector, preserving event identity for downstream delivery. | Corroborates the identity/deduplication half of the outbox pattern; external connector is incompatible and duplicate evidence, not a new recommendation. |
| [SQLite Atomic Commit](https://sqlite.org/atomiccommit.html) | `Last-Modified: 2026-08-24T14:49:52Z`; accessed `2026-08-27` | Commit is the durability boundary; rollback before commit leaves no committed state. | Exact persistence boundary: the intent must be inside the same commit as the tag mutation; child start cannot be the durability boundary. |
| [systemd-run](https://www.freedesktop.org/software/systemd/man/latest/systemd-run.html) | `Last-Modified: 2026-07-23T17:13:53Z`; systemd man page; accessed `2026-08-27` | A transient service is managed by systemd; `--wait` observes completion and `--remain-after-exit` retains unit status after process termination. | Strong surviving-owner precedent, but it requires systemd, is not cross-platform, and violates the default no-daemon/no-runtime-dependency shape. External comparator only. |
| [Temporal durable execution](https://docs.temporal.io/workflows) | page `Last-Modified: 2026-08-27T03:00:26Z`; accessed `2026-08-27` | Ordered event history records workflow progress; replay reconstructs state after application failure and exposes completion independently of the original process. | Strong durable-history precedent, but requires a Temporal service/runtime. External comparator only. |
| [Amazon S3 replication status](https://docs.aws.amazon.com/AmazonS3/latest/userguide/replication-status.html) | `Last-Modified: 2026-08-27T02:36:50Z`; accessed `2026-08-27` | Replication exposes `PENDING`, `COMPLETED`, `FAILED`, and `REPLICA`; pending work can resume and failed work needs retry. | Useful honest receipt vocabulary. Remote replication is not a local handoff owner, so vocabulary only. |
| [khepin/liteq](https://github.com/khepin/liteq/tree/ed648f234d9475ef9d38047c42717f368115784a) | immutable commit `ed648f234d9475ef9d38047c42717f368115784a`, pushed `2026-06-24T01:36:07Z`; accessed `2026-08-27` | SQLite `jobs` table has queued/fetched status, execute-after time, remaining attempts, fetched/finished timestamps, errors, partial unique dedupe indexes, reset-on-visibility-timeout, and complete/fail operations. | Closest Go/SQLite local spool precedent. Its dependency and consumer pool are not proposed; copy the persisted state, lease timeout, retry, and dedupe invariants only. |
| [mikestefanello/backlite](https://github.com/mikestefanello/backlite/tree/dbe6ff326acde357458ee45014836bb9d0f7e017) | immutable commit `dbe6ff326acde357458ee45014836bb9d0f7e017`, pushed `2026-07-21T01:04:21Z`; accessed `2026-08-27` | Embedded SQLite task rows support persistence, retry/backoff, scheduled execution, status/retention, transaction-attached task creation, context-aware processing, and lease/reclamation configuration. | Mature embedded queue precedent. Its dispatcher/UI/type machinery is overkill; transaction-attached insertion and explicit terminal status are reusable. |

## Re-grade since `20260827T032519Z`

No external adopter, implementation, rejection, or withdrawal was found for the terminal-receipt
families since the watermark. All remain score `0`; self-adoption, report-only agreement, and source
corroboration are not score events.

| Recommendation ID | Stable fingerprint | Re-grade | Direction Lock |
|---|---|---|---|
| `PA-TX-OUTBOX-TERMINAL-001` | `fe9cb982dba957a8388de6696e90f587f02ae5d7b5ca4d919f16b8092b1c19de` | **unadopted proposal**, still smallest fit; zero score | No lock: no external adopter and no decisive implementation gate. |
| `PA-SPOOL-RETRY-TERMINAL-001` | `56917e624cc7b79d7d3e171454e3fd3a395ad588487a20152b5ca50eaf237894` | **unadopted proposal**, corroborated by liteq/backlite; zero score | No lock. |
| `PA-S3-STATUS-001` | `cc3c1a33b705b8341e3e709cd3de6a17c84229d16376cb9801225021fd5a1203` | **partial/narrowed**, vocabulary only; zero score | No lock. |
| `PA-SYSTEMD-OWNER-001` | `af1c49b2eaf4242bf74591ec04c527b745740de77590fd8b1d8bf5b0a7383c47` | **rejected for sovereign core**, external dependency; zero score | Not applicable. |
| `PA-TEMPORAL-DURABLE-PUBLISH-001` | `8bbacec11c2a18c9ce58db8292816b4caa3a816973695aaaabf50e861bd97936` | **rejected for sovereign core**, external service; zero score | Not applicable. |
| `PA-SQLITE-QUEUE-001` | `1cb31235386ea8e937e64de7ff9332527da13072cc6846464869f76d3069e7c8` | **duplicate/narrowed** into `PA-SPOOL-RETRY-TERMINAL-001`; zero score | No lock. |

Unrelated regrades retained without rescoring: `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains the
existing technical-only external adoption/Direction Lock; context-writer-token and SQLite interrupt
families remain partial or narrowed. They do not solve T7 and are not terminal-receipt adoption.

## Duplicates and rejected mechanisms

- `Process.Release`, `Wait`, `WaitDelay`, pidfd, `setsid`, and append-only logs are process observation
  or cleanup, not a durable pending intent or terminal application state.
- systemd, Temporal, and broker-backed Debezium are surviving external owners, but duplicate the
  rejected external-runtime category for RawClaw's sovereign default.
- liteq and backlite are evidence for the same SQLite spool family, not separate RawClaw proposals;
  their worker pools, UI, and third-party dependency are out of scope.
- S3 status is vocabulary only. It cannot make a local pre-`Start` handoff durable.
- `4ac774a4` remains malformed and its own checkout's default-scope failure is not current-base
  evidence; `d918706` remains rejected because deleting `AcquireConsolidatedFence` can lose
  acknowledged direct consolidated writes. `987c6a3`, `0b39b82`, and `1ddf6ba` prove the gap, not a fix.

## Graphify and Who Not How record

`graphify reflect --if-stale` ran; lessons contained no marked local outcomes. Because this checkout
has no graph, the read-only graph at `/Users/jay-m4/code/rawclaw/graphify-out/graph.json` was used.
Vocabulary-only queries used `receipt tag transaction sqlite outbox parent process durable publication
intent pending failed retry child start release fence`, followed by `explain PA-DEBEZIUM-OUTBOX-001`
and `path runTagWrite StampIngestWatermark`. The graph confirmed the tag-write/fence/watermark path;
the `transaction`→`receipt` query had no path and was recorded as a dead end. No graph edge was treated
as external proof.

Who Not How sources: Chris Richardson's transactional-outbox pattern, SQLite's atomic-commit model,
and the maintainers of liteq/backlite already solved the relevant decision. The reusable mechanism is
the smallest persisted row/state transition, not a new detached-process framework.

## Direction Lock and score boundary

Terminal recommendation status: **NO DIRECTION LOCK**. The lock schema is incomplete because there is
no same-base red/green implementation comparison, external adopter receipt, or decisive focused/full
gate for a durable outbox/spool. No score change: Han `+2`, Furiosa `+9`, Ozzy `+3` remain the
authoritative totals from the ledger; this lane adds zero.

## Timestamps, hashes, and cleanliness

- Prior-art ledger was read before research; valid watermark was `20260827T032519Z`.
- Report source observations were captured on `2026-08-27`; no future-dated receipt was used.
- `FINDINGS.md` commit and push: `3b99335`.
- This report is intentionally uncommitted until its content is complete; the next step is atomic
  commit and push. No production/test gate is claimed because this is report-only.
