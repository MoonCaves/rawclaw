# Furiosa Tick 49 — durable detached receipt audit

Run completion: `2026-08-27T03:29:53Z` (UTC; captured with `date -u`). Base:
`ef2eebf414e77086be06281539c5a50ba036a32a`. This is report-only. No product
code, mailbox, cursor, or graph output was modified.

## Inputs and boundary

The requested literal `PRIOR_ART_LOG` was not present in the checkout. The
authoritative cumulative ledger was read at
`/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/PRIOR_ART_LOG.md`;
its last valid watermark is `20260827T030805Z`. The closest checked-in Tick 49
ledger is `TICK49_EXACT_MECHANISMS.md`, which records the same watermark and
the existing detached-terminal-receipt family.

Graphify was run against the root graph before source inspection. It connects
`spawnIngestChild`, `spawnAutosyncChild`, `spawnVectorTopupChild`, their append
logs, spawn tokens, and existing pending/retry tests. Current source confirms
the gap: `internal/cli/bg_ingest.go:73-104` starts a child and only appends to
`ingest.log`; `internal/archive/autosync.go:54-70` and
`internal/semantic/topup.go:40-48` provide analogous logs. `Start` followed by
`Release` or a best-effort `Wait` cannot establish child entry or application
terminal success.

## Smallest zero-runtime mechanism found

The smallest fit is the already-ledgered durable outbox / detached-terminal
receipt family, not a new mechanism:

1. In the same local SQLite transaction as the trigger decision, insert a
   stable job identity and `pending` state, with payload, attempt count, and
   lease/claim timestamps. Commit before `exec.Cmd.Start`.
2. Start the detached child with the job identity. The child atomically claims
   `pending` (or an expired lease), performs the work, and commits exactly one
   terminal `succeeded` or `failed` row containing the identity, attempt, and
   error/result metadata.
3. A later ordinary RawClaw invocation reclaims `pending`/expired `running`
   rows and retries them. A unique identity plus terminal-state conditional
   update makes duplicate child delivery harmless. A missing child receipt is
   therefore visible as pending, rather than inferred from process state.

Smallest RawClaw adaptation: replace `autosync.log`, `vector-topup.log`, and
`ingest.log` as the machine-readable completion signal with one existing local
SQLite receipt table (or the already-known pending/terminal schema), and add
one transaction-bound insert before spawn plus child terminal update and a
later drain/reclaim path. Keep `setsid` only for best-effort launch isolation.
Do not add a daemon, queue service, connector, or new runtime dependency.

## Exact external precedents

* SQLite, “Atomic Commit In SQLite,”
  <https://www.sqlite.org/atomiccommit.html>, Last-Modified
  `2026-08-24T14:49:52Z`, accessed `2026-08-27`: rollback journal/WAL sync and
  commit boundary. Relevance: terminal success and the pending-to-terminal
  transition must be published only after a successful commit.
* Go 1.24.6 `os/exec`, immutable source at
  <https://github.com/golang/go/blob/go1.24.6/src/os/exec/exec.go>, tag commit
  `7f36edc26d4e3becb6d9c9008ff00f260bb19055`, accessed `2026-08-27`.
  `Cmd.Start`, `Cmd.Wait`, `Cmd.Release`, `WaitDelay`, and `Cancel` are process
  launch/observation and pipe cleanup; none is a durable application receipt.
* `khepin/liteq`, main commit
  `ed648f234d9475ef9d38047c42717f368115784a`,
  <https://github.com/khepin/liteq/tree/ed648f234d9475ef9d38047c42717f368115784a>,
  pushed `2026-06-24T01:37:46Z`, accessed `2026-08-27`. `db/schema.sql` has
  durable `jobs.id`, `job_status`, `remaining_attempts`,
  `consumer_fetched_at`, `finished_at`, and partial unique indexes for queued
  and fetched deduplication. `internal/methods.go` exposes
  `QueueJob`, `GrabJobs`, `ResetJobs`, `CompleteJob`, and `FailJob`, using
  context-aware SQL and visibility-timeout reclamation. This is a concrete
  local SQLite pending/claim/retry/dedup precedent, but it is the same outbox
  family already recorded in the ledger.
* `mikestefanello/backlite`, main commit
  `dbe6ff326acde357458ee45014836bb9d0f7e017`,
  <https://github.com/mikestefanello/backlite/tree/dbe6ff326acde357458ee45014836bb9d0f7e017>,
  pushed `2026-07-21T01:04:21Z`, accessed `2026-08-27`. `internal/query/schema.sql`
  persists `backlite_tasks` with a text primary-key ID, `claimed_at`,
  `attempts`, and a separate `backlite_tasks_completed` terminal table.
  `internal/task/task.go:InsertTx` writes identity and payload in a caller
  transaction; `internal/task/completed.go:InsertTx` writes terminal success or
  failure through `Tx.ExecContext`; dispatcher `releaseAfter` reclaims work.
  This is an especially close implementation precedent, but uses
  `github.com/google/uuid` and therefore is not a dependency to import into
  RawClaw's sovereign core.

## Regrade, rivals, and rejection

Existing stable IDs remain unchanged:

* `PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001` — partial, score `0`.
* Existing durable outbox / atomic-commit / detached-terminal-receipt family —
  pending or partial, score `0`; no new ID minted.

The liteq and backlite mechanisms are duplicate enrichment of that family, not
new RawClaw adoption. Their public repositories were scanned for adoption
signals; no RawClaw product commit, immutable adopter receipt, withdrawal, or
rebuttal was found after watermark `20260827T030805Z`. Public rival refs and
report-only branches are not adoption evidence.

Rejected as non-solutions: `setsid`, `Process.Release`, `WaitDelay`, `Cmd.Wait`,
Linux pidfds, systemd state, Temporal, Kafka/NATS acknowledgements, Debezium
outbox, S3, and any external queue/service. They observe or transport process
state, or add a runtime dependency; none supplies the required local durable
ownership and later reclamation in the sovereign binary.

## Verdict

**No new prior-art recommendation. Score delta: 0.** The smallest adaptable
shape is a local SQLite transactional pending/terminal receipt with immutable
job identity, lease expiry, retry, and idempotent terminal commit. It is already
represented by the ledger's atomic-commit/outbox/detached-terminal-receipt
family. Local durability cannot force survival through an arbitrary supervisor
kill; the later RawClaw invocation is the recovery owner. Direction Lock
`PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technical-only and locked. **NO
MERGE AUTHORIZATION.**

Next lead: add a parent-exit/child-entry loss harness and a later-invocation
reclaim test before considering implementation; do not treat green `Start` or
append-log tests as terminal publication proof.
