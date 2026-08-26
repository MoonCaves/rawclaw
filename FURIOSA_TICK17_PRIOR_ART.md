# Furiosa Tick 17 prior-art raid

Run timestamp: 2026-08-26T22:00:41Z (UTC; captured with `date -u`).

## Scope and method

This was a report-only prior-art pass from exact base
`8c8216e25e22496b2e3e919fce836be49d692e25`, branch
`worker/furiosa-priorart-t17-20260827`, in the isolated worktree
`/Users/jay-m4/code/rawclaw-furiosa-priorart-t17-20260827`. No RawClaw source was inspected or
edited. Graphify was run first (`reflect --if-stale`, then literal-vocabulary queries for
detached publication and source-scoped reconciliation); it confirmed `runTagWriteCmd`,
`spawnTagPublishChild`, `ReplaceSessionSegments`, and `SyncConsolidatedFrom` as the relevant
anchors. The graph also records the known dead end: a detached child is not itself a durable
terminal receipt.

The valid input watermark was `20260826T202709Z`. Conforming top-level mailbox receipts after it
and through this run were read; hidden, malformed, quarantined, and future-dated entries were
excluded. The sweep found no receipt that supplies a complete Direction Lock. Current evidence
keeps `fb99037` as a best-effort queued publication, leaves `96aa522` under attack, and leaves
the nil-scope/default and process-death receipt gaps unresolved.

## Re-grade of existing recommendations

- `PA-K8S-SCOPED-RECONCILIATION-001`: remains `partial`/blocked. The current composite's
  per-source authority is materially closer to the Kubernetes desired/current-state model, but
  no independent immutable green adoption receipt proves the whole deletion and co-contributor
  contract. No score event.
- `PA-ETCD-CAS-CURSOR-001`: remains `pending`; no new cursor-CAS adoption receipt was found.
- `PA-DEBEZIUM-OUTBOX-001`: remains `pending`; the live terminal lane still has a queued child
  without durable ownership/retry evidence.
- `PA-PG-MERGE-SCOPED-001`: remains `pending`; no source-scoped sidecar adoption receipt.
- `PA-S3-TERMINAL-RECEIPT-001`: remains `pending`; current receipts explicitly say queued is
  not durable terminal success.
- `PA-TEMPORAL-DURABLE-PUBLISH-001`: remains `pending`; no persisted execution history or
  terminal-result implementation is evidenced.
- `PA-DIRECT-LUNA-001` and `PA-CURSOR-OWNER-001`: remain `externally_adopted`, unchanged.
- `PA-COMPOSITE-ID-001`: remains `partial`/blocked by the deleted-topic red chain and lack of
  a whole-session replacement proof.
- `PA-DIRECTION-LOCK-001`: remains `partial`; no exact-base red/green winner plus independent
  immutable adopter packet exists.

The receipts `20260826T212617Z-17f01764...`, `20260826T212620Z-10ed43b2...`, and
`20260826T215639Z-3d6709ee...` explicitly constrain the detached contract. The receipts
`20260826T212619Z-45db4c3f...` and `20260826T213606Z-22f139d7...` keep the sidecar candidate in
adversarial review. These are evidence and control inputs, not score-eligible adoption events.

## Three additional exact mechanisms

### `PA-NATS-JS-DURABLE-ACK-001`

Source: https://docs.nats.io/nats-concepts/jetstream/consumers — “Consumers” / NATS
JetStream documentation, last modified 2026-08-26T00:56:38Z (HTTP `Last-Modified`), accessed
2026-08-26T21:59:54Z.

Inspected mechanism: a durable consumer tracks delivery state; explicit acknowledgements mark
work complete, while unacknowledged deliveries are redelivered. The mechanism separates “worker
received/queued” from “durably acknowledged.”

