# Tick 25 external mechanisms: bounded research

Base inspected: `878f631b74e68aa76302f382e28096dc3d60b545`.

Scope is limited to three additional exact mechanisms for (a) concurrent replace/rebuild versus direct tag/vector writers, (b) scan-heavy tombstone pruning, and (c) all-project fallback/duplicate writers. No product files were changed.

## Findings

### PA-SQLITE-BEGIN-IMMEDIATE-001 — in-database writer admission

- External precedent: [SQLite Transactions](https://www.sqlite.org/lang_transaction.html), SQLite documentation, updated 2026-08-24 (HTTP `Last-Modified`), accessed 2026-08-27.
- Inspected mechanism: `BEGIN IMMEDIATE` starts a write transaction immediately; SQLite says it can fail with `SQLITE_BUSY` when another write transaction is active. The database's single-writer rule is therefore the admission boundary, rather than an application-side snapshot/rename race.
- RawClaw touchpoint: `internal/index/consolidated.go:553-632` (`SyncConsolidatedFrom`), `:1166-1246` (`pruneTombstoned`), and direct writers named by `graphify` (`runTagWrite`, vector top-up).
- Precise invariant: no rebuild snapshot or direct tag/vector mutation may proceed as an admitted writer unless it owns the database write transaction; `SQLITE_BUSY` is retryable and must not be reported as successful publication.
- Smallest adaptation: at the start of each write-through/rebuild/direct-write unit, issue `BEGIN IMMEDIATE` on the same connection used for the mutation; use the existing bounded busy/retry policy, and commit before any receipt or rename-visible success. Do not hold this transaction across unrelated project scans.
- Why non-duplicate: the log's universal `consolidated.lock` recommendation is an external file-fence around publication. This is an SQLite-native, connection-level writer admission primitive that also serializes writers which bypass the file fence; it is not the already logged atomic-commit durability recommendation.
- Adoption grade: no immutable external RawClaw adoption evidence found; pending, score `0`.
- Normalized recommendation text: `Use SQLite BEGIN IMMEDIATE as a per-database writer admission gate: begin the write transaction before reading the rebuild snapshot, so competing tag/vector/rebuild writers serialize and SQLITE_BUSY is handled as retryable.`
- Recommendation fingerprint SHA-256: `7e0c7cf321c3b845baf8518173f636fdf86eda2d81b69cd8b00b635cff80b214`.

### PA-FTS5-DELETEMERGE-001 — tombstone-density-triggered bounded merge

- External precedent: [SQLite FTS5](https://www.sqlite.org/fts5.html#the_deletemerge_configuration_option), SQLite documentation, updated 2026-08-24 (HTTP `Last-Modified`), accessed 2026-08-27.
- Inspected mechanism: FTS5 contentless-delete tables retain delete markers as tombstones; `deletemerge` makes a b-tree eligible for merge once a configurable tombstone percentage is reached. The `merge` command accepts a page budget and can be repeated until `sqlite3_total_changes()` shows no work; the `optimize` equivalent is explicitly described as repeated negative-budget merges.
- RawClaw touchpoint: `internal/index/consolidated.go:529-531` and `:619-621` call `pruneTombstoned` in the foreground; `:1166-1246` scans tombstones and deletes session/message/topic sidecars in one transaction.
- Precise invariant: deletion cleanup must eventually remove stale index state, but closeout latency must be bounded by a finite cleanup slice; each slice is idempotent and may stop without claiming full cleanup when new work remains.
- Smallest adaptation: replace one unbounded tombstone-prune pass with a persisted/observable candidate threshold and bounded batches (for example, N affected sessions or SQLite page budget), then continue in detached consolidation. Preserve source-scoped sidecar deletion semantics; this is scheduling/merging, not permission to delete co-contributor rows.
- Why non-duplicate: existing recommendations address orphan sidecar selection and detached phase timing. This is the SQLite FTS-specific tombstone-density and page-budget algorithm for amortizing scan/merge work; it does not choose which sessions are safe to delete.
- Adoption grade: no immutable external RawClaw adoption evidence found; pending, score `0`.
- Normalized recommendation text: `Use FTS5 deletemerge plus bounded incremental merge: make tombstone density trigger eligibility, then issue bounded merge work in background and stop when sqlite3_total_changes shows a no-op, keeping tombstone cleanup off closeout's foreground path.`
- Recommendation fingerprint SHA-256: `21ae4bb81dff3c0531f08b62290258442aaea11b75949aeb2e7bca2996e240a2`.

### PA-GO-SINGLEFLIGHT-FALLBACK-001 — duplicate suppression per fallback key

- External precedent: [golang.org/x/sync/singleflight](https://github.com/golang/sync/blob/master/singleflight/singleflight.go), Go Authors, latest inspected file commit `2026-05-30T15:55:25Z`, accessed 2026-08-27.
- Inspected source: `singleflight/singleflight.go`, `Group.Do` and `Group.Forget`. `Do` guarantees one in-flight execution for a key; duplicate callers wait and receive the same result, and the key is removed after completion.
- RawClaw touchpoint: `internal/index/consolidated.go:1320-1361` (`PerProjectDBs`/freshness discovery), `internal/cli/tagrefresh.go:24-30` (`runTagPrepCmd`), and fallback paths that can spawn duplicate ingest/tag/vector writers.
- Precise invariant: for one project/database and operation generation, at most one fallback ingest/rebuild writer is active; duplicate triggers coalesce onto that result rather than independently writing the same database.
- Smallest adaptation: one package-local `singleflight.Group`; key by canonical database path plus operation (and generation/freshness token if needed). Wrap only fallback-triggered ingest/rebuild. Keep durable freshness metadata and the existing writer fence; `singleflight` suppresses in-process duplicates but is not cross-process ownership or a completion receipt.
- Why non-duplicate: Kubernetes scoped reconciliation and the log's SQLite UPSERT/CAS/outbox recommendations concern source ownership, durable identity, or publication. `singleflight` is a minimal in-process trigger coalescer specifically preventing duplicate fallback writers before they reach those durable mechanisms.
- Adoption grade: no immutable external RawClaw adoption evidence found; pending, score `0`.
- Normalized recommendation text: `Use golang.org/x/sync/singleflight.Group.Do keyed by project/database and operation: only one in-flight fallback ingest/rebuild runs per key, while duplicate callers wait for and share the first result, preventing duplicate writers without durable queue machinery.`
- Recommendation fingerprint SHA-256: `1532e53cf1b582d958f6fec89bcb723cf2da7681bc696a5b7cfbc0fe4bf3465a`.

## Re-grade and exclusions

No immutable external adoption receipt was found for any pending recommendation in the cumulative log, so all three remain score `0`. Existing recommendations were not re-graded because this bounded search found no immutable adoption evidence. The three mechanisms are intentionally complementary: SQLite admission, FTS5 cleanup budgeting, and in-process duplicate suppression.
