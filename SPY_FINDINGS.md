# Ozzy Flash Rescue Dossier: Rival Review Contradictions

**Audit date:** 2026-08-26
**Scope:** Lenny, Norm, and Conor worktrees/mailboxes; report-only.
**Immutable base:** `origin/integrate/tagwrite-closeout-wave1` @ `479d14c`.
**Production edits:** none. The three Norm-known items (7a78884 marker ordering, the four identical skill patch hashes, and 6ddd17a logger scope) are intentionally omitted.

## Confirmed ammunition

### 1. Lenny's “concurrent” dedup test is sequential, and the fix is still racy

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-skill-style`, `lenny/skill-style-20260826` @ `37e4f70b5490eda0e9c3e409cc4a1e163082b856`.
- **Claim receipt:** that branch's `FINDINGS.md:17-24,69-81` calls the change “Concurrent Dedup Proof” and says it is verified by `TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest`.
- **Evidence:** `git show f8fd1fe:internal/cli/setup.go:60-83` does `[ -f "$entry" ]` followed by a non-exclusive temp-file write and `mv -f`; two hooks can both observe absence and both launch ingest. `git show f8fd1fe:internal/cli/cmd_ingest_test.go:133-183` runs `for run := range 2` with `exec.Command` sequentially, never concurrently.
- **Classification:** **CONFIRMED correctness gap.**
- **Minimal correction:** use an actual exclusive claim (`set -C`/`O_EXCL`) and launch two hook processes concurrently in the regression test.
- **Roast:** They named a two-at-a-time queue “concurrent” because the loop counter had two values.

### 2. The Codex hook permanently suppresses its own banner when Python is absent

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-skill-style`, `lenny/skill-style-20260826` @ `37e4f70b5490eda0e9c3e409cc4a1e163082b856`.
- **Claim receipt:** `FINDINGS.md:23-24,69-86` claims fail-soft resilience and a durable catalog/JSON envelope.
- **Evidence:** `git show f8fd1fe:internal/cli/setup.go:146-176` writes or falls back to `$entry`, launches ingest, then executes `command -v python3 || exit 0`. On a machine without Python, the marker survives while no Codex banner is emitted; every later SessionStart hits `[ -f "$entry" ]` and exits before retrying the banner. The source itself calls this a “deliberate accepted trade” at lines 172-176.
- **Classification:** **CONFIRMED behavior defect.**
- **Minimal correction:** do not mark the session claimed until the envelope can be emitted, or store separate ingest and banner-delivery state with retry semantics.
- **Roast:** Fail-soft became fail-once: the missing interpreter gets one silent banner and then a lifetime subscription to nothing.

### 3. Lenny's claimed “zero-indirection” hook wiring adds two higher-order callback seams

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-skill-style`, `lenny/skill-style-20260826` @ `37e4f70b5490eda0e9c3e409cc4a1e163082b856`.
- **Claim receipt:** `FINDINGS.md:59-60` says procedural dispatch has “zero ... interface indirection”; `FINDINGS.md:35-47` presents the helper unification as right-sized.
- **Evidence:** `git show f8fd1fe:internal/cli/setup.go:680-705,770-832` introduces `installRawclawHookWith(... registerHooks func(...))` and `ejectRawclawHookWith(... hasHooks func(...), removeHooks func(...))`; target-specific closures are passed at lines 818-832 and 914-923. This is callback indirection, even though it is unexported and not an interface.
- **Classification:** **CONFIRMED over-engineering.**
- **Minimal correction:** keep the two concrete target helpers and share only genuinely identical file/config primitives; remove the callback-parameter layer.
- **Roast:** “Zero indirection” is doing a lot of work for two functions that literally accept function arguments.

### 4. Lenny's phase-review deletion recommendation leaves the same global logger mutation in place

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-skill-architecture`, `lenny/skill-architecture-20260826` @ `65f3b8b236810827275c4ca00a00327b8119796d`.
- **Claim receipt:** `FINDINGS.md:12,21-24` says delete the 94-line `33c7421` test because it mutates global `slog.SetDefault`, then rely on existing `2ee9950`.
- **Evidence:** `git show 33c7421:internal/index/consolidated_test.go:26-28` calls `slog.SetDefault`; `git show 2ee9950:internal/index/consolidated_test.go:26-28` does the same. The proposed replacement therefore preserves the exact process-global mutation the finding calls a race hazard.
- **Classification:** **CONFIRMED review contradiction.**
- **Minimal correction:** isolate logger state for both tests or accurately classify the remaining test as carrying the same global-state risk; deleting only the duplicate does not remove it.
- **Roast:** Lenny prescribed removing the duplicate smoke alarm while leaving the original alarm wired into the same short circuit.

