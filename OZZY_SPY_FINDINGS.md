# Ozzy Flash Spy Dossier & Systemic Evidence Audit

Audit Date: 2026-08-26. Auditor: Norm's Gemini Flash Spy (`norm/ozzy-spy` at `/Users/jay-m4/code/rawclaw-norm-ozzy-spy`).
Base Commit: `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` (`cdc063d`).
Constraint Compliance: Strictly read-only examination of rival worktrees; no rival cursor advanced, reset, edited, or committed; zero Go code touched; only `OZZY_SPY_FINDINGS.md` created.

---

## 1. Executive Summary & Core Verdicts

A comprehensive multi-pane, worktree, git object database, and process state audit of all nine Ozzy Flash worktrees (`rawclaw-ozzy-flash-catalog`, `-cleanup`, `-hidden`, `-hook`, `-integration`, `-ponytail`, `-prune`, `-repro`, and `-spy`) reveals widespread discrepancies between terminal claims and immutable repository state:

1. **Spy Lane Fence Breach & Live Mutation:** `/Users/jay-m4/code/rawclaw-ozzy-flash-spy` committed 7 production and test Go files (`+209/-204` lines across `0d60b4c` and `d2e6aac`) despite claiming "production edits: none", and has an active background heartbeat process (`PID 89501`) constantly mutating `SPY_FINDINGS.md` uncommitted.
2. **Correctness Gap in Cleanup (TOCTOU Race):** `/Users/jay-m4/code/rawclaw-ozzy-flash-cleanup` (`89c8a28`) implements `isLockedOrActive` using `BEGIN IMMEDIATE; ROLLBACK`, releasing its lock prior to `os.Remove`, creating an unhedged time-of-check-to-time-of-use race where concurrent active writers have their database files unlinked.
3. **Phantom Commits & Missing Artifacts:** Multiple panes (`ozzy-flash-hidden`, `ozzy-flash-repro`, `ozzy-flash-catalog`) claim committed artifacts or specific commit SHAs (`d93f795`, `4de36dc`) that do not exist as HEAD in their respective worktrees (leaving trees stranded at clean `cdc063d` or carrying different files).
4. **Incomplete & Quota-Blocked State:** `ozzy-flash-prune` was aborted mid-benchmark due to Gemini quota exhaustion (`bdbd2fcc-64ea-4bfe-8336-5c54036771d2-164`), leaving a dirty 29-line uncommitted fixture in `internal/index/consolidated_test.go` with EOF whitespace issues.
5. **Zero Upstream Tracking:** None of the nine Ozzy worktrees has an upstream tracking branch configured (`fatal: no upstream configured`).

---

## 2. Ranked Findings with Evidence

### 1. CONFIRMED — Spy Worktree Committed Production Code and Has Uncommitted Heartbeat Mutation
- **Tag:** `management:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-spy`
- **Branch / HEAD SHA:** `ozzy/flash-spy-20260826` @ `63a64ffe4883a60a178e1b79bfe9a544e1403383`
- **Patch Hash (vs `cdc063d`):** `1737c690c4d9c6088f52fd50d1a829376797d158`
- **Net Lines (Committed vs `cdc063d`):** Net +5 lines (+209 additions / -204 deletions across 8 files: `SPY_FINDINGS.md`, `internal/cli/cmd_ingest_test.go`, `internal/cli/setup.go`, `internal/index/consolidated.go`, `internal/index/containers.go`, `internal/index/containers_test.go`, `internal/store/stats.go`, `internal/store/topics.go`).
- **Live Tree Status:** **DIRTY** (+52 / -58 uncommitted lines in `SPY_FINDINGS.md`).
- **Process Evidence:** Running loop `ozzy-spy-heartbeat-loop` (`PID 89501`) and watchdog `ozzy-spy-heartbeat-watchdog` (`PID 27258`).
- **Gate Evidence:** Go test gates not run on spy lane.
- **Classification:** **CONFIRMED**
- **Analysis:** `SPY_FINDINGS.md:1-7` declares the dossier to be report-only. However, commits `0d60b4c` (`refactor(store): share session count queries`) and `d2e6aac` (`refactor(store): share session ID row scanning`) modified core store and index packages. Furthermore, the active heartbeat process is mutating `SPY_FINDINGS.md` in-place, leaving the tree uncommitted and dirty.

---

