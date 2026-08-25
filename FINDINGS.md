# Ponytail Review: internal/semantic/**

Review for over-engineering, unnecessary complexity, dead code, and standard library opportunities across `internal/semantic/**`.

## Findings

internal/semantic/semantic.go:L59-70: delete: `unpackVec` is dead code (unused in production since in-place cosine scoring). Delete helper and update unit test to test `packVec` directly.
internal/semantic/semantic.go:L73-76: shrink: `contentHash` encodes 20-byte SHA-1 to 40 hex chars then slices `[:16]`. `hex.EncodeToString(sum[:8])` produces 16 hex chars directly with fewer allocations.
internal/semantic/semantic.go:L135-139,L501-505: stdlib: `len([]rune(text)) < MinChars` allocates a new rune slice on the heap for every message. Extract `cleanCandidate` helper using `utf8.RuneCountInString` to dedup prose stripping and eliminate heap allocations.
internal/semantic/semantic.go:L230-233: stdlib: manual 4-line min check for `numWorkers`. Built-in `min(8, len(batches))`, 1 line.
internal/semantic/semantic.go:L366-374: stdlib: `sort.Slice` uses reflection and closure capture for 3-field tuple sorting. `slices.SortFunc` with `cmp.Or` and `cmp.Compare` is type-safe, non-allocating, and declarative.
internal/semantic/semantic.go:L392-397: shrink: separate `if err != nil` and `if len(stored) == 0` guards. Combine into single `if err != nil || len(stored) == 0` check; pre-size `out` capacity with `min(k, len(cand))`.
internal/semantic/semantic.go:L461-466: stdlib: `sort.Slice` for RRF tiebreak sorting. `slices.SortFunc` with `cmp.Or(cmp.Compare(score[b], score[a]), cmp.Compare(a, b))`, 2 lines.
internal/semantic/semantic.go:L507-509: shrink: explicit zero-value struct literal `CoverageStats{Candidates: 0, Vectored: 0, Missing: 0}`. `CoverageStats{}`, 1 line.
internal/semantic/topup.go:L26-35: shrink: `topupLockPath` and `topupTokenPath` duplicate directory and clean base path resolution. Extract private `topupPath(dbp, ext)` helper.
internal/semantic/topup.go:L49-51: stdlib: manual two-sided duration bounds check `age > -topupWindow && age < topupWindow`. `now.Sub(st.ModTime()).Abs() < topupWindow`.
internal/semantic/topup.go:L83-100: stdlib: `sync.RWMutex` + `bool` guarding `--no-vector` flag. `atomic.Bool` from `sync/atomic`, lock-free, -13 lines.
internal/semantic/topup.go:L120-137: shrink: sequential nested early-return guards in `MaybeVectorTopup`. Combine into single guard expression.

## Duplication / Clones (Report Only)

- `internal/semantic/semantic_test.go:588-599` and `601-612`: mechanical dupl target in test helper setup. Held test file — report only.

## Cross-Package Opportunities (Report Only)

- `internal/embed/embedtest`: Test embedder doubles (`fakeEmbedder`, `mockEmbedder`, `nullEmbedder`) are duplicated across `internal/adapters`, `internal/agentproto`, and `internal/semantic`. A shared package in `internal/embed/embedtest` (mirroring `storetest`) would centralize test embedders without cross-fence coupling.

## Accepted Deviations

- `internal/semantic/semantic.go:358`: `slices.SortFunc` with `cmp.Compare` for cosine similarity sorting treats NaN as ordered (NaN < non-NaN in `cmp.Compare`), whereas previous `sort.Slice` with `>` treated NaN comparisons as false. Deliberately kept per merge-gate review ruling.

## Scoring

net: -52 lines possible.
