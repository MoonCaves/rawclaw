# Performance branch audit — 2026-11-03

Source of this report: exact Git objects inspected for issue #102; no production changes or performance measurements.

## Scope and ruling

- Main snapshot: `0afecdfab2321abb49733746f7250e6572d5ec5e`.
- `olumbra/pure-go-perf-hardening` snapshot: `134fdc6d4081c39b89af819b4272e7dc2756f43f`.
- Merge base: `a61e247a86ff93514ddf7955d901a59fa3590cb5`.

**VERIFIED:** the issue's named `internal/agentproto/outline_bench_test.go` exists in neither inspected snapshot. The branch has one 2,737-line production `agentproto.go`, rather than three production files totaling 2,738 lines. Main has the focused modules. These are snapshot differences, not proof that the performance commit collapsed them.[3][5]

**VERIFIED:** Go 1.24.0 `B.Loop` excludes setup before its first invocation and cleanup after it returns false.[1]
Missing manual `ResetTimer` alone would not establish timer inversion. The inspected index and connection loops have a preceding manual `ResetTimer` as well.[6][8]
The adapter and KNN loops also explicitly reset timing.[9][10]

**OPEN / HOLD for performance claims or a branch transplant:** this audit establishes no measured speedup, slowdown, or user-facing regression. Do not apply the whole historical branch to main as a benchmark repair. No production fix is justified by the two allegations as stated.

## Break attempts and exact-object receipts

The attempted counterexample was the issue's named benchmark file. Both direct lookups failed:

```text
$ git show 134fdc6d4081c39b89af819b4272e7dc2756f43f:internal/agentproto/outline_bench_test.go
fatal: path 'internal/agentproto/outline_bench_test.go' does not exist in '134fdc6d4081c39b89af819b4272e7dc2756f43f'
$ git show 0afecdfab2321abb49733746f7250e6572d5ec5e:internal/agentproto/outline_bench_test.go
fatal: path 'internal/agentproto/outline_bench_test.go' does not exist in '0afecdfab2321abb49733746f7250e6572d5ec5e'
$ git log --all --oneline -- internal/agentproto/outline_bench_test.go
```

The last command emitted no output. This bounds the negative finding to locally available history; it cannot rule out an unavailable or deleted external branch.

```text
$ git show 134fdc6d4081c39b89af819b4272e7dc2756f43f:internal/agentproto/agentproto.go | wc -l
    2737
$ git show 134fdc6d4081c39b89af819b4272e7dc2756f43f^:internal/agentproto/agentproto.go | wc -l
    2770
$ git diff 134fdc6d4081c39b89af819b4272e7dc2756f43f^ 134fdc6d4081c39b89af819b4272e7dc2756f43f -- '*bench*'
$ git diff --numstat 0afecdfab2321abb49733746f7250e6572d5ec5e 134fdc6d4081c39b89af819b4272e7dc2756f43f -- '*bench*'
0	29	internal/index/index_bench_test.go
```

The tip-to-parent benchmark diff emitted no output. The only snapshot difference among matching benchmark paths is main's additional `BenchmarkIncrementalAppend`, not rewritten outline benchmark loops.[7][11]

## Architecture: branch history versus main impact

**VERIFIED:** the merge-base tree already has `internal/agentproto/agentproto.go`. The performance tip edits that existing file, reducing it by 33 lines; it also edits `internal/index/consolidated.go` and `internal/source/opencode/opencode.go`. It does not perform a modular-file collapse.[11]

**VERIFIED:** main's split was commit `94e066b3843a7a08265da5739df11d46d702ed6f`, which deleted the monolith and introduced `locate.go`, `outline.go`, `read.go`, `render.go`, `search.go`, `topics.go`, and `types.go`. The inspected branch preserves older organization; main already contains the split.[4][5]

The branch does retain public functions and package-level tests in the same `agentproto` package. File length alone does not demonstrate destroyed adapter boundaries or inability to test independently: that stronger claim remains **OPEN**. Branch `agentproto.go:483`, `:1501`, `:2010`, and `:2458` still define Search, Read, Outline, and Topics respectively.[3]

**VERIFIED:** main already calls `enrichSearchResults` at `internal/agentproto/search.go:131`; its implementation at `:559-600` attaches topics and activity in one pass. The branch's enrichment idea is therefore not evidence of a remaining missing main capability.[13]

The older branch implementation at `agentproto.go:1364-1415` combines topic/activity enrichment and leaves both legacy helpers calling the combined helper. No transplant is recommended by this report.[3][11]

## Actual Go measurement semantics

The branch declares Go 1.24.0. The official `go1.24.0` tag resolves to `3901409b5d0fb7c85a3e6730a59943cc93b2835c`; its fetched `src/testing/benchmark.go:385-425` supplies both implementation and API documentation.[1]

> Loop resets the benchmark timer the first time it is called in a benchmark,
> so any setup performed prior to starting the benchmark loop does not count
> toward the benchmark measurement. Likewise, when it returns false, it stops
> the timer so cleanup code is not measured.[1]

`loopSlowPath` invokes `ResetTimer` on first entry and `StopTimer` at fixed-iteration completion; `stopOrScaleBLoop` stops timing at duration completion. `ResetTimer` resets elapsed time and allocation counters (`:148-168`). It does not exclude later operations merely because a developer calls them setup.[1]

A successful loop iteration therefore includes executed checks, predicates, formatting, allocations, and I/O inside its body unless the timer is explicitly paused. Failure-reporting bodies such as `b.Fatalf` are conditional: their formatting is not executed on the successful path. Moving a `ResetTimer` before the loop cannot remove successful-path assertions or fixture writes within each iteration.[6][8][9]

