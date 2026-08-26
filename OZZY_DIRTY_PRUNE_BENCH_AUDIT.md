# Ozzy dirty prune benchmark audit

## Verdict

**HOLD.** The dirty change is novel, not a duplicate of `61b7957` or the
Lenny `skill-architecture` benchmark work. It is a valid measurement of the
missing-session fast path, but the broad name and lack of a baseline make it
misleading as evidence about tombstone pruning overall. Salvage would require
narrowing the name and claim, or adding a representative live session/subagent
case plus a controlled comparison. Until then, drop the benchmark from the
candidate series.

`net: -29 lines possible.`

## Immutable identity and dirty patch

- Rival worktree: `ozzy/flash-prune-benchmark` at immutable `HEAD`
  `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`.
- Dirty path: `internal/index/consolidated_test.go` only, 29 additions and
  zero deletions; production delta is zero, test delta is `+29`.
- Stable Git patch ID: `7c6141c4932d06a08e20a290a43c86a65dd13eef`.
- Raw dirty-diff SHA-256:
  `c5e4fccfc0ba60e54703f7973678f4d4a630509f86dbdc009f855de5223acebd`.
- `git diff --check` reports a new blank line at EOF in the dirty file.

## Exact change and identity comparison

- New function: `internal/index/consolidated_test.go:2190`
  `BenchmarkPruneTombstonedIDs`.
- Setup creates one consolidated DB, ensures schemas, then constructs 2,000
  IDs named `missing-bench-0` through `missing-bench-1999` at lines 2201-2207.
- The timed body at lines 2210-2214 repeatedly calls
  `pruneTombstonedIDs(con, ids)`. None of those IDs exists, so the timed path
  performs existence checks, skips every ID, and commits; it performs no
  DELETE of a session, message, source, file-index, topic, or verdict row.
- `61b7957` is an unrelated 8-line deletion in
  `internal/store/connect_bench_test.go`, changing the existing
  `BenchmarkConnectionPragmas` matrix. It has no
  `BenchmarkPruneTombstonedIDs` or `pruneTombstonedIDs` identity overlap.
- Lenny `skill-architecture` `HEAD b5f570b` contains only
  `seedRealisticBenchStore`, `runConnectionBench`, and
  `BenchmarkConnectionPragmas` in `internal/store/connect_bench_test.go`.
  No matching prune benchmark exists there. The dirty patch is therefore not
  a transplant or duplicate of that work.
- The immutable `cdc063d` baseline has the existing focused test
  `TestPruneTombstonedIDs_SkipsMissingIDsQuicklyAndPrunesExistingThreads` at
  lines 1960-2035. That test already covers 2,001 mostly-missing IDs and also
  seeds/deletes a real `victim` and `victim/agent-1`; the dirty benchmark drops
  the live-prune assertion and turns the same missing-ID case into a timed
  benchmark without adding a correctness contract.

## Read-safe verification

Commands were run from the rival worktree with a temporary `GOCACHE` and
`GOTMPDIR=/tmp`; no rival files were edited, staged, reset, cleaned, or
formatted.

Focused correctness test:

```text
/usr/bin/time -p env GOCACHE=<mktemp outside worktree> GOTMPDIR=/tmp go test -run '^TestPruneTombstonedIDs_SkipsMissingIDsQuicklyAndPrunesExistingThreads$' -count=1 ./internal/index
```

Result: `ok github.com/MoonCaves/rawclaw/internal/index 1.449s`; shell timing
`real 16.38`, `user 43.72`, `sys 8.37` (first isolated compile/cache fill).

Repeated benchmark:

```text
/usr/bin/time -p env GOCACHE=<same temporary cache> GOTMPDIR=/tmp go test -run '^$' -bench '^BenchmarkPruneTombstonedIDs$' -benchmem -benchtime=200ms -count=5 ./internal/index
```

Result on `darwin/arm64`, Apple M4:

```text
27  8696671 ns/op  1279245 B/op  36038 allocs/op
30  8738319 ns/op  1267917 B/op  36016 allocs/op
30  9373057 ns/op  1264522 B/op  36009 allocs/op
28  9301838 ns/op  1264490 B/op  36009 allocs/op
24 10138665 ns/op  1264506 B/op  36008 allocs/op
```

The five samples span about 16%; there is no control run, no competing
implementation, and no `benchstat` comparison. These numbers support only the
narrow statement “2,000 missing-ID checks cost roughly 9-10 ms on this machine
under this setup.” They cannot support a prune speedup, regression claim, or
end-to-end tombstone-pruning claim.

## Ruling

**HOLD / salvageable, not ACCEPT as-is.** Keep the 29-line patch only if the
owner narrows the benchmark identity and documentation to the missing-ID fast
path, or adds a mixed corpus with at least one existing tombstoned root and
subagent plus a controlled baseline. Otherwise remove it; the existing
focused test already exercises the intended behavior and the cleanest result
is `net: -29 lines possible`.
