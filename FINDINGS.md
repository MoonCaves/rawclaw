# Hostile Review Findings: Prewarm & Setup Area

## Candidates Evaluated

### 1. `internal/cli/cmd_prewarm.go:102-114` — Single COALESCE Query in `prewarmSourcePath`
- **Rival Commit:** `cc404551a32e426f14b6ccd0c5e70772e61699c4` (`luna/skills-cmd-prewarm-20260826`) / `a4e1a67aba368283548a9078f86def813b5c9542`
- **Candidate Proposal:** Replace sequential fallback queries (`session_sources` then `file_index`) with a single `COALESCE` query:
  ```go
  SELECT COALESCE(
      (SELECT source_path FROM session_sources WHERE session_id=? AND source_path != '' LIMIT 1),
      (SELECT path FROM file_index WHERE session_id=? LIMIT 1),
      ''
  )
  ```
- **Reproducible Evidence:** In legacy SQLite stores where the `session_sources` table does not exist, SQLite fails query preparation with `no such table: session_sources` for the entire statement. The two separate `QueryRow` calls allow graceful fallback when the first query fails against older schemas.
- **RULING:** `REJECT`

---

### 2. `internal/cli/cmd_prewarm.go:86` — Non-Atomic File Write for Prewarm Dump
- **Rival Commit:** `cc404551a32e426f14b6ccd0c5e70772e61699c4` (`luna/skills-cmd-prewarm-20260826`)
- **Candidate Proposal:** Replace `durable.WriteAtomic(dumpPath, content.Bytes())` with `os.WriteFile(dumpPath, content.Bytes(), 0o600)`.
- **Reproducible Evidence:** `os.WriteFile` truncates and writes in-place without directory sync or temp-file rename semantics. A process killed mid-write leaves a corrupt partial prewarm dump on disk. `durable.WriteAtomic` guarantees crash safety.
- **RULING:** `REJECT`

---

### 3. `internal/cli/cmd_prewarm.go:48-59` — Removal / Bypassing of `LocateConsolidatedSession` Guard
- **Rival Commit:** `cc404551a32e426f14b6ccd0c5e70772e61699c4` (`luna/skills-cmd-prewarm-20260826`) / `e187834167b286a938359fafb5020535887f679e` / `71067bda7bdff81004b19598f5a80fb8da245ab0`
- **Candidate Proposal:** Inlining `locatePrewarmStore` to directly call `agentproto.LocateConsolidatedSession` and deduplicate `refreshTagSession` branching.
- **Reproducible Evidence:** Already integrated into base `origin/integrate/tagwrite-closeout-wave1` (`ac39b70`). The `locateErr != nil` check ensures present sessions in consolidated store are refreshed in private cache without triggering unnecessary re-folds (verified by `TestRunPrewarmExternalBehaviors/present_session_is_not_refolded`).
- **RULING:** `CLEAN`

---

### 4. `internal/cli/setup.go:903-920`, `940` & `internal/cli/setup_test.go:682,721` — Remove Impossible Error from `addRawclawAntigravityHooks`
- **Rival Commit:** `7d5a6a550dc018519cca8f106b86786597d66540` (`norm/prewarm-ponytail` / `norm/prewarm-adversarial-review`)
- **Candidate Proposal:** Change `addRawclawAntigravityHooks(data map[string]any, scriptPath string) error` to void return type, eliminating unreachable error handling in `installRawclawAntigravityHook` and test assertions.
- **Reproducible Evidence:** `addRawclawAntigravityHooks` performs pure in-memory dictionary insertions and slice assignments on a passed-in `map[string]any`. It cannot fail and always returns `nil`. Removing the fake error return eliminates dead error handling code across 9 lines in `setup.go` and `setup_test.go` while preserving exact hook registration behavior.
- **RULING:** `TRANSPLANT_EXACTLY`

---

### 5. `internal/cli/setup.go:antigravityHooksPath` — Deduplicate with `codexHooksPath`
- **Rival Commit:** `ee187c2` (`norm/prewarm-ponytail`)
- **Candidate Proposal:** Alias `antigravityHooksPath` to `codexHooksPath` because both return `<configDir>/hooks.json`.
- **Reproducible Evidence:** While both paths share the file name `hooks.json`, Antigravity and Codex represent distinct editor integration seams with separate hook JSON schema structures. Keeping distinct helper functions maintains clear architectural boundaries and isolation.
- **RULING:** `REJECT`

---

### 6. `internal/cli/tagrefresh_test.go:133-148` — Dead `_ = src` in Unchanged Transcript Test
- **Rival Commit:** `142fce8ac1dc0a584cb6cb71698bc15b73db9343` (`luna/prewarm-ponytail-20260826`)
- **Candidate Proposal:** Delete `_ = src` assignment.
- **Reproducible Evidence:** On current base, `src` is actively used on line 139 (`reg := tagTestRegistration("prewarm-test", src)`). There is no dead assignment in the file.
- **RULING:** `CLEAN`

---

## Confirmed Transplant Target
- **`internal/cli/setup.go` & `internal/cli/setup_test.go`:** Remove dead error return from `addRawclawAntigravityHooks` (Net: -9 lines).
