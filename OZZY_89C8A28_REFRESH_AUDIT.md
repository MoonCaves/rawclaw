# Ozzy `89c8a28` refresh cleanup audit

Date: 2026-08-26. Read-only audit; no production or rival files changed.
`gofmt -w`: N/A (report-only).

## Artifact and diff

Target: `/Users/jay-m4/code/rawclaw-ozzy-flash-cleanup`, commit
`89c8a284d20e4f6adba72accb3c0b34831a3b422`, parent
`cdc063d058cc775ec2ee45a4231d8458ad3e9d43`, branch `ozzy/flash-refresh-cleanup`.

`git show --stat` reports `internal/index/containers.go` `+61/-15` and
`internal/index/containers_test.go` `+143/-0`: production net `+46`, test net
`+143`, documentation net `0`. The commit's stable patch-id is
`7e3d7d4981316e4cfb2b11f6a8eacbb71b314bdc`; it is
distinct from the later grouped-fence follow-up `aae80a4`.

The implementation groups `.db`, `-wal`, and `-shm` names by base at
`internal/index/containers.go:50-90`, then calls `isLockedOrActive` at `:82-89`
before `removeRefreshDB`. The guard opens SQLite and executes
`BEGIN IMMEDIATE; ROLLBACK` at `:93-103`, then closes via `defer db.Close()` at
`:98`. The actual unlink sequence is three independent calls at `:110-114`:
`os.Remove(db)`, `os.Remove(db-wal)`, and `os.Remove(db-shm)`.

## Finding: HOLD — probe-to-unlink race and non-atomic sidecar deletion

`isLockedOrActive` releases its transaction before the caller unlinks anything.
A concurrent writer can acquire the same database after `:108` returns and
before `removeRefreshDB` executes at `:110-113`; the cleanup can then delete a
live DB or its WAL/SHM sidecars. The three removals are also not one atomic
generation operation. This is a concurrency/data-loss risk, not merely a
style concern. Tags: `delete` (remove the unsafe sweeper), `native` (hold one
SQLite/file generation fence), `yagni` (the probe is not ownership).

The tests at `internal/index/containers_test.go:710-807` create a real WAL DB,
hold an exclusive transaction, run an unrelated `EnsureFreshContainer`, assert
the stale DB survives, release the lock, then assert a later sweep deletes it.
That proves the easy before/after path. It does not insert a writer between the
probe and unlink, does not assert DB/WAL/SHM deletion as one unit, and does not
prove a live writer cannot be reopened in that gap. Therefore the claimed
“concurrency-safe” commit is HOLD, not adopt.

## Observed gate

The worker's exact focused command was:

`go test -race -count=5 -shuffle=on ./internal/index -run 'Test(Container|EnsureIndexedContainers|AppendContainer)'`

The worker log records PASS in `15.224s` (and a semantic container subset in
`22.542s`); its full `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...`
receipt reports PASS in `131.983s`. Those tests are success-path and held-lock
coverage; no probe-to-unlink interleaving was observed. This spy did not rerun
the full package gate. No repository-wide green claim is made.

## Prior art and related candidates

The later `aae80a4` grouped-fence work is the relevant safer direction: retain
DB/WAL/SHM ownership as one generation under a writer fence and prove rebuild
failure leaves the live store untouched. The later `54bf2b0` lane instead
deleted the stale sweeper and its dedicated test (`internal/index/containers.go`
and `containers_test.go`), yielding production `-42` and test `-119`; that is a
safer deletion candidate than adopting `89c8a28`'s unlocked unlink path.

## Verdict

HOLD `89c8a28`. Its `+46` production lines add a lock probe whose ownership ends
before deletion, while the tests never exercise the critical interleaving.
Adopt only a complete writer fence spanning the decision and grouped DB/WAL/SHM
unlink, or delete the sweeper if stale cleanup is not required. The strongest
defensible deduction is that a passing 10x shuffled race suite cannot validate
a race window it never schedules.
