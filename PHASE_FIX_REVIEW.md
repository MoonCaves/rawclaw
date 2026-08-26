# Immutable phase-fix review: `6e7d29a`

## Verdict

**APPROVE — no production semantic drift found.**

Reviewed the single production diff `fd01a92..6e7d29a494f47648df2b2ffd974c61c2a6cb0525` in `internal/index/consolidated.go`. The helper is a behavior-preserving extraction and remains net-negative: 34 insertions, 42 deletions, net **-8 lines**.

## Contract checks

- `beginConsolidatePhase` captures `started := time.Now()` at `consolidated.go:21`, before the `event=start` record at `:26`; completion uses the same captured start at `:28`. This preserves the old timestamp-before-start-log interval.
- Source-bearing calls pass the same `src` and use `filepath.Base(source)` at `:24` and `:30`. Source-less outer phases still omit the source attribute.
- Phase names are unchanged: `schema-migrate`, `source-migrate`, `attach`, `prepare`, `merge`, `detach`, `tombstone-prune`, `watermark-stamp`, and `connection-close`; fence `acquire`/`release` code is untouched.
- Every explicit early-error completion call in `ConsolidateFrom`, `SyncConsolidatedFrom`, and `consolidateOne` remains present. The attach completion still runs on attach error; the detach completion remains in the post-attach defer.
- The merge defer at `:779-784` receives `done` as an argument. It therefore completes the exact merge phase registered immediately above, then invokes `consolidateAfterMergeHook` in the same order as the parent. The hook remains before the enclosing detach and connection-close defers.
- The connection-close defer still closes `con` before emitting completion (`:472-476`, `:582-586`). Fence release ordering is unchanged.
- The helper owns only local timestamp and argument slices. No shared mutable state or new goroutine path was introduced; race coverage passed.

## Counterexample probes

- Temporarily moved `started := time.Now()` below the start log. The phase test still passed, demonstrating a coverage gap for start-marker timing; the mutation was restored and the tree is clean.
- Temporarily removed the explicit merge-defer argument. The file failed to compile (`not enough arguments in call to func(done func())`), then the exact production form was restored.

## Observed receipts

- `CGO_ENABLED=0 go test -race -count=5 ./internal/index -run 'TestConsolidate_(Phase|RetryAfterAbruptPostMergeExit)|Test.*Phase'` — PASS, 4.976s.
- `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...` — PASS, 197.892s.
- Temporary timestamp mutation: `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_LogsPhaseStartsAndDurations$'` — PASS, 4.193s (expected coverage gap).
- Temporary defer-capture mutation: same package fault test — compile failure (expected invalid mutation).
- `gofmt -w internal/index/consolidated.go`, `gofmt -l internal/`, and `git diff --check` — clean after restoration.
- Graphify orientation was attempted (`graphify reflect --if-stale`, reflection read, and query), but this worktree has no `graphify-out/graph.json`; the CLI reported `graph file not found`. No graph result was used as evidence.

The next worthwhile review target is the phase recorder test: it checks presence of start/duration records but not ordering, exact cardinality, source values, or that duration spans the start event. That is a test-strengthening opportunity, not a defect in `6e7d29a`.
