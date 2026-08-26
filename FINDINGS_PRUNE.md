# Hostile review: tombstone prune `386ec9d`

## Verdict

**HOLD.** Correctness tests pass, but the claimed performance improvement is not
merge-ready: the commit has no before/after baseline, and its set-based query
shape is unproven across the small-tombstone case where the previous indexed
probe was already cheap.

## Ranked findings

### P1 — no before/after benchmark, so speed claim is unsupported

`internal/index/consolidated_prune_bench_test.go:10-84` measures only the new
implementation. Six repetitions of the new benchmark are not a comparison.
The fixture is 600 missing plus 200 existing IDs, and resets/seeds outside the
timed region; it cannot establish a win for the common one-to-few existing-ID
case or quantify the setup/transaction cost. Exact reproducer:

```sh
go test ./internal/index -run '^$' -bench BenchmarkPruneTombstonedIDs -benchtime=100ms -count=6
```

Observed on Apple M4 arm64: 15.90–22.70 ms/op. **No point for speed** until
the parent implementation runs the identical fixture for at least six samples
and a benchstat comparison is recorded. Deduction: **-3**.

### P1 — correlated `EXISTS` deletes may discard the indexed-session advantage

`internal/index/consolidated.go:1177-1180,1187-1188` scans each affected table
and evaluates a correlated OR predicate against every tombstone pattern. The
old path did an indexed `sessions(id=?)` probe first and skipped all six
deletes for missing IDs. The new path does one full set-based delete per table,
even when the filtered temp set is small. Exact plan reproducer is to run
`EXPLAIN QUERY PLAN` for any of the six statements after creating
`temp.tombstone_prune_ids`; expected risk is a table scan with a correlated
subquery, not an indexed equality lookup. This is **UNCERTAIN** until measured
against the parent on both 1-existing and 600-missing shapes. Deduction: **-2**.

### P2 — cancellation remains impossible during a large prune

`internal/index/consolidated.go:1147-1201` accepts `*sql.DB` and has no context
or cancellation checks. A large tombstone batch can spend its whole transaction
in inserts and six deletes after the caller's context is cancelled. This is
pre-existing behavior, not introduced by the patch, so it is a contract gap,
not a regression. Deduction: **-1** (advisory).

## Checks performed

- `go test ./internal/index -run 'TestPruneTombstoned' -count=1`: PASS.
- `CGO_ENABLED=0 go test -race ./internal/index -run 'Test(Consolidate|Prune|Tombstone)' -count=1`: PASS.
- `gofmt -l internal/`: empty.
- Wildcard `_` behavior and six-table deletion were exercised by existing tests.
- Temp-table cleanup is explicit at line 1194 and is transaction-scoped; no
  stale rows were reproduced. The defer rollback protects early errors.
- Batch size 400 × 2 parameters stays below the usual SQLite 999-variable
  limit; no schema migration is introduced.

## Rival review

No Furiosa/Han branch exposed an equivalent tombstone-prune implementation in
the inspected worktrees. Their current work concerns detached publication,
cancellation, and harness/reporting rather than this deletion core.
