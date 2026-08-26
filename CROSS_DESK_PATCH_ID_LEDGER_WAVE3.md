# Cross-desk patch-ID ledger — Wave 3

Audit snapshot: 2026-08-26, based on immutable current worktree heads from `git worktree list`.
Graphify was oriented first from `/Users/jay-m4/code/rawclaw` (`reflect --if-stale`, then literal
query/explain for `ConsolidateFrom`, `session_sources`, `tag`, and `lock`). Patch IDs below are
computed from Go/test source only (`git diff-tree ... -- '*.go' | git patch-id --stable`).
Docs, FINDINGS, and report-only commits are recorded as `NO-NOVELTY` and excluded from patch
equivalence.

## Rulings

| Norm commit | immutable SHA | stable Go/test patch ID | source delta | merge base used | current rival equivalent / ruling |
|---|---|---|---:|---|---|
| b2ff61c | `b2ff61c53d1abd67ee87e9acabd47283b76a7a8f` | `0c8b28032a1f8baf7a6a076ac6205e47d753f476` | `50/-65 = -15` | `5b9756b2200ff6bd670f07407407d84d9f42d84b` | `b944d082e9b8d02611b018a25ce9a049066629fc` on Conor/Ozzy ancestry, exact patch ID: **DUPLICATE** |
| a317766 | `a317766e1906e92ff92300c62131c69d366b4939` | `cea8cc66c09632db4cd9980063e2e69a3646260c` | `1/-7 = -6` | `5b9756b2200ff6bd670f07407407d84d9f42d84b` | `78b6a4fe5a90771d9de7a1e3e83e0c046ed834a8` (Ozzy) and `fb893ed7ae8a1da95f3bbb5b651176cfb2275f6a` (Conor), exact patch ID: **DUPLICATE** |
| 61b7957 | `61b79574f72d8de1b0b8caa3a6402c3093a6173f` | `82e142f3630e29de6ffcf0182f05eba2050357ea` | `0/-8 = -8` | `5b9756b2200ff6bd670f07407407d84d9f42d84b` | no current Conor/Lenny/Ozzy source-bearing head with this patch ID: **NOVEL in the rival set** |
| bd8346c | `bd8346c5468435ba8636042c4846032e26460dba` | `d04dfd2a5176fa19377cbad7c786d1ee31433a2c` | `239/-74 = +165` | `5b9756b2200ff6bd670f07407407d84d9f42d84b` | `37ec96bebb2a8317617544836ef9730149e1f0d4` (Ozzy) is a **successor equivalent, not exact**: same `cmd_ingest_test.go` +157, but setup.go is `60/-28` versus `82/-74`; **HOLD** for transplant review |
| 50c6d0d (rejected) | `50c6d0d627b950c359f1f6a6adeec4e3bf6272bd` | `9e46225c354f062bdfe797002c15cf9bfdb6df36` | `47/-103 = -56` | `8824e256066518a685e685aa70eb2ed59019dfc8` | no rival source patch with this ID: **NOVEL (rejected candidate)** |
| 89c8a28 (rejected) | `89c8a284d20e4f6adba72accb3c0b34831a3b422` | `7e3d7d4981316e4cfb2b11f6a8eacbb71b314bdc` | `189/-15 = +174` | `479d14c782a229d3348b290885028c5efa7a8740` | no exact rival patch; current Ozzy implementation remains **HOLD**: `isLockedOrActive` releases its probe before `removeRefreshDB` (`containers.go:93-114`), leaving probe-to-unlink TOCTOU and non-atomic sidecar removal |
| b0d9e0f (rejected) | `b0d9e0fc5890f653fb17aefa66917c5800a87f26` | `a1cf7f2752ebce96c38d0e8dd1c7cf2048b06556` | `12/-95 = -83` | `c39872650a3ded47c7777e3ffad0ae3739b16f6b` | exact same patch is current Lenny `raid-hooks` and Conor `lenny-hook-wait-trap`: **DUPLICATE (rejected candidate)** |
| d345f80 (rejected) | `d345f80578b7210d496ed7c0796ac60a67802339` | `69c6ebec30a7689525a42f6b6e567a39dee722c0` | `101/-0 = +101` | `fc1a0759d429c43bb5cf150f77ac79f10c18d3fc` | exact same patch is current Lenny `raid-locate`: **DUPLICATE (rejected candidate)** |

## Successor difference: bd8346c versus Ozzy 37ec96b

Both patches have the same merge base (`5b9756b...`) and carry the same 157-line
`internal/cli/cmd_ingest_test.go` addition. The implementation is not patch-identical:

- Norm `bd8346c`: `internal/cli/setup.go` `82` additions / `74` deletions, net `+8`.
- Ozzy `37ec96b`: `setup.go` `60` additions / `28` deletions, net `+32`.
- The Ozzy successor retains the existing noclobber claim shape and replaces it with a
  temporary-directory + hard-link claim. Norm's integrated version also removes the old
  claim block and adds the same invalid-key fail-soft path, but has an additional 22-line
  replacement delta in setup.go. This is a distinct successor, not a duplicate patch.

The target's whole Go/test delta is `+239/-74 = +165`; Ozzy's is `+217/-28 = +189`.
No behavior gate was run because this ledger found identity and source-level differences;
the existing Ozzy audit's focused race checks do not clear the TOCTOU HOLD.

## Current worktree inventory and provenance

`git worktree list` found these source-bearing current heads. Other Conor/Lenny/Ozzy worktrees
were docs/report-only at their tips and are `NO-NOVELTY` for this ledger.

- Conor: `6d20bda91501aeb341c46181556137d273d77a38`, `0193241b6ce317ec0c931e6160b8e82b21f48161`, `ed1527ef8c8d7f8386b4908ef843fb9416535886`, `25b8d3762bc768f5ca6aa069fd1aeb5948dc36d7`, `fb893ed7ae8a1da95f3bbb5b651176cfb2275f6a`, `bf7cdd0de71f8fbfd6e86c34852062f0766fddc7`, `5b9756b2200ff6bd670f07407407d84d9f42d84b`.
- Lenny: `d7106e9bd0cb6b4f98e5e8bfdedd82dde8dd9bd9`, `6ddd17a373114f8ca643cabe26014370e9e432a9`, `b0d9e0fc5890f653fb17aefa66917c5800a87f26`, `d345f80578b7210d496ed7c0796ac60a67802339`, `c3b3d2bcdf9fbd26b27fae76277c21d33789fca2`, `b5f570baeb30522c0e002427ff4ec0177a04b3b7`.
- Ozzy: `89c8a284d20e4f6adba72accb3c0b34831a3b422`, `a24a2bbcf23aa369bad83d1e6477a7f9bf7217e5`, `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`, `78b6a4fe5a90771d9de7a1e3e83e0c046ed834a8`.

The exact duplicates are attributable to shared commit ancestry, not independent reimplementations:
`b944d08` is reachable from both `conor/ozzy-range-shrink` and `ozzy/harvest-wave1-20260826`;
`78b6a4f` is Ozzy's harvest tip; `fb893ed` is Conor's corresponding tip; `b0d9e0f` is
reachable from Lenny raid-hooks and Conor's wait-trap; and `d345f80` is Lenny raid-locate.

`net: -205 duplicate lines avoidable` (b2ff61c `15` + a317766 `6` + b0d9e0f `83` +
d345f80 `101`). The bd8346c/37ec96b successor delta is distinct and therefore excluded
from duplicate-line savings.
