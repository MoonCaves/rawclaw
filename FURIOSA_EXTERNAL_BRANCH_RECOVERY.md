# Furiosa external branch recovery audit

Date: 2026-08-27 WITA. Read-only audit; no existing branch, worktree, mailbox,
or cursor was rewritten.

## Verdict

The current reproduction head is `2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce`.
It is a child of report commit `987c6a31186bb15615175c5198389aa0d31846f6`.
The report is safely rehomed on
`worker/furiosa-terminal-receipt-referee-20260827@0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f`;
that referee tip contains `987c6a3` as its parent. The local supervisor evidence
branch also contains the reproduction, but its remote-tracking ref is stale at
the report commit.

Winner `8c8216e25e22496b2e3e919fce836be49d692e25` is not the whole rival universe.
Range-diff and file-level inspection show that the Han detached/overlay work is
the same publication family as the winner, while Ozzy has additional mechanisms.
Three are worth preserving for a later, separately reviewed integration:

1. Catalog/TDir fast-path resolution, with `nil` meaning default catalog scope,
   an explicit empty slice meaning no fast lookup, and source refresh before
   authoring. Commits: `a6c6dbd`, `3bb7a2f`, `0bbcc4d`, `3d91fbb`.
2. Narrow publisher authority protection: preserve higher-origin data, compare
   revisions within the publishing origin, reject equal revisions, and detect
   symlink/hard-link aliases. Commits: `0672141`, `946a533`, `4e5af10`, `828a49e`.
3. Prune topic and verdict sidecars when a source removes an entire session.
   Commit: `0633aabaf5bb79b06cb746ac5782efc1cd53402c`.

These are integration candidates, not accepted merges. The current reproduction
still demonstrates that a consolidated-only default nil-scope write can block
behind the consolidated fence; none of the three source/catalog candidates
fixes that case because there is no surviving source/catalog row to resolve.

## Topology and reachability

The relevant exact refs observed before this report commit were:

| branch | HEAD | relationship |
|---|---|---|
| `han/flash-candidate-stomp-20260827` | `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` | fixed base |
| `han/luna-detached-tag-publisher-20260827` | `d2315cbbdcbf2edf6e3f9f8b4821acf9c622afca` | Han detached-publisher stack |
| `han/luna-tag-overlay-20260827` | `cabab43fb2ca7bd5cd2a20edf8ded27ea0bdc98e` | Han overlay stack |
| `han/luna-overlay-publisher-integration-20260827` | `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2` | Han combined overlay/publication stack |
| `ozzy/narrow-tag-publisher-20260827` | `4f8ea6cbf0c59d2d82764c01e0a1429d0ae4892c` | Ozzy narrow publisher |
| `ozzy/fix-fastpath-source-refresh-20260827` | `7dad56df1cdf235fe72e14618cb15a81ed965611` | Ozzy source refresh |
| `ozzy/fix-tagprep-detached-fold-20260827` | `cb339acf8db4043775cc512b9926e76b5526aa16` | Ozzy detached tag-prep fold |
| `ozzy/composite-instant-tagwrite-20260827` | `fb99037cda7c4ca80b6f5294631e5e5c0acc71b6` | Ozzy composite tip |
| `ozzy/luna-sidecar-referee-20260827` | `0633aabaf5bb79b06cb746ac5782efc1cd53402c` | sidecar-prune referee |
| `worker/furiosa-detached-publication-20260827` | `8c8216e25e22496b2e3e919fce836be49d692e25` | winner |
| `worker/furiosa-integration-winner-20260827` | `8c8216e25e22496b2e3e919fce836be49d692e25` | winner alias |
| `worker/furiosa-terminal-receipt-20260827` | `987c6a31186bb15615175c5198389aa0d31846f6` | terminal report |
| `worker/furiosa-terminal-receipt-referee-20260827` | `0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f` | report rehomed and independently reproduced |
| `supervisor/furiosa-evidence-20260827` | `2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce` | local supervisor evidence |
| `origin/supervisor/furiosa-evidence-20260827` | `987c6a31186bb15615175c5198389aa0d31846f6` | remote-tracking ref one commit stale |

Ancestry is direct for `0d1da19c -> 987c6a3 -> 2789d6f`. The referee is
`987c6a3 -> 0b39b82`. The local supervisor evidence and this recovery branch
both contain `2789d6f`; the remote-tracking supervisor ref contains only
`987c6a3`. No cleanup was performed.

## Patch and range evidence

