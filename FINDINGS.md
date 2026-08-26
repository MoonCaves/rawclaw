# Ozzy refresh-cleanup raid

Target: `89c8a28` (`internal/index/containers.go`, `internal/index/containers_test.go`), compared with base `0d60b4c`.

## Findings

- `internal/index/containers.go:93-107`: `isLockedOrActive` performs a short-lived `BEGIN IMMEDIATE; ROLLBACK` probe, then releases the SQLite lock before `removeRefreshDB` runs. A refresh writer can begin after the probe and before the three unlinks, so the claimed concurrency fence is a TOCTOU check, not a writer fence. The existing active-writer test (`89c8a28:internal/index/containers_test.go:710-805`) only covers a lock already held before the probe; the distinct-database worker test (`:807-851`) never races cleanup against a stale target.
- `internal/index/containers.go:110-114`: `removeRefreshDB` deletes `.db`, `-wal`, and `-shm` independently while ignoring all errors. The “single atomic group” comment at lines 57-58 is false: a crash, permission error, or concurrent creator can leave a split database and sidecars.
- `internal/index/containers.go:104-107`: the post-probe freshness check examines only the base `.db`; it does not revalidate the grouped sidecar state or identity before unlinking. A sidecar can become active after grouping and still be removed.

## Ruling

NO TAKEOVER. A shorter deletion-only patch can remove Ozzy’s 61 production lines and 143 test lines, but that reintroduces the already-rejected uncoordinated refresh-cache deletion. Preserving both stale-cache reclamation and failed-publish cache survival requires a lock shared by every refresh writer and the sweeper (or a stronger atomic ownership protocol); `89c8a28` does not provide that seam. No code edit is safe within the requested file fence without first changing that writer contract.

## Evidence

The exact Ozzy focused gate passed 10 shuffled race repetitions in 35.41s:

```text
CGO_ENABLED=0 go test -race -shuffle=on -count=10 -run '^(TestEnsureFreshContainer_PruneStaleLeftovers|TestEnsureFreshContainer_PruneStaleLeftovers_ActiveWriterFenced|TestEnsureFreshContainer_ConcurrentIsolation|TestEnsureFreshContainer_PreservesRefreshDBOnPublishFailure)$' ./internal/index
```

That green result does not cover the probe-to-unlink interleaving described above.
