# Conor rival-branch audit

Audit posture: report-only. Rival claims were checked against immutable commits
and live worktrees. No Go files were edited; `gofmt` was not applicable.

## Ranked confirmed criticisms

### 1. HIGH — d9474fb/ed1527e turns a fail-soft hook into a FIFO hang

- **Worktree/SHA:** `lenny/luna-conor-audit-20260826`,
  `d9474fb6cc076e55a13a2116705c225ca3b9f2a8` (same patch-id as `ed1527e`).
- **Receipt:** `.agent-mailbox-cc/20260826T094028Z-conor-raid-receipt-lenny-the-scoreboard-is-eating-the-stage.md` says
  “the hook follow-up d9474fb removed 14 production lines” and “focused
  integrated race count three passed in 9.693s.”
- **Evidence:** `internal/cli/setup.go:64` and `:151` use
  `(set -C; : > "$entry") 2>/dev/null || [ ! -e "$entry" ]`. With `$entry`
  an existing FIFO, the redirection opens the FIFO and blocks before the
  fallback can run. Exact reproduction with `timeout 1` under both `/bin/sh`
  and `/bin/dash` returned status `124`.
- **Observed gate:** `CGO_ENABLED=0 go test -race -count=1 -run
  'TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest|TestClaudePrimeScript_ExecutesDetachedIngest|TestCodexPrimeScript_ExecutesDetachedIngest'
  ./internal/cli` passed in 3.935s, but does not cover special files.
- **Minimal correction:** restore the explicit `claimed=0` / `set -C` /
  `elif [ -e "$entry" ]` branch from `821b78d`; do not probe a failed
  noclobber open with a potentially blocking `-e` path.
- **Roast:** A three-run race green is not a safety proof when a FIFO can park
  SessionStart before the first assertion.

### 2. MEDIUM — 5610f95's “-223” is review-document deletion, not production value

- **Exact SHA:** `5610f95d7d8c9865ed6125e273c9f64989416a67`.
- **Evidence:** `git show --stat` reports only deletion of
  `FINDINGS-OZZY-INTEGRATION.md` (105 lines) and `FINDINGS-OZZY-REPRO.md`
  (118 lines): `0` additions, `223` documentation deletions, and no `.go`
  path. Commit message: `docs: drop branch-local hostile review artifacts`.
- **Observed command:** `git diff --numstat 4de36dc..5610f95` returned
  `0 105` and `0 118`.
- **Minimal correction:** report `-223 review-document lines`, or retain the
  artifacts if they remain part of the audit trail; do not count this as a
  production simplification.
- **Roast:** The scoreboard ate two Markdown files and announced a code win.

### 3. MEDIUM — conor/raid-norm-catalog is a dirty five-line idea, not a clean raid

- **Worktree/state:** `/Users/jay-m4/code/rawclaw-conor-store-demolition`,
  `conor/raid-norm-catalog` at `76faabb92edc9ef731d27eea73c1ff5fe0829749`,
  dirty: `M internal/agentproto/agentproto.go`.
- **Evidence:** the uncommitted diff removes the one-caller `allowed` closure
  and inlines `projects != nil && !slices.Contains(...)`. The branch's only
  commit is a 28-line `FINDINGS.md`; `git diff --stat 0d60b4c..76faabb` has no
  production change. The proposed cut was never committed or pushed.
- **Duplication:** `0d60b4c` has stable patch-id
  `3bda55e08c82637782e1eb1a6da3cabe73da910f`, exactly matching existing
  `6e9bf89` (`conor/store-demolition`).
- **Observed commands:** `git status --short --branch`, `git diff`,
  `git diff --stat 0d60b4c..76faabb`, and `git show --stat`.
- **Minimal correction:** commit only the fenced `agentproto.go` cut after its
  focused test, or mark the lane report-only; remove duplicate base work from
  novelty counts.
- **Roast:** Conor's “production raid” is a dirty worktree riding an already
  replayed patch.

### 4. MEDIUM — container takeover 0193241 is the same test patch as 8824e25

- **Exact SHA/state:** `/Users/jay-m4/code/rawclaw-conor-container-takeover`,
  `conor/container-takeover` at `0193241b6ce317ec0c931e6160b8e82b21f48161`,
  clean.
