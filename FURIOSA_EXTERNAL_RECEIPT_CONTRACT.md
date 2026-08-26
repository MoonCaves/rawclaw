# External receipt contract referee report

Date: 2026-08-27 WITA

## Verdict

**Refute the claimed terminal-receipt guarantee. The post-`Start`, pre-child-entry
window is real and leaves no terminal receipt.** The smallest honest disposition
is a best-effort contract with precise wording, unless the product explicitly
requires durable eventual publication. Durable ownership and retry is the
smallest solution for that stronger guarantee, but it is not a tiny correction.

## Exact source evidence

Reviewed candidate `8c8216e25e22496b2e3e919fce836be49d692e25`, whose detached
publisher is also introduced by `d0d716c46f32573fee02b6e951acf76aa7a04c8f`.

- `internal/cli/cmd_tag.go:519-525`: after the authoritative write, a nil
  `spawnTagPublish` result prints `tag-write: publication queued
  (read-after-write is eventual)`.
- `internal/cli/tagpublish.go:43-65`: `spawnTagPublishChild` builds the child,
  calls `cmd.Start()`, ignores `Process.Release()`'s error, and returns nil.
  There is no `Wait`, supervisor, durable job record, or retry owner.
- `internal/cli/tagpublish.go:68-78`: the child emits the only terminal
  `published` or error line, after it has entered `runTagPublishChild`.
- `internal/cli/tagpublish.go:81-90`: the log is append-only output, not a
  state record that can distinguish queued, started, vanished, failed, or done.

The standard-library contract corroborates the distinction: `exec.Cmd.Start`
starts without waiting and, on success, only sets `c.Process`; `Process.Release`
relinquishes the parent's process resources. Neither establishes child user-code
entry or terminal completion.

## Adversarial window proof

The observable ordering is:

```text
parent: cmd.Start() returns
parent: Process.Release(); return nil
parent: print "publication queued"
child:  process scheduling / startup / argument handling
child:  runTagPublishChild -> terminal log line
```

A process can be killed, reaped, or otherwise fail after `Start` reports success
and before the child reaches `runTagPublishChild`. In that execution the parent
has already returned success and printed queued, while the only terminal writer
has not run. `Release` cannot close the interval because it removes wait
ownership; it does not wait, supervise, or create a receipt.

This is an ordering proof, not a claim about a likely scheduler delay. The
candidate contains no synchronization or startup handshake that could make the
interval unobservable. A startup log line would only prove entry if written by
the child; it still would not supply completion after a subsequent disappearance.

## Test coverage and mutation limits

The candidate test history contains:

- `TestRunTagPublishChildHonorsContextTimeout` in
  `internal/cli/cmd_tag_detached_test.go:17-60`: proves bounded child context
  cancellation and an error receipt while child code is running.
- `TestRunTagWriteCommandSeamIsNotIsolated` at `:62-131`: proves the command
  path can be blocked in guarded lookup before the authoritative write. It does
  not reach or control `Start`.
- `2ad239c776b143a3b18ca686ad1bcc5c908f735f` adds boundary tests for empty/self
  sources and cancellation. None injects a post-`Start` child kill or asserts a
  receipt for that interval.

Therefore the candidate tests do not cover the interval, but an independent
temporary harness did reproduce it against candidate `8c8216e`. The reproduction
recorded 20/20 immediate-kill runs with `Start` returned, `kill_err=<nil>`, and
`child_receipt_bytes=0`. Two 20 ms controls produced zero bytes once and a
partial pre-receipt fence line once. This directly demonstrates child
disappearance after `Start` and before the terminal line. No production code or
test was changed in that reproduction.

The reproduction is recorded in historical commit
`0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f`, whose focused candidate race gate
passed in 3.022s:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run \
  'TestRunTagPublishChildHonorsContextTimeout|TestRunTagWriteCommandSeamIsNotIsolated'
ok github.com/MoonCaves/rawclaw/internal/cli 3.022s
```

The current report checkout’s focused command was:

```text
/usr/bin/time -p go test -race -count=1 ./internal/cli
FAIL: TestTagWriteDefaultScopeConsolidatedOnlyDoesNotBlock (0.44s)
FAIL github.com/MoonCaves/rawclaw/internal/cli 167.843s
real 169.63
user 69.09
sys 29.81
```

This is a pre-existing unrelated failure in the current checkout, not evidence
that the receipt claim passes or fails. Candidate-specific tests were not rerun
because this report branch does not contain `tagpublish.go`; running them would
require changing the checkout or creating another worktree, outside the stated
fence.

## Prior art and smallest honest contract

`internal/cli/autosync.go:63-91` and `internal/cli/vectortopup.go:25-47` use the
same intentional start-and-release pattern. Their comments describe detached
background work and receipt logs, but neither supplies durable ownership. This
is useful prior art for best-effort background work, not proof of a terminal
receipt guarantee.

Recommended wording for the existing behavior:

> `tag-write: authoritative write complete; publication started on a best-effort
> detached child. The consolidated read may remain stale, and the receipt log may
> contain no terminal line if the child does not enter or complete.`

If that uncertainty is unacceptable, replace detached fire-and-forget with a
durable pending publication record owned by a later retrying command/timer (or
wait synchronously). The record must be written before reporting queued, and
completion/failure must transition that record idempotently. Designing and
testing that owner is a separate feature, not a safe one-line fix.

## Git and Graphify receipts

- Report base/current HEAD: `2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce`.
- Prior finding: `987c6a31186bb15615175c5198389aa0d31846f6`; patch-id
  `33c107108b0ddd123cddaea88f5119e157c392a2`.
- Candidate: `8c8216e25e22496b2e3e919fce836be49d692e25`.
- Independent mutation reproduction: `0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f`.
- Best-effort wording correction in prior art: `1e61f732fc3f9234dd9b94a55407d7419fe71b47`.
- Candidate introduction: `d0d716c46f32573fee02b6e951acf76aa7a04c8f`; patch-id
  `8df42cb9ffc51473888b85bc823d23a15b713e7c`.
- Candidate baseline publisher: `f35625beb0a2895917fdfdadc53a6923d846b3e4`; patch-id
  `ee5d30372560e9a1820b952eb3f8da30b6b8992e`.
- Boundary test commit: `2ad239c776b143a3b18ca686ad1bcc5c908f735f`; patch-id
  `3739ebfa20344dd23a8459f1db3db231a6cf4100`.

Graphify receipts:

- `graphify reflect --if-stale` ran and refreshed
  `graphify-out/reflections/LESSONS.md`; it reported zero stored lessons.
- Freshness check found no local `graphify-out/graph.json`.
- Literal query `spawnTagPublishChild tag-publish receipt`, explain
  `spawnTagPublishChild`, path `spawnTagPublishChild` -> `tag-write`, and the
  later query/explain/path for `Start Process Release tagPublishLogLine` all
  returned `graph file not found`.
- No graph answer was used as evidence; source and Git evidence were used
  instead.

Push receipt for the first report commit: `6f5667e` was pushed successfully to
`origin/worker/furiosa-external-receipt-contract-20260827`; the final commit is
recorded in the scratchpad below.
