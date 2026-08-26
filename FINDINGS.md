# Hostile default-scope mutation findings

Base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.

The following Ozzy tips descend directly from `740101c3ba8380b91ad8e7657a39e2587fbcf115`:

| tip | claim | diff from base | patch-id |
| --- | --- | ---: | --- |
| `497818fa1e02a6b799bd862f2585fe1a9618cc42` | catalog/source fast path | +1230/-35 (15 files) | `2d84aa387c9eab1fb21f58990b9c3a5f0ea4ade4` |
| `cb339acf8db4043775cc512b9926e76b5526aa16` | detached tag-prep fold | +1163/-90 (15 files) | `7411fcf4f4a3a8cd11044e2305c87b90eb747037` |
| `857dc62414426b540b57a497609122721982a367` | combined catalog + detached fold | +1266/-97 (16 files) | `d2181e52dfcced9cfcf5a2f15df04ea0b644bca0` |

Rival worktrees were clean when inspected. Their focused race gates passed:

- `497818f`: catalog nil-scope, ambiguity, DB/TDir author-before-fence tests passed (`2.152s`).
- `cb339ac`: tag-prep source refresh, contention/detached-fold, stale/failure tests passed (`3.638s`).

The exact-base reproduction in `internal/cli/hostile_default_scope_test.go` is intentionally red. It seeds a consolidated-only retained session, holds `consolidated.lock`, then invokes the generated command’s `runTagWriteCmd` shape with `scope=nil`. The command resolves `consolidated.db` and blocks acquiring the same fence; the 300ms terminal assertion fails. This path has no source/catalog row for the candidate fast path to use, so the three candidate tips do not address it.

Observed focused receipt:

```text
--- FAIL: TestTagWriteDefaultScopeConsolidatedOnlyDoesNotBlock (0.37s)
hostile_default_scope_test.go:66: default nil-scope tag-write blocked behind the held consolidated fence
```

This is a narrow liveness finding, not a universal performance or merge verdict. Publication, cancellation, and terminal visibility for the surviving source/catalog path were not claimed beyond the focused candidate tests above.
