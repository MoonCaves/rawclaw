# NORM PHASE CONTRACT TAKEOVER — FINDINGS & RULINGS

**Lane**: NORM PHASE CONTRACT TAKEOVER (`lenny/raid-phase-20260826`)
**Base Commit**: `479d14c` (`origin/integrate/tagwrite-closeout-wave1`)
**Rival Target**: `/Users/jay-m4/code/rawclaw-norm-phase`, branch `norm/phase-contract`, reported final `33c7421`
**File Fence**: `FINDINGS.md`; `internal/index/consolidated.go`; `internal/index/consolidated_test.go`; `internal/index/consolidated_fence.go`; `internal/index/consolidated_fence_test.go`

---

## Finding 1 — Duplicated Phase Lifecycle Closures Across Consolidation Entry Points

- **Rival SHAs**: `33c7421` / `6e7d29a` (`norm/phase-contract`), `7e86623` (`norm/phase-adversarial-review`), `fc7cfde` (`luna/phase-timing-20260826`)
- **File & Line**: `internal/index/consolidated.go:470-488`, `580-598`, `665-671`, `691-699`, `781-795`
- **Reproducible Evidence**:
  `ConsolidateFrom`, `SyncConsolidatedFrom`, and `consolidateOne` each define identical local closures `phase := func(name string) func()` capturing `started := time.Now()` and emitting `"consolidate fold phase"` start and duration log records. Additionally, the `connection-close` defer blocks in `ConsolidateFrom` and `SyncConsolidatedFrom` and the `detach` defer block in `consolidateOne` duplicate raw `started := time.Now(); slog.Info(...)` blocks by hand.
- **RULING**: `ADAPT_TO_CURRENT`
- **Action**: Extract a single package-private helper `beginConsolidatePhase(src, name string) func()` in `internal/index/consolidated.go` that captures `started := time.Now()` before start emission and emits identical log messages, phase names, `event=start`, and elapsed `duration` values. Route `connection-close` and `detach` defers through this helper. Net reduction: -10 lines in production code without behavioral or logging drift.

---

## Finding 2 — Consolidation Phase Starts and Durations Contract Test

- **Rival SHAs**: `2ee9950` (`HEAD`), `6e7d29a` (`norm/phase-contract-fix`), `33c7421` (`norm/phase-contract`)
- **File & Line**: `internal/index/consolidated_test.go:19-82`
- **Reproducible Evidence**:
  `TestConsolidate_LogsPhaseStartsAndDurations` (pinned in `2ee9950`) exercises all 11 consolidation phases (`schema-migrate`, `source-migrate`, `attach`, `prepare`, `merge`, `detach`, `tombstone-prune`, `watermark-stamp`, `connection-close`, `acquire`, `release`) against a custom `slog.Handler` recorder, asserting that every phase produces both a start event and a typed `time.Duration`.
- **RULING**: `CLEAN`
- **Action**: Keep `TestConsolidate_LogsPhaseStartsAndDurations` as the contract verification referee.

---

## Finding 3 — Post-Merge Exit Same-Store Retry Hardening (Issue #32)

- **Rival SHAs**: `479d14c` (`HEAD`), `c14e806` (`HEAD~2`), `fd01a92` (`norm/integration-wave1`)
- **File & Line**: `internal/index/consolidated_test.go:100-220`
- **Reproducible Evidence**:
  `TestConsolidate_PostMergeExit_RetryRecoversSameStore` in `479d14c` correctly restores the parent `HOME` inside the child process to avoid isolation divergence, verifies post-exit lock and SQLite artifacts, mutates the source DB to force a non-watermark fold, and observes 5 race-enabled retries completing in ~63-72ms.
- **RULING**: `CLEAN`
- **Action**: Preserve `479d14c`'s test implementation intact as a verified negative reproduction and regression guard.

---

## Finding 4 — Cross-Lane Refactoring of `containers.go` & Rebuild Guard Removal

- **Rival SHA**: `9d6564d` / `5c50c7c` (`norm/phase-contract`)
- **File & Line**: `internal/index/consolidated.go:418-422`, `internal/index/containers.go:575-610`
- **Reproducible Evidence**:
  `9d6564d` touched `internal/index/containers.go` (extracting `containerMeta`) which violates this lane's file fence (owned by `containers` lane). Furthermore, removing `if rebuild {` around `prevLive = dst` contaminates non-rebuild tag preservation paths.
- **RULING**: `REJECT`
- **Action**: Reject `9d6564d`. Do not touch `internal/index/containers.go`. Maintain strict lane boundaries.

---

## Finding 5 — Luna 108-Line Phase Expansion & Schema Renaming

- **Rival SHA**: `8551a83` (`luna/phase-timing-20260826`)
- **File & Line**: `internal/index/consolidated.go`
- **Reproducible Evidence**:
  Expands `consolidated.go` by +108 lines and renames `schema-migrate` to `schema-heal-migrate`, breaking the established logging and telemetry contract.
- **RULING**: `REJECT`
- **Action**: Reject `8551a83` entirely.
