# Tick 52 prior-art cancellation delta

## Run and adoption ruling

- `run_timestamp`: `2026-08-27T04:10:58Z` (`date -u`).
- `prior_watermark`: `20260827T032519Z` (the last authoritative conforming
  Furiosa receipt in the supplied ledger).
- `last_launch_grade`: `rejection` for new adoption. The Tick 49 durable-receipt
  addendum (`liteq` and `backlite`) supplied useful implementation comparators,
  but both were duplicate enrichment of existing atomic-commit, outbox,
  detached-terminal-receipt, and lease families. No independent adopter,
  immutable green receipt, or score-eligible event was present.
- `scope`: caller-context-aware SQLite writer admission and busy wait;
  transaction-bound watermark publication and rollback; durable pending-to-
  terminal receipts with lease/retry/idempotent commit; and an authoritative
  write surface surviving snapshot-and-rename.
- `score_eligible_events`: none. Score delta `0`; authoritative totals remain
  Furiosa `+9`, Han `+2`, Ozzy `+3`.

## Independently inspected external mechanisms

### PA-SQLITE-UNLOCK-NOTIFY-ADMISSION-001

- URL: https://www.sqlite.org/unlock_notify.html
- Title: `Using the sqlite3_unlock_notify() API`.
- Publication/update date: official page HTTP `Last-Modified` was not exposed
  by the fetch; inspected `2026-08-27T04:10:58Z`.
- Immutable evidence: official SQLite documentation and its embedded C
  example; SQLite does not publish a Git commit for this documentation page.
- Exact mechanism: on `SQLITE_LOCKED`, register `sqlite3_unlock_notify()` with
  a callback, block on a condition variable until the blocking transaction
  ends, retry the statement, and treat a returned `SQLITE_LOCKED` as a
  deadlock condition. The documented recipe explicitly handles callback
  delivery racing the wait setup.
- Relevance: a precise lock-admission wait primitive, stronger than sleeping
  or a fixed busy timeout, and useful as a comparator for a context-aware
  writer-admission design.
- Limitations: the API is for SQLite shared-cache locking, is available only
  when the library is built with the relevant unlock-notify support, and is a
  C connection-level callback. The page supplies no Go `context.Context`
  cancellation contract, no deadline-aware callback policy, and no durable
  watermark or terminal-receipt publication. It cannot replace RawClaw's
  cross-process `AcquireConsolidatedFence`.
- Recommendation status: `rejected-as-new-adoption` / duplicate-enrichment;
  score `0`. It does not supersede `PA-GO-CONTEXT-WRITER-TOKEN-001` or
  `PA-SQLITE-INTERRUPT-BEGIN-001`; modernc v1.45.0 still has no verified
  supported public unlock-notify or busy-handler seam.
- Normalized recommendation text: `Use a driver-supported SQLite unlock-notify
  admission wait that observes caller cancellation and deadline, returns
  incomplete on deadlock or cancellation, and retains the cross-process fence;
  publish no watermark or terminal receipt before commit.`
- Recommendation fingerprint SHA-256:
  `d19d221314d63b737757cba13a6e989dd5003e3419a3d3c071bde9c15b20a4e8`.

### PA-RIVER-TRANSACTIONAL-RECEIPT-001

- URL: https://github.com/riverqueue/river/tree/v0.23.0
- Title: `River` v0.23.0.
- Publication/update date: tag commit timestamp was not relied on; inspected
  `2026-08-27T04:10:58Z`.
- Immutable evidence: tag `v0.23.0`, commit
  `99e45ec023354d8670f3314fe1909b85e860ecd0`.
- Exact mechanism: `example_complete_job_within_tx_test.go` begins a database
  transaction with the live context, performs application work, calls
  `river.JobCompleteTx` to transition the job to completed in that same
  transaction, and commits only after both the work and completion transition
  succeed. The generated SQL uses `QueryRowContext`; River's job model carries
  stable kind/unique identity, attempt, state, and finalized timestamps for
  retry and terminal-state handling.
- Relevance: concrete mature precedent for binding application work and a
  durable terminal transition to one commit, with stable identity and retry
  state rather than interpreting worker disappearance as success.
- Limitations: River is a PostgreSQL job system, not a SQLite snapshot-and-
  rename index. The inspected example does not itself demonstrate lease
  reclamation after parent death, SQLite busy admission, or an authoritative
  sidecar that survives replacing the derived database. It requires external
  database/runtime machinery and cannot be adopted into RawClaw's zero-runtime-
  dependency core as-is.
- Recommendation status: `rejected-as-new-adoption` / duplicate-enrichment;
  score `0`. It duplicates the existing durable terminal-receipt/outbox and
  lease families; no new stable recommendation ID is warranted.
- Normalized recommendation text: `Bind pending job identity, application
  work, and terminal success or failure to one context-aware transaction;
  reclaim expired leases and make retries idempotent by stable identity.`
