# External branch recovery receipt

Audit branch before report commit: `worker/furiosa-external-branch-recovery-20260827@2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce`.

Exact report topology:

```text
0d1da19c4c21961b86cb3ca84ed047d941c83ed3
  -> 987c6a31186bb15615175c5198389aa0d31846f6  [worker/furiosa-terminal-receipt-20260827]
  -> 2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce  [supervisor/furiosa-evidence-20260827, worker/furiosa-external-branch-recovery-20260827]
987c6a31186bb15615175c5198389aa0d31846f6
  -> 0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f  [worker/furiosa-terminal-receipt-referee-20260827]
```

Exact branch@HEAD values inspected:

```text
han/luna-detached-tag-publisher-20260827@d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca
han/luna-tag-overlay-20260827@cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e
han/luna-overlay-publisher-integration-20260827@8e9c9b77e1eed984bfa847b9d613f263e2d46dd2
ozzy/narrow-tag-publisher-20260827@4f8ea6cbf0c59d2d82764c01e0a1429d0ae4892c
ozzy/fix-fastpath-source-refresh-20260827@7dad56df1cdf235fe72e14618cb15a81ed965611
ozzy/fix-tagprep-detached-fold-20260827@cb339acf8db4043775cc512b9926e76b5526aa16
ozzy/composite-instant-tagwrite-20260827@fb99037cda7c4ca80b6f5294631e5e5c0acc71b6
ozzy/luna-sidecar-referee-20260827@0633aabaf5bb79b06cb746ac5782efc1cd53402c
worker/furiosa-detached-publication-20260827@8c8216e25e22496b2e3e919fce836be49d692e25
worker/furiosa-integration-winner-20260827@8c8216e25e22496b2e3e919fce836be49d692e25
worker/furiosa-terminal-receipt-20260827@987c6a31186bb15615175c5198389aa0d31846f6
worker/furiosa-terminal-receipt-referee-20260827@0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f
supervisor/furiosa-evidence-20260827@2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce
origin/supervisor/furiosa-evidence-20260827@987c6a31186bb15615175c5198389aa0d31846f6
```

Stable patch IDs:

```text
8c8216e  3a409032463981bbdcf625eeeac1ff9424973a14
987c6a3  33c107108b0ddd123cddaea88f5119e157c392a2
2789d6f  1f324d9b1a667bcf8e3e0b6f89167ab1f39626e4
0b39b82  59e62cb90a5b5eddd0b7163478148a5eb69b52dc
7edd58d  6f276e8e4dcba0dedb80739a1966dfc10a3ca64a
ebc1711  6f276e8e4dcba0dedb80739a1966dfc10a3ca64a
cabab43  72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6
3529f5f  72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6
4f8ea6c  fdf6b91cda7b2204274781303b335ec12c59d55a
a6c6dbd  8d5211ee5d1eff010ad2d9f2776a4ae2376822a2
3bb7a2f  2e719bca83b940d2b79801bac026b87543407aa8
0bbcc4d  ec3ee871e20fe8a141f78ed28b78271505cd20ad
3d91fbb  eeca83456293ccf24fef13b1aa4e34183da61163
0672141  58b87536edf210389425b7a92dc23316df4f7b61
946a533  c4d719b04550d41e3b45e022dd07fbae5831b97a
4e5af10  9d415fa195ffca6195f6b83df8daa53a64102663
828a49e  bc3b2bc94c22fae138f9ba60818aeaf433fa312d
0633aab  88512d6e9bddea3f848b235a72ea9dc823c0197f
```

Range-diff receipts:

```text
git range-diff --no-dual-color 0d1da19c..han/luna-detached-tag-publisher-20260827 0d1da19c..8c8216e25e22496b2e3e919fce836be49d692e25
git range-diff --no-dual-color 0d1da19c..ozzy/narrow-tag-publisher-20260827 0d1da19c..8c8216e25e22496b2e3e919fce836be49d692e25
git range-diff --no-dual-color 0d1da19c..ozzy/composite-instant-tagwrite-20260827 0d1da19c..8c8216e25e22496b2e3e919fce836be49d692e25
git diff --name-status 8c8216e25e22496b2e3e919fce836be49d692e25..ozzy/composite-instant-tagwrite-20260827
git show <commit> --format= --binary | git patch-id --stable
```

Classifications:

```text
UNIQUE / worth review: catalog + TDir fast path; nil-vs-empty scope; source refresh.
UNIQUE / worth review: narrow publication authority, per-origin revisions, alias/equal rejection.
UNIQUE / worth review: removed-session topic/verdict sidecar pruning.
UNIQUE / reject direct integration: detached tag-prep fold; async ownership and receipt contract unresolved.
DUPLICATE / reject: Han detached publisher; stable patch-id equals Ozzy ebc1711 and range-diff maps to winner f35625b.
DUPLICATE / reject: Han overlay; stable patch-id equals the alternate overlay commit and is in the winner-family seam.
DUPLICATE / reject: Ozzy cancellation/deletion/missing-source prune steps; range-diff marks shared steps as equivalent.
```

Graphify receipts:

```text
graphify reflect --if-stale
graphify . --code-only --no-viz
graphify query 'TagWrite Consolidated Furiosa Han Ozzy' --budget 4000
graphify explain 'TagWrite'
graphify path 'TagWrite' 'Consolidated' --context call --budget 4000
graphify query 'RefreshDBPath EnsureFreshContainer tagresolve_fast' --budget 2500 --context call
graphify query 'runTagPrepCmdWithSources tagrefresh SyncConsolidatedFromContext' --budget 2500 --context call
graphify query 'consolidation_affected_sessions topic_segment session_verdict pruneTombstoned' --budget 2500 --context call
graphify path 'runTagWriteCmd()' 'AcquireConsolidatedFence()' --context call --budget 3000
graphify save-result ... --outcome useful
```

Graphify result: current checkout graph fresh at 3,008 nodes and 9,926 edges;
it links tag-write/tag-prep to source refresh, topic reads, consolidated sync,
and the fence. It does not index branch-only source additions.

Safe supervisor commands to consider, not executed:

```text
git update-ref refs/backup/furiosa-terminal-receipt-20260827 987c6a31186bb15615175c5198389aa0d31846f6
git update-ref refs/backup/furiosa-reproduction-20260827 2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce
git update-ref refs/backup/furiosa-terminal-referee-20260827 0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f
git branch --contains 987c6a31186bb15615175c5198389aa0d31846f6
git branch --contains 2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce
git fsck --no-reflogs --full
```

Uncertainty: no full test gate was claimed; the consolidated-only reproduction
is intentionally red. Graphify was code-only, and patch-id alone was not used
as semantic proof. No existing branch was modified or cleaned.
