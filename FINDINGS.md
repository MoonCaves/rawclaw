# Tick 31 Direction Lock referee

Verdict: **LOCK STANDS**.

The Tick 29 and Tick 30 evidence does not invalidate the locked technical
direction. The lock is for `PA-CONSOLIDATED-SIDECAR-PRUNE-001`, whose immutable
record specifies the sidecar-prune recommendation, fingerprint
`d07f69f8d056f9f145bd9a864e3fa11660afadf13af3aca9acad39ea722bcb72`, exact
candidate base `878f631b74e68aa76302f382e28096dc3d60b545`, source winner
`c38f79acf9c9ae43ebd091a95f36837f43c0e423`, adaptation
`0cd00e44c7eb87e30fcf72f8ae790e7060635b09`, rejected candidate
`a78b39b3d87c82a4f83878359afc98e2b8fde2d4`, and patch IDs
`6a62ff59b1b20a5873006b17ce72cd64229f65a6` and
`57bdcd672364438b3b898f35d6f60c7cc178f5ca`.

The current referee base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` is the
common ancestor of the locked base and adaptation, not a rebased or changed
product candidate. It is the base of the report-only Tick 29/30 branches.

## Invalidation-trigger check

| Trigger | Result | Evidence |
| --- | --- | --- |
| Base change | Not triggered | `git merge-base 0d1da19 878f631` and `git merge-base 0d1da19 0cd00e44` both return `0d1da19`; the locked candidate remains based on its recorded `878f631` and adaptation `0cd00e44`. |
| Production patch change | Not triggered | `git log --ancestry-path 0cd00e44..origin/worker/furiosa-final-current-base-20260827` and `git diff --name-status 0cd00e44..origin/worker/furiosa-final-current-base-20260827` are empty. The unrelated `386ec9d` benchmark patch is a separate report-only branch, not a change to the locked candidate. |
| Failing mutation on locked selector | Not triggered | Tick 30's no-live-ID mutation targets `BenchmarkPruneTombstonedIDs`, not the locked selector `TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor`; the ledger records the locked selector's prior red `a78b39b3` / green `c38f79a` result with no newer failure. |
| Focused/full gate regression | Not triggered | Tick 29/30 entries report no new product candidate or gate regression. The recorded focused race gate and serialized `./internal/cli ./internal/index` gate remain PASS. |
| Adoption receipt invalidation | Not triggered | The ledger retains the immutable adoption receipt SHA-256 `fb8147aa46baf4668699e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`; Tick 29/30 add no invalidation or supersession evidence. |

The exact lock gate record remains:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$'
CGO_ENABLED=0 go test -p 1 -race -count=1 ./internal/cli ./internal/index
```

No decisive product gate is rerun here: **N/A**, because no product candidate
changed and no invalidation trigger was observed. The Tick 30 benchmark
mutation is evidence about benchmark strength only; it does not alter the
locked sidecar-prune production patch.

## Report-only branch verification

All listed branches were present locally and on `origin`, clean, and at
upstream `0/0`:

```text
worker/furiosa-t29-adoption-regrade-20260827   976bff78f0d4044fbf0e5da888f198fe16c230ea
worker/furiosa-t29-live-census-20260827        3bcb29561c40434539306c98a45c5d713e09789e
worker/furiosa-t29-external-mechanisms-20260827 3c31ccbb413cff8ab829aa150864bb030f9249f8
worker/furiosa-t29-sqlite-research-20260827   08ede1c134a7b4dd1c716bd74431e02d0b8eb5e4
worker/furiosa-t30-wal-dedupe-20260827        b3813cbb352492551e9d9387edc7fa4039165cd6
worker/furiosa-t30-semaphore-audit-20260827  8b6c0c3d89cb4d0a0efe78cd1a6d5844c42970c0
worker/furiosa-t30-386-bench-replacement-20260827 5878c48064a797314986a884e10163b086a84c5c
```

`git diff-tree --name-status` shows report-only commits: the Tick 29 commits
add `PRIOR_ART_FINDINGS.md`; the Tick 30 audit commits add `FINDINGS.md`. None
changes the locked candidate. The lock remains technical direction only and
does not authorize a merge.
