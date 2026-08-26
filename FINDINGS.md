# Furiosa T28 Han Claim Spy

Audit cutoff: `2026-08-26T23:14:15Z`; evidence freeze: `2026-08-26T23:53:30Z`.
Audit worktree base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.

## Verdicts

1. `20260826T232158Z-18d84aa2-tick25-harvest-challenge-lock-.md` — **NO SCORE CLAIM**.
   This is a Furiosa challenge, not a Han claim or adoption receipt. It requires a same-base
   candidate comparison and gates; no Han reply/adopter SHA exists in the post-cutoff mailbox.
2. `20260826T233808Z-50375dae-tick27-adopt-one-match-preflig.md` — **NO SCORE CLAIM**.
   This is a Furiosa challenge requiring an exact `go test -list` count of one before a focused
   race green; no Han reply/adopter SHA exists in the post-cutoff mailbox.

No material Han claim or adoption event occurred after the previous Claim Spy completion. The
post-cutoff mailbox inventory is exactly eight files:

```text
20260826T231442Z-132e4afc-ten-minute-tick-25-harvest-int.md
20260826T232158Z-18d84aa2-tick25-harvest-challenge-lock-.md
20260826T232443Z-013c17e8-ten-minute-tick-26-claim-spy.md
20260826T233443Z-2e56505e-ten-minute-tick-27-prior-art-r.md
20260826T233808Z-50375dae-tick27-adopt-one-match-preflig.md
20260826T234443Z-6083591c-ten-minute-tick-28-mutation-an.md
20260826T234857Z-662940ee-tick28-mutation-challenge-sile.md
20260826T234913Z-3bbe6f61-tick28-correction-complete-mut.md
```
Headers show only `two-supervisor scheduler` or `Imperator Furiosa / Evidence Prosecutor` as
senders; none is from Han. Exact challenge hashes are `917cf6421c396110d66e64b0c4a9faa6707cc1a4a62f67cc3546e8638f2c9760`
and `6dcad2897d60c5eb1ec92f9e18fd0813070df00b1c5eaef6b2b15de63b82a11a`.

## Branch and remote checks

`git for-each-ref refs/remotes/origin/han` found 17 Han refs. Their newest tip is
`origin/han/tick7-prior-art-20260827@6d36741cf6e2e02fa78387492813f3f4d637beed`, committed
`2026-08-27T04:22:07+08:00` (`2026-08-26T20:22:07Z`), before the cutoff. The supervisor branch
remained `supervisor/han-mechanism-20260827@0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; the
challenge itself records clean state and upstream parity `0/0` at the freeze. `git ls-remote
origin 'refs/heads/han/*'` returned the same 17 tip SHAs as local `origin/han/*` refs.

No Han payload exists to adjudicate for SHA, ancestry/range-diff, patch identity, mutation,
gates, or net production/test/doc lines. Those fields are therefore not invented and do not
earn credit. The focused-test exact-match rule in `50375dae` is retained as an unadopted
challenge, not treated as a Han implementation claim.

## Reproduction commands

```sh
git log --all --since='2026-08-26T23:14:15Z' --format='%H %ad %D %s' --date=iso-strict
git for-each-ref refs/remotes/origin/han --format='%(refname:short)|%(objectname)|%(committerdate:iso-strict)|%(subject)'
git ls-remote origin 'refs/heads/han/*'
find /Users/jay-m4/code/rawclaw-supervisor-han-b/.agent-mailbox -maxdepth 1 -type f -print \
  | sed 's#.*/##' | awk '$0 >= "20260826T231415Z"'
shasum -a 256 /Users/jay-m4/code/rawclaw-supervisor-han-b/.agent-mailbox/20260826T232158Z-18d84aa2-tick25-harvest-challenge-lock-.md \
  /Users/jay-m4/code/rawclaw-supervisor-han-b/.agent-mailbox/20260826T233808Z-50375dae-tick27-adopt-one-match-preflig.md
```

No Go files were edited; no Go gate was claimed. Only this report is in the editable fence.
