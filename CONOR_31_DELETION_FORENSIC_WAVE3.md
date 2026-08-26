# Conor Issue #31 deletion forensic — wave 3

## Verdict

**HOLD for standalone `luna/conor-31-log-tests-20260826` (`d5d036b`).**
The commit is test-only and removes a fold-phase contract test from its own
parent, leaving only the failed-fence acquire proof. The deletion is **SAFE as
duplicate cleanup only after** the independent `2ee9950` integration test is
present on the adoption base. `d5d036b` is not descended from `2ee9950`
(`git merge-base --is-ancestor 2ee9950 d5d036b` returned 1), so the Conor
branch by itself has a real coverage hole.

## Exact deletion and assertion inventory

`d5d036b9dd94c59a9ee3da2da8fb8d1039cb671d` has parent
`2bb219f8aeb412dbf9add6fe691cf606ad8805f1` and deletes only
`internal/index/consolidated_logging_test.go` lines 38-93 in the parent:

- `L49-L51`: one `t.Fatalf` assertion that the fold succeeds.
- `L53-L55`: the nine expected fold phases: `schema-migrate`,
  `source-migrate`, `attach`, `prepare`, `merge`, `detach`,
  `tombstone-prune`, `watermark-stamp`, and `connection-close`.
- `L58-L84`: recorder parsing and typed-duration classification for fold
  records.
- `L85-L92`: eighteen assertions (start plus typed duration for each of the
  nine phases), emitted through `t.Errorf` at `L87` and `L90`.

The commit has no production-file changes: `0 additions, 57 deletions`,
all in a test file. Stable patch ID:
`804bbd4fb74175854b4a824ff154b4b5724e62f6`.

## Surviving coverage map

On d5's tree, `internal/index/consolidated_logging_test.go:L38-L93` is gone.
The survivor at `L38-L88` is `TestConsolidatedFence_LogsAcquireDurationOnTimeout`:
it holds `consolidated.lock`, calls `AcquireConsolidatedFence`, and checks only
the failed **acquire** start and duration at `L80-L87` (the parent line numbers
are `L139-L144`). It does not execute `ConsolidateFrom` and covers none of the
nine deleted fold phases, including `prepare`, `merge`, `detach`, and
`connection-close`; it also does not check fence release.

The independent integration commit `2ee995096db544be1ba8c889e4c68e3eb7ef24d1`
adds `internal/index/consolidated_test.go:L19-L82` and has stable patch ID
`f55a2d8c37d3e6d1a89e5bd18d572c7c7b0355fb`. Its `L73-L81` checks all nine
fold phases plus fence `acquire` and `release`, with start and typed-duration
requirements via `L63-L71`. Thus the deleted fold assertions are semantically
duplicated by 2ee, but not by d5's own surviving tree.

## Mutation evidence

Two disposable worktrees were used; rival worktrees were not edited.

1. At `2ee9950`, replacing the `prepare` phase start/finish setup in
   `internal/index/consolidated.go` with a no-op caused the surviving broad
   test to fail:

   `consolidated_test.go:77: consolidate fold phase phase "prepare" has no start log`

   and the matching no-duration error. This confirms 2ee kills a missing fold
   phase log.

2. At disposable `d5d036b`, removing all fold-phase start logs while retaining
   the production duration calls left
   `TestConsolidatedFence_LogsAcquireDurationOnTimeout` **passing** under
   `-race`. Therefore d5's survivor does not kill the deleted fold-start
   behavior; this is a reproducible coverage gap on the standalone branch.

## Gates and measurements

- d5 survivor: `CGO_ENABLED=0 go test -race -count=5 -run '^TestConsolidatedFence_LogsAcquireDurationOnTimeout$' ./internal/index` — **PASS**, package `2.033s`, shell elapsed `4.580s`.
- 2ee broad successor: `CGO_ENABLED=0 go test -race -count=5 -run '^TestConsolidate_LogsPhaseStartsAndDurations$' ./internal/index` — **PASS**, package `2.642s`, real `3.19s`.
- `gofmt` was not applicable: report-only change, no Go files touched.

The deletion can be accepted only with an explicit base guarantee that the
2ee contract test (or an equivalent test) is retained. Otherwise HOLD due to
loss of all nine fold-phase assertions.

net: -57 duplicate lines possible
