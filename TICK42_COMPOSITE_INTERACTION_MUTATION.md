# Tick 42 composite interaction referee

Verdict: **REBUTTED as an automatically composable pair; no merge authorization.**

## Identity and payloads

- Exact base: `ef2eebf414e77086be06281539c5a50ba036a32a`.
- Sidecar adaptation A: `0cd00e44c7eb87e30fcf72f8ae790e7060635b09`, direct parent `878f631b74e68aa76302f382e28096dc3d60b545`, production `+20/-0`, tests `+55/-0`, docs `0`.
- Han overlay/publisher candidate B: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`, direct parent `4119698525e806025ec36d00e0c85a5b1b3574a7`, production `+12/-5`, tests `+88/-0`, docs `0`.
- Direct whole-payload patch IDs: A `57bdcd672364438b3b898f35d6f60c7cc178f5ca`; B `4aef91de56b2e0c4756103ebedeae821f1570dec`.
- Path patch IDs: `consolidated.go` A `ab5ee7d69f18a12786a85166f6dec53c32caedd6`, B `044d7551d753396dd5709300988181b40dd20d0c`; `consolidated_test.go` A `ac5ee690834ba263b5e03dcfdf17473f8fae07f4`, B `46a21c8d7f5ea757a0f06fea5569676de385ffdf`.

The direct payloads were reconstructed only as `git diff <direct-parent> <commit>`; no stacked ancestry or report files were imported. `git range-diff parentA..A parentB..B` reports two unrelated one-commit ranges, with no equivalent patch mapping:

```text
1: 0cd00e4 < -: ------- fix(index): prune orphaned consolidated sidecars
-: ------- > 1: 8e9c9b7 fix(index): prune only missing source topics
```

## Same-base composition

Both orders were attempted in disposable detached worktrees at the exact base with `git apply --3way --index`:

| Order | Result | Evidence |
|---|---|---|
| A then B | conflict | both files conflicted; markers at `consolidated.go:817-843` and `consolidated_test.go:465-601` |
| B then A | conflict | first B application conflicted in both files; second could not proceed because the index was unmerged |

There is therefore no resulting combined patch identity. Naive additive net lines would be production `+27`, tests `+143`, docs `0`, but those numbers are not a valid combined candidate because the overlapping SQL cleanup and adjacent tests require semantic resolution.

The overlap is substantive: A deletes topic/verdict sidecars for affected sessions with no remaining `session_sources`; B changes source-topic pruning to preserve co-contributors and retain authoritative incoming replacement rows. A manual resolution must define the ordering and ownership interaction explicitly before any focused gate is meaningful.

## Isolated gates and mutation

- A isolated preflight: `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$' -timeout 120s` — **PASS**, `1.803s`.
- B isolated preflight: `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_(PreservesTopicsWhenCoContributorRemains|DoesNotRewriteUnchangedSoleSourceTopics)$' -timeout 120s` — **PASS**, `2.169s`.
- Disposable mutation: inverted A's sidecar cleanup `AND NOT EXISTS` to `AND EXISTS`. The A test failed with `mutation_exit=1`, reporting orphan topic rows `1` instead of `0` and co-contributor rows `0` instead of `1`.
- Mutation restoration: restored the disposable worktree and reran the exact A filter — **PASS**, `1.760s`.
- Composite focused race suite: **UNRUN**, because composition did not apply cleanly and no hand-resolved product candidate was authorized.
- Full package gate: **UNRUN** for the same reason.

The isolated tests are blind to the interaction: each candidate's tests pass independently, but neither test can observe the other's overlapping hunk or the required sidecar/topic cleanup ordering. The mutation proves the A assertion is meaningful; it does not establish cross-feature compatibility.

## Smallest correction and direction lock

Smallest required action: create a fresh exact-base resolution that manually merges the two SQL paths in `consolidateOneContext`, adds one test covering deleted sidecar plus co-contributor plus authoritative incoming topic replacement, then reruns exact-one preflights and the focused race gate. Do not transplant either commit unchanged.

Direction Lock impact: **no lock / existing sidecar-prune lock remains unchanged; this composite is not eligible**. No score event is claimed; no merge authorization is granted.

Referee worktree is report-only and clean after the report commit; no product files were changed.
