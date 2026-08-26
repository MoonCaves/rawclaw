# Hostile Integration Review: c14e806..cdc063d

**Target Scope:** `c14e806..cdc063d` (7 commits, 6 files, +461 / -43 lines)  
**Evaluator:** Antigravity (Adversarial Integration Auditor / Ponytail Inspector)  
**Posture:** Hostile REPORT-ONLY Review  
**Date:** 2026-08-26  

---

## 1. Executive Summary & Verdict

### Final Verdict: **ACCEPT WITH ADVISORIES (SHIP-READY)**

The integration wave `c14e806..cdc063d` consists of 7 commits covering three critical subsystems:
1. **Catalog Resolution & Source Semantics (`54afa70`, `bf7cdd0`, `cdc063d`):** Resolves a high-severity cross-source ambiguity defect where foreign catalog hits (Codex/Antigravity/Goose) previously bypassed prefix ambiguity checking when mixed with Claude sessions.
2. **Hook Prewarm & Deduplication (`92d0067`):** Implements atomic `set -C` (noclobber) deduplication for SessionStart ingest triggers in Claude and Codex hook templates, preventing process storms on rapid or concurrent session initialization.
3. **Consolidation Logging, Fence Timing & Fault Injection (`2ee9950`, `479d14c`, `e685556`):** Pins slog phase start and duration contracts across all 9 fold phases and 2 fence phases; corrects test isolation so child subprocesses share the parent store under fault injection; verifies SQLite WAL/SHM artifacts and lock recovery.

No product regressions, data-loss vulnerabilities, or race conditions were introduced by the final tip `cdc063d`. All targeted race suites pass cleanly (`internal/index` in 6.09s, `internal/cli` in 4.42s, `internal/agentproto` in 180.49s).

---

## 2. Review Matrix & Findings Table

| Severity | Commit SHA | File:Line | Category | Finding / Defect Summary | Net Lines |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **HIGH (Resolved)** | `54afa70` → `cdc063d` | `internal/agentproto/agentproto.go:1810-1818` | Source Semantics / Correctness | `54afa70` introduced a `continue` when encountering non-Claude catalog entries (`tdir == ""`), which caused mixed-source prefix matches (1 Claude + 1 Codex) to silently discard the Codex hit and resolve Claude without reporting ambiguity. Corrected in `cdc063d` by returning `nil` to abandon narrowing and fall back to `more()`. | 0 |
| **MEDIUM** | `92d0067` | `internal/cli/setup.go:64-88,155-179` | CLI / Bloat (Ponytail) | Duplicated dedup/claim/catalog logic between `rawclawPrimeScript` (Claude) and `rawclawCodexPrimeScript` (Codex) templates (28 lines identical script logic). | -24 |
| **LOW** | `internal/agentproto/agentproto.go:1799-1804` | `internal/agentproto/agentproto.go:1799-1804` | Bloat (Ponytail) | `allowed` helper closure in `catalogCands` wraps `slices.Contains`. Can be inlined directly in the loop. | -6 |
| **LOW** | `bf7cdd0`, `cdc063d` | `internal/cli/cmd_tag_onestore_test.go:94-98,127-131,175-179` | Bloat (Ponytail) | Test environment setup (`HOME`, `CLAUDE_CONFIG_DIR`, `RAWCLAW_CATALOG_DIR`) repeated verbatim across 3 new test functions. | -12 |
| **INFORMATIONAL** | `479d14c` | `internal/index/consolidated_test.go:1807-1813` | Test Isolation / Correctness | `TestConsolidate_FaultInjectionHelper` subprocess must receive parent `HOME` via `RAWCLAW_CONSOLIDATE_FAULT_HOME` because `TestMain` isolates each process's cache. Validated and pinned. | 0 |
| **INFORMATIONAL** | `e685556` | `internal/index/consolidated_fence_test.go:84-134` | Observability / Race Contract | `TestConsolidatedFence_LogsAcquireDurationOnTimeout` verifies deferred duration slog output on fence acquire failure. | 0 |
| **INFORMATIONAL** | `2ee9950` | `internal/index/consolidated_test.go:19-82` | Observability / Log Contract | `TestConsolidate_LogsPhaseStartsAndDurations` pins start/duration contracts for all 9 fold phases and 2 fence phases. | 0 |

---

## 3. Detailed Subsystem Analysis

### 3.1. Catalog Fallback & Source Semantics (`54afa70`, `bf7cdd0`, `cdc063d`)
- **Initial State:** `catalogCands` assumed all catalog hits had non-empty `paths.ProjectDirOf(hit.Path)`. For foreign sources (Codex, Antigravity, Goose), `hit.Path` lives outside `~/.claude/projects`, returning `tdir == ""`.
- **Intermediary Defect in `54afa70`:** When `tdir == ""`, `54afa70` executed `continue`. If a prefix query matched both a Claude transcript and a Codex transcript, `continue` dropped the Codex candidate from `narrowed`. If only 1 Claude candidate remained, `decideSession` resolved the Claude session as a unique match, silently masking the ambiguity collision with Codex.
- **Resolution in `cdc063d`:** When `tdir == ""` is encountered on any candidate, `catalogCands` immediately executes `return nil`. This abandons catalog-level scope narrowing and falls back to `more()` in `LocateSessionGuarded`, which performs a source-aware scan across all registered source stores and properly yields `ErrAmbiguousSession`.
- **Verification:** Pinned by `TestGuardedSessionLookupPreservesMixedSourceAmbiguity` and `TestGuardedSessionLookupUsesForeignPreResolvedScope`.

