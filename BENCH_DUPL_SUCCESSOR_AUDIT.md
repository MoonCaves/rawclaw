# Benchmark Duplicate Successor Audit

Audit branch: `norm/ozzy-spy`.
Target context: current `norm/integration-wave2` tip `a317766e1906e92ff92300c62131c69d366b4939`, where `internal/store/connect_bench_test.go:192-204` and `:205-217` contain structurally duplicated Search and Browse loops.

## Verdict: SAFE TO ADOPT — clean successor `61b79574`

At the initial patrol point, no clean commit was present; the best available shape was an uncommitted staged diff in the integration worktree. That exact diff was subsequently committed as `61b79574f72d8de1b0b8caa3a6402c3093a6173f`, directly on `a317766`. The clean successor deletes the second `cold × connector` loop and places the Browse `b.Run` immediately after Search inside the first loop.

That staged shape is behavior-preserving and net-negative:

- exact delta: `0 additions / 8 deletions`, net `-8` test lines;
- all 16 benchmark names remain: Search and Browse × four connectors × Warm and Cold;
- Search retains `runConnectionBench(..., "SearchHits", ..., search, func(hits) bool { return len(hits) > 0 })`;
- Browse retains `runConnectionBench(..., "BrowseSessions", ..., browse, func(sessions) bool { return len(sessions) > 0 })`;
- `cold`, `connector`, and `mode` are the same loop variables for both sub-benchmarks, so no parameter or closure contract changes.

The integration worktree was already dirty before this audit. I did not edit, stage, reset, commit, or otherwise mutate it.

## Exact identity and prior-art checks

Current target:

```text
a317766e1906e92ff92300c62131c69d366b4939
parent b2ff61c53d1abd67ee87e9acabd47283b76a7a8f
refactor(cli): shrink segment range bounds
```

The target file has the residual loops:

```go
for _, cold := range []bool{false, true} {
    for _, connector := range benchConnectors {
        mode := "Warm"
        if cold { mode = "Cold" }
        b.Run("Search/"+connector.name+"/"+mode, ...)
    }
}
for _, cold := range []bool{false, true} {
    for _, connector := range benchConnectors {
        mode := "Warm"
        if cold { mode = "Cold" }
        b.Run("Browse/"+connector.name+"/"+mode, ...)
    }
}
```

The staged integration diff and committed `61b79574` have stable patch ID `82e142f3630e29de6ffcf0182f05eba2050357ea` and `0/8` numstat. The commit is now present on `norm/integration-wave2` and its remote ref, directly based on `a317766`. The earlier benchmark demolitions are different:

- `e19b80e324fc1b459d2f4d610602e9f58630fc4a` (`e19b80e`) and its transplant `b5f570baeb30522c0e002427ff4ec0177a04b3b7` removed the original 16 hand-written benchmark bodies and introduced `runConnectionBench`; neither removes this residual pair.
- `e19b80e` stable patch ID is distinct from `82e142f...`; it is historical prior art for the larger demolition, not this exact successor.
- The current `a317766` branch is itself a duplicate of the earlier range-shrink patch family, unrelated to the benchmark deletion.

`dupl` was not installed in the audit environment (`zsh: command not found: dupl`), so the duplicate report was independently checked by exact source comparison and the supplied line ranges. Graphify was unavailable for this worktree because `graphify-out/graph.json` is absent; `graphify reflect --if-stale` produced an empty lessons file.

## Contract and compile evidence

The staged shape was inspected at `/Users/jay-m4/code/rawclaw-norm-integration-wave2/internal/store/connect_bench_test.go:192-210`. `gofmt -d` produced no output. The target compiled with the race detector:

```text
env CGO_ENABLED=0 go test -race -run '^$' ./internal/store
ok github.com/MoonCaves/rawclaw/internal/store 1.274s [no tests to run]
```

The benchmark itself ran with one iteration per sub-benchmark:

```text
env CGO_ENABLED=0 go test -race -run '^$' -bench '^BenchmarkConnectionPragmas$' -benchtime=1x ./internal/store
PASS: ok github.com/MoonCaves/rawclaw/internal/store 13.348s
```

The observed run emitted all 16 expected names, including both Search and Browse for Baseline, MmapOnly, MmapQueryOnly, and FullTuned in Warm and Cold modes. Every sub-benchmark retained its non-empty-result assertion; no benchmark result or allocation assertion was removed. This one-iteration run is a structural smoke check, not a performance claim or statistical comparison.

The user-provided current-tip full race result is not independently repeated here. The local compile and one-iteration benchmark run are the gates observed for this report.

## Minimum patch shape

The smallest safe change is exactly the existing staged 8-line deletion:

```diff
     for _, connector := range benchConnectors {
         ... Search ...
-    }
-}
-for _, cold := range []bool{false, true} {
-    for _, connector := range benchConnectors {
-        mode := "Warm"
-        if cold {
-            mode = "Cold"
-        }
         ... Browse ...
     }
 }
```

This keeps the same loop nesting and only co-locates the second operation under the already-existing iteration. No helper, interface, generic type, or new abstraction is needed. Ponytail ruling: **shrink by 8 test lines**; do not transplant the dirty diff without committing it in the integration lane and running the required full race gate there.

## Final ruling

**SAFE TO ADOPT `61b79574`.** It is a clean, directly based, exact 8-line test-only deletion with no Search/Browse contract loss. The owning worker reports `CGO_ENABLED=0 go test -race -count=3 -shuffle=on ./internal/store` PASS in `9.324s` and `golangci-lint` 0 issues. Those gates were not independently rerun in this report worktree; the independently observed compile and one-iteration benchmark smoke gate remain recorded above.

No production code was edited. The only intended worktree change is this report.
