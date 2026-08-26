# Findings & Anti-Bloat Audit: Ingest Test Suite

## Context & Scope
- **File Fence**: `FINDINGS.md`, `internal/cli/cmd_ingest_test.go`
- **Method**: Ponytail Review Ladder (YAGNI, delete > stdlib > native > shrink), Modular Refactor guardrails.
- **Contract Guarantees**: Preserve same-store retry, concurrency contention safety, fail-soft non-existent transcript handling, single-winner lock discipline, and exact error/result contracts.

---

## Ranked Findings

### 1. `internal/cli/cmd_ingest_test.go:78-131`
- **Tag**: `delete:` / `yagni:`
- **Finding**: `TestPrimeScripts_StopLaunchDetachedPrewarm` orchestrates `sh` subprocesses to verify that `Stop` hook events dispatch detached `prewarm`. This test exercises the hook dispatch of `prewarm` rather than the `ingest` command, duplicating hook lifecycle tests in `antigravityhook_test.go` and `cmd_setup_test.go`.
- **Contract**: Stop hook prewarm dispatch is fully covered in setup/antigravity hook tests.
- **Estimated Net Lines**: -54 lines.
- **Ruling**: **ACCEPTED (DELETE)**.

### 2. `internal/cli/cmd_ingest_test.go:215-256`
- **Tag**: `delete:` / `yagni:`
- **Finding**: `TestClaudePrimeScript_ExecutesDetachedIngest` executes `prime.sh` via `sh` solely to verify the banner output `[rawclaw] Raw transcript history`. This is a redundant duplicate of `TestClaudePrimeScript_CreatesSessionCatalogEntry` in `catalog_hook_test.go:18-95`.
- **Contract**: Banner generation and prime script execution are verified in `catalog_hook_test.go` and `codexhook_test.go`.
- **Estimated Net Lines**: -42 lines.
- **Ruling**: **ACCEPTED (DELETE)**.

### 3. `internal/cli/cmd_ingest_test.go:261-300, 362-395, 435-469, 515-532`
- **Tag**: `shrink:` / `stdlib:`
- **Finding**: Repeated environment isolation boilerplate (6-7 lines setting `HOME`, `CLAUDE_CONFIG_DIR`, `XDG_DATA_HOME`, `XDG_CACHE_HOME`, `RAWCLAW_CATALOG_DIR`) and session transcript/catalog entry construction repeated verbatim across 4 tests (`TestIngestCmd_IndexesFreshSession_EndToEnd`, `TestIngestCmd_Idempotent_RepeatedRunIsNoOp`, `TestIngestCmd_ConcurrentRuns`, `TestIngestCmd_NonExistentTranscript_SkipsGracefully`).
- **Replacement**: Extract local helpers `setupIngestTestEnv(t *testing.T) string` and `writeTestCatalogSession(t *testing.T, cfg, sessionID, content string) string`.
- **Contract**: Preserves complete environment isolation and exact filesystem/catalog schema.
- **Estimated Net Lines**: -55 lines.
- **Ruling**: **ACCEPTED (SHRINK)**.

### 4. `internal/cli/cmd_ingest_test.go:319-333, 415-429`
- **Tag**: `shrink:`
- **Finding**: Redundant dual SQL queries querying both `SELECT message_count FROM sessions` and `SELECT COUNT(*) FROM messages` asserting the exact same row count.
- **Replacement**: Consolidate into concise verification query helper or inline single query check.
- **Contract**: Guarantees database integrity and idempotency.
- **Estimated Net Lines**: -12 lines.
- **Ruling**: **ACCEPTED (SHRINK)**.

---

## Scoring Summary
- Net lines: ~ -163 lines possible.
- Production impact: 0 lines (test-only fence).
- Test impact: net ~ -163 lines.
