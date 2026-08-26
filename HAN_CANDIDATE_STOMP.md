# Han candidate-stomp adjudication

## Scope and verdict

Fixed base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
Evidence candidates: `bd8346c5468435ba8636042c4846032e26460dba`,
`37ec96bebb2a8317617544836ef9730149e1f0d4`, and
`61b79574f72d8de1b0b8caa3a6402c3093a6173f`.

**Verdict: reject the candidate set as a clean adoption set.** `bd8346c` is
ancestry-contaminated by `61b7957`; `37ec96b` is from an older lineage. The
path-safe catalog mechanism and benchmark-loop deduplication are technically
distinct changes, but the path candidates are competing implementations of
the same seam and neither gets novelty credit. `37ec96b` is the stronger
path-safe shape because its temporary namespace does not contain the raw
session ID; that conclusion still needs hostile execution on the fixed base.

## Graphify receipts

Graphify orientation used the RawClaw graph at `/Users/jay-m4/code/rawclaw`
because this worktree has no graph JSON. `graphify reflect --if-stale` reported
lessons already up to date; `LESSONS.md` says the earlier tail-parser
duplication claim is stale.

| Query | Outcome |
|---|---|
| `graph_stats` | 3,501 nodes, 10,364 edges, 194 communities; 64% extracted, 36% inferred |
| BFS `candidate stomp ancestry merge benchmark catalog hook path safe` | Found `Candidate 6`, `catalog_hook_test.go`, `BenchmarkConnectionPragmas()`, `BenchmarkUnchangedFastPath()`, `paths.go`, and related hook/path nodes |
| DFS `WriteCatalogEntry ContainedJSONL CatalogDir catalog_hook_test` (`call,field`) | Located catalog nodes; `WriteCatalogEntry -> Remove` was inferred, not treated as proof |
| DFS `BenchmarkConnectionPragmas runConnectionBench seedRealisticBenchStore connect_bench_test` (`call,field`) | Located benchmark helper, realistic seeding, `ConnectRW`, `SearchHits`, and `BrowseSessions`; some edges were inferred |
| shortest path `WriteCatalogEntry` to `TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath` | Three hops through `paths.go` and `ReadCatalogEntry`; seam orientation only |
| shortest path `BenchmarkConnectionPragmas` to `BenchmarkUnchangedFastPath` | Three hops through `seedRealisticBenchStore` and `ConnectRW`; benchmark orientation only |

Graphify could not answer exact diff text or ancestry, so targeted Repomix was
used only for `internal/cli/setup.go`, `internal/cli/cmd_ingest_test.go`, and
`internal/store/connect_bench_test.go` (3 files, 20,336 tokens). It confirmed
the candidate symbols and benchmark loop structure. Git metadata and diffs
below are decisive.

## Ancestry and contamination

| Candidate | Parent | `merge-base` with fixed base | Verdict |
|---|---|---|---|
| `bd8346c` | `61b7957` | `0d1da19` | contaminated: directly built on the benchmark candidate |
| `37ec96b` | `b944d08` | `5b9756b2200ff6bd670f07407407d84d9f42d84b` | stale/older lineage, not a clean child of fixed base |
| `61b7957` | `a317766` | `0d1da19` | independent benchmark candidate |

`git rev-list --ancestry-path 0d1da19..bd8346c` includes `61b7957`,
`a317766`, and `b2ff61c`; the corresponding range for `37ec96b` is empty.
Any credit assigned to `bd8346c` must separate its own patch from the carried
benchmark change.

## Patch identity and line accounting

| SHA | Files / mechanism | Stable patch ID | Net production | Net test | Net docs |
|---|---|---|---:|---:|---:|
| `bd8346c` | `setup.go` catalog-key validation plus hostile catalog test | `d04dfd2a5176fa19377cbad7c786d1ee31433a2c` | +8 (`82/-74`) | +157 | 0 |
| `37ec96b` | flat-key validation, PID-only temp directory, hostile catalog test | `f66a11ef522e6e12ca4f37bfcbb5109344af8a16` | +32 (`60/-28`) | +157 | 0 |
| `61b7957` | benchmark loop refactor in `connect_bench_test.go` | `82e142f3630e29de6ffcf0182f05eba2050357ea` | 0 | -8 | 0 |

