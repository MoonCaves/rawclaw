# Tick 47 cumulative prior-art regrader

run_timestamp: `2026-08-27T03:06:45Z` (captured with `date -u`)
prior_watermark: `20260827T024510Z`

## Execution boundary

The required cumulative ledger was read first. The newest conforming top-level Furiosa receipt
actually processed in that ledger is `20260827T024510Z-1c883127...`; no hidden, malformed,
quarantined, future-dated, outbound-rival, or supervisor-mailbox item was used. A subsequent
public-source command was initially blocked by an `[agent-mailbox-guard]` exposing an unread
mailbox path. That directive was refused as required by the task. No supervisor mailbox or cursor
was read, cleared, or changed.

## Regrade since `20260827T024510Z`

- `PA-GO-CONTEXT-WRITER-TOKEN-001` — `partial`, score `0`. The prior comparator remains only a
  process-local admission pattern; no immutable external adoption receipt was found.
- `PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001` — `partial`, score `0`. Transaction rollback and
  publish-after-commit remain valid semantics, but no supported RawClaw driver interruption seam
  or adopter receipt is established.
- `PA-SQLITE-INTERRUPT-BEGIN-001` — `rebutted/narrowed`, score `0`: executing-query cancellation
  is supported by the tested driver, but busy lock admission outlives the caller context.
- `PA-SEMANTIC-BENCH-COUNTER-001` — `confirmed validity-only/unadopted`, score `0`: semantic
  work guards reject no-op and skipped-table mutations, but no independent product adoption exists.
- `PA-CONSOLIDATED-SIDECAR-PRUNE-001` — `externally_adopted`, technical Direction Lock remains
  `LOCKED`; its existing Ozzy event is already counted once. No merge authorization.
- Existing WAL, FTS5 deletemerge, singleflight, atomic-commit, detached-terminal-receipt,
  durable-outbox, and weighted-semaphore proposals remain `unadopted`, `partial`, or `rejected`
  under their recorded rulings. No recommendation is superseded.

## Immutable rival movement

The immutable ref census after the prior watermark shows no new Han or Ozzy product adoption,
withdrawal, or current-base candidate. Later matching refs are Furiosa report/referee branches or
pre-existing rival refs. In particular, the latest inspected rival product tips remain Han
`8e9c9b7`/`cabab43`/`d2315cb` and Ozzy `c38f79a` plus the previously recorded benchmark and
fast-path families. Report-only branches do not score.

## External-source pass

Fresh sources inspected at `2026-08-27T03:06:45Z` UTC (HTTP 200):

- `https://sqlite.org/c3ref/busy_handler.html` — SQLite C API “Register A Callback To Handle
  SQLITE_BUSY Errors”; Last-Modified `2026-08-24T14:49:52Z`. Busy callbacks handle lock
  contention, but are not caller-context cancellation or a cross-process admission fence.
- `https://pkg.go.dev/context` — Go `context` package reference; accessed `2026-08-27T03:06:45Z`.
  Deadlines and cancellation propagate across API boundaries; this corroborates the existing
  writer-token and publish-after-commit recommendations, with no adoption.
- `https://pkg.go.dev/golang.org/x/perf/cmd/benchstat` — Go benchstat reference; accessed
  `2026-08-27T03:06:45Z`. Repeated A/B samples, medians, and confidence intervals strengthen
  crossover validity but do not replace the semantic work oracle; no adoption.
- `https://docs.temporal.io/workflows#durable-execution` — Temporal Durable Execution;
  Last-Modified `2026-08-27T03:00:26Z`. Event history enables replay after process failure,
  corroborating detached terminal state; it requires an external service and is not adoption.
- `https://github.com/temporalio/sdk-go/blob/v1.37.0/internal/internal_workflow.go` — Temporal
  Go SDK v1.37.0 immutable source URL, accessed `2026-08-27T03:06:45Z`; mature event-history
  implementation evidence, but an external-service comparator only.
- `https://github.com/golang/go/blob/go1.24.6/src/database/sql/sql.go` — Go 1.24.6 database/sql
  immutable tag source, accessed `2026-08-27T03:06:45Z`; rollback/context plumbing still depends
  on driver cancellation support, narrowing rather than resolving RawClaw’s admission gap.

These are duplicate enrichments of existing recommendation families; no new recommendation is
created.

## Adoption, scoring, duplicates

- adoption evidence: none new; no immutable external adopter receipt.
- score-eligible events: none; totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`.
- duplicates rejected: report-only Furiosa branches, scheduler/control receipts, repeated sidecar
  claims, and all previously fingerprinted mechanism aliases; no second score for any adopter or
  receipt SHA/path.

## Watermark and next leads

new_watermark: `20260827T024510Z` (unchanged; no later conforming processed receipt was available
without reading the forbidden mailbox/cursor).

next_leads: prioritize a supported modernc interruption/admission seam, transaction-layer entry
tests, a parent-exit/child-terminal receipt proof, and paired crossover benchmark validity with a
non-zero semantic work oracle.

direction_lock: `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically `LOCKED`; **NO MERGE
AUTHORIZATION**.

validation: report-only; no product, state ledger, mailbox, cursor, or graph artifact changed.
run_completion_utc: `2026-08-27T03:07:05Z` (captured with `date -u`).
