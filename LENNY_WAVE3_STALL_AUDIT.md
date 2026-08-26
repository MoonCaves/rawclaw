# Lenny Wave 3 stall audit

Report-only audit performed 2026-08-26 from the RawClaw checkout. No Lenny
worktree was edited, reset, or tested with writes. Graphify was refreshed and
queried first (`graphify reflect --if-stale`, then a literal query over
`containers`, `fence`, `hooks`, `locate`, and `prewarm`). The supplied review
skills were read before inspection.

## Gate evidence

- `rawclaw-lenny-raid-phase` at `c3b3d2bcdf9fbd26b27fae76277c21d33789fca2`:
  `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...` PASS,
  package 105.672s, wall 106.87s.
- `rawclaw-lenny-raid-hooks` at `b0d9e0fc5890f653fb17aefa66917c5800a87f26`:
  `CGO_ENABLED=0 go test -race -count=1 -run 'TestPrimeScript|TestSetupCmd' ./internal/cli`
  PASS, package 8.102s, wall 8.96s.
- `rawclaw-lenny-skill-architecture` at
  `b5f570baeb30522c0e002427ff4ec0177a04b3b7`:
  `CGO_ENABLED=0 go test -race -count=1 -run '^$' ./internal/store/...`
  PASS (no tests run), wall 2.70s. This is compile evidence only.

The supervisor supplied independent context that the integrated baseline
`bd8346c` is clean and full-repo green. That does not convert an unmerged
Lenny head into a release claim.

## Ranked rulings

### 1. `raid-hooks` — HOLD for the head; production salvage is plausible

Shared ancestor: `479d14c782a229d3348b290885028c5efa7a8740`.

Tip patch ID: `eef44604f862b9062abedd00638cb6a1502a2720`.
Tip delta: 13 added / 95 removed in `internal/cli/cmd_ingest_test.go`, plus
13 added / 27 removed in `FINDINGS.md`; cumulative ancestor delta is +595 / -193
(net +402).

The focused hook matrix passes, but the claimed path-safe catalog claim is not
present in this head. Both hook templates assign `entry="$catalog_dir/$session_id"`
at `internal/cli/setup.go:82` and `:165`, then derive temporary paths directly
from the same untrusted value at `:91-92` and `:174-175`. The tests exercise
ordinary IDs (`catalog_hook_test.go:34`, `:118`, `:210`, `:278`, `:378`) and do
not falsify slash, `..`, or other path-component input. A malformed ID falls
through to fail-soft ingest, but a slash-bearing ID can still address a path
outside the catalog directory before that fallback. The later integrated
invalid-ID advisory is therefore not evidence for this immutable Lenny head.

Ruling: HOLD the head. ACCEPT only the separately reviewed catalog-claim
implementation after a path-component guard and hostile-ID test land.

### 2. `raid-phase` — ACCEPT, narrowed to logger/test seam

Shared ancestor: `479d14c782a229d3348b290885028c5efa7a8740`.

Tip patch ID: `4d0d2aa5cd93bb8c3fd505b920a1b857f45b4886`.
Tip delta: 24 added / 13 removed; cumulative ancestor delta is +111 / -49
(net +62), including the prior phase-helper transplant.

The production helper at `internal/index/consolidated.go:650-659` removes
duplicated phase logging and preserves source attribution. The fence uses the
same logger at `internal/index/consolidated_fence.go:35-40` and `:80-88`.
The race gate passed. Caveat: `consolidatePhaseLogger` is a mutable package
global test seam (`consolidated.go:640-647`), so it must remain non-parallel
test-only usage; no evidence supports making it a general runtime extension.

Ruling: ACCEPT the helper/scoped logger change, narrowed to the existing test
seam. Do not expand the global hook or claim a production logging feature.

### 3. `raid-containers` — HOLD the head for assertion deletion

Shared ancestor: `479d14c782a229d3348b290885028c5efa7a8740`.

Tip patch ID: `9993c0b0a6afdbc1c7f2aa1db422bee441909fb7`.
Tip delta: 0 added / 99 removed in `internal/index/containers_test.go` and
15 added / 6 removed in `FINDINGS.md`; cumulative ancestor delta is +167 / -30
(net +137).

The production sidecar-generation cleanup at `internal/index/containers.go:50-84`
is a coherent change, and `containerMeta` extraction at `:581-608` removes
duplication. However, the tip deletes the only direct table-driven contract
test for `containerMeta` (`containers_test.go` prior tip lines 812-913),
including source size, fingerprint, subagent parent, missing-file behavior,
and backing-path assertions. That is assertion deletion, not simplification
of a redundant test; the replacement coverage was not demonstrated by this
head.

