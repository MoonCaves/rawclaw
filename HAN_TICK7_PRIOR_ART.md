# Han Tick 7 prior-art raid

Run timestamp: `2026-08-26T20:17:09Z` (UTC, captured with `date -u`).

This report records the Tick-7 regrade and three bounded external mechanisms. The full evidence and hashes are appended to the shared prior-art ledger at the same run.

## Regrade

- `PA-DIRECT-LUNA-001` and `PA-CURSOR-OWNER-001`: `externally_adopted`; no new score.
- `914c527`: `REJECT_COMPLETE/ADOPT_SLICE`; patch ID `10cb762b575702cb95e78eb947e117e4d06d38bf`; shellcheck `UNCERTAIN`; no independent adopter or immutable green receipt; score zero.
- `74a0285`: `WITHDRAWN`; `00e587d`: `partial/superseded`; `4119698`/`537641b`: direct-deletion comparators; `74d4ee9`: pending scoped author-before-fence candidate.
- Totals remain Han `+2`, Furiosa `+9`; no score event claimed.

## Three exact mechanisms

1. `PA-K8S-SCOPED-RECONCILIATION-001` — Kubernetes Controllers, https://kubernetes.io/docs/concepts/architecture/controller/ (last modified 2024-09-01; accessed 2026-08-26). Desired/current-state reconciliation and linked ownership/labels prevent one controller deleting another's resources. Adapt as per-source expected-key replacement: delete only keys absent from that source, never from a partial/skipped source, and preserve co-contributors. Normalized recommendation SHA-256: `fe219500925ec6924763c8810bcfc36d22d8079451b90f5d79c669d414326e10`. Pending, no adoption.

2. `PA-ETCD-CAS-CURSOR-001` — etcd API, https://etcd.io/docs/v3.5/learning/api/ (last modified 2024-04-06; accessed 2026-08-26). Transaction predicates compare version/create revision/modification revision/value and select success/failure operations atomically. Adapt canonical receipt-key validation, owner authorization, and monotonic cursor CAS. Normalized recommendation SHA-256: `0da2f5b99c94a8d560d25e423a942fe4e7f80ba8095cd0b4c753147c0677ddc0`. Pending, no adoption.

3. `PA-DEBEZIUM-OUTBOX-001` — Debezium Outbox Event Router, https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html (date not stated; accessed 2026-08-26). A transactional outbox commits an event with internal state, then asynchronously routes committed rows. Adapt durable author-before-derived-publication with immutable `(SessionID, StartUUID, revision)` and terminal receipt. Normalized recommendation SHA-256: `f78d296ab78da985f22b1597bb11c6509f91e6f19d35c97c7255cf6a05798bb8`. Pending, no adoption.

## Decision effect

Graphify on the current integration graph identified `runTagWriteCmd → SyncConsolidatedFromContext`, with `pruneTombstoned`, `ReplaceSessionSegments`, and `mergeTopicsSQLFor` as relevant seams. The mechanisms change decisions by narrowing deletion authority, requiring owner/CAS cursor writes, and making authoritative authoring durable before detached publication. Direction Lock is incomplete; no exact-base red/green winner plus independent immutable adoption receipt exists. New valid watermark remains `20260826T200025Z`; the historical `20260826T20:20:00Z` entry is future-dated and preserved but cannot advance it.

Shared ledger SHA-256: pre `66ffaeaba0eadcc2e79a0e1700332f5409b04b6710cbbabc4572f2bbe598faab`; post `0a0ff2aae02c01bc597c4b2fdf158bad6368032ff152bd890494f4b60ecdc673`.

Graphify useful-result memory was saved. Mnemon remember receipt: `6f2fd2ed-69fc-4e81-852f-969fbfe014f0`.