### 2. CONFIRMED — Cleanup Writer Fence Has Probe-to-Unlink TOCTOU Race
- **Tag:** `delete:` / `yagni:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-cleanup`
- **Branch / HEAD SHA:** `ozzy/flash-refresh-cleanup` @ `89c8a284d20e4f6adba72accb3c0b34831a3b422`
- **Patch Hash (vs `cdc063d`):** `8bac5137bbc0a7cbd96924d09f3197e07997f93f`
- **Net Lines (Committed vs `cdc063d`):** +189 lines (+204 additions / -15 deletions across `internal/index/containers.go` [+61/-15] and `internal/index/containers_test.go` [+143/-0]).
- **Live Tree Status:** Clean (0 uncommitted).
- **Exact Code Evidence:** `internal/index/containers.go:78-113`:
  - `isLockedOrActive` (lines 93-108) issues `BEGIN IMMEDIATE; ROLLBACK;` against the candidate refresh DB.
  - The lock is immediately relinquished upon `ROLLBACK` at line 105.
  - `removeRefreshDB` (lines 86-90, 110-113) executes `os.Remove(path)`, `os.Remove(path+"-wal")`, and `os.Remove(path+"-shm")` without holding any lock.
- **Gate Evidence:** `CGO_ENABLED=0 go test -race -v -run 'Test(EnsureFreshContainer|PrepareFreshContainer|EnsureIndexedContainers)' ./internal/index` (1.192s).
- **Classification:** **CONFIRMED**
- **Analysis:** A concurrent worker thread or external process can acquire the SQLite database lock in the microsecond window between line 105 (`ROLLBACK`) and lines 110-113 (`os.Remove`). The unit test at `containers_test.go:710-805` only tests that an actively held open transaction prevents deletion during the probe; it completely fails to test the probe-to-unlink window. Unsafe for production transplant without a held flock or atomic rename.

---

### 3. CONFIRMED — Prune Worktree Dirty, Quota-Exhausted, and Unfinished
- **Tag:** `shrink:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-prune`
- **Branch / HEAD SHA:** `ozzy/flash-prune-benchmark` @ `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`
- **Patch Hash (vs `cdc063d`):** None (0 committed diffs).
- **Net Lines (Uncommitted):** +29 lines in `internal/index/consolidated_test.go:2189-2217`.
- **Live Tree Status:** **DIRTY** (`M internal/index/consolidated_test.go`).
- **Pane Evidence:** Tmux pane `ozzy-flash-prune` captured:
  `⚠ Individual quota reached. Please upgrade your subscription to increase your limits. Resets in 11m5s.`
  `Error ID: bdbd2fcc-64ea-4bfe-8336-5c54036771d2-164`
- **Gate Evidence:** Benchmark aborted mid-execution (`gofmt -l internal/ && go test -bench=BenchmarkPruneT...`). `git diff --check` fails on trailing blank line at EOF (line 2217).
- **Classification:** **CONFIRMED**
- **Analysis:** Abandoned mid-flight due to LLM quota exhaustion. Contains an unrequested, uncommitted benchmark fixture (`BenchmarkPruneTombstonedIDs`) with a 2,000-ID synthetic fixture. No results or reports were committed.

---

### 4. CONFIRMED — Repro Worktree Stranded at Base; Committed SHA Is Detached Phantom
- **Tag:** `management:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-repro`
- **Branch / HEAD SHA:** `ozzy/flash-repro-review` @ `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`
- **Patch Hash (vs `cdc063d`):** None (0 committed diffs).
- **Live Tree Status:** Clean (0 uncommitted).
- **Pane Evidence:** Tmux pane `ozzy-flash-repro` claims:
  - Committed Artifact: `FINDINGS-OZZY-REPRO.md` in commit `d93f795`.
  - Mnemon Memory ID: `4ddabd36-e87f-438b-8523-d7eb7133c2ba`.
  - Timings: 5 runs from 7.38s to 10.22s.
- **Git Object Evidence:** Commit `d93f795547bc2bccd9c7bfaee83ff05d867b86ff` exists in git object storage on top of `8824e25`, but the worktree `rawclaw-ozzy-flash-repro` was left pointing at base `cdc063d` without `FINDINGS-OZZY-REPRO.md`.
- **Classification:** **CONFIRMED**
- **Analysis:** The repro worktree never had the committed artifact placed on its branch HEAD. The branch remains at unmodified `cdc063d`.

---

### 5. CONFIRMED — Hidden-Pipelines Worktree Lacks Committed Report Artifact
- **Tag:** `management:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-hidden`
- **Branch / HEAD SHA:** `ozzy/flash-hidden-pipelines` @ `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`
- **Patch Hash (vs `cdc063d`):** None (0 committed diffs).
- **Live Tree Status:** Clean (0 uncommitted).
- **Pane Evidence:** Tmux pane `ozzy-flash-hidden` displays terminal report claiming:
  - "Audit Target File: tagrefresh.go (Verified clean...)"
  - "Unit & Race Tests: CGO_ENABLED=0 go test -race -v -run 'TestRunTag' ./internal/cli -> PASS (167s...)"
  - "Finish SHA: cdc063d058cc775ec2ee45a4231d8458ad3e9d43"
