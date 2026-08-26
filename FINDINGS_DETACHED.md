# Detached publication shootout

Review target: Ozzy tree tip `b7752f4` (supersedes `3170b19`, `2ad239c`, and the
short-lived compile-failing `f54829c`). Han tree tip `d2315cb` includes
implementation `7edd58d` plus its test-isolation follow-up; both core patches
have stable patch ID `6f276e8e4dcba0dedb80739a1966dfc10a3ca64a`. Furiosa
`8aab2cb`/`cc3e088` are characterization and cancellation audits. Current-base
source and focused race tests were inspected independently.

## Findings

1. **P1 — publication has no durable retry marker or queue.**
   `internal/cli/cmd_tag.go:518-526` starts one detached child and reports
   “publication queued”; if the machine exits, the binary is upgraded, or the
   child times out, the authoritative per-project tag remains absent from the
   consolidated cache until an unrelated consolidate/index pass. Reproducer:
   replace `spawnTagPublish` with a function returning `exec: not found`, run a
   successful tag-write, then read through consolidated-only access; the tag is
   missing although the command printed success. This is eventual-consistency
   behavior, not data loss, but “queued” overstates durability because no
   pending work survives process loss. **Ozzy and Han: -2 each.**

2. **P2 — child timeout is a hard process exit, not context-aware cancellation.**
   `internal/cli/tagpublish.go:75-83` accepts `ctx` only for a preflight check;
   `SyncConsolidatedFrom` receives no context. Reproducer: hold
   `consolidated.lock` or inject a long SQLite phase, invoke the hidden command
   with `--timeout 25s`, and observe watchdog exit 124 rather than a returned
   cancellation receipt. SQLite transaction recovery is expected, but the log
   can end mid-phase and cannot distinguish timeout from crash without the
   watchdog line. The bound is real, but cancellation observability is weak.
   **Ozzy and Han: -1 each.**

3. **P2 — duplicate tag writes create duplicate detached processes (both implementations).**
   `internal/cli/tagpublish.go:39-63` has no spawn token/coalescing marker.
   Reproducer: concurrently invoke `tag-write` ten times for the same project
   and count `tag-publish` children; ten are launched. The existing consolidated
   fence makes folds serialize and the fold is idempotent, so this is a process
   storm/performance defect rather than a correctness failure. **Ozzy and Han: -1 each.**

## Positive checks

- Authoring DB is closed before `spawnTagPublish` is called (`cmd_tag.go:508`
  then `:518`), so the child does not race an open parent connection.
- Parent stdout/stderr are not inherited: child streams go to
  `tag-publish.log`, and stdin is nil.
- Consolidated self-publication is rejected for empty paths, alternate clean
  spellings, and symlinks (`tagpublish.go:67-78`); the boundary tests pass.
- The authoritative write returns success even when child startup fails, with
  an explicit deferred-publication diagnostic.

## Point deductions

| Candidate | Score | Reason |
|---|---:|---|
| Furiosa `8aab2cb` + `cc3e088` | 0/10 | Useful latency/cancellation evidence, but no publisher implementation. |
| Han tree `d2315cb` (`7edd58d` implementation) | 6/10 | Same core detached handoff, bounded child, logs, and eventual-read output; lacks Ozzy's path-clean/symlink self-source guard and cancellation preflight. |
| Ozzy `b7752f4` | 6/10 | Small working detached handoff with honest eventual-read output, bounded child, logs, and self-source protection; deductions above remain. |

Verdict: **Ozzy and Han are equivalent core candidates, conditional on
accepting eventual publication without a durable retry queue. Ozzy has the
stronger self-source boundary; Han has equivalent publisher behavior.**
