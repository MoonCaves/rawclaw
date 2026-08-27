# Tick 49 modernc.org/sqlite v1.45.0 source audit

Audit scope: official, version-pinned modernc.org/sqlite v1.45.0 and Go
database/sql. Base: `ef2eebf414e77086be06281539c5a50ba036a32a`. Prior-art
watermark read before research: `20260827T030805Z` (the cumulative ledger's
Tick 47/48 entry). This is report-only; no product code or cumulative ledger
was edited.

## Verdict

modernc v1.45.0 supports cancellation of an executing statement through the
standard `database/sql` Context APIs, but it does not expose a supported public
seam that cancels SQLite's lock-admission wait. Its internal implementation
uses `sqlite3_interrupt` for context cancellation while a statement is
executing, and it uses SQLite `unlock_notify` for one internal
`SQLITE_LOCKED` shared-cache retry. Neither is a caller-context-aware busy
admission mechanism. The smallest valid RawClaw adaptation is therefore
context-aware admission before entering SQLite, followed by
`BeginTx`/`ExecContext`/`QueryContext` and explicit incomplete-result handling.
Do not claim that modernc's context support bounds a first write blocked in the
busy handler.

## Exact modernc v1.45.0 mechanisms

Source identity: module sum `h1:r51cSGzKpbptxnby+EIIz5fop4VuE4qFoVEjNvWoObs=`;
VCS tag `v1.45.0`, GitLab commit
`b8975b7dcf269f7c09929073c1feb64701066f41`.

* `(*conn).ExecContext` and `(*conn).QueryContext` are implemented in
  `conn.go` and delegate to `stmt.exec`/`stmt.query` with the caller context.
  `stmt.exec` and `stmt.query` install the package-private
  `interruptOnDone(ctx, c, &done)` goroutine. On context cancellation that
  helper invokes package-private `(*conn).interrupt`, which calls
  `sqlite3.Xsqlite3_interrupt`. The deferred completion path maps an
  interrupted operation back to `ctx.Err()` and finalizes the statement.
  This is executing-statement cancellation, not guaranteed lock admission.

* `(*conn).BeginTx` calls package-private `begin`, which constructs a `tx`.
  `newTx` executes `begin`, `begin immediate`, or `begin exclusive` through
  `sqlite3_exec`. `tx.exec` also installs `interruptOnDone` when a context is
  supplied. Thus the context is wired into the transaction-start SQL, but
  SQLite's busy handler can still sleep/retry before the interrupt is observed;
  the API contract does not promise a context deadline for this wait.
  `_txlock=immediate` is a mode selector, not an admission cancellation API.

* `conn.go`'s `step` retries only `SQLITE_LOCKED | (1 << 8)` (shared-cache
  locking) through private `retry`, which invokes generated
  `sqlite3_unlock_notify`, waits on a native mutex, resets the statement, and
  retries. There is no context or deadline select around this wait. This is
  distinct from ordinary `SQLITE_BUSY` caused by another process/connection.

* The generated `lib/sqlite_<platform>.go` contains bindings for
  `sqlite3_busy_handler`, `sqlite3_busy_timeout`, `sqlite3_interrupt`,
  `sqlite3_progress_handler`, and `sqlite3_unlock_notify`, but those generated
  bindings are not the public Go API. The package's public API exposes no
  `BusyHandler`, progress-handler, interrupt, or unlock-notify registration
  function accepting a caller context.

* The public `Driver.RegisterConnectionHook` and package-level
  `RegisterConnectionHook` do provide a supported connection hook. Its
  callback receives only `ExecQuerierContext` (the `driver.ExecerContext` and
  `driver.QueryerContext` interfaces), not a raw SQLite handle. The exported
  `HookRegisterer` permits pre-update, commit, and rollback hooks; these are
  observation/transaction hooks and do not expose busy-handler or interrupt
  control. The concrete `*conn` is unexported.

* `Limit(*sql.Conn, id, newVal)` is a public helper using `sql.Conn.Raw` and
  type-asserting the unexported concrete connection internally. It changes
  SQLite limits only; it is not a general raw-connection escape hatch and does
  not expose `sqlite3_interrupt`, progress, busy, or unlock-notify callbacks.