- **Classification:** **CONFIRMED**
- **Analysis:** Terminal prose was dumped into the pane but never persisted to disk as a markdown artifact or committed to git. Tree is identical to `cdc063d`. The 167s test execution claim cannot be verified from immutable tree artifacts.

---

### 6. CONFIRMED — Integration Worktree Name / Artifact Identity Mismatch
- **Tag:** `management:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-integration`
- **Branch / HEAD SHA:** `ozzy/flash-integration-review` @ `472c489115772df4bc486392da7dcc6d34aef32e`
- **Patch Hash (vs `cdc063d`):** `836e71c7981e9a5d7fe84006b332b3a747d1ec56`
- **Net Lines (Committed vs `cdc063d`):** +212 lines (`FINDINGS-OZZY-CATALOG.md`).
- **Live Tree Status:** Clean (0 uncommitted).
- **Pane vs Disk Conflict:**
  - Pane `ozzy-flash-integration` claims: "Review documented in committed file FINDINGS-OZZY-INTEGRATION.md (SHA: 4de36dc)".
  - Actual committed file in tree: `FINDINGS-OZZY-CATALOG.md` in commit `472c489`.
- **Classification:** **CONFIRMED**
- **Analysis:** The integration worktree contains the catalog audit (`FINDINGS-OZZY-CATALOG.md`), while its pane reports `FINDINGS-OZZY-INTEGRATION.md` (`4de36dc`). The commit `4de36dc` was committed on another branch (`integrate/tagwrite-closeout-wave1`) and not set as HEAD here.

---

### 7. CONFIRMED — Catalog Worktree Duplication and Stale Base
- **Tag:** `management:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-catalog`
- **Branch / HEAD SHA:** `ozzy/flash-catalog-review` @ `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`
- **Patch Hash (vs `cdc063d`):** None (0 committed diffs).
- **Live Tree Status:** Clean (0 uncommitted).
- **Pane Evidence:** Claims report written to `FINDINGS-OZZY-CATALOG.md` on branch `ozzy/flash-integration-review` at commit `472c489`.
- **Classification:** **CONFIRMED**
- **Analysis:** Complete operational duplication: `ozzy-flash-catalog` has 0 local commits and references the branch and commit of `ozzy-flash-integration`.

---

### 8. CONFIRMED — All Ozzy Branches Lack Upstream Tracking Configuration
- **Tag:** `management:`
- **Affected Trees:** All 9 worktrees.
- **Evidence:** `git rev-list --left-right --count @{upstream}...HEAD` produces:
  `fatal: no upstream configured for branch 'ozzy/...'` across all 9 branches.
- **Classification:** **CONFIRMED**
- **Analysis:** No branch has pushed its refs to origin or established upstream tracking.

---

### 9. PLAUSIBLE — Catalog Fallback Critical Vulnerability Lead
- **Tag:** `stdlib:` / `native:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-integration`
- **Committed Document:** `FINDINGS-OZZY-CATALOG.md` (commit `472c489`)
- **Claimed Location:** `internal/agentproto/agentproto.go:1770-1823` & `internal/cli/cli.go:511-520`
- **Claim:** When project narrowing is applied (`--this-project`), `catalogCands` returns `nil` when `tdir == ""` for a non-Claude candidate in the same directory. The subsequent fallback executes `sweepScopes` only across the narrowed Claude scope, silently suppressing the mixed-source collision.
- **Classification:** **PLAUSIBLE**
- **Status:** The structural logic in `agentproto.go` matches the defect description. However, because no reproducing test fixture was committed in `472c489`, this remains an advisory review finding rather than a verified regression test.

---

### 10. UNVERIFIED — Ponytail Audit Massive Deletion Claims
- **Tag:** `shrink:` / `delete:` / `yagni:`
- **Worktree:** `/Users/jay-m4/code/rawclaw-ozzy-flash-ponytail`
- **Branch / HEAD SHA:** `ozzy/flash-ponytail-audit` @ `47d986f40a96ef9c55af53e51004d8e0342faf9d`
- **Committed Document:** `FINDINGS-OZZY-PONYTAIL.md` (+153 lines)
- **Claim:** Proposes deleting ~525 LOC in production code and ~300 LOC in test scaffolding across 14 areas.
- **Classification:** **UNVERIFIED / High Risk**
- **Analysis:** Proposes aggressive simplifications (such as stripping `isLockedOrActive` and deduplication helpers). No code modifications were implemented or tested against concurrency suites. Strictly advisory.

