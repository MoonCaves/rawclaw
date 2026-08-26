# Locate/tagging salvage review

Base: `9d6564d`  
Candidate: `10572cff0d2933a7263cc41de3fc2d9f08f93bdc`  
Scope: the three candidate production files in the assigned fence. The candidate
commit has no test hunks and does not touch `FINDINGS.md`.

## Rulings

- `internal/agentproto/agentproto.go`:1797-1805 — **transplant-exactly**. Inline
  the one-use `allowed` closure into the existing loop. `nil` project scope still
  permits all projects, non-nil scope still uses the same `slices.Contains`
  predicate and preserves hit ordering and empty behavior. Net: -4 lines.
- `internal/cli/cmd_tag.go`:171-207 — **transplant-exactly**. Extract the exact
  existing UUID/message-ID range resolution into `resolveSegmentRange`; callers
  retain the same bounds, fallback, clamping, and tagging order. Net: -1 line in
  this hunk after adding the shared helper below (overall file net is tracked at
  the end).
- `internal/cli/cmd_tag.go`:282-328 — **transplant-exactly**. Reuse the same
  resolver in `findPrevSegment`; the candidate preserves pointer selection,
  `bestStart` tie behavior, and target containment. Net: -42 lines before the
  helper is counted.
- `internal/cli/cmd_tag.go`:329-376 — **transplant-exactly**. Add the shared
  `resolveSegmentRange` helper with byte-for-byte equivalent fallback and bounds
  semantics. This removes duplicated parsing without changing nil/empty results
  or ordering. Net: +19 lines.
- `internal/cli/cmd_tag.go`:554-566 — **transplant-exactly**. Return
  `store.UpsertVerdict` directly. The prior `source == VerdictSourceFloor` branch
  and unconditional nil return are dead after the write; error identity and
  propagation remain unchanged. Net: -6 lines.
- `internal/cli/tagrefresh.go`:112-121 — **transplant-exactly**. Replace the
  equivalent consolidated-only `LocateSession` probe plus nil scope builder with
  `LocateConsolidatedSession`; this preserves the no-sweep contract while using
  the named API. No behavior, error, or output change. Net: -4 lines.

## Rejections and tests

No hunks rejected. No candidate tests were deleted or altered; existing tests are
load-bearing and remain in place. No accepted deviations.

## Net-line accounting

Candidate production diff: 53 insertions, 84 deletions, **net -31 lines**.
The net reduction is wholly deletion/refactoring: no new dependencies, public
surface, output strings, error wrapping, source-adapter behavior, or scope
semantics.
