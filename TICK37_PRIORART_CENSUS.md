# Tick 37 prior-art census

Run scope: report-only census on audit base `ef2eebf414e77086be06281539c5a50ba036a32a`.
Authoritative prior watermark: `20260827T010052Z` from the append-only prior-art ledger.
The forbidden supervisor mailbox was not read, modified, acknowledged, or advanced.

## Verdict

No newly adopted mechanism is proven since `20260827T010052Z`. The only counted adoption
remains the previously recorded Ozzy sidecar-prune event (`+3`); this census creates no score
event and grants no merge authorization. Report-only, inherited, self-adoption, scheduler,
control, challenge, and branch-ancestry claims score zero.

The current totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`. The technical Direction Lock for
`PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains locked on its recorded base and candidates, but is
technical direction only: **NO MERGE AUTHORIZATION**.

## Regraded mechanism families

| Family and stable fingerprint | Current ruling | Evidence after the watermark |
|---|---|---|
| `PA-SQLITE-BEGIN-IMMEDIATE-001` — `7e0c7cf321c3b845baf8518173f636fdf86eda2d81b69cd8b00b635cff80b214` | Narrowed; unadopted; score 0 | Mutation `47b1c2b9e1ed1fce5df46fc7f6ec66d64d960831` moved contention to transaction admission, but the existing 10-second busy timeout still delayed a 200 ms-context contender about 10.207 s. It is not context-bounded admission by itself. |
| `PA-SEMANTIC-BENCH-COUNTER-001` — `c0bb59011b65af9866dccc35ba701b834ec6daebfaed3ecb95a1fcfc1d83d11c` | Confirmed benchmark-validity rule; unadopted; score 0 | Mutation report `c0c8bcf077ea4b8403e2171351f567d24fdead4c` showed that latency can improve while useful work disappears. `B.ReportMetric` is insufficient: an explicit non-zero intended-work assertion and a rows examined/deleted/committed counter are required. |
| `PA-SQLITE-PROGRESS-BUDGET-001` — `6d296d6f799c6da5b26f79bd3ad51327a018d7bf11ca8324817b7b8c7753e42b` | Narrowed; unadopted; score 0 | modernc/sqlite v1.45.0 supports context cancellation/interruption through `ExecContext`/`QueryContext`, but no supported public progress-handler registration API was found. Any interruption must report incomplete work and publish only after commit; no custom-driver adoption exists. |
| `PA-SQLITE-BUSY-TIMEOUT-001` — `69634beea1d95e0696cb1f451e95a60df291d4487c6658ea0d231b25a9d5b841` | Duplicate; score 0 | RawClaw already configures `_pragma=busy_timeout(5000)` for read-only and `(10000)` for read-write connections. Preserve and test that existing setting; it is distinct from context deadlines, writer admission, and the cross-process fence, but it is not new prior art. |
| `PA-SQLITE-WAL-IDLE-CHECKPOINT-001` — `efe829449d40df34663d26fa33920d98f25d65fac768b66bc2e1218d8c201e91` (alias `PA-SQLITE-WAL-PASSIVE-CHECKPOINT-001` rejected) | Pending; unadopted; score 0 | Canonical recommendation remains idle `wal_checkpoint(PASSIVE)`/autocheckpoint scheduling outside closeout. The alias is deduplicated by recommendation fingerprint (`86a2faf...`); no independent adopter or Ozzy PASSIVE response was found. |
| `PA-FTS5-DELETEMERGE-001` — `21ae4bb81dff3c0531f08b62290258442aaea11b75949aeb2e7bca2996e240a2` | Pending; unadopted; score 0 | FTS5 `deletemerge` and bounded incremental `merge` amortize tombstone compaction. This is distinct from sidecar deletion eligibility and WAL checkpoint scheduling. No product payload or immutable adopter exists. |
| `PA-GO-SINGLEFLIGHT-FALLBACK-001` — `1532e53cf1b582d958f6fec89bcb723cf2da7681bc696a5b7cfbc0fe4bf3465a` | Pending; unadopted; score 0 | In-process result coalescing remains distinct from durable ownership, fencing, freshness, and reconciliation. No independent adoption receipt exists. |
| `PA-CONSOLIDATED-SIDECAR-PRUNE-001` — `d07f69f8d056f9f145bd9a864e3fa11660afadf13af3aca9acad39ea722bcb72` | Externally adopted; technically locked; no new score | The existing immutable adopter receipt remains the sole counted event. `c38f79a` and adapted `0cd00e4` are one mechanism; same-effect `a78b39b`/`96aa522`/`a62ab05` are one rejected duplicate effect family. Co-contributor preservation and absent-sidecar-table behavior remain the contract. |
| `PA-SQLITE-INTERRUPT-BEGIN-001` — `105d41020b8678e8a376b20bf41ef13ba8c27f6f200e39bbfed664702b8fc7c2` | Pending; unadopted; score 0 | Tailscale SQLite `master` (`8034b3e3`) combines `BEGIN IMMEDIATE` with a driver-level interrupt/cancellation hook. It is distinct from admission mode alone, but applicability to RawClaw’s modernc v1.45.0 driver is unproven. |
| `PA-SQLITE-BUSY-HANDLER-DEADLINE-001` — `cd5c50923ed6c22c45079656c5e6715364bc7460c9da00405956296c91c75530` | Narrowed external comparator; unadopted; score 0 | SQLite’s documented `sqlite3_busy_handler` can stop retrying at a caller deadline, but no supported public modernc hook was found. It is a callback variant, not a second fixed busy-timeout recommendation. |
| Ozzy `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` batch-prune speed claim | Rebutted/uncertain; unadopted; score 0 | Functional pruning was narrowly confirmed, but the speed claim lacks a fair old baseline, paired samples/benchstat, semantic work guard, and independent adoption. A no-op mutation cannot establish performance. |