### 3.2. Session-Start Hook Deduplication & Prewarm Race Safety (`92d0067`)
- **Mechanism:** In `rawclawPrimeScript` and `rawclawCodexPrimeScript`:
  ```sh
  claimed=0
  if (set -C; : > "$entry") 2>/dev/null; then
      claimed=1
  elif [ -e "$entry" ]; then
      exit 0
  fi
  ```
- **Atomicity & Fail-Soft Properties:**
  - `set -C` (POSIX `noclobber`) in a subshell ensures atomic file creation (`O_CREAT | O_EXCL`).
  - If `$entry` exists, the subshell fails, `claimed` stays `0`, `elif [ -e "$entry" ]` catches the existing file and exits `0` immediately, suppressing duplicate background ingest and duplicate discovery banners.
  - Winner writes JSON to PID-specific `$tmp_entry` in the same directory and performs atomic `mv -f "$tmp_entry" "$entry"`.
  - If a crash occurs before `mv`, the 0-byte `$entry` still exists and serves as a valid dedup marker.
  - Background ingest (`nohup "$RAWCLAW" ingest "$session_id"`) is guarded strictly by `if [ "$claimed" -eq 1 ]`.
- **Verification:** Pinned by `TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest` with concurrent subshell race execution.

### 3.3. Fault Injection, Store Sharing & Phase Logging (`2ee9950`, `479d14c`, `e685556`)
- **Fault Injection (`479d14c`):**
  - Fixes Issue #32 test fidelity: `RAWCLAW_CONSOLIDATE_FAULT_HOME` passes the parent test's isolated `HOME` into the child process helper so child and parent share the exact same `consolidated.db` and lock file.
  - Verifies presence of WAL/SHM artifacts and `consolidated.lock` after child abrupt exit (exit code 124).
  - Mutates the source DB before parent retry consolidation to ensure retry is not a watermark no-op (`st.Messages == 2`).
- **Phase Logging Contracts (`2ee9950`, `e685556`):**
  - Verifies typed slog logging (`duration: slog.KindDuration` and `event: "start"`) for all fold phases (`schema-migrate`, `source-migrate`, `attach`, `prepare`, `merge`, `detach`, `tombstone-prune`, `watermark-stamp`, `connection-close`) and fence phases (`acquire`, `release`).
  - Verifies acquire duration is logged even on context timeout.

---

## 4. Ponytail Audit & Optimization Opportunities

Applying the Ponytail Ladder (`delete:`, `stdlib:`, `native:`, `yagni:`, `shrink:`):

1. `internal/agentproto/agentproto.go:1799-1804`:
   `shrink: 6-line allowed helper closure. Inline as: if projects != nil && !slices.Contains(projects, hit.Project) { continue }` (Saves 6 lines).
2. `internal/cli/cmd_tag_onestore_test.go:94-98,127-131,175-179`:
   `shrink: repeated 5-line tempdir and env setup. Extract helper setupCatalogTestEnv(t) (string, string)` (Saves 12 lines).
3. `internal/cli/setup.go:64-88,155-179`:
   `shrink: duplicate catalog dedup shell snippets between Claude and Codex templates. Substitute @@RAWCLAW_CATALOG_HOOK@@` (Saves 24 lines).

**Total Net Reduction Opportunity:** **-42 lines**

---

## 5. Focused Race Test Verification

All focused tests executed with `CGO_ENABLED=0 go test -race -count=1`:

| Package | Test Command | Result | Duration |
| :--- | :--- | :--- | :--- |
| `internal/index` | `go test -race -count=1 -run '^(TestConsolidate_LogsPhaseStartsAndDurations\|TestConsolidate_RetryAfterAbruptPostMergeExit\|TestConsolidatedFence_LogsAcquireDurationOnTimeout)$' ./internal/index` | **PASS** | 6.088s |
| `internal/cli` | `go test -race -count=1 -run '^(TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest\|TestGuardedSessionLookupDoesNotTreatForeignCatalogPathAsClaude\|TestGuardedSessionLookupUsesForeignPreResolvedScope\|TestGuardedSessionLookupPreservesMixedSourceAmbiguity)$' ./internal/cli` | **PASS** | 4.422s |
| `internal/agentproto` | `go test -race -count=1 ./internal/agentproto/...` | **PASS** | 180.492s |

---

## 6. Recommendations for Follow-up Waves

1. **Test Helper Extraction:** Introduce a shared catalog test setup helper in `cmd_tag_onestore_test.go` to eliminate repeated environment setup boilerplate.
2. **Template Deduplication:** Unify the session-start shell deduplication logic in `internal/cli/setup.go` via a template placeholder.
3. **No Main Merge:** Adhere strictly to the directive: do not merge into `main`. The branch `integrate/tagwrite-closeout-wave1` remains clean and verified.
