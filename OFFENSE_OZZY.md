# Hostile evidence audit: Ozzy active heads

Date: 2026-08-26

## Verdict

`CONFIRMED DEDUCTION`: `89c8a284d20e` adds a cleanup race. It probes a stale
SQLite database, ends its transaction, and only then unlinks the database and
sidecars. The probe does not transfer ownership of the path to the remover.

`NARROWED`: the active cleanup tests prove only the easy interleavings. They
prove that a lock held for the whole probe is preserved and that an unlocked
stale file is removed. They do not exercise a reader/writer opening the file
between the probe's `ROLLBACK` and `removeRefreshDB`.

`NO DEDUCTION`: the hostile hook report's unwritable-catalog ingest loss is not
a current-base defect. The assigned base contains the fail-soft repair, and
the current hook regression tests pass. The mixed-source global ambiguity issue
reported against `cdc063d` is likewise in a commit already contained by base;
the report's claim is historical evidence, not an active Ozzy production patch.

## Immutable patch and ancestry receipts

Assigned base:

```text
5b9756b2200ff6bd670f07407407d84d9f42d84b
```

The catalog/hidden/prune/repro head is already in base:

```text
cdc063d058cc775ec2ee45a4231d8458ad3e9d43
parent bf7cdd0de71f8fbfd6e86c34852062f0766fddc7
git merge-base --is-ancestor cdc063d base => 0
```

The four heads based on it are one-commit siblings, not independent waves:

```text
89c8a284d20e  fix(index): concurrency-safe refresh DB and WAL/SHM cleanup
9010fcca1215  docs: add hostile review of 92d0067 hook ingest dedup
472c48911577  docs: hostile review of guarded catalog and fallback semantics
47d986f40a96  docs: whole-repo anti-bloat audit on cdc063d
```

Stable patch IDs, computed from each commit's immutable patch:

```text
89c8a284d20e  7e3d7d4981316e4cfb2b11f6a8eacbb71b314bdc
9010fcca1215  cc85c664df60215cbc1c5134b92f8f1f4ca07e0d
472c48911577  836e71c7981e9a5d7fe84006b332b3a747d1ec56
47d986f40a96  32aed4e337cd9cbaf0f2300ed1dbd7bac9c0e3e8
```

`git range-diff base...cdc063d base...89c8a28` maps all prior commits
one-for-one and shows only `89c8a28` as an added commit. The other three
heads add report files only: 264, 212, and 153 Markdown lines respectively.

## Confirmed cleanup defect: probe-to-unlink TOCTOU

Receipt: `89c8a284d20e:internal/index/containers.go` lines 78-114.

The implementation does:

```text
L82   dbPath := filepath.Join(dir, base)
L86   if isLockedOrActive(dbPath) { continue }
L89   removeRefreshDB(dbPath)

L94   sql.Open(...)
L99   db.Exec("BEGIN IMMEDIATE; ROLLBACK;")
L104  os.Stat(dbPath)       // a second, separate freshness observation
L107  return false
```

`BEGIN IMMEDIATE` obtains and then releases the SQLite transaction lock in the
same call. After `isLockedOrActive` returns false, another process may open
the stale path, create a new connection, or begin a writer before line 89.
The sweeper then unconditionally calls `os.Remove` on the database and its
`-wal`/`-shm` siblings. This is a confirmed ownership deduction from the
immutable control flow: the process never holds a lock or uses an inode/path
claim spanning the decision and unlink. It can delete live refresh state after
the check has passed.

The mtime check at lines 104-107 does not close this gap; it is also performed
before the return and is not revalidated at unlink time. The same grouping
logic at lines 57-75 makes the decision from a directory snapshot, while the
remove operation trusts the path later.

### Reproduction evidence and gate limitation

The supplied focused test gate was run against immutable `89c8a284d20e`:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/index \
  -run 'TestEnsureFreshContainer_PruneStaleLeftovers_ActiveWriterFenced|TestEnsureFreshContainer_ConcurrentIsolation'
ok github.com/MoonCaves/rawclaw/internal/index 14.036s
```

`TestEnsureFreshContainer_PruneStaleLeftovers_ActiveWriterFenced` only holds
the transaction throughout the probe, so it exercises the busy branch and
passes. `TestEnsureFreshContainer_ConcurrentIsolation` uses distinct database
paths, so it cannot race a stale-file owner. Neither test inserts an opener in
the interval between `ROLLBACK` and `os.Remove`; therefore the green result is
not evidence that cleanup is concurrency-safe.

## Active dirty payload

The active prune worktree `/Users/jay-m4/code/rawclaw-ozzy-flash-prune` is not
clean. Its uncommitted payload is one benchmark-only addition to
`internal/index/consolidated_test.go` (29 lines, `BenchmarkPruneTombstonedIDs`).
It is not present in commit `cdc063d` or any named Ozzy head. The immutable
receipt is:

```text
git -C rawclaw-ozzy-flash-prune status --porcelain
 M internal/index/consolidated_test.go
git -C rawclaw-ozzy-flash-prune diff --check
internal/index/consolidated_test.go:2217: new blank line at EOF.
```

Classification: `CONFIRMED DEDUCTION` for a dirty report-only/test payload,
not production behavior. It is test-only and fails the required whitespace
gate.

All other named Ozzy worktrees were clean at audit time. The cleanup worktree
was clean after its committed test addition; the report-only heads contain no
production changes beyond the already-integrated base.

## Hook hostile-path review

The hook report at `9010fcca1215:FINDINGS-OZZY-HOOK.md` records a real defect
against historical `92d0067`, but explicitly says it was fixed by `821b78d`.
The assigned base includes the later fail-soft hook changes. Current source
`internal/cli/setup.go:299-314` still has the baked absolute path, PATH
fallback, and quiet exit contract. The base gate passed:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli \
  -run 'TestPrimeHook_|TestPrimeScript_CatalogWriteFailure_NeverFailsHook|TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest'
ok github.com/MoonCaves/rawclaw/internal/cli 13.481s
```

Classification: `NO DEDUCTION` against the current base; do not re-file the
historical unwritable-catalog or mixed-source-global claims as active defects.

## Closeout

No production code was modified. This file is the only intended audit output.