- Recommendation fingerprint SHA-256:
  `e6fae22169da6fc9731dd812893c850fadc7e9c5527d729fb5633b74a4400895`.

### PA-GO-BEGIN-TX-CONTEXT-001

- URL: https://github.com/golang/go/blob/go1.24.6/src/database/sql/sql.go
- Title: Go `database/sql` `DB.BeginTx` contract and implementation.
- Publication/update date: Go 1.24.6 tag; inspected
  `2026-08-27T04:10:58Z`.
- Immutable evidence: tag `go1.24.6`, commit
  `7f36edc26d4e3becb6d9c9008ff00f260bb19055`.
- Exact mechanism: `DB.BeginTx(ctx, opts)` passes the context into driver
  transaction start; the package documents that cancellation rolls back the
  transaction and causes `Tx.Commit` to return an error. The implementation
  creates a child context and schedules rollback when cancellation occurs.
  `DB.Begin()` is explicitly the background-context variant.
- Relevance: authoritative contract for transaction-bound source/derived rows
  and watermark publication: cancellation, statement failure, or commit
  failure must withhold terminal success, and watermark advancement must occur
  only in the committed transaction.
- Limitations: `database/sql` cannot guarantee prompt return when the driver
  does not support cancellation; it does not create a context-aware SQLite
  busy-admission callback. It also cannot make separate snapshot-and-rename
  files one authoritative write surface.
- Recommendation status: `partial` / duplicate-enrichment; score `0`. This
  independently confirms `PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001` and the
  existing transaction-bound watermark family, but does not close their
  modernc first-write admission gap.
- Normalized recommendation text: `Use BeginTx with the live caller context;
  keep source rows, derived rows, and watermark or terminal receipt in one
  transaction; publish success only after Commit and treat cancellation as
  incomplete.`
- Recommendation fingerprint SHA-256:
  `fd26e32939b0cc233abec56b05325b10f60b8094deb636b23b6d18ec26c9471a`.

## Regrade and deduplication

- No source shows external adoption of a RawClaw product change after
  `20260827T032519Z`. Same-family or self adoption scores zero. Local worker
  branches, report commits, scheduler/control receipts, and source inspection
  are not adopter receipts.
- `sqlite3_unlock_notify` is distinct from a fixed busy timeout but remains a
  C/shared-cache comparator, not a supported RawClaw modernc implementation.
- River's transaction-bound completion is distinct in product shape from the
  SQLite watermark family, but not distinct as a durable terminal-receipt
  recommendation; no second score is counted.
- `BeginTx` confirms cancellation/rollback semantics but is not writer
  admission, not a busy handler, and not a separate authoritative write
  surface. It therefore receives no new ID or score.
- No recommendation is superseded. `PA-CONSOLIDATED-SIDECAR-PRUNE-001`
  remains technically `LOCKED` on its recorded base/candidate/gates/adopter
  receipt, with `NO MERGE AUTHORIZATION`.

## Direction Lock

```text
direction_lock:
  recommendation_id: PA-CONSOLIDATED-SIDECAR-PRUNE-001
  recommendation_fingerprint_sha256: d07f69f8d056f9f145bd9a864e3fa11660afadf13af3aca9acad39ea722bcb72
  exact_base_sha: 878f631b74e68aa76302f382e28096dc3d60b545
  candidate_shas_or_patch_ids: a78b39b3d87c82a4f83878359afc98e2b8fde2d4 (rejected), c38f79acf9c9ae43ebd091a95f36837f43c0e423 (winner), 0cd00e44c7eb87e30fcf72f8ae790e7060635b09 (adapted)
  decisive_test_filter: ^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$
  gate_commands: CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$'; CGO_ENABLED=0 go test -p 1 -race -count=1 ./internal/cli ./internal/index
  external_adopter_receipt: /Users/jay-m4/code/rawclaw/.agent-mailbox-ozzy/20260826T224735Z-5b061dd1-external-adoption-c38-current-.md
  invalidation_triggers: base change; production patch change; mutation failure; focused/full gate regression; adoption receipt invalidation
  status: LOCKED; technical direction only; NO MERGE AUTHORIZATION
```

## Watermark and next leads

- `new_watermark`: `20260827T040941Z`, the newest conforming top-level receipt
  actually processed in this dedicated inbox before completion. It is not
  future-dated relative to the captured completion.
- `delta_timestamp_utc`: `2026-08-27T04:10:58Z`.
- `run_completion_utc`: `2026-08-27T04:12:50Z`, captured with `date -u` before
  final validation and commit.
- Next leads: obtain a supported modernc first-write cancellation/admission
  proof; isolate transaction-layer entry mutations for `BeginTx` and watermark
  publication; and require an independent process-death terminal-receipt
  adoption before scoring.
