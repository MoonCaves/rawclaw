# Six-skill anti-bloat audit

Target: `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` (`conor/six-skill-audit`).
Scope is report-only. Findings are ranked by estimated removable lines, with
mechanical duplication treated as a lead that still requires human judgment.
No correctness, security, or performance claim is inferred from a style finder.

## Ranked findings

1. `internal/store/connect_bench_test.go:132-450` — `shrink:` twelve warm/cold benchmark bodies repeat the same setup, query, assertions, and cleanup for four connection constructors; replace with table-driven `runSearchBench`/`runBrowseBench` helpers that retain the existing subtest names and connector matrix; estimated net **-120 test lines**; **RULING: needs contract test** (preserve all eight search/browse names and warm/cold modes).
2. `internal/store/topics.go:198-244` — `shrink:` `SessionIDsIn` and `TaggedSessionIDs` duplicate the rows-close/scan/rows.Err loop; replace with one private query-and-scan helper parameterized by SQL and error label; estimated net **-18 production lines**; **RULING: fix now**.
3. `internal/store/stats.go:25-53` — `shrink:` `CountSessions` and `CountTopLevelSessions` duplicate connection/error/query boilerplate; replace with one private `countSessions(dbp, query)` helper and two one-line public wrappers; estimated net **-11 production lines**; **RULING: fix now**.
4. `internal/paths/paths_test.go:611-685` — `shrink:` direct-hit and prefix-hit tests duplicate catalog setup and four assertions; replace with a table-driven case carrying the id, query prefix, cwd, and expected session/project; estimated net **-25 test lines**; **RULING: needs contract test** (retain exact-vs-prefix resolution coverage).
5. `internal/store/verdict.go:167-184` — `delete:` `RoutineVerdictSet` has no source callers on `cdc063d` (`rg -n 'RoutineVerdictSet\\(' --glob '*.go'` finds only its declaration); delete the dead 18-line helper; replacement: `RoutineSet`, the only current caller-facing routine-set implementation; estimated net **-18 production lines**; **RULING: fix now**.

**Total confirmed estimated net deletion: -192 lines (-145 test, -47 production).**

The total is an estimate from current line spans and the smallest stated
replacement shape, not a claim that an unperformed refactor has already passed
tests.

## Six supporting skills

### `ponytail-review`

Contributed the required one-line finding vocabulary (`delete`, `stdlib`,
`native`, `yagni`, `shrink`) and the net-line scoring discipline. It kept the
report focused on complexity rather than correctness.

### `ponytail-audit`

Contributed the whole-tree hunt: dead helpers, repeated loops, duplicated test
scaffolding, and single-purpose wrappers. It supplied the ranking rule: biggest
proven cut first.

### `modular-refactor`

Contributed the right-sizing and guard requirement. The proposed shared helpers
are private seams with two or more callers; the benchmark and path-test cuts
retain characterization coverage before any refactor.

### Matt Pocock `codebase-design`

Contributed the deep-module test: prefer a small private interface with shared
behavior and locality, while rejecting a new abstraction when it would merely
add a one-implementation pass-through. No new exported interface is proposed.

### `golang-how-to`

Contributed Go-specific routing to style, testing, database, and error-flow
review. Existing `rows.Err()` handling was checked; this lane did not turn
correctness concerns into bloat findings.

### `golang-modernize`

Contributed the Go-version and `modernize`-linter scan. `golangci-lint run
--enable-only modernize ./...` returned clean, so no modernization finding is
invented merely to increase the deletion score.

## Receipts and stale-audit corrections

- `graphify reflect --if-stale`: 3.394s; no prior marked lessons.
- `graphify . --no-viz`: refreshed the branch graph at 17:14:38; graph query,
  explain, and path were run before source verification. Graphify inferred
  callers for `RoutineVerdictSet`; source `rg` disproved those edges, and the
  correction was saved with `graphify save-result`.
- `/Users/jay-m4/go/bin/dupl -plumbing -threshold 80 internal`: 11.764s;
  confirmed the repeated benchmark, topics, and path-test spans used above.
- `/Users/jay-m4/go/bin/golangci-lint run --enable-only modernize ./...`:
  15.597s, clean, no issues reported.
- The prior `IndexStatusUnknown` and transactional `restoreSession` findings
  were rechecked at current source and are stale: both fixes are present on
  `cdc063d`. The prior `SessionHasRealSegments` correction is also present.
