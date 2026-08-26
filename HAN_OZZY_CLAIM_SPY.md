# Han claim spy: Ozzy publication/deletion candidates

Audit base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
Audit worktree: `han/ozzy-claim-spy-20260827`, clean at report creation.
Graphify orientation used the root graph (`graphify reflect --if-stale`, lessons,
`query`/`explain`/`path`); the decisive path was
`runTagWriteCmd -> SyncConsolidatedFrom -> consolidateOne`, with topic replacement
and `AcquireConsolidatedFence` as the risk boundaries. `mnemon --store rawclaw recall
consolidated` was also run before inspection.

## Verdicts

| SHA | Verdict | Evidence and boundary |
| --- | --- | --- |
| `4f8ea6cbf0c59d2d82764c01e0a1429d0ae4892c` | **CONFIRMED, incomplete** | Detached one-session publication is real and based on the audit base. It removes foreground `SyncConsolidatedFrom` and publishes after the authoritative write, under a fence in the child. It does whole-session destructive replacement and has no foreign-origin or revision guard. Do not adopt alone. Patch ID `fdf6b91cda7b2204274781303b335ec12c59d55a`; branch `ozzy/narrow-tag-publisher-20260827` is upstream-equal. |
| `0672141c74cd5eb0847f576e67d4bf77d103f254` | **REBUTTED as complete** | Adds a local timestamp guard and preserves foreign-origin rows, but an empty/older local snapshot can fail to delete a previously published local row because `sourceRevision` starts at zero and deletion is skipped when the existing local revision is newer. It also does not solve overlay deletion, hard-link self-alias, or out-of-order publication. Authority test exists, but no full gate was claimed. Patch ID `58b87536edf210389425b7a92dc23316df4f7b61`; branch not observed as a pushed upstream branch. |
| `593b16ea5b5c18b6bdc1344cb50d8ff93c07ca4e` | **NO SCORE CLAIM** | Test-only co-contributor preservation proof. It supplies no production fix; the corresponding `publishSession` behavior remains destructive. Patch ID `8c0ebacae303965e43f10d4b17783189dd6336dd`. |
| `92e83cd4dd564d12f4afe67c114b34f2b6e51d4d` | **REBUTTED / superseded** | DBP-only fast path can author before the guarded fence, but misses scopes carrying only `TDir`; its own later replacement is `74d4ee9`. Patch ID `bdbb3ab817ed1484a72922572f402085a5305bb9`. |
| `74d4ee9b1bfcece8a37d17eecb91dfe4ac71f300` | **CONFIRMED, scoped** | Adds `TDir -> index.DBPath(TDir)` to the fast path. Personally reproduced `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestTagWrite(FastPath|TDirFastPath)AuthorsBeforeConsolidatedFence$'`: `ok` in 2.083s on the immutable SHA. This is only current-project `{Project,TDir}` author-before-fence; nil/all-project still uses guarded lookup and remains intentionally non-instant. Patch ID `ec3ee871e20fe8a141f78ed28b78271505cd20ad`; `ozzy/fix-author-before-lookup-20260827` equals its remote. |
| `9c845edf45ab5b11561616cfa66b2c7ef56f7262` | **CONFIRMED red proof** | Reproduced `TestOverlayAuthoritativeTopicsDropsDeletedBoundary`: failed because the old derived `bbb` boundary survived. This is immutable invalidation evidence, not an adoption. Patch ID `2ea17f5ea04cf2c1758e4d8572d9fe61ddf88428`; remote red branch observed. |
| `bfc2fbd7f257800cfa3a33154fbd381441a33b9c` | **CONFIRMED red proof** | Adds the ordinary consolidation deletion red. It demonstrates the ghost-topic gap; no fix in this SHA. Patch ID `f59837d18fcb21a6ca51d01ad342a74441da281b`; remote red branch observed. |
| `ff094cb0606e7ea2302b5a3cd5b9522cd1055b24` | **CONFIRMED limitation proof** | `TestRunTagWriteCommandSeamIsNotIsolated` passes on this SHA, proving the old command path waits in guarded lookup while the consolidated lock is held. It is not a production improvement. Patch ID `88cef6c556bf533b2f2ac9a98b1b843ab21c3c94`. |
| `37294537fba063298811a5a7f6db8997ff0e6fc4` | **CONFIRMED red proof** | Reproduced `TestIsConsolidatedSourceRejectsHardLinkAlias`: failed. `EvalSymlinks`/pathname equality cannot identify a hard-link alias of the same SQLite file. Patch ID `ff9aa6ed9e366d5e950e5f7896cff6afb5a0c76a`; remote red branch observed. |
| `d7f4532f00ac5ee5f293437d6ff7c9c2e5edd40c` | **CONFIRMED red proof** | Reproduced `TestPublishSessionRejectsOutOfOrderSnapshot`: failed; older snapshot replaced newer. Patch ID `eed924d532d1b8d7f78ef143e396e6a2f201b83d`. |
| `74a02857edbb3ab431eb460db786ae9e4e013055` | **WITHDRAWN / NO SCORE CLAIM** | Reproduced `TestPublishSession_PreservesForeignContributor`: failed; whole-session replacement erased the foreign contributor. Treat its union expectation as withdrawn because authoritative whole-session authored-unit semantics require provenance authority, not a franken-union. Patch ID `5cebb05bb5a1d8eb0c6167e09963eb03774ee9f6`. |