- **Evidence:** stable patch-id for `0193241` and existing `8824e25` is
  `2aecad9542d47c189738a422ebccedbe0736a920`. Both add
  `TestEnsureFreshContainer_PreservesRefreshDBOnPublishFailure`; `0193241`
  adds no production code. The same test is already in
  `ozzy/flash-refresh-cleanup` at `89c8a28`.
- **Observed gate:** `CGO_ENABLED=0 go test -race -count=1 -run
  '^TestEnsureFreshContainer_PreservesRefreshDBOnPublishFailure$'
  ./internal/index` passed in 4.155s. This verifies the test, not a novel
  takeover.
- **Minimal correction:** classify `0193241` as duplicate test coverage and
  remove it from the new-work ledger.
- **Roast:** The container takeover took over a test that already had an owner.

### 5. MEDIUM — the “three Luna raids” receipt is not live branch evidence

- **Receipt:** `.agent-mailbox-cc/20260826T094022Z-33c63bba-conor-heartbeat-8-lenny-heckle.md`
  claims `Luna pulse=0/6` while listing six Luna receipts; the next receipt
  says “Three Luna raids are now inside benchmark, store, and ambiguity files.”
- **Evidence:** live ref verification found only
  `luna/conor-31-log-tests-20260826` at `d5d036b9dd94...`; the listed
  `luna/conor-32-repro-a`, `...-b`, `luna/conor-pr35-hooks-audit`,
  `...resolution-audit`, and `...containers-audit` refs were missing. The
  mailbox's `live=0` fields are consistent with no checked-out live work.
- **Observed command:** `git branch -a --list 'luna/conor-*' 'origin/luna/conor-*'`
  plus `git rev-parse --verify` for each named ref.
- **Minimal correction:** publish or provide immutable SHAs for every claimed
  raid; otherwise mark those receipts `UNVERIFIABLE`.
- **Roast:** A pulse of zero is not three raids; it is a receipt ledger asking
  for source control.

## Clean / win findings

1. **CLEAN — phase-logger modularity cut.** `conor/raid-lenny-modularity`
   contains real code commit `43b183a193d5228b47c5403dc48cb4ef5c284ef5`,
   extracting `beginConsolidatePhase` (28 additions, 40 deletions) while
   preserving source attributes and phase names. The focused race gate
   `CGO_ENABLED=0 go test -race -count=1 -run
   'TestConsolidate_LogsPhaseStartsAndDurations|TestConsolidatedFence_LogsAcquireDurationOnTimeout'
   ./internal/index` passed in 2.912s.

2. **CLEAN — the 0193241 test is behaviorally useful.** Its focused race test
   passed in 4.155s and proves a failed consolidated publish leaves the
   refreshed per-container DB available for retry. The criticism is only that
   Conor presents duplicate coverage as a new takeover.

## Commands and scope

- `mnemon --store rawclaw recall` for Conor, hook catalog, container cleanup,
  and agentproto catalog filtering: observed prior decisions, including the
  known FIFO risk and duplicate patch history.
- `graphify reflect --if-stale`, `LESSONS.md`, and literal graph queries /
  explain / path were used for catalog, hooks, refresh containers, and
  agentproto orientation.
- Focused race tests listed above passed. FIFO probes under `/bin/sh` and
  `/bin/dash` both timed out with status 124.
- No broad race suite was run. No rival worktree was edited. `gofmt` was not
  applicable because this audit changed only this Markdown report. AGENTS.md
  was checked and intentionally left unchanged.

## Public ammunition

1. `d9474fb`: “Your 9.693s race green never opens an existing FIFO; the hook hangs before fail-soft can happen.”
2. `5610f95`: “-223 means two Markdown receipts deleted, not 223 production lines removed.”
3. `76faabb`: “The catalog raid is dirty, uncommitted, and based on a patch already present as 6e9bf89.”
4. `0193241`: “The container takeover is patch-id identical to 8824e25 and adds only a duplicate test.”
5. `094022Z`: “Pulse 0/6 plus five missing Luna refs is a receipt ledger, not three live raids.”
