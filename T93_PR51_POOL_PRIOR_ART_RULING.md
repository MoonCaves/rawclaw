# T93 PR51 database/sql pool prior-art ruling

## Verdict

**PATCH.** PR #51 is based on a real `database/sql` hazard: separate `db.Exec` calls are pool operations and, with an unrestricted `*sql.DB`, `BEGIN IMMEDIATE` and `ROLLBACK` can use different physical connections. `SetMaxOpenConns(1)` makes this probe's two calls same-connection in the absence of other users of that probe pool, and the focused PR tests pass. It is not a database-file-wide serialization guarantee. The smallest explicit correction is to acquire one `*sql.Conn` and issue both statements through `Conn.ExecContext`; retain the lock through unlink, then roll back and close. This removes reliance on pool scheduling and documents the ownership boundary.

The PR's lock-through-unlink intent is materially important: the SQLite lock prevents a cooperating writer from entering between probe and unlink, while unlinking the database and WAL files is still a destructive filesystem operation not governed by `database/sql`. The current test proves one in-process writer case, not every process/filesystem case. Existing prior art marks this sidecar-prune direction technically locked and gives no merge authorization.

## Exact PR identity

- Base: `029f60d77e7e03192bc966de3a835a4a32a00fe2`
- Head: `45259be3166df5d5b6642873d3f96d40b11676bb`
- Merge-base: `029f60d77e7e03192bc966de3a835a4a32a00fe2`
- Stable patch ID (base to head): `b180613fe91906a2963ed07ea2b4f28c0e697150`
- PR: https://github.com/MoonCaves/rawclaw/pull/51

## Mechanism findings

1. `database/sql` `DB.ExecContext` obtains/releases pool connections per call. `DB.SetMaxOpenConns(1)` caps that particular pool, so it is sufficient for this private, uncontended probe but is a global serialization knob for that `*sql.DB`, not for other pools/processes. `DB.Conn(ctx)` returns a dedicated connection and the documentation guarantees queries on that `Conn` use the same database session until `Conn.Close`.
2. `BEGIN IMMEDIATE` is the right SQLite admission primitive for a writer-side eviction fence. The PR correctly refuses eviction when begin fails, and keeps the transaction open during unlink. `ROLLBACK` after unlink is cleanup of the probe transaction; it cannot restore files already unlinked.
3. SQLite WAL is persistent state while connections are open; separating/removing the database from its `-wal`/`-shm` files can lose committed transactions or corrupt the database. All three files must be treated as one eviction unit, as PR #51 does. This is why the operation must remain fail-closed on any lock/admission error.

## Sources (accessed 2026-08-28)

- https://pkg.go.dev/database/sql — “sql package”, current Go documentation, accessed 2026-08-28. `SetMaxOpenConns` limits the maximum number of open connections in one pool; `Conn` states that queries on one Conn run in the same database session and must be returned with `Close`. Directly confirms the pool-scope distinction and the safer pinning API.
- https://sqlite.org/wal.html — “Write-Ahead Logging”, SQLite documentation, accessed 2026-08-28. WAL is part of persistent database state and separating the database from `-wal` may lose committed transactions or corrupt the database; concurrent readers can prevent reset. Supports grouped sidecar handling and conservative eviction.
- https://gitlab.com/cznic/sqlite/-/tree/v1.45.0 — modernc.org/sqlite v1.45.0 implementation, accessed 2026-08-28. Mature pure-Go SQLite driver used by RawClaw; its `database/sql` driver connection is session-bound, so the explicit `*sql.Conn` recommendation is compatible with the actual driver rather than only the standard API.

## Reproduction and gates

- Deterministic source reproduction: the unsafe form is `db.Exec("BEGIN IMMEDIATE")` followed by `db.Exec("ROLLBACK")` with the default unlimited pool; these are independent pool acquisitions. Setting `SetMaxOpenConns(1)` prevents a second physical connection in this private probe. A separate `ConnectRW`/reader pool remains outside that cap, so the cap cannot be claimed as file-global protection.
- PR-focused reproduction run from exact PR head: `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'Test(RefreshDBPath_PrunesStaleCacheButRetainsFreshAndReused|EvictStaleRefreshDB_RetainsActiveWriterAndDeletesAfterCommit)$'` => `ok github.com/MoonCaves/rawclaw/internal/index 2.885s`.
- No product files were edited in this worktree; only this report is owned.

## Graphify receipt

Ran in `/Users/jay-m4/code/rawclaw-khan-graph` before source inspection:

- `graphify reflect --if-stale` => lessons already up to date.
- Literal-token query/explain/path attempted for `sql DB Conn SetMaxOpenConns BeginImmediate`; this installation emitted no result. Graphify was therefore non-authoritative; source, Git, official docs, and the focused gate are the evidence.

## T93 prior-art contract

- Canonical ledger SHA-256 verified: `1e517b5cc1061c40a2a94f1dda04385a72661518a78c7b55bfe71535f988f513`.
- Predecessor verified: commit `98ae85385a328f58640ec262e7c189c1d821083d`; file SHA-256 `dd1f0d4aa2a10bd22123fb6e847b59c887247b73710f02701f3352726847b974`.
- Shared ledger was not modified.

## Proposed append-only delta

Stable ID: `PA-GO-DATABASE-SQL-CONN-PIN-EVICTION-001`

Fingerprint (SHA-256 of the normalized recommendation text): `ebbe6715ee6ada10f56c2d11476f52afc5170d95896cdd0d0ab34356b788b57f`

Recommendation: when a multi-statement SQLite fence must remain on one physical session, use `db.Conn(ctx)` and execute `BEGIN IMMEDIATE`, filesystem operation, and `ROLLBACK` through that Conn; `SetMaxOpenConns(1)` is a valid narrow fallback for a private uncontended pool but does not coordinate other pools or processes. Status: pending, score 0; this report is not external adoption. Direction lock `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically LOCKED; no merge authorization.

Expected ledger SHA-256 after appending the exact stable delta text above (without a timestamp wrapper): `b1eb06b857023cdf064bc0586276ad5b8c9397f2ec19946073995609d25f45ce`. A supervisor adding its required timestamp/wrapper must recompute from those exact bytes.

## Terminal ruling

**PATCH** — accept the root-cause diagnosis as narrowly correct, but require explicit `*sql.Conn` pinning (or equivalent proof that the probe `*sql.DB` is private and uncontended) before shipping. Do not count this worker report or PR branch as prior-art adoption.
