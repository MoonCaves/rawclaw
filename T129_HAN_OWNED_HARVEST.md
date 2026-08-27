# Tick 129 Han-owned PR77 harvest inventory

Date: 2026-08-28 WITA. Public main at inspection:
`9ddacb19cc27355873f36ed7fbaa6208b34c0d03`.

## Method and process state

Graphify was run first against
`/private/tmp/rawclaw-han-t99-ed9d.Z0t0CD/src/graphify-out/graph.json`.
The exact symbols surfaced were `runCloseout()`, `acquireCloseoutToken()`,
`reclaimCloseoutToken()`, `releaseCloseoutToken()`, `runCloseoutChild()`,
`runCloseoutTagger()`, `closeoutTokenPath()`, and
`TestCloseoutToken_ConcurrentStaleTakeoverHasOneWinner()`.

`git worktree list`, process inspection, tmux pane inspection, branch tracking,
and report/receipt mtimes were used. No source was inspected or changed. No
worker process or pane had a current path under the eight recent PR77 worker
worktrees. The only live Han-related panes were the supervisor Codex at
`/Users/jay-m4/code/rawclaw-supervisor-han-b` and its wake relay; those are not
worker execution receipts.

All eight branch worktrees below track their origin branch at `0/0` unless
noted. “Harvest” means retain the report/commit as evidence for the public-main
follow-up; “terminate” means the lane is idle and has no worker process/pane to
kill, so no process termination was performed.

## Recent owned lanes

| lane / worktree | branch @ HEAD; dirty; upstream | receipts and observed evidence | independent ruling / action |
|---|---|---|---|
| T98 crash-CAS: `/Users/jay-m4/code/rawclaw-han-t98-pr77-crash-cas` | `worker/han-t98-pr77-crash-cas-20260828 @ ed9d12f5f019df143ccdb1e2a446f8fcca52a0c9`; **dirty**: `M internal/cli/cmd_closeout_test.go`; origin `0/0` | Commit at 07:09:03. Commit message says 119-line `bg_ingest.go`/test repair, PID lease metadata, serialized local stale reclaim, and 32-way stale-takeover repro. No final report file. Patch ID vs public main: `f29e6c15de196cfd64e40597028ef254c970402a`. | **HOLD / do not harvest.** The commit is pushed but the worktree has an uncommitted test mutation and no independently observed gate receipt in the lane. Preserve for supervisor review; terminate only after the owner confirms the dirty test is disposable. |
| T97 crash reclaim: `/Users/jay-m4/code/rawclaw-han-t97-pr77-crash-reclaim` | `worker/han-t97-pr77-crash-reclaim-20260828 @ e125d80de791ba6893c83acfdf7eec1c2e9f2924`; clean; origin `0/0` | `T97_PR77_CRASH_RECLAIM_AUDIT.md` fresh at 06:47:01. It accepts candidate `8dd0706`, reports focused race gates (1.97s and 1.70s), and an isolated stale-lease state-machine test (1.92s). Patch ID of report branch vs public main: `c64bff1d5a028cf398d9ab224fd80ce844952a31`. | **ACCEPT evidence; HARVEST report.** It directly addresses the Tick126 immediate-retry red, but its own residual limitation says no empirical SIGKILL run. No process/pane remains; terminate idle lane after harvesting. |
| T97 process bounds: `/Users/jay-m4/code/rawclaw-han-t97-pr77-process-bounds` | `worker/han-t97-pr77-process-bounds-20260828 @ 831b11298cead6e2d71ed9f63d68dc52fc1fbd52`; clean; origin `0/0` | `T97_PR77_PROCESS_BOUNDS_AUDIT.md` fresh at 06:46:39. Accepts 8dd0706; focused tagger/watchdog race passed 4.386s; Windows product cross-build passed; Windows tests failed to compile only because unrelated Unix-only tests use `syscall.Mkfifo`; mutations removing process-group kill were killed, zero grace survived. Patch ID: `8fac5da07fede826efb45aee0fc716180d9c2338`. | **ACCEPT evidence; HARVEST report.** Keep the explicit Windows-test limitation. No process/pane remains; terminate idle lane after harvesting. |
| T97 harvest compare: this worktree | `worker/han-t97-pr77-harvest-compare-20260828 @ b232cbf14ea789e5dfb7f0cb94feb23a81471687`; clean; origin `0/0` | `T97_PR77_HARVEST_COMPARE.md` committed/pushed. It selected 8dd0706 as the only same-base winner and rejected older partials. | **ACCEPT report; HARVEST.** This is the current lane and remains the report source for this Tick 129 inventory. |
| T96 current/addendum: `/Users/jay-m4/code/rawclaw-han-t96-pr77-current` | `worker/han-t96-pr77-detach-addendum-20260828 @ b174dd334ffe157f692b1bcc2dfedf01bdfe2aab`; clean; origin `0/0` | `T96_PR77_DETACH_MUTATION.md` fresh at 06:21:09. Current-head addendum retained **REJECT**: detach/wait/start mutations survived; SIGKILL proved only failure-receipt continuation. Patch ID: `fef13d45edfe4537201735ccbc399ad903aa80aa`. | **REJECT harvest as product candidate; HARVEST negative evidence.** No worker process/pane; terminate idle lane. |
| T96 terminal: `/Users/jay-m4/code/rawclaw-han-t96-pr77-terminal` | `worker/han-t96-pr77-terminal-20260828 @ d5d5516cd8f6ac717123c2625db5dd739d8a7b3d`; clean; origin `0/0` | `T96_PR77_TERMINAL_MUTATION.md` fresh at 06:22:23. Current-head focused race passed, but empty arrays, malformed/relative config, and pass-limit mutations survived; **REJECT**. Patch ID: `2dadc76464cd4a720540df7cc39d74ff6c2579eb`. | **REJECT harvest as product candidate; HARVEST negative evidence.** No worker process/pane; terminate idle lane. |
| T96 duplicate: `/Users/jay-m4/code/rawclaw-han-t96-pr77-duplicate` | `worker/han-t96-pr77-duplicate-20260828 @ 5f98aa36cc93d722068bec55cdd0d267f810b5b1`; clean; origin `0/0` | `T96_PR77_DUPLICATE_AUDIT.md` fresh at 06:17:00. Found process-wrapper and session-sanitization duplication, but judged the lock/process-tree repairs distinct and retained; patch ID: `b068bb0b3bb0cda9578a2daa58edadf6ff1121e1`. | **ACCEPT as review evidence only; HARVEST report.** No product transplant: helper extraction was explicitly a follow-up. No worker process/pane; terminate idle lane. |
| T96 detached: `/Users/jay-m4/code/rawclaw-han-t96-pr77-detach` | `worker/han-t96-pr77-detach-20260828 @ 4c02d017476fc60cf06b7b21673c7a7131abfc81`; clean; origin `0/0` | `T96_PR77_DETACH_MUTATION.md` fresh at 06:13:21. Baseline focused race passed; closeout-specific detach/wait/start mutations survived; real parent-exit run showed queued return and later failure receipt only; **REJECT**. Patch ID: `dfbfc9c7e5a37dc0d8d80c9e423e69275faac4d7`. | **REJECT harvest as product candidate; HARVEST negative evidence.** No worker process/pane; terminate idle lane. |

