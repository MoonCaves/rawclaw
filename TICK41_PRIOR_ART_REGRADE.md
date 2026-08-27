# Tick 41 cumulative prior-art regrade

run_timestamp: `2026-08-27T02:00:53Z`
run_completion_utc: `2026-08-27T02:00:53Z`
prior_watermark: `20260827T013550Z`
new_watermark: `20260827T020038Z`

## Boundary and method

This is an append-ready, report-only delta. The authoritative ledger was read
through its Tick 39 preliminary watermark. The current UTC clock was sampled
at completion. Per the instruction, no supervisor mailbox, cursor, scorecard,
or shared `PRIOR_ART_LOG.md` was edited. Graphify was run against the shared
read-only graph (`reflect --if-stale`, the required literal query, `explain
pruneTombstoned`, and `path SyncConsolidatedFrom AcquireConsolidatedFence`).
Its useful result is that `SyncConsolidatedFrom` reaches the consolidated
fence and `pruneTombstoned` is called by both consolidation paths.

The supervisor confirms it processed the conforming Tick 40 final receipt at
`20260827T020038Z`; this is the only watermark advance in this delta. No
independently adopted product receipt after `20260827T013550Z` was reported.
The watermark therefore advances even though score eligibility does not. The visible Tick 40
Han/Ozzy reports are report-only evidence; their mailbox receipts are outside
scope.

## Cumulative recommendation regrade

The schema status below uses only `unadopted`, `partial`, `externally_adopted`,
`rejected`, and `superseded`; historical qualifiers are retained in detail.

| ID | fingerprint | status | Tick 41 ruling |
|---|---|---|---|
| `PA-DIRECT-LUNA-001` | `b5e48f35a6311765be1e82f92f86d8968965b48ad1a08c8e3bd16219b4384498` | externally_adopted | unchanged; direct collaboration receipts are already scored |
| `PA-COMPOSITE-ID-001` | `5669844a4a350d1d8c38b3a6dbaafab1ff16b330f1d4a945ca012f4db85d9796` | partial | unchanged; deleted-topic red proofs still block whole-session replacement |
| `PA-CURSOR-OWNER-001` | `1916c8a6f38efc9ffa934982b66683492f3afe7fbcca671fa33c30d74eb1ca72` | externally_adopted | unchanged; preserve `UNATTRIBUTED / DO NOT GUESS` for Tick 35 actor |
| `PA-DIRECTION-LOCK-001` | `60eeec2f5642c9f2eb7c2d77966bac014499d105d1304f35a8ec8e9fbecf991e` | partial | unchanged; no complete reusable lock receipt |
| `PA-CONSOLIDATED-SIDECAR-PRUNE-001` | `d07f69f8d056f9f145bd9a864e3fa11660afadf13af3aca9acad39ea722bcb72` | externally_adopted | technically LOCKED only; preserve no-merge-authorization direction |
| `PA-PG-MERGE-SCOPED-001` | ledger fingerprint | unadopted | unchanged; external precedent only |
| `PA-S3-TERMINAL-RECEIPT-001` | ledger fingerprint | unadopted | unchanged; no adopter or immutable green receipt |
| `PA-TEMPORAL-DURABLE-PUBLISH-001` | ledger fingerprint | unadopted | unchanged; no adopter or immutable green receipt |
| `PA-ETCD-CAS-CURSOR-001` | ledger fingerprint | unadopted | unchanged; no adopter or immutable green receipt |
| `PA-DEBEZIUM-OUTBOX-001` | ledger fingerprint | unadopted | unchanged; no adopter or immutable green receipt |
| `PA-SQLITE-SCHEMA-CAPABILITY-001` | ledger fingerprint | partial | unchanged; capability probe is not adoption |
| `PA-KAFKA-TRANSACTIONAL-TERMINAL-001` | ledger fingerprint | unadopted | unchanged; precedent only |
| `PA-NATS-JS-DURABLE-ACK-001` | ledger fingerprint | unadopted | unchanged; precedent only |
| `PA-GIT-REF-TRANSACTION-001` | ledger fingerprint | unadopted | unchanged; precedent only |
| `PA-SYSTEMD-RESTART-STATE-001` | ledger fingerprint | unadopted | unchanged; precedent only |
| `PA-K8S-SCOPED-RECONCILIATION-001` | ledger fingerprint | partial | unchanged; independent whole-deletion/co-contributor proof absent |
| `PA-SQLITE-UPSERT-COMPOSITE-001` | `e469d659ebee6f6d10301c46ec06c6594f780eb84677a57a474de12db87fe6c1` | unadopted | unchanged; no immutable adoption |
| `PA-SQLITE-ATOMIC-COMMIT-001` | `c063e5ece27bfbb90105f159565a752711ed6dd67fefa8c3ba7d6032b02c4901` | unadopted | unchanged; no immutable adoption |
| `PA-PG-SKIP-LOCKED-RETRY-001` | `43ecf08af238d737d56cbdfc65c6eaa6637a44133dbc9e32d42374cb3f16f12e` | unadopted | unchanged; no immutable adoption |
| `PA-SQLITE-BEGIN-IMMEDIATE-001` | `7e0c7cf321c3b845baf8518173f636fdf86eda2d81b69cd8b00b635cff80b214` | partial | narrowed: admission mode alone exceeded a 200 ms context under 10 s busy timeout |
| `PA-FTS5-DELETEMERGE-001` | `21ae4bb81dff3c0531f08b62290258442aaea11b75949aeb2e7bca2996e240a2` | unadopted | unchanged; no bottleneck proof or adopter |
| `PA-GO-SINGLEFLIGHT-FALLBACK-001` | `1532e53cf1b582d958f6fec89bcb723cf2da7681bc696a5b7cfbc0fe4bf3465a` | unadopted | unchanged; no adopter |
| `PA-SQLITE-WAL-IDLE-CHECKPOINT-001` | `efe829449d40df34663d26fa33920d98f25d65fac768b66bc2e1218d8c201e91` | unadopted | unchanged; alias remains duplicate, no adoption |
| `PA-GO-WEIGHTED-SEMAPHORE-WRITER-001` | `3be536e7d5aa2e34267b8b0b334b81165311f124ce38d5bfd45ac57676593c40` | rejected | unchanged; absent dependency is unnecessary for weight-one admission |
| `PA-SQLITE-PROGRESS-BUDGET-001` | `6d296d6f799c6da5b26f79bd3ad51327a018d7bf11ca8324817b7b8c7753e42b` | partial | narrowed: modernc exposes context cancellation, not a public progress handler |
| `PA-SQLITE-BUSY-TIMEOUT-001` | `69634beea1d95e0696cb1f451e95a60df291d4487c6658ea0d231b25a9d5b841` | rejected | duplicate of RawClaw's existing 5 s RO / 10 s RW settings |
| `PA-SEMANTIC-BENCH-COUNTER-001` | `c0bb59011b65af9866dccc35ba701b834ec6daebfaed3ecb95a1fcfc1d83d11c` | unadopted | validity requirement confirmed; permanent guard/adopter absent |
| `PA-SQLITE-INTERRUPT-BEGIN-001` | `105d41020b8678e8a376b20bf41ef13ba8c27f6f200e39bbfed664702b8fc7c2` | partial | rebutted for lock admission; confirmed only for executing statements |
| `PA-SQLITE-BUSY-HANDLER-DEADLINE-001` | `cd5c50923ed6c22c45079656c5e6715364bc7460c9da00405956296c91c75530` | partial | external comparator only; modernc public hook remains unproved |

