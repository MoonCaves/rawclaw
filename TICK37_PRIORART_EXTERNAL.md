# Tick 37 external prior-art expansion

Run completed `2026-08-27T01:20:35Z` UTC. Audit base:
`0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
The cumulative ledger was read first; pre-run SHA-256:
`fa4941faf71dbc4dded6b4f9b544ab55417852e6368b6b27a3a5ef885e36bb51`.

## Regrade before new recommendations

| Existing recommendation | Current status | Evidence boundary |
|---|---|---|
| `PA-SQLITE-PROGRESS-BUDGET-001` (`6d296d6f...`) | `NARROWED`, score 0 | Tick 34 mutation found modernc.org/sqlite v1.45.0 supports context interruption through `ExecContext`/`QueryContext`, but exposes no public progress-handler registration API. Do not claim SQLite C progress callbacks are available through RawClaw’s driver. |
| `PA-SQLITE-BUSY-TIMEOUT-001` (`69634be...`) | `DUPLICATE`, score 0 | RawClaw already sets `_pragma=busy_timeout(5000)` for RO and `10000` for RW. A finite timeout is not a caller’s 200ms context budget. |
| `PA-SEMANTIC-BENCH-COUNTER-001` (`c0bb590...`) | `CONFIRMED` validity rule, unadopted, score 0 | `B.ReportMetric` can report zero useful work while the benchmark exits green; an explicit non-zero work assertion is required. |
| `PA-SQLITE-BEGIN-IMMEDIATE-001` | `NARROWED`, score 0 | Tick 34 modernc reproduction moved lock failure from first write to admission with `_txlock=immediate`, but `busy_timeout(10000)` still delayed a 200ms context about 10.207s. |

No prior recommendation crossed the adoption threshold. Existing totals remain
Furiosa `+9`, Han `+2`, Ozzy `+3`; this run adds no score event.

## Additional external sources

Access time for all sources: `2026-08-27T01:19:32Z` UTC.

1. `https://github.com/tailscale/sqlite/blob/master/sqlite.go` — Tailscale
   SQLite driver connection/transaction implementation; current `master`
   resolved by `git ls-remote` to `8034b3e3f7a53544a8089c3682af35b0081c27ddbd1f283f316df3093513a83c`
   (update date unavailable; source ETag
   `94025a464a3c24a7e86d0018f6be3cfd495e81fa5b46755f7bd64bde70e33f5f`).
   Inspected `txInit`: writable transactions issue `BEGIN IMMEDIATE`; inspected
   `ExecContext`/`QueryContext`: opt-in `WithQueryCancel` calls the underlying
   SQLite interrupt and waits for the cancellation handler before returning.
   Relevance: strongest mature Go precedent for combining immediate writer
   admission with cancellation; it requires driver-level interrupt access and
   does not prove modernc exposes the same API.

2. `https://www.sqlite.org/c3ref/busy_handler.html` — SQLite C Interface:
   Register A Callback To Handle SQLITE_BUSY Errors; Last-Modified
   `2026-08-24T14:49:52Z`; accessed above. Inspected
   `sqlite3_busy_handler`: callback receives retry count and can return zero to
   stop waiting. Relevance: a deadline-aware busy callback can bound lock wait
   by caller policy, unlike a fixed 10-second timeout. This is C API evidence,
   not RawClaw modernc-driver evidence; no supported public modernc hook was
   found.

3. `https://pkg.go.dev/testing#B.ReportMetric` — Go `testing.B.ReportMetric`;
   update date unavailable; accessed above. Inspected the standard benchmark
   metric API and its requirement that callers supply a named metric. Relevance:
   semantic work can be reported beside latency, but this API performs no
   correctness assertion; the benchmark must separately fail on zero intended
   work. This source is an enrichment of the existing semantic-counter
   recommendation, not a new recommendation.

## New or deduplicated mechanisms

### `PA-SQLITE-INTERRUPT-BEGIN-001`

Normalized text: `Use a driver-supported interrupt/cancellation hook around
BEGIN IMMEDIATE so a canceled lock wait returns incomplete and never publishes
terminal success.` Fingerprint SHA-256:
`105d41020b8678e8a376b20bf41ef13ba8c27f6f200e39bbfed664702b8fc7c2`.

Status: `PENDING`, score 0. The Tailscale source is a concrete mature-driver
precedent, but RawClaw’s modernc v1.45.0 applicability is unproven. The smallest
next experiment is a disposable two-pool test using the actual modernc driver:
hold a write lock, call `BeginTx`/`BEGIN IMMEDIATE` with a 200ms context, and
verify whether admission returns by the deadline or remains blocked by
`busy_timeout`. If no driver-supported interrupt hook exists, reject this as an
unimplementable adapter claim rather than importing a C-only API.

Duplication boundary: distinct from `PA-SQLITE-BEGIN-IMMEDIATE-001` because
that recommendation changes lock admission mode only; this one requires
context-driven interrupt and incomplete-publication semantics. Distinct from
the existing busy-timeout recommendation because it aborts via caller context,
not elapsed fixed timeout.

### `PA-SQLITE-BUSY-HANDLER-DEADLINE-001`

Normalized text: `Use sqlite3_busy_handler callback deadline accounting instead
of a fixed busy timeout, returning zero at context expiry and retrying only
before terminal publication.` Fingerprint SHA-256:
`cd5c50923ed6c22c45079656c5e6715364bc7460c9da00405956296c91c75530`.

Status: `NARROWED`, score 0, not a new adopted mechanism. SQLite officially
documents the callback, but modernc v1.45.0 has no supported public API exposing
it, and RawClaw already has a busy-timeout configuration. Retain only as an
external design comparator. Next experiment: inspect the driver’s public
connection seam and prove a callback can be installed without unsafe private
type access; otherwise reject.

Duplication boundary: not a second busy-timeout recommendation; it is a
callback-based variant, and it cannot be credited unless the actual driver
supports it.

### Existing `PA-SEMANTIC-BENCH-COUNTER-001` enrichment

Normalized text remains: `Require closeout performance benchmarks to assert
non-zero intended work and report a semantic work counter, such as rows
examined, deleted, or committed, alongside latency so a no-op optimization
cannot pass.` Existing fingerprint remains
`c0bb59011b65af9866dccc35ba701b834ec6daebfaed3ecb95a1fcfc1d83d11c`; no new
ID or score. `ReportMetric` is useful output, not the guard. Next experiment is
the already-required mutation: zero live IDs and return-before-delete must make
the benchmark fail, while the restored candidate reports non-zero deleted rows.

## Maintenance cancellation and publication boundary

The Tailscale `WithQueryCancel` pattern is the only additional source here that
directly spans live SQL cancellation. It interrupts an executing statement and
waits for the handler before returning, while its transaction initialization
uses `BEGIN IMMEDIATE`. RawClaw still needs its own incomplete-result contract:
cancellation before commit must not advance a watermark or terminal receipt;
publication remains valid only after commit. This is a proposed adaptation,
not evidence that modernc supports the same public interrupt primitive.

No external adoption, independent green receipt, Direction-Lock invalidation,
or merge authorization was observed. Scheduler/control text and prior internal
mutation reports are not external adoption evidence.
