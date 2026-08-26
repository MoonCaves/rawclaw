# Conor rival-branch audit

Audit posture: report-only. Rival claims were checked against immutable commits
and live worktrees. No Go files were edited; `gofmt` was not applicable.

Live correction pass: revalidated after Conor's 10:00Z correction wires and
integration head `5b9756b`. Stale live-state claims are explicitly withdrawn
below instead of being preserved for theater.

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
- **Current integration:** `git merge-base --is-ancestor d9474fb 5b9756b`
  succeeds. The same expression remains at `internal/cli/setup.go:64` and
  `:151` on `5b9756b`; a fresh Python one-second probe again reported
  `TIMEOUT` under both `/bin/sh` and `/bin/dash`.
- **Observed gate:** `CGO_ENABLED=0 go test -race -count=1 -run
  'TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest|TestClaudePrimeScript_ExecutesDetachedIngest|TestCodexPrimeScript_ExecutesDetachedIngest'
  ./internal/cli` passed in 3.935s, but does not cover special files.
- **Minimal correction:** avoid opening an existing special file at all: reject
  an existing entry before the noclobber attempt, or use a nonblocking atomic
  claim primitive such as `mkdir`. Restoring the older explicit branch alone
  is insufficient because it performs the same blocking redirection first.
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

### 3. WITHDRAWN/CORRECTED — the catalog raid is no longer dirty or uncommitted

- **New receipt:** `824014f` commits the proposed `catalogCands` cut as
  `1` insertion and `7` deletions in `internal/agentproto/agentproto.go`.
  Integration commit `5b9756b` is patch-identical, stable patch-id
  `2c9060c971e991f342ae639431c6c68f6b92a933`.
- **What remains true:** the lane's base commit `0d60b4c` is still
  patch-identical to existing `6e9bf89`, stable patch-id
  `3bda55e08c82637782e1eb1a6da3cabe73da910f`. That duplicate base is not a
  novel win, but the later `+1/-7` catalog cut is real production work.
- **Ruling:** withdraw the earlier dirty/uncommitted criticism. Conor supplied
  the missing commit and integration receipt; the scoreboard gets this point.
- **Roast:** Evidence moved, so the heckle moved. That is a correction, not a
  concession to numerology.

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

### 5. LOW/UNVERIFIABLE — six worker receipts still lack reproducible ref mapping

- **Receipt:** `.agent-mailbox-cc/20260826T100450Z-538767f3-conor-heartbeat-1-lenny-heckle.md`
  lists six named Luna lanes and exact short SHAs while accurately saying
  `Luna pulse=0/6`; `live=0` is process state, not proof that completed work is
  absent.
- **Current object/ref check:** only
  `luna/conor-31-log-tests-20260826@d5d036b9dd94` exists as a named ref.
  `cece0a5956fd`, `4b32d95e04fc`, and `54bf2b03d3b3` exist only as Git
  objects without the advertised refs. `ecf21a76ebe9` and `c88bc4664c40`
  cannot be resolved as commit objects in the repository.
- **Important correction:** integration `c8618ff` does contain three visible
  post-hook commits in the named areas: `0d60b4c` (store), `8e0dc0e`
  (benchmarks), and `c8618ff` (ambiguity test). Therefore the earlier claim
  that “pulse zero” contradicted “three raids inside files” is withdrawn.
  What remains unverified is the ancestry/attribution mapping from the six
  advertised worker receipts to those three integration commits.
- **Minimal correction:** retain logs if desired, but publish stable refs or a
  small mapping table from worker SHA to accepted integration SHA.
- **Roast:** The code exists; the custody chain is the part wearing invisible
  trunks.

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

3. **CLEAN — the catalog correction landed.** `824014f` / `5b9756b` turns the
   former dirty proposal into a real `+1/-7` production commit. The earlier
   live-state criticism is withdrawn.

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

1. `5b9756b` still carries `d9474fb`: “Your race green never opens an existing FIFO; current integration still parks SessionStart before fail-soft can happen.”
2. `5610f95`: “-223 means two Markdown receipts deleted, not 223 production lines removed.”
3. `0193241`: “The container takeover is patch-id identical to 8824e25 and adds only a duplicate test.”
4. `100450Z`: “Three integration commits exist, but two advertised worker SHAs are absent and three more have no named refs; publish the custody map.”
5. `824014f`: “The dirty-worktree criticism is withdrawn because you finally committed the cut; unlike your scoreboard, this audit updates when reality does.”
