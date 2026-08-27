# PA-SQLITE-BEGIN-IMMEDIATE-001 — Tick 34 mutation referee

## Verdict

**CONFIRMED, narrowly.** RawClaw's current writer transaction admits without
the SQLite write lock: `internal/index/containers.go:391` calls `con.Begin()`,
and `internal/store/store.go:340`'s `ConnectRW` DSN has WAL and
`busy_timeout(10000)` but no `_txlock`. `SetMaxOpenConns(1)` at
`internal/store/store.go:345` limits one `*sql.DB` pool, not other pools or
processes using the same database file.

modernc.org/sqlite v1.45.0 explicitly supports `_txlock=deferred|immediate|exclusive`
(`driver.go:73-75`). Its `BeginTx` implementation emits `begin <mode>` only
when that DSN mode is configured (`tx.go:newTx`), so database/sql `BeginTx`
does not imply `BEGIN IMMEDIATE`.

## Smallest reproduction

Disposable test (restored byte-exactly afterward) used the actual modernc
driver and `ConnectRW` for the current path. Two independent pools opened the
same temporary WAL database. Holder began a transaction and inserted one row,
retaining the writer lock. Contender used a 200ms context.

Command (exact filter matched one top-level test and two subtests):

```text
gofmt -w internal/store/begin_immediate_repro_test.go
go test -run '^TestBeginImmediateRepro$' -count=1 -v ./internal/store
```

Observed output (run hash `d1366c1e5568b620568286fa86b53b78f28b46e9c7b7dd322d14c4ef96b78f4d`):

```text
case=deferred-current-ConnectRW expected=BEGIN admit_err=<nil> admit_elapsed=18.958µs write_err=context deadline exceeded write_elapsed=10.205303083s ctx_err=context deadline exceeded
case=immediate-txlock expected=BEGIN IMMEDIATE admit_err=database is locked (5) (SQLITE_BUSY) admit_elapsed=10.207439958s write_err=<nil> write_elapsed=0s ctx_err=context deadline exceeded
PASS
```

The first row proves the current path's lock failure occurs at the first write,
not admission. The second proves `_txlock=immediate` moves it to admission,
but with RawClaw's 10s busy timeout the bounded context does not make admission
finish within 200ms. The exact driver error is `database is locked (5)
(SQLITE_BUSY)`.

## Boundary and test needed

No permit or cross-database fence is involved in this path; this is per-file
SQLite admission only. `Cross-process AcquireConsolidatedFence` would not prove
this behavior and was not used.

The smallest product test should use two independently opened `ConnectRW`
pools, hold a transaction after its first write, then assert that a contender
using the proposed admission mechanism fails at `BeginTx` with SQLITE_BUSY (or
that a deliberately bounded busy policy returns within the context). It must
also assert the current baseline: `BeginTx` succeeds and the first write fails.
The test must inspect both elapsed phases and `ctx.Err()`, because a 200ms
context alone does not bound modernc's 10s SQLite busy handler.

No product fix was implemented. Disposable Go test was deleted; final worktree
contains only this report.

