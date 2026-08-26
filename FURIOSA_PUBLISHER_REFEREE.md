# Furiosa publisher referee

Date: 2026-08-27 (WITA)

## Verdict

**Winner: Furiosa `8c8216e25e22496b2e3e919fce836be49d692e25`, as-is.** It is the smallest candidate that passes the required detached-publication, invalidation, cancellation, authority, deletion, co-contributor, and row-ID checks. Do not adopt Han `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`: its `len(srcPaths) == 1` pruning gate leaves stale sole-source topics whenever an otherwise unrelated second source is folded. Ozzy `537641b` is smaller, but lacks context cancellation and fails the required overlay deletion red.

No production files were changed by this referee. The report branch is based on `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.

## Candidate identity

All three candidates are descendants of the exact base `0d1da19` and their worktrees were clean and tracking their origin branches.

| candidate | final commit patch-id | base-range patch-id | base-range lines | final lines |
|---|---|---|---:|---:|
| Furiosa `8c8216e` | `3a409032463981bbdcf625eeeac1ff9424973a14` | `8df42cb9ffc51473888b85bc823d23a15b713e7c` | +641/-16 (net +625) | +15/-12 (net +3) |
| Ozzy `537641b` | `b7aaaee70fe88073287bb0fecc0c9b81beb80368` | `44e02ee11a6d405507f4a71811abc9b0582d4203` | +483/-12 (net +471) | +59/-7 (net +52) |
| Han `8e9c9b7` | `4aef91de56b2e0c4756103ebedeae821f1570dec` | `489776deb8f3f3f0df46d9b3998109af3c54c264` | +742/-17 (net +725) | +100/-5 (net +95) |

The final commit subjects are all the same deletion fix, but the full patches are not equivalent. Han's parent `4119698` is an authority/deletion predecessor, not an exact Ozzy patch transplant.

## Gates observed

On Furiosa `8c8216e`:

- Focused race filter matched and passed both packages: `internal/cli` and `internal/index`; wall time 7.77s. It covered context timeout, fenced lookup seam, whole-session overlay replacement/deletion, delayed publication, cancellation, origin authority, sole-source deletion, co-contributor preservation, and unchanged topic row-ID stability.
- Full `CGO_ENABLED=0 go test -race -count=1 ./internal/cli`: PASS, wall time 63.89s.
- Full `CGO_ENABLED=0 go test -race -count=1 ./internal/index`: PASS, wall time 81.44s.
- Full `CGO_ENABLED=0 go test -race -count=1 ./...`: PASS, wall time about 80.7s. All repository packages completed; no timeout or failure.
- Required test filters were nonzero. The initial combined filter was corrected after an accidental package-regex mismatch; the rerun explicitly matched the CLI tag/overlay family and passed.

Required immutable red proofs reproduced on their exact commits:

- Ozzy `9c845ed`: `TestOverlayAuthoritativeTopicsDropsDeletedBoundary` FAIL; stale derived `deleted-by-retag` row remained.
- Han `fab3c3d`: `TestRunTagPublishChildHonorsCancellationWhileWaitingForFence` FAIL; canceled child waited through the fence and failed its 300ms assertion.
- Ozzy `bfc2fbd7`: `TestConsolidate_DeletesTopicsRemovedFromSource` FAIL; removed topic B remained (`1` row).

## Semantic checks

- Overlay is a complete authoritative per-session replacement. Furiosa drops derived-only boundaries, including an empty authoritative set. Ozzy's overlay union fails the immutable deletion red.
- Consolidation preserves a co-contributor's topic and does not rewrite an unchanged sole-source topic row ID. Furiosa's tests pass these checks.
- Origin authority is monotonic: the higher `origin_machine` wins regardless of timestamp; equal origin uses newer `tagged_at`. Furiosa's two subtests pass.
- The `ff094cb` scoped pre-write lookup test remains correctly labeled as a seam limitation: it proves explicit scoped lookup can block before publication; it is not evidence that the generated default nil-scope command path is fixed.
- Furiosa propagates the child context through fence acquisition, context-aware query/transaction operations, and phase boundaries. This is directly proven by its fenced 80ms timeout test. Ozzy's implementation retains `context.Background()` in the fold and fails the Han cancellation red.
- No speed ranking was inferred. The observed times are gate wall times only; they are not a performance claim.

## Han-specific falsification

I added a disposable test (then removed it; Han worktree returned clean) with source A containing a two-boundary session and unrelated source B. After folding `[A,B]`, source A was retagged to remove its second boundary and `[A,B]` was folded again. Han returned one stale row for the removed sole-source boundary. The cause is visible in `ConsolidateFrom`: `consolidateOneFor(con, src, len(srcPaths) == 1)` disables the stale-topic deletion predicate for every multi-source pass. This is a real correctness failure, not a patch-size preference.

## Adoption/scoring implication

Adopt Furiosa's exact candidate. Ozzy's smaller net diff is not semantically acceptable because it fails overlay deletion and cancellation. Han should receive no product-correctness adoption credit for the larger patch; its cancellation/overlay portions are valid, but the multi-source pruning gate blocks it as a whole candidate. Any scoring for process or external mechanism adoption must remain separate from product correctness.

Remaining uncertainty: the focused and full races exercise the checked-in tests and required scenarios; no production edit or new permanent test was made by this referee. The default nil-scope `tag-prep` path remains the known `ff094cb` seam limitation and should not be advertised as eliminated by this winner.
