# Tick 98/126 PR77 merged claim-spy ruling

Run completion anchor: `2026-08-27T23:03:59Z`  
Supervisor: Han Solo / Mechanism Raider  
Phase: claim spy  
Verdict: `PATCH`

## Current immutable state

- Historical Tick 98 was superseded by the current Tick 126 claim-spy packet.
- PR #77 is now `MERGED`, not open: base `758aa4417794c7a000e90f67c19e51f03817bdfd`, head `8dd07064357bbb1b922e1c4953d58ff0fbaaaf31`, merge commit and current public `main` `9ddacb19cc27355873f36ed7fbaa6208b34c0d03`, merged at `2026-08-27T22:56:15Z`.
- GitHub Go 1.24, stable, and lint checks were all terminal `SUCCESS` before merge.
- The merged payload is eight files, `+800/-2`; stable patch ID `91ac968c947974ece46c4c410d1a2ae65aaedb79`.
- The repair branch `worker/luna-issue24-root-20260828-a@8dd07064357bbb1b922e1c4953d58ff0fbaaaf31` is clean, pushed, and upstream-equal `0/0`.
- Orphan process PID `73971` is absent at this tick. No process was killed without provenance.

## Graphify-first orientation

The exact-content candidate graph at `8dd07064357bbb1b922e1c4953d58ff0fbaaaf31` has `3112` nodes and `10321` edges. `graphify reflect --if-stale` reported current lessons. Literal graph-vocabulary query:

`closeout token lease lock child start timeout reclaim stale acquire process kill`

Graphify located `acquireCloseoutToken` and `runCloseout` in community 8, including the existing single-owner, stale-reclamation, spawn-failure, and child-release tests. It did not contain a `closeoutTokenTTL` node, so exact source corroboration followed.

## Independent current-main gates

Disposable exact-main worktree: `/private/tmp/rawclaw-han-t98-main.KEbqjt/src` at `9ddacb19cc27355873f36ed7fbaa6208b34c0d03`. Its tracked content is byte-equivalent to PR head `8dd0706`. Disposable test SHA-256: `c9e26a76688b37b7515cc4efc5361f56677c9f35a90f507cb69dc0ac4fd28b8a`.

Existing focused tests were a valid green control:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^Test(RunCloseout|CloseoutToken)'
ok github.com/MoonCaves/rawclaw/internal/cli 2.834s
```

The anchored real-parent-death test was listed exactly, then failed:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli \
  -run '^TestT98RunCloseout_ParentKilledBeforeChildStart_AllowsImmediateRetry$'
--- FAIL: TestT98RunCloseout_ParentKilledBeforeChildStart_AllowsImmediateRetry
immediate retry launches = 0, want 1; output =
"closeout already queued for 98989898-2222-3333-4444-555555555555\n"
```

The helper parent acquired the token, reached the `spawnCloseout` seam, and was killed before child `Start`. Immediate retry had no live child to perform the work, but the foreground command returned success and falsely reported queued work.

The 32-way concurrent stale-takeover test also failed under `-race -count=10`: eight observed iterations produced `3`, `4`, or `5` successful owners instead of exactly one.

## Root cause

1. `closeoutTokenTTL` is `closeoutChildTimeout * 2`, or ten minutes (`internal/cli/bg_ingest.go:24`).
2. `acquireCloseoutToken` treats an existing directory as live until `reclaimCloseoutToken` accepts it (`internal/cli/bg_ingest.go:64-89`). A parent killed after acquisition but before child start therefore blocks honest retry for ten minutes.
3. `runCloseout` collapses every acquisition failure into `closeout already queued` (`internal/cli/cmd_closeout.go:80-89`), although no work may exist.
4. Reclamation is still a pathname TOCTOU. Multiple contenders can all `Stat` the old stale directory at `internal/cli/bg_ingest.go:113-116`; after the first contender renames it and recreates the live path, a later contender's `Rename(path, quarantine)` at line 123 can rename that new live directory. The observed result is multiple valid acquisition winners.

## Worker and rival inventory

- `t97_crash_reclaim`: report branch `e125d80de791ba6893c83acfdf7eec1c2e9f2924`, clean/pushed `0/0`. Its simulated stale lease accepted delayed reclamation but did not exercise real immediate parent death. Candidate-level `ACCEPT` is rebutted; report evidence remains preserved.
- `t97_process_bounds`: report branch `831b11298cead6e2d71ed9f63d68dc52fc1fbd52`, clean/pushed `0/0`. Focused race and Windows cross-build evidence remains useful, but it does not cover either red above. Candidate-level `ACCEPT` is rebutted.
- `t97_harvest_compare`: report branch `b232cbf14ea789e5dfb7f0cb94feb23a81471687`, clean/pushed `0/0`. Its selection of `8dd0706` over older branches is historically correct but not a current-main safety ruling.
- New bounded Luna Medium repair lane: `/Users/jay-m4/code/rawclaw-han-t98-pr77-crash-cas`, branch `worker/han-t98-pr77-crash-cas-20260828@9ddacb19`, dedicated mailbox, file fence `internal/cli/bg_ingest.go` plus `internal/cli/cmd_closeout_test.go`. It must executably trial existing lock/lease prior art before editing and kill both reds.
- Live isolated supervisor processes observed: Norm PID `3871`, Furiosa PID `3874`, Rabbit PID `55898`, Ozzy PID `70622`. Khan had no live `codex` process in the inventory. PID `5888` remains rooted at the retired shared `/Users/jay-m4/code/rawclaw` checkout and is not treated as isolated-seat evidence.

## Mailbox integrity

- The Han cursor had been poisoned to a nonexistent quarantined future target `20260828T064100Z-t97-process-receipt-ack.md`.
- It was restored to the previously frozen exact value `20260828T004000Z-pr60-hook-ack.md`, SHA-256 `8ac6f512ea8cfaed995a3585d2759b965e335101df3a9709906efc586c3a0566`.
- A worker's manually named future handshake was preserved in `.clock-format-quarantine/`; SHA-256 `6a1cfe3228e5a5f25e4b708e64d8e020aa4b51028877d5ce09dec5f3122a493e`. The worker was redirected to the mailbox send helper and forbidden from touching this cursor.

## Tick receipt

- inbox messages acted on: historical Tick 98, current Tick 126, three T97 final reports, and current PR77 merge movement.
- all visible worker rulings: preserved report evidence `ACCEPT`; their candidate-wide `ACCEPT` verdicts `REBUTTED`; merged PR77 safety `PATCH`.
- owner-directed action: preserve all pushed T97 reports; do not remove rival worktrees; repair current `main` through the bounded current-base lane.
- prior-art handoff: no prior-art ledger launch or append in this claim-spy phase. Existing Backlite/River/systemd work remains comparator evidence, not scoreable adoption.
- score change: `0`. This independent post-merge finding has not yet received external adoption or rebuttal.
- archetype behavior: returned with the exact real-kill and stale-path interleavings that changed the merged candidate's outcome, then redirected a bounded lane toward the smallest existing mechanism.
- Who Not How source: standard atomic-rename/compare-and-swap semantics, the existing RawClaw token seam, the prior Backlite/River durable-job comparisons, and a Luna Medium worker required to executably trial a mature lock/lease mechanism.
- next risk: a follow-up must prove immediate crash recovery and exactly one stale-takeover winner together; fixing only the ten-minute delay can amplify duplicate children.

Public integration ruling: PR #77 is already merged, but the merged closeout lock is neither honestly retryable after pre-`Start` death nor single-winner under concurrent stale reclamation. Ship a current-main follow-up only after both red tests turn green and existing focused race gates remain green. `PATCH`
