# Lint Baseline Findings (2026-08-26)

- **Date:** 2026-08-26
- **Branch:** `chore/golangci-config`
- **Tool:** `golangci-lint` v2.12.2
- **Linters Enabled:** `dupl` (threshold: 75), `gocritic`, `modernize` (extending standard defaults and `errcheck` configuration in `.golangci.yml`)

---

## Executive Summary & Noise Evaluation

`golangci-lint` was executed across the entire repository (`./...`) without `--fix` (to preserve source integrity during active multi-worker operations).

### Summary Counts

| Linter | Uncapped Findings | Default Runner Findings (cap=50) | Primary Focus / Concern |
| :--- | :--- | :--- | :--- |
| **`gocritic`** | 1 | 1 | Code quality, control flow clarity (`ifElseChain`) |
| **`dupl`** | 55 | 50 | Code duplication (threshold 75 tokens) |
| **`modernize`** | 117 | 38 | Idiomatic Go updates (iterators, generics, slice/string helpers) |
| **Total** | **173** | **89** | **Signal-rich, well below the 200-finding noise threshold** |

### Noise & Tuning Rationale

- **Threshold Evaluation:** The total uncapped findings across all three newly enabled linters is 173, which is well below the project noise threshold (>200 findings).
- **Signal Quality:**
  - `dupl` at threshold 75 surfaced genuine architectural clones (e.g., an 80-line identical block between `internal/scopes/antigravity.go` and `internal/scopes/goose.go`, identical SQL upsert/merge blocks in `internal/store/verdict.go`, and repeated CLI option validators).
  - `modernize` identified high-value opportunities to adopt modern Go builtins (`max`/`min`, `slices.Contains`, `strings.Cut`, iterators).
  - `gocritic` identified 1 actionable switch-statement simplification.
- **Tuning Decision:** Retain `dupl.threshold = 75` with no blanket suppressions. The signal-to-noise ratio is high, and findings represent clean, actionable refactoring targets.

---

## Findings by Linter (Ordered by Severity)

### 1. `gocritic` (Severity: High / Code Quality & Control Flow)

**Count:** 1 finding

Provides diagnostics that check for bugs, performance, and style issues.

- `internal/source/goose/goose.go:395:2`: `ifElseChain`: rewrite if-else to switch statement

---

### 2. `dupl` (Severity: Medium / Architectural & Code Duplication)

**Count:** 55 findings (27 clone clusters)  
**Configuration:** `threshold: 75`

#### High-Priority Production Code Clones

1. **Scope Resolution Adapter Clone (80 lines):**
   - `internal/scopes/antigravity.go:20-100` $\leftrightarrow$ `internal/scopes/goose.go:20-100`
   - *Rationale:* Identical scope directory traversal and session matching logic between Antigravity and Goose adapters. Refactoring target for shared scope-discovery helper.

2. **Verdict Upsert/Merge Queries (16-18 lines):**
   - `internal/store/verdict.go:41-56` (`UpsertVerdict`) $\leftrightarrow$ `internal/store/verdict.go:72-90` (`MergeVerdict`)
   - *Rationale:* Near-identical SQL query structure and parameter binding.

3. **Vector Key Scanning Loops (16 lines):**
   - `internal/store/fts.go:162-178` $\leftrightarrow$ `internal/store/vectors.go:112-127`
   - *Rationale:* Identical `VecKey` row-scanning loop over SQLite query results.

4. **Topic Query Construction (19-22 lines):**
   - `internal/store/topics.go:208-226` $\leftrightarrow$ `internal/store/topics.go:232-254`
   - *Rationale:* Duplicated topic query formatting and filter application.

5. **Snippet Formatting / Retrieval (10 lines):**
   - `internal/retrieve/retrieve.go:478-488` $\leftrightarrow$ `internal/retrieve/retrieve.go:729-739`
   - *Rationale:* Duplicated match extraction and boundary calculation.

6. **Foreign Archive Git Parsing (14 lines):**
   - `internal/archive/foreign.go:26-40` $\leftrightarrow$ `internal/archive/foreign.go:79-93`
   - *Rationale:* Duplicated remote URL splitting and host validation.

7. **CLI Flag & Option Parsing (17 lines each):**
   - `internal/cli/cli.go:1102-1119` $\leftrightarrow$ `internal/cli/cli.go:1122-1139` $\leftrightarrow$ `internal/cli/cli.go:1142-1159`
   - *Rationale:* Repetitive validation blocks across subcommand parameter parsing.

#### Test Code Clones

