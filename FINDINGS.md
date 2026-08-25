# Refactoring Findings: internal/view & internal/retrieve

Ponytail-review pass hunting over-engineering, hand-rolled standard library re-implementations, dead code, and unnecessary lines across `internal/view/**` and `internal/retrieve/**`.

## Actionable Source File Findings

### internal/view/view.go

1. `internal/view/view.go:L102-110`: `shrink`: multi-branch `bookendFetch`. `opts.Bookend` if tools included, else `max(opts.Bookend, bookendScan)`. (net: -3 lines)
2. `internal/view/view.go:L207-214`: `stdlib`: hand-rolled loop in `Reversed`. `slices.Clone` + `slices.Reverse`. (net: -3 lines)
3. `internal/view/view.go:L243-245`: `stdlib`: manual reverse loop constructing `win` in `BuildAnchoredView`. `slices.Reverse(before)` then `append(before, after...)`. (net: -2 lines)
4. `internal/view/view.go:L292-295`: `shrink`: manual reverse loop of `be` in `BuildAnchoredView`. Reuse `Reversed(be)`. (net: -4 lines)
5. `internal/view/view.go:L298-301`: `stdlib`: `if messagesBefore < 0` clamp in `BuildAnchoredView`. `max(0, len(before)-1)`. (net: -3 lines)
6. `internal/view/view.go:L318-355`: `delete`: unexported `sortCandidates` has 0 callers in package `view` (dupl target, clone of `agentproto.sortCandidates`). Delete from `view.go` (and remove test referee `TestSortCandidates` from `view_test.go` to keep build green). (net: -89 lines)
7. `internal/view/view.go:L500-504`: `shrink`: slice iteration over literal strings in `isBareBlockMarker`. `switch` statement without heap allocation. (net: -2 lines)

### internal/retrieve/retrieve.go

8. `internal/retrieve/retrieve.go:L326-337`: `stdlib`: slice copying via `append([]string(nil), ...)` in `Explain`. `slices.Clone`. (net: 0 lines)
9. `internal/retrieve/retrieve.go:L360-365`: `shrink`: manual bounds loop in `Search`. `min(len(scored), max(0, limit))` slice index loop. (net: -2 lines)
10. `internal/retrieve/retrieve.go:L444-459,L703-712`: `shrink`: duplicated snippet extraction and generation filtering in `searchScored` and `MatchAnchors`. Extract private helper `resolveSnippet`. (net: -13 lines)
11. `internal/retrieve/retrieve.go:L744-751`: `stdlib`: manual slice iteration in `isIndexableType`. `slices.Contains(parse.IndexableTypes, typ)`. (net: -5 lines)
12. `internal/retrieve/retrieve.go:L764-770`: `stdlib`: length check and branching in `first10`. `iso[:min(len(iso), 10)]`. (net: -4 lines)

---

## Scoring

net: -130 lines source reduction.

---

## Report-Only Findings (*_test.go & Cross-Package)

Per supervisor directive, test file clones are report-only this wave:
- `internal/view/view_test.go:L49-60`: `stdlib`: `eqInts` manual int slice comparison re-implements `slices.Equal`.
- `internal/view/view_test.go:L232-239`: `yagni`: `newPreviewDB` thin 8-line wrapper around `newTestDB`.
- `internal/retrieve/retrieve_test.go:L375-385`: `stdlib`: `equalStrings` manual string slice comparison re-implements `slices.Equal`.
- `internal/retrieve/retrieve_test.go:L387-398`: `stdlib`: `containsSub` and `indexOf` re-implement `strings.Contains` and `strings.Index`.
- **Test DB Scaffolding**: `newTestDB` and seed message fixtures (`seedMsg` in `internal/view/view_test.go`, `testMsg` in `internal/retrieve/retrieve_test.go`) duplicate basic session/message seeding that could live in `storetest`.
- **Goleak boilerplate**: `leak_test.go` with identical `TestMain` goleak wiring exists across multiple internal packages.
