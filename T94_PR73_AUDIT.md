# T94 PR #73 claim-spy audit

## Terminal verdict

- Audited head: `3036e86c51d91234da180b22aa97b9f3040a5794`.
- Claim: CONFIRMED.
- Terminal ruling: SHIP.

## Current-base and exact-ref evidence

Fresh fetch resolved `origin/main` to `f0bceb012153223aff4de7780e146d513bed8ae4` and PR #73 to `3036e86c51d91234da180b22aa97b9f3040a5794`. The PR is directly based on current main (`git merge-base --is-ancestor origin/main HEAD` returned 0). The new head is a rebase of the prior audited `0798fe3` over merged PR #72 (`6318977`); the prior SHA is not an ancestor because of that rebase.

The earlier supplied `5886e1a` was superseded first by `0798fe3`; it was based on `ae8703b`, not current main, and conflicted with the intervening `ExpandHome` rename. That old conflict remains historical evidence only.

## Payload identity and line accounting

`git range-diff 3b46d4564ab5252bbe91344f47cb6bee62a5f131...0798fe36fee6c14d832e7af3545fca9ad925b32c origin/main...HEAD` reports `0798fe3 = 3036e86`. Stable patch-id is unchanged: `0d5593cc0d4612d6c5669f1d99b059b23518eea5` for both prior and current payloads.

The exact current payload changes only `internal/index/index.go`, `internal/paths/paths.go`, and `internal/paths/paths_test.go`: `+22/-42` overall. Production is `+13/-33` (net `-20`); tests are `+9/-9` (net `0`); docs are unchanged.

## Semantic equivalence

Current main already contains the canonical longest-existing-prefix implementation as `internal/paths.realpath`. The PR exports it as `paths.Realpath`, updates the paths-package callers, and keeps the index package-local `realpath` wrapper so all existing index callers retain their route. The helper algorithm is unchanged: existing symlinks resolve, missing tails are lexically re-appended, and the function remains error-free. The affected callers cover transcript discovery, explicit-directory fallback, symlink-out containment, project-directory derivation, and index/vault/container watermark paths; no removed helper reference remains outside the compatibility wrapper.

## Checks and semantic oracle

- `CGO_ENABLED=0 go test -race -count=1 ./internal/paths`: PASS (`1.968s`).
- `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'Test(Find|Contained|Reindex|Update|EnsureIndexed|DeleteTheStoreAndRebuild)'`: PASS (`51.226s`).
- Focused symlink/missing-path oracle using `TestContainedJSONL`, `TestFindTranscriptDir`, and `TestFindTranscriptDirExplicit`: PASS (`2.288s`). This covers symlink-out exclusion and missing/explicit fallback behavior.
- `git diff --check origin/main...HEAD`: PASS.
- GitHub checks for this head: Go 1.24 PASS, stable PASS, lint PASS.

No product files were edited by the audit. Graphify code-only orientation and literal query/explain/path revisits were already recorded in `graphify-out/memory/`.
