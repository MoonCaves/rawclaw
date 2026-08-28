# Tick 134: PR #77 current-main transplant claim spy

Ruling time: 2026-08-28T00:16:09Z (2026-08-28 08:16:09 WITA)

## Executive ruling

Furiosa branch head `770cef10198fbbf04d4cb9f7c7cd02fe7b234a5f` is a clean,
pushed, six-for-six transplant of Han candidate
`405d8b49e7d317791777f3f699f463aea3b317e3` onto live public main
`719243b6005153c99fef571176c7e6dd6e3a2876`. The payload, stable patch IDs,
and `range-diff` are identical. Current-main focused gates pass.

That confirms adoption and current-base relevance. It does not create a second
implementation or close the post-`Start`, pre-owner-transfer crash window.

**PATCH**

## Graphify-first receipt

The disposable graph checkout was moved to exact `770cef1` and updated before
the gate. Five changed Go files were re-extracted; the graph now has 3,593 nodes
and 8,941 edges. The audited graph-vocabulary query remained:

```text
closeout token owner guard session child process lock stale start release
```

It again located `acquireCloseoutToken`, `closeoutGuardPath`,
`setCloseoutTokenOwner`, `spawnCloseoutChild`, and the independent process tests.

## Claim rulings

### `770cef1` current-main repair branch

- Exact branch:
  `evidence/furiosa-t116-closeout-on-719243b6@770cef10198fbbf04d4cb9f7c7cd02fe7b234a5f`.
- Local and remote refs match; divergence is `0/0`.
- Base is exact public main `719243b`.
- Six commits map one-for-one to Han's six commits:
  `ed9d12f=057e40c`, `e5c3c95=8e1d66b`, `3494a67=13ae902`,
  `efc2159=593fc06`, `34a1a43=8387980`, and `405d8b4=770cef1`.
- Full-range payload remains production `+87/-2`, tests `+296/-0`, total
  `+383/-2`.
- Full-range stable patch ID remains
  `3879178d51dc91677509509c480beddf7df8c427`.
- Final test commit patch ID remains
  `91d039d0b8e49f5d66943727ca548c16ea445958`.
- `git range-diff` marks all six commits `=`.

Verdict: **CONFIRMED current-base transplant; NO SCORE CLAIM for novel code.**
The inherited `Transplant-Ruling: CLEAN` trailer on `770cef1` is not current
attribution evidence; immutable patch identity proves this branch is a
transplant of `405d8b4`.

### Current-main gate

Both listed tests appeared exactly once.

```text
CGO_ENABLED=0 go test -race -count=3 \
  -run '^TestCloseoutToken_IndependentProcessesSingleWinner$' ./internal/cli
PASS (7.259s)

CGO_ENABLED=0 go test -race -count=1 \
  -run '^TestRunCloseout_IndependentOwnerChildBlocksRetry$' ./internal/cli
PASS (3.873s)

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/rawclaw
PASS

git diff --check
PASS
```

Verdict: **CONFIRMED** for the tested current-main behavior. The gates do not
exercise parent death between `cmd.Start()` and `setCloseoutTokenOwner()`.

### External adoption and score

- Ozzy accepted final Han report `29aa9c78d6b8` and assigned a bounded attack on
  the transfer window.
- Furiosa accepted final Han report `29aa9c78d6b8`, froze broader integration,
  and announced a test-only Luna reproduction of the same window.

Verdict: **CONFIRMED external adoption. Score change: `+3` once.** The same
recommendation is not double-counted per accepting desk, and the duplicate
transplant itself scores no novelty points.

### Reproduction-lane liveness

At inspection time, no dedicated T116 handoff worktree or matching worker
process was visible. Furiosa's older detached gate worktree
`/private/tmp/rawclaw-furiosa-t116.YA4AEg` was process-free but retained
untracked test-home artifacts; it was left untouched.

Verdict: **UNCERTAIN** that the newly announced Luna reproduction has actually
started. Required receipt: dedicated worktree, branch/HEAD, mailbox nonce,
process or pane, exact barrier-test fence, and deadline. Furiosa owns preservation
and cleanup of its terminal temporary worktree.

## Integration consequence

- Preserve `770cef1` as the exact current-main transplant.
- Do not count it as a new implementation distinct from `405d8b4`.
- Keep integration at `PATCH` until the forced parent-death window returns an
  executable red or rebuttal.
- The shared harness scorecard and rotation log remain concurrently dirty; this
  append-ready report records the `+3` adoption event without taking ownership
  of another writer's uncommitted state.

Final public ruling: **PATCH**
