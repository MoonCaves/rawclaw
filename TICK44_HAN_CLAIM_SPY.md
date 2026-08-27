# Furiosa Tick 44 Han claim spy

## Scope and immutable boundary

- Audit base: `ef2eebf414e77086be06281539c5a50ba036a32a`, parent
  `2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce`.
- Tick40 cutoff: Han report `a260f69ffa704a9be39fca5de08b465d086f1f4b`,
  committed `2026-08-27 09:57:23 +0800`.
- Evidence checkout: `/Users/jay-m4/code/rawclaw-supervisor-han-b`, branch
  `supervisor/han-mechanism-20260827`, tip
  `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; upstream `0/0`. It has only
  untracked `.cursor` and
  `20260826T190006Z-54d02a0d-acknowledged-graphify-only-sco.md`; neither is
  claim evidence. The checkout was read-only. No mailbox or cursor was read,
  changed, or staged.
- Graphify orientation, before Git/source inspection: `graphify reflect
  --if-stale` (no prior lessons), `graphify query "8e9c9b7 cancellation
  composite" --budget 4000`, `graphify explain "8e9c9b7"` (no matching node),
  and `graphify path "8e9c9b7" "0cd00e4"` (no matching node). The query
  surfaced cancellation and vector/index seams; exact identity and behavior
  below come from Git and immutable reports.
- Mnemon recall was run before inspection with the requested key. It returned
  the Tick38/Tick40 Han reports and the Tick42 composite/mutation outcomes.

## Cutoff census: no new Han claim

`git log --all --since='2026-08-27 09:57:24 +0800'` contains no commit on a
`han/*` ref and no commit after the cutoff on the Han supervisor branch. The
Han refs remain capped at their earlier tips, the latest product candidate
being `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` at `04:09:23 +0800`.
Therefore since Tick40 there is **no immutable Han adoption receipt, no Han
rebuttal, and no Han current-base resolution** to score. Post-cutoff commits
`06cf95c36ba821f88bb35b648325a8411648a0f7` and
`1dd45346dc72a5abd0aab8871c4289f3dd1b2202` are Furiosa/referee reports, not
Han work; `386ec9d` is likewise explicitly non-Han by ref/provenance.

## Existing Han claims and current verdicts

| Han evidence | exact identity and direct payload | observed response | verdict |
|---|---|---|---|
| `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e` authoritative topic overlay | Parent `9a1b53c710eb409c6f346b5cd95bbdd7212dccf6`; `merge-base` with audit base is `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; `internal/cli/tagrefresh.go` `+38/-1`, net production `+37`; whole/path stable patch ID `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6`. | Tick40 focused CLI test passed with one selected test, proving committed-topic visibility only. No detached survival, terminal receipt, complete cancellation, or full current-base gate. | **CONFIRMED narrowly; UNCERTAIN readiness; NO SCORE CLAIM** |
| `d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca` detached-publication fixture | Parent `7edd58d93c7b50e7615d397c9da0492c550acc84`; `tagrefresh_test.go` `+5`, net test `+5`; stable patch ID `17db9874f86317dda02a64327fc584d35b0318e2`. | Five-line fixture removes consolidated sidecars before a seam assertion. It proves neither child process survival nor terminal publication. | **CONFIRMED fixture isolation; NO SCORE CLAIM** |
| `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` integrated overlay/publisher/pruning | Parent `4119698525e806025ec36d00e0c85a5b1b3574a7`; `merge-base` with audit base is `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; production `internal/index/consolidated.go` `+12/-5` (net `+7`), test `+88`, docs `0`; whole stable patch ID `4aef91de56b2e0c4756103ebedeae821f1570dec`; path IDs `044d7551d753396dd5709300988181b40dd20d0c` (Go) and `46a21c8d7f5ea757a0f06fea5569676de385ffdf` (test). | Tick40 selected four CLI tests plus two index tests passed. Tick42 mutation report (`06cf95c`, report SHA-256 `e3ab45e95794fff34c896d8ba9d5b44d93249e10004bc5ee96c3169b1881c085`) found fence cancellation and child-context propagation genuinely covered, but `BeginTx` and watermark-query mutations stayed green because the test canceled before those layers; detached survival/terminal receipt had no proving test. | **CONFIRMED narrow; UNCERTAIN integrated readiness; NO SCORE CLAIM** |

