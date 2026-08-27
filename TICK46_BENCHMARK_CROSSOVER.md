# Tick 46 benchmark crossover

## Verdict

There is no universal performance claim, and this reproduction does not establish a
stable sign-change threshold. On the requested 10/100/600 tombstone-ID matrix with
0/1/5/10/25/50% IDs present and 200 unrelated live sessions, 386ec9d was statistically
indistinguishable at most points and modestly faster at several small cases. The
earlier blanket regression claim is not reproduced by this fixture either.

The result is fixture-specific. A dynamic choice between the old per-ID loop and the
new temp-table implementation would be YAGNI on this evidence.

## Exact provenance

- Baseline parent: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`
- Candidate: `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`
- Candidate subject: `perf(index): batch tombstone pruning`
- Candidate whole commit patch-id: `356c1cb3878d142f910494843358b2737554dace`
- Candidate path patch-id (`internal/index/consolidated.go`): `6b42e87e9d75eccc8a5527faa6c001653c15be82`
- Benchmark listing: `go test ./internal/index -list '^BenchmarkTick46PruneCrossover$'` printed `BenchmarkTick46PruneCrossover`.
- Host: `go1.26.3 darwin/arm64`, Apple M4; `goos=darwin`, `goarch=arm64`, `cpu=Apple M4`.

The identical disposable benchmark was run in separate worktrees, alternating
baseline then candidate, with `-benchmem -count=10 -benchtime=1ms`. Setup and
reseeding were outside the timed region. Each operation runs a semantic oracle: all
tombstoned rows must disappear while exactly 200 unrelated sessions remain.

## Observed matrix

`benchstat` sec/op, candidate versus baseline:

| IDs | present 0% | 1% | 5% | 10% | 25% | 50% |
|---:|---:|---:|---:|---:|---:|---:|
| 10 | ~ | ~ | -11.91% (p=.011) | -5.82% (p=.035) | -16.02% (p=.007) | -10.21% (p=.011) |
| 100 | ~ | ~ | ~ | ~ | ~ | ~ |
| 600 | -2.17% (p=.001) | -1.24% (p=.004) | ~ | ~ | ~ | -0.99% (p<.001) |

All comparisons have n=10. Allocation deltas were effectively zero (geomean
`-0.08%` bytes/op, `-0.01%` allocs/op). The run output hashes are:

- baseline: `242db476fa965b64eba81ba4ed994b80005477f9579a19922fa32d5e5146cc3f`
- candidate: `88c79e1009d736487f6a90413860333da9c91b9b608b0445394af65d66e297be`
- benchstat: `f17d7136bcfb9570522d5a031dc895e41c659b3ee7bb5333d0eed385e8c61069`

## Work and query-plan audit

The baseline performs an existence lookup for each ID, then up to six DELETE
statements for each present ID. The candidate inserts IDs into a keyed temporary
table, removes missing exact IDs with one `NOT EXISTS`, then executes six correlated
DELETE statements. `EXPLAIN QUERY PLAN` for the candidate reports a scan of each
target table plus a correlated scalar subquery scanning `t`; the baseline equality
probe uses `sqlite_autoindex_sessions_1`, while the DELETE with `id OR LIKE` reports
a table scan. Thus the candidate changes round trips and statement count, but does
not turn the target-table deletes into indexed joins. This explains why data shape,
cache state, and unrelated rows can move the result without defining a general
threshold.

## Semantic mutation ruling

The benchmark's postcondition is a real work oracle, not `B.ReportMetric`: a complete
no-op leaves the present tombstone rows and fails (`sessions != 200`), while a mutant
that skips `session_verdict` deletion leaves the session row count correct but fails
the existing focused test `TestPruneTombstonedIDs_SkipsMissingIDsQuicklyAndPrunesExistingThreads`.
The fixture seeds messages, session sources, file index, topic segments, and verdicts
with the same schema/bytes on both revisions. No evidence supports a universal speed
claim or a benchmark-selection/DCE artifact.

## Gates and limitations

Focused prune tests passed in both worktrees. The 1 ms benchmark target was chosen to
finish all 36 cells while retaining 10 independent samples; higher benchtime runs
exposed host temp-directory pressure and were not used as the result. No product code
was changed. Disposable Go benchmark files were removed before the report commit.
