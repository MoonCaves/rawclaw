# Seven-blade findings: issue-32 retry test

## ponytail

- `internal/index/consolidated_test.go:1796-1914` — `shrink`: keep the child HOME handoff, committed-row checks, source mutation, and retry count, but remove diagnostic artifact enumeration and combine independent source mutation statements where SQLite permits it. Test net-line opportunity: about -18. Helped: yes.

## ponytail-review

- `internal/index/consolidated_test.go:1844-1860` — `delete`: post-exit artifact logging is observational ceremony, not a required assertion; retain the lock assertion and database queries. Test net-line opportunity: -11. Helped: yes.

## ponytail-audit

- `internal/index/consolidated_test.go:1796-1914` — `yagni`: no new helper, interface, factory, or dependency is justified for one same-store subprocess characterization test. Test net-line opportunity: avoid additions. Helped: yes.

## modular-refactor

- `internal/index/consolidated_test.go:1796-1914` — `yagni`: this is a small, already-seamed test around `ConsolidateFrom`; right-sizing says stop rather than extract a test adapter. Test net-line opportunity: 0 added. Helped: yes.

## codebase-design

- `internal/index/consolidated_test.go:1796-1914` — `shrink`: the existing test crosses the correct interface and already has a deep production module behind it; adding a test-only seam would make the interface shallower. Test net-line opportunity: 0 added. Helped: yes.

## golang-how-to

- `internal/index/consolidated_test.go:1878-1906` — `stdlib`: use existing `database/sql` execution and current test helpers; no custom abstraction is needed. Test net-line opportunity: small reduction by consolidating mutation setup. Helped: yes.

## golang-modernize

- `internal/index/consolidated_test.go:1796-1914` — `delete`: no relevant Go 1.24+ modernization shrinks this test without changing fault timing or assertions. Test net-line opportunity: 0. Helped: no.

## Verdict

Preserve same-store HOME restoration, post-merge/pre-DETACH fault, committed session/watermark/lock evidence, source-row mutation, retry count, and timing. Delete only redundant artifact logging and setup ceremony.
