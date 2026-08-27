# Tick 38 semantic benchmark mutation review

## Verdict

The candidate benchmark at `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` is not semantically self-checking as committed. Its timing can be green while pruning does no useful work. A disposable aggregate row-count assertion caught all three tested semantic mutations. The guard is recommended for the benchmark, but was not committed because this review permits only this findings report.

## Scope and selection evidence

- Repository/branch: `worker/furiosa-t38-semantic-bench-20260827`
- Base and candidate: `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`
- Benchmark selector:
  `env -u AGENT_MAILBOX_DIR go test ./internal/index -list '^BenchmarkPruneTombstonedIDs$'`
  returned exactly one benchmark: `BenchmarkPruneTombstonedIDs`.
- Related semantic test selector:
  `go test ./internal/index -list '^Test(Consolidate|Prune|Tombstone)'`
  returned exactly 41 tests.

## Baseline and disposable guard

Baseline command:

```text
env -u AGENT_MAILBOX_DIR CGO_ENABLED=0 go test ./internal/index -run '^$' -bench '^BenchmarkPruneTombstonedIDs$' -benchtime=1x -count=1
```

Observed baseline: `BenchmarkPruneTombstonedIDs` — approximately `15.314 ms/op`, PASS.

The fixture inserts 200 live IDs into each of six target tables, for 1,200 committed rows total. The disposable guard counted rows across `sessions`, `messages`, `session_sources`, `file_index`, `topic_segment`, and `session_verdict` before pruning and required exactly 1,200; after pruning it required exactly zero. This checks committed semantic work directly. `ReportMetric` alone is insufficient because it can report a timing metric even when the operation leaves all rows behind.

The guard itself passed at approximately `16.035 ms/op` in the initial run and `15.170041 ms/op` in an independent worker-checkout run. Stable disposable patch ID: `15386d825a71ef40739e2bf7d8ef95e553ec5564`.

## Mutation results

Each mutation was applied only to disposable copies, with `gofmt -w` before testing, and then reverted.

1. Changed benchmark IDs from `live_*` to `gone_*` so no ID could match. Result: FAIL, `semantic work incomplete: 1200 rows remain`.
2. Changed the production call to `pruneTombstonedIDs(con, nil)`. Result: FAIL, `semantic work incomplete: 1200 rows remain`.
3. Skipped the `session_verdict` delete loop in production. Result: FAIL, `semantic work incomplete: 200 rows remain`.

After restoration, both `internal/index/consolidated.go` and `internal/index/consolidated_prune_bench_test.go` matched HEAD byte-for-byte:

```text
consolidated.go: 132910237f875894dd1ea5888a9af6430e84217de746c9b5f2d39bab9209d21b
consolidated_prune_bench_test.go: 76cbf7a6cc0cb5d52b7f1fc5ace45c394434c2e784221550a246535041db5bfe
```

## Boundary

The existing 41 semantic tests cannot simply be composed into `go test -bench`: Go benchmark execution does not run ordinary `Test*` functions as semantic postconditions for each benchmark iteration. The benchmark therefore needs an assertion over durable state (or an equivalent independent oracle) in its own body. The aggregate count is intentionally narrow and committed-work based; it detects the tested false greens without changing production behavior.
