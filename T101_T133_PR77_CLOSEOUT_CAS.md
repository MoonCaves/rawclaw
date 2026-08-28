# Tick 101 to Tick 133: PR #77 closeout crash-CAS ruling

Ruling time: 2026-08-28T00:00:55Z (2026-08-28 08:00:55 WITA)

## Ruling

`405d8b49e7d317791777f3f699f463aea3b317e3` is the only current candidate that
survived the independent process race, live-child retry, unrelated-session,
focused-package, Windows, formatting, and mutation gates below. It repairs the
process-local mutex and global-guard defects in the earlier candidates.

It is not final. The parent can still die after `cmd.Start()` and before
`setCloseoutTokenOwner()` records the detached child's PID. A retry may then
reclaim the dead parent's lease while the detached child is alive. The smallest
next correction is a bounded handoff protocol: the child must self-record its PID
before doing work, and a dead parent lease must remain non-reclaimable for the
handoff grace period. Add a barrier test that kills the parent in that exact
window and proves the retry remains blocked.

**PATCH**

## Mailbox and live assignment

- Historical Tick 101 was read without changing mailbox state.
- Tick 132 supplied the mutation/duplicate phase; Tick 133 arrived during the
  run and supplied the current harvest/integration phase.
- Owned cursor remained `20260828T004000Z-pr60-hook-ack.md`.
- Cursor file SHA-256 remained
  `8ac6f512ea8cfaed995a3585d2759b965e335101df3a9709906efc586c3a0566`.
- No foreign cursor was read or advanced.

## Graphify-first orientation

The disposable graph checkout was moved from `ed9d12f` to exact `405d8b4`, then
`graphify reflect --if-stale` and `graphify update .` were run before source
inspection. The update rebuilt 3 changed Go files into 3,587 nodes and 8,912
edges. The audited 11-token graph-vocabulary query was:

```text
closeout token owner guard session child process lock stale start release
```

Graphify located the exact mechanism and tests:

- `acquireCloseoutToken()` at `internal/cli/bg_ingest.go:68`;
- `closeoutGuardPath()` at `internal/cli/bg_ingest.go:134`;
- `setCloseoutTokenOwner()` at `internal/cli/bg_ingest.go:163`;
- `spawnCloseoutChild()` at `internal/cli/cmd_closeout.go:95`;
- the three independent-process tests in `internal/cli/cmd_closeout_test.go`.

The `Who Not How` source is RawClaw's existing `gofrs/flock` mechanism in
`internal/archive/lock.go`; the candidate reuses that dependency instead of
inventing a second cross-process lock implementation.

## Worker and rival inventory

| Lane | Immutable result | State | Ruling |
| --- | --- | --- | --- |
| Han harvest comparison | `c7ce876d12e2ec712d7109097bb130fe6c9b3058` | clean, pushed, upstream `0/0`, no process | ACCEPT evidence harvest |
| Han global-guard referee | `e0396ef3aa79a3b75ef142e6a2752c85d695778f`; red test `5ea0f1a762aba0db062a601006e56e6faf13e36b` | clean, pushed, upstream `0/0`, no process | ACCEPT rejection of `efc2159` |
| Han crash-CAS repair | `405d8b49e7d317791777f3f699f463aea3b317e3` | clean, pushed, upstream `0/0`, no process | ACCEPT as winner; PATCH before integration |
| Furiosa challenge acceptance | `20260827T233759Z-02d26944-accept-t129-challenge-efc-orig.md` | supervisor branch `d552feeb`, upstream `0/0`, untracked mailbox artifacts; left untouched | ACCEPT challenge receipt, zero adoption score |

No worker tmux pane remained. Only supervisor, heartbeat, and wake-relay sessions
were listed. Terminal worker worktrees were preserved; no rival cleanup occurred.
No replacement worker was launched because this is the user-directed closeout
race and all three owned lanes had terminal, pushed results.

## Same-base and duplicate accounting

- PR #77 merge commit/base for the repair: `9ddacb19cc27355873f36ed7fbaa6208b34c0d03`.
- Live GitHub `main`: `719243b6005153c99fef571176c7e6dd6e3a2876`.
- `9ddacb19` is an ancestor of live main.
- Live main changed none of `internal/cli/bg_ingest.go`,
  `internal/cli/cmd_closeout.go`, or `internal/cli/cmd_closeout_test.go` after
  `9ddacb19`; the repair remains current-base relevant.
- Full repair range: production `+87/-2`, tests `+296/-0`, total `+383/-2`.
- Full-range stable patch ID:
  `3879178d51dc91677509509c480beddf7df8c427`.
- Production-only stable patch ID:
  `c9cb08b0c3904b5b3d797beee280ea4ac84466ee`.
- Final deterministic-test commit patch ID:
  `91d039d0b8e49f5d66943727ca548c16ea445958`.
- `git range-diff` shows `ed9d12f` retained and five unique corrective commits
  added: adversarial tests, cross-process guard and PID transfer, test cleanup,
  per-session guard isolation, and deterministic process barriers.

## Independently observed gate

Every exact test name was preflighted with `go test -list`; each produced exactly
one match. Every command used `set -euo pipefail`.

```text
CGO_ENABLED=0 go test -race -count=10 \
  -run '^TestCloseoutToken_IndependentProcessesSingleWinner$' ./internal/cli
PASS (13.935s)

CGO_ENABLED=0 go test -race -count=1 \
  -run '^TestRunCloseout_IndependentOwnerChildBlocksRetry$' ./internal/cli
PASS (3.653s)

CGO_ENABLED=0 go test -race -count=1 \
  -run '^TestCloseoutToken_UnrelatedSessionsDoNotShareGuard$' ./internal/cli
PASS (1.556s)

CGO_ENABLED=0 go test -race -count=1 \
  -run '^Test(RunCloseout|CloseoutToken)' ./internal/cli
PASS (6.162s)

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/rawclaw
PASS

gofmt -l internal/cli/bg_ingest.go internal/cli/cmd_closeout.go \
  internal/cli/cmd_closeout_test.go
PASS (no output)

git diff --check
PASS
```

No `cli.test`, RawClaw closeout, or closeout-helper process survived the gate.

## Disposable mutation

On an exact `405d8b4` disposable worktree, the liveness predicate was inverted:

```diff
- return process.Signal(syscall.Signal(0)) == nil
+ return process.Signal(syscall.Signal(0)) != nil
```

The exact live-child gate failed deterministically:

```text
--- FAIL: TestRunCloseout_IndependentOwnerChildBlocksRetry
retry output = "closeout queued for 67676767-8989-0101-bcbc-dededededede\nPASS\n",
want already queued while child is live
```

The mutant worktree was removed after confirming no surviving helper process.

## Score and direction lock

- Score change: `0`. Furiosa accepted a challenge, but no separate desk has yet
  adopted the final per-session guarded repair.
- Direction lock: not opened. External adoption is absent and the
  `Start`-to-owner-transfer invalidation remains.
- Next action requested from another desk: reproduce the forced parent-death
  handoff window on `405d8b4`, then either supply the bounded grace/self-claim
  correction or rebut the window with an executable proof.

Final public ruling: **PATCH**
