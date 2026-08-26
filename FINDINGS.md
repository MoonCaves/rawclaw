# Hostile Review Findings: Hooks, Prewarm, and Setup

**Date:** 2026-08-26  
**Target Codebase:** RawClaw (`github.com/MoonCaves/rawclaw`)  
**Base Commit:** `479d14c782a229d3348b290885028c5efa7a8740` (`lenny/raid-hooks-20260826`)  
**Scope:** SessionStart hooks, prewarm dump atomicity, hook deduplication, and setup refactoring across Claude, Codex, and Antigravity.

---

## Executive Summary & Candidate Matrix

| # | Candidate / Area | Rival SHA | File:Line | Reproducible Evidence | RULING |
|---|------------------|-----------|-----------|------------------------|--------|
| 1 | **PR35 Hook Ingest Dedup** | `a017b6e`, `1c9c48a` | `internal/cli/setup.go:62-84,148-170` | SessionStart hook scripts previously launched detached ingest *before* verifying the catalog entry file `$entry`, spawning redundant background ingest jobs on repeated starts. Moving `nohup "$RAWCLAW" ingest` after `$entry` write eliminates duplicate spawns without suppressing needed ingest. | **ADAPT_TO_CURRENT** |
| 2 | **Atomic Prewarm Cache Publishing** | `71067bd` | `internal/cli/cmd_prewarm.go:88-100`, `internal/durable/durable.go:261-272` | Direct `os.WriteFile` to prewarm dumps and `.state` fingerprints exposes concurrent readers (tag-prep / concurrent agents) to partial writes. Exporting and using `durable.WriteAtomic` ensures atomic POSIX rename and fsync. | **ADAPT_TO_CURRENT** |
| 3 | **Hook Wiring Helper Unification** | `6b37f40` | `internal/cli/setup.go:658-795,930-960` | Installation and ejection routines for Claude, Codex, and Antigravity duplicated file removal, config merging, and empty directory pruning. Extracting `installRawclawHookWith` and `ejectRawclawHookWith` eliminates ~80 lines of duplicate plumbing while preserving exact JSON semantics. | **ADAPT_TO_CURRENT** |
| 4 | **Remove Impossible Antigravity Error** | `7d5a6a5` | `internal/cli/setup.go:900-920` | `addRawclawAntigravityHooks` unconditionally returned `nil` error; removing the dead error return simplifies callers and removes unreachable error paths. | **ADAPT_TO_CURRENT** |
| 5 | **Prewarm Refresh & SQL Simplification** | `cc40455` | `internal/cli/cmd_prewarm.go:40-75,100-115` | Branching in `runPrewarmCmd` duplicated `refreshTagSession` calls. Streamlining with `agentproto.LocateConsolidatedSession` and collapsing `prewarmSourcePath` into a single `COALESCE` query avoids redundant code and queries. | **ADAPT_TO_CURRENT** |
| 6 | **Collapse `antigravityHooksPath` into `codexHooksPath`** | `ee187c2` | `internal/cli/setup.go:860` | Proposal to alias `antigravityHooksPath` to `codexHooksPath` because both point to `hooks.json`. | **REJECT** (Distinct target seam documentation should remain explicit; sharing across disparate tool namespaces creates accidental coupling). |
| 7 | **Replace `prewarmSourcePath` with `store.SessionBackingFor`** | `ee187c2` | `internal/cli/cmd_prewarm.go:100` | Proposal to replace direct query with `store.SessionBackingFor`. | **REJECT** (`prewarmSourcePath` intentionally falls back to `file_index` for unindexed legacy sessions, which `SessionBackingFor` does not cover). |

---

## Detailed Findings

### Finding 1: PR35 Hook Ingest Dedup Contract
- **Rival SHA:** `a017b6e7041b3f9c7b8b573e0f394b09ba809228` / `1c9c48a932569dfad60263bfb6cef002e5d17012`
- **File & Line:** `internal/cli/setup.go:62-84`, `internal/cli/setup.go:148-170`, `internal/cli/cmd_ingest_test.go:133-205`
- **Mechanism:**  
  In `rawclawPrimeScript` (Claude) and `rawclawCodexPrimeScript` (Codex), `nohup "$RAWCLAW" ingest "$session_id"` was invoked at lines 63 and 149, before checking `if [ -f "$entry" ]; then exit 0; fi`.  
  On every SessionStart trigger for an existing session, a detached ingest process was spawned despite the catalog marker already being present.  
  Moving the `nohup "$RAWCLAW" ingest` invocation inside the block—after writing the temporary catalog entry and atomically moving it to `$entry`—ensures only the initial session registration spawns detached ingest.
