# T52 composite claim spy

Date: 2026-08-27 WITA
Worker: Max Rockatansky

## Verdict

| Claim | Verdict | Evidence |
|---|---|---|
| Han fresh current base is `48661f403f880e2c1dac7615f39bbb8264eeafe7` | REBUTTED | `git rev-parse origin/main` returned `c818ea1212bb1f1110cefa65472f658b844840ef`; `origin/main <= 48661f4` returned exit `0`, so 48661f4 is descended from main, not the current main ref. |
| Ozzy PR40 tip is fetchable at `bc1682071e3c9bb734c2783ee121f43002d814d0` | CONFIRMED | Exact fetch succeeded; `git rev-parse origin/ozzy/composite-instant-tagwrite-pr-20260827` returned that SHA. |
| PR40 wholly contains PR35 through `a33ab02` | REBUTTED | `a33ab02` and `bc16820` share only the connection-benchmark commit's stable patch ID (`82e142f3630e29de6ffcf0182f05eba2050357ea`). They are not ancestors either way (`a33<=bc rc=1`, `bc<=a33 rc=1`), and their whole-tree comparison differs in 16 paths (`1478 insertions, 97 deletions`). Narrow benchmark-commit equivalence is CONFIRMED; whole-PR containment is not. |
| Corrected `4ac774a4` boundary is fetchable | UNCERTAIN | `git rev-parse --verify 4ac774a4^{commit}` failed with `fatal: Needed a single revision`; no matching origin ref was returned by `git ls-remote`. The requested malformed-own-checkout versus absent-current-base-test split cannot be empirically scored without the object. |
| `d918706` is patch-equivalent to PR40 | REBUTTED | `d918706` stable patch ID is `5133121d630d549c255f82606b13c1012c6c748f`; no commit in `origin/main..origin/ozzy/composite-instant-tagwrite-pr-20260827` has that ID. PR40 source at `cmd_tag.go` still acquires the consolidated fence for the consolidated-store path. |
| Removing the fence in `d918706` admits snapshot-and-rename lost-write risk | CONFIRMED | `git show d918706` removes the `AcquireConsolidatedFence` block from `runTagWriteCmd` exactly where a nil-scope consolidated write is opened. The same file's PR40 tip retains that block. A concurrent snapshot/rename rebuild can therefore replace the store after the direct write without the shared fence. This is source-mechanism evidence; no mutation test was claimed. |
| PR40 default-scope gate is present and green on exact tip | CONFIRMED | Exact-one list preflight and focused run on detached `bc16820`: `go test -count=1 -list TestRunTagWriteDefaultCatalogFastPathBeforeFence ./internal/cli` printed `TestRunTagWriteDefaultCatalogFastPathBeforeFence`; focused `go test -count=1 -v -run '^TestRunTagWriteDefaultCatalogFastPathBeforeFence$' ./internal/cli` ended `--- PASS ...` and `ok ... 0.341s`. |
| PR40 co-contributor preservation gate is present and green | CONFIRMED | Exact-one list preflight printed `TestConsolidate_PreservesTopicsWhenCoContributorRemains`; focused run ended `ok github.com/MoonCaves/rawclaw/internal/index 0.536s`. |
| PR40 legacy co-contributor-skipped gate is present and green | CONFIRMED | Exact-one list preflight printed `TestConsolidateFrom_PreservesLegacySourceWhenCoContributorSkipped`; focused run ended `--- PASS ...` and `ok ... 0.264s`. |
| PR40 cancellation gate exists | REBUTTED | Exact-one list preflight for `TestConsolidate_ContextCancellationDoesNotPublishAndRetryPublishesWatermark` produced no test name; verbose run ended `testing: warning: no tests to run` and `ok ... [no tests to run]`. Do not claim this test on PR40. |
| Han `77947bd` is fetchable | CONFIRMED | `git rev-parse --verify 77947bd^{commit}` returned `77947bd769ac9cf219aaa68fc2f06b336dd9bea5`. It is not descended from PR40 tip: `bc16820 <= 77947bd` exit `1`; both share merge-base `c818ea1`. |
| Han `0cd0b9c` cancellation implementation/test is green | CONFIRMED | Exact-one list preflight printed `TestConsolidate_ContextCancellationDoesNotPublishAndRetryPublishesWatermark`; focused run ended `ok github.com/MoonCaves/rawclaw/internal/index 0.426s`. Stable patch ID: `0e42e5863f27b36d40ef718cb02ab7c0c7fd1729`. |
| Han `0cd0b9c` test proves cancellation while SQLite is blocked | REBUTTED | The test acquires the process-local writer token at `consolidated_test.go:54-57`, then separately starts the SQLite holder transaction at `:58-65`, and only launches `SyncConsolidatedFromContext` at `:67-70`. The production function tries the same writer token first at `consolidated.go:587-591`, before `AcquireConsolidatedFence`/`ConnectRW` at `:592-600`. The canceled goroutine is stopped at the channel and never enters SQLite's blocked `Begin`/first `Exec`; the green test proves admission cancellation and no publication, not cancellation inside a SQLite wait. |
| Han `0cd0b9c` 15-line global channel/init is proven necessary | UNCERTAIN | Payload adds `consolidatedWriterGate` plus `init`, and changes database calls to context-aware forms. The supplied test is gated before SQLite, so it does not isolate channel/init from `ExecContext`; no deletion recommendation is evidence-backed. |
| Han `77947bd` hostile mutation claims are independently proven | UNCERTAIN | The SHA is fetchable and its stable patch ID is recorded, but this lane has no exact mutation transcript, output hash, or lint receipt for those claims. Do not score the mutations as green. |

## Receipts

Fetch command:

```text
git fetch origin '+refs/heads/*:refs/remotes/origin/*' --prune
```

Relevant fetch output:

```text
bc1682071e3c9bb734c2783ee121f43002d814d0 refs/heads/ozzy/composite-instant-tagwrite-pr-20260827
```

PR40 range shape:

```text
git range-diff --no-dual-color origin/main...origin/ozzy/composite-instant-tagwrite-pr-20260827
```

The range reported PR40 as 85 commits added from the `origin/main` base; it did not establish equivalence for `d918706`.

Patch statistics observed for `origin/main..bc16820`: 33 files, 3350 insertions, 673 deletions, net +2677 lines. The `a33ab02..bc16820` comparison differs in 16 paths, 1478 insertions, and 97 deletions; patch-id establishes only the shared benchmark commit, not whole-PR containment.

Graphify orientation used the merged graph at `/Users/jay-m4/code/rawclaw/graphify-out/graph.json` because this checkout has no `graphify-out/graph.json`. `SyncConsolidatedFrom()` calls `AcquireConsolidatedFence()` and is called by `runTagWriteCmd()`; Graphify path output was:

```text
Shortest path (1 hops): SyncConsolidatedFrom() <--calls [INFERRED]-- runTagWriteCmd()
Shortest path (1 hops): runTagWriteCmd() --calls [INFERRED]--> AcquireConsolidatedFence()
```

## Supervisor challenge

Do not score `48661f4` as the current base, do not treat one shared benchmark patch as whole-PR35 containment, and do not claim a cancellation test on PR40: the exact-one preflight and verbose run show that test is absent there. `d918706` is not PR40-equivalent and its fence deletion leaves the snapshot-and-rename write window open. The `0cd0b9c` test is channel-admission evidence, not SQLite busy-wait evidence; channel/init necessity remains UNCERTAIN until a symmetric production-path mutant is run.
