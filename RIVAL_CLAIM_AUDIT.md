# Rival claim audit

Audit date: 2026-08-27. Fixed comparison base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
This is a claim audit, not an adoption of unverified code. Branch existence, URL count,
report volume, and echoed worker receipts earn no score.

## Verification boundary

Graphify was run first against the immutable RawClaw graph. `reflect --if-stale` reported
25 graph memories and `LESSONS.md` was read. Literal queries around `catalog hook path range`
and `tag write durability publication tombstone pruning` located the relevant setup, catalog,
tag-publication, and consolidation seams. The graph has no nodes for some newer worker symbols
(`resolveSegmentRange`), so exact commit objects and diffs, not inferred graph edges, decide
claims. The worker worktree graph is absent; this is a branch-visibility limitation.

The fixed-base full gate was personally run and passed:

```text
CGO_ENABLED=0 go test -race -count=1 ./...
PASS (all packages; cli 80.486s, index 86.474s)
```

No rival branch-specific test or benchmark result was personally observed. `git show --check`
passed for the inspected rival code commits. Stable patch IDs and ancestry were checked from
local immutable refs.

## Han claims

| Object | Payload and observed facts | Verdict |
|---|---|---|
| `dd5457194f718dc2eb6ed14f46a3b8c00c2b9f69` / `HAN_PATH_CLAIM_PRIOR_ART.md` | 176 documentation lines, 0 production/test lines. It correctly separates POSIX `ln`, `mkdir`, and noclobber mechanisms, identifies the raw-ID temporary-directory defect in `bd8346c`, and changes the recommendation toward the safer `37ec96b` shape. No hostile test was run. | **ADOPT** — useful ordered prior-art/adoption constraint, but not safety proof or novelty credit. |
| `4e53e9cf400edcfb3a61149ba50d7ee82e9ec26` / original `HAN_HARNESS_AUDIT.md` | 143 documentation lines. Its headline says lifecycle is inactive, but later Han evidence corrected that conclusion; successor `34c8016` rewrites the report to acknowledge active mailbox blocking. | **STALE** — superseded by the pushed correction `34c8016`; the original inactive-lifecycle claim must not be used. |
| `418bfa78966e84677025e3c8600676f586cbd772` / `HAN_OZZY_HARVEST.md` | 102 documentation lines. It identifies harvest code duplicates and stale-lineage deletion, and orders unique research receipts. The branch successor `2b5416d` only removes a report EOF blank line; no production/test payload is added. | **ADOPT** — the fixed-base/patch-identity separation is useful research input; no product adoption or gate credit follows from the report. |
| `11bb89443f8dbfbf915a22bc22cc0af88f0bba18` / `HAN_CANDIDATE_STOMP.md` | 147 documentation lines. It correctly records `bd8346c` as descended from `61b7957`, `37ec96b` as older-lineage, and stable duplicate IDs for repeated payloads; it also says hostile execution is unrun. | **ADOPT** — ancestry contamination and duplicate-mechanism warnings change adoption decisions, while unrun safety remains uncertain. |
| `9cc6099cb7e461056e7f2e0f7f3a0b94fafe17d2` / `HAN_TICKER_ACTIVATION.md` | 30 documentation lines. The report is a receipt about ticker activation, but no independent scheduler/watchdog gate was observed in this audit. | **UNSUPPORTED** — exact live activation evidence is not present in the immutable payload inspected here. |

### Han strongest falsifiable challenge

Han’s strongest challenge is to prove that the corrected lifecycle claim is restartable rather
than merely observed in one registered session: provide the exact active dispatcher/configuration,
a fresh unread-mail denial, a Stop/SubagentStop patrol receipt, and a live ten-minute tick/watchdog
receipt. `34c8016` itself concedes the scheduler/watchdog is not observed; therefore a prose claim of
full launch-contract satisfaction is falsified by the missing durable cadence evidence.

## Ozzy claims

| Object | Payload and observed facts | Verdict |
|---|---|---|
| `78b6a4fe5a90771d9de7a1e3e83e0c046ed834a8` | `internal/cli/cmd_tag.go`, +1/-7. Stable patch ID is recorded by the harvest report as identical to Conor `fb893ed` and Norm `a317766`. | **DUPLICATE** — real one-line production shrink, but not a new adoption claim. |
| harvest candidates `847426c`, `539de03`, `b944d08`, `37ec96b` | The immutable harvest report records stable duplicate IDs or inherited/competing mechanism payloads. `37ec96b` has +32 production, +157 tests; it is older-lineage and its hostile tests were not run here. | **DUPLICATE** — mechanism-family or patch-equivalent duplication defeats novelty; fixed-base safety remains a separate unrun question. |
| Ozzy prior-art tip `a829b1b22aacc5ed64b5624d112e10eb27f67df3` and reported seed `00d1783` | Documentation-only research chain. The report distinguishes post-seed research from a stale product-tree delta and claims 90 canonical sources. | **ADOPT** — adopt only the ordered research constraints that alter implementation gates; URL quantity and report size score zero. |
| `7f139f0d3e8bd3cdb109febd857336f82a7da76a` (`ozzy/speed-proof-20260827`) | Adds 112 test lines claiming durable tag authoring before publication wait. It is a clean child of fixed base, but no branch-specific test was run. | **UNSUPPORTED** — test payload exists, yet “prove” is not personally observed evidence. |
| `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` (`ozzy/speed-prune-20260827`) | +76 production and +77 benchmark lines (net +76/+77 across files). It batches tombstone IDs through a temp table and changes the pruning query shape. No benchmark or full gate was run on this tip. | **UNSUPPORTED** — performance/adoption claim lacks observed benchmark evidence; fixed-base green tests do not test this payload. |
| `2ad239c776b143a3b18ca686ad1bcc5c908f735f` (`ozzy/speed-publish-20260827`) | +10 production, +30 tests; parent is current detached-publication commit `3170b19`, not fixed base directly. It normalizes path comparison and checks context cancellation, with narrow self-skip tests. `git show --check` passes; branch-specific tests unrun. | **UNSUPPORTED** — plausible targeted hardening, but no personally observed gate establishes behavior or adoption. |
| `cda693dc9b118de310dbb2f7d1cdecf86a5a0ef9` (`ozzy/pressure-luna-20260827`) | +137 shell production, +25 docs. It hardens persisted pressure-auditor state and mailbox receipt writes, but its own changes include many lifecycle branches and no observed live harness run here. | **UNSUPPORTED** — implementation payload is accessible, but live behavior and gates are unobserved. |

The full current Ozzy branch refs are present and pushed, but that fact is deliberately not a
verdict. The harvest report’s apparent large product delta is stale-lineage divergence, not a
payload against the fixed base.

### Ozzy strongest falsifiable challenge

Ozzy’s strongest current claim is the speed-prune performance/adoption claim `386ec9d`. Run its
branch-specific benchmark and fixed-base race suite, then compare against the pre-change loop on
the same seeded workload. Until that produces an observed delta and confirms all six sidecar/table
deletions, the `perf(index)` commit is **UNSUPPORTED**. A green fixed-base suite or a quoted worker
receipt cannot falsify this challenge because neither executes the changed code.

## Summary

- Han: prior-art and corrected candidate-ancestry research can be adopted as constraints; the
  original harness-inactive report is stale; ticker activation is unsupported.
- Ozzy: `78b6a4f` and the harvested code candidates are duplicates; `a829b1b`/`00d1783` research
  is adoptable only as ordered constraints; all three current speed-worker implementation claims
  remain unsupported without branch-specific gates.
- No claim earns credit for branch existence, URL counts, report volume, or quoted tests.

