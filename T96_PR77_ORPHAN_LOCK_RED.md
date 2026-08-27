# Tick 96 PR77 orphan-lock current-head red

## Identity

- Public base: `758aa4417794c7a000e90f67c19e51f03817bdfd`.
- PR77 current head: `daffc5f50c306e09f7008b295262a7ccedab6cd3`.
- Stable current patch ID: `ebdf56d63bdf5828550c6e964d9e0e2485f54d49`.
- Graphify impact: 95 nodes across 13 communities and eight changed files.
- Current delta: 301 production Go lines, 231 test lines, and 11 documentation/findings lines; net `+543`.

## Baseline gate

The exact current checkout listed nine Closeout tests, including concurrent
parent launch, lifetime token, child token release, and descendant timeout.

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Closeout'
ok github.com/MoonCaves/rawclaw/internal/cli 2.425s
```

The current-head worker rerun also showed that removing the closeout-specific
detach call, replacing asynchronous Wait with Process.Release, and returning
before cmd.Start all survive the focused tests. The new process-group timeout
test does kill removal of tagger process-group setup.

## Forced crash interleaving

The new `acquireCloseoutToken` creates a permanent exclusive lock. Only the
child's deferred `releaseCloseoutToken` removes it. To force the exact gap
between lock acquisition and child start:

1. Build exact head `daffc5f50...`.
2. Configure an absolute tagger.
3. Replace the disposable HOME's `ingest.log` with a FIFO. This blocks the
   parent inside `openIngestLog`, after token acquisition and before
   `cmd.Start`.
4. Start `rawclaw closeout 97979797-1111-2222-3333-444444444444`.
5. Wait until the exact closeout lock exists and verify the parent is live.
6. Send SIGKILL to that exact parent PID, remove the FIFO, and retry the same
   full session ID.

Observed output:

```text
lock_seen_before_kill=1
parent_state_before_kill=RN
lock_after_kill=1
retry_exit=0
retry_output=closeout already queued for 97979797-1111-2222-3333-444444444444
```

The zero-byte lock remained. No child existed to execute the deferred release.
The retry reported success while doing no work, and the lock has neither a
lease nor an owner-recovery path.

A normal foreground exit did produce a child failure log. A separate worker's
100 ms delayed SIGKILL probe produced 20/20 child failure receipts. Those prove
the post-Start path, not the acquisition-to-Start crash boundary above.

## Ruling

**REJECT** current PR77. Add a regression that kills the parent after lock
acquisition but before Start, then use a durable lease/owner/reclamation
mechanism or an ordering that cannot strand the lock. Require exact red/green,
retry behavior, patch ID, net lines, and explicit public-action authority.
Green CI does not override this current-head red.
