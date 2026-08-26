# Terminal-receipt prior-art attack

Base: `8c8216e25e22496b2e3e919fce836be49d692e25`.

## Ruling

`spawnTagPublishChild` provides detached best-effort publication and a local log, but it cannot
guarantee a terminal receipt after the parent returns. `cmd.Start` only establishes that the child
was started; `Process.Release` gives the parent no completion or retry owner. If the parent, terminal,
or child dies after `Start` and before the child emits its first byte, the authoritative tag write
survives but the terminal receipt can be absent forever. A durable guarantee therefore requires a
surviving queue/retry owner and a persisted state machine; a cosmetic log, `Wait`, or parent-side
flush cannot provide that guarantee while retaining detached foreground semantics.

## Exact process-interleaving attack matrix

| ID | Interleaving | Durable state | Observable terminal result | Verdict |
|---|---|---|---|---|
| T1 | Parent writes authoritative tags; `Start` returns; parent calls `Release`; parent exits before child enters `runTagPublishChild`. | Per-project tags exist; no publication record. | Zero child receipt bytes; no owner retries. | **FAIL** for guaranteed receipt. |
| T2 | Parent writes tags; child starts; child is killed before `runTagPublishChild` reaches `SyncConsolidatedFromContext`. | Per-project tags exist; consolidated view may be stale. | Zero or partial log; disappearance is indistinguishable from success. | **FAIL** for durable handoff. |
| T3 | Parent writes tags; child completes `SyncConsolidatedFromContext`; child dies before `tagPublishLogLine`. | Derived store may be current. | No terminal success receipt despite successful publication. | **FAIL** for receipt truth. |
| T4 | Parent writes tags; child logs success; process dies before log append reaches stable storage. | Publication may be complete; log durability is unspecified. | Missing or truncated terminal line. | **FAIL** for immutable receipt. |
| T5 | Two tag writes for the same source race; both detached children publish in opposite order. | Each source write may be valid; derived state depends on fold order. | Logs do not identify an ordered durable job/result pair. | **UNCERTAIN** unless publication identity/order is persisted. |
| T6 | Parent returns the foreground "publication queued" line; detached child later hits `SQLITE_BUSY` or timeout. | Authoritative tags exist; derived publication failed. | Foreground output reported queueing, child log reports failure later. | **BEST-EFFORT ONLY**, not success. |
| T7 | Parent is killed after authoritative DB commit but before `Start`. | Per-project tags exist; no child was created. | No child bytes and no discoverable pending work. | **FAIL** unless enqueue is transactional with the write. |
| T8 | Parent is killed during the authoritative write before commit. | Transaction rolls back or is otherwise implementation-dependent. | No valid terminal receipt should claim publication. | **SAFE only if the write remains atomic and status is explicit.** |

## Prior-art implication

The smallest proven shape is an embedded transactional-outbox/local-spool record committed in the
same SQLite transaction as the authoritative tag mutation, with an explicit pending/running/
completed/failed state and retry identity. A later `rawclaw` invocation can drain it. This fits the
single-binary/no-daemon constraint, but it changes the current contract from "detached best effort"
to "durably queued, eventually published" and needs a separate design and gate. systemd transient
units and Temporal provide surviving owners externally, but add a daemon/service dependency and are
not fits for RawClaw's sovereign core. S3 replication status is useful status vocabulary, not a local
handoff mechanism. SQLite-backed queues validate the embedded persistence/retry pattern, but their
worker lifecycle must not be mistaken for parent-death durability.

No production change is recommended from this lane.
