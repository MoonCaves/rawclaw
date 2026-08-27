# Tick 34 prior-art mutation and duplicate referee

Audit base `ef2eebf414e77086be06281539c5a50ba036a32a`. Input report commit `be9efc4192d60a9d916c7747dba085bece9ac07a`; SHA-256 `2f48b3d9a884de6bf277752614366db4f54b7f7fa3d20b80f0801c980c37104f`. Current module: Go `1.24.0`, `modernc.org/sqlite v1.45.0`; host `go1.26.3 darwin/arm64`.

Method: Mnemon recall preceded inspection. Because this checkout has no graph file, Graphify was run first against `/Users/jay-m4/code/rawclaw/graphify-out/graph.json`: `graphify query "sqlite modernc busy timeout" --budget 4000`, `graphify explain "ConnectRW"`, and `graphify path "ConnectRW" "sqlite3"` (no path). Source was then checked with `rg`, `go doc`, and numbered source. No product/graph/mailbox files were changed. The unrelated pre-existing `FINDINGS.md` was preserved.

## PA-SQLITE-PROGRESS-BUDGET-001 — NARROWED

RawClaw uses the pure-Go modernc driver through `*sql.DB` (`internal/store/store.go:321-350`). The driver's supported `ConnectionHookFn` is only `(ExecQuerierContext, dsn)` (`modernc.org/sqlite/sqlite.go:353-358`), and `go doc modernc.org/sqlite.Driver` exposes no progress-handler registration. `sqlite3_progress_handler` appears only in generated internal/lib code. The driver's `conn.interrupt` is unexported (`modernc.org/sqlite/conn.go:539-551`), so directly installing a C-style VM-step callback would require unsupported internals or a custom fork.

The driver does support context cancellation: `ExecContext`/`QueryContext` (`conn.go:1052-1070`) use internal `interruptOnDone` (`sqlite.go:75-115`). This is a supported bounded-operation seam, but is not a progress callback or step counter. Existing rebuild/consolidation fence paths use `context.Background()` (`internal/index/rebuild.go:83`, `internal/index/consolidated.go:403,557`), so any retrofit must retain rollback/incomplete-publication semantics. Not a duplicate of WAL checkpoint scheduling, FTS5 merge scheduling, or receipt ownership.

Corrected recommendation: “Use a caller-owned context deadline with modernc `ExecContext`/`BeginTx` to bound maintenance statements; treat cancellation as incomplete and publish completion only after commit. Do not claim `sqlite3_progress_handler` support without a supported driver API or explicitly accepted custom fork.”

## PA-SQLITE-BUSY-TIMEOUT-001 — DUPLICATE

This already exists at the connection seam. `ConnectRO` uses `_pragma=busy_timeout(5000)` (`internal/store/store.go:321-328`); `ConnectRW` uses WAL plus `_pragma=busy_timeout(10000)` (`store.go:331-350`). `internal/store/connect_test.go:30-36` asserts the RO timeout, and `rg -n "busy_timeout" . --glob '*.go'` found the same DSN pattern in benchmarks, Goose readers, and tests. modernc documents `_pragma` as supported (`driver.go:45-52`) and applies `busy_timeout` first (`sqlite.go:140-157`). This bounded lock wait is distinct from fence admission/context deadlines and does not replace retries or publication ownership.

Corrected recommendation: “No new recommendation: preserve and test the existing per-connection busy-timeout settings. Keep busy-wait expiry distinct from context/fence deadlines; do not advance watermarks or terminal receipts after `SQLITE_BUSY`.”

## PA-SEMANTIC-BENCH-COUNTER-001 — CONFIRMED

Mutation in disposable `/tmp/benchmetric`:

```go
func BenchmarkReportMetricOnly(b *testing.B) {
    work := 0; b.ResetTimer()
    for b.Loop() { b.ReportMetric(float64(work), "rows/work") }
}
func BenchmarkReportMetricAssert(b *testing.B) {
    work := 0; b.ResetTimer()
    for b.Loop() { if work == 0 { b.Fatal("zero useful work") }; b.ReportMetric(float64(work), "rows/work") }
}
```

Commands/results:

```text
go test -run '^$' -bench 'BenchmarkReportMetricOnly$' -benchtime=1x -count=1
BenchmarkReportMetricOnly-10  1  542.0 ns/op  0 rows/work
PASS
go test -run '^$' -bench 'BenchmarkReportMetricAssert$' -benchtime=1x -count=1
--- FAIL: BenchmarkReportMetricAssert
    metric_test.go:18: zero useful work
FAIL
```

Therefore `B.ReportMetric` is output only; it cannot reject zero work. RawClaw benchmarks use `b.Loop()`/`ReportAllocs` (`internal/store/connect_bench_test.go:133-178`, `internal/index/index_bench_test.go:70-178`) and have no semantic counter today. This is a measurement-contract rule, not a production mechanism, and does not duplicate the other recommendations.

Corrected recommendation: “In each maintenance benchmark, assert the fixture performs non-zero intended work (rows examined/deleted/committed > 0), then report that counter with `B.ReportMetric`; a reported zero is a failure.”

## Verdicts

| ID | Verdict | Boundary |
|---|---|---|
| PA-SQLITE-PROGRESS-BUDGET-001 | NARROWED | Supported context interruption exists; progress-handler API does not. |
| PA-SQLITE-BUSY-TIMEOUT-001 | DUPLICATE | 5 s RO / 10 s RW DSN configuration already shipped and tested. |
| PA-SEMANTIC-BENCH-COUNTER-001 | CONFIRMED | Mutation proves explicit assertion is mandatory; metric alone gives a green zero-work run. |

Validation: `git diff --check` passed. Only this uniquely named report is intended for commit; pre-existing `FINDINGS.md` remains untouched.
