# Tick 129 PR77 rival/public harvest

Date: 2026-08-28 WITA  
Scope: PR #77 closeout descendants, rival seats, and public-current evidence

## Decisive ruling

**HOLD current-main follow-up; no rival candidate is harvestable.** PR #77 is
already merged at public `main` `9ddacb19cc27355873f36ed7fbaa6208b34c0d03`
(merge of `8dd07064357bbb1b922e1c4953d58ff0fbaaaf31`). The only visible
post-merge PR77 candidate is Han's `ed9d12f5f019df143ccdb1e2a446f8fcca52a0c9`
on `worker/han-t98-pr77-crash-cas-20260828`; it is patch-unique against
public main (`f29e6c15de196cfd64e40597028ef254c970402a`, `+119/-0`) but is
**REJECT as ready to harvest** because the current-main claim-spy evidence
records two red gates: immediate retry after parent death before child start,
and exactly-one winner under concurrent stale takeover. Owner action: keep
`ed9d12f` isolated and repair both reds together; preserve prior reports.

No branch superseding `ed9d12f` was visible in local refs at this harvest.

## Graphify and memory orientation

Required graph source was `/private/tmp/rawclaw-han-t99-ed9d.Z0t0CD/src/graphify-out/graph.json`.
`graphify reflect --if-stale` completed with zero stored lessons. Literal
query `PR77 closeout process timeout candidate patch` located the closeout
token, process, timeout, and test nodes; `graphify explain runCloseoutTagger`
identified the tagger wrapper and its focused tests. Memory recall surfaced
the prior same-base selection of `8dd0706` and the later post-merge crash-CAS
red evidence; all claims below were checked against Git and visible reports.

## Public and candidate identity

```text
origin/main = 9ddacb19cc27355873f36ed7fbaa6208b34c0d03
PR77 head   = 8dd07064357bbb1b922e1c4953d58ff0fbaaaf31
post-merge  = ed9d12f5f019df143ccdb1e2a446f8fcca52a0c9
```

`git merge-base --is-ancestor ed9d12f 9ddacb1` exited 1; the inverse exited
0. Thus `ed9d12f` is strictly ahead of public main, not merged. Its diff is
two files (`internal/cli/bg_ingest.go`, `internal/cli/cmd_closeout_test.go`),
44 additions/1 deletion in production and 75 test additions. `git patch-id
--stable` for the commit emitted `f29e6c15de196cfd64e40597028ef254c970402a`.

## Rival-seat inventory and rulings

| seat | visible branch/report | evidence and base | ruling | owner-directed action |
|---|---|---|---|---|
| Furiosa | `worker/furiosa-t122-pr77-parent-20260828@cb045a7`; `worker/furiosa-t122-pr77-duplicate-20260828@f2d49c9` | process-tree and duplicate-token mutation reports against pre-merge `daffc5f`; report-only, not current-main candidates | **REJECT as harvest**; preserve as historical corroboration | do not cherry-pick; retain mutation evidence in the current-main repair review |
| Ozzy | no PR77 current-base branch or report visible; `supervisor/ozzy-isolated-20260828@029f60d` is Issue #40 instant tag-write work | unrelated issue/base, no PR77 closeout identity or gate | **REJECT** | no harvest; request a current-main PR77 SHA only if Ozzy claims one |
| Rabbit | no PR77 current-base branch or report visible; `rabbit/supervisor-tick63-20260828@029f60d` is Issue #40 | unrelated issue/base and older public state | **REJECT** | no harvest; terminate/close only after supervisor confirms no separate owned work |
| Norm | no PR77 candidate/report visible; `supervisor/norm-issue-audit-20260828@f73ee0e` is an Issue #46 closeout-contract document | unrelated issue/base; no process-tree or crash-CAS proof | **REJECT** | no PR77 harvest; preserve as non-candidate context |
| Khan | `khan/feat-issue24-closeout-narrowed@7341986d`; older `10403c85` recovery branch | partial/older Issue #24 slices; lack bounded process-tree, ownership, and current-main repair chain | **REJECT** | do not harvest independently; these are superseded historical alternatives |

The visible live-process inventory showed Furiosa/Ozzy/Nom/Rabbit Codex
processes or relays, but process presence is not a branch or gate receipt.
Khan had no distinct live Codex process in the inspected process listing.

## Evidence for the post-merge candidate

The visible `T98_PR77_8DD_CLAIM_SPY.md` report independently records:

- Current public main `9ddacb1` is byte-equivalent to merged PR77 head `8dd0706`.
- A real parent-death interleaving before child `Start` leaves the token and
  causes an immediate retry to report `closeout already queued` with zero
  launches. **REJECT gate.**
- A 32-way stale-takeover race produces 3, 4, or 5 winners in repeated
  `-race -count=10` iterations. **REJECT gate.**
- The `ed9d12f` repair adds PID metadata and an in-process mutex, but it has
  not earned a current-main acceptance receipt; the red behavior is the
  reason it remains on HOLD.

## Visible process/seat and cleanliness metadata

Observed isolated supervisor processes included Furiosa PID 3871, Ozzy PID
70622, and Rabbit PID 55898; Norm's timer/relay was visible. These are
presence observations only. The tracked candidate refs above were clean at
inspection; no dirty or upstream-diverged PR77 candidate was found that could
supersede `ed9d12f`.

## Final owner-directed harvest

1. Preserve `8dd0706` and all T97 reports as historical evidence, not as a
   new candidate.
2. Keep `ed9d12f` as the sole patch-unique post-merge repair candidate, but
   **HOLD** it until both crash-before-start retry and concurrent stale-CAS
   exactly-one-winner gates are green on current public main.
3. Do not harvest any Furiosa, Ozzy, Rabbit, Norm, or Khan branch listed above
   as a PR77 winner; each is absent, unrelated, stale, report-only, or
   independently rebutted.

