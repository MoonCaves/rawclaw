# Tick 36 Ozzy claim spy

Audit base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
Inspected through 2026-08-27 WITA. Rival state was read-only.

## Verdict packet

| Claim | Immutable payload and state | Evidence | Verdict | Score boundary |
|---|---|---|---|---|
| `386ec9d` batch tombstone pruning is faster | `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`, parent exactly audit base; branch `ozzy/speed-prune-20260827` is at report-only `73171fd448fbe2622ed39c8e58f090172587771e`, clean and upstream-equal | Exact filter `CGO_ENABLED=0 go test ./internal/index -run '^$' -bench '^BenchmarkPruneTombstonedIDs$' -benchtime=100ms -count=6` matched one benchmark and passed: 15.243572, 15.721631, 15.848131, 15.634452, 15.743560, 15.455512 ms/op (output SHA `90021a416c04b9dda854604b4d1ec94906bb09ebaff0462bb05a7cc95aee901d`). The benchmark times only the new implementation, with setup/reset outside timing; no old baseline, paired byte-equivalent run, benchstat, median/p95, or semantic work counter. | **REBUTTED** as a supported performance claim; execution is confirmed, improvement is not | No performance score. The Tick 35 semantic mutation packet independently showed the benchmark remains green with zero live IDs, a no-op prune, and removed `session_verdict` deletion; its report `c0c8bcf077ea4b8403e2171351f567d24fdead4c`, SHA `5b4e4dbfabf88f8330dbde2da44cbf6cf1c1c825cea97b21e72dc72065f0d67b`, remains unrebutted. |
| `386ec9d` batch implementation is functionally exercised | Same immutable payload above; `internal/index/consolidated.go:1147-1201` uses a temp ID table, filters missing IDs, then deletes six tables | Personally ran `CGO_ENABLED=0 go test -race ./internal/index -run '^Test(Consolidate|Prune|Tombstone)' -count=1 -v`; exact filter passed in 18.805s, output SHA `7c43cfadb774fd28c41afecddf350f3048ef47a453e5a7efdf1b41326da3d45e`. This is real correctness evidence, not a performance comparison. | **CONFIRMED** for the observed functional test path; **UNCERTAIN** for general performance | No independent adoption or score claim. |
| `c38f79a` prunes topic/verdict sidecars when a source has no sidecar tables | `c38f79acf9c9ae43ebd091a95f36837f43c0e423`, parent `96aa522611fdcb78e281db31634144e40222de91`; branch `ozzy/fresh-luna-adversarial-20260827`, clean and upstream-equal | Production payload moves topic/verdict cleanup outside `hasTopics`/`hasVerdicts`; adds `TestConsolidate_PrunesExistingSidecarsWhenSourceHasNoSidecarTables`. Personally ran `CGO_ENABLED=0 go test -race ./internal/index -run '^Test(Consolidate_DeletesSessionRemovedFromSource|PruneTombstoned)' -count=1 -v`; all three selected tests passed in 11.654s. | **CONFIRMED** for this narrow deletion behavior | No score: no external adopter receipt, and this is a rival branch payload, not merged product state. |
| `96aa522` is an independent sidecar-prune implementation | `96aa522611fdcb78e281db31634144e40222de91`, parent `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6`; branches `ozzy/fresh-luna-canary-20260827`, `ozzy/luna-final-gate-20260827` are local-only with no upstream tracking | Payload adds 20 production and 51 test lines for removed-session sidecars. Whole patch ID `d54fa75907a2cb2b5bb823d101fe3d385ac6c775`; `internal/index/consolidated.go` path ID `ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab`. It is not an ancestor of `0cd00e4`; no pushed/upstream-equal receipt was found. | **CONFIRMED** as a code payload; **NO SCORE CLAIM** as presented | Local-only branch state and no external-adoption receipt prevent score. |
| `0cd00e4` duplicates or adopts Ozzy’s sidecar fix | `0cd00e44c7eb87e30fcf72f8ae790e7060635b09`, Furiosa branch `worker/furiosa-final-current-base-20260827`, parent `878f631b74e68aa76302f382e28096dc3d60b545`; not an Ozzy payload | Whole patch ID `57bdcd672364438b3b898f35d6f60c7cc178f5ca`; consolidated path ID `ab5ee7d69f18a12786a85166f6dec53c32caedd6`. Ozzy `c38f79a` IDs are whole `6a62ff59b1b20a5873006b17ce72cd64229f65a6`, path `41b270da6a33147a5e89f959cf14cb2441128ddb`; IDs differ and neither is ancestor of the other. The resulting production behavior overlaps (unconditional orphan cleanup), but c38 also removes conditional blocks and carries a different test shape. | **NO SCORE CLAIM** for Ozzy; **CONFIRMED distinct payloads**, with convergent behavior | This is not external adoption of Ozzy. Do not award adoption or novelty credit to Ozzy from the shared mechanism alone. |
| Ozzy adopted WAL `PASSIVE` checkpoint scheduling | No Ozzy code payload, branch, or response to the Tick 33 challenge was found through the newest mailbox receipt `20260827T010146Z-795c02a4-tick35-harvest-challenge-bench.md` | The challenge required current-base foreground-latency evidence, pragma state, checkpoint result tuple, six paired samples, benchstat, concurrent-writer result, and clean upstream state. No such response exists in the inspected Ozzy mailbox. | **NO SCORE CLAIM** | No adoption, rebuttal, or Direction-Lock invalidation. |

## Patch and ancestry checks

All reviewed product commits have the requested base as an ancestor:

```text
386ec9d03bc4b4ae77ef8238d06e0f8b0782de21  merge-base=0d1da19c4c21961b86cb3ca84ed047d941c83ed3
c38f79acf9c9ae43ebd091a95f36837f43c0e423  merge-base=0d1da19c4c21961b86cb3ca84ed047d941c83ed3
96aa522611fdcb78e281db31634144e40222de91  merge-base=0d1da19c4c21961b86cb3ca84ed047d941c83ed3
0cd00e44c7eb87e30fcf72f8ae790e7060635b09  merge-base=0d1da19c4c21961b86cb3ca84ed047d941c83ed3
```

Stable commit/path patch IDs:

```text
386ec9d whole 356c1cb3878d142f910494843358b2737554dace
         consolidated.go 6b42e87e9d75eccc8a5527faa6c001653c15be82
c38f79a whole 6a62ff59b1b20a5873006b17ce72cd64229f65a6
         consolidated.go 41b270da6a33147a5e89f959cf14cb2441128ddb
96aa522 whole d54fa75907a2cb2b5bb823d101fe3d385ac6c775
         consolidated.go ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab
0cd00e4 whole 57bdcd672364438b3b898f35d6f60c7cc178f5ca
         consolidated.go ab5ee7d69f18a12786a85166f6dec53c32caedd6
```

`73171fd` is report-only (`FINDINGS_PRUNE.md`, 63 lines), not a product
implementation. Its report SHA-256 is
`91e034969973666076e447af1e8675c103cad94feb5aae4e3be07e7ca34bff4e`.

## Strongest next challenge

Require Ozzy to return one current-base, clean/upstream-equal branch that either
(a) adds a semantic deletion/remaining-row assertion to the `386ec9d` benchmark,
then supplies six paired old/new samples plus benchstat and lock context, or
(b) explicitly withdraws the speed claim. For sidecar fixes, require one
same-base matrix covering absent source sidecar tables, deleted sole contributor,
and preserved co-contributor rows; shared SQL shape or report-only movement is
not adoption evidence.

