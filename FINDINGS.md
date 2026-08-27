# T56 ExecContext minimal comparator

- `internal/index/consolidated.go`: the base context-aware entry point reaches `consolidateOneContext`, but the fold transaction uses non-contextual `Exec` calls after `BeginTx(ctx, nil)`. Compare that smallest API boundary against Han's full candidate while an independent SQLite writer transaction holds the real database lock.
- `internal/index/consolidated.go`: Han's `consolidatedWriterGate` is process-local admission control. It cannot prove cancellation inside SQLite when the harness leaves the gate unused and holds a real independent writer transaction.

Initial ruling: measure the smallest production-path change that reaches the disputed SQLite first write; do not add synchronization to make the test pass.

Observed base RED: the production call reached the merge phase but stayed blocked 354 ms after cancellation while an independent SQLite writer transaction held the database; publication remained absent until holder release.

Observed full Han `0cd0b9c` RED under the same harness: all candidate `ExecContext`/`QueryRowContext` changes plus `consolidatedWriterGate` still stayed blocked 352 ms at the real SQLite boundary. The gate was unused and therefore cannot explain or solve this result.

Observed gate-deleted/full-context RED: removing the candidate global gate and both admission calls changed no behavior; the same production call stayed blocked 351 ms. Patch ID: `1e37a069462cc5715a6720e21be360fa233302cd`.

Observed minimal transaction-only `tx.ExecContext(ctx, ...)` RED: 17 replacements (17 insertions, 17 deletions), patch ID `12cebe4d80999da9fe420d2e70b6918c38b98adb`; the call still stayed blocked 352 ms. Broadening to attach/query context APIs did not alter the boundary.

The real first blocked operation is therefore below Go's context-aware `database/sql` call boundary for this modernc SQLite busy wait. The context on `BeginTx` still causes eventual rollback/unwind after release, but does not provide bounded cancellation while the writer lock is held.
