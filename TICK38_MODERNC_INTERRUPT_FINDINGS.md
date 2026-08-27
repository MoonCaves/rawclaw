# Tick 38 modernc interrupt/admission findings

Audit base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` (HEAD at start and
finish). No product change remains; the disposable test was deleted.

## Verdict

**REBUTTED for bounded lock admission; CONFIRMED only for executing-statement
cancellation.** modernc.org/sqlite v1.45.0 has a supported
`database/sql/driver.ConnBeginTx` implementation and internally wires context
cancellation to its connection interrupt, but a lock wait inside SQLite's
busy handler was not interrupted by a 200ms context while RawClaw's
`busy_timeout(10000)` was active. The pending `PA-SQLITE-INTERRUPT-BEGIN-001`
mechanism therefore does not transfer from Tailscale's driver to RawClaw's
public modernc seam for admission. No score or adoption event follows.

## Reproduction

Disposable test used two independent pools on one temporary WAL database. The
holder called `Begin` and inserted a row, retaining the write lock. The
contender used a 200ms context. It ran both `ConnectRW` (current deferred DSN)
and a public `_txlock=immediate` DSN. A third case ran a long recursive query
under the same current helper.

```text
gofmt -w internal/store/modernc_interrupt_repro_test.go
CGO_ENABLED=0 go test -run '^TestModerncInterruptRepro$' -count=1 -v ./internal/store
```

The exact filter selected one top-level test and two subtests; the long-query
case was in that test body. Observed output SHA-256:
`3dad5e1958590ff0d47be5022d2154843756da9384467e34532cb1e46811b177`.

```text
case=current-ConnectRW-deferred admit_err=<nil> admit_elapsed=19.417µs write_err=context deadline exceeded write_elapsed=10.194988333s ctx_err=context deadline exceeded
case=txlock-immediate admit_err=database is locked (5) (SQLITE_BUSY) admit_elapsed=10.212584041s write_err=<nil> write_elapsed=0s ctx_err=context deadline exceeded
case=long-query err=context deadline exceeded elapsed=202.075166ms ctx_err=context deadline exceeded sum=0
PASS
```

The current path admits the deferred transaction immediately, then blocks its
first write for about 10.195s. `_txlock=immediate` moves the failure to
`BeginTx`, but still waits about 10.213s and returns
`database is locked (5) (SQLITE_BUSY)`, far beyond the 200ms context. In
contrast, the executing recursive query returns `context deadline exceeded`
at about 202ms. Cancellation is therefore effective for statement execution,
not bounded for busy lock admission.

## Public symbols and source evidence

The inspected module files and SHA-256 hashes were:

```text
modernc.org/sqlite@v1.45.0/driver.go 9172c39b20af262b3fc462ef76bc037ad7af65113e314789abfae864ffafd0b6
modernc.org/sqlite@v1.45.0/tx.go     52dcd7772e0bda0cb9cfb71645bd5f243e38fd1bfc4848f47d97b506657f8225
modernc.org/sqlite@v1.45.0/sqlite.go b39f671f58c94a134e90a5fb372241e244b65044016f6c82ee2abc387bea8e80
modernc.org/sqlite@v1.45.0/stmt.go  860414776f665afda647d23f71c5442d53b7b633a8c048ed9bb9b3a3611ce7ca
RawClaw internal/store/store.go    8e6b7dd1453ee06fdfffb46bc59e84f66471515fb9a0a49bd41ab938cc7e86ce
```

Relevant public/configured seams:

- `driver.go:73-75`: `_txlock` accepts `deferred`, `immediate`, or
  `exclusive`; default is deferred.
- `conn.BeginTx` in `conn.go:1032-1040`: forwards to `newTx`.
- `tx.go:19-31`: emits `begin` or `begin <mode>` through SQLite exec.
- `database/sql` `BeginTx` and `ExecContext` are the only application seams
  used here. modernc's `interruptOnDone` in `sqlite.go:75-116` is unexported;
  its internal `c.interrupt` call is not a supported RawClaw API to invoke.
- `stmt.go:95-112`: statement execution installs that internal cancellation
  interrupt, matching the observed long-query cancellation.
- RawClaw `ConnectRW` at `internal/store/store.go:339-350` sets WAL,
  `busy_timeout(10000)`, and `SetMaxOpenConns(1)`; the latter is per pool and
  does not serialize independent pools/processes.

Tailscale prior art was compared at verified Git HEAD
`15a02b90c60613ae3b6caa4a07c945cb3c874611`: its public driver implementation
issues `BEGIN IMMEDIATE` and offers `WithQueryCancel` that calls an interrupt.
That is a valid mature-driver precedent, but not evidence that modernc's
unexported interrupt can bound its busy-handler admission wait.

## Narrowed recommendation and next experiment

`PA-SQLITE-INTERRUPT-BEGIN-001` fingerprint
`105d41020b8678e8a376b20bf41ef13ba8c27f6f200e39bbfed664702b8fc7c2` is
**REBUTTED/NARROWED**, score 0: context interruption works for an executing
statement, not for a lock-admission wait with the current busy timeout.

The smallest credible follow-up is either a driver-supported busy callback or
a bounded retry/admission policy that explicitly sets a lock-wait budget lower
than the request deadline and returns incomplete/retryable status. It must
re-run the two-pool test and assert admission elapsed, first-write elapsed,
`ctx.Err()`, and exact SQLite error. A private generated `sqlite3_interrupt`
call or a result observed only after 10 seconds is not sufficient.

No permit/fence was bypassed or tested; this is per-database SQLite admission,
separate from `AcquireConsolidatedFence`.

