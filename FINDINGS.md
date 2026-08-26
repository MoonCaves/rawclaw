# Benchmark bloat findings

Baseline: `internal/store/connect_bench_test.go` at `8824e256066518a685e685aa70eb2ed59019dfc8`.

## Confirmed clone groups

- `internal/store/connect_bench_test.go:208-227`, `229-248`, `250-269`, `271-290`: four Search cold benchmark bodies differ only by connector function and benchmark name.
- `internal/store/connect_bench_test.go:368-387`, `389-408`, `410-429`, `431-450`: four Browse cold benchmark bodies differ only by connector function and benchmark name.
- The warm Search matrix at lines 148-202 and warm Browse matrix at lines 298-352 repeats the same lifecycle, timer, allocation reporting, error check, and non-empty check for four connector variants each.

## Proposed seam

Use one local table of connector variants (`name`, `connect`) and two small benchmark helpers parameterized by operation and result label. Keep the existing `b.Run` names exactly as `Search/{variant}/{Warm|Cold}` and `Browse/{variant}/{Warm|Cold}`. Warm helpers open once before `ResetTimer` and defer close; cold helpers open and close inside each `b.Loop` iteration, including close-before-fatal on operation or empty-result errors. The operation callbacks preserve the existing `store.SearchHits` and `store.BrowseSessions` calls and checks.

No production seam or interface is justified: this is one benchmark file with four concrete connector functions and two operations. A local benchmark table is the smallest seam that removes duplication without inventing a package abstraction.

## Contract

- Preserve all 16 sub-benchmark names and connector variants.
- Preserve warm versus cold connection lifecycle.
- Preserve `b.ResetTimer`, `b.ReportAllocs`, `b.Loop`, operation-specific error text, and empty-result checks.
- Preserve the existing `connect*RO` functions and DSNs.

## Estimate

- Baseline: 451 lines total.
- Final refactor: 218 lines total, deleting 298 and adding 65 test lines (net `-233`).
- Acceptance floor: exceeded; the final file saves 233 lines and keeps the matrix explicit at the benchmark call sites.

## Rival-history check

`git log --all -- internal/store/connect_bench_test.go` found only the original tuning commits (`f5dacf7`, `c95f9a1`) and the later error-message split (`d31a9e3` / `665ee23`). None table-drove the matrix. The strongest rival was `d31a9e3` because it preserved distinct error and empty-result diagnostics, but it left all 16 bodies duplicated; the weakest was the original `f5dacf7`, which introduced the 395-line matrix without a local seam. This refactor retains both diagnostic contracts while removing the clone groups.
