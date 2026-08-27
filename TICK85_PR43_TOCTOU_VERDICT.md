# T85 PR #43 TOCTOU verdict

## Initial finding

`internal/index/containers.go:evictStaleRefreshDB` has two correctness defects:

- `BEGIN IMMEDIATE` errors other than SQLite busy/locked authorize deletion,
  so malformed SQLite and I/O failures can destroy a cache.
- The successful probe rolls back and closes before unlinking. A writer can
  enter that gap and its WAL-backed write can be deleted.

The existing PR test is insufficient: its holder acquires `BEGIN IMMEDIATE`
before the probe, so it does not exercise the release-to-unlink gap.

## Evidence

Base: `029f60d77e7e03192bc966de3a835a4a32a00fe2`

PR head: `8d2cb52047ea00d4b123ea747fa5d035d3deff4c`

Current worker before fix: `764eb0d80db23e3c32e346d79184b42d306c27eb`

Current worker after fix: recorded in the final commit receipt.

The PR implementation is the RED baseline (`8d2cb520`): the barrier
reproduction loses the concurrent WAL write. The focused regression is GREEN
after the fix; the unreadable-cache test passes and the fixed eviction path
keeps the transaction through unlinking. The full package race gate is
`UNCERTAIN`: it was interrupted after 177.225s while waiting on pre-existing
`goose.test` / `rawclaw` consolidated-store holders, and is not reported as a
pass.

The deterministic WAL test opens a valid SQLite database, performs the exact
probe (`BEGIN IMMEDIATE`, rollback, close), releases a barrier, lets a second
connection commit a write, then removes the database and reopens it. The write
is absent after reopen. It uses channels, not sleeps or literal stale bytes.

| Probe result | Action | Reason |
| --- | --- | --- |
| `BEGIN IMMEDIATE` succeeds | Remove files while the transaction remains open; then rollback and close | The probe established a writable, valid cache and the lock prevents a writer entering before unlink |
| `SQLITE_BUSY` / `SQLITE_LOCKED` | Retain | Another connection owns the cache |
| `SQLITE_NOTADB` | Retain | The cache could not be safely inspected |
| I/O / other probe error | Retain | Uncertainty must not authorize destructive removal |

Focused commands:

```text
git merge-tree 029f60d77e7e03192bc966de3a835a4a32a00fe2 8d2cb52047ea00d4b123ea747fa5d035d3deff4c
CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'Test(EvictStaleRefreshDB_RetainsUnreadableCache|RefreshCacheProbeReleaseBeforeRemoveLosesWALWrite|RefreshDBPath_PrunesStaleCacheButRetainsFreshAndReused|RefreshDBPath_RetainsInUseStaleSQLite)' -v
CGO_ENABLED=0 go test -race -count=1 ./internal/index
git diff --check
```

Graphify corroboration (canonical project `/Users/jay-m4/code/rawclaw`):

- MCP `get_pr_impact(pr_number=43, repo="MoonCaves/rawclaw", project_path="/Users/jay-m4/code/rawclaw")` returned CI `SUCCESS`, base `main`, 37 nodes across 4 communities, and exactly `internal/index/containers.go` plus `internal/index/containers_test.go`.
- MCP `query_graph(question="cache sqlite close remove lock busy stale database file evict", project_path="/Users/jay-m4/code/rawclaw", depth=2, token_budget=2500)` connected `CacheDir()` and `.Close()` context; source corroboration found `evictStaleRefreshDB` in `internal/index/containers.go`.

## Verdict

PR #43 as submitted: **REJECT / HOLD**. The patch is unsafe on failed probes and
has a close-to-unlink TOCTOU. The worker patch at the final pushed HEAD fixes
both within the original file fence. Merge remains subject to the owning
package race gate being rerun without the unrelated consolidated-store
holders.
