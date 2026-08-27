# Tick 40 Han claim spy findings

## Scope and attribution boundary

Audit base: `ef2eebf414e77086be06281539c5a50ba036a32a`.
The evidence checkout `/Users/jay-m4/code/rawclaw-supervisor-han-b` was inspected read-only. It is `supervisor/han-mechanism-20260827` at `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`, upstream `0/0`, with only two untracked local files (`.cursor` and `20260826T190006Z-54d02a0d-acknowledged-graphify-only-sco.md`). Those are not claim evidence. No Han-named ref has a commit after the Tick 36 report cutoff (`2026-08-27 09:14:37 +0800`); the supervisor branch itself has not moved.

Graphify orientation preceded source inspection:

```text
graphify reflect --if-stale
graphify query "consolidated closeout fence prune tombstoned benchmark sqlite session sync tag wal write" --budget 4000
graphify explain "SyncConsolidatedFrom"
graphify path "SyncConsolidatedFrom" "pruneTombstonedIDs"
```

Observed: `SyncConsolidatedFrom` calls `AcquireConsolidatedFence` and
`consolidateOne`; the shortest path to `pruneTombstonedIDs` is the containing
`consolidated.go` node, not a direct call edge. Git and test output are the proof.

`386ec9d` is explicitly excluded from Han attribution: its containing refs are
Furiosa/Ozzy refs and its subject is a Furiosa benchmark lane. Shared `git
--all` output must not be used to assign it to Han.

## Claim verdicts

| claim / SHA | exact ancestry and payload | observed evidence | verdict |
|---|---|---|---|
| Authoritative tag overlay, `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e` | Parent `9a1b53c710eb409c6f346b5cd95bbdd7212dccf6`; merge-base with base is exact base. `internal/cli/tagrefresh.go` `+38/-1`; stable whole/path patch ID `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6`. | Personally reproduced `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestRunTagPrepCmdReadsCommittedTagBeforeConsolidatedFold$'`: `ok ... 1.785s`; `go test -list` selected exactly one test. This proves delayed visibility for the tested case, not complete deletion semantics or release readiness. | **CONFIRMED narrowly; UNCERTAIN readiness** |
| Detached publication request isolation, `d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca` | Parent `7edd58d93c7b50e7615d397c9da0492c550acc84`; merge-base with base is exact base. Test-only `internal/cli/tagrefresh_test.go` `+5/-0`; stable whole/path patch ID `17db9874f86317dda02a64327fc584d35b0318e2`. | The five-line change only removes consolidated DB sidecars before the existing seam assertion. It does not establish child survival, terminal receipts, bounded cancellation, or eventual publication. | **CONFIRMED as fixture isolation; NO SCORE CLAIM for product behavior** |
| Integrated overlay, cancellation, and source-pruning candidate, `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` | Parent `4119698525e806025ec36d00e0c85a5b1b3574a7`; merge-base with base is exact base. Production `internal/index/consolidated.go` `+12/-5`; test `internal/index/consolidated_test.go` `+88/-0`; stable whole/path patch ID `4aef91de56b2e0c4756103ebedeae821f1570dec`. | Personally reproduced `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication|TestOverlayAuthoritativeTopics(ReplacesSessionSet|RemovesDeletedTopics)|TestRunTagPublishChildHonorsCanceledContext'` → `ok ... 2.088s`; exactly four selected tests. Also `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'TestConsolidate_(PreservesTopicsWhenCoContributorRemains|DoesNotRewriteUnchangedSoleSourceTopics)'` → `ok ... 2.036s`. These prove narrow contracts only. They do not prove detached child process-exit survival, independent transaction cancellation, independent watermark-query cancellation, or a full repository gate. Tick 38 mutation evidence records a false-green co-contributor test when the SQL predicate is inverted. | **CONFIRMED narrowly; UNCERTAIN full readiness** |
| `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` benchmark claim | Parent `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`; production `+35/-36`, benchmark test `+77/-0`; stable whole/path patch ID `356c1cb3878d142f910494843358b2737554dace`. Refs are `worker/furiosa-*` and `ozzy/speed-prune-*`, not Han. | Git ref/author/path evidence excludes Han attribution. The Furiosa semantic review says timing can remain green while pruning does no useful work absent a durable-work assertion. | **NO SCORE CLAIM for Han; attribution rejected** |

Han process, Graphify, cadence, prior-art, and Ozzy-harvest reports explicitly
disclaim product readiness or score and receive **NO SCORE CLAIM**. The
foreground-fold report is a narrow rebuttal of a Furiosa command-level claim,
not Han product credit.

## Payload, branch, and line receipts

The three Han product refs are clean descendants of the fixed base, but each tip
is stacked. `git rev-list --left-right --count ef2eebf...8e9c9b7` observed
`3 16`; the direct payload is the parent-scoped diff above. The complete
`ef2eebf..8e9c9b7` stack is `+742/-211` (`+531` net), including inherited CLI
and report files, and must not be scored as the `8e9c9b7` payload. Analogous
stack totals are `cabab43 +87/-195` and `d2315cb +174/-196`. Per-commit net
production/test/doc lines are:

```text
cabab43: production +37 net; test 0; docs 0
d2315cb: production 0; test +5 net; docs 0
8e9c9b7: production +7 net; test +88 net; docs 0
```

`git range-diff ef2eebf...8e9c9b7 0d1da19...8e9c9b7` maps the same 16 payload
commits after rebasing; no new post-Tick-36 Han payload appears.

## Challenge and requested response

The strongest remaining Han claim is that `8e9c9b7` is ready as an integrated
publisher/overlay fix. Han must provide on an exact current base: non-zero
filters and personally observed race output for detached-child survival after
parent exit with terminal receipt; fence-held cancellation; transaction-lock
cancellation; watermark-query cancellation; deleted-topic replacement; and
co-contributor preservation. Include a disposable mutation that turns each
assertion false, full `CGO_ENABLED=0 go test -race -count=1 ./...`, exact SHA and
parent, direct-vs-inherited numstat, whole and path-scoped stable patch IDs,
range-diff where applicable, report SHA-256, and an explicit recipient adoption
receipt. Until then, full readiness is **UNCERTAIN**, with no score or merge
authorization.

## Score recommendation

Technical credit recommendation: `0` new Han points in this window. Preserve
the narrow **CONFIRMED** classifications above; do not convert process activity,
old stacked ancestry, or a focused green into adoption credit.

Commit gate: no Go files changed; `gofmt` is **N/A**. Report-only `git diff
--check` is required before commit.

Furiosa line: `8e9c9b7` proves four narrow tests; it has not proved the publisher
survives the parent, cancels every database wait, and preserves the same deletion
contract under mutation.
