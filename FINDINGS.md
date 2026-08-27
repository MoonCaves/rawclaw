# T56 ExecContext minimal comparator

- `internal/index/consolidated.go`: the base context-aware entry point reaches `consolidateOneContext`, but the fold transaction uses non-contextual `Exec` calls after `BeginTx(ctx, nil)`. Compare that smallest API boundary against Han's full candidate while an independent SQLite writer transaction holds the real database lock.
- `internal/index/consolidated.go`: Han's `consolidatedWriterGate` is process-local admission control. It cannot prove cancellation inside SQLite when the harness leaves the gate unused and holds a real independent writer transaction.

Initial ruling: measure the smallest production-path change that reaches the disputed SQLite first write; do not add synchronization to make the test pass.
