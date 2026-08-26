# Issue #32 fault-reproduction findings

## Rulings

- `internal/index/consolidated.go`: **restore-exactly**. Keep the existing
  test-only `consolidateAfterMergeHook` and its placement in the merge defer:
  it runs after the transaction commit has returned and before the earlier
  `DETACH` defer, which models `os.Exit` skipping detach and connection-close
  cleanup without adding a production behavior.
- `internal/index/consolidated_test.go`: **accepted-deviation**. Strengthen
  the child-process test with assertions that the committed session and sync
  watermark are visible after the child exits, that the consolidated WAL/SHM
  artifacts are recorded when present, and that the retry actually changes the
  store. This is the smallest load-bearing proof that the retry is against the
  same post-commit store rather than merely a second successful no-op.
- New production globals or a wall-clock signal race: **shrink**. The existing
  hook plus the test binary's child process is sufficient and deterministic.
- A separate helper-process implementation: **replace** with reuse of the
  existing `TestConsolidate_FaultInjectionHelper`; no equivalent simpler seam
  exists in this package.

## Baseline observation

The current test proves only exit status, merge logging, absence of a detach log,
and successful retry. It does not inspect WAL/SHM/lock state, prove the merge
transaction committed before exit, or prove the retry performed work. Repeated
race-enabled runs are required after the assertion changes; the commit body
will record their observed retry timings and whether a multi-second stall
reproduced.