- **`internal/store/connect_bench_test.go` (16 findings):**
  - Lines 132-149 $\leftrightarrow$ 151-168 $\leftrightarrow$ 170-187 $\leftrightarrow$ 189-206 (Read concurrency benchmarks)
  - Lines 208-227 $\leftrightarrow$ 229-248 $\leftrightarrow$ 250-269 $\leftrightarrow$ 271-290 (Write concurrency benchmarks)
  - Lines 292-309 $\leftrightarrow$ 311-328 $\leftrightarrow$ 330-347 $\leftrightarrow$ 349-366 (Mixed workload benchmarks)
  - Lines 368-387 $\leftrightarrow$ 389-408 $\leftrightarrow$ 410-429 $\leftrightarrow$ 431-450 (WAL mode benchmarks)
- **`internal/paths/paths_test.go` (4 findings):**
  - Lines 611-647 $\leftrightarrow$ 649-685
  - Lines 755-771 $\leftrightarrow$ 773-789
- **`internal/semantic/semantic_test.go` (4 findings):**
  - Lines 349-359 $\leftrightarrow$ 361-375
  - Lines 588-599 $\leftrightarrow$ 601-612
- **`internal/source/antigravity/antigravity_test.go` (2 findings):**
  - Lines 469-498 $\leftrightarrow$ 500-529
- **`internal/cli/cmd_setup_test.go` (2 findings):**
  - Lines 48-69 $\leftrightarrow$ 765-782
- **`internal/cli/cmd_tag_test.go` (2 findings):**
  - Lines 480-492 $\leftrightarrow$ 496-509
- **`internal/index/incremental_test.go` vs `internal/index/tail_edge_test.go` (2 findings):**
  - `internal/index/incremental_test.go:490-513` $\leftrightarrow$ `internal/index/tail_edge_test.go:444-467`
- **`internal/index/tail_edge_test.go` (2 findings):**
  - Lines 798-813 $\leftrightarrow$ 836-851
- **`internal/parse/parse_test.go` (2 findings):**
  - Lines 465-478 $\leftrightarrow$ 579-624
- **`internal/archive/ssh_test.go` (2 findings):**
  - Lines 26-43 $\leftrightarrow$ 45-62
- **`internal/retrieve/explain_test.go` (2 findings):**
  - Lines 29-37 $\leftrightarrow$ 38-46

---

### 3. `modernize` (Severity: Low / Modern Go Idiom Improvements)

**Count:** 117 findings across 10 analyzer rules

#### 3.1 `rangeint` (34 findings)
*Simplification: `for i := 0; i < N; i++` $\rightarrow$ `for i := range N` (Go 1.22+)*

- `internal/agentproto/agentproto_scope_test.go:179:6`
- `internal/agentproto/agentproto_store_test.go:23:6`
- `internal/agentproto/agentproto_store_test.go:317:6`
- `internal/cli/cmd_upgrade.go:224:6`
- `internal/index/incremental_test.go:270:6`
- `internal/index/index_bench_test.go:36:6`
- `internal/index/tail_edge_test.go:690:6`
- `internal/index/tail_edge_test.go:855:6`
- `internal/index/tail_edge_test.go:1041:6`
- `internal/lifecycle/floor_test.go:14:6`
- `internal/live/session_test.go:257:6`
- `internal/provenance/provenance_test.go:44:6`
- `internal/provenance/provenance_test.go:45:6`
- `internal/semantic/semantic_test.go:548:6`
- `internal/semantic/semantic_test.go:724:6`
- `internal/source/antigravity/antigravity_bench_test.go:19:6`
- `internal/source/antigravity/antigravity_test.go:382:6`
- `internal/source/antigravity/antigravity_test.go:393:6`
- `internal/source/source_test.go:79:6`
- `internal/source/source_test.go:83:8`
- `internal/source/source_test.go:96:8`
- `internal/source/source_test.go:104:8`
- `internal/source/source_test.go:115:8`
- `internal/store/connect_bench_test.go:39:6`
- `internal/store/connect_test.go:96:6`
- `internal/store/connect_test.go:100:8`

#### 3.2 `stringsseq` (23 findings)
*Simplification: `strings.Split` / `strings.Fields` in `for range` $\rightarrow$ `strings.SplitSeq` / `strings.FieldsSeq` (Go 1.24+)*

- `internal/agentproto/agentproto_test.go:102:23`
- `internal/agentproto/agentproto_test.go:429:24`
- `internal/agentproto/warnings_test.go:251:23`
- `internal/cli/cmd_tag_test.go:62:23`
- `internal/cli/cmd_upgrade.go:400:23`
- `internal/durable/durable_test.go:145:23`
- `internal/index/incremental_test.go:147:23`
- `internal/index/incremental_test.go:496:23`
- `internal/index/tail_edge_test.go:450:23`
- `internal/live/client.go:208:23`
- `internal/query/query.go:65:20`
- `internal/query/query.go:198:20`
- `internal/query/query.go:221:20`
- `internal/source/antigravity/antigravity.go:333:25`
- `internal/source/antigravity/antigravity.go:373:23`
- `internal/source/antigravity/antigravity.go:404:23`
- `internal/source/antigravity/antigravity.go:433:23`
- `internal/source/claude/claude.go:85:23`
- `internal/source/codex/codex.go:140:23`
- `internal/store/topics.go:284:20`

