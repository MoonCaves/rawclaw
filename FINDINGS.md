# T56 ExecContext minimal comparator

- `internal/index/consolidated.go`: the base context-aware entry point reaches `consolidateOneContext`, but the fold transaction uses non-contextual `Exec` calls after `BeginTx(ctx, nil)`. Compare that smallest API boundary against Han's full candidate while an independent SQLite writer transaction holds the real database lock.
- `internal/index/consolidated.go`: Han's `consolidatedWriterGate` is process-local admission control. It cannot prove cancellation inside SQLite when the harness leaves the gate unused and holds a real independent writer transaction.

Initial ruling: measure the smallest production-path change that reaches the disputed SQLite first write; do not add synchronization to make the test pass.

Observed base RED: the production call reached the merge phase but stayed blocked 354 ms after cancellation while an independent SQLite writer transaction held the database; publication remained absent until holder release.

Observed full Han `0cd0b9c` RED under the same harness: all candidate `ExecContext`/`QueryRowContext` changes plus `consolidatedWriterGate` still stayed blocked 352 ms at the real SQLite boundary. The gate was unused and therefore cannot explain or solve this result.
