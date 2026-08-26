# Adoption audit

Base under test: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.

This is an independent reproduction. Graphify was used first with the shared
`/Users/jay-m4/code/rawclaw/graphify-out/graph.json`; it framed
`renderHookScript()`, the catalog-claim tests, and the Antigravity registration
helper. The graph is a shared snapshot and cannot see candidate-branch deltas,
so ancestry, diffs, cherry-picks, and gates below are direct Git/runtime
evidence. Required AGENTS.md files and CONTRIBUTING.md were read. No edits
outside this report were made.

## Identity and ancestry

| commit | parent | merge-base with base | stable commit patch-id | raw commit payload |
| --- | --- | --- | --- | --- |
| `bd8346c5468435ba8636042c4846032e26460dba` | `61b79574f72d8de1b0b8caa3a6402c3093a6173f` | `0d1da19` | `d04dfd2a5176fa19377cbad7c786d1ee31433a2c` | `internal/cli/setup.go` and `internal/cli/cmd_ingest_test.go`; +239/-74 |
| `37ec96bebb2a8317617544836ef9730149e1f0d4` | `b944d082e9b8d02611b018a25ce9a049066629fc` | `5b9756b` | `f66a11ef522e6e12ca4f37bfcbb5109344af8a16` | same two files; +217/-28 |
| `61b79574f72d8de1b0b8caa3a6402c3093a6173f` | `a317766e1906e92ff92300c62131c69d366b4939` | `0d1da19` | `82e142f3630e29de6ffcf0182f05eba2050357ea` | `internal/store/connect_bench_test.go`; -8 |

Pair merge-bases: bd/37=`5b9756b`, bd/61=`61b7957`, and 37/61=`5b9756b`.
The raw `37ec96b` ancestry is older and is not a current-base transplant.

Current-base cumulative diffs are contaminated by ancestor commits and are
reported separately: bd `+283/-147` across four files, 37 `+688/-398` across
nine files, and 61 `+44/-73` across two files. These numbers are not used to
claim that those ancestor changes belong to the candidate payload.

## Semantic payload

`bd8346c` is the current-base path-safety payload. It validates flat catalog
keys, routes invalid IDs to quoted fail-soft ingest without using them as path
components, and uses a same-directory temporary directory plus hard-link claim
so an existing FIFO or directory is not opened or overwritten. It also removes
an impossible Antigravity helper error return and adds the hostile-path matrix.

`37ec96b` attempts the same hook mechanism, but is based on the older setup
shape. Its source payload is not byte-identical to bd: it changes the older
empty-file/move implementation and has no current-base-compatible patch.

`61b7957` combines the Search and Browse benchmark matrices into one nested
loop, deleting eight duplicated test lines. It has no production-code delta.

## Current-base transplant and gates

- Cherry-picking raw `37ec96b` onto `0d1da19` conflicts in
  `internal/cli/setup.go` in both Claude and Codex hook blocks. The conflict is
  not safely resolved by accepting one side wholesale because the base carries
  later catalog-claim changes. This raw commit is therefore not adoptable as an
  ordered commit.
- Cherry-picking `61b7957` onto base is clean (local result `7bb4e51`), with
  exactly 8 deletions.
- Cherry-picking `bd8346c` alone onto base is clean (local result `a7df834`),
  with the stated two-file payload. Cherry-picking `61b7957` then `bd8346c`
  is also clean (local results `7bb4e51`, `696a95b`).
- On bd-alone, the focused hostile-path matrix passed with
  `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run
  '^TestPrimeScripts_SessionStartCatalogClaimIsPathSafe$'`: 19.836s wall,
  13.906s test time.
- On bd-alone, `CGO_ENABLED=0 go test -race -count=1 ./internal/cli` passed:
  111.558s wall. The full `CGO_ENABLED=0 go test -race -count=1 ./...`
  passed: 111.378s wall (all packages green).
- On raw 37, the same focused hostile-path matrix passed: 9.029s wall,
  4.312s test time. This does not overcome its current-base conflict.
- On raw 61, `CGO_ENABLED=0 go test -race -count=1 ./internal/store` passed:
  7.356s wall, 5.212s test time.
- On the combined `61` then `bd` transplant, `~/go/bin/golangci-lint run`
  passed with 0 issues in 6.966s wall. The unqualified `golangci-lint` name
  was unavailable; the repository-installed path was used.

No empirical lint or test gate is claimed for the raw 37 transplant because it
cannot be constructed without a conflict resolution. No benchmark runtime
claim is made: the 61 change was compile/test-gated, not benchmark-measured.

## Verdict

The smallest safe ordered adoption retains both independent current-base
payloads: benchmark deduplication first, then the current-base path-safe hook
commit. The raw older 37 commit is rejected as a transplant, not rewarded for
sharing test bytes with bd.

ADOPT_BD8346C_PAYLOAD

1. `61b7957` (cherry-pick onto `0d1da19`)
2. `bd8346c` (cherry-pick after `61b7957`)