The Han chain is stacked. `git rev-list --left-right --count` against the
audit base is `3 3` for `cabab43`, `3 4` for `d2315cb`, and `3 16` for
`8e9c9b7`; its `16`-commit stack must not be counted as the direct payload.
`git range-diff ef2eebf..8e9c9b7 0d1da19..8e9c9b7` maps the same 16 commits;
that is ancestry alignment, not a new post-Tick40 claim or adoption.

## Post-cutoff rebuttal and composition evidence

| challenge | action | immutable response | ruling |
|---|---|---|---|
| Does Han's integrated cancellation claim cover every cancellation layer? | Furiosa mutated fence acquisition, child fold context, `BeginTx`, watermark query, and `Setsid`. | Fence and child-context mutations went red. `BeginTx`, watermark-query, and detach mutations stayed green/unsupported; the report explicitly records those as false-green or unobserved. | **REBUTTED** as a complete cancellation proof; only the two narrow layers are confirmed. |
| Can Han `8e9c9b7` compose with the current-base sidecar adaptation `0cd00e4`? | Referee applied both payloads with `git apply --3way --index` in both orders at exact base `ef2eebf`. | Both orders conflicted in `consolidated.go` and `consolidated_test.go`; no combined payload, focused gate, or full gate exists. Composite report `1dd45346...`, SHA-256 `3e67b2e3483f45c03066402740f7f806619605f470dfdd6d484920bafeec3844`. | **REBUTTED** as automatically composable; no current-base resolution. |
| Did any post-Tick40 activity create a score-bearing adoption? | Census searched Han refs, supervisor tip, report ancestry, and current-base receipts. | No Han commit after cutoff; all later visible evidence is report-only Furiosa/referee work. Existing scores remain Furiosa `+9`, Han `+2`, Ozzy `+3`. | **NO SCORE CLAIM**; silence, self-adoption, inherited ancestry, and report-only evidence do not score. |

The composite payload identities were independent: `0cd00e4` whole ID
`57bdcd672364438b3b898f35d6f60c7cc178f5ca`, Han `8e9c9b7` whole ID above;
their path IDs also differ. Naive additive totals (`+27` production,
`+143` tests) are not a candidate because semantic resolution was never made.

## Readiness and required response

- Current audit base is exact and unchanged; no Han tip is based directly on
  it (`8e9c9b7` is based on `4119698`, though its merge-base is the earlier
  `0d1da19` line). The supervisor checkout is dirty only by the two unrelated
  untracked files noted above; its branch is upstream `0/0`.
- The strongest Han product claim remains **UNCERTAIN**, not adopted. The
  full `CGO_ENABLED=0 go test -race -count=1 ./...` pass recorded in the
  cancellation report is a gate on that candidate's narrow tests, not proof
  of detached process survival, transaction admission cancellation, or
  watermark interruption. Composite full gate was **UNRUN** because both
  application orders conflicted.
- Unique required response from Han: provide a fresh exact-audit-base
  candidate or explicitly accept the uncertainty, with non-zero exact-one
  filters, separate transaction and watermark cancellation tests, a real
  parent-exit/child-terminal-receipt harness, one-fault red mutations for each
  assertion, stable whole/path patch IDs, direct-vs-inherited line counts,
  range-diff, full race output, and an independent recipient adoption receipt.
- This report records evidence and challenges only. It grants **no merge
  authorization** and does not authorize transplanting either candidate.

## Completion receipt

- Report-only; no Go files changed; `gofmt -w` is **N/A**.
- Required pre-commit check: `git diff --check`.
- After commit: verify clean worktree and upstream `0/0`; record report
  SHA-256 and Mnemon remember ID.
