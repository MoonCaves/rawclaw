# Code mechanisms C: logging, benchmarks, and review identity

Scope: report-only extraction at RawClaw `5b9756b2200ff6bd670f07407407d84d9f42d84b`. Product code was not changed.

## 1. Scoped structured phase logging

**Upstream:** Go `log/slog`, commit [`7f36edc26d4e3becb6d9c9008ff00f260bb19055`](https://github.com/golang/go/tree/7f36edc26d4e3becb6d9c9008ff00f260bb19055).

- **File/functions:** [`src/log/slog/logger.go`, `Logger`, `New`, `With`, `SetDefault`](https://github.com/golang/go/blob/7f36edc26d4e3becb6d9c9008ff00f260bb19055/src/log/slog/logger.go#L83-L151).
- **Mechanism:** `New(handler)` constructs a concrete logger; `With` clones the logger and derives a handler with attributes. The package documents `Logger` as concurrency-safe. `SetDefault` is explicitly the process-global top-level-function path, not the operation-scoped path.
- **Minimal shape:**

  ```go
  func fold(ctx context.Context, log *slog.Logger, src string) error {
      phaseLog := log.With("source", filepath.Base(src))
      started := time.Now()
      phaseLog.InfoContext(ctx, "consolidate fold phase", "phase", name, "event", "start")
      defer func() {
          phaseLog.InfoContext(ctx, "consolidate fold phase", "phase", name,
              "duration", time.Since(started))
      }()
      return work(ctx)
  }
  ```

- **RawClaw mapping:** `internal/index/consolidated.go:637-647` (`beginConsolidatePhase`) and `internal/index/consolidated.go:667-685` (`consolidateOne`) already have the right one-start/one-deferred-duration shape and capture `started` before the start record. The unsafe test seam is `internal/index/consolidated_test.go:19-28`, where `TestConsolidate_LogsPhaseStartsAndDurations` mutates `slog.SetDefault`; sibling tests do the same at `internal/index/consolidated_fence_test.go:93-96` and `internal/index/index_test.go:1415-1418`.
- **Verdict:** **ADAPT.** Pass or bind one operation logger at the consolidation seam and derive source attributes with `With`; use a handler recorder owned by each test. Do not add a global “current phase” variable. Do not use `slog.SetDefault` in tests that can run in parallel.
- **Semantic trap:** a start record emitted after `started := time.Now()` is part of the duration contract. A test that only checks record presence will miss timestamp/order drift. Logging duration also says nothing about freshness or success; preserve the existing stale/error status separately.
- **Exact gate:** run two recorder-backed phase tests in parallel, then `CGO_ENABLED=0 go test -race -count=10 -run 'TestConsolidate_LogsPhaseStartsAndDurations|TestConsolidatedFence_.*Logs' ./internal/index/...`. The test must assert every expected phase has both `event=start` and a duration-typed completion, and must contain no `slog.SetDefault` call. Run `rg -n 'slog\.SetDefault' internal/index` as a structural check; any remaining call needs an explicit non-parallel justification.

## 2. Stable benchmark names, setup boundaries, metrics, and concurrency

**Upstream A:** Go testing benchmark implementation, same Go 1.24.6 source commit [`7f36edc26d4e3becb6d9c9008ff00f260bb19055`](https://github.com/golang/go/tree/7f36edc26d4e3becb6d9c9008ff00f260bb19055).

- **File/functions:** [`src/testing/benchmark.go`, `B.ReportMetric`, `B.Loop`, `B.Run`, `B.RunParallel`](https://github.com/golang/go/blob/7f36edc26d4e3becb6d9c9008ff00f260bb19055/src/testing/benchmark.go#L372-L389), [`B.Loop`](https://github.com/golang/go/blob/7f36edc26d4e3becb6d9c9008ff00f260bb19055/src/testing/benchmark.go#L476-L515), [`B.Run`](https://github.com/golang/go/blob/7f36edc26d4e3becb6d9c9008ff00f260bb19055/src/testing/benchmark.go#L784-L850), and [`B.RunParallel`](https://github.com/golang/go/blob/7f36edc26d4e3becb6d9c9008ff00f260bb19055/src/testing/benchmark.go#L922-L948).
- **Mechanism:** `B.Run(name, fn)` makes a selectable stable sub-benchmark; `B.Loop()` keeps setup outside the measured body and keeps results alive; `ReportMetric(value, unit)` adds or overrides a named metric; `RunParallel` reports wall-time `ns/op` for the benchmark as a whole and forbids timer operations with global effect inside workers.
- **RawClaw mapping:** `internal/index/index_bench_test.go:54-109` (`BenchmarkFTS5Search`) correctly seeds/indexes before `Warm` and `Cold`, uses `b.Run("Warm")`/`b.Run("Cold")`, calls `ResetTimer`, `ReportAllocs`, and checks both errors and non-empty hits. `scripts/bench-concurrency.sh:168-229` is the external contention harness: it records each successful search, records writer/search errors, waits for every child, and fails the run if any worker fails.
- **Verdict:** **COPY/ADAPT.** Keep `BenchmarkFTS5Search/Warm` and `/Cold` names stable; add only explicit dimensions through `b.Run(fmt.Sprintf("corpus=%d/query=%s", ...))` if they become necessary. Add custom metrics only for defined quantities (for example `hits/op` or `bytes/s`), never a second benchmark function for every dimension. Keep setup, database seeding, connection opening, and fixture generation before the timer.
- **Semantic trap:** changing case, spelling, or dimension order silently breaks `-bench` selectors and historical comparisons. `ReportMetric` replaces a metric with the same unit, so duplicate units must not be emitted under competing meanings. A parallel benchmark’s `ns/op` is wall time for the whole benchmark, not the sum of worker CPU time.

**Upstream B:** etcd, commit [`03e41f2858df8a3141131b2c283926feda72aa0e`](https://github.com/etcd-io/etcd/tree/03e41f2858df8a3141131b2c283926feda72aa0e).

- **File/function:** [`server/etcdserver/txn/range_bench_test.go`, `BenchmarkRange`](https://github.com/etcd-io/etcd/blob/03e41f2858df8a3141131b2c283926feda72aa0e/server/etcdserver/txn/range_bench_test.go#L23-L111).
- **Mechanism:** a table of named cases (`no_sort_no_limit`, `key_ascend_limit_10`, etc.) is crossed with sorted corpus-size dimensions and emitted through deterministic `b.Run("keys_%d/%s", ...)`; each case builds its fixture before `b.Loop`, calls `ReportAllocs`, and fails on operation errors. This is the direct pattern for preserving benchmark names while adding dimensions.
- **Verdict:** **COPY.** Use the table/name pattern if RawClaw adds query or corpus dimensions. Do not copy etcd’s database-specific fixture; retain RawClaw’s Claude-shaped corpus.
- **Exact gate:** `go test -run '^$' -bench '^BenchmarkFTS5Search/(Warm|Cold)$' -benchmem -count=10 ./internal/index/... | tee bench.txt`, then compare repeated old/new files with `benchstat`. For the shell harness, run at least five isolated repetitions with `BENCH_SEARCHERS=20 BENCH_WRITERS=3 BENCH_DURATION=60`; report sample count, errors, corpus, machine, and nearest-rank p50/p95. p50/p95 are latency quantiles over successful samples only; any worker error invalidates that run rather than disappearing from the denominator.

## 3. Duplicate patches, reordered series, and competing-branch review

**Upstream:** Git, commit [`f78ce2f7b6df702f93d40b85d6bda92a3f65da79`](https://github.com/git/git/tree/f78ce2f7b6df702f93d40b85d6bda92a3f65da79).

- **Files/functions:** [`Documentation/git-patch-id.adoc`](https://github.com/git/git/blob/f78ce2f7b6df702f93d40b85d6bda92a3f65da79/Documentation/git-patch-id.adoc#L14-L35), [`builtin/patch-id.c:get_one_patchid`](https://github.com/git/git/blob/f78ce2f7b6df702f93d40b85d6bda92a3f65da79/builtin/patch-id.c#L47-L177), and [`Documentation/git-range-diff.adoc`](https://github.com/git/git/blob/f78ce2f7b6df702f93d40b85d6bda92a3f65da79/Documentation/git-range-diff.adoc#L14-L42).
- **Mechanism:** `git patch-id --stable` hashes file diffs while ignoring line numbers, whitespace, and file-diff order. Git documents its primary use as finding likely duplicate commits. `git range-diff <base> <rev1> <rev2>` pairs two patch series by patch similarity, then shows added, removed, and modified patches; its output is explicitly human-readable and not stable machine-readable output.
- **RawClaw mapping:** the supervisor’s review workflow compares worker branches against active base `5b9756b`; duplicate product changes must be identified before merging rival logging/benchmark/test branches. A test-only or reordered/squashed commit can have the same patch identity or be paired by range-diff without being a novel product mechanism. Preserve the commit SHA and file fence in the review receipt.
- **Verdict:** **COPY/ADAPT.** Use patch-id as a first-pass equivalence key, then range-diff for competing series and changed implementations. Never treat “different commit SHA” as novel code. Never use patch-id alone as semantic approval: it ignores line numbers and whitespace, and a test-only patch is still a different patch if its diff differs. Review the actual changed paths after matching.
- **Semantic traps:** patch-id is valid for equivalent diff content, not for proving behavioral equivalence; squashing can combine independent product and test changes; a reordered series can match while changing dependency/order semantics; range-diff’s display must not become a machine contract.
- **Exact experiment/gate:**

  ```sh
  base=5b9756b2200ff6bd670f07407407d84d9f42d84b
  git rev-list --no-merges "$base..worker-a" | git diff-tree --patch --stdin |
      git patch-id --stable | sort > worker-a.patchids
  git rev-list --no-merges "$base..worker-b" | git diff-tree --patch --stdin |
      git patch-id --stable | sort > worker-b.patchids
  join -j 1 worker-a.patchids worker-b.patchids
  git range-diff "$base..worker-a" "$base..worker-b" --no-color
  git diff --name-status "$base...worker-a" -- internal/index scripts
  ```

  A matching patch-id is “duplicate candidate”; a range-diff `=` pairing is “same series patch”; only the final path/content review decides whether to keep, drop, or merge. Attach fresh gate commands (`CGO_ENABLED=0 go test -race -count=1 ./...`, focused benchmark command, and harness receipt) and distinguish fresh output from cached output.

## Directives

1. Inject a per-operation `*slog.Logger` and capture handlers per test; remove global-default mutation from parallel phase-log tests.
2. Preserve `BenchmarkFTS5Search/Warm` and `/Cold`, keep setup outside timers, and add named metrics/errors only with repeated `benchstat` and honest nearest-rank p50/p95.
3. Run stable patch-id before accepting competing worker branches, then range-diff and path-review; commit identity is not product novelty.
