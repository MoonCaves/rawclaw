# T97 PR77 `efc2159` global-guard referee

## Ruling

**RED / REJECT.** The new nonblocking `ingest-spawns/closeout.lock` is a
single global guard. While any process holds it, `acquireCloseoutToken` returns
`false` for every session ID, and `runCloseout` converts that failure into the
misleading `closeout already queued` response. Unrelated sessions therefore
contend and can be falsely reported as queued.

Candidate under test: `efc21594c37e3786c530faf5772284fb1fb376d7` (`test:
stabilize independent closeout process gate`). Only the requested files were
inspected: `internal/cli/bg_ingest.go` and `internal/cli/cmd_closeout_test.go`.

## Deterministic test

Added and committed in disposable exact-candidate worktree
`/private/tmp/rawclaw-t97-efc-referee`:

- Test: `TestRunCloseout_UnrelatedSessionIgnoresGlobalGuard`
- Test commit: `5ea0f1a762aba0db062a601006e56e6faf13e36b`
- Test branch: `worker/han-t97-process-bounds-efc-test`
- Test branch pushed and upstream-equal.

The helper process acquires the exact `ingest-spawns/closeout.lock` using
`flock.Lock()`, prints a `ready` handshake, and blocks on stdin. The parent
then invokes `runCloseout` for an unrelated session while the lock is
provably held, with `spawnCloseout` replaced by a recording seam. No sleep or
timing race is used.

Exact command:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli \
  -run '^TestRunCloseout_UnrelatedSessionIgnoresGlobalGuard$' -v
```

Observed output and exit:

```text
=== RUN   TestRunCloseout_UnrelatedSessionIgnoresGlobalGuard
    cmd_closeout_test.go:398: launches = [], want unrelated session to launch
--- FAIL: TestRunCloseout_UnrelatedSessionIgnoresGlobalGuard (0.03s)
FAIL
FAIL github.com/MoonCaves/rawclaw/internal/cli 0.744s
exit 1
```

This is a direct behavioral red: the unrelated session does not launch while
another session holds the global guard.

## Source evidence

At `internal/cli/bg_ingest.go`, `acquireCloseoutToken` constructs one
`flock.New(filepath.Join(dir, "closeout.lock"))` for all session IDs and calls
`TryLock`; a busy result returns `"", false` before computing the per-session
`closeoutTokenPath`. `runCloseout` then treats all acquisition failures as
`closeout already queued`, which is only valid for a same-session ownership
collision, not a global guard collision.

The per-session lock directories do not prevent this cross-session contention,
because the global flock is acquired first. `releaseCloseoutToken` also takes
the same global guard, so a holder can block unrelated release attempts.

## Scope and next action

No production fix was made. The test is intentionally preserved on its own
exact-candidate branch so the owner can choose the smallest repair (remove or
narrow the global guard, or distinguish guard-busy from same-session ownership)
and rerun this test plus the existing closeout race suite. Do not harvest
`efc2159` as green until this test exits 0.

Transplant-Ruling: ADAPT_TO_CURRENT
Candidate: efc21594c37e3786c530faf5772284fb1fb376d7; exact candidate test exposed global guard cross-session contention

