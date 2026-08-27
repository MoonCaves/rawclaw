# Furiosa Tick 33 external prior-art findings

run_timestamp: `2026-08-27T00:42:52Z` (captured with `date -u` before research)
prior_watermark: `20260826T235525Z`

## External sources

- `https://www.sqlite.org/c3ref/progress_handler.html` | SQLite C Interface: Query Progress Callbacks | Last-Modified `2026-08-24T14:49:52Z`; accessed `2026-08-27T00:42:54Z` | inspected `sqlite3_progress_handler`, callback cadence in VM instructions, non-zero callback interruption, and the callback restriction against modifying its connection | directly supports a bounded maintenance budget: interrupt long prune/checkpoint work and surface incomplete work rather than blocking closeout indefinitely.
- `https://www.sqlite.org/c3ref/busy_timeout.html` | SQLite C Interface: Set A Busy Timeout | Last-Modified `2026-08-24T14:49:52Z`; accessed `2026-08-27T00:42:55Z` | inspected finite sleep accumulation and `SQLITE_BUSY` after the timeout, plus the one-busy-handler-per-connection rule | directly supports bounded native lock waiting without a new Go dependency; timeout exhaustion remains retryable and must not be reported as completed publication.
- `https://github.com/mattn/go-sqlite3/blob/master/sqlite3.go` | mattn/go-sqlite3 driver, connection initialization | repository source inspected at current `master`, accessed `2026-08-27T00:42:56Z` | inspected `_busy_timeout` DSN parsing and `PRAGMA busy_timeout` application | mature public Go adoption shows the native SQLite busy-timeout mechanism is copyable at the connection seam rather than requiring a custom waiter.
- `https://pkg.go.dev/testing#B.ReportMetric` | Go `testing` package: `B.ReportMetric` | accessed `2026-08-27T00:43:06Z` | inspected the standard benchmark custom-metric interface alongside `B.Loop`/`ReportAllocs` | supports recording semantic work counters with latency, making a benchmark auditable for “did useful work” rather than only “ran faster.”

## Recommendations

### PA-SQLITE-PROGRESS-BUDGET-001

normalized_text: `Use SQLite progress-handler cancellation for bounded maintenance: interrupt prune/checkpoint work after an explicit VM-step or context budget, return an incomplete status, and publish completion only after the maintenance transaction commits.`

fingerprint_sha256: `6d296d6f799c6da5b26f79bd3ad51327a018d7bf11ca8324817b7b8c7753e42b`
status: `pending`; score: `0`

Decision impact: this adds a distinct, native cancellation seam for scan-heavy closeout phases. A callback must only observe cancellation state; it must not mutate the connection. Interruption is an incomplete result, not success, and the durable terminal receipt is emitted only after commit. This is not FTS5 `deletemerge` scheduling or the already-ledgered WAL checkpoint mode.

### PA-SQLITE-BUSY-TIMEOUT-001

normalized_text: `Configure a finite SQLite busy timeout per connection so lock contention waits are bounded and returns SQLITE_BUSY for retry; never let busy waiting extend foreground closeout beyond its deadline.`

fingerprint_sha256: `69634beea1d95e0696cb1f451e95a60df291d4487c6658ea0d231b25a9d5b841`
status: `pending`; score: `0`

Decision impact: this is a native bounded-wait policy, not a replacement for `BEGIN IMMEDIATE`, the cross-process consolidated fence, or durable ownership. Configure once per connection; on timeout preserve `SQLITE_BUSY` as retryable and do not advance a watermark or terminal receipt. The mature Go driver source demonstrates the exact connection-level placement.

### PA-SEMANTIC-BENCH-COUNTER-001

normalized_text: `Require performance benchmarks to report a semantic work counter (rows examined/deleted/committed) and reject runs with zero useful work, alongside latency metrics, so a no-op optimization cannot pass.`

fingerprint_sha256: `c0bb59011b65af9866dccc35ba701b834ec6daebfaed3ecb95a1fcfc1d83d11c`
status: `pending`; score: `0`

Decision impact: every closeout benchmark comparing prune/checkpoint variants must expose the work-conservation counter and assert the fixture exercised the intended rows. `B.ReportMetric` is the standard output channel; the zero-work rejection is the local benchmark contract. This directly addresses the prior mutation whose apparent speedup came from deleting no live IDs, and is not a new production mechanism.

## Mandatory cumulative re-grade

- `PA-SQLITE-BEGIN-IMMEDIATE-001`, `PA-FTS5-DELETEMERGE-001`, and `PA-GO-SINGLEFLIGHT-FALLBACK-001` remain `pending`, score `0`; no independent adopter or immutable green adoption receipt was found after `20260826T235525Z`.
- `PA-SQLITE-WAL-IDLE-CHECKPOINT-001` remains `pending`, score `0`; no adoption evidence changed its status.
- `PA-GO-WEIGHTED-SEMAPHORE-WRITER-001` remains `rejected` as unnecessary dependency-specific machinery; no score.
- Earlier pending/partial recommendations (`PA-ETCD-CAS-CURSOR-001`, `PA-DEBEZIUM-OUTBOX-001`, `PA-PG-MERGE-SCOPED-001`, `PA-S3-TERMINAL-RECEIPT-001`, `PA-TEMPORAL-DURABLE-PUBLISH-001`, `PA-NATS-JS-DURABLE-ACK-001`, `PA-GIT-REF-TRANSACTION-001`, `PA-SYSTEMD-RESTART-STATE-001`, `PA-KAFKA-TRANSACTIONAL-TERMINAL-001`, and `PA-SQLITE-UPSERT-COMPOSITE-001`/`PA-SQLITE-ATOMIC-COMMIT-001`), plus `PA-SQLITE-SCHEMA-CAPABILITY-001` partial and `PA-COMPOSITE-ID-001` partial/blocked, remain unchanged at zero new score.
- `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains externally adopted and technically locked; no invalidation trigger or merge authorization follows.

## Adoption, scoring, and duplicates

adoption_evidence: none after the prior watermark; all three recommendations are pending until an independent adopter supplies an immutable receipt and focused/full gates.
score_eligible_events: none; score delta `0`; totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`.
duplicates_rejected:

- Progress-handler cancellation is bounded operation interruption, distinct from WAL idle checkpoint scheduling, FTS5 merge budgeting, and terminal receipt ownership.
- Busy timeout is native lock-wait expiry, distinct from `BEGIN IMMEDIATE` admission, weighted semaphore/singleflight, and the cross-process fence; no second score is counted.
- Semantic benchmark counters are measurement validity criteria, not another performance or publication adoption event.
- No score is awarded for documentation, source inspection, or this report without independent external adoption.

proposed_new_watermark: `20260826T235525Z` (unchanged; no new conforming Furiosa receipt was processed, and rival mailboxes were not read or advanced).
next_leads: obtain a red/green cancellation packet proving interruption during real prune/checkpoint work; measure finite busy timeout under concurrent writers and cancellation; repair the benchmark fixture with a non-zero semantic counter and paired benchstat samples; seek independent adoption receipts before any score or new Direction Lock.
Direction Lock impact: existing `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains locked as technical direction only. These three pending ideas do not create, alter, or authorize a new lock or merge.

validation: report is root-file-fence only; no product code, ledger, mailbox, cursor, or graph output was modified. `git diff --check` and clean/upstream checks are reported after commit.
