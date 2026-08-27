# Hostile rival census: sidecar correctness T52

Base under review: `48661f403f880e2c1dac7615f39bbb8264eeafe7`.

## Method rules applied

- `golang-testing`: make the regression an observable behavior test; use an
  exact named test and an auditable anchored `go test -list` preflight before
  `-run`; keep the test independent and race-gated; assert both deletion and
  co-contributor preservation rather than implementation details.
- `golang-concurrency`: distinguish process-local transaction behavior from
  cross-process ownership; do not infer safety from a green unit test; keep
  the database mutation atomic and avoid adding concurrency to the proof.
- `golang-context`: preserve the existing context propagation when examining
  the context-aware consolidation path; do not create a detached request
  context inside the fold.
- `golang-safety`: treat stale sidecar deletion as data-loss-sensitive; scope
  deletes by contributor identity and use explicit absence checks rather than
  broad cleanup; do not assume a missing table is equivalent to an empty
  contributor set without tracing the ownership rule.
- `golang-troubleshooting`: reproduce the stated trap first, use one
  hypothesis at a time, trace callers and SQL data flow to root cause, and do
  not propose a fix before a failing regression is observed.
- Ponytail: prefer the smallest existing SQL mechanism; reuse the affected
  session set and contributor predicates, and add no abstraction unless the
  current code cannot express the invariant.
- Ponytail Review: classify unnecessary machinery with `delete`, `stdlib`,
  `native`, `yagni`, or `shrink` and account for net lines; correctness is
  separate from the over-engineering review.
- `modular-refactor`: right-size the change, preserve the existing seam and
  behavior with a characterization guard, and keep the change localized.
- `codebase-design`: reason in terms of the deep module, its interface
  invariants, seam, implementation, and locality; do not widen the interface
  for a one-case fix.
- Graphify: orient from graph evidence before source inspection, use literal
  graph-vocabulary queries plus `explain` and `path`, and record whether the
  result was useful, a dead end, or corrected.

## Preliminary current-base finding

`internal/index/consolidated.go` only prunes stale topic rows inside
`if hasTopics` and stale verdict rows inside `if hasVerdicts`. Therefore a
source database that has neither sidecar table cannot prune sidecars left in
the consolidated store for a session removed from that source. The intended
ownership rule is narrower: delete a sidecar only when its source contribution
is absent and no other `session_sources` contributor remains; preserve the
sidecar when a co-contributor remains.

This is a hypothesis until the exact-one regression test is discovered and
run against the current base. No test or production edit has been run yet.

## Review boundaries

Files permitted by the task: this report, `RIVAL_SIDECAR_CENSUS_T52.md`,
`internal/index/consolidated_test.go`, and `internal/index/consolidated.go`
only if the regression proves the current-base bug. No merge, score, or merge
authorization is claimed.