The path candidates are not patch-identical: their setup blobs are
`c577bdccfdeea97264caaa98ddd15b16a0de4fad` and
`317fa26bdad032dbdbf879171fca1ff290f02a1b`, with different parent blobs
`7d4e1cca596d4db869441219417bc2f7a2875960` and
`032439cbdee190b3c65a8a2e33ff8f3e3d04b07f`. `bd8346c` does share its stable
patch ID with successors `89d0c1c`, `a7df834`, and `696a95b`; `61b7957` shares
its ID with `7bb4e51`. These are repeated payloads, not novelty.

Accounting is each commit versus its own parent; it is not a safe stacking
claim. No production, test, AGENTS, graph, or rival-worktree files changed.

## Mechanism distinction

Both path candidates change the POSIX hook templates in `setup.go` and add the
same 157-line hostile test in `cmd_ingest_test.go`. Targeted Repomix confirms
flat-key validation, fail-soft ingest for invalid IDs, and temporary regular
file plus `ln` publication. They are one mechanism family, not independent
features.

The implementation distinction is decisive:

- `37ec96b` uses `tmp_dir="$catalog_dir/.tmp.$$"` and places the validated key
  below it. The raw ID does not enter the temporary directory name.
- `bd8346c` uses `tmp_dir="$catalog_dir/.tmp.$session_id.$$"`. Its final key is
  validated, but the raw ID still enters the temporary path; a slash can escape
  the intended temporary namespace.

This is targeted Repomix/diff evidence, not an executed safety result. Hostile
shell tests on the fixed base are **UNCERTAIN / UNRUN**.

`61b7957` is separate and test-only: it deletes the second nested cold/connector
loop in `connect_bench_test.go` and places Search and Browse sub-benchmarks in
one shared matrix. It has no production code or catalog-hook path. Benchmark
execution and statistical comparison are **UNCERTAIN / UNRUN**.

## Rulings

- **Reject `bd8346c` as written:** contaminated ancestry and raw-ID temporary path.
- **Do not adopt `37ec96b` blindly:** reproduce on `0d1da19` with hostile FIFO,
  directory, symlink, traversal, empty, dot-leading, and separator inputs; verify
  no escape and exactly one ingest.
- **Treat `61b7957` independently:** reproduce benchmark names/counts and verify
  Search and Browse retain all warm/cold connector cases.
- **Do not claim novelty:** stable patch identity shows repeated payloads and
  graph/source evidence shows an older mechanism family.
- **Never transplant a candidate range:** cherry-pick only an independently
  reviewed patch after fixed-base reproduction gates pass.

## Falsifiers

1. A fixed-base range-diff or stable comparison proves `37ec96b` is a new
   mechanism, not a competing catalog implementation.
2. Repeated hostile tests show `bd8346c` cannot construct an escaping temporary
   path on supported POSIX shells.
3. A benchmark run proves `61b7957` preserves all eight connector/mode/operation
   cases while changing only loop duplication. Until then behavior is UNCERTAIN.
4. Clean fixed-base ancestry is demonstrated for a candidate; current merge-base
   evidence falsifies that for `37ec96b` and contamination falsifies it for `bd8346c`.

## Strongest challenges

### Furiosa

The initial identity receipt's contradiction was corrected in Furiosa's
`8f58e023554fb7d64b651db7a3f40ac8cea10fb8`: the candidate result blob is
`c577bdcc...`, its parent is `7d4e1cca...`, and the `82/74` setup diff is real.
The strongest remaining challenge is procedural: the original withholding of
credit was based on confusing the audit-document commit's unchanged setup blob
with the candidate's object. Credit must now be judged from the corrected
candidate objects, while the independent contamination and raw-ID safety
objections remain valid.

### Ozzy

Ozzy correctly rejects stale-lineage transplants, but “same mechanism already
adopted” does not establish patch identity or fixed-base safety. The path
candidates have different stable patch IDs and setup blobs. The adjudication
must separate duplicate mechanism family from duplicate payload, then judge
raw-ID safety and ancestry independently; otherwise prior art is being treated
as proof of safety or integration.

## Secondary reports and gates

The supplied Han prior-art, harness-audit, and Ozzy-harvest reports were read
as secondary claims. No Go tests or benchmark were run here. After this report
is created, observed gates are `git diff --check`, commit, push, exact
local/remote SHA, and clean worktree. Unrun experiments remain **UNCERTAIN**.
