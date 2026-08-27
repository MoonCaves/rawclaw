# Tick 34 semantic mutation findings

## Verdict

`386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` contains a semantically blind
benchmark. The exact benchmark filter selects one benchmark and reports
`PASS`/exit 0 even when pruning does no work or leaves a target table behind.
The candidate is therefore `HOLD` for benchmark-based performance claims.

No product or test change is proposed in this report. All mutations and the
temporary guard were restored byte-exactly.

## Candidate identity and filters

- Candidate: `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`
- Candidate stable patch ID: `356c1cb3878d142f910494843358b2737554dace`
- Before and after source hashes:
  - `internal/index/consolidated.go`
    `132910237f875894dd1ea5888a9af6430e84217de746c9b5f2d39bab9209d21b`
  - `internal/index/consolidated_prune_bench_test.go`
    `76cbf7a6cc0cb5d52b7f1fc5ace45c394434c2e784221550a246535041db5bfe`
- `CGO_ENABLED=0 go test ./internal/index -list '^BenchmarkPruneTombstonedIDs$'`
  matched exactly 1: `BenchmarkPruneTombstonedIDs`.
- `CGO_ENABLED=0 go test ./internal/index -list '^TestPruneTombstonedIDs_SkipsMissingIDsQuicklyAndPrunesExistingThreads$'`
  matched exactly 1 semantic test.

## Baseline observation

Command:

```sh
CGO_ENABLED=0 go test ./internal/index -run '^$' \
  -bench '^BenchmarkPruneTombstonedIDs$' -benchtime=1x -count=1 -benchmem
```

Observed candidate sample: `1 15594334 ns/op 5613704 B/op 6696 allocs/op`;
exit 0, `PASS`. A later restored run was `1 15394875 ns/op 5613672 B/op`
and `8902 allocs/op`, also exit 0. These are samples only, not a baseline or
performance claim.

## Disposable mutations

Each mutation was applied alone, measured, and restored before the next one.
Patch IDs are stable IDs of the disposable diff.

1. **Zero live IDs** — changed benchmark setup from `i < 200` to `i < 0`.
   Patch ID `8af606ccd82c8a9ea082e88beee51e15f4a92540`; mutated benchmark
   hash `15e17baf5d8c92fa340ca00d618b7cbb63ab8fbbddd4d475d3e22f3ce4d8b23a`.
   Without a guard it still emitted `1 2492917 ns/op 4207984 B/op 6696
   allocs/op`, `PASS`, exit 0. This is a false green.

2. **Return before transaction/delete** — inserted `return nil` after the
   empty-ID check. Patch ID `0fe906145ddb37a824258dc4289ce526d2415af6`;
   mutated source hash `4b2d210a107523d9cd324fff043eeb1c634e38461d22c63eb97d8979ca086347`.
   Without a guard it emitted `250.0 ns/op`, `0 B/op`, `0 allocs/op`,
   `PASS`, exit 0. This is a false green. The existing semantic test failed
   with six target-table row-survival reports and exit 1.

3. **Leave one target table undeleted** — removed the `session_verdict`
   delete statement. Patch ID `7cec39e380413a4c0cda69b9a20c4c57338bb3b0`;
   mutated source hash `0704572c1570ca514cc48cc6eb2baf1486ceddc761a12dde017c8bc9935ab8d1`.
   The benchmark still emitted `1 14323500 ns/op 5608376 B/op 8892
   allocs/op`, `PASS`, exit 0. The semantic test failed specifically with
   two surviving `session_verdict` rows and exit 1.

## Disposable semantic guard

The smallest local guard tested was a post-prune query of `COUNT(*) FROM
sessions`, failing if any rows remained. Guard patch ID
`2d652b45a3b95fee2f57c2bdb30242e0662843a6`; guarded benchmark hash
`aad7548aef33366c54a54d1703213c75c65e59c54306b966e822c9f2c92263f4`.

- Restored candidate plus guard: `PASS`, exit 0.
- Zero-live-ID plus guard: `prune left 200 sessions`, exit 1.
- No-op plus guard: `prune left 200 sessions`, exit 1.

The guard was then removed. Final product/test hashes equal the candidate
hashes above, and `git diff --exit-code` was clean before this report.

## Final interpretation

Timing output alone is not proof that intended pruning occurred. The exact
benchmark has no semantic assertion or completed-work counter: at least two
catastrophic mutations remain valid-looking `PASS` results. The existing
semantic unit test catches the partial-table mutation, but it is not selected
by the benchmark command. A future benchmark revision needs a semantic
remaining-row/deleted-row assertion or explicit completed-work metric before
performance results can be trusted.
