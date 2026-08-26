# Han attack on Furiosa foreground-fold claim

## Immutable receipts

- Base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
- Imported Furiosa commit: `8aab2cbfc6fe6a395017807a39a0c2a894672bcf`.
- Cherry-pick result on this branch: `ba4213f79bb6ec2bfbca06b359f4c329e63a741d` (parent is the exact base).
- Stable patch ID: `0f176ba2fa8d89766fdf05b014c8a215fe20b1f7`.
- Imported files and lines: `FOREGROUND_FOLD_AUDIT.md` +40, `internal/cli/cmd_tag_test.go` +120; net +160 lines.
- Imported commit was pushed before attack to `origin/han/luna-attack-furiosa-fold-20260827`.
- Focused filter matched nonzero: `go test ./internal/cli -list '^TestRunTagWriteForegroundFoldLatency$'` listed `TestRunTagWriteForegroundFoldLatency`.
- Focused race gate: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestRunTagWriteForegroundFoldLatency$' -timeout 90s` PASS (2.138s).
- Timing repeat: `CGO_ENABLED=0 go test -race -count=5 ./internal/cli -run '^TestRunTagWriteForegroundFoldLatency$' -timeout 180s` PASS (7.207s).

## Attack and findings

The test writes with `runTagWrite` directly, then starts `index.SyncConsolidatedFrom` in a goroutine while holding `consolidated.lock`. It does not invoke `runTagWriteCmd`; therefore it cannot pin the command wrapper's lookup, authoritative write, fold, and return as one measured operation. The audit's limitation about this seam is accurate.

Disposable assertion mutation: changed the fenced-reader assertion from `if len(hits) != 0` to `if len(hits) < 0`. The focused race test still PASSed. This mutation demonstrates that the test can falsely pass if the consolidated reader sees a hit while fenced; the later post-release assertion does not detect that visibility-ordering regression. The file was restored exactly before finalization, and `git diff --check` is clean.

The synchronization and ordering checks are still useful: the source topic is read from the project store before the fold is released; the fold does not return during the 100 ms hold; releasing the lock permits publication; and the post-release consolidated read finds the session. Five race repetitions passed, so the 100 ms observation threshold was stable on this runner, but it remains a machine-timing threshold rather than a fully deterministic CI contract.

## Rulings

- `runTagWriteCmd` foreground blocking: **UNCERTAIN**. This test does not call it.
- `SyncConsolidatedFrom` blocks on a held consolidated fence and publishes after release: **CONFIRMED** by the focused race gate and five repetitions.
- Consolidated visibility is absent while the lock is held: **CONFIRMED**, but the disposable mutation proves this assertion is necessary and not redundant.
- Furiosa's broader product conclusion that the observed behavior is undesirable or should be detached: **UNCERTAIN**. The test establishes coupling, not policy.
- Furiosa audit's `REBUTTED` verdict about foreground fold coupling: **REBUTTED** if it denies that coupling; **CONFIRMED** only as a statement that the stronger `runTagWriteCmd` command-level measurement is unavailable.

## Smallest required correction

No production correction is authorized or required in this lane. Keep the exact `len(hits) != 0` assertion. If a command-level latency claim is needed, add an explicit injectable seam immediately before `SyncConsolidatedFrom` (outside this authorized fence) and test `runTagWriteCmd` through that seam. Until then, reports must name this as a `runTagWrite` + `SyncConsolidatedFrom` visibility test, not a direct command-return test.

