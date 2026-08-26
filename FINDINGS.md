# Findings & Anti-Bloat Audit: Ingest Test Suite

## Context & Scope
- **File Fence**: `FINDINGS.md`, `internal/cli/cmd_ingest_test.go`
- **Method**: Ponytail Review Ladder (YAGNI, delete > stdlib > native > shrink), Modular Refactor guardrails.
- **Contract Guarantees**: Preserve same-store retry, detached child lifecycle, exact messages-row count idempotence, concurrency contention safety, fail-soft non-existent transcript handling, single-winner lock discipline, and exact error/result contracts.

---

## Ranked Findings & Rulings

### 1. `internal/cli/cmd_ingest_test.go:78-131` (`TestPrimeScripts_StopLaunchDetachedPrewarm`)
- **Tag**: `delete:` / `yagni:`
- **Finding**: Proposal to delete `TestPrimeScripts_StopLaunchDetachedPrewarm` under the assumption that hook lifecycle tests in `antigravityhook_test.go` and `cmd_setup_test.go` supersede it.
- **Audit & Evaluation**: `antigravityhook_test.go` only exercises the Antigravity hook script template. Claude and Codex prime script templates specifically rely on `TestPrimeScripts_StopLaunchDetachedPrewarm` to pin detached child lifecycle execution on Stop events without blocking or hanging sessions.
- **Ruling**: **REJECTED (PRESERVE)** — Retained in full to uphold detached child lifecycle and hook execution contracts.

---

### 2. `internal/cli/cmd_ingest_test.go:215-256` (`TestClaudePrimeScript_ExecutesDetachedIngest`)
- **Tag**: `delete:` / `yagni:`
- **Finding**: Proposal to delete `TestClaudePrimeScript_ExecutesDetachedIngest` under the assumption that `catalog_hook_test.go` covers prime banner execution.
- **Audit & Evaluation**: `TestClaudePrimeScript_ExecutesDetachedIngest` tests standalone POSIX sh execution of Claude prime script banner emission and detached background invocation with a stubbed rawclaw binary.
- **Ruling**: **REJECTED (PRESERVE)** — Retained in full to guarantee non-blocking hook execution contract across POSIX shells.

---

### 3. `internal/cli/cmd_ingest_test.go:261-300, 362-395, 435-469, 515-532` (Test Environment & Catalog Setup Fixture Boilerplate)
- **Tag**: `shrink:` / `stdlib:`
- **Finding**: Repeated environment isolation setup (5 environment variable overrides) and catalog entry session file construction duplicated across 4 test cases (`TestIngestCmd_IndexesFreshSession_EndToEnd`, `TestIngestCmd_Idempotent_RepeatedRunIsNoOp`, `TestIngestCmd_ConcurrentRuns`, `TestIngestCmd_NonExistentTranscript_SkipsGracefully`).
- **Replacement**: Extract local test helpers `setupIngestTestEnv(t *testing.T) string` and `writeTestCatalogSession(t *testing.T, cfg, sessionID, content string) string`.
- **Contract**: Preserves 100% of test isolation, concurrency semantics, and catalog entry contracts.
- **Net Lines**: Net -58 test lines.
- **Ruling**: **ACCEPTED (SHRINK)** — Applied cleanly.

---

### 4. `internal/cli/cmd_ingest_test.go:319-333, 415-429` (Dual SQL Message and Session Row Assertions)
- **Tag**: `shrink:`
- **Finding**: Proposal to eliminate `SELECT COUNT(*) FROM messages WHERE session_id=?` in favor of single `message_count` column check.
- **Audit & Evaluation**: Deleting `messages` row count check relaxes verification of underlying table idempotence (e.g. duplicate row inserts where session summary was not bumped).
- **Ruling**: **REJECTED (PRESERVE)** — Both `sessions.message_count` and `messages` row count queries are retained in full to pin exact table idempotence.

---

## Scoring Summary
- Production lines: 0 lines (test-only change).
- Test lines: net -58 lines (boilerplate helper extraction only; 0 behavioral tests or assertions dropped).
- Coverage: 100% preserved (detached child lifecycle, hook execution, idempotence row counts, concurrency safety, writer lock nesting pins).
