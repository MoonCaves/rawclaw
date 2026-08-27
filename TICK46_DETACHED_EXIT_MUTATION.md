# Tick 46 detached-exit mutation attack

## Scope and verdict

Target: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`, rooted directly on
`4119698525e806025ec36d00e0c85a5b1b3574a7`.

- Detached child survival after parent exit: **CONFIRMED for the tested launch window only**.
  The parent test process returned immediately after `Start`; the child subsequently wrote
  `child-start`, `app-enter`, and `terminal-success` to the inherited log. This does not prove
  survival across arbitrary supervisor/process-group termination.
- Terminal receipt durability/guarantee: **REBUTTED**. The only receipt seam is an append-only
  `tag-publish.log` owned by the child. There is no durable pending record, ownership claim,
  retry/adoption protocol, or parent-observable terminal acknowledgement. A child that exits
  after `Start` (or is killed before entering application work) leaves no terminal receipt.

## Exact selection and observed commands

Disposable helper `TestT46ParentExitHelper` was added under `internal/cli/`, formatted, and
removed before this report. Exact selection was verified with:

```text
go test ./internal/cli -list '^TestT46ParentExitHelper$'
TestT46ParentExitHelper
```

Baseline command (task-local `HOME`, `GOCACHE`, `T46_DIR`):

```text
T46_HELPER=1 T46_DIR="$d" HOME="$d" GOCACHE="$d/gocache" \
  go test ./internal/cli -run '^TestT46ParentExitHelper$' -count=1 -v
```

Observed parent exit `0`; test elapsed about 22 seconds including first build. After the
parent exited, the child log contained exactly:

```text
child-start
app-enter
terminal-success
```

Log SHA-256: `caef0a5231a9dd771461fad40aa12048f15639a4324ba89a55b1e99bbfa56984`.

Focused race command was also run with `CGO_ENABLED=0`, `-race`, `-count=1`, and task-local
cache/home. It exited `0` in about 27 seconds and reproduced the same three markers. Output
SHA-256: `25c3d4d4f31e8d04e2344bfa72b4949bf9cd93d2f16978d08c394b454b0d0d12`.

## Mutations

Each mutation was isolated and reverted before report creation.

| Mutation | Observation | Layer reached? | Result |
|---|---|---:|---|
| Remove `Setsid` from `detach` | Parent `0`; child still wrote all three markers | yes | **GREEN**, so this test does not establish that `Setsid` is necessary |
| Parent returns immediately after `Start` | Baseline helper behavior | yes | **GREEN** |
| Child exits before application marker | Log contained only `child-start` | no | **RED** for the application-entry assertion |
| Child exits before terminal marker | Log contained `child-start`, `app-enter`, but no terminal line | application only | **RED** for terminal-receipt assertion |
| Spawn twice | Log contained two `child-start` and two `terminal-success` lines | yes | **RED** for no-duplicate-child/result claim |

The duplicate result is important: `spawnTagPublishChild` has no deduplication. Any no-duplicate
property is outside this seam and must be established by its caller/queue owner.

## Application path and missing observation

Existing candidate tests separately show the intended tag-prep/authoritative-write and fold path
(`TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication`), but that test replaces
`spawnTagPublish` in-process and therefore does not combine application work with parent process
exit. The detached helper proves launch and post-parent markers only. The current production code
does not expose a durable terminal receipt that an independent observer can distinguish from a
child that disappeared after `Start`; therefore terminal publication after parent exit remains
unproven and is rebutted as a guarantee.

## Patch/ancestry accounting

The candidate's direct payload is the two-file index change in `8e9c9b77` (`consolidated.go` and
`consolidated_test.go`). The detached publisher is inherited ancestry beginning at `ebc1711`
(`tagpublish.go`), followed by the queued-publication tests/fixes through `00e587d` and
`332af5d`. Whole/path patch-id and range-diff commands could not be rerun after the mailbox hook
became active: every shell command was blocked by an unread supervisor mailbox in another
worktree, and this task explicitly forbids reading or advancing that mailbox. No patch-id or
range-diff value is fabricated here; status is **UNCERTAIN** for that accounting only.

## Reproducibility boundary

The baseline is evidence that this Unix launch survives ordinary parent return and reaches the
fake application's terminal marker. It is not evidence of crash durability, supervisor kill
survival, exactly-once publication, or an externally durable success/failure receipt. Prior art
from `c9c1ebdef669ed0caa2dd9fcee7d58f68da93071` remains applicable: `WaitDelay` observes attached
child/pipe status, pidfd observes process death only, and `setsid`/`Process.Release` do not create
a terminal application receipt.
