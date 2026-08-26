# Issue 31/32 rival audit

Date: 2026-08-26. Evidence-only audit from the dedicated `norm/conor-spy`
worktree. No issues, production code, or rival trees were edited. `gofmt -w`:
N/A (report-only change).

## Authoritative scope

GitHub Issues #31 and #32 are CLOSED. Issue #31's body requires start and
elapsed logs for fence acquisition, schema migration, source migration, attach,
prepare, merge, DETACH, tombstone pruning, watermark stamping, and connection
close; its acceptance gate is `CGO_ENABLED=0 go test -race -count=1
./internal/index/...` plus clean gofmt. The final comment says `2ee9950` closed
the contract, but this is issue metadata, not a fresh local gate.

Issue #32's body requires a test-only forced exit after merge and before DETACH,
then a second fold of the same source. Its comments correct the original
`c14e806` evidence: child `TestMain` changed HOME, so it was not a same-store
test. `fd01a92` and integration `479d14c` add HOME restoration, committed-state
checks, source mutation, and a second-message assertion. The final authoritative
comment reports five retries around 115–329 ms without reproducing the claimed
multi-second stall.

## Findings

### 1. CONFIRMED: `d5d036b` is a test deletion, not an Issue-31 implementation

`d5d036b9dd94c59a9ee3da2da8fb8d1039cb671d` has parent `2bb219f` and deletes
57 lines from `internal/index/consolidated_logging_test.go`. On that checked-out
tree, `CGO_ENABLED=0 go test -list
'TestConsolidated|TestConsolidate_Logs' ./internal/index` listed only
`TestConsolidatedFence_ReportsHolderOnceAfterThreshold` and
`TestConsolidatedFence_LogsAcquireDurationOnTimeout`; no
`TestConsolidate_LogsPhaseStartsAndDurations` exists there. Therefore the
worker's focused command can pass while not exercising the full Issue-31 fold
phase contract. Production net: 0. Test net: -57 lines in this commit. Doc
net: 0. Tags: `delete`, `yagni`; deleting duplicate coverage is reasonable only
after the integrated `2ee9950` contract test is the artifact being gated.

### 2. CONFIRMED: corrected Issue-32 same-store test passes, but only proves a negative reproduction

On `/Users/jay-m4/code/rawclaw-luna-conor-32b` at
`cece0a5956fd7692746415ffe67b1db25e093bff`, I ran:

`/usr/bin/time -p env CGO_ENABLED=0 go test -race -count=5 -shuffle=on ./internal/index -run '^TestConsolidate_RetryAfterAbruptPostMergeExit$' -v`

Observed result: PASS, package `3.484s`, wall `3.99s`; retry duration in the
verbose output was `143.494833ms`. The test output showed child exit after merge,
then the second fold's DETACH, tombstone-prune, watermark-stamp, connection-close,
and fence-release phases. Commit `cece0a5` adds same-source mutation and the
two-message assertion at `internal/index/consolidated_fault_test.go:90-127`.
Production net: 0. Test net: +28/-2 in the commit (+133 in the worker's
comparison). This is a real, non-vacuous test improvement; it does not prove
that an unrelated production multi-second stall is impossible under all load.

### 3. CONFIRMED: original `c14e806` and corrected `fd01a92`/`479d14c` are distinct evidence

`c14e806` exits before DETACH but used a different child HOME under `TestMain`.
`fd01a922a6826cb4b6d0e4de45e1403d422e114e` adds the same-store correction; its
patch is carried unchanged as `479d14c782a229d3348b290885028c5efa7a8740` on
the integrated branch. Counting both as independent implementations is wrong:
their subject and file diff are identical (`internal/index/consolidated_test.go`,
`+60/-2`). Patch-id deduplication should count this correction once.

### 4. CONFIRMED: current Conor hook candidate fixed the prior directory trap

`/Users/jay-m4/code/rawclaw-conor-ambiguity-contract` at
`6d20bda` changes the claim to a temporary directory plus
`ln "$tmp_entry" "$catalog_dir"` at `internal/cli/setup.go:84-95` and adds
directory, symlink-directory, and detached-ingest assertions at
`internal/cli/catalog_hook_test.go:163-220`. I ran:

`env CGO_ENABLED=0 go test -race -count=3 -shuffle=on ./internal/cli -run '^TestPrimeScripts_CatalogClaimNeverOpensExistingSpecialPath$' -v`

Observed PASS: package `32.913s`, wall `34.28s`, covering `sh`, `dash`, and
`bash`, Claude/Codex, regular/FIFO/directory/symlink/socket variants. This is a
credible fix of the earlier `2cc11d6` directory-destination counterexample.
Production/test net in `6d20bda`: `+178/-36`; no doc net. It is not an
Issue-31/32 change and must not be scored as one.

### 5. PLAUSIBLE: worker full-gate receipts remain incomplete or unstable

Luna worker trees have untracked runtime artifacts (`.agent-mailbox/`,
`.codex-run.log`, `.codex-final-message.txt`; some also `graphify-out/`) and no
upstream tracking refs. The Issue-31 worker receipt claimed focused race count
five and package race, but the local test inventory above proves its deleted
fold-contract test was absent. The Issue-32A receipt claimed package green while
its log recorded SQLite `out of memory (14)` and package FAIL at `172.083s`.
The corrected Issue-32B result is the defensible sibling result; do not merge
these receipts into one “all green” conclusion.

### 6. UNVERIFIED: newer adjacent Conor candidates are outside Issues 31/32

Current heads include `b944d08`/`fb893ed` range-resolution work and
`35bc58f`/`0d1da19` claim-spy or hook follow-ups. Their branch stats and mailbox
claims may be useful for adjacent audits, but I did not run their full CLI gates
and did not treat their reports as Issue-31/32 evidence. A mailbox wire claims
`b944d08` passed 900,000 parent/refactor cases and five tag/prep/write shuffles
in `95.77s`, while an unrelated detached-child test remained red; this is
explicitly not a full-suite green claim.

## Gate ledger

| Artifact | Command / timing | Local verdict |
|---|---|---|
| Issue #31 Luna tree | `go test -list 'TestConsolidated|TestConsolidate_Logs'` | PASS command, but fold timing test absent |
| `cece0a5` Issue #32 | `CGO_ENABLED=0 go test -race -count=5 -shuffle=on ...Retry...` | PASS, 3.484s package / 3.99s wall; retry 143.494833ms |
| `6d20bda` hook candidate | `CGO_ENABLED=0 go test -race -count=3 -shuffle=on ...SpecialPath...` | PASS, 32.913s package / 34.28s wall |
| Issue #31/#32 repository-wide gates | not run by this spy | UNVERIFIED |

## Verdict

The strongest defensible deduction is narrow: Issue #32's corrected same-store
test is real and non-vacuous, and it repeatedly fails to reproduce the
multi-second stall; Issue #31's `d5d036b` deletion cannot itself demonstrate
the fold-phase contract because that test is absent from the worker tree. Count
`fd01a92`/`479d14c` once by patch-id, retain the negative reproduction as
bounded evidence, and require a fresh full gate before treating any worker
receipt as repository green.