## New deletion candidates

`537641b0231d8690005a916bc138303b17b43c87` is **CONFIRMED as a correctness
candidate, not a complete transplant**. I personally ran, on that SHA,

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/index -run \
  'TestConsolidate_(DeletesTopicsRemovedFromSource|OriginAuthorityWinsForConflictingTopicSegments)|TestConsolidateFrom_PreservesLegacySourceWhenCoContributorSkipped'
```

and observed `ok` in 3.191s. Its SQL deletes only missing topic boundaries for a
sole source and preserves co-contributor rows; it also has a no-rewrite regression.
The SHA is on `ozzy/fix-consolidated-topic-delete-20260827`, clean and equal to
`origin/ozzy/fix-consolidated-topic-delete-20260827`. Patch ID:
`b7aaaee70fe88073287bb0fecc0c9b81beb80368`.

However, against Han's same-base `4119698525e806025ec36d00e0c85a5b1b3574a7`
(patch ID `03aff4c8d25d8306660da314f742f82b4cd5afe4`), `537641b` is not a
drop-in equivalent. The diff shows it reintroduces/removes context-aware
`QueryRowContext`/`BeginTx` propagation and differs in the origin-authority
logic. Transplant only the minimal stale-topic SQL after preserving the
context-aware and provenance-authority code.

Furiosa's latest `8c8216e25e22496b2e3e919fce836be49d692e25` (branch
`worker/furiosa-detached-publication-20260827`, remote-equal) also passed focused
overlay/deletion/no-rewrite/co-contributor tests in 5.9s. Its final SQL is a
narrower sole-source rule than `2ca75b9`; the diff removes the `srcOrigin`
origin-aware branch. **UNCERTAIN** for sessions whose source identity carries
provenance authority; do not call it a universal winner without those tests and
the full race gate.

## Smallest-winner recommendation

1. Adopt `74d4ee9` only for the scoped `{Project,TDir}` author-before-fence path;
   preserve guarded nil/all-project behavior.
2. Keep Han's `4119698` context-aware/provenance-authority base and transplant the
   smallest verified missing-topic SQL from `537641b` (or compare against the
   complete composite `8c8216e`), then run origin-authority, co-contributor,
   overlay-deletion, ordering, self-alias, focused race, and full repository race
   gates.
3. Do not adopt `4f8ea6c` or `0672141` standalone; do not revive `74a0285`; treat
   `9c845ed`, `bfc2fbd7`, `ff094cb`, `3729453`, and `d7f4532` as red/limitation
   evidence only.

No merge authorization, score, universal “instant” claim, or full-gate green is
claimed by this report.