Earlier terminal/publication proposals remain at their prior pending/partial/blocked rulings.
Two newer same-tick rival mechanisms were found, both zero-score and unadopted: the
driver-supported interrupt-plus-BEGIN precedent and the C-level deadline busy-handler variant.
Neither proves applicability to RawClaw’s actual driver, and neither changes the adoption ruling.

## Dedupe and evidence boundary

Deduplication used all available identity layers: normalized recommendation fingerprint, patch
ID, ancestry, and observed behavior. A changed commit SHA does not create a new recommendation;
an alias with the same behavior and fingerprint is one event; a report or audit is not product
adoption. In particular, BEGIN IMMEDIATE is not a duplicate of atomic-commit durability or the
file fence; FTS5 merge budgeting is not sidecar deletion; singleflight is not durable ownership;
WAL scheduling is not FTS5 merge budgeting; and progress interruption is not a completion receipt.

The post-watermark Git census found the Tick 37 external expansion (`1a980631`, report SHA
`598e9673c05f9ffcacd7e989e2f7329d7e3d04a795ebfe59de0c440d232af848`) in addition to Tick 34
mutation reports (`bd653930`, `47b1c2b9`, `c0c8bcf`),
Tick 36 Han claim-spy/correction (`620455f`, `c095126`), Tick 36 Ozzy claim-spy/correction
(`b1ec0e3`, `78971f4`), the Tick 36 score referee (`bdb5ba5`), and the Tick 37 regrade
(`2065541`). These are evidence and adjudication documents only. No adoption receipt or product
change followed the watermark.

The worktree is the dedicated branch
`worker/furiosa-t37-priorart-census-20260827`, based at `ef2eebf...`; the branch was clean before
this report. Related refs include the Tick 36 claim-spy/referee branches and Tick 37 regrade;
their reports do not alter the adoption ruling. The report is the only permitted file and no
mailbox, scorecard, rotation log, product source, shared harness, or graph state is in scope.

## Next experiments, ranked by decision value

1. **Current-base interrupt/admission test.** Hold a competing writer, run a rebuild contender
   with `BEGIN IMMEDIATE` and a live context deadline through the actual modernc driver, and
   record Begin, first write, cancellation, `SQLITE_BUSY`, and terminal-publication timings.
   This decides whether the Tailscale-style interrupt precedent is implementable without a
   10-second stall.
2. **Semantic benchmark guard.** Add only a disposable benchmark assertion that fails when the
   intended rows are zero, reports a work counter, and compares paired old/new samples with
   benchstat. This decides whether the observed speedup is real work or a no-op.
3. **Driver-supported cancellation on real maintenance.** Exercise context interruption against
   actual prune/checkpoint work, including partial progress and commit boundaries. This decides
   whether a progress-budget proposal is implementable without a driver fork or false success;
   if interrupt is unavailable, reject the new mechanism rather than importing a C-only API.

These are experiments, not authorization to merge. Any future score requires a fresh current-base
payload, one stable fingerprint, exact patch-id and ancestry proof, an immutable adopter receipt,
and personally observed focused/full gates.

## Validation contract

This file is report-only. Commit and push only `TICK37_PRIORART_CENSUS.md`; then verify a clean
worktree, upstream divergence `0 0`, and the report SHA-256. No product behavior is changed.
