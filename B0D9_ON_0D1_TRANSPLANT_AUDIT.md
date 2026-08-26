# b0d9e0f on current 0d1da19: transplant audit

Audit date: 2026-08-26. Source candidate b0d9e0fc5890f653fb17aefa66917c5800a87f26.
Its parent is c39872650a3ded47c7777e3ffad0ae3739b16f6b. Recipient/current tree is
0d1da19c4c21961b86cb3ca84ed047d941c83ed3. The candidate commit is test/docs-only;
its production parent behavior is not the transplant under review.

## Verdict: HOLD

Do not transplant b0d9e0f as a patch onto 0d1da19. There is no clean deletion subset
with positive net value on the current tree: current 0d1da19 does not contain the
hostile-path matrix or the standalone injected-directory test that b0 folds together.
Applying the commit therefore requires reconstructing a new test rather than deleting
redundant current code.

The dangerous divergence is visible in current cmd_ingest_test.go:133–274. Current 0d1
retains:

- TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest at :133–213
- TestPrimeScripts_SessionStartIngestsWhenCatalogUnavailable at :215–274

The latter is absent from b0’s tree. It is load-bearing for the fail-soft launch contract
when the catalog parent is a blocking file. Deleting or omitting it would weaken current
coverage. Net safe deletion: 0 lines. Ponytail ruling: HOLD; no delete/shrink transplant.

## What b0 actually deletes

Against its direct parent c398726, b0 changes:

| path | delta | result |
|---|---:|---|
| internal/cli/cmd_ingest_test.go | +12 / -95 = -83 | folds a standalone injected-directory test into a hostile matrix |
| FINDINGS.md | +13 / -27 = -14 | merges two review findings |
| production | 0 / 0 | no production change |

The deleted 95-line test is c398’s TestPrimeScripts_SessionStartDirectoryInjectedBeforeLinkDeduplicatesWithoutNesting
(c398 cmd_ingest_test.go:298–388). The surviving b0 matrix adds the same injection at
cmd_ingest_test.go:181–188 and checks no ingest, no nested artifacts, and no leaked temp
directory at :274–291. That shrink is internally sound on c398’s tree.

It is not a deletion available on 0d1: current 0d1 has neither the matrix nor the
standalone injected-directory test. Its test file instead contains separate, current
contracts for Stop/prewarm dispatch, catalog-unavailable fallback, concurrent
deduplication, and detached Claude ingest.

## Current-tree load-bearing coverage

The current 0d1 test contracts are not redundant:

- cmd_ingest_test.go:78–131,
  TestPrimeScripts_StopLaunchDetachedPrewarm — proves Stop dispatches prewarm without
  blocking or emitting the SessionStart banner.
- cmd_ingest_test.go:133–213,
  TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest — proves two concurrent
  starts produce one ingest call and waits long enough to observe the detached child.
- cmd_ingest_test.go:215–274,
  TestPrimeScripts_SessionStartIngestsWhenCatalogUnavailable — proves a catalog setup
  failure still launches fail-soft ingest and exits successfully.
- cmd_ingest_test.go:276–317,
  TestClaudePrimeScript_ExecutesDetachedIngest — proves the rendered Claude hook emits
  its banner and executes without blocking.

The b0 change also raises the concurrency case from 2 to 20 callers and tightens its
post-dispatch assertion. That is an optional test-strengthening addition, not a safe
deletion. It must be ported as a fresh, separately reviewed edit if desired.

## Production behavior and input boundary

Current 0d1 setup.go:60–96 and :159–195 already uses the basename-isolated hard-link
claim: private tmp directory, candidate basename equal to session_id, ln to the catalog
directory, cleanup, then detached ingest for the winner or fail-soft path. Therefore b0
does not supply a production fix to current 0d1.

The shared implementation still uses raw session_id in tmp_dir and tmp_entry names
(setup.go:72–73 and :171–172). Neither b0 nor current 0d1 tests slash-containing,
dot-dot, or shell-metacharacter IDs. This remains UNVERIFIED input-boundary coverage,
not a reason to delete current tests.

## Personally observed gate

Current 0d1, run from the clean integration worktree:

    CGO_ENABLED=0 go test -race -count=3 ./internal/cli -run 'TestPrimeScripts_(SessionStartDeduplicatesConcurrentIngest|SessionStartIngestsWhenCatalogUnavailable|StopLaunchDetachedPrewarm|ClaudePrimeScript_ExecutesDetachedIngest)'
    ok github.com/MoonCaves/rawclaw/internal/cli 7.913s
    wall: 9.151s

This is a focused gate only. No full CLI or repository gate was run for this audit.
No Go files were changed in the spy worktree; gofmt -w is N/A.

## Exact tree and patch evidence

- b0d9e0f is published at origin/lenny/raid-hooks-20260826.
- merge-base(b0d9e0f, c398726) is c398726.
- b0 direct diff: cmd_ingest_test.go +12/-95 and FINDINGS.md +13/-27.
- Current 0d1 cmd_ingest_test.go has no hostile matrix symbol and still has the
  catalog-unavailable test at :215–274.
- Current 0d1 setup.go has the hard-link implementation, so a production transplant
  from b0’s ancestry would duplicate existing behavior.

## Final ruling

Reject b0d9e0f as a direct transplant onto 0d1. Preserve all current 0d1 tests, including
catalog-unavailable fail-soft and detached-child assertions. If hostile path coverage is
wanted, port the matrix as a new test addition and run a separate review; it does not
create a safe deletion here. Safe net deletion: 0 lines. Status: HOLD.
