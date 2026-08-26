# Hostile Review Findings: Norm / Lenny Prewarm Takeover

## Review Scope & File Fence
- **Review Base:** `origin/integrate/tagwrite-closeout-wave1` (head commit: `479d14c`)
- **Rival Targets:**
  - `/Users/jay-m4/code/rawclaw-norm-prewarm` (`norm/prewarm-ponytail` around `ee187c2`, `7d5a6a5`)
  - `/Users/jay-m4/code/rawclaw-lenny-prewarm` (`d80081e`, `3bbfc1a`, `bcf6ca5`)
- **File Fence:**
  1. `FINDINGS.md`
  2. `internal/cli/cmd_prewarm.go`
  3. `internal/cli/tagrefresh.go`
  4. `internal/cli/tagrefresh_test.go`
  5. `internal/cli/setup.go`
  6. `internal/cli/setup_test.go`
  7. `internal/index/containers.go`
  8. `internal/index/containers_test.go`

---

## Candidates Evaluated

### 1. `internal/cli/cmd_prewarm.go:100-114` — Single COALESCE Query in `prewarmSourcePath`
- **Rival Commits:** `cc404551a32e426f14b6ccd0c5e70772e61699c4` (`luna/skills-cmd-prewarm-20260826`) / `a4e1a67aba368283548a9078f86def813b5c9542`
- **Candidate Proposal:** Merge sequential fallback queries (`session_sources` then `file_index`) into a single `COALESCE` query:
  ```go
  SELECT COALESCE(
      (SELECT source_path FROM session_sources WHERE session_id=? AND source_path != '' LIMIT 1),
      (SELECT path FROM file_index WHERE session_id=? LIMIT 1),
      ''
  )
  ```
- **Reproducible Evidence:** In legacy stores where `session_sources` table does not exist, SQLite fails query compilation with `no such table: session_sources` for the entire statement. The sequential query pattern is essential for fallback compatibility.
- **RULING:** `REJECT`

---

### 2. `internal/cli/cmd_prewarm.go:86` — Non-Atomic Prewarm Dump Write
- **Rival Commits:** `cc404551a32e426f14b6ccd0c5e70772e61699c4` (`luna/skills-cmd-prewarm-20260826`)
- **Candidate Proposal:** Replace `durable.WriteAtomic(dumpPath, content.Bytes())` with `os.WriteFile(dumpPath, content.Bytes(), 0o600)`.
- **Reproducible Evidence:** `os.WriteFile` is non-atomic and lacks directory sync. An interrupted process leaves a corrupted dump file. `durable.WriteAtomic` guarantees crash consistency.
- **RULING:** `REJECT`

---

### 3. `internal/cli/cmd_prewarm.go:48-59` — Removal / Bypassing of `LocateConsolidatedSession` Guard
- **Rival Commits:** `cc404551a32e426f14b6ccd0c5e70772e61699c4`, `d80081e9ea8f76bbda0c5347fc6ccb0eddbbe6b9`
- **Candidate Proposal:** Remove `LocateConsolidatedSession` lookup in `runPrewarmCmd` and fold unconditionally on `toFold`.
- **Reproducible Evidence:** Removing this lookup causes present sessions to be re-folded. Verified by `TestRunPrewarmExternalBehaviors/present_session_is_not_refolded`, which fails 3/3 (`consolidated message count = 2, want unchanged 1`).
- **RULING:** `REJECT` (Guard must remain).

---

### 4. `internal/cli/setup.go:903-920`, `940` & `internal/cli/setup_test.go:682,721` — Remove Impossible Error from `addRawclawAntigravityHooks`
- **Rival Commit:** `7d5a6a550dc018519cca8f106b86786597d66540` (`norm/prewarm-ponytail`)
- **Candidate Proposal:** Change `addRawclawAntigravityHooks(data map[string]any, scriptPath string) error` to return void, eliminating dead error handling branches in `installRawclawAntigravityHook` and tests.
- **Reproducible Evidence:** The function only performs in-memory map/slice manipulations and always returned `nil`. Removing the return value eliminates dead error handling code across 9 lines without changing behavior or editor hook schemas.
- **RULING:** `TRANSPLANT_EXACTLY`

---

### 5. `internal/cli/setup.go:antigravityHooksPath` — Alias to `codexHooksPath`
- **Rival Commit:** `ee187c2` (`norm/prewarm-ponytail`)
- **Candidate Proposal:** Alias `antigravityHooksPath` to `codexHooksPath`.
- **Reproducible Evidence:** While both point to `hooks.json`, they represent distinct editor integration seams. Maintaining separate helper functions prevents cross-target coupling.
- **RULING:** `REJECT`

---

### 6. `internal/cli/tagrefresh.go`, `internal/index/containers.go` — General Refactoring
- **Candidates Evaluated:** Replacing explicit closes with defers in tight loops, merging container resolution helpers.
- **Reproducible Evidence:** Existing implementations in `tagrefresh.go` and `containers.go` are already minimal and behavior-preserving. No safe net-negative reduction exists without violating lifecycle and freshness contracts.
- **RULING:** `CLEAN` / `NO CHANGE`

---

## Final Delta Summary
- **Transplanted:** Commit `fa485c8` (`refactor(cli): remove impossible antigravity hook error`).
- **Net Source Lines (excluding FINDINGS.md):** `-9` lines (5 insertions, 14 deletions).
