# Issue #31 phase instrumentation review

## Finding 1 — duplicated phase lifecycle closures

- Evidence: `internal/index/consolidated.go` duplicates the start/elapsed
  closure in `ConsolidateFrom`, `SyncConsolidatedFrom`, and `consolidateOne`.
- RULING: consolidate to one private helper that accepts an optional source and
  emits the existing message, phase names, `event=start`, and elapsed duration
  attributes exactly as currently emitted. No fold logic changes.
- Net target: remove at least 20 lines after adding the helper.

## Finding 2 — duplicated connection-close and DETACH instrumentation

- Evidence: connection close is hand-written twice and DETACH is a third copy;
  each must retain completion logging on all return paths.
- RULING: route all three through the same helper. Keep the DETACH defer and
  connection-close defers in their current ownership/order; keep ignored close
  errors and existing DETACH error policy unchanged.
- Net target: remove at least 10 lines.

## Finding 3 — fence timing must remain complete on every outcome

- Evidence: fence acquire/release already use start plus elapsed completion
  records; acquire completion is deferred so mkdir, lock error, timeout,
  cancellation, and success all report duration.
- RULING: use the shared helper for fence acquire/release only if the emitted
  message and attributes remain identical. Preserve `held` on release and the
  current error policy.
- Net target: remove at least 4 lines without changing fence behavior.

## Test ruling

Add one recorder-backed load-bearing test covering every issue #31 phase and
asserting each has both a start and duration record. Include actual DETACH and
fence completion; include fence timeout/error completion through the existing
test seams. Do not alter fold behavior, log levels, phase names, or error
handling policy.

## Rejected alternative

Luna `8551a83` is rejected wholesale: its 108-line expansion is not the
smallest change, and its `schema-heal-migrate` naming does not preserve this
branch's existing `schema-migrate` contract. Only the optional-source helper
shape is retained, with this branch's exact names/messages.
