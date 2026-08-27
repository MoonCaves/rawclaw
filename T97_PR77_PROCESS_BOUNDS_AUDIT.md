# T97 PR77 process-bounds audit

## Verdict

**ACCEPT for the process-tree, timeout-bound, stdio-capture, and child-ownership mechanisms at current candidate `8dd07064357bbb1b922e1c4953d58ff0fbaaaf31`.** The historical assignment base was `6e3694366473b8b656d931b9eb4fae2e03a4fe2e`; the supervisor explicitly re-anchored decisive evidence to `8dd0706`.

No product files were changed. Patch identity: current candidate is `8dd0706` (`fix: type closeout child flag`), descendant of historical `6e36943`; `git diff 6e369436 8dd0706 -- internal/cli` contains the token/timeout/process changes under audit.

## Code-path findings

- Unix `configureCloseoutProcess` sets `SysProcAttr.Setpgid=true`; timeout termination sends `SIGKILL` to the negative process-group PID. This reaches shell descendants that inherit stdout/stderr.
- Windows `terminateCloseoutProcess` runs `taskkill /T /F /PID` under a one-second `CommandContext`, then directly calls `Process.Kill`; process-already-done is ignored, while real kill/tree errors are returned. The caller waits at most one further second for `cmd.Wait`, so the cleanup path is bounded.
- `runCloseoutTaggerContext` captures stdout and stderr in independent temporary files. Descendants inheriting those descriptors cannot keep the parent blocked on an in-memory pipe; files are closed and removed on all return paths. Stderr is rewound and copied before errors are returned.
- The tagger has a configurable deadline (`closeoutTaggerTimeout`, 60s default). The detached child has a five-minute context; each self-command is `exec.CommandContext`, and each tagger invocation is explicitly terminated and wait-bounded.
- Closeout ownership is an opaque 32-byte random token stored in an atomic lock directory. The child validates the exact token before work and defers token release. A stale lock is reclaimed only after the five-minute child timeout multiplied by two.
- The process is deliberately not launched with `exec.CommandContext` for the tagger: cancellation must be followed by process-group/tree termination, otherwise a descendant can survive while holding inherited descriptors.

## Commands and observed exits

All commands below ran from a disposable detached worktree at the exact current candidate.

```text
git rev-parse HEAD
8dd07064357bbb1b922e1c4953d58ff0fbaaaf31

CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunCloseoutTagger|TestWatchdog'
ok   github.com/MoonCaves/rawclaw/internal/cli  4.386s
exit 0

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./internal/cli
exit 0

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -run '^$' ./internal/cli
exit 1: existing Unix-only catalog_hook_test.go and cmd_ingest_test.go use syscall.Mkfifo
```

The Windows product package cross-builds. Windows tests cannot compile as a package because unrelated tests are not build-tagged away from Windows.

## Disposable mutation results

Mutations were applied only in `/private/tmp/rawclaw-t97-current`, gofmt'd, tested, and restored; no mutation was committed.

1. Unix `Setpgid: true` replaced by an empty `SysProcAttr`: `TestRunCloseoutTagger_TimeoutKillsDescendantHoldingStdio` reached its functional `PASS` assertion but package exit was **1** because goleak found the `cmd.Wait` goroutine blocked on the surviving descendant. **Killed**.
2. Unix `terminateCloseoutProcess` changed to return nil without signalling: same focused test exited **1**, with goleak reporting blocked `cmd.Wait`. **Killed**.
3. Post-termination wait grace changed from `time.After(time.Second)` to `time.After(0)`: focused test exited **0**. **Survived**. This identifies a coverage gap around delayed termination; it does not show an unbounded production path because the implementation's actual termination is synchronous and the caller still has the one-second wait in the unmutated candidate.

## Scope limits and residual risk

- No live Windows execution was available; Windows behavior is established by source inspection plus successful cross-build, not an empirical descendant-kill run.
- The one-second Windows `taskkill` timeout plus one-second `cmd.Wait` grace is a clear finite bound, but the report does not claim a strict wall-clock bound for OS scheduling or process-launch overhead.
- No source or test edits were needed. Net product lines: `0`; audit artifact: `+1` file.

