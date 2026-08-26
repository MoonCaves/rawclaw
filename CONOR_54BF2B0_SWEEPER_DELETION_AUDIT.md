# Audit: Conor `54bf2b0` refresh-container sweeper deletion

## Verdict

**HOLD — do not adopt unconditionally.**

This candidate is a safety retreat from Ozzy `89c8a28`: removing the unlocked probe-to-unlink path avoids the confirmed possibility of deleting a live refresh database. It is not a concurrency-safe replacement, however. The deletion also removes the only bounded cleanup for abandoned refresh databases and their `-wal`/`-shm` sidecars, so repeated failed or superseded refreshes can grow the private cache without bound.

The observed risk is cache leakage, not proven transcript data loss. Adoption needs either a complete writer fence covering generation selection through the grouped DB/WAL/SHM unlink, or an explicit contract change accepting retained cache growth and documenting how it is reclaimed.

## Artifact and patch identity

- Candidate: `54bf2b03d3b32bf639924ff0a1f8f6885772eb81`
- Parent: `d7c22ba9b5bf9b41eb8b473bd1e48227e4fe6c7`
- Stable patch-id: `2a97a7d582b8591e1c87dc23a3b7651392edbb69`
- Candidate branch: `luna/conor-pr35-containers-audit-20260826`
- Candidate worktree: `/Users/jay-m4/code/rawclaw-luna-conor-pr35-containers`
- Diff scope: `internal/index/containers.go`, `internal/index/containers_test.go`
- Diff size: production `-42` lines, tests `-119` lines, docs `0` lines

## Contract removed

On `origin/main`, `internal/index/containers.go:42-71` defines `pruneStaleRefreshDBs`, and `PrepareFreshContainer` invokes it at line 86. The associated contract test in `internal/index/containers_test.go:591-708` covers stale leftovers, fresh-WAL preservation of a mixed-age database, and failed-publish preservation of the refresh database.

`54bf2b0` deletes `refreshStaleAfter`, `pruneStaleRefreshDBs`, the cleanup call, and this entire stale-pruning test, including its active-writer case.

## Comparison with Ozzy `89c8a28`

Ozzy's `89c8a284d20e4f6adba72accb3c0b34831a3b422` (patch-id `7e3d7d4981316e4cfb2b11f6a8eacbb71b314bdc`) adds `isLockedOrActive` at `internal/index/containers.go:93-108`. It performs `BEGIN IMMEDIATE; ROLLBACK`, closes the database, and then independently calls `removeRefreshDB` at `:110-113`. The lock probe and subsequent unlink are separate operations: a writer can begin after the probe and before unlink. That is a confirmed probe-to-unlink TOCTOU deletion path.

`54bf2b0` removes that path by removing the sweeper. It should therefore be preferred over `89c8a28` on live-state integrity, but it must not be described as a writer-safe sweeper fix. A later safer direction (`aae80a4`) groups the database and sidecar removal under a complete fence.

## Verification

Personally run on the candidate worktree:

```sh
/usr/bin/time -p env CGO_ENABLED=0 go test -race -count=3 -shuffle=on ./internal/index -run 'TestEnsureFreshContainer|TestPrepareFreshContainer' -v
```

Result: **PASS**, package time `5.899s`, wall time `7.51s`. Normal consolidate phase logs were emitted. This gate does not exercise stale cleanup because the candidate deletes that test.

The worker reported `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...` passing in `131.983s`; that result was not independently rerun and is recorded as worker-reported evidence only.

## Required disposition

Keep this candidate on **HOLD** until either a replacement sweeper uses one complete writer fence for generation choice and grouped DB/WAL/SHM unlink, with tests racing direct tag/vector writes and replacement, or the product contract explicitly accepts abandoned refresh-cache growth and supplies a bounded, separately safe reclamation mechanism.

