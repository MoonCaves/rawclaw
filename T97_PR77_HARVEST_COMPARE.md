# T97 PR77 Issue 24 harvest comparison

Date: 2026-08-28 WITA

## Verdict

**ACCEPT `8dd07064357bbb1b922e1c4953d58ff0fbaaaf31` as the same-base winner.**
It is the current exact candidate supplied by the supervisor, descends from the
public PR77 head, and carries the closeout crash-reclaim, bounded-token,
directory-ownership, and typed-child-flag repairs. **REJECT `10403c85`,
`7341986d`, and the superseded `6e369436`/`a59a59b` snapshots as winners.**
The first two are unrelated/under-implemented Issue 24 slices; the latter two
are ancestors of the accepted candidate, not competing end states.

## Identity and ancestry

The comparison base is public main
`758aa4417794c7a000e90f67c19e51f03817bdfd`. The public PR77 head is
`daffc5f50c306e09f7008b295262a7ccedab6cd3` (`fix: bound closeout locks and
process trees`). The exact local starting point was
`6e3694366473b8b656d931b9eb4fae2e03a4fe2e`; the current discoverable branch
head is `8dd07064357bbb1b922e1c4953d58ff0fbaaaf31`.

`git range-diff` proves the shared chain:

```text
3c4b62d = 3c4b62d  docs: record issue 24 closeout findings
9086418 = 9086418  feat(cli): add detached session closeout
daffc5f = daffc5f  fix: bound closeout locks and process trees
                      ee7db88  recover abandoned closeout leases safely
                      c94f56f  return captured closeout output
                      a808d03  check closeout temp cleanup
                      7b2ab4b  harden closeout ownership and timeouts
                      6e36943  cover closeout ownership failure paths
                      a59a59b  use bounded opaque closeout tokens
                      e8fa068  finalize opaque closeout ownership
                      8287622  make closeout ownership directory-scoped
                      8dd0706  type closeout child flag
```

`daffc5f` is an ancestor of `6e369436`, `a59a59b`, and `8dd0706`; the latter
is therefore the newest viable continuation rather than a duplicate patch.
The exact `daffc5f..8dd0706` stable patch ID is
`8b0a65bdea749410c0d7993673f4d3eb929ff787`. The full base-to-head patch ID is
`91ac968c947974ece46c4c410d1a2ae65aaedb79`.

## Candidate inventory

| candidate | base relation | delta from public base | patch ID / status |
|---|---|---:|---|
| `8dd0706` | current, descendant of `daffc5f` | 13 commits; `+800/-2` across 8 files | `91ac968c`; **ACCEPT** |
| `8287622` | immediate predecessor chain | 12 commits; `+799/-2` across 8 files | superseded by `8dd0706` |
| `a59a59b` | ancestor of accepted head | 10 commits; `+776/-2` across 8 files | superseded; not a winner |
| `6e369436` | assigned historical point | 9 commits; `+800/-2` across 8 files | superseded; not a winner |
| `daffc5f` | public PR77 head | 4 commits; `+543` across 8 files | process-tree repair, but crash red fixed later |
| `10403c85` | one commit from older `0421e794` | `+367/-384` across 22 files | **REJECT** partial recovery-only slice |
| `7341986d` | one commit from older `f73ee0e8` | `+509/-608` across 26 files | **REJECT** older broad/under-proven slice |

The requested newer exact branch was found at `worker/luna-issue24-root-20260828-a`
(`8dd0706`); no newer descendant was present after fetch/prune. Current-base
`git merge-tree` is clean for `daffc5f`, `6e369436`, `a59a59b`, `8dd0706`, and
`10403c85`. `7341986d` has a `FINDINGS.md` content conflict and is not a clean
current-base transplant.

## Mechanism and gate evidence

The accepted chain keeps distinct mechanisms for distinct contracts:

- `acquireCloseoutToken` uses an exclusive opaque lock held through child
  completion; the T122 64-caller gate observed one winner on `daffc5f`, while
  the old duplicate-token race observed three winners.
- Unix process-group termination and the Windows Job Object bound the tagger
  process tree. The exact descendant-holding-stdio focused gate passed on
  `daffc5f`; the direct-child-kill mutation hung until its external bound.
- The T96 crash interleaving against `daffc5f` observed the parent killed after
  lock acquisition, a stranded lock, and a retry falsely reporting “already
  queued”. The later `ee7db88` lease/reclaim chain is the direct repair for that
  red. No claim is made here that a fresh SIGKILL reproduction was independently
  rerun on `8dd0706`.
- On a clean temporary checkout of exact `8dd0706`, the observed gate
  `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Closeout'`
  passed in `3.233s`.
- Existing T96 mutation evidence remains relevant: on the earlier PR77 head,
  removal of closeout-specific detach, asynchronous wait, and child-start
  ordering survived; current-head mutations for error reporting and process
  tree termination were killed, while empty-array/config/pass-limit cases
  survived. The focused green is therefore not treated as a full mutation or
  full-repository gate. The requested full race suite was not observed green;
  an earlier run was stopped for unrelated long-running index/archive tests.

`10403c85` and `7341986d` only add a recovery command/manual tag-prep message
and do not contain the detached bounded closeout loop, lease reclaim,
process-tree cleanup, or current tests. Their clean/partial focused behavior
cannot substitute for the accepted chain. `7341986d` also carries report and
unrelated deletion churn, so it is not a minimal transplant.

## Net-line accounting

For the accepted candidate, the measured base-to-head aggregate is `+800/-2`
across eight files, including `+9` findings, `+2` README, `+1` CLI wiring,
`+97/-2` ingest/token support, `+19` Unix process support, `+28` Windows
process support, `+300` closeout production code, and `+344` closeout tests.
The supervisor-supplied current increment from `daffc5f` is `+312/-57` and
patch ID `8b0a65b…`; it is the relevant unique repair delta. No product edits
were made in this worktree.

## Final ruling

**ACCEPT `8dd0706` for harvest/integration as the only current same-base
winner.** Preserve its nine repair commits as one causal chain. Do not harvest
`10403c85` or `7341986d` independently, and do not treat `6e369436`,
`a59a59b`, or `8287622` as separate alternatives: they are historical points
already contained in the accepted ancestry.
