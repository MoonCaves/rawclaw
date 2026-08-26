# Catalog-claim mutation audit — corrected candidate evidence

This correction supersedes the conclusion in `56bbb4a`; that commit remains transport evidence from base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` but did not exercise the assigned candidate-only test.

Graphify was re-run first with literal catalog/claim/traversal queries. Its graph lacks the candidate symbol and commit nodes, so exact relationships were then confirmed from named objects. Fresh detached worktrees were created at `bd8346c5468435ba8636042c4846032e26460dba` and `37ec96bebb2a8317617544836ef9730149e1f0d4`; both contain `TestPrimeScripts_SessionStartCatalogClaimIsPathSafe`.

Candidate baselines passed: bd package `2.739s`, wall `5.498s`; raw 37 package `2.379s`, wall `4.536s`, using `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run TestPrimeScripts_SessionStartCatalogClaimIsPathSafe`.

The candidate test covers `sh`/`dash` x Claude/Codex x FIFO, directory, and `x/../../outside` traversal, with a 5-second context timeout. It does not cover regular files, symlinks, sockets, or bash.

## Candidate mutants

### C1 — remove flat-ID validation: SURVIVED

In fresh bd worktree `bd8346c`, both `catalog_session_id` validation `case` blocks were removed. The bounded candidate command returned `ok`, `VALIDATION_MUTANT_EXIT=0`, package `2.417s`. The candidate test **SURVIVED**. Its fixture precreates `.tmp.x`, but implementation uses `.tmp.$$`; the unvalidated traversal temporary path fails to create/write and falls through to best-effort ingest without a root artifact. This is a test coverage hole, not proof validation is redundant.

### C2 — remove dangling-symlink existence check: SURVIVED

In fresh bd worktree `bd8346c`, both `[ -e ] || [ -L ]` checks became `[ -e ]`. The bounded candidate command returned `ok`, `CANDIDATE_SYMLINK_MUTANT_EXIT=0`, package `2.116s`, wall `2.519s`. The candidate test **SURVIVED** because it has no symlink case.

## Comparison with base test

The base `TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath` passed its larger 3-shell x 2-template x 9-state matrix, including regular, symlink, dangling-symlink, symlink-directory, symlink-FIFO, and socket, in `7.886s` wall. Its FIFO-opening mutant timed out at 12s; omitting `[ -L ]` failed all six dangling-symlink combinations in `8.335s`. Thus base kills both, while the candidate test kills neither C1 nor C2.

## Corrected verdict

The bd/37 candidate payload contains flat-ID validation and same-directory hard-link claiming, and its supported traversal fixture passes. However, its candidate test has semantic coverage gaps: removing flat-ID validation and removing dangling-symlink recognition both survive. Retain the original base-matrix evidence, but do not credit the candidate test as mutation-backed for those properties.
