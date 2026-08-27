# T96 PR77 detached closeout mutation audit

Date: 2026-08-28 (WITA)

## Scope and identity

- Audited checkout: `/Users/jay-m4/code/rawclaw-han-t96-pr77-detach`
- Branch: `worker/han-t96-pr77-detach-20260828`
- Exact PR77 head: `e3ec1b86e4ea96d6e11016a63e65bda294d18b3f`
- `git merge-base HEAD origin/main`: `758aa4417794c7a000e90f67c19e51f03817bdfd`
- Product source was restored byte-for-byte after each disposable mutation.

## Filter verification and baseline

Command (verified before trusting the filter):

```text
go test ./internal/cli -list 'Closeout|Detach|SpawnCloseout|TagPublish'
```

The list contained these relevant tests:

```text
TestRunCloseout_MissingConfigPrintsManualRecovery
TestRunCloseout_LaunchesOncePerSession
TestRunCloseout_RequiresFullSessionID
TestRunCloseoutTagger_RejectsFailureAndMalformedOutput
TestDetach_NewSession
TestRunTagPublishChildHonorsCanceledContext
```

Baseline command and observed output:

```text
go test -race -count=1 ./internal/cli -run 'TestRunCloseout_|TestDetach_|TestRunTagPublishChildHonorsCanceledContext'
ok   github.com/MoonCaves/rawclaw/internal/cli  2.298s
```

## Disposable mutations

| Mutation | Command/result | Verdict |
|---|---|---|
| `detach_unix.go`: change `Setsid: true` to `Setsid: false` | Same focused filter failed only `TestDetach_NewSession`: `Setsid:false, want Setsid`; source restored exact | Killed, but only by generic mechanical property test |
| `cmd_closeout.go`: remove the closeout-specific `detach(cmd)` call | Same focused filter: `ok ... 1.091s` | **Survived** |
| `cmd_closeout.go`: replace closeout child `go cmd.Wait()` with `cmd.Process.Release()` | Same focused filter: `ok ... 0.864s` | **Survived** |
| `cmd_closeout.go`: return from `spawnCloseoutChild` before `cmd.Start()` | Same focused filter: `ok ... 0.930s` | **Survived** |

The last three survivors show that no current closeout test launches the real
child, observes its process/session relationship, or requires the child to
reach a terminal success state.

## Real parent-exit interleaving

Built the exact checkout with:

```text
go build -o /tmp/rawclaw-t96-test-bin ./cmd/rawclaw
```

Created a disposable `HOME`, configured an absolute `/bin/true` tagger, then
ran the parent and allowed it to exit immediately:

```text
HOME="$home" /tmp/rawclaw-t96-test-bin closeout 11111111-2222-3333-4444-555555555555 > "$home/parent.out" 2> "$home/parent.err"
sleep 1
```

Observed:

```text
parent_rc=0
parent.out:
closeout queued for 11111111-2222-3333-4444-555555555555
parent.err:
(empty)
```

After the parent had exited, the child had written `~/.cache/session-search/ingest.log`:

```text
session "11111111-2222-3333-4444-555555555555" not found in scope
closeout: failed: tag-prep: exit status 1
tag-prep: exit status 1
```

This proves the detached worker was started and continued far enough to emit
a receipt after the foreground process returned. It does not prove successful
tagging, durable terminal completion, or that the closeout survives a terminal
or process-group signal. The test used a nonexistent session deliberately, so
the observed child result is a durable failure receipt, not a success path.

## Assessment

PR77's implementation has `setsid`, redirects child output to a log, and uses a
best-effort `Wait` goroutine. However, the existing suite does not prove the
closeout-specific detach call, does not prove `Wait` is needed, and does not
assert a durable terminal success marker. The real interleaving only proves
post-parent-exit child execution and logging.

**REJECT** — the detached closeout claim is under-proven. A ship-quality gate
needs a real subprocess test with a valid session and a controllable slow
tagger, parent exit immediately after queueing, an independent success receipt
assertion, and (where claimed) a terminal-signal/process-group survival check.

## Addendum: current head `daffc5f50c306e09f7008b295262a7ccedab6cd3`

This addendum audits the follow-up process-tree and closeout-lock changes. The
original report above is unchanged.

### Current filter and baseline

Verified the filter first:

```text
go test ./internal/cli -list 'Closeout|Detach|TagPublish'
```

The current list includes the new `TestRunCloseout_ConcurrentParentsLaunchOnce`,
`TestCloseoutTokenHeldUntilExplicitRelease`,
`TestRunCloseoutChild_ReleasesTokenAfterCompletionOrFailure`, and
`TestRunCloseoutTagger_TimeoutKillsDescendantHoldingStdio` tests.

Current focused race baseline:

```text
go test -race -count=1 ./internal/cli -run 'TestRunCloseout_|TestDetach_|TestRunTagPublishChildHonorsCanceledContext'
ok   github.com/MoonCaves/rawclaw/internal/cli  2.677s
```

### Current-head mutations

| Mutation | Result | Compared with old head |
|---|---|---|
| Remove `detach(cmd)` from `spawnCloseoutChild` | Focused filter passed: `ok ... 0.919s` | Old survivor remains a survivor |
| Remove `configureCloseoutProcess(cmd)` from `runCloseoutTagger` (no process group) | `TestRunCloseoutTagger_TimeoutKillsDescendantHoldingStdio` exceeded an 8-second harness timeout (`rc=124`) | **Old suite gap is now killed** by the new process-tree test |
| Replace closeout child `go cmd.Wait()` with `cmd.Process.Release()` | Focused filter passed: `ok ... 0.875s` | Old survivor remains a survivor |
| Return before `cmd.Start()` in `spawnCloseoutChild` | Focused filter passed: `ok ... 0.967s` | Old survivor remains a survivor |

All mutations were disposable and the current source was restored exactly
before the report branch was updated.

### 20-way immediate-parent SIGKILL probe

Built the current head, configured `/bin/true` as the absolute tagger, and ran
20 unique closeout parents. Each parent was SIGKILLed 100ms after launch; after
2 seconds the shared child log was counted:

```text
failure_receipts=20
log_exists=yes
```

All 20 children emitted the expected `closeout: failed: tag-prep: exit status 1`
receipt for the deliberately nonexistent session. This demonstrates that the
child/log path survives an immediate parent kill once the parent has had time
to queue the worker. It does not establish successful terminal completion,
because the session is intentionally invalid; nor does it cover the narrower
post-`Start`/pre-child-entry kill window.

### Current-head verdict

The follow-up correctly adds bounded process-group cleanup and closeout token
ownership, and the new timeout mutation is killed. The three closeout-spawn
mutations still survive, and the SIGKILL probe proves only failure receipt
continuation. **REJECT remains the current-head verdict** for the stronger
detached closeout / durable terminal-completion claim.
