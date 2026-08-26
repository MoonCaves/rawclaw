# Code mechanisms for consolidated rebuild, refresh ownership, and tombstone pruning

Scope: upstream source mechanisms checked at RawClaw `5b9756b2200ff6bd670f07407407d84d9f42d84b`. These are implementation precedents, not claims that RawClaw should adopt an upstream dependency.

## 1. Install a complete replacement generation, then publish it with one directory rename

- Upstream: `rqlite/rqlite`, commit [`de423f3adf08f6929325d12767035c7962cda64f`](https://github.com/rqlite/rqlite/blob/de423f3adf08f6929325d12767035c7962cda64f/snapshot/sink.go#L190-L251), `snapshot.Sink.Close`.
- Mechanism: finish the DB/WAL files in `<snapshot>.tmp`; move any local WAL directory into that private tree; write metadata; sync the temporary directory; `os.Rename(tmp, final)`; sync the parent directory. Failure removes only the temporary tree (and incremental failures are treated as fatal because the WAL chain would otherwise be broken).
- RawClaw mapping: `internal/index/consolidated.go:402-550`, `ConsolidateFrom(rebuild=true)`, currently uses `<consolidated.db>.rebuild` as a sibling file but renames only the DB and cleans sidecars afterward.
- Verdict: **ADAPT**. Keep the live generation untouched until replacement DB, WAL, SHM, schema, tags, provenance, and validation are complete. Prefer a private generation directory if sidecar grouping becomes difficult; otherwise retain the sibling basename and publish under the fence.
- Semantic trap: renaming only the main DB while an old `-wal` or `-shm` remains can attach state belonging to the old inode; a directory rename groups the generation. Cross-filesystem temp paths lose atomicity.
- Gate: hold `AcquireConsolidatedFence`; seed live data; block/inject a direct tag or vector writer after the rebuild snapshot; force a rebuild failure and assert the original DB plus its WAL/SHM remain readable; on success assert the post-snapshot write is present after reopening the published generation.

## 2. Use a crash-replayable, idempotent mutation plan for cleanup and checkpoint steps

- Upstream: `rqlite/rqlite`, same commit, [`snapshot/store.go`](https://github.com/rqlite/rqlite/blob/de423f3adf08f6929325d12767035c7962cda64f/snapshot/store.go#L576-L674), `Store.reap`, and [`snapshot/plan/executor.go`](https://github.com/rqlite/rqlite/blob/de423f3adf08f6929325d12767035c7962cda64f/snapshot/plan/executor.go#L27-L108), `Executor.Rename`/`Checkpoint`.
- Mechanism: serialize rename/remove/checkpoint operations to a plan file before mutation; operations are idempotent (`src` missing plus `dst` present is success); a leftover destination WAL is checkpointed first; the plan is removed only after execution and parent-directory sync.
- RawClaw mapping: rebuild cleanup in `ConsolidateFrom`, plus `internal/index/containers.go` refresh cleanup and any future `pruneStaleRefreshDBs` generation sweep.
- Verdict: **ADAPT selectively**. A small marker/manifest can make interrupted `.rebuild` cleanup retryable. Do not import rqlite’s snapshot plan subsystem.
- Semantic trap: a plan containing paths that are later renamed can be unsafe to replay unless each operation has an explicit “already done” check; deleting a live path on ambiguous state is not idempotence.
- Gate: kill a helper at each phase (after DB close, after WAL checkpoint, after rename, before cleanup), restart cleanup, and assert no live generation is deleted and the retry converges without duplicate data.

## 3. Close all handles before generation rename and sync the parent directory

- Upstream: `rqlite/rqlite`, commit above, [`snapshot/upgrader.go`](https://github.com/rqlite/rqlite/blob/de423f3adf08f6929325d12767035c7962cda64f/snapshot/upgrader.go#L104-L171), `Upgrade7To8`.
- Mechanism: write the upgraded SQLite file under a new temporary directory; explicitly ensure file handles are closed before rename/remove; rename the complete directory; `SyncDirParentMaybe(new)`; only then remove the old generation.
- RawClaw mapping: the `ConsolidateFrom` defer around `con.Close` and its rebuild swap in `internal/index/consolidated.go`; refresh DB ownership in `EnsureFreshContainer`.
- Verdict: **COPY the ordering, ADAPT the names**.
- Semantic trap: a deferred close registered in the wrong order can leave SQLite handles open when sidecars are removed or renamed. Parent-directory sync is durability evidence, not a substitute for the writer fence.
- Gate: inject a failure before `con.Close`, after close but before rename, and after rename; inspect open descriptors and reopen the generation from a fresh process.

## 4. Lock the SQLite writer boundary using SQLite’s own pending/shared byte range

- Upstream: `benbjohnson/litestream`, commit [`63225f17ccbb8dedfb26d03f7d3d07e74c6cf69f`](https://github.com/benbjohnson/litestream/blob/63225f17ccbb8dedfb26d03f7d3d07e74c6cf69f/internal/lock_unix.go#L11-L50), `LockFileExclusive`/`UnlockFile`.
- Mechanism: acquire an fcntl write lock at SQLite’s pending byte, then the 510-byte shared-lock range; if the second range fails, release the first. Unlock reverses both ranges.
- RawClaw mapping: `internal/index/consolidated_fence.go:35-87`, currently a separate `consolidated.lock` flock; direct writers include `SyncConsolidatedFrom`, `internal/archive/tagapply.go`, `internal/cli/cmd_tag.go` tag-floor, and `internal/cli/cli.go` vector top-up.
- Verdict: **REJECT as the primary RawClaw mechanism; ADAPT as an optional diagnostic/native lock**. RawClaw’s existing named fence is simpler and portable across all direct writers; SQLite byte locks alone do not fence the separate rebuild rename protocol.
- Semantic trap: `database/sql` pool limits (`SetMaxOpenConns(1)`) do not coordinate another `*sql.DB`, process, or file rename. A SQLite byte lock also does not automatically make non-SQLite filesystem operations participate in the same critical section.
- Gate: run real `tag-write`, `tag-floor`, and vector upsert concurrently with rebuild; assert no post-snapshot row disappears and no stale sidecar is published. Run with two processes, not only goroutines.

## 5. Keep WAL alive and separate checkpoint ownership from WAL readers

- Upstream: `benbjohnson/litestream`, commit above, [`db.go`](https://github.com/benbjohnson/litestream/blob/63225f17ccbb8dedfb26d03f7d3d07e74c6cf69f/db.go#L1046-L1099), `DB.Open`; and [`db.go`](https://github.com/benbjohnson/litestream/blob/63225f17ccbb8dedfb26d03f7d3d07e74c6cf69f/db.go#L1996-L2035), `DB.sync`.
- Mechanism: open with a busy timeout and `wal_autocheckpoint(0)`; set `PERSIST_WAL`; hold a long-running read transaction to prevent other checkpoints; use a dedicated `chkMu` read lock around WAL reads so checkpointing cannot race the WAL snapshot.
- RawClaw mapping: `internal/store/store.go:ConnectRW/ConnectRO`, `internal/index/containers.go` private refresh DB generation, and `ConsolidateFrom`/`SyncConsolidatedFrom` sidecar lifecycle.
- Verdict: **ADAPT the ownership rules, REJECT the replication-specific tables** (`_litestream_seq`, `_litestream_lock`). Keep genuine `-wal`/`-shm` with the DB generation and make cleanup prove no active writer owns that generation.
- Semantic trap: `-shm` is shared-memory coordination, not durable content; deleting it while a connection is open can disrupt active readers/writers. `TRUNCATE` checkpointing is blocking; passive checkpointing can return `SQLITE_BUSY` and must not be reported as fresh success.
- Gate: create a real WAL database, keep an active reader and writer open, inspect nonzero DB/WAL/SHM files, run refresh cleanup concurrently, and assert the active generation remains usable; separately inject `SQLITE_BUSY` during checkpoint and preserve stale/error status.

## 6. Use a private generation directory to group DB, WAL, SHM, and metadata

- Upstream: `rqlite/rqlite`, commit above, [`snapshot/sink.go`](https://github.com/rqlite/rqlite/blob/de423f3adf08f6929325d12767035c7962cda64f/snapshot/sink.go#L214-L251), `Sink.Close`, plus [`snapshot/upgrader.go`](https://github.com/rqlite/rqlite/blob/de423f3adf08f6929325d12767035c7962cda64f/snapshot/upgrader.go#L34-L51), interrupted-temp cleanup.
- Mechanism: each generation owns all files under one temporary/final directory; stale temp directories are removed on the next attempt; publication is one directory rename after sync.
- RawClaw mapping: `RefreshDBPath` and `EnsureFreshContainer` in `internal/index/containers.go`; current refresh cleanup must not unlink a DB while its `-wal`/`-shm` or writer is active.
- Verdict: **ADAPT for refresh caches**. Use generation IDs and an active marker/lock; publish a new directory, then reap only generations that are both inactive and older than the retention threshold.
- Semantic trap: mtime/age alone cannot prove inactivity. A long-lived reader may keep a generation live after its files look old; a writer can recreate sidecars after a cleanup scan.
- Gate: open generation N with an active reader and writer, publish N+1, run cleanup repeatedly, and assert N survives until both handles close; then assert N is eventually removed as a unit (DB, WAL, SHM, metadata).

## 7. Materialize large membership sets as SQLite data, not host-variable lists

- Upstream: SQLite, commit [`86a15af2b148c655bfd352162d04c6756399f4ea`](https://github.com/sqlite/sqlite/blob/86a15af2b148c655bfd352162d04c6756399f4ea/src/expr.c#L3230-L3263), `sqlite3FindInIndex`; [`sqlite3CodeRhsOfIN`](https://github.com/sqlite/sqlite/blob/86a15af2b148c655bfd352162d04c6756399f4ea/src/expr.c#L3631-L3760) builds/reuses an ephemeral B-tree for `x IN (SELECT ...)`.
- Mechanism: SQLite’s planner treats a subquery RHS as a set and can materialize it into an ephemeral table for membership tests; the RHS is not a giant host SQL string. Feed IDs through a temp table (batched inserts if needed), then use `DELETE ... WHERE EXISTS (...)` or an anti-join.
- RawClaw mapping: `internal/index/consolidated.go:1147`, `pruneTombstonedIDs`; current code loops IDs and executes six deletes per ID, which is O(ids × tables) and never exercises the large-set path.
- Verdict: **ADAPT**. Create a transaction-local `TEMP TABLE tombstone_ids(id TEXT PRIMARY KEY)`, insert IDs in bounded batches, then delete descendants with joins/`EXISTS` across messages, sessions, provenance, file index, and sidecars.
- Semantic trap: a host-built `IN (?, ?, ...)` hits SQLite’s variable limit; `NOT IN` has three-valued NULL semantics and is not a safe anti-join replacement. Temp-table lifetime is connection/transaction scoped, so use the same `*sql.Tx`.
- Gate: 3,000+ IDs (above the configured SQLite variable limit), verify all six tables and descendants are deleted, verify an unrelated ID survives, run `EXPLAIN QUERY PLAN`, and assert no “too many SQL variables” error.

## 8. SQLite’s multirow DELETE collects keys before mutating the table

- Upstream: SQLite, same commit, [`src/delete.c`](https://github.com/sqlite/sqlite/blob/86a15af2b148c655bfd352162d04c6756399f4ea/src/delete.c#L496-L532), `sqlite3DeleteFrom`.
- Mechanism: for complex multirow deletes, SQLite opens an ephemeral table/RowSet for rowids or primary keys, scans the predicate, then performs deletion from the collected stable key set. This prevents mutation from invalidating the scan.
- RawClaw mapping: descendant pruning in `pruneTombstonedIDs`, especially if replacing the per-ID loop with one statement that deletes sessions and dependent rows.
- Verdict: **COPY the semantic shape, ADAPT through SQL**: materialize the tombstone set first, then delete dependents before parent rows inside one transaction.
- Semantic trap: deleting parent rows first can make later descendant/provenance joins lose the key needed to clean child rows; foreign-key cascades are not guaranteed for this schema and should not be assumed.
- Gate: seed parent, nested descendant, messages, file index, topic segment, verdict, and provenance rows; prune in one transaction; assert zero rows remain in every dependent table after commit and all remain after rollback injection.

## 9. Escape literal session IDs before descendant `LIKE` matching

- Upstream: SQLite, commit above, [`test/like.test`](https://github.com/sqlite/sqlite/blob/86a15af2b148c655bfd352162d04c6756399f4ea/test/like.test#L642-L669) covers the three-argument LIKE/ESCAPE behavior; SQLite’s `LIKE` implementation accepts an explicit escape character.
- Mechanism: represent a literal prefix as `escapeLike(id) + '/%'` and issue `LIKE ? ESCAPE '\\'`; escape `\\`, `_`, and `%` before binding.
- RawClaw mapping: `internal/index/consolidated.go:1125-1130,1167-1179`, `escapeLike` and session/message deletion.
- Verdict: **COPY exactly**. This is the right defense for subagent descendants and IDs containing wildcard characters.
- Semantic trap: `id || '/%'` without escaping treats `_` and `%` inside an ID as wildcards and can delete sibling sessions. `NOT LIKE` or a prefix range is not equivalent when IDs contain arbitrary punctuation.
- Gate: tombstone IDs containing `_`, `%`, and `\\`; seed sibling IDs plus descendants; assert only exact parent and `parent/...` descendants are removed. Include `LIKE` escaping in the large temp-table pruning test, not only a one-ID unit test.

## Strongest directives for RawClaw

1. Make the rebuild publish a complete private generation under the existing consolidated fence; race real tag, floor, and vector writers against the snapshot/swap and prove no post-snapshot write is lost.
2. Treat DB/WAL/SHM as one refresh generation with active-writer protection; age-based unlinking is insufficient, and `SQLITE_BUSY` must remain stale/error rather than fresh.
3. Replace the per-ID tombstone loop with a transaction-local indexed temp table plus `EXISTS`/anti-join deletes; test beyond SQLite’s variable limit and retain the escaped descendant predicate.

