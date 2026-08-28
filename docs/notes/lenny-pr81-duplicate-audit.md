# PR81 Duplicate Machinery & YAGNI Audit Report

**Worker:** `lenny-pr81-duplicate`  
**Target:** PR81 HEAD `8323fd9f69c06669f0ad529686008b775e783052`  
**Base:** `719243b6005153c99fef571176c7e6dd6e3a2876` (`origin/main` merge-base)  
**Date:** 2026-08-28  
**Verdict:** `ACCEPT`

---

## 1. Executive Summary & Verdict

- **Final Verdict:** `ACCEPT`
- **Net Production Code Deletions:** None (`NO CHANGE` to production code; report-only).
- **Novelty Assessment:** PR81 contains no duplicate machinery or dead speculative abstractions. It replaces the flawed, deadlocking catalog-first queries of rejected PR #52 (`7f7c1da`) and naive full-discovery walks of `f77f791` with a lean, source-aware `Lookup` interface contract on `source.Registration` and authoritative durable metadata resolution in `internal/cli`.
- **Invariants:** 100% compliant with RawClaw north star (`CGO_ENABLED=0`, zero external runtime dependencies, no LLM, race-clean).

---

## 2. Exact Patch Identity & History Analysis

### 2.1 PR81 Commit Sequence & Stable Patch-IDs

| Commit | Stable Patch-ID | Subject |
|---|---|---|
| `19ba6df425224f707d96c3c77ed9d9eaf6046598` | `ba9cbe7cea6a55b73f42b716919993d54413696e` | `fix(cli): resolve exact resume IDs without discovery` |
| `ad2c1795b5f3c80d3b67e490e5918b55875d260d` | `dd6aee366fbb15f1b98e9e68e2f584a02d7193d9` | `fix(cli): tighten exact resume lookup boundaries` |
| `77814c7d16091d2f751dbcd94698b77c7c4dcb2c` | `be921cefffc4eed902bf2e9a6cd2cc74ab47b14e` | `fix(source): make exact adapter lookups source-aware` |
| `f0bd9c33bddcc3447d65355d6a9b315ba302be26` | `b32971a8c51c61db37e8741bef058dc8149a8864` | `fix(cli): bound exact resume fallback` |
| `adad41ae8017186d54dc053a920b570164d78ea3` | `bb2f48059882b02ead4b62254fa4394fe3ea87fb` | `test(goose): preserve parent-only exact lookup metadata` |
| `c64509934ab025e5186a500ad595e6105f9d05e4` | `a7dba3ea8f547a5af3ff513f29c26f7c7e154264` | `fix(cli): make resume metadata authoritative` |
| `abc12541acacd4edc12f7d2a296b11a5ed2ee739` | `1cd2dd5868a82c291212c80db12c2527f19b04eb` | `refactor(cli): remove dead resume metadata state` |
| `8323fd9f69c06669f0ad529686008b775e783052` | `69b3c7b0fa7c0c5963f96c66aa9484d0eed337ba` | `fix(cli): explain unavailable exact resume targets` |

### 2.2 Comparison with Preserved PR #52 & Sibling Branches

- **PR #52 (`7f7c1da` / `4923caa`):** Queried the consolidated index first in `runResume` without source awareness, foreign replica filtering, or subagent checks. Caused SQLite lock contention, false-positive matches, and ghost container generation when backing paths were removed or moved.
- **Probe/Exploration Branch `f77f791` (`luna-issue50-root-20260828-a`):** Attempted live probes by triggering full `Registration.Discover()` across all sources on every resume, causing severe timing regressions (~30s timeout) under concurrent activity.
- **PR #81 Adaptation:** Completely supersedes PR #52. It introduces:
  1. `durable.Exact(id)` for immediate zero-walk vaulted session resolution.
  2. `classifyResumeMetadata` + `resumeConsolidatedMetadata` with explicit safety guards (`retained`, `foreign`, `subagent`, `parentID != ""`, `regularFile(path)`).
  3. `source.Registration.Lookup` contract implemented individually by each source adapter (`claude`, `antigravity`, `codex`, `goose`) avoiding disk discovery scans.

---

## 3. Symbol-Level Duplication & Shrink Analysis (Ponytail Review)

| Symbol | Location | Category | Analysis & Ruling |
|---|---|---|---|
| `isFullResumeID` | `internal/cli/cli.go:L991-L1006` | stdlib / shrink | Validates 36-char 8-4-4-4-12 UUID format. Hand-rolled 16-line loop. While `google/uuid` exists in `go.mod` (indirect), using stdlib/inline keeps zero direct external runtime dependencies. **Verdict: Retain (Lean & zero-dep).** |
| `resumeExactMetadata` | `internal/cli/cli.go:L1008-L1060` | logic | Coordinates exact resolution via durable vault, consolidated metadata, and targeted adapter lookups. **Verdict: Retain (Core fast-path logic).** |
| `classifyResumeMetadata` | `internal/cli/cli.go:L1070-L1082` | logic | Centralizes policy for foreign, retained, subagent, and unsupported runtimes. **Verdict: Retain (Enforces safety invariants).** |
| `resumeConsolidatedMetadata` | `internal/cli/cli.go:L1084-L1127` | logic | Reads exact rows from `session_sources` and `sessions` with parameterization. **Verdict: Retain.** |
| `regularFile` | `internal/cli/cli.go:L1129-L1135` | helper | 7-line `os.Stat` wrapper checking regular file mode. Simple and readable. **Verdict: Retain.** |
| `appendResumeCandidate` | `internal/cli/cli.go:L1137-L1144` | helper | Dedupes candidates by source and session ID to prevent double reporting. **Verdict: Retain.** |
| `durable.Exact` | `internal/durable/durable.go:L42-L61` | adapter | O(1) single-session vault check via `PathFor(id)` without directory scan. **Verdict: Retain.** |
| `antigravity.Lookup` | `internal/source/antigravity/antigravity.go:L60-L85` | adapter | Direct path check under brain directory; includes `findParentForChild` for subagent disambiguation. **Verdict: Retain.** |
| `codex.Lookup` | `internal/source/codex/codex.go:L50-L79` | adapter | Walks only rollout files matching target ID via `standardRolloutName`. **Verdict: Retain.** |
| `goose.Lookup` | `internal/source/goose/goose.go:L139-L188` | adapter | Queries SQLite `sessions` or metadata table directly for `id`. **Verdict: Retain.** |
| `claude.Lookup` | `internal/source/claude/claude.go:L47-L69` | adapter | Reads catalog entry or scans direct project JSONL files without parsing history. **Verdict: Retain.** |

---

## 4. Verification & Quality Gates

1. **Full Race Test Suite:**
   ```bash
   CGO_ENABLED=0 go test -race -count=1 ./...
   ```
   **Result:** `PASS` across all 31 packages (including `internal/cli`, `internal/source/*`, `internal/durable`, `internal/index`).
2. **Formatting:**
   ```bash
   gofmt -l internal/
   ```
   **Result:** Clean (no unformatted files).
3. **Diff Hygiene:**
   ```bash
   git diff --check
   ```
   **Result:** Clean (no trailing whitespace or conflict markers).

---

## 5. Conclusion & Action

PR81 is accepted as-is without code modifications. It provides the minimal, correct, and robust implementation of Issue #50's fast exact resume requirement without unnecessary layers or duplicate machinery.
