# Ponytail findings: consolidated fence tests

Baseline: `6ddd17a` (`test(index): harden fence acquisition timeout and phase log ordering assertions`). Scope is limited to the requested test files; production code is untouched.

1. `shrink` — `internal/index/consolidated_fence_test.go:21-29,64-72,88-96`: the three tests repeat the same `flock.New`/`TryLock`/`defer Unlock` orchestration. Extract one small test helper returning an unlock cleanup, then reuse it. RULING: restore the exact lock path, acquisition failure message, and cleanup semantics; do not merge the tests or weaken contention setup. Expected net reduction: about 14 lines.
2. `shrink` — `internal/index/consolidated_test.go:39-73` and `internal/index/consolidated_fence_test.go:114-151`: both tests scan `slog.Record` attributes to recover phase/event/duration positions. RULING: accepted deviation for this pass; the tests assert different scopes (successful full fold versus timed-out fence), and a shared parser would add a new abstraction whose net line benefit is not established.
3. `delete` — none confirmed. The timeout test and strict start-before-duration checks pin distinct observables and must remain.

RULINGS FOR IMPLEMENTATION

- Only apply finding 1 if the resulting diff is shorter and `gofmt` leaves the test behavior unchanged.
- Do not alter production files, timeout values, lock paths, log messages, or assertions about ordering and elapsed duration.
- If focused race tests show flakiness or the helper obscures the contract, revert the shrink and keep this report as the complete audit.

Net possible: approximately `-14` test lines, `0` production lines, `0` dependencies.