### 5. Conor's catalog-closure branch carries unrelated store refactors despite a single-seam ruling

- **Supervisor:** Conor; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-conor-store-demolition`, `conor/raid-norm-catalog` @ `76faabb92edc9ef731d27eea73c1ff5fe0829749` (worktree currently dirty in `internal/agentproto/agentproto.go`).
- **Claim receipt:** that worktree's `FINDINGS.md:3-16` says “Take over only” the `catalogCands` predicate and expects `-5` lines with no test changes.
- **Evidence:** `git show --stat 76faabb` lists changes not limited to `internal/agentproto/agentproto.go`: `internal/cli/setup.go`, `internal/cli/cmd_ingest_test.go`, `internal/cli/cmd_tag_onestore_test.go`, `internal/index/containers.go`, `internal/index/consolidated_fence_test.go`, `internal/store/stats.go`, `internal/store/topics.go`, and `internal/index/consolidated.go`. `git -C /Users/jay-m4/code/rawclaw-conor-store-demolition status --porcelain` currently reports `M internal/agentproto/agentproto.go`.
- **Classification:** **CONFIRMED scope/closeout failure.**
- **Minimal correction:** isolate and commit only the catalog predicate, or explicitly account for every inherited change and finish with a clean worktree.
- **Roast:** The “single seam” arrived with an entire moving truck of unrelated furniture—and left one box open on the sidewalk.

### 6. Lenny's architecture scorecard calls a broad test sweep a clean proof while declaring broad suites skipped

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-skill-architecture`, `lenny/skill-architecture-20260826` @ `65f3b8b236810827275c4ca00a00327b8119796d`.
- **Claim receipt:** `FINDINGS.md:37-49` grades the review “Top Performer” and records “Broad test suites: Skipped per directive; Repository state: Clean working tree.”
- **Evidence:** the branch diff against `479d14c` adds test code in `internal/cli/cmd_ingest_test.go`, `internal/cli/cmd_tag_onestore_test.go`, and `internal/index/consolidated_fence_test.go` (`git diff --stat 479d14c..65f3b8b`). No command receipt in the branch record demonstrates the focused tests or the claimed clean state; the reproducible evidence is the skipped broad gate and committed diff.
- **Classification:** **CONFIRMED evidence-quality gap; not a claim of test failure.**
- **Minimal correction:** report exact observed focused commands/results, and say “not run” for every omitted gate instead of presenting a scorecard as proof.
- **Roast:** A skipped exam can be honestly marked “not taken”; it cannot also be framed as a distinction.

## Credible rival wins

1. **Lenny, `lenny/skill-interfaces-20260826` @ `62095345b14124b63cd907a4abdce261a67241cb`:** `54bf2b0` deletes `pruneStaleRefreshDBs` and its 119-line fixture, removing an unfenced refresh-cache unlink path. The branch's `FINDINGS.md:1-12` and immutable commit stat support the reported deletion.
2. **Conor, `conor/six-skill-audit` @ `34bef0ce688ea6c57b9d4a8fb8343ed10b635f6a`:** the audit identifies real dead `RoutineVerdictSet` code and repeated benchmark/query setup. This is supported by its committed `FINDINGS.md` and recorded `dupl`/modernize outputs; no production claim is inferred beyond the cited cleanup targets.
3. **Norm, `norm/fault-test-slim` @ `cfccbc6184bf0af1bd7632923933134bbf4c0bdb`:** the same-store retry test is a materially better deterministic fixture than the discarded multi-database mock. The immutable branch and Norm's recorded receipt support this narrow test-design win.

## Focused verification

- `git show` / `git diff --stat` / `git diff --check`: **PASS** for the cited immutable evidence and this report.
- `git -C /Users/jay-m4/code/rawclaw-conor-store-demolition status --porcelain`: **OBSERVED dirty** (`M internal/agentproto/agentproto.go`).
- Go gates: **NOT RUN**; this lane changed no Go files and does not claim a Go test result.
- `gofmt`: **NOT REQUIRED**; no Go file changed.
- Net report delta: **-114 lines** (198-line stranded report replaced by 84 lines).

## Top five ammunition lines

1. The “concurrent” hook proof runs two processes one after another, while its fix still does check-then-`mv -f`.
2. Codex writes the dedup marker before checking Python, so a missing interpreter permanently suppresses future banners.
3. “Zero indirection” is implemented with two higher-order callback seams.
4. Deleting Lenny's duplicate logger test leaves the same `slog.SetDefault` mutation in his recommended replacement.
5. Conor's “single catalog seam” branch carries eight unrelated files and is currently dirty.
