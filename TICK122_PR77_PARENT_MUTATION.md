# T122 PR77 parent-process mutation

## Scope and identity

Disposable worktree: `/Users/jay-m4/code/rawclaw-furiosa-t122-pr77-parent`

Branch: `worker/furiosa-t122-pr77-parent-20260828`

Tested current public PR77 head: `daffc5f50c306e09f7008b295262a7ccedab6cd3`.
The earlier frozen implementation at `e3ec1b86` was superseded by this repair
head. No production files were changed by this lane.

## Graphify and memory orientation

Graphify was queried first for `process group kill descendants closeout`, but
the available worktrees have no `graphify-out/graph.json`; the MCP response was
`graph.json not found`. A second query against the known graph worktree had the
same missing-artifact result. `mnemon --store rawclaw recall closeout process
group descendants` returned the prior repair receipt identifying `daffc5f` as
the process-tree fix with adversarial tests.

## Source result

At `internal/cli/cmd_closeout.go:177-210`, `runCloseoutTagger` now uses
`exec.Command`, calls `configureCloseoutProcess(cmd)`, starts and waits in a
goroutine, and on timeout calls `terminateCloseoutProcess(cmd)` before waiting
for completion. The platform helpers are
`internal/cli/closeout_process_unix.go` and
`internal/cli/closeout_process_windows.go`; the latter uses a Windows Job
Object. This repairs the prior direct-child-only `CommandContext` behavior.

## Exact focused reproduction

Command:

```sh
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunCloseout|TestCloseout|TestProcess' -v
```

Observed result: PASS, `ok github.com/MoonCaves/rawclaw/internal/cli 2.144s`.
Relevant exact test:
`TestRunCloseoutTagger_TimeoutKillsDescendantHoldingStdio` passed in `0.11s`.
The same run also passed closeout launch deduplication, token release after
completion/failure, and malformed/empty/nonzero tagger handling.

The adversarial test invokes a shell that starts `sleep 30 & wait`, lowers the
tagger timeout to `100ms`, and verifies timeout. Because the test's descendant
holds the inherited standard-error stream, a direct-child-only timeout would
hang or leave the descriptor open; the repaired Unix process-group path passed
and returned boundedly.

For comparison, the prior disposable mutation against PR77 `e3ec1b86` ran:

```sh
sh -c 'sleep 120 & echo $! > "$1"; exit 7' sh "$pidfile"
```

and observed `tagger_rc=7 elapsed_ms=57 descendant_pid=73664
descendant_alive=1`, proving the old leak shape. That process was explicitly
killed after observation. The repaired head's exact adversarial regression is
green.

## Verdict

`PATCH/REJECT/SHIP`: **ACCEPT current head `daffc5f` for this HIGH process-tree
blocker**. Unix process-group termination and Windows Job Object cleanup are
present, and the inherited-stdio descendant racer is green. This report does
not make claims about unrelated closeout terminal-receipt semantics.

Files/net lines: report only, `+68/-0`.
