# T96 PR77 terminal/tagger mutation audit

Date: 2026-08-28 WITA

## Scope and revisions

- Initial requested PR77 head: `e3ec1b86e4ea96d6e11016a63e65bda294d18b3f`.
- Current PR77 head supplied by supervisor: `daffc5f50c306e09f7008b295262a7ccedab6cd3` (`fix: bound closeout locks and process trees`).
- The initial checkout was left on its requested branch and restored byte-for-byte. Current-head checks ran in disposable clean worktree `/tmp/rawclaw-t96-current2.p0DbD6`; it was never rebased into the requested worktree.
- No product edits remain. Only this report is added in the requested worktree.

## Exact test filters

Initial head:

```text
go test ./internal/cli -list 'Test.*(Closeout|Watchdog|Detach|Tagger|Timeout)'
```

Listed 27 tests, including four closeout/tagger tests. The focused race gate

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Test.*(Closeout|Watchdog|Detach|Tagger|Timeout)'
```

passed: `ok github.com/MoonCaves/rawclaw/internal/cli 0.796s`.

Current head:

```text
go test ./internal/cli -list 'Test.*(Closeout|Watchdog|Detach|Tagger|Timeout)'
```

listed the same 27-name filter, now including the added closeout tests:
`TestRunCloseout_ConcurrentParentsLaunchOnce`,
`TestCloseoutTokenHeldUntilExplicitRelease`,
`TestRunCloseoutChild_ReleasesTokenAfterCompletionOrFailure`, and
`TestRunCloseoutTagger_TimeoutKillsDescendantHoldingStdio`.

The current-head focused race gate passed:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Test.*(Closeout|Watchdog|Detach|Tagger|Timeout)'
```

`ok github.com/MoonCaves/rawclaw/internal/cli 0.836s`.

The requested full-repository race gate was started on the initial head but
was stopped after ~88s because unrelated long-running archive/agentproto tests
were still producing output. It did not produce a repository-wide green result;
therefore no full-suite pass is claimed.

## Mutations and outcomes

Each mutation was applied to the named source, `gofmt -w internal/cli/cmd_closeout.go`
was run, the exact focused command was run, and the source was restored before
the next mutation. `SURVIVED` means the existing tests stayed green despite
the behavioral deletion/change.

### Initial head `e3ec1b8`

1. Removed `len(segments) == 0` rejection in `runCloseoutTagger`.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run '^TestRunCloseoutTagger' -count=1 -v`.
   Output: `PASS`, including the existing `empty` case. That case emits no
   bytes, not JSON `[]`, so the empty-array contract was untested.
   **SURVIVED.**

2. Replaced `context.WithTimeout(..., closeoutTaggerTimeout)` with
   `context.WithCancel(context.Background())`.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run '^TestRunCloseout|^TestRunCloseoutTagger' -count=1 -v`.
   Output: all tests passed, `ok .../internal/cli 0.940s`.
   **SURVIVED** (no timeout/cancellation test existed at this head).

3. Changed `closeoutMaxPasses` from `256` to `0`.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run '^TestRunCloseout|^TestRunCloseoutTagger' -count=1`.
   Output: `ok .../internal/cli 1.069s`.
   **SURVIVED** (no child loop/max-pass test existed).

4. Removed absolute-executable and non-empty-argv validation in
   `loadCloseoutTaggerConfig`.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run '^TestRunCloseout' -count=1`.
   Output: `ok .../internal/cli 1.024s`.
   **SURVIVED** (missing config was covered; malformed/empty/relative config
   was not).

5. Removed the `closeoutFailure` log/error return.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run '^TestRunCloseout' -count=1`.
   Output: `ok .../internal/cli 0.946s`.
   **SURVIVED** (child failure reporting had no test).

6. Removed `acquireIngestSpawnToken` and duplicate-output branch.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run '^TestRunCloseout' -count=1 -v`.
   Output: `TestRunCloseout_LaunchesOncePerSession` failed with
   `launches = [aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee], want one launch`.
   **KILLED.**

### Current head `daffc5f`

7. Removed `len(segments) == 0` rejection in `runCloseoutTagger`.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run '^TestRunCloseoutTagger' -count=1`.
   Output: `ok .../internal/cli 0.939s`.
   **SURVIVED.** The new timeout/process-tree test does not exercise `[]`.

8. Removed absolute-executable and non-empty-argv validation in
   `loadCloseoutTaggerConfig`.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run 'TestRunCloseout_(MissingConfigPrintsManualRecovery|LaunchesOncePerSession|ConcurrentParentsLaunchOnce|RequiresFullSessionID)|TestCloseoutTokenHeldUntilExplicitRelease|TestRunCloseoutChild_ReleasesTokenAfterCompletionOrFailure' -count=1`.
   Output: `ok .../internal/cli 0.794s`.
   **SURVIVED.**

9. Changed `closeoutMaxPasses` from `256` to `0`.
   Command: `CGO_ENABLED=0 go test ./internal/cli -run 'TestRunCloseout|TestCloseoutToken' -count=1`.
   Output: `ok .../internal/cli 1.097s`.
   **SURVIVED.** The new child tests cover token release, not pass exhaustion.

10. Removed `closeoutFailure` logging and error propagation.
    Command: `CGO_ENABLED=0 go test ./internal/cli -run '^TestRunCloseoutChild_ReleasesTokenAfterCompletionOrFailure$' -count=1 -v`.
    Output: failure in `failure`: `runCloseoutChild unexpectedly succeeded`.
    **KILLED.**

11. Replaced process-tree termination with `cmd.Process.Kill()`.
    Command: `env CGO_ENABLED=0 gtimeout 8s go test ./internal/cli -run '^TestRunCloseoutTagger_TimeoutKillsDescendantHoldingStdio$' -count=1 -v`.
    Output reached only `=== RUN TestRunCloseoutTagger_TimeoutKillsDescendantHoldingStdio`
    before the external 8-second bound expired. The descendant retained stdio,
    demonstrating that direct-child kill is insufficient.
    **KILLED** by the current descendant/process-tree test (bounded externally
    to avoid waiting for the deliberately sleeping descendant).

## Verdict

**REJECT** for terminal/tagger test completeness at current head.

Current PR77 correctly kills the tested token-release, child-error, and
process-tree mutations, but empty JSON arrays, malformed/empty/relative config,
and the 256-pass termination bound still have surviving mutations. The focused
terminal gate is green, yet the mutation evidence is not strong enough to ship
the claimed terminal/tagger contract without those tests.
