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