Stable patch IDs observed for the key commits:

| commit | stable patch-id | meaning |
|---|---|---|
| `8c8216e` | `3a409032463981bbdcf625eeeac1ff9424973a14` | winner final prune fix |
| `987c6a3` | `33c107108b0ddd123cddaea88f5119e157c392a2` | terminal report |
| `2789d6f` | `1f324d9b1a667bcf8e3e0b6f89167ab1f39626e4` | reproduction head |
| `0b39b82` | `59e62cb90a5b5eddd0b7163478148a5eb69b52dc` | referee update |
| `7edd58d` | `6f276e8e4dcba0dedb80739a1966dfc10a3ca64a` | Han detached publisher |
| `ebc1711` | `6f276e8e4dcba0dedb80739a1966dfc10a3ca64a` | Ozzy/Han detached publisher duplicate |
| `cabab43` / `3529f5f` | `72d417eb5ed2eafeb7949d7960ee63d76eb5d9c6` | overlay duplicate |
| `4f8ea6c` | `fdf6b91cda7b2204274781303b335ec12c59d55a` | narrow publication fence |
| `a6c6dbd` | `8d5211ee5d1eff010ad2d9f2776a4ae2376822a2` | catalog default resolution |
| `3bb7a2f` | `2e719bca83b940d2b79801bac026b87543407aa8` | nil versus empty scope |
| `0bbcc4d` | `ec3ee871e20fe8a141f78ed28b78271505cd20ad` | TDir fast path |
| `3d91fbb` | `eeca83456293ccf24fef13b1aa4e34183da61163` | refresh TDir before lookup |
| `0672141` | `58b87536edf210389425b7a92dc23316df4f7b61` | preserve authority |
| `946a533` | `c4d719b04550d41e3b45e022dd07fbae5831b97a` | authoritative narrow publication |
| `4e5af10` | `9d415fa195ffca6195f6b83df8daa53a64102663` | per-origin revision |
| `828a49e` | `bc3b2bc94c22fae138f9ba60818aeaf433fa312d` | alias/equal-revision rejection |
| `0633aab` | `88512d6e9bddea3f848b235a72ea9dc823c0197f` | removed-session sidecars |

Commands used, reproducible without changing refs:

```sh
git range-diff --no-dual-color 0d1da19c..han/luna-detached-tag-publisher-20260827 0d1da19c..8c8216e25e22496b2e3e919fce836be49d692e25
git range-diff --no-dual-color 0d1da19c..ozzy/narrow-tag-publisher-20260827 0d1da19c..8c8216e25e22496b2e3e919fce836be49d692e25
git range-diff --no-dual-color 0d1da19c..ozzy/composite-instant-tagwrite-20260827 0d1da19c..8c8216e25e22496b2e3e919fce836be49d692e25
git diff --name-status 8c8216e25e22496b2e3e919fce836be49d692e25..ozzy/composite-instant-tagwrite-20260827
git show <commit> --format= --binary | git patch-id --stable
```

The Han range-diff maps its detached implementation to winner `f35625b` with
context and documentation differences, and identifies the winner's later
cancellation/deletion/prune commits as additional winner-only steps. The Ozzy
narrow range-diff maps the shared cancellation and deletion tests (`=`) but
leaves authority, fast-path, and other commits unmatched (`<`): those are
semantically unique rather than patch-identical duplicates. The composite file
diff touches `tagpublish.go`, `tagrefresh.go`, `tagresolve_fast.go`,
`cmd_tag.go`, `index.go`, and `consolidated.go`, confirming that its unmatched
claims are not report-only metadata.

## Duplicate rejection receipts

- **REJECT duplicate: Han detached publisher.** `7edd58d` and Ozzy `ebc1711`
  have the same stable patch-id. Range-diff places the Han implementation in
  the winner's detached-publication slot. Keep the winner lineage; do not
  integrate a second publisher.
- **REJECT duplicate: Han authoritative overlay.** `cabab43` and `3529f5f`
  have the same stable patch-id. Its read-after-write purpose is already in the
  winner-family tag-prep/publication stack; the differing surrounding commits
  do not establish a second mechanism.
- **REJECT duplicate: Ozzy consolidated cancellation/deletion/prune fixes.**
  Range-diff marks the corresponding Ozzy commits as equivalent to winner
  steps, including cancellation propagation, deleted-topic proof, and the
  final missing-source prune. Stable patch IDs differ where context changed,
  so the `=` range-diff result is the controlling evidence, not hash equality.
