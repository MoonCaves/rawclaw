# T90 PR43 independent mutation referee

Date: 2026-08-28 WITA  
Scope: PR #43 submitted `8d2cb52047ea00d4b123ea747fa5d035d3deff4c`; Ellen branch `/Users/jay-m4/code/rawclaw-furiosa-t85-pr43-toctou` at `2f1992ac21e26bb1398824b4488a9e5605881aab`. Report-only; no product or test edits in this worktree.

## Verdict

**HOLD / REJECT as a proven fix.** Ellen’s code change correctly makes failed SQLite probes non-destructive and keeps the successful `BEGIN IMMEDIATE` transaction open through unlink. That is a plausible lock barrier, but the submitted regression does not exercise `evictStaleRefreshDB`; it manually performs the unsafe sequence and manually unlinks the files. The key close-to-unlink claim therefore remains unproven. Keeping an SQLite transaction open while unlinking the database and WAL/SHM sidecars also needs an explicit platform/lifecycle safety proof.

## Patch identity and ancestry

- PR43 submitted head `8d2cb520` has parent `029f60d`, merge-base `029f60d`, and stable patch ID `686367ff7cd075f9db1762c7f8ae7772216723bd`.
- Ellen’s code baseline is `764eb0d` (same implementation patch ID `686367ff7cd075f9db1762c7f8ae7772216723bd`).
- Ellen’s fix commit is `3de6a4a`, parent `764eb0d`; its code/test delta is `+6/-4` production and `+77/-8` tests. Code/test-only stable patch ID: `a74816bb088473a827440c2a29268bde02b6d67f`.
- Branch head `2f1992ac` adds the final report; it is not the code fix commit. The branch’s merge-base with PR43 is `029f60d`.
- `git merge-tree --write-tree 029f60d 3de6a4a` produced a tree with no conflict output (`dd59398fda2a3b7aaf791fc4b72a3eedbf768f96`).

## Mechanism review

### 1. Valid-SQLite entrant barrier — HOLD

Ellen changes `evictStaleRefreshDB` to:

1. `BEGIN IMMEDIATE` with `busy_timeout(0)`.
2. On success, unlink `.db`, `-wal`, and `-shm` while the transaction remains open.
3. Roll back and close afterward.

This removes PR43’s explicit rollback/close-to-unlink gap if SQLite’s held write transaction is the intended entrant barrier. However, the purported deterministic test, `TestRefreshCacheProbeReleaseBeforeRemoveLosesWALWrite`, does not call `evictStaleRefreshDB`. It performs `BEGIN IMMEDIATE`, rollback, close, waits for a writer to commit, then directly calls `removeRefreshDBFiles`. That test demonstrates the old unsafe sequence, but it passes under both the fixed and mutated implementations.

Mutation evidence: in a disposable worktree, I reverted Ellen’s guard and ordering to PR43. The WAL-window test still passed. Therefore it is mutation-insensitive for the close-to-unlink fix and cannot establish that Ellen’s implementation closes the race.

### 2. Non-busy error classification — ACCEPT, narrowly

Ellen removes the `if !isBusy(err) { removeRefreshDBFiles(...) }` branch. Any failed `BEGIN IMMEDIATE`, including `SQLITE_NOTADB` and I/O-like probe errors, now retains the cache. `sql.Open` is lazy and is not a validity proof, so retaining on every failed probe is the safer classification.

The focused `TestEvictStaleRefreshDB_RetainsUnreadableCache` passes on Ellen’s branch. Under the disposable mutation reverting the guard, it fails because the invalid cache is deleted. This test therefore detects the non-busy deletion mutation.

### 3. Existing stale/fresh and busy gates — ACCEPT for their narrow claims

Observed on Ellen’s branch with `CGO_ENABLED=0 go test -race -count=1 ./internal/index` and the focused regex:

- `TestRefreshDBPath_PrunesStaleCacheButRetainsFreshAndReused`: PASS.
- `TestRefreshDBPath_RetainsInUseStaleSQLite`: PASS.
- `TestEvictStaleRefreshDB_RetainsUnreadableCache`: PASS.
- `TestRefreshCacheProbeReleaseBeforeRemoveLosesWALWrite`: PASS, but not a test of the eviction function.

The full package race gate was not run in this referee. No green focused result is treated as proof of the missing interleaving.

## Final disposition

- Non-busy error deletion: **ACCEPT Ellen’s minimal narrowing**.
- Valid-SQLite WAL entrant barrier: **HOLD**; the test must invoke `evictStaleRefreshDB` and synchronize a writer entrant against the actual eviction transaction.
- “Closes close-to-unlink gap without unsafe deletion”: **REJECT as established**. The implementation may be directionally correct, but the current test does not prove it and unlinking live SQLite files while a handle remains open requires explicit safety evidence.
- Ellen branch as a merge-ready PR43 fix: **HOLD** pending a real mutation-sensitive eviction test and a complete allowed race gate.
