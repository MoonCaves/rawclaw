# Hostile Review Findings: Raid & Fence Candidates

## Base State & Scope
- **Target Branch**: `lenny/raid-fence-20260826`
- **Base Ref**: `origin/integrate/tagwrite-closeout-wave1` (`479d14c`)
- **Focus Area**: Universal consolidated store writer fencing, fold phase instrumentation (Issue #31), failed acquisition timing, and post-merge abrupt exit / retry robustness (Issue #32).

---

## Candidate 1: `work/luna-fence`
- **Rival SHA**: `91741a09de5472fa73a8cbf33ebfd67e3c06046d`
- **Locations**:
  - `internal/index/consolidated_fence.go:1-105`
  - `internal/index/consolidated_fence_test.go:1-56`
  - `internal/index/rebuild.go:80-88`
  - `internal/index/consolidated.go:403`, `538`, `613-637`, `696-701`
- **Hostile Review & Reproducible Evidence**:
  - `91741a0` introduced the initial `ConsolidatedFence` flock abstraction and initial fold phase logging.
  - In today's base, `91741a0` was merged (`2388ea1`) and hardened:
    - Timer reallocation on poll was eliminated (`e1f88fa` in `consolidated_fence.go`).
    - Fencing was extended to direct tag-write commands (`7344304` in `cmd_tag.go`).
    - Fold phase timing was extended to cover all tail phases (`e4b7695` and `2dca4d9`).
- **Ruling**: `CLEAN`
  - The fence architecture is fully present and further hardened in current `HEAD`.

---

## Candidate 2: `luna/phase-timing-20260826`
- **Rival SHA**: `fc7cfdea89f8c1bdcf83a380dd9e0568e2418618`
- **Locations**:
  - `internal/index/consolidated.go:662`
  - `internal/index/consolidated_fence.go:38-40`
- **Hostile Review & Reproducible Evidence**:
  - Added paired `event=start` and `duration` logs for fold phases and deferred timing for fence acquire.
  - Verified in current `HEAD`: `internal/index/consolidated.go` and `internal/index/consolidated_fence.go` emit start and duration events for all 9 fold phases (`schema-migrate`, `source-migrate`, `attach`, `prepare`, `merge`, `detach`, `tombstone-prune`, `watermark-stamp`, `connection-close`) and both fence phases (`acquire`, `release`).
  - Running `go test -v -run '^TestConsolidate_LogsPhaseStartsAndDurations$' ./internal/index` asserts the complete logging contract.
- **Ruling**: `CLEAN`
  - Fully integrated in current `HEAD` via `2dca4d9` and pinned by `2ee9950`.

---

## Candidate 3: `luna/fault-repro-20260826`
- **Rival SHA**: `a32c4a183393386a575b7b3bdbd1aa83f3a1194e`
- **Locations**:
  - `internal/index/consolidated.go:32`, `788-793`
  - `internal/index/consolidated_test.go:1730-1783`
- **Hostile Review & Reproducible Evidence**:
  - Attempted to reproduce Issue #32 via `consolidateAfterMergeHook` and child subprocess `TestConsolidate_FaultInjectionHelper`.
  - **Defect Identified**: `a32c4a1` did not propagate the parent's `HOME` directory to the child. Because RawClaw's `TestMain` isolates cache directories per process based on `$HOME`, the child created an isolated store in its own temp dir rather than writing to the parent's test store.
  - Furthermore, `a32c4a1` did not mutate the source after the child exited, meaning the retry was a watermark no-op rather than real merge work.
  - Both defects were resolved in `479d14c` (`RAWCLAW_CONSOLIDATE_FAULT_HOME` environment propagation, presence check for `-wal`/`-shm`/`consolidated.lock`, and source mutation requiring `st.Messages == 2`).
- **Ruling**: `CLEAN`
  - Superseded by the corrected implementation in `479d14c`.

---

## Candidate 4: `luna/conor-31-log-tests-20260826` (Conor Fence-Test Takeover)
- **Rival SHAs**:
  - `2bb219f8aeb412dbf9add6fe691cf606ad8805f1` (`test(index): capture consolidated phase timing logs`)
  - `d5d036b9dd94c59a9ee3da2da8fb8d1039cb671d` (`test(index): keep failed fence timing proof`)
- **Locations**:
  - `internal/index/consolidated_logging_test.go:1-145` (in `2bb219f`)
  - `internal/index/consolidated_logging_test.go:1-68` (in `d5d036b`)
- **Hostile Review & Reproducible Evidence**:
  - In `2bb219f`, Conor added duplicate fold logging tests and a failed-fence acquisition timeout log test in a new file `consolidated_logging_test.go`.
  - In `d5d036b`, Conor pruned the duplicate fold logging test (recognizing that `2ee9950` in main already owned the 9-phase fold contract) and kept only `TestConsolidatedFence_LogsAcquireDurationOnTimeout`.
  - **Defects in `d5d036b`**:
    1. **Global Slog Race**: Mutated `slog.SetDefault()` inside test execution, creating race conditions under parallel test execution.
    2. **Structural Duplication**: Introduced a standalone test file `consolidated_logging_test.go` and defined a redundant `consolidatedLogRecorder` type instead of using the canonical test infrastructure in `consolidated_fence_test.go` and `testhelpers_test.go`.
    3. **No Added Coverage**: The deferred start and duration logging in `AcquireConsolidatedFence` is already active and verified in `TestConsolidatedFence_ReportsHolderOnceAfterThreshold` (0.12s) and `TestIsBusy_RecognizesConsolidatedFenceTimeout` (0.06s), while `TestConsolidate_LogsPhaseStartsAndDurations` pins the complete 9 fold phases plus fence acquire/release contract.
- **Ruling**: `REJECT`
  - Reject duplicate file and struct from `d5d036b`/`2bb219f`. Current HEAD already has clean, race-free coverage.

---

## Candidate 5: `remotes/candidates/conor-32a`
- **Rival SHA**: `8947c217e1c9c980d5956159c38583fce23bfe9a`
- **Locations**:
  - `internal/index/consolidated_fault_test.go:1-85`
- **Hostile Review & Reproducible Evidence**:
  - Split fault injection into a separate file `consolidated_fault_test.go` and forwarded `HOME` / `XDG_*` variables.
  - **Defect Identified**: Does not mutate the source before the retry, so the retry simply exits immediately on the existing sync watermark and fails to test the post-exit merge path.
- **Ruling**: `REJECT`
  - Incomplete verification; superseded by `479d14c`.

---

## Candidate 6: `remotes/candidates/conor-32b`
- **Rival SHA**: `cece0a5956fd7692746415ffe67b1db25e093bff`
- **Locations**:
  - `internal/index/consolidated_fault_test.go:90-132`
- **Hostile Review & Reproducible Evidence**:
  - Mutated source before retry and asserted message count equals 2.
  - This exact behavior is already live in `HEAD` (`internal/index/consolidated_test.go:1879-1906`), which also records and verifies the `-wal`, `-shm`, and `consolidated.lock` file states.
- **Ruling**: `CLEAN`
  - Active and verified in `HEAD`.

---

## Summary Matrix

| Candidate | SHA | Ruling | Rationale |
|---|---|---|---|
| `work/luna-fence` | `91741a0` | `CLEAN` | Writer fence architecture integrated and hardened in HEAD |
| `luna/phase-timing` | `fc7cfde` | `CLEAN` | Fold and fence phase start/duration logging fully implemented |
| `luna/fault-repro` | `a32c4a1` | `CLEAN` | Superseded by fixed same-store fault repro in `479d14c` |
| `conor-31test` | `2bb219f`, `d5d036b` | `REJECT` | Standalone file duplication; redundant `consolidatedLogRecorder`; global slog mutation race |
| `conor-32a` | `8947c21` | `REJECT` | Incomplete retry test (watermark no-op) |
| `conor-32b` | `cece0a5` | `CLEAN` | Source mutation & message count assertions already live in `479d14c` |