- **Reproducible Evidence:**  
  `TestPrimeScripts_SessionStartDeduplicatesDetachedIngest` executes the rendered script twice in POSIX `sh` against a mocked `rawclaw` binary and asserts that `calls.log` receives exactly 1 call to `ingest <session_id>`.
- **Ruling:** **ADAPT_TO_CURRENT** (Adopts the fix across all prime script templates and brings in the regression test).

---

### Finding 2: Prewarm Dump Atomic Publishing
- **Rival SHA:** `71067bda7bdff81004b19598f5a80fb8da245ab0`
- **File & Line:** `internal/cli/cmd_prewarm.go:88-100`, `internal/durable/durable.go:261-272`
- **Mechanism:**  
  When writing closeout dumps and `.state` fingerprints, `runPrewarmCmd` called raw `os.WriteFile(dumpPath, ...)` and `os.WriteFile(dumpPath+".state", ...)`. Under high-concurrency multi-agent workloads, another process running `rawclaw tag` or `tag-prep` could read a partially-written dump file.  
  Using `durable.WriteAtomic(dst, data)` stages the file into a temporary sibling file, executes `fsync`, atomically renames it to destination, and syncs the parent directory.
- **Reproducible Evidence:**  
  `TestRunPrewarmRegeneratesWhenDumpMissing` verifies that if the dump file is deleted, `runPrewarmCmd` cleanly regenerates it atomically.
- **Ruling:** **ADAPT_TO_CURRENT** (Export `durable.WriteAtomic` and use it for prewarm dumps and state files).

---

### Finding 3: Hook Installation & Ejection Helper Unification
- **Rival SHA:** `6b37f4007d4b47e5b2257fbaebfdf91f2e153e7f`
- **File & Line:** `internal/cli/setup.go:658-795`, `internal/cli/setup.go:930-960`
- **Mechanism:**  
  `installRawclawHookAt` and `installRawclawAntigravityHook` share identical sequence logic: render script -> write hook script -> read JSON config -> mutate hook map -> write JSON config -> remove legacy script.  
  Similarly, `ejectRawclawHookAt` and `ejectRawclawAntigravityHook` share file removal, parent directory cascading pruning (`removeIfEmpty`), and config cleanup.  
  Unifying via `installRawclawHookWith` and `ejectRawclawHookWith` higher-order functions removes redundancy without changing byte output or schema definitions.
- **Ruling:** **ADAPT_TO_CURRENT** (Integrate clean helper parameterization).

---

### Finding 4: Dead Error Return in `addRawclawAntigravityHooks`
- **Rival SHA:** `7d5a6a550dc018519cca8f106b86786597d66540`
- **File & Line:** `internal/cli/setup.go:900-920`, `internal/cli/setup_test.go:679-725`
- **Mechanism:**  
  `addRawclawAntigravityHooks(data map[string]any, scriptPath string)` performs in-memory map mutations and always returns `nil`. Returning `error` forced callers and test functions to include dead `if err != nil` assertions.
- **Ruling:** **ADAPT_TO_CURRENT** (Remove unreachable error return).

---

### Finding 5: Consolidated Prewarm Refresh Flow
- **Rival SHA:** `cc404551a32e426f14b6ccd0c5e70772e61699c4`
- **File & Line:** `internal/cli/cmd_prewarm.go:40-75`, `internal/cli/cmd_prewarm.go:100-115`
- **Mechanism:**  
  `runPrewarmCmd` previously branched on `locatePrewarmStore`, calling `refreshTagSession` in both branches with differing parameter bindings. Unifying the call and conditioning only the `SyncConsolidatedFrom` step on `locateErr != nil` simplifies the execution path. In addition, `prewarmSourcePath` combines two fallback SELECT queries into a single `COALESCE` query.
- **Ruling:** **ADAPT_TO_CURRENT** (Incorporate streamlined flow).

---

### Finding 6: Cross-Target Hook Path Aliasing
- **Rival SHA:** `ee187c2118302a11ed4473c694397c4063515a76`
- **File & Line:** `internal/cli/setup.go:860`
- **Mechanism:** Proposal to delete `antigravityHooksPath` and reuse `codexHooksPath`.
- **Ruling:** **REJECT** (Distinct target seam documentation should remain explicit; sharing across disparate tool namespaces creates accidental coupling).

---

### Finding 7: Replacing `prewarmSourcePath` with `store.SessionBackingFor`
- **Rival SHA:** `ee187c2118302a11ed4473c694397c4063515a76`
- **File & Line:** `internal/cli/cmd_prewarm.go:100`
- **Mechanism:** Proposal to replace `prewarmSourcePath` with `store.SessionBackingFor`.
- **Ruling:** **REJECT** (`prewarmSourcePath` intentionally falls back to `file_index` for unindexed legacy sessions, which `SessionBackingFor` does not cover).
