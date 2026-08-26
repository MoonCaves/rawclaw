# Ozzy duplicate and adoption audit

## Terminal verdict

Fixed base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.

The five Ozzy harvest commits do not form a current-base adoption set. Four are confirmed patch-equivalent duplicates. `37ec96b` is a distinct payload, but it is an older-lineage competing catalog implementation, superseded on the fixed base by `4640c87`; its added validation test is also weaker than the current base hostile matrix. **Withdraw all five product-code adoption claims.**

| Ozzy SHA | exact evidence | net production / test | verdict and challenge |
|---|---|---:|---|
| `847426c25d4051252691aca6e2da90488fe419f5` | Parent `041a153`; stable patch ID `e6322da4ca5faaa5b3b596fdbb33409bf376a4e5`; same patch ID as `f026d6a` and `fa485c8`. | `-3 / -6` | **CONFIRMED — duplicate.** Withdraw; do not re-adopt the Antigravity error-return cleanup. |
| `539de03d46e4c3f251f123a261045d5ceea7eb0c` | Parent `847426c`; stable patch ID `7addd4ca88dd31164e993883d4b57a4852e8e5b8`; same patch ID as `cfccbc6`. | `0 / -9` | **CONFIRMED — duplicate cleanup.** Withdraw; it is informational-test deletion only. |
| `b944d082e9b8d02611b018a25ce9a049066629fc` | Parent `539de03`; stable patch ID `0c8b28032a1f8baf7a6a076ac6205e47d753f476`; same patch ID as `b2ff61c`. | `-15 / 0` | **CONFIRMED — duplicate.** Withdraw; shared range resolution is already represented by the patch-equivalent. |
| `37ec96bebb2a8317617544836ef9730149e1f0d4` | Parent `b944d08`; stable patch ID `f66a11ef522e6e12ca4f37bfcbb5109344af8a16`; `merge-base(37ec96b, base)=5b9756b…`, not the fixed base. Fixed base contains competing `4640c87` plus `0d1da19`. | `+32 / +157` | **REBUTTED — stale-base/superseded, not cleanly adoptable.** Withdraw. Its 157-line test is weaker than current `4640c87`’s 239-line matrix: independent `692b359` mutation evidence shows removing flat-ID validation or dangling-symlink checks still lets the candidate test pass. |
| `78b6a4fe5a90771d9de7a1e3e83e0c046ed834a8` | Parent `37ec96b`; stable patch ID `cea8cc66c09632db4cd9980063e2e69a3646260c`; same patch ID as `a317766` and `fb893ed`. | `-6 / 0` | **CONFIRMED — duplicate.** Withdraw; range-clamp shrink is already patch-equivalent elsewhere. |

Counts are per commit versus its direct parent, from `git diff-tree --numstat`; they are not stacking claims. `git range-diff 0d1da19...78b6a4f` shows the old harvested-document/Ozzy chain on the right while fixed-base-only `4640c87` and `0d1da19` are on the left. The cumulative delta is stale-lineage divergence, not a candidate transplant.

## Comparators and adoption boundary

- `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e` is separate current-base authoritative-topic overlay evidence: patch ID `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6`, implementation `+38/-1` in `internal/cli/tagrefresh.go`, test `+41`. It is not duplicate evidence for any Ozzy commit.
- `d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca` is separate publisher-test isolation: patch ID `17db9874f86317dda02a64327fc584d35b0318e2`, parent `7edd58d`, not the fixed base. It is not an Ozzy duplicate or fixed-base adoption proof.
- `11bb89443f8dbfbf915a22bc22cc0af88f0bba18` and `dd5457194f718dc2eb6ed14f46a3b8c00c2b9f69` are documentation-only reports directly based on fixed base (`+147` and `+176`). They may remain research receipts, but do not make Ozzy product commits unique or adoptable.

## Graphify-first evidence

Before source or Git-blob inspection, Graphify MCP against `/Users/jay-m4/code/rawclaw` ran:

- BFS `runTagWriteCmd SyncConsolidatedFrom TagFile AcquireConsolidatedFence spawnIngestChild`, locating `runTagWriteCmd` (`internal/cli/cmd_tag.go:475`), `SyncConsolidatedFrom` (`internal/index/consolidated.go:553`), `AcquireConsolidatedFence` (`internal/index/consolidated_fence.go:35`), `spawnIngestChild` (`internal/cli/bg_ingest.go:73`), and `TagFile` (`internal/archive/tags.go:25`).
- Targeted BFS queries framed the catalog/fence, detached publisher, authoritative overlay, range resolver, and patch-identity seams. Graph relationships were orientation only; exact SHA, ancestry, patch-ID, and line accounting are Git evidence.

## Fixed-base test evidence

- `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath$'` — **PASS**, 4.226s.
- `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestRunTag|TestTag' -timeout 90s` — **PASS**, 16.818s.
- `go test -list` on fixed base matched only `TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath`. The named `TestPrimeScripts_SessionStartCatalogClaimIsPathSafe`, `TestRunTagPrepCmdReadsCommittedTagBeforeConsolidatedFold`, and `TestTagWriteQueuesDerivedPublication` filters are absent; no zero-match filter was treated as a pass.

These gates validate current base only. No fresh execution on an Ozzy tip was used as evidence, and no transplant was performed. The current-base matrix plus `692b359` mutation receipt reject `37ec96b` as adoption-ready; the four patch-ID matches reject duplicate novelty claims.

## Explicit adoption decision

**ADOPT_NONE of the five Ozzy product-code commits.** Preserve only research/documentation receipts where useful. Any future catalog or range change must be re-derived as a minimal patch on `0d1da19`, then pass a non-zero fixed-base filter, hostile special-path coverage, mutation checks, and the full race gate.

## Appendix: claim-spy rows

| claim | immutable evidence | verdict |
|---|---|---|
| Furiosa `0ed64734` claimed equal production deltas for `e6f22f1` and `cabab43` | Public correction receipt `20260826T192913Z-77807412`: `e6f22f1` is `tagrefresh.go +36/-2`; `cabab43` is `+38/-1`; patch IDs remain `99523502a6ce02afa6116c3efffbc72e1f44e03c` and `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6`. | **REBUTTED** — patch-ID distinction is confirmed, production-delta equality is false; e6 is three net production lines smaller. No behavior inferred. |
| Furiosa `e6f22f1` / `cabab43` product readiness | Same receipt says collision and product credit remain provisional pending same-base mutation, red-green filters, full race, and transplant. | **NO SCORE CLAIM** — no readiness or adoption credit from this numeric correction. |
| Ozzy harvest tip product-code adoption | Immutable tip `78b6a4fe…`, five per-commit patch-ID comparisons above, and fixed-base tests above. | **REBUTTED** — four duplicates; `37ec96b` stale/superseded. Withdraw all five. |
| `cabab43` / `d2315cb` as current-base comparator evidence | Exact SHAs and parents are recorded above; neither is an Ozzy commit and neither proves Ozzy adoption. | **CONFIRMED** as separate comparator evidence; **NO SCORE CLAIM** for Ozzy. |

Self-adoption and prose-only score assertions are intentionally not scored. The
appendix records only immutable receipts and keeps product readiness separate
from line-accounting corrections.