- **REJECT as direct integration: detached tag-prep fold (`a6d24cc` and
  `cb339ac`).** It is unique in changing `runTagPrepCmdWithSources` from a
  foreground fold to `maybeSpawnIngest`. It is not a safe drop-in: ownership,
  retry, and terminal receipt semantics are not established, and the detached
  publisher already has a proven post-`Start` receipt gap.

## Graphify receipts

Graphify was refreshed with `graphify . --code-only --no-viz` because the
worktree has no LLM key for its 31 documentation files. The resulting graph
was fresh for this checkout: 3,008 nodes, 9,926 edges, 138 communities.

Required orientation receipts:

- `graphify reflect --if-stale`; read `graphify-out/reflections/LESSONS.md`:
  zero prior marked lessons.
- `graphify query 'TagWrite Consolidated Furiosa Han Ozzy' --budget 4000`:
  connected `runTagWrite`, `runTagWriteCmd`, `SyncConsolidatedFrom`,
  `ConsolidatedPath`, `TopicsForSession`, and tag-refresh tests.
- `graphify explain 'TagWrite'` resolved the node to `newTagWriteCmd` and its
  `runTagWriteCmd` call edge.
- `graphify path 'TagWrite' 'Consolidated' --context call --budget 4000`:
  `runTagWrite <- runTagWriteCmd -> SyncConsolidatedFrom <- consolidated.go`.
- `graphify query 'RefreshDBPath EnsureFreshContainer tagresolve_fast' --budget 2500 --context call`:
  linked refresh paths, `EnsureFreshContainer`, `runTagWriteCmd`, and
  `SyncConsolidatedFrom`.
- `graphify query 'runTagPrepCmdWithSources tagrefresh SyncConsolidatedFromContext' --budget 2500 --context call`:
  linked tag prep, source refresh, topic reads, and consolidated sync.
- `graphify query 'consolidation_affected_sessions topic_segment session_verdict pruneTombstoned' --budget 2500 --context call`:
  linked the consolidation affected-session set, topic/verdict storage,
  tombstone pruning, and `SyncConsolidatedFrom`.
- `graphify path 'runTagWriteCmd()' 'AcquireConsolidatedFence()' --context call --budget 3000`:
  one inferred call hop.
- `graphify save-result ... --outcome useful` recorded that the current-tree
  graph explains existing relationships but cannot see source additions that
  exist only on other branches.

Important boundary: Graphify indexes the current reproduction checkout, not
all Git branches. Rival-only symbols such as `locateTagWriteFast` and the
later `publishSession` variants were therefore adjudicated with `git show`,
range-diff, and file-level diffs after Graphify identified the corresponding
current-tree seams. No Graphify result is being presented as evidence that a
rival branch was indexed.

## Safe recovery commands for the supervisor to consider

Do not run these as part of this audit. Before any cleanup, preserve the objects
with explicit refs and verify reachability:

```sh
git update-ref refs/backup/furiosa-terminal-receipt-20260827 987c6a31186bb15615175c5198389aa0d31846f6
git update-ref refs/backup/furiosa-reproduction-20260827 2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce
git update-ref refs/backup/furiosa-terminal-referee-20260827 0b39b82a5d5b87edfe4d8b9c80c16c4b1038103f
git branch --contains 987c6a31186bb15615175c5198389aa0d31846f6
git branch --contains 2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce
git fsck --no-reflogs --full
```

Only after the backup refs and remote coordination are confirmed should the
supervisor consider deleting or moving
`supervisor/furiosa-evidence-20260827`. The current evidence says cleanup can
be lossless because `987c6a3` is independently retained by the terminal-report
branch and referee, and `2789d6f` is retained by this recovery branch and the
local supervisor branch. The remote-tracking ref must not be treated as proof
that the remote has the reproduction: it currently stops at `987c6a3`.

## Uncertainty boundaries

- No existing branch was modified, merged, rebased, deleted, or force-pushed.
- The code-only Graphify graph excludes documentation semantics and cannot
  represent branch-only source that is not checked out here.
- No full test gate is claimed. The report's existing evidence says the
  consolidated-only reproduction is intentionally red; this audit did not
  convert that red result into a green claim.
- Stable patch-id is whitespace/context-insensitive and is evidence of patch
  identity, not proof of behavioral identity. Range-diff and source/file diffs
  supplied the semantic judgment.
- The three integration candidates are recommendations for supervisor review,
  not validated merges. In particular, the nil-scope consolidated-only liveness
  case remains open.