## Public-main relevance and deduplication

`9ddacb1` is the merge of `8dd0706`; therefore T97’s accepted 8dd reports are
historical evidence for the already merged implementation, not new product
commits. T98 `ed9d12f` is the only post-merge implementation candidate, but its
dirty test file prevents treating the pushed commit as a finished result. Its
commit claims a PID lease and serialized local stale reclaim; memory records a
known remaining cross-process stale-pathname CAS limitation. That limitation is
not silently promoted to ACCEPT.

The four T96 lanes are distinct audit artifacts, not four product candidates.
Their patch IDs are all unique because they add different reports to inherited
snapshots; no source patch should be harvested from them. The T97 process and
crash-reclaim reports overlap on 8dd mechanisms but cover different contracts
(process-tree bounds versus crash/stale ownership reclaim), so retain both as
non-duplicate evidence.

## Final actions

- **Harvest:** T97 crash-reclaim, T97 process-bounds, T97 harvest comparison,
  and T96 duplicate/terminal/detach reports as evidence; preserve their exact
  SHAs, gates, and mutation limitations.
- **Hold:** T98 `ed9d12f` until its dirty test edit receives a final owned-lane
  receipt and independently observed gates. Do not terminate the owner while
  the worktree is dirty without an explicit owner disposition.
- **Terminate idle lanes:** T96 current/addendum, T96 terminal, T96 duplicate,
  T96 detached, T97 crash-reclaim, and T97 process-bounds have no worker
  process/pane. No process kill was needed; their idle worktrees may be
  reclaimed by the supervisor after receipt collection.

No full-repository race gate is claimed. The exact observed gates are limited
to those recorded above and in the cited reports.
