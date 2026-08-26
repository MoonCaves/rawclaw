# Ozzy Claim Spy — Tick 28

Audit window: after Furiosa completion `20260826T231415Z`, frozen at
`20260826T235330Z`. The Ozzy mailbox contained exactly two post-cutoff files:
`20260826T232243Z-57796a92...` and `20260826T233141Z-066a0cb2...`. No mailbox
cursor was read or changed.

## Verdicts

### `bc8af914d7d5736a8155929e0d81c998a4be5efc` — NO SCORE CLAIM

The challenged latest tip is exactly a docs-only commit. Evidence:

```text
git rev-list --parents -n1 bc8af914...
bc8af914... b1015e7f...
git diff --stat bc8af914^ bc8af914
 docs/design/tombstone-consolidation-contract.md | 22 ++++++++++++----------
 1 file changed, 12 insertions(+), 10 deletions(-)
git diff --name-status bc8af914^ bc8af914
M docs/design/tombstone-consolidation-contract.md
git diff bc8af914^ bc8af914 | git patch-id --stable
1b46d699f573efb107f8825f983771b4c9161d61 0000000000000000000000000000000000000000
```

The whole-commit and docs-path stable patch IDs are both
`1b46d699f573efb107f8825f983771b4c9161d61`. Its ancestry includes the prior
implementation history; that does not make this tip new code. Therefore it
earns no implementation or adoption score.

### `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` — UNCERTAIN

The withdrawn “no deletes” objection is rebutted by the source. The benchmark
seeds 200 live IDs into six tables and, inside the stopped-timer reset, executes
real `DELETE FROM ...` statements for all six tables (`internal/index/consolidated_prune_bench_test.go:34-42,66-73`). The timed call then invokes
`pruneTombstonedIDs`.

Exact identity and ancestry:

```text
git rev-list --parents -n1 386ec9d
386ec9d... 0d1da19c4c21961b86cb3ca84ed047d941c83ed3
git diff --stat 386ec9d^ 386ec9d
 .../consolidated.go                  | 71 +++++++++++------------
 .../consolidated_prune_bench_test.go | 77 +++++++++++++++++++++++++
 2 files changed, 112 insertions(+), 36 deletions(-)
git diff 386ec9d^ 386ec9d | git patch-id --stable
356c1cb3878d142f910494843358b2737554dace 0000000000000000000000000000000000000000
git diff 386ec9d^ 386ec9d -- internal/index/consolidated.go | git patch-id --stable
6b42e87e9d75eccc8a5527faa6c001653c15be82 0000000000000000000000000000000000000000
```

The exact whole patch ID is `356c1cb3878d142f910494843358b2737554dace`
and consolidated path ID is `6b42e87e9d75eccc8a5527faa6c001653c15be82`.
`git range-diff 0d1da19..386ec9d 0d1da19..386ec9d` maps the commit to itself;
there is no rival old/new benchmark series to compare.

The base has no `consolidated_prune_bench_test.go`; only the new benchmark
exists. Its construction is one temporary SQLite DB, `EnsureSchema` plus
`EnsureTopicSchema`, 600 missing IDs and 200 live IDs, with reset and reseed
outside the timed region. It does not provide a byte-equivalent old/new pair,
old implementation command, six raw samples for each side, benchstat,
median/p95, allocation counts, or serialized-versus-concurrent lock/busy
results. The benchmark source has no explicit PRAGMA setup beyond the store
helpers.

Independent gates/evidence:

```text
CGO_ENABLED=0 go test -race ./internal/index -run 'Test(Prune|Consolidate|Tombstone)' -count=1
ok github.com/MoonCaves/rawclaw/internal/index 19.153s

go test ./internal/index -run '^$' -bench BenchmarkPruneTombstonedIDs -benchtime=100ms -count=6
7 15314023 ns/op
7 15317452 ns/op
7 15158947 ns/op
7 15201917 ns/op
7 15193762 ns/op
7 15231792 ns/op
PASS (darwin arm64, Apple M4)
```

This proves six new-only runs, not a speedup. No current-base readiness or
performance credit follows from them. Ozzy's own `FINDINGS_PRUNE.md` reaches
the same HOLD conclusion (no before/after baseline).

## State and report hashes

Relevant pushed worktrees were clean and at upstream parity:

```text
ozzy/speed-prune-20260827 73171fd  dirty=0  0/0
ozzy/composite-instant-tagwrite-20260827 bc8af91 dirty=0 0/0
```

The abandoned `ozzy/flash-prune-benchmark` worktree was dirty with one
uncommitted file: `internal/index/consolidated_test.go` (+29 lines). It is not
part of `73171fd` or any pushed tip.

The inspected Ozzy speed report SHA-256 is:

```text
91e034969973666076e447af1e8675c103cad94feb5aae4e3be07e7ca34bff4e  FINDINGS_PRUNE.md
```
