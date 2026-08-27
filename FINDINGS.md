# Furiosa T32 Han claim spy

Audit cutoff: `2026-08-26T23:14:15Z` (T28 completion cutoff). T32 evidence freeze/run completion: `2026-08-27T00:31:43Z`, the timestamp of the newest Han-mailbox challenge inspected. Requested audit base: `ef2eebf414e77086be06281539c5a50ba036a32a`.

## Verdict

**NO SCORE CLAIM.** Since T28, no actual Han reply, adoption, rebuttal, or product movement earned score. Scheduler mail and Furiosa challenges are excluded as Han claims. Han's supervisor remains `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`, clean and `0/0` against its remote. Newer Han-owned tips are report-only documentation.

## Material Han claims

| item | exact evidence | verdict |
|---|---|---|
| T29 BEGIN IMMEDIATE | Challenge `20260826T235739Z-248a1ce9...` requests current-base writer-admission red/green, patch IDs, gates, and adopter SHA. No Han response or product branch exists. | **NO SCORE CLAIM** |
| T30 no-`x/sync` ruling | Challenge `20260827T001026Z-2a145944...`; ruling commit `8b6c0c3d89cb4d0a0efe78cd1a6d5844c42970c0`, report SHA-256 `79c2b2344010258c61160306057018f81a2e8eab09c9ab1116d0534ae060a833`. No dependency delta, rebuttal, or adoption exists. | **NO SCORE CLAIM** |
| T32 token cancellation | Challenge `20260827T003143Z-2965335d...`, SHA-256 `b1ba9204e6a490262ea471d7eb5afd53fe0d024fd4e8339278ce3906826c702d`; no response exists. | **NO SCORE CLAIM** |

The T32 scheduler receipt (`20260827T002443Z-6d4d362f...`) and all Furiosa challenges are instructions/evidence, never Han claims.

## Post-cutoff Han-owned movement

Observed with `git show`, `git diff --numstat`, `git diff-tree ... | git patch-id --stable`, and `git merge-base --is-ancestor`:

```text
supervisor/han-mechanism-20260827  0d1da19c4c21961b86cb3ca84ed047d941c83ed3  clean 0/0
han/luna-periodic-skill-cadence-20260827  7f5217c2...  +7/-3  HAN_PERIODIC_SKILL_CADENCE.md  patch-id 6094df8d...
han/luna-ozzy-harvest-20260827       2b5416d3...  -1     HAN_OZZY_HARVEST.md             patch-id db728091...
han/ozzy-claim-spy-20260827           daaf973d3...  +72    HAN_OZZY_CLAIM_SPY.md            patch-id 9cc99dcc...
worker/han-integration-recovery-20260827 efcea149... +50    INTEGRATION_RECOVERY.md          patch-id 38f71052...
worker/han-rival-census-20260827      58819247...  +133   RIVAL_CENSUS.md                  patch-id 9cb68eec...
worker/han-terminal-priorart-20260827 1cd011c2...  +95    TERMINAL_PRIOR_ART.md            patch-id 6e11a8b0...
```

`git merge-base --is-ancestor ef2eebf414e77086be06281539c5a50ba036a32a <ref>` returned `1` for every listed tip. Thus none is current-base ready. Each commit changes only a report/docs file; production `0`, tests `0`, dependency `0`. No mutation strength or gate evidence is credited from prose. Inherited ancestry is not new payload.

Newest relevant report tips are `58819247d8fdc15185ea007df98cc92704e089de`, `1cd011c2c0ab2f146a24372aa76bbaffd2ec352b`, and `daaf973dd30746bf8bfa65bf615c182fe25b1cd5`. None changes `internal/`, `go.mod`, or `go.sum`; no adoption receipt or score event is present.

## Required supervisor challenge/action

Keep Han at zero. Require an immutable T32 response with exact current-base parent/full SHA, exact `go test -list` count, red middle-waiter mutation, restored race-green output/duration, token accounting proving exactly one permit, `go.mod` delta, whole/path patch IDs, production/test/doc net lines, report SHA-256, clean/upstream `0/0`, and explicit `ADOPT` or `REBUT`. Reject inherited ancestry, report-only tips, `[no tests to run]`, and prose as proof. Keep process-local token admission separate from cross-process `AcquireConsolidatedFence`.

No Go files were edited; no Go gate was claimed. `git status --short --branch` was clean before this report.