* Package initialization applies `_pragma` DSN parameters using
  `c.exec(context.Background(), ...)`; the package sorts `busy_timeout` first.
  This supports a finite busy timeout configured by DSN/PRAGMA, but a timeout
  is an elapsed-time cap returning `SQLITE_BUSY`, not cancellation by the
  caller's context.

## Exact Go database/sql contract

Official Go 1.24.6 immutable source:
<https://github.com/golang/go/blob/go1.24.6/src/database/sql/sql.go>, tag
commit `7f36edc26d4e3becb6d9c9008ff00f260bb19055`.

The package documentation states: “Drivers that do not support context
cancellation will not return until after the query is completed.” `DB.BeginTx`
documents that its context is used until commit/rollback and that cancellation
causes the sql package to roll back; `Tx.Commit` returns an error if the
BeginTx context was canceled. `DB`/`Tx` `ExecContext` and `QueryContext` pass
the context to the driver's optional context interfaces. `database/sql` can
select a connection while waiting for pool availability, but it cannot impose
caller cancellation on a driver operation that has entered a non-cancelable
busy wait. Therefore `database/sql` does not add a missing lock-admission seam.

## RawClaw adaptation boundary

1. Acquire the existing process-local writer token with `select { case <-ctx.Done(): ... }`
   before taking the SQLite path; retain the cross-process fence.
2. Pass the live context to `BeginTx`, every `Tx.ExecContext`/`Tx.QueryContext`,
   and the watermark write. Treat `context.Canceled`, `context.DeadlineExceeded`,
   interrupted errors, and busy expiry as incomplete. Never publish success or
   a freshness watermark on those paths.
3. Keep a bounded `busy_timeout` only as a retryable fallback. It is not a
   substitute for admission cancellation and should not be described as one.
4. A connection hook may configure ordinary per-connection pragmas, but cannot
   implement a context-aware busy callback using the supported v1.45.0 API.
   Reaching generated `lib` bindings or the unexported `conn` would require an
   unsupported/private seam and is not a valid sovereign-core adaptation.

## Prior-art IDs and regrade

* `PA-GO-CONTEXT-WRITER-TOKEN-001`: **partial**, score 0. modernc context
  cancellation confirms execution cancellation; it does not solve first-write
  busy admission.
* `PA-SQLITE-INTERRUPT-BEGIN-001`: **rebutted/narrowed**, score 0 for lock
  admission; retained only for executing-statement interruption. `BEGIN
  IMMEDIATE` changes where contention appears but does not make busy waits
  context-bounded.
* `PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001`: **partial**, score 0. Context
  cancellation and SQLite transaction rollback still require publication to
  wait for a successful commit; modernc exposes no new commit/publish seam.
* Existing busy-timeout, busy-handler, progress-handler, WAL, weighted-token,
  and detached-receipt families remain duplicate or negative applicability
  evidence. No new recommendation ID is warranted.

## Rival adoption and score

The public `MoonCaves/rawclaw` commit query after the watermark
(`since=2026-08-27T03:08:05Z`) returned no commits. Local rival refs likewise
contained no post-watermark adoption commit matching modernc context,
interrupt, busy admission, or watermark publication. Local worker/report refs
are not adopter receipts. Score change: **0**; no external event was observed.

## Sources

* modernc package docs, versioned: <https://pkg.go.dev/modernc.org/sqlite@v1.45.0>
  (API/version anchor; inspected 2026-08-27).
* modernc v1.45.0 immutable source: <https://gitlab.com/cznic/sqlite/-/tree/v1.45.0>
  (driver, context interruption, transaction, hooks, and generated SQLite
  bindings; inspected 2026-08-27).
* Go 1.24.6 `database/sql` immutable source: <https://github.com/golang/go/blob/go1.24.6/src/database/sql/sql.go>
  (context/driver contract; inspected 2026-08-27).

No implementation approval, score credit, direction lock, or merge
authorization is created by this report. **NO MERGE AUTHORIZATION.**