#### 3.3 `minmax` (13 findings)
*Simplification: conditional assignment $\rightarrow$ `min(...)` / `max(...)` builtins (Go 1.21+)*

- `internal/agentproto/agentproto.go:484:5`
- `internal/agentproto/agentproto.go:640:6`
- `internal/agentproto/agentproto.go:1679:6`
- `internal/live/serve.go:102:5`
- `internal/live/session.go:170:6`
- `internal/live/session.go:195:5`
- `internal/query/query.go:125:5`
- `internal/query/query.go:129:5`
- `internal/retrieve/retrieve.go:379:5`
- `internal/semantic/semantic.go:231:5`
- `internal/view/view.go:299:5`

#### 3.4 `slicescontains` (11 findings)
*Simplification: manual element search loops $\rightarrow$ `slices.Contains` / `slices.ContainsFunc` (Go 1.21+)*

- `internal/agentproto/agentproto_store_test.go:427:2`
- `internal/cli/cli.go:1931:2`
- `internal/cli/cmd_completion_test.go:194:5`
- `internal/cli/setup.go:533:3`
- `internal/durable/durable.go:200:2`
- `internal/index/consolidated_test.go:1217:2`
- `internal/index/index.go:855:2`
- `internal/retrieve/retrieve.go:745:2`
- `internal/scopes/scopes_test.go:142:3`
- `internal/source/claude/claude.go:120:2`
- `internal/view/view.go:500:2`

#### 3.5 `stringscut` (11 findings)
*Simplification: `strings.Index` / `strings.IndexByte` + manual slice slicing $\rightarrow$ `strings.Cut` (Go 1.18+)*

- `internal/cli/cmd_ingest.go:312:12`
- `internal/index/containers.go:88:12`
- `internal/index/tail.go:451:11`
- `internal/index/tail.go:454:13`
- `internal/source/antigravity/antigravity.go:365:11`
- `internal/source/antigravity/antigravity.go:375:14`
- `internal/source/antigravity/antigravity.go:382:14`
- `internal/source/antigravity/antigravity.go:384:13`
- `internal/source/antigravity/antigravity.go:395:11`
- `internal/source/antigravity/antigravity.go:539:11`
- `internal/source/antigravity/antigravity.go:542:13`

#### 3.6 `slicesbackward` (8 findings)
*Simplification: reverse index iteration loop $\rightarrow$ `for i, v := range slices.Backward(...)` (Go 1.23+)*

- `internal/cli/cmd_tag.go:192:9`
- `internal/cli/cmd_tag.go:323:9`
- `internal/cli/cmd_tag.go:746:9`
- `internal/index/retained.go:170:6`
- `internal/scopes/scopes.go:176:6`
- `internal/view/view.go:210:6`
- `internal/view/view.go:243:6`
- `internal/view/view.go:293:6`

#### 3.7 `stringsbuilder` (7 findings)
*Simplification: iterative `+=` string concatenation in loop $\rightarrow$ `strings.Builder`*

- `internal/index/index_bench_test.go:19:3`
- `internal/index/index_test.go:415:3`
- `internal/paths/paths_test.go:18:3`
- `internal/source/antigravity/antigravity_test.go:22:3`
- `internal/source/claude/claude_test.go:18:3`
- `internal/source/codex/codex_test.go:28:3`
- `internal/store/sessions.go:67:3`

#### 3.8 `fmtappendf` (5 findings)
*Simplification: `[]byte(fmt.Sprintf(...))` $\rightarrow$ `fmt.Appendf(...)` (Go 1.19+)*

- `internal/index/tail.go:343:16`
- `internal/index/tail.go:494:16`
- `internal/provenance/provenance.go:100:29`
- `internal/source/antigravity/antigravity.go:584:16`
- `internal/source/codex/codex.go:321:16`

#### 3.9 `stringscutprefix` (4 findings)
*Simplification: `strings.HasPrefix(...)` + `strings.TrimPrefix(...)` $\rightarrow$ `strings.CutPrefix(...)` (Go 1.20+)*

- `internal/agentproto/warnings_test.go:252:6`
- `internal/archive/gitrun_test.go:40:6`
- `internal/archive/gitrun_test.go:61:6`

#### 3.10 `any` (1 finding)
*Simplification: `interface{}` $\rightarrow$ `any` (Go 1.18+)*

- `internal/store/triage_test.go:139:10`

---

## Conclusion & Next Steps

The addition of `dupl` (threshold 75), `gocritic`, and `modernize` to `.golangci.yml` introduces clean, high-signal static analysis. Since parallel workers are actively modifying source files, source modifications (`--fix`) are deferred. The findings above serve as a baseline for prioritized remediation.