For a conventional `b.N` loop, `runN` starts timing before calling the benchmark function (`:181-205`), so setup exclusion requires explicit timer management. Main's `BenchmarkIncrementalAppend` already calls `ResetTimer` before its `b.N` loop; append formatting, file append, indexing, and the success check remain inside the measurement (`internal/index/index_bench_test.go:205-210`).[1][7]

## Measured-work inventory (source inspection, no timings)

| Location at branch SHA | Timed successful-path work | Interpretation and main impact |
| --- | --- | --- |
| `internal/index/index_bench_test.go:85-90` | Warm search plus error/nonempty condition. | Fixture and connection setup precede `Loop`; not a pure unchecked query measurement.[6] |
| Same file `:96-108` | Open connection, search, success checks, close. | Named Cold case explicitly measures a new connection per query; lifecycle work is consistent with the comment at `:52-53`.[6] |
| Same file `:136-140` | Unchanged-directory indexing plus error condition. | Initial corpus/index setup precedes `Loop`.[6] |
| Same file `:171-181` | Increment/format replacement records, helper-driven file rewrite, reindex, success check. | Includes fixture production and filesystem work; cannot be described as isolated SQLite replacement latency. Main contains the same body at `:173-184`.[6][7] |
| `internal/store/connect_bench_test.go:146-180` | Query, error/nonempty predicates; cold mode also connects/closes. | Warm connection setup excluded; named cold lifecycle included. This file has no snapshot diff.[8] |
| `internal/source/antigravity/antigravity_bench_test.go:44-105` | Discover and count check; normalize and string-content check; incremental indexing and status check. | `strings.Contains` in normalization is measured validation overhead; record/corpus setup precedes loops. This file has no snapshot diff.[9] |
| `internal/semantic/semantic_bench_test.go:59-64` | KNN call and result-length condition. | Vector corpus generation precedes loop. This file has no snapshot diff.[10] |

**OPEN:** whether validation or fixture-generation overhead materially affects any comparison. Inclusion establishes measurement scope, not a quantified distortion or invalid speedup. The branch tip's “zero-alloc string building” commit subject supplies no benchmark output; this audit does not validate that performance description.[11]

The branch also carries historical `docs/design/bench-concurrency.md` results, but that document explicitly describes uncontrolled machine load and says its smoke run is not a speedup claim. Those historical observations do not establish a before/after performance effect for the audited tip.[14]

## Follow-up fences and limitations

- **HOLD, measurement interpretation:** `internal/index/index_bench_test.go:171-181` on the branch (`:173-184` on main) includes replacement-fixture generation. If isolated indexing latency is the intended metric, first agree on that workload and a separate benchmark-file-only fence; changing the timer now would change what is measured. No test edit is authorized here.
- **HOLD, missing evidence:** an outline benchmark allegation needs an actual repository/ref/SHA and file before it can support remediation. No substitute benchmark was invented.
- **HOLD, performance claim:** any numerical speedup requires agreed before/after revisions, the same fixture and toolchain, repeated samples, and recorded comparison output. No benchmark command or fixture was approved or run in this report-only audit.
- This is not a correctness or provenance review of every production difference between the diverged snapshots. It reports no production defect requiring a patch and no full-suite test result.

The existing root `AGENTS.md` Index already points at `docs/design/`; this report changes no structure or contract, so owning documentation was checked and left unchanged. RawClaw sources are MIT licensed. Go's cited source is under its BSD-style three-clause license; no implementation is copied into RawClaw by this report.[12][2]

## Sources

[1] https://github.com/golang/go/blob/3901409b5d0fb7c85a3e6730a59943cc93b2835c/src/testing/benchmark.go — Go 1.24.0 benchmark timing
[2] https://github.com/golang/go/blob/3901409b5d0fb7c85a3e6730a59943cc93b2835c/LICENSE — Go license
[3] https://github.com/MoonCaves/rawclaw/blob/134fdc6d4081c39b89af819b4272e7dc2756f43f/internal/agentproto/agentproto.go — Branch agentproto
[4] https://github.com/MoonCaves/rawclaw/commit/94e066b3843a7a08265da5739df11d46d702ed6f — Main modular split
[5] https://github.com/MoonCaves/rawclaw/tree/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto — Main package tree
[6] https://github.com/MoonCaves/rawclaw/blob/134fdc6d4081c39b89af819b4272e7dc2756f43f/internal/index/index_bench_test.go — Branch index benchmarks
[7] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/index/index_bench_test.go — Main index benchmarks
[8] https://github.com/MoonCaves/rawclaw/blob/134fdc6d4081c39b89af819b4272e7dc2756f43f/internal/store/connect_bench_test.go — Connection benchmarks
[9] https://github.com/MoonCaves/rawclaw/blob/134fdc6d4081c39b89af819b4272e7dc2756f43f/internal/source/antigravity/antigravity_bench_test.go — Adapter benchmarks
[10] https://github.com/MoonCaves/rawclaw/blob/134fdc6d4081c39b89af819b4272e7dc2756f43f/internal/semantic/semantic_bench_test.go — KNN benchmarks
[11] https://github.com/MoonCaves/rawclaw/commit/134fdc6d4081c39b89af819b4272e7dc2756f43f — Branch tip change
[12] https://github.com/MoonCaves/rawclaw/blob/134fdc6d4081c39b89af819b4272e7dc2756f43f/LICENSE — RawClaw license
[13] https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/agentproto/search.go
[14] https://github.com/MoonCaves/rawclaw/blob/134fdc6d4081c39b89af819b4272e7dc2756f43f/docs/design/bench-concurrency.md
