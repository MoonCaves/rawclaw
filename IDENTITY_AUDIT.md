# Identity audit: `bd8346c`, `37ec96b`, `61b7957`

Independent audit from fixed base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` on
`worker/furiosa-audit-identity-20260827`. No rival report or preliminary probe
was used as evidence; Mnemon was queried only for orientation.

## Graphify and scope

First codebase action, against the public base graph:

```text
graphify reflect --if-stale
graphify query "bd8346c 37ec96b 61b7957 identity patch" --budget 4000 --graph /Users/jay-m4/code/rawclaw/graphify-out/graph.json
```

The graph was refreshed (`Reflected 0 memories`) and is a graph of another
checkout, not this worker's branch. It framed `renderHookScript` and catalog
hook tests; claims below were verified with immutable Git objects. A second
pass used `explain "renderHookScript"` and `query "catalog session claim setup hook"`.

## Commit identity and ancestry

Parents:

```text
bd8346c5468435ba8636042c4846032e26460dba -> 61b79574f72d8de1b0b8caa3a6402c3093a6173f
37ec96bebb2a8317617544836ef9730149e1f0d4 -> b944d082e9b8d02611b018a25ce9a049066629fc
61b79574f72d8de1b0b8caa3a6402c3093a6173f -> a317766e1906e92ff92300c62131c69d366b4939
```

Merge bases:

```text
merge-base(bd8346c,37ec96b) = 5b9756b2200ff6bd670f07407407d84d9f42d84b
merge-base(bd8346c,61b7957) = 61b79574f72d8de1b0b8caa3a6402c3093a6173f
merge-base(37ec96b,61b7957) = 5b9756b2200ff6bd670f07407407d84d9f42d84b
```

Commits from fixed base (`git rev-list --oneline --reverse base..commit`):

```text
bd8346c: b2ff61c, a317766, 61b7957, bd8346c
37ec96b: c653543, 2367b58, 86e3d52, 041a153, 847426c, 539de03, b944d08, 37ec96b
61b7957: b2ff61c, a317766, 61b7957
```

`bd8346c` is not a standalone replay of `37ec96b`: it is on a different line,
and carries `b2ff61c`, `a317766`, and `61b7957` as ancestry. `61b7957` is
literally an ancestor of `bd8346c`.

## Stable per-commit patch IDs

Computed with `git show --format= --no-ext-diff <commit> | git patch-id --stable`
(these are per-commit IDs, not range-diff IDs):

```text
bd8346c d04dfd2a5176fa19377cbad7c786d1ee31433a2c
37ec96b f66a11ef522e6e12ca4f37bfcbb5109344af8a16
61b7957 82e142f3630e29de6ffcf0182f05eba2050357ea
b2ff61c 0c8b28032a1f8baf7a6a076ac6205e47d753f476
b944d08 0c8b28032a1f8baf7a6a076ac6205e47d753f476
a317766 cea8cc66c09632db4cd9980063e2e69a3646260c
```

`b2ff61c` and `b944d08` are exact stable-patch-ID duplicates. The three target
commits are pairwise non-identical by per-commit patch ID. `git range-diff
37ec96b^! bd8346c^!` was observed as a correspondence/rewrite view and was not
used as a per-commit identity claim.

## Paths, stats, and byte identity

```text
bd8346c: internal/cli/cmd_ingest_test.go (157 additions), internal/cli/setup.go (82 additions, 74 deletions)
37ec96b: internal/cli/cmd_ingest_test.go (157 additions), internal/cli/setup.go (60 additions, 28 deletions)
61b7957: internal/store/connect_bench_test.go (8 deletions)
```

The resulting `cmd_ingest_test.go` blobs are byte-identical:

```text
37ec96b result = c7915a38424ad80ee40dfeeb159e447e2d9e9e96
bd8346c result = c7915a38424ad80ee40dfeeb159e447e2d9e9e96
```

This does not make the commits identical. Their `setup.go` result blobs differ:

```text
37ec96b result = 317fa26bdad032dbdbf879171fca1ff290f02a1b
bd8346c result = 7d4e1cca596d4db869441219417bc2f7a2875960
```

`37ec96b` parent `setup.go` = `032439cbdee190b3c65a8a2e33ff8f3e3d04b07f`.
`bd8346c` parent `setup.go` = `7d4e1cca596d4db869441219417bc2f7a2875960`.
`internal/store/connect_bench_test.go` is unchanged by `37ec96b` and changed
only by `61b7957`.

## Cherry-pick from fixed base

A disposable detached worktree was created from the fixed base, each commit was
tested independently, then the worktree was removed cleanly:

```text
cherry-pick 37ec96b: CONFLICT in internal/cli/setup.go; cmd_ingest_test.go staged; status M internal/cli/cmd_ingest_test.go, UU internal/cli/setup.go
cherry-pick 61b7957: clean, 8 deletions (synthetic commit 59ffbf8)
cherry-pick bd8346c: clean, 239 insertions, 74 deletions (synthetic commit 2549b13)
```

No conflict resolution was invented or silently credited. The clean current-base
transplant is `bd8346c`; the older commit requires resolution because its patch
context is tied to divergent ancestry.

## Verdict

`bd8346c` is a distinct successor/reintegration commit, not identical to
`37ec96b`, despite sharing the exact 157-line test addition and commit subject.
It supersedes the earlier implementation on newer ancestry and includes the
already-committed `61b7957` benchmark deletion. The two feature commits must not
be counted as two independent full implementations: shared test blob proves
overlap, while different setup blobs and stable patch IDs prove non-identity.
`61b7957` is independently identifiable as a separate ancestor benchmark cleanup.
