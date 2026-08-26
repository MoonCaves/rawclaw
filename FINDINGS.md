# Issue #32 adversarial review

## Verdict

**CLEAN — no production or test defect found in c14e806 + 178e8fc.**

## Findings

- **P0:** none.
- **P1:** none.
- **P2:** none.
- **P3:** none.

## Rulings

- Keep the child-process fault seam in `internal/index/consolidated.go`; it is
  test-only and exits from the merge-phase defer after the transaction commit,
  before the enclosing `DETACH` defer.
- Keep restoration of the parent `HOME` in the child helper. Package `TestMain`
  isolates every process, so omitting this would make the child create a
  different consolidated store and invalidate the repro.
- Keep the post-exit row/watermark assertions and the source mutation before
  retry. They prove shared-store commit visibility and prevent a watermark
  no-op from making retry timing meaningless.
- Keep WAL/SHM/lock observations as diagnostics only; do not assert their
  presence or size because SQLite sidecar state is implementation- and
  timing-dependent. The lock artifact is expected from the fence, but its
  existence is not evidence of SQLite recovery correctness by itself.

## Review scope

Reviewed commits `c14e806` and `178e8fc`, limited to the assigned consolidated
store fault-retry path and its test helpers. No production/test code changes
are authorized by this review unless a reproducible defect is found.

## Issue close-comment accuracy

The existing close comment citing `c14e806` alone is **materially misleading**
for same-store recovery. In c14, the child inherited the package `TestMain`
scratch `HOME` rather than the parent's `isolateCache` HOME, so the child wrote
to a different consolidated store; its successful retry was not evidence that
the parent could reopen the child's committed store. `178e8fc` corrected this
by passing `RAWCLAW_CONSOLIDATE_FAULT_HOME` and restoring it in the child.

Suggested replacement:

> Issue #32 repro validated in `178e8fc` (building on `c14e806`): the child
> restores the parent's isolated `HOME`, commits one session and its sync
> watermark, then exits from the merge defer before `DETACH`; the parent sees
> that same store, mutates the source, and a retry folds the second message.
> `CGO_ENABLED=0 go test -race -count=5 ./internal/index -run
> '^TestConsolidate_RetryAfterAbruptPostMergeExit$'` passed five times, with
> retry durations observed at 120–224 ms and no multi-second stall reproduced.
