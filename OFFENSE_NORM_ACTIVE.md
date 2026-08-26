# Norm active-lane hostile audit

Date: 2026-08-26

Scope: report-only audit of the six named Norm candidate commits plus the dirty
payloads in `rawclaw-norm-flash-catalog` and `rawclaw-norm-flash-ingest`.

## Immutable receipts

- Audit checkout: `lenny/offense-norm-active-20260826`, HEAD
  `5b9756b2200ff6bd670f07407407d84d9f42d84b`; checkout was clean before this
  report. None of the six candidate tips is an ancestor of HEAD.
- Candidate tips and parents: `10a7c19` parent `dc2f6e2`; `178e8fc` parent
  `83c927f`; `cfccbc6` parent `89d7977`; `6ac7f1a` parent `5a121fb`;
  `21b8011` parent `da0bda8`; `b218528` parent `3d11fe1`.
- Patch IDs: `10a7c19=6f4a94c50bbfe16bc96aca541c88a926d492e479`,
  `178e8fc=1ccc2b83923c1b93b130bce2fec51b5f410b8ecf`,
  `cfccbc6=7addd4ca88dd31164e993883d4b57a4852e8e5b8`,
  `6ac7f1a=adfb05b7734c67a0bffb6782d56883ce9d037fe6`,
  `21b8011=14d8c058c645ace2d1480d913c950d38ad7c5d47`,
  `b218528=bb038818f44fd83ab497358f189c334ba294d3a9`.
- Duplicate patch receipts: `178e8fc` equals `479d14c` and `fd01a92` by
  patch ID; `cfccbc6` equals `539de03` by patch ID. `git range-diff` reports
  both pairs as identical one-commit ranges.
- Dirty payload receipts: catalog patch ID
  `2c9060c971e991f342ae639431c6c68f6b92a933`, 1 insertion/7 deletions in
  `internal/agentproto/agentproto.go`; ingest patch ID
  `d1ef94259c2a75e904ca2e4be8de3c63ca0bdd3b`, 50 insertions/238 deletions in
  `internal/cli/cmd_ingest_test.go`. Both pass `git diff --check`.
- Graphify was refreshed with `graphify reflect --if-stale`; lessons were
  empty. A literal query against the public graph found the relevant
  `AcquireConsolidatedFence`, `locateSession`, `NormalizeRecord`, and
  tombstone/consolidation test nodes. Mnemon recalls were run for `rawclaw
  norm`, `rawclaw atomic claim`, and `rawclaw fault retry` before the related
  review areas.

## Confirmed

### C1 — Dirty ingest payload deletes contract tests and weakens data checks

`/Users/jay-m4/code/rawclaw-norm-flash-ingest/internal/cli/cmd_ingest_test.go`:

- The dirty diff deletes the runtime `TestPrimeScripts_StopLaunchDetachedPrewarm`
  test (the Stop hook's actual detached `prewarm <session>` behavior) and
  `TestPrimeScripts_SessionStartIngestsWhenCatalogUnavailable` (the exact
  fail-soft contract that `10a7c19` claims to fix).
- It also deletes `TestClaudePrimeScript_ExecutesDetachedIngest`, leaving only
  a static substring assertion for the invocation rather than an executable
  shell-hook probe.
- In `TestIngestCmd_IndexesFreshSession_EndToEnd`, the raw
  `COUNT(*) FROM messages` assertion and JSON `role`/`snippet` checks are
  removed. `sessions.message_count` alone cannot prove that the message rows
  exist or have the expected searchable/rendered fields.
- In `TestIngestCmd_Idempotent_RepeatedRunIsNoOp`, the raw message-row count is
  removed, so duplicate rows can pass while the aggregate count remains 2.
- In `TestIngestCmd_ConcurrentRuns`, the prior explicit “all sessions were
  properly indexed” row verification is removed; the replacement only checks
  aggregate session counts. This is a silent-data-loss coverage regression,
  not merely test cleanup.

This is a confirmed hostile finding because the deletions are present in the
dirty payload and each removed assertion covered a distinct observable
contract. The helper extraction itself is not the problem.

### C2 — `10a7c19` is a behavior-preserving six-line duplication, not a fix

`10a7c19:internal/cli/setup.go:66-70,157-161` replaces the existing compact
condition `if (set -C; : > "$entry") ... || [ ! -e "$entry" ]; then` with an
`elif [ -e ... ]; then exit 0; else claimed=1` branch in both generated hook
templates. The old condition and the new branch have the same outcomes for
claim success, existing entry, and unavailable catalog. The patch adds six
lines and duplicates the explanatory comment twice without changing the
shell behavior. Ponytail ruling: `shrink:` retain the compact condition or
extract the shared predicate; do not transplant this commit as a functional
fix. This is a confirmed complexity/offense finding, not a correctness claim.

## Narrowed / no deduction

- `178e8fc` is materially better than its predecessor as a repro: it restores
  the child to the parent's HOME, mutates the source before retry, and asserts
  the retry reaches two consolidated messages. However, it does not assert the
  new UUID/content or that the watermark value changes to the new source mark;
  it only checks watermark-row existence. This is a test-strength limitation,
  not proof of a product bug. It is also duplicate patch content of `479d14c`
  and `fd01a92`.
- `cfccbc6` removes only diagnostic WAL/SHM logging from that repro. The
  deletion is a reasonable shrink, but the commit is duplicate patch content
  of `539de03`; no independent correctness deduction.
- `6ac7f1a` extracts the repeated consolidated-fence holder setup in tests,
  reducing net lines. No unsafe behavior or missing contract was found.
- `21b8011` changes a reverse index loop to `slices.Backward`; the loop
  direction and predicate are preserved. No deduction.
- `b218528` is documentation-only and corrects its own line-count claim. No
  product or test deduction.
- The dirty catalog payload in
  `/Users/jay-m4/code/rawclaw-norm-flash-catalog/internal/agentproto/agentproto.go`
  inlines a one-use predicate into the caller and is net-negative. No
  semantic change or offense found.

## Focused gate receipts

- `CGO_ENABLED=0 go test -count=1 ./internal/cli -run
  '^TestPrimeScripts_StopLaunchDetachedPrewarm$'`: PASS (1.201s).
- `CGO_ENABLED=0 go test -count=1 ./internal/cli -run
  '^TestPrimeScripts_SessionStartIngestsWhenCatalogUnavailable$'`: PASS
  (1.086s).
- `CGO_ENABLED=0 go test -count=1 -timeout=30s ./internal/index -run
  '^TestConsolidate_RetryAfterAbruptPostMergeExit$'`: PASS (0.459s).
- The requested race attempts were not green as a repository-health claim:
  the focused CLI race invocation failed under the concurrent runner's
  one-second detached-process deadlines, while the index race package was
  interrupted after 112.890s amid cross-test consolidated-lock contention.
  These are recorded as environment/runner evidence, not attributed to the
  candidate commits. The same three focused tests pass without race, as shown
  above.

## Verdict

Reject the dirty ingest payload until the deleted hook, catalog-failure, raw
message-row, search-field, and concurrent-session assertions are restored or
replaced by equally strong behavioral checks. Reject `10a7c19` as a functional
fix; it is a six-line behavior-preserving expansion. The remaining candidate
lanes are narrowed or clean, with the two duplicate patch pairs explicitly
deduplicated rather than treated as new behavior.

Net offense: `+6` unnecessary production lines in `10a7c19`; `-188` dirty test
lines in the ingest payload, including confirmed contract coverage loss.
