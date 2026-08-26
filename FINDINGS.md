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

## Candidate 4: `remotes/candidates/conor-31test`
- **Rival SHA**: `2bb219f8aeb412dbf9add6fe691cf606ad8805f1`
- **Locations**:
  - `internal/index/consolidated_logging_test.go:1-145`
- **Hostile Review & Reproducible Evidence**:
  - **Flaw 1 (Global Slog Race)**: `conor-31test` mutated `slog.SetDefault()` inside parallel/unfenced tests (`TestConsolidate_LogsFoldPhaseStartsAndDurations` and `TestConsolidatedFence_LogsAcquireDurationOnTimeout`), introducing race conditions when other concurrent tests or goroutines log.
  - **Flaw 2 (Duplicate Infrastructure)**: Defined a redundant `consolidatedLogRecorder` type rather than using the canonical `testLogRecorder` in `internal/index/testhelpers_test.go`.
  - **Flaw 3 (False Confidence on Failed Acquisition)**: Conor claimed to test timeout duration logging in a separate file, but deferred timing in `AcquireConsolidatedFence` runs unconditionally on any exit path.
  - In current `HEAD`, `internal/index/testhelpers_test.go` provides `testLogRecorder`, which `TestConsolidate_LogsPhaseStartsAndDurations` uses safely in `internal/index/consolidated_test.go:19-82` to assert all 9 fold phases plus fence acquire/release.
- **Ruling**: `REJECT`
  - Reject duplicate file and struct. Any missing edge assertion (such as logging during timeout) belongs in canonical test files using existing test infrastructure.

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
| `luna/phase-timing-20260826` | `fc7cfde` | `CLEAN` | Fold and fence phase start/duration logging fully implemented |
| `luna/fault-repro-20260826` | `a32c4a1` | `CLEAN` | Superseded by fixed same-store fault repro in `479d14c` |
| `conor-31test` | `2bb219f` | `REJECT` | Global slog mutation race; duplicate recorder struct; redundant file |
| `conor-32a` | `8947c21` | `REJECT` | Incomplete retry test (watermark no-op) |
| `conor-32b` | `cece0a5` | `CLEAN` | Source mutation & message count assertions already live in `479d14c` |
