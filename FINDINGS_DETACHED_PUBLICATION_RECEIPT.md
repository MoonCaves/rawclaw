# Detached publication receipt finding

Base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`
Candidate reviewed: `8c8216e` (parent stack through `f35625b`)

## Red finding

The detached tag publisher can disappear after the parent reports that it was
queued without producing an observable terminal success/failure receipt.

Evidence in the candidate:

- `internal/cli/cmd_tag.go:520-527` prints `publication queued` after
  `spawnTagPublish` returns nil. That nil means only that `exec.Cmd.Start`
  returned successfully.
- `internal/cli/tagpublish.go:56-65` starts the child and immediately calls
  `Process.Release`, ignoring its error. The parent does not wait, supervise,
  or persist a completion record.
- `internal/cli/tagpublish.go:68-78` writes the only terminal `published` or
  error receipt from inside the child, after startup and before/after the
  consolidated sync. There is no startup receipt and no parent-side failure
  receipt for a child that dies between `Start` and entering `runTagPublishChild`.

The failure window is an OS-level ordering fact, not a timing-test guess:
`Start` returning does not imply that the child has executed user code. A
SIGKILL, launch-service reaping, or equivalent process failure in that window
leaves the parent's `publication queued` line as the only observation and the
tag-publish log without a terminal line. `Process.Release` cannot close this
window because it only relinquishes the parent's wait ownership.

## Observable contract that is currently missing

The foreground command promises only "publication queued (read-after-write is
eventual)". There is no durable state distinguishing queued, started, failed,
completed, or vanished. A caller cannot tell whether eventual visibility is
pending or permanently lost.

## Smallest safe disposition

Keep the finding open for the detached-publication design review. Adding a
single log line after `Start` would prove launch, but would not provide a
terminal receipt after a post-launch disappearance. Closing this finding
requires either a durable retry/queue owner or an explicit contract that
publication is best-effort and that absence of a terminal log line is an
allowed outcome; neither is a one-line correction.

No production code was changed in this report-only red proof.

## Independent referee reproduction

Reviewed candidate `8c8216e25e22496b2e3e919fce836be49d692e25` in detached
worktree `/private/tmp/furiosa-candidate-run-20260827`. The focused race gate
passed:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run \
  'TestRunTagPublishChildHonorsContextTimeout|TestRunTagWriteCommandSeamIsNotIsolated'
ok   github.com/MoonCaves/rawclaw/internal/cli  3.022s
```

I also built that candidate and ran a temporary harness that performs the same
bare `exec.Command` + `Start` + `Process.Kill` + `Release` sequence against
the real `tag-publish` child. Across 20 immediate-kill runs, every run
reported `Start` returned and `kill_err=<nil>` while the redirected child log
had `child_receipt_bytes=0`. Two 20 ms control runs also had zero bytes once
and a partial pre-receipt fence line once. This is direct timing evidence that
the child can be killed after `Start` returns but before its terminal log line;
the foreground's queued line would therefore be the only receipt.

The reproduction is deliberately external and temporary; no production source
or test was changed. It establishes the missing receipt, not a proposed queue
semantics. The smallest honest disposition remains **keep open**: durable
queue/retry ownership is required for guaranteed completion, while a best-effort
contract must explicitly permit a missing terminal line. A startup log line or
`Release` handling alone cannot close the post-`Start` window.
