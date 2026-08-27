# T85 PR #43 TOCTOU verdict

## Initial finding

`internal/index/containers.go:evictStaleRefreshDB` has two correctness defects:

- `BEGIN IMMEDIATE` errors other than SQLite busy/locked authorize deletion,
  so malformed SQLite and I/O failures can destroy a cache.
- The successful probe rolls back and closes before unlinking. A writer can
  enter that gap and its WAL-backed write can be deleted.

The existing PR test is insufficient: its holder acquires `BEGIN IMMEDIATE`
before the probe, so it does not exercise the release-to-unlink gap.

## Planned evidence

Add a deterministic SQLite WAL barrier between the probe and removal. A second
connection will attempt a write in that controlled window; no sleeps or stale
bytes are evidence of locking.
