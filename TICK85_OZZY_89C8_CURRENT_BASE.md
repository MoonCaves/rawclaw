# T85 Ozzy 89c8a284 current-base evidence report

Date: 2026-08-28 WITA  
Base under review: `029f60d77e7e03192bc966de3a835a4a32a00fe2`  
Candidates: Ozzy `89c8a284d20e4f6adba72accb3c0b34831a3b422`, PR #42 `453ab9b7fefa2cfbc0700f45671f2607578c6db3`, PR #43 `8d2cb52047ea00d4b123ea747fa5d035d3deff4c`

## Verdict

The smallest transplantable winner is PR #42/PR #43's shared eviction ordering, but neither is a complete concurrency proof. PR #43 is already the current-base-equivalent candidate and is the smallest clean transplant. Accept its TTL/grouping behavior and close-before-unlink improvement; HOLD the safety mechanism because a valid SQLite probe still ends before unlink and no interleaving test falsifies that race. Reject Ozzy as a current-base transplant.

| Mechanism | Ozzy 89c8a284 | PR #42 453ab9b | PR #43 8d2cb52 |
|---|---|---|---|
| TTL | `refreshStaleAfter = 24h` (`containers.go:29-30`) | `refreshCacheStaleAfter = 30d` (`containers.go:28`) | same 30d |
| Trigger | `RefreshDBPath` does not prune; `EnsureFreshContainer` calls global prune (`containers.go:50-90`, call at 165) | `RefreshDBPath` prunes immediately, skips requested path (`containers.go:33-65`) | same |
| Freshness | Groups `.db`, `-wal`, `-shm`; newest mtime wins | Groups by each `.db`, stats sidecars; newest mtime wins | same |
| Deletion | Probe helper defers close, then caller unlinks all three files | Closes after probe/rollback, then unlinks all three files | same |
| Non-busy probe error | Retains only `sql.Open` failure or busy; a non-busy `BEGIN` error falls through to deletion | Explicitly deletes on non-busy `BEGIN` error | same |
| Current-base merge | `CONFLICT` in `containers.go` plus unrelated files | clean merge-tree | clean merge-tree |
| Current-base patch apply | fails in `containers.go` and tests | `git apply --check`: clean | `git apply --check`: clean |

## Identity and ancestry

True merge bases with current base:

- Ozzy: `86c5ce06b789e9e287154ba25acc699d04ff2c7b`
- PR #42: `9fd82d3bf6ba0ce1027cdf84cec51efe3ba87b5c`
- PR #43: `029f60d77e7e03192bc966de3a835a4a32a00fe2` (current base itself)

Stable patch IDs are distinct:

- Ozzy: `7e3d7d4981316e4cfb2b11f6a8eacbb71b314bdc`
- PR #42: `1a4fe5eb1fdc10ed86cdc6931ce298d1e8a776e0`
- PR #43: `686367ff7cd075f9db1762c7f8ae7772216723bd`

Current-base net changes (`git diff 029f60d..candidate`, limited to the owned implementation/test files):

- Ozzy: production `+90/-8`, tests `+237/-32`; this includes its divergent ancestor and does not represent a narrow transplant.
- PR #42: production `+56/-1`, tests `+54/-0`.
- PR #43: production `+56/-1`, tests `+97/-0`.

The candidate commit-local stats are also distinct: Ozzy `+46/-15` production and `+143/-0` tests; PR #42 `+17/-14` production and no test-file change; PR #43 `+56/-1` production and `+97/-0` tests.

## Safety findings

1. **Stale grouping / sidecars — ACCEPT for PR #42/#43; HOLD for Ozzy.** PR #42/#43 prune from `RefreshDBPath`, preserve the requested path, use a 30-day TTL, and treat the newest of the database and sidecars as freshness. Ozzy's 24-hour policy is materially more aggressive and its prune call is on `EnsureFreshContainer`, not path acquisition. The Ozzy grouping is otherwise directionally correct.

2. **Busy/in-use fencing — ACCEPT as a narrower guard, not as a race proof.** All candidates use `BEGIN IMMEDIATE` with `busy_timeout(0)` and retain a database when the probe reports busy. Focused tests observed:

   - Ozzy `TestEnsureFreshContainer_PruneStaleLeftovers_ActiveWriterFenced`: PASS under `CGO_ENABLED=0 go test -race -count=1`.
   - PR #42 `TestRefreshDBPath_PrunesStaleCacheButRetainsFreshAndReused`: PASS.
   - PR #43 `TestRefreshDBPath_PrunesStaleCacheButRetainsFreshAndReused`: PASS.
   - PR #43 `TestRefreshDBPath_RetainsInUseStaleSQLite`: PASS.

   These tests cover an already-held lock and ordinary stale/fresh files. They do not synchronize a writer between probe completion and unlink.

3. **Valid-SQLite post-probe close-to-unlink race — REJECT as closed / HOLD as unresolved.** Ozzy's `isLockedOrActive` returns and its deferred `db.Close()` runs before `removeRefreshDB` (`89c8a284:containers.go:93-113`), so a writer can acquire the database after the probe and before deletion. PR #42/#43 improve ordering by executing `ROLLBACK`, closing the handle, and then unlinking (`containers.go:69-89`), but that creates the same logical gap explicitly: `Close()` and `removeRefreshDBFiles()` are separate operations. None atomically coordinates the probe and unlink with a writer. No candidate closes this race.

4. **Non-busy error deletion — REJECT as narrowed policy.** PR #42/#43 delete when `BEGIN IMMEDIATE` returns any non-busy error after `sql.Open` succeeds (`containers.go:74-79`). A stale invalid SQLite file therefore gets deleted; the PR #43 stale plain-file test passes because of this behavior. Ozzy also deletes after a non-busy probe error by falling through to `false`. The newer code makes the branch explicit but does not narrow deletion to a validated SQLite state. `sql.Open` itself is lazy, so it is not a validity proof.

## Evidence boundary

Green focused tests establish the stated retention behavior only. There is no candidate test that controls the probe/close/unlink interleaving, and no mutation run was claimed as empirical evidence. Therefore the concurrency verdict is `HOLD`, not a safety pass.

## Final disposition

- Ozzy `89c8a284`: **REJECT** for current-base transplant: 24-hour TTL, later prune trigger, broader divergent diff, and probe-to-unlink TOCTOU.
- PR #42 `453ab9b`: **HOLD**: cleanly applies and improves close-before-unlink, but does not close the post-probe race or narrow non-busy deletion.
- PR #43 `8d2cb52`: **HOLD / smallest winner**: current-base-equivalent, clean apply/merge-tree, 30-day grouped TTL and focused retention tests; same unresolved race and non-busy deletion policy.