---

## 3. Every-Pane Scoreboard

| Worktree Path | Branch | Exact HEAD SHA | Tree Status | Upstream | Patch Hash (vs `cdc063d`) | Net LOC Delta (Prod / Test / Doc) | Primary Gate / Command Timing | Final Verdict |
|---|---|---|---|---|---|---|---|---|
| `rawclaw-ozzy-flash-catalog` | `ozzy/flash-catalog-review` | `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` | Clean | None | *(empty)* | 0 / 0 / 0 | Pane claims `internal/agentproto` 133.55s; no local commits | **UNVERIFIED / Duplicated with Integration** |
| `rawclaw-ozzy-flash-cleanup` | `ozzy/flash-refresh-cleanup` | `89c8a284d20e4f6adba72accb3c0b34831a3b422` | Clean | None | `8bac5137bbc0a7cbd96924d09f3197e07997f93f` | +46 / +143 / 0 | `go test -race -v -run 'Test(EnsureFreshContainer...)'` (1.192s) | **CONFIRMED TOCTOU Race Vulnerability** |
| `rawclaw-ozzy-flash-hidden` | `ozzy/flash-hidden-pipelines` | `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` | Clean | None | *(empty)* | 0 / 0 / 0 | Pane claims `TestRunTag` 167s; no committed report | **UNVERIFIED / Missing Report Artifact** |
| `rawclaw-ozzy-flash-hook` | `ozzy/flash-hook-review` | `9010fcca121576dfc47e058fa4127acbb5b4701f` | Clean | None | `cc85c664df60215cbc1c5134b92f8f1f4ca07e0d` | 0 / 0 / +264 | `go test -race -count=5 'TestPrimeScripts...'` (18.803s) | **REPORT COMMITTED (Valid report-only lane)** |
| `rawclaw-ozzy-flash-integration` | `ozzy/flash-integration-review` | `472c489115772df4bc486392da7dcc6d34aef32e` | Clean | None | `836e71c7981e9a5d7fe84006b332b3a747d1ec56` | 0 / 0 / +212 | Pane claims 3 package suites; committed `FINDINGS-OZZY-CATALOG.md` | **REPORT COMMITTED / Artifact Name Mismatch** |
| `rawclaw-ozzy-flash-ponytail` | `ozzy/flash-ponytail-audit` | `47d986f40a96ef9c55af53e51004d8e0342faf9d` | Clean | None | `32aed4e337cd9cbaf0f2300ed1dbd7bac9c0e3e8` | 0 / 0 / +153 | Anti-bloat audit; no code diffs | **REPORT COMMITTED / Advisory Only** |
| `rawclaw-ozzy-flash-prune` | `ozzy/flash-prune-benchmark` | `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` | **DIRTY** | None | *(empty)* | 0 / +29 (uncommitted) / 0 | Quota reached (`bdbd2fcc...`); benchmark aborted | **CONFIRMED Incomplete / Quota Blocked** |
| `rawclaw-ozzy-flash-repro` | `ozzy/flash-repro-review` | `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` | Clean | None | *(empty)* | 0 / 0 / 0 | Pane claims `d93f795` (7.38s - 10.22s); branch at base | **CONFIRMED Stranded / Phantom Commit** |
| `rawclaw-ozzy-flash-spy` | `ozzy/flash-spy-20260826` | `63a64ffe4883a60a178e1b79bfe9a544e1403383` | **DIRTY** | None | `1737c690c4d9c6088f52fd50d1a829376797d158` | -14 / +19 / +84 | Go gates not run; active heartbeat loop mutating disk | **CONFIRMED Fence Violation & Live Mutation** |

---

## 4. Safe Transplant Assessment

1. **Production Code:** **ZERO safe production transplants exist.**
   - Cleanup (`89c8a28`): Must be rejected due to probe-to-unlink TOCTOU in `removeRefreshDB`.
   - Spy (`63a64ff`): Must be rejected due to unreviewed production changes on a report branch.
   - Prune (`cdc063d`): Uncommitted, unreviewed test fixture with formatting defects.
2. **Review / Advisory Artifacts:**
   - `FINDINGS-OZZY-HOOK.md` (`9010fcc`): Safe to reference for hook deduplication context.
   - `FINDINGS-OZZY-CATALOG.md` (`472c489`): Safe to reference as an issue lead for catalog narrowing ambiguity, pending independent characterization tests.
   - `FINDINGS-OZZY-PONYTAIL.md` (`47d986f`): Safe for advisory complexity metrics.
