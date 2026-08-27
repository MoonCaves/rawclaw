# Tick 45 prior-art research

Run timestamp: `2026-08-27T02:36:17Z` (UTC). Exact base: `ef2eebf414e77086be06281539c5a50ba036a32a`.
Prior-art watermark: `20260827T022635Z`.

The requested `/Users/jay-m4/code/rawclaw-supervisor-furiosa-a/PRIOR_ART_LOG.md` does not exist.
The available cumulative ledger used for deduplication is
`/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/PRIOR_ART_LOG.md`.
This path discrepancy is recorded rather than silently substituting a file.

## Findings

### PA-GO-CONTEXT-WRITER-TOKEN-001 — sharpened, no new ID

* Official source: <https://pkg.go.dev/database/sql#DB.BeginTx>, accessed
  `2026-08-27T02:35:47Z` (HTTP date observed; pkg.go.dev does not expose a stable
  Last-Modified header). `database/sql` states that drivers which do not support
  context cancellation may not return until the operation finishes, and the
  `BeginTx` contract rolls a transaction back when its context is canceled.
* Transferable rule: a context-aware admission gate is useful only before entering
  a possibly uninterruptible driver operation; once inside, cancellation must be
  treated as incomplete and callers must not publish success. This validates the
  existing token-plus-cross-process-fence split, but does not prove that modernc's
  SQLite busy wait is context-bounded.
* RawClaw mapping: `AcquireConsolidatedFence(ctx)` in
  `internal/index/consolidated_fence.go:35`, and the consolidated write path in
  `internal/index/consolidated.go:403,553`; current `SyncConsolidatedFrom` still
  has a non-contextual public wrapper. Every writer must share both gates.
* Limitations: this is a standard-library API contract, not evidence that a
  process-local token exists in RawClaw or that the modernc driver interrupts lock
  admission. Existing ledger fingerprints and the x/sync/pgxpool comparators cover
  the token mechanism; awarding another ID would double-count.
* Evidence quality: A (official API documentation; direct contract wording).
* Status/points: `partial`, score `0`; no adoption, withdrawal, rebuttal, or score
  event since the watermark.

### PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001 — sharpened, no new ID

* Official sources: <https://sqlite.org/c3ref/interrupt.html> (SQLite
  `sqlite3_interrupt`, accessed `2026-08-27T02:00:00Z`, already ledgered) and
  <https://sqlite.org/atomiccommit.html>, Last-Modified
  `2026-08-24T14:49:52Z`, accessed `2026-08-27T02:35:49Z`. SQLite's atomic-commit
  documentation describes commit as the durability boundary; the transaction
  documentation also states that an error can roll back a transaction.
* Transferable rule: a canceled/interrupted transaction is not a successful
  publication. Keep watermark, terminal receipt, and other externally visible
  completion state behind the same successful commit boundary; retry from a clean
  transaction after interruption.
* RawClaw mapping: transaction and publication in `ConsolidateFrom`/
  `SyncConsolidatedFrom` (`internal/index/consolidated.go:402-598`), and watermark
  publication via `StampIngestWatermark` (`internal/index/consolidated.go:1304`).
* Limitations: SQLite's C interrupt semantics are authoritative for SQLite itself,
  but no supported public raw-interrupt seam was proven for modernc v1.45.0. The
  prior mutation evidence specifically found statement cancellation but not a
  context-bounded busy admission wait. This strengthens the invariant, not the
  proposed implementation.
* Evidence quality: A (official SQLite documentation), with the modernc transfer
  limitation independently observed in prior ledger evidence.
* Status/points: `partial`, score `0`; unchanged since `20260827T022635Z`.

### PA-CONSOLIDATED-SIDECAR-PRUNE-001 — corroborated, locked, no new ID

* Official source: <https://kubernetes.io/docs/concepts/architecture/garbage-collection/>,
  accessed `2026-08-27T02:35:51Z`. Kubernetes documents owner references,
  dependent cleanup, orphan handling, and foreground/background deletion. This is
  an independent deployed precedent for deleting derived dependents only after
  authoritative owners are absent, while preserving dependents with another
  owner.
* Transferable rule: sidecar cleanup must be ownership-scoped and conservative:
  remove a derived row only when no authoritative source/owner remains; preserve
  co-contributor rows; make background cleanup observable and retryable.
* RawClaw mapping: `pruneTombstoned`/`pruneTombstonedIDs` in
  `internal/index/consolidated.go:1121-1198`, deleting `topic_segment` and
  `session_verdict` only for sessions without remaining source ownership.
* Limitations: Kubernetes is an API-controller model, not SQLite and not a proof
  of RawClaw's SQL predicate or writer-fence correctness. The existing
  `PA-K8S-SCOPED-RECONCILIATION-001` and locked sidecar recommendation already
  capture this ownership-scoped effect; this source is corroboration only.
* Evidence quality: A- (official deployed-system documentation; analogy is
  explicit and bounded).
* Status/points: `externally_adopted`, technical Direction Lock remains locked on
  base `878f631b74e68aa76302f382e28096dc3d60b545`, with no merge authorization;
  score delta `0`.

## New stable IDs

None. `database/sql` cancellation, SQLite atomic commit, and Kubernetes owner
garbage collection are materially useful additional sources, but each normalizes
to an already-ledgered mechanism. No new fingerprint or score is warranted.

## Re-grade after `20260827T022635Z`

The public GitHub API query
`GET /repos/MoonCaves/rawclaw/commits?since=2026-08-27T02:26:35Z` returned an
empty list (`0` commits). `git ls-remote --heads origin` exposed no new public
main/integration adoption after the watermark. The visible branches remain
worker/report branches, including `han/luna-overlay-publisher-integration-20260827`
at `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`, `han/luna-tag-overlay-20260827`
at `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e`, and
`integrate/tagwrite-closeout-wave1` at `a33ab023eae0ca324956a66cf17b7ffa5b16c39d`.
These are not immutable external adoption receipts and do not establish uptake.

No withdrawal, rebuttal, independent uptake, or score-eligible event was found
for any of the three standing IDs. Silence is not adoption. Score delta: `0`;
totals remain Furiosa `+9`, Han `+2`, Ozzy `+3` pending supervisor adjudication.

## Verdict

Additional official evidence validates the behavioral boundaries, while modernc's
missing interrupt seam still blocks implementation-level confidence. Sidecar
pruning remains the only locked technical direction. No new recommendation IDs,
no score change, and no merge authorization.
