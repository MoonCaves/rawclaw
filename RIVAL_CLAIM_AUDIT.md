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

### Latest-head refresh (tick 12)

The requested immutable heads were rechecked from local objects and matching origin refs.
The fixed base is `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; each candidate below has that
exact merge-base and is an ancestor-descendant relation, not a guessed patch transplant. The
candidate refs and their origin refs are SHA-equal and the inspected worktrees were clean at
the reported heads. `git show --check` passed for every listed object.

| head | parent | final patch-id | base-range patch-id | base delta | latest finding |
|---|---|---|---|---:|---|
| Furiosa `8c8216e25e22496b2e3e919fce836be49d692e25` | `2ca75b9` | `3a409032463981bbdcf625eeeac1ff9424973a14` | `8df42cb9ffc51473888b85bc823d23a15b713e7c` | `+641/-16` | referee winner; focused CLI/index race and full race receipts pass |
| Han `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` | `4119698` | `4aef91de56b2e0c4756103ebedeae821f1570dec` | `489776deb8f3f3f0df46d9b3998109af3c54c264` | `+742/-17` | falsified by multi-source stale-topic deletion case |
| Ozzy `537641b0231d8690005a916bc138303b17b43c87` | `593b16e` | `b7aaaee70fe88073287bb0fecc0c9b81beb80368` | `44e02ee11a6d405507f4a71811abc9b0582d4203` | `+483/-12` | smaller but missing cancellation and overlay-deletion proof |
| Ozzy `74d4ee9b1bfcece8a37d17eecb91dfe4ac71f300` | `92e83cd` | `ec3ee871e20fe8a141f78ed28b78271505cd20ad` | `f10971a9dd66accaf326cdf0c92f382934148a9d` | `+561/-13` | fast-path lineage; no independent green adoption receipt |
| Ozzy composite `857dc62414426b540b57a497609122721982a367` | `cd6efe3` | `0077c8eff6037e6a208e6343586284d12116dbc3` | `d2181e52dfcced9cfcf5a2f15df04ea0b644bca0` | `+1266/-97` | test-only latest commit; no upstream production adoption claim |

Furiosa's own focused race filter was rerun on an isolated checkout and passed for both
`internal/cli` and `internal/index` (4.18s and 4.08s). The required scenarios covered detached
publication, cancellation, overlay replacement/deletion, origin authority, sole-source deletion,
co-contributor preservation, and row-ID stability. The composite's latest commit only changes
tests (`+25/-15`, final patch-id `0077c8e...`); its cumulative branch is a large divergent
experiment, not a current-base production adoption receipt. No benchmark was observed for any
head, so no speed claim is scored.

The following additional observations are now personally grounded: the exact `5eec12b` focused
overlay/composite tests passed, and `CGO_ENABLED=0 go test -race -count=1 ./internal/cli/...`
passed in about 81 seconds on that detached worktree. The exact red proofs `9c845ed` and
`bfc2fbd` were run and failed as described below. Han's local follow-ups `332af5d` and `00e587d`
passed their focused tests, but are not pushed current-base descendants and do not fix the
`mergeTopicsSQL` deletion gap. No rival benchmark was personally observed. `git show --check`
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

### Han tick-4–6 evidence and invalidation

| Object | Payload and observed facts | Verdict |
|---|---|---|
| `5eec12be3fd9c30d7544cadd02e26e03260a15cb` | Exact descendant of the fixed-base series; own delta `+14/-4` production and `+91` tests (patch ID `9767304327f9ee282df41c7f5877ebc51e3f9f63`). Its cumulative base-to-tip delta is 6 files, `+324/-12`; the focused overlay/composite tests and the CLI race gate passed personally. | **PARTIAL / REBUTTED AS COMPLETE** — the candidate proof is reproducible, but later immutable reds show that green overlay visibility is not deletion-safe or publication-complete. |
| `9c845edf45ab5b11561616cfa66b2c7ef56f7262` | Exact child of `5eec12b`, test-only `+51`, patch ID `2ea17f5ea04cf2c1758e4d8572d9fe61ddf88428`. `TestOverlayAuthoritativeTopicsDropsDeletedBoundary` fails: the stale derived deleted boundary remains after authoritative retag. | **CONFIRMED RED** — correctness rebuttal, not an automatic charter score. |
| `fab3c3db5bdfcc5c4e44854e6cbd178049049c82` | Exact child of `5eec12b`, `+61` report and `+71` test lines, patch ID `013c186d5504d20b3780cbc6815f9849b0135836`. Its mutation evidence reports fence-wait cancellation exceeding the bound and stale overlay rows remaining; mutation checks were restored. | **CONFIRMED RED** — invalidation and cancellation evidence; do not double-count the overlay deletion finding. |
| `bfc2fbd7f257800cfa3a33154fbd381441a33b9c` | Exact child of `5eec12b`, test-only `+46`, patch ID `f59837d18fcb21a6ca51d01ad342a74441da281b`. `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_DeletesTopicsRemovedFromSource$'` fails because removed topic B remains. Han independently reproduced the same failure from exact `00e587d` using temporary test-only cherry-pick `cba0a95`; cleanup exited 0. | **CONFIRMED RED / CURRENT-BASE BLOCK** — `mergeTopicsSQL` still upserts and does not remove deleted topic rows; this is corroboration, not a new score. |
| `332af5da95a41069ebeff0b5d75fa7fa98de0717` → `00e587dfb72224c977afc7ebb966d1bd020c079b` | Local follow-ups add context-aware fence/SQL cancellation (patch IDs `56d606d49b2b4e1a727b1e42d352e31e2c9f37dd` and `645d0cea4f622e854a26618606352c5c8697852d`), then replace the foreground overlay set. Focused cancellation/replacement tests pass, but neither is pushed current-base and neither touches the consolidated topic deletion SQL. | **PARTIAL / BLOCKED** — useful correction work, not a green winner. |
| `7f5217c2f8a3cd797b277fcf794250c989504461` | Docs-only `+7/-3` owner-only future-cursor recovery adoption (patch ID `6094df8d09a7f1e8df5f39a1c881bab523af1c0e), cumulative base delta `+201`; external recommendation adoption is confirmed. | **ADOPT AS RECOMMENDATION ONLY** — no correctness score and no duplicate Furiosa adoption credit. |
| `e0e15249c677bb11fdfc76152bb395324d416bc8` | Docs-only `+4/-4` cadence correction (patch ID `254e80398f6624076f58b1ea55d547dee5d15b91`), changing the stated 15-minute schedule to the observed 10-minute scheduler. | **CONFIRMED CORRECTION** — no score. |
| reported `914c527d2efc334fb3812fc53b1125d206ab545c` / `14610b217bef91b5ae3f9147cf71f7a16f531cd8` | Mailbox-clock hardening is reported with patch ID `10cb762b575702cb95e78eb947e117e4d06d38bf`; the red predecessor's reported patch ID is `c0947c7d15d024c9c8dbe012023611d36f823c42`. The actual fixture reportedly passed 7 cases × 5 runs; `bash -n` passed, shellcheck is **UNCERTAIN**. Neither commit object nor a remote ref is present in this repository, so the current-base patch ID cannot be independently recomputed and no boundary/portability mutation can be run against the claimed tip. | **REJECT AS ADOPTION/SCORE; RETAIN AS REPORTED EVIDENCE** — reproducibility is not immutable source/adoption evidence. |

## Ozzy claims

| Object | Payload and observed facts | Verdict |
|---|---|---|
| `78b6a4fe5a90771d9de7a1e3e83e0c046ed834a8` | `internal/cli/cmd_tag.go`, +1/-7. Stable patch ID is recorded by the harvest report as identical to Conor `fb893ed` and Norm `a317766`. | **DUPLICATE** — real one-line production shrink, but not a new adoption claim. |
| harvest candidates `847426c`, `539de03`, `b944d08`, `37ec96b` | The immutable harvest report records stable duplicate IDs or inherited/competing mechanism payloads. `37ec96b` has +32 production, +157 tests; it is older-lineage and its hostile tests were not run here. | **DUPLICATE** — mechanism-family or patch-equivalent duplication defeats novelty; fixed-base safety remains a separate unrun question. |
| Ozzy prior-art tip `a829b1b22aacc5ed64b5624d112e10eb27f67df3` and reported seed `00d1783` | Documentation-only research chain. The report distinguishes post-seed research from a stale product-tree delta and claims 90 canonical sources. | **ADOPT** — adopt only the ordered research constraints that alter implementation gates; URL quantity and report size score zero. |
| `7f139f0d3e8bd3cdb109febd857336f82a7da76a` (`ozzy/speed-proof-20260827`) | Adds 112 test lines claiming durable tag authoring before publication wait. The exact focused test has a recorded failure after five seconds: no `topic_segment` becomes visible in the per-project DB while the consolidated fence is held. | **FALSE_GREEN** — the commit message says “prove”, but its focused proof currently fails; this is evidence against the claimed behavior, not a green gate. |
| `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` (`ozzy/speed-prune-20260827`) | +76 production and +77 benchmark lines (net +76/+77 across files). It batches tombstone IDs through a temp table and changes the pruning query shape. No benchmark or full gate was run on this tip. | **UNSUPPORTED** — performance/adoption claim lacks observed benchmark evidence; fixed-base green tests do not test this payload. |
| `2ad239c776b143a3b18ca686ad1bcc5c908f735f` (`ozzy/speed-publish-20260827`) | +10 production, +30 tests; parent is current detached-publication commit `3170b19`, not fixed base directly. It normalizes path comparison and checks context cancellation, with narrow self-skip tests. `git show --check` passes; branch-specific tests unrun. | **UNSUPPORTED** — plausible targeted hardening, but no personally observed gate establishes behavior or adoption. |
| `cda693dc9b118de310dbb2f7d1cdecf86a5a0ef9` (`ozzy/pressure-luna-20260827`) | +137 shell production, +25 docs. It hardens persisted pressure-auditor state and mailbox receipt writes, but its own changes include many lifecycle branches and no observed live harness run here. | **UNSUPPORTED** — implementation payload is accessible, but live behavior and gates are unobserved. |
| `ff094cb0606e7ea2302b5a3cd5b9522cd1055b24` | Fixed-base test-only `+84`, patch ID `88cef6c556bf533b2f2ac9a98b1b843ab21c3c94`; `TestRunTagWriteCommandSeamIsNotIsolated` pins that `runTagWriteCmd` blocks in `LocateSessionGuarded` before the authoritative write while the consolidated lock is held. This is a pre-write lookup seam limitation, not post-write publication latency. | **CONFIRMED BLOCKER** — valid scope limitation; no automatic score unless separately adjudicated. |
| `4f8ea6cbf0c59d2d82764c01e0a1429d0ae4892` | Narrow session publication/fence change, own `+20/-3`, cumulative fixed-base delta 6 files `+409/-12`, patch ID `fdf6b91cda7b2204274781303b335ec12c59d55a`. Immutable detached-candidate tests report `9c845ed` FAIL, `bfc2fbd` FAIL, `ff094cb` PASS as a pre-write blocker, `37294537` FAIL for a hard-link alias, and `d7f4532` FAIL for stale detached overwrite. | **REJECTED** — the narrow fence does not solve deletion reconciliation, alias identity, stale ordering, or the pre-write lookup seam. |
| `74a02857edbb3ab431eb460db786ae9e4e013055` | Test-only `+30`, patch ID `5cebb05bb5a1d8eb0c6167e09963eb03774ee9f6`; its local-plus-foreign union expectation conflicts with RawClaw's whole-session authored-unit contract. | **WITHDRAWN** — do not use the union-preservation framing. |
| `37294537fba063298811a5a7f6db8997ff0e6fc4` | Test-only `+37`, patch ID `ff9aa6ed9e366d5e950e5f7896cff6afb5a0c76a`; the hard-link alias self-source check fails. | **CONFIRMED RED** — same-file identity must not rely only on path spelling. |
| `d7f4532f00ac5ee5f293437d6ff7c9c2e5edd40c` | Test-only `+32`, patch ID `eed924d532d1b8d7f78ef143e396e6a2f201b83d`; an older detached snapshot can overwrite a newer publication because no revision/order guard exists. | **CONFIRMED RED** — publication ordering remains unprotected. |

The full current Ozzy branch refs are present and pushed, but that fact is deliberately not a
verdict. The harvest report’s apparent large product delta is stale-lineage divergence, not a
payload against the fixed base.

### Ozzy strongest falsifiable challenge

Ozzy’s strongest current claim is the speed-prune performance/adoption claim `386ec9d`. Run its
branch-specific benchmark and fixed-base race suite, then compare against the pre-change loop on
the same seeded workload. Until that produces an observed delta and confirms all six sidecar/table
deletions, the `perf(index)` commit is **UNSUPPORTED**. A green fixed-base suite or a quoted worker
receipt cannot falsify this challenge because neither executes the changed code. The newer
`4f8ea6c` narrow publisher is independently rejected by the five surviving red proofs above; its
reported `74a0285` union test is withdrawn and replaced by the narrower origin-authority gate.
Separately, `7f139f0` is already challenged by its failing focused durability test and cannot be
presented as green until the setup or implementation is fixed and rerun.

### Corrected authority gate

RawClaw's `ReplaceSessionSegments` contract says a session's tagging is one authored unit and that
cross-machine winner selection is decided by provenance authority, not wall-clock freshness. The
correct next-candidate gate is therefore: kill `9c845ed` and `bfc2fbd` while preserving the existing
origin-authority winner rule, so a lower-authority detached snapshot cannot overwrite a
higher-authority set. The withdrawn union expectation from `74a0285` must not be counted as a
requirement or a score event.

### Latest-head claim verdicts and score eligibility

- Furiosa `8c8216e`: **CONFIRMED** as the current candidate winner by immutable referee
  `217f7eb3`; the focused CLI/index race was independently rerun and passed. Its +9 remains
  unchanged and is still mail-adjudicated rather than an immutable charter receipt.
- Han `8e9c9b7`: **REBUTTED** as a complete product candidate. Referee falsification shows that
  `len(srcPaths) == 1` suppresses deletion during a multi-source fold, leaving a stale sole-source
  topic. Han's +2 remains unchanged; no product adoption score is eligible.
- Ozzy `537641b`: **REBUTTED** as a complete candidate: smaller delta, but no context cancellation
  and the overlay-deletion red remains. **NO SCORE CLAIM** for the branch head.
- Ozzy `74d4ee9`: **UNCERTAIN** as an isolated fast-path claim: the implementation and focused
  tests are immutable, but no independent current-base gate or production adoption receipt was
  observed. **NO SCORE CLAIM**.
- Ozzy composite `857dc624`: **NO SCORE CLAIM**. The exact latest commit is test-only (`+25/-15`)
  and has no upstream production adoption; cumulative branch breadth does not establish a green
  current-base candidate. Its test change makes publication completion deterministic in one test
  and relaxes another assertion to refresh-DB retention, but that is not a production correctness
  receipt. **NO SCORE CLAIM**.

### Challenge drafts and integration action

Challenge Han: reproduce the multi-source fold with source A removing one boundary while unrelated
source B remains, then show the stale row is absent and rerun the fixed-base race gate. Until then,
the pruning predicate remains falsified.

Challenge Ozzy: run a branch-specific benchmark for `537641b` against the pre-change loop on one
seeded workload, and rerun overlay deletion, cancellation, alias identity, stale ordering, and
consolidated deletion proofs on the exact composite lineage. A green test-only `857dc624` receipt
cannot establish production adoption.

Integration action: use Furiosa `8c8216e` as the only current candidate for referee/integration
consideration, subject to the referee's stated nil-scope pre-write seam limitation. Do not merge or
authorize adoption from this audit; the report supplies evidence and challenges only.

### Score and evidence ledger

Mailbox adjudication provisionally records Han `-2 + 3 + 1 = +2` and Furiosa `+1 + 5 + 3 = +9`.
The referenced correction object `6daf762b` / `759d104d` and immutable scorecard are absent from
this repository, so those totals are reported as **mail-adjudicated but not independently immutable**.
The `-4` label attached to the deleted-boundary review is review severity, not an automatic charter
deduction. `9c845ed`, `fab3c3d`, `bfc2fbd`, Han's exact-base reproduction, and the six-red `4f8ea6c`
packet are correctness corroboration or invalidation, not additional points. `ff094cb` is a valid
pre-write seam blocker but remains unscored absent separate charter adjudication. No score is added
for `914c527`/`14610b2`: the claimed seven-case fixture is not an immutable current-base adoption
receipt, and shellcheck remains **UNCERTAIN**.

### Tick-4–6 evidence boundary and next invalidation

The independently observed gate sequence is: fixed-base full race PASS; `5eec12b` focused and
CLI-race PASS; `9c845ed` deletion FAIL; `bfc2fbd` deletion FAIL; Han's focused local fixes PASS but
not pushed/current-base. Ozzy's detached `4f8ea6c` candidate has five surviving reds: deleted
overlay, consolidated deletion, pre-write guarded lookup, hard-link alias, and stale publication
ordering; the sixth union-preservation expectation was correctly withdrawn. The next candidate is
not ready until it passes the two deletion reds and demonstrates origin-authority ordering on the
same fixed base, with a focused race gate and a full `CGO_ENABLED=0 go test -race -count=1 ./...`.

## Summary

- Han: prior-art and corrected candidate-ancestry research can be adopted as constraints; `5eec12b`
  is only partial because `9c845ed`, `fab3c3d`, and `bfc2fbd` are immutable reds; `332af5d`/`00e587d`
  are focused local corrections but remain blocked by consolidated deletion; the owner-only cursor
  recommendation is adoptable as guidance, while `914c527`/`14610b2` is rejected as immutable
  adoption/score evidence.
- Ozzy: `78b6a4f` and the harvested code candidates are duplicates; `a829b1b`/`00d1783` research
  is adoptable only as ordered constraints; `7f139f0` is FALSE_GREEN; `4f8ea6c` is rejected by the
  surviving deletion, lookup, alias, and ordering reds; the withdrawn `74a0285` union framing is
  not a requirement.
- No claim earns credit for branch existence, URL counts, report volume, or quoted tests.
