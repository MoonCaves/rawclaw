# Tick 30 benchmark replacement: live-ID mutation

Verdict: **UNCERTAIN**.

The 386ec9d benchmark does execute the 200 live-ID deletes in its checked-in
form, but its output contains only elapsed time. A disposable mutation changed
both `i < 200` bounds in `internal/index/consolidated_prune_bench_test.go` to
`i < 0`, removing the live IDs from both the input list and seeded rows. The
mutation still compiled and ran, and reduced the measured time by about 89%.
That timing delta proves the benchmark is sensitive to the live work, but no
benchmark assertion or counter reports that the 200 rows were actually
deleted. Therefore the output does not independently expose semantic loss;
timing alone cannot establish correctness.

## Source and mutation evidence

- Base: `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`
- Original benchmark blob: `d3e72efd4163322294f4d6793bf8d1cfa0798105d` (working-tree hash `d3e72efd4163322294f4d6793bf8d1cfa0798105d` is intentionally recorded below as the exact checked-out value).
- Mutation: two `i < 200` bounds changed to `i < 0`; patch was `+2/-2`, net 0 lines.
- Restored benchmark blob after mutation: `d3e72efd4163322294f4d6793bf8d1cfa0798105d`.

The exact hash verification command was:

```text
git hash-object internal/index/consolidated_prune_bench_test.go
d3e72efd4163322294f4d6793bf8d1cfa0798105d
git show HEAD:internal/index/consolidated_prune_bench_test.go | git hash-object --stdin
d3e72efd4163322294f4d6793bf8d1cfa0798105d
```

## Runs

Command for each arm:

```text
PATH=/Users/jay-m4/go/bin:$PATH CGO_ENABLED=0 go test -run '^$' -bench '^BenchmarkPruneTombstonedIDs$' -benchtime=1s -count=3 ./internal/index
```

This was a short evidence-freeze replacement (3 samples per arm, not the
original six-pair request).

Old parent (`0d1da19`, benchmark overlaid temporarily): `28.717846 ms`,
`28.672994 ms`, `28.839345 ms`; median `28.717846 ms`, p95 (max of 3)
`28.839345 ms`.

Restored 386ec9d: `15.109754 ms`, `15.142249 ms`, `15.141889 ms`; median
`15.141889 ms`, p95 `15.142249 ms`.

No-live-ID mutation: `1.629832 ms`, `1.638063 ms`, `1.618531 ms`; median
`1.629832 ms`, p95 `1.638063 ms`.

`benchstat` old versus restored:

```text
PruneTombstonedIDs-10  28.72m ± ∞ ¹  15.14m ± ∞ ¹  ~ (p=0.100 n=3) ²
¹ need >= 6 samples for confidence interval at level 0.95
² need >= 4 samples to detect a difference at alpha level 0.05
```

`benchstat` restored versus mutation:

```text
PruneTombstonedIDs-10  15.142m ± ∞ ¹  1.630m ± ∞ ¹  ~ (p=0.100 n=3) ²
¹ need >= 6 samples for confidence interval at level 0.95
² need >= 4 samples to detect a difference at alpha level 0.05
```

All runs reported `PASS`; the mutation was restored byte-for-byte before this
report was written. The disposable old-parent worktree and benchmark outputs
were outside this branch.