No recommendation is superseded in this interval. No new recommendation is
added: no novel mechanism has both a preserved fingerprint and an independent
RawClaw adoption receipt.

## Adoption, scores, and duplicate rejection

Adoption evidence remains limited to the immutable receipts already recorded
in the ledger: sidecar-prune receipt
`fb8147aa46baf466869e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`, the
earlier direct-Luna and owner-cursor receipts, and their existing score events.
Tick 40 Han report `TICK40_HAN_CLAIM_SPY_FINDINGS.md` (report SHA-256
`d0616359290141b6b75a258db2205e36a32b9c75f91d3824b0c48bcfbcfaedf5`) keeps
the integrated `8e9c9b7` payload narrow/UNCERTAIN and explicitly provides no
adopter or score. Tick 40 Ozzy report `TICK40_OZZY_CLAIM_SPY_FINDINGS.md`
(report SHA-256 `06f9bc4d3dae1e3ec1e1615c8eae91abe5e0f41d74805eaea3501f60ce3860a3`)
keeps the existing sidecar adoption at +3 and rejects new speed credit.

Score-eligible events: none. Preserve totals exactly: **Furiosa +9, Han +2,
Ozzy +3**. Reject duplicate score events by
`fingerprint + adopter + immutable receipt SHA/path`; this rejects repeated
sidecar claims, WAL aliases, report-only audits, self-adoption, inherited
ancestry, and the Ozzy/Han Tick 40 challenge reports. No score or merge
authorization follows. Direction Lock `PA-CONSOLIDATED-SIDECAR-PRUNE-001`
remains technical-only and locked.

## Next leads (maximum three external-search mechanisms)

1. Driver-supported cancellation at real `BEGIN IMMEDIATE`/busy admission,
   with a bounded policy below the caller deadline.
2. SQLite FTS5 tombstone maintenance with paired before/after samples,
   query plans, and a non-zero semantic work oracle.
3. Durable publication/receipt coupling (SQLite atomic commit or Git ref
   transaction) that proves retry ownership and no lost direct writes.

Exact source and report receipts remain those already recorded in
`PRIOR_ART_LOG.md`: SQLite transactions/FTS5/WAL/progress/busy-handler pages;
Go `x/sync`/`ReportMetric`; Tailscale SQLite HEAD
`15a02b90c60613ae3b6caa4a07c945cb3c874611`; and the Tick 40 Han/Ozzy report
SHAs above. No product code or shared ledger was changed.

## Correction note — 2026-08-27T02:03:20Z

The original wording incorrectly paired `new_watermark: 20260827T020038Z`
with “the watermark does not advance.” The processed conforming receipt and
later completion make the advance valid; only adoption and score remain
unchanged.
