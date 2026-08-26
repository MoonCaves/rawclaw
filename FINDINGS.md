# Furiosa Tick 26 speed-claim audit: Ozzy `386ec9d`

## Verdict: UNCERTAIN / HOLD

`386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` is the exact ref target
`ozzy/speed-prune-20260827` (`perf(index): batch tombstone pruning`). Its parent is
`0d1da19c4c21961b86cb3ca84ed047d941c83ed3`. The requested base
`878f631b74e68aa76302f382e28096dc3d60b545` is not an ancestor of the claim: the
merge-base is `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; base is nine commits
ahead of that parent. The claim is therefore audited parent-to-commit, not by
silently treating the requested base as its parent.

The commit changes only `internal/index/consolidated.go` and adds
`internal/index/consolidated_prune_bench_test.go`: +112/-36 (net +76). Whole
patch-id is `356c1cb3878d142f910494843358b2737554dace`; path patch-ids are
`internal/index/consolidated.go` = `6b42e87e9d75eccc8a5527faa6c001653c15be82`
and the benchmark = `ca293434ea17286206af6c8a8c97b00e393f1c4c`.
`git range-diff 878f631...386ec9d 878f631...ozzy/speed-prune-20260827`
matches `386ec9d` exactly and adds only the follow-up report `73171fd`.

## What is actually evidenced

- The benchmark exists and measures the new implementation only. It uses
  `ConnectRW`, `EnsureSchema`, and `EnsureTopicSchema`, seeds 600 missing IDs
  plus 200 live IDs, puts one row for each live ID in `sessions`, `messages`,
  `session_sources`, `file_index`, `topic_segment`, and `session_verdict`, and
  calls real DELETE statements during the out-of-timer reset. The timed call
  includes `Begin`, temp-table creation/inserts, missing-ID filtering, all six
  DELETE statements, temp-table drop, and `Commit` (`b.StartTimer` precedes
  `pruneTombstonedIDs`).
- The only claim receipt found is the later `73171fd` report at
  `ozzy/speed-prune-20260827:FINDINGS_PRUNE.md`. It records a bounded run as
  `15.90–22.70 ms/op`, but gives no raw output lines, no before/after baseline,
  and no benchstat comparison. The branch tip adds no benchmark evidence.
- The report's “six repetitions” wording does not satisfy the requested
  evidence contract: exact raw samples are absent, and there is no median/p95,
  allocation/DB-work report, or serialized/concurrent lock packet.

## Why it does not prove the locked correctness shape

The locked `c38f79a` and `0cd00e4` correctness changes exercise end-to-end
`ConsolidateFrom` source-removal/affected-session reconciliation, including
sidecars when source tables are absent and co-contributor preservation. They are
not ancestors of `386ec9d` (both have the same merge-base `0d1da19c`), and the
benchmark directly calls `pruneTombstonedIDs` on one synthetic database. It does
not run the c38/0cd source-removal shape, measure existing-vs-missing counts
from a real consolidation pass, or exercise serialized/concurrent callers.
Thus it is a different workload even though it includes the six sidecar tables.

## Missing evidence and required action

Keep HOLD/UNCERTAIN. To upgrade the speed claim, publish the exact command,
schema/index inventory and PRAGMAs, existing/missing tombstone counts, warmup
policy, at least six raw samples for both old parent and new commit, median/p95,
`-benchmem` allocations plus DB work, proof transaction commit is timed, a real
DELETE workload, and serialized plus concurrent lock-contention results on the
locked c38/0cd correctness fixture. Include a benchstat comparison and raw
output. Do not award a speed point from the current single-implementation
benchmark or the report's range alone.

No Go files were edited in this audit.
