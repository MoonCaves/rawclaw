# Review of 5c50c7c

Base: c14e806. Competitor parent: 23b5863f68353e7e9c50d2e80fe2b492691de31a. The owned files in that parent match this branch for the reviewed hunks.

## Rulings

- `internal/index/consolidated.go`: **transplant-exactly**. The adjacent `if rebuild` blocks have no intervening statement or changed scope. Moving `prevLive` and replacement-store setup into the tag-preservation block keeps the same early `readTagState` error, assignment order, cleanup defer registration, #31 phase instrumentation, writer fence, and atomic rename behavior. Net: -2 lines.
- `internal/index/containers.go`: **transplant-exactly**. `vaultContainer` and `vaultContainerAll` construct the same `durable.Meta` and backing path, and both call `backingFileState` with the same best-effort error semantics. The helper returns the path only to the append path that needs `StoreFile`; it does not cross the vault/transaction lifecycle boundary. Callers retain their existing source/origin gates and error wrapping. Net: -11 lines.

No hunk is rejected. No test-only change is required: existing tests cover rebuild failure preservation, carry-forward/tombstone behavior, container vaulting, append transactions, source metadata, and current phase behavior. Focused negative gates will be run after the transplant.

## Verdict

Safe and net-negative to transplant. Claimed source reduction is -13 lines (8 additions, 19 deletions), despite the competitor description saying net -11.