Relevance: strong conceptual fit for a detached publisher: write an immutable publication event,
acknowledge only after the derived store and terminal receipt are durable, and leave unacked work
recoverable. It would be optional seam behavior, not a NATS dependency in RawClaw's sovereign
core. Status: `pending`; no RawClaw adoption or score.

Normalized-text SHA-256:
`696a59f7cc3f0ecb948ccaf1218f427e94b5661ec962d7807e7d43ed5cf2f7c7`.

### `PA-GIT-REF-TRANSACTION-001`

Source: https://git-scm.com/docs/git-update-ref — “git-update-ref”, date not stated, page last
modified 2026-08-25T04:46:40Z (HTTP `Last-Modified`), accessed 2026-08-26T21:59:55Z.

Inspected mechanism: `update-ref --stdin` supports a transaction with start/prepare/commit and
compare-old-value checks, so several ref updates become all-or-nothing and stale writers fail
instead of silently publishing over a changed value.

Relevance: useful exact model for detached publication ownership: compare the expected receipt or
revision, atomically publish the derived pointer plus receipt reference, and make a crash or
stale worker observable as uncommitted rather than “queued success.” It is an adaptation pattern,
not authorization to change RawClaw's archive contract. Status: `pending`; no RawClaw adoption or
score.

Normalized-text SHA-256:
`9ac9a46c39864def365666bf0fe4bc4e2a4e7d177404140000f87329f6b20d21`.

### `PA-SYSTEMD-RESTART-STATE-001`

Source: https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html —
“systemd.service”, last modified 2026-07-23T17:13:53Z (HTTP `Last-Modified`), accessed
2026-08-26T21:59:55Z.

Inspected mechanism: service units expose explicit active/failed lifecycle state; `Restart=` can
restart abnormal failures, and `ExecStopPost=` runs for cleanup/post-mortem handling even when
normal stop commands are skipped. Process disappearance therefore remains a managed failure
state rather than completion.

Relevance: a plausible optional host adapter for a durable publisher: supervise the worker,
restart failures, and persist a terminal receipt from post-stop handling. It cannot by itself
prove publication committed, and it adds an external runtime, so it is not suitable for the
default single-binary path. Status: `pending`; no RawClaw adoption or score.

Normalized-text SHA-256:
`4e3a233c9aed714499ac7bd0a925282fa7152db78f9e592d09f8fc954c59c335`.

## Dedupe and score accounting

No new score-eligible adoption event was found. The three mechanisms are distinct from the
already logged Go Context, Kubernetes controllers, Kafka, Git object IDs/notes, etcd CAS,
Debezium outbox, PostgreSQL MERGE, S3 replication statuses, and Temporal workflow history. The
NATS acknowledgement model is not counted as Kafka offset semantics; Git transactional refs are
not counted as Git object-ID/notes identity; systemd restart/state is not counted as Temporal
durable workflow history. Existing `fb99037`, `96aa522`, `f6d4a55`, `e43127e`, `5eb3a383`, and
`3b641ce` receipts are re-grade evidence only. Duplicate scoring by recommendation fingerprint +
adopter + immutable receipt remains rejected.

## Direction Lock

`NO LOCK`. Required complete record is absent: there is no independently adopted same-base
red/green winner with exact candidate SHAs or patch IDs, decisive focused filter and full gate,
immutable external adopter receipt, and invalidation triggers. In particular, the detached
publication receipt gap remains an active invalidation risk.

## Watermark and next leads

The new valid watermark is `20260826T215639Z`, the newest conforming receipt actually processed
before this run completion. Next leads: obtain an immutable red/green proof for process death
between `Start` and child entry; prove durable retry/ownership or explicitly narrow the contract;
and attack `96aa522` with co-contributor retention, missing-sidecar-table, re-add, and rollback
mutations. Then re-grade the three pending mechanisms without awarding prose-only adoption.

Run completion: `2026-08-26T22:02:13Z` (UTC; captured with `date -u` immediately after the shared
ledger append).