Ruling: HOLD until those durable-meta assertions are retained or replaced by
an equally direct contract test. The production refactor may be reviewed
separately.

### 4. `raid-locate` — ACCEPT, with semantic coverage credited

Shared ancestor: `479d14c782a229d3348b290885028c5efa7a8740`.

Tip patch ID: `69c6ebec30a7689525a42f6b6e567a39dee722c0`.
Tip delta: 101 added / 0 removed in tests; cumulative ancestor delta is
+180 / -84 (net +96), including the production resolver/range simplification.

The production changes use `LocateConsolidatedSession` in
`internal/cli/tagrefresh.go:116` and centralize range resolution in
`internal/cli/cmd_tag.go:260`. The added matrix tests cover exact, unique, and
ambiguous lookup plus prep/write mutation refusal. No duplicate patch ID was
found among the ten audited tips.

Ruling: ACCEPT the production salvage and its matrix tests. This is a real
implementation/test stack, not a report-only head.

### 5. `raid-prewarm` — ACCEPT the small production deletion; head is otherwise report-heavy

Shared ancestor: `479d14c782a229d3348b290885028c5efa7a8740`.

Tip patch ID: `d41e349fcc07aa931283552c6c1964bb931acc51`.
Tip delta: 7 added / 5 removed in `FINDINGS.md`; cumulative ancestor delta is
+85 / -14 (net +71). The only novel production patch is the impossible-error
cleanup in `internal/cli/setup.go:901-940` (commit `fa485c8`), which removes
an error return that the helper cannot produce.

Ruling: ACCEPT `fa485c8` as a narrow deletion. The final `0635190` head itself
adds documentation only and is NO-NOVELTY beyond that production commit.

### 6. `raid-fence` — ACCEPT as test hardening, no production novelty

Shared ancestor: `479d14c782a229d3348b290885028c5efa7a8740`.

Tip patch ID: `10f365a2d574e1b722fb69959e1e895967b27300`.
Tip delta: 74 added / 0 removed in `consolidated_fence_test.go`, 20 added /
7 removed in `consolidated_test.go`; cumulative delta is +203 / -7 (net +196).

Ruling: ACCEPT the timeout/order assertions as test-only hardening. No novel
production implementation is present in the tip.

### 7. `skill-architecture` — ACCEPT benchmark shrink, compile-only gate

Shared ancestor: `bf7cdd0de71f8fbfd6e86c34852062f0766fddc7`.

Tip patch ID: `f53f9c38b28681f4c90883d1a555598b4e604676`.
Tip delta: 65 added / 298 removed in `connect_bench_test.go`, plus 48 added /
30 removed in `FINDINGS.md`; cumulative delta is +133 / -298 (net -165).

The generic benchmark runner begins at `internal/store/connect_bench_test.go:141`
and replaces repeated warm/cold connector loops. The compile-only race gate
passed in 2.70s; no benchmark timing claim was made.

Ruling: ACCEPT as a test/benchmark-only shrink. Do not treat it as a production
performance result without running the benchmark matrix.

### 8-10. `skill-modernize`, `skill-interfaces`, `skill-style` — NO-NOVELTY

Shared ancestor for all three: `bf7cdd0de71f8fbfd6e86c34852062f0766fddc7`.

- `5e65260ac73d49089514674e17aa79f9d7142a32`, patch ID
  `9a69678d02ffab4d9c1f3747e8b7b7998feb9f81`: `FINDINGS.md` only, +117 / 0.
- `997016fe40f330611bc7dbdd6e29ef57be73e837`, patch ID
  `fafec34081aaf1db5e2efa196982d5c6f4f5eed0`: `FINDINGS.md` only, +165 / 0.
- `354b0d87414e0f5d47a8627dd6b96910cbed00b4`, patch ID
  `820f12e8c8f1c62ec715614195efe6a939fc914d`: `FINDINGS.md` only, +124 / 0.

Ruling: NO-NOVELTY for each tip. Their branch history contains prior shared
fixes, but the named immutable heads add only report text.

## Ponytail findings

- `internal/index/containers_test.go` prior tip lines 812-913: `delete:`
  assertion deletion is unsafe; restore the direct durable-meta contract test.
- `internal/index/consolidated.go:640-647`: `yagni:` keep the logger override
  as a narrowly scoped test seam; do not grow it into a runtime abstraction.
- `internal/store/connect_bench_test.go:141-181`: accepted `shrink:`
  consolidation; the table-driven runner is shorter than the deleted repeated
  matrix and uses the current `b.Loop` idiom.
- `internal/cli/setup.go:82,91-92,165,174-175`: `delete/shrink:` no amount
  of test prose substitutes for validating the session ID before path joining;
  use one shared path-component guard for both templates.

net: -165 lines possible.
