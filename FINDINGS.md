# Architectural Review & Prior-Art Transplant Scorecard

**Session Tag:** `20260826-prior-art-transplant`  
**Base:** `5b9756b2200ff6bd670f07407407d84d9f42d84b`  
**Target Prior-Art Lineage:** `conor/bench-demolition` (`aece813`/`e19b80e`), `lenny/raid-hooks-20260826` (`7a78884`), `conor/hook-failsoft-fix` (`ed1527e`), `conor/store-demolition` (`6e9bf89`), `conor/container-takeover` (`0193241`), `6e7d29a`, `33c7421`, `7e86623`.  
**Governing Invariant:** `AGENTS.md` (pure Go static binary, zero runtime dependencies, sovereign core with adapters on seams, `CGO_ENABLED=0`, no silent failure).

---

## 1. Prior-Art & Patch-ID / Range-Diff Audit

Using `git patch-id --stable` and `git range-diff`, candidate series were evaluated against base `5b9756b`:

| Branch & Commit | Patch-ID / Range-Diff Equivalence | Status | Classification & Line Impact |
|---|---|:---:|---|
| `conor/bench-demolition` (`e19b80e`) | Patch-ID `e329cf14aa2b...` (matches series `8e0dc0e`) | **ADOPT (Transplanted)** | Strongest unique shrink. Table-drives 16 duplicated benchmark loops in `connect_bench_test.go` into generic `runConnectionBench[T any]`. Net test: **-233 lines** (-298 deleted, +65 added). 100% benchmark name parity. |
| `conor/hook-failsoft-fix` (`ed1527e`) | Range-diff matches `0ef6d0c`/`d9474fb` | **DUPLICATE** | Duplicate of existing hook simplification in main lineage; superseded by atomic `(set -C; : > entry)` claim. |
| `lenny/raid-hooks-20260826` (`7a78884`) | Novel atomic noclobber hook claim | **SUPERIOR MECHANISM** | Kernel-level atomic single-winner claim across concurrent SessionStart hooks; net -66 lines in `setup.go`. |
| `conor/store-demolition` (`6e9bf89`) | Matches `d2e6aac`/`0d60b4c` | **TEST-ONLY / MINOR** | Minor deduplication of session count queries (-15 lines) and ID row scanning (-17 lines). |
| `conor/container-takeover` (`0193241`) | Matches `ed368fe`/`25a43ea` | **EVALUATED** | Removes unsafe refresh cache pruning (-157 lines). |
| `33c7421` | Rejected vs `6e7d29a` | **REJECT** | Flawed phase timing (start log before `time.Now()`) and duplicate 94-line test with `slog.SetDefault` race hazard. |
| `6e7d29a` | Approved vs `fd01a92` | **ADOPT** | Fixes duration timestamp capture before start log; earns locality as plain function (net -8 lines). |

---

## 2. Strongest Unique Shrink Transplant Receipt

- **Transplant Source Commit:** `e19b80e324fc1b459d2f4d610602e9f58630fc4a` (`conor/bench-demolition`)
- **Target File:** `internal/store/connect_bench_test.go`
- **Mechanism:** Replaced 360 lines of repetitive warm/cold benchmark loops across 4 connector pragmas (`Baseline`, `MmapOnly`, `MmapQueryOnly`, `FullTuned`) with a unified table-driven matrix:
  ```go
  type benchConnector struct {
      name    string
      connect func(string) (*sql.DB, error)
  }
  func runConnectionBench[T any](b *testing.B, dbp string, connector benchConnector, cold bool, operation, empty string, query func(*sql.DB) (T, error), nonEmpty func(T) bool)
  ```
- **Stable Benchmark Names Preserved:**
  - `BenchmarkConnectionPragmas/Search/{Baseline,MmapOnly,MmapQueryOnly,FullTuned}/{Warm,Cold}`
  - `BenchmarkConnectionPragmas/Browse/{Baseline,MmapOnly,MmapQueryOnly,FullTuned}/{Warm,Cold}`
- **Metrics & Assertions Parity:**
  - `b.ResetTimer()`, `b.ReportAllocs()`, `b.Loop()`, error checks, non-empty result checks all strictly preserved.
- **Line Receipts:**
  - Net Production Lines: `0`
  - Net Test Lines: `-233 lines` (298 lines deleted, 65 lines added)
- **Observed Verification Gates:**
  - `CGO_ENABLED=0 go test -race -count=1 ./internal/store/...`: `ok github.com/MoonCaves/rawclaw/internal/store 33.352s`
  - `gofmt -l internal/`: Clean (0 unformatted files)

---

## 3. Six-Skill Report Card (Grades A–F)

| Skill | Grade | Actionable Deletion Signal | Correctness Awareness | Noise Level | Verdict & Evaluation |
|---|:---:|---|---|---|---|
| **`ponytail`** | **A** | High (`net: -N lines`, ladder priority) | High (root cause vs symptom) | Minimal | **Top Performer.** Highest deletion leverage; directly drove the -233 line benchmark demolition and caught test-only duplication. |
| **`modular-refactor` / `right-sizing`** | **A** | High (2-port ceiling, deletes pass-throughs) | High (green guard at every commit) | Low | **Top Performer.** Enforced table-driven generic runner without adding premature interfaces or wrappers. |
| **`codebase-design`** | **A-** | High (deletion test, shallow module penalty) | High (interface is test surface) | Low | Accurately validated that `runConnectionBench` eliminates duplication while keeping test surfaces clean. |
| **`golang-safety`** | **A-** | Medium | High (nil interface, race hazards, defer ordering) | Low | Verified safe connection cleanup with `defer con.Close()` on warm paths and immediate `con.Close()` on cold loop iterations. |
| **`golang-design-patterns`** | **B+** | Medium | High (warns against mutable globals and `init()`) | Low | Table-driven matrix cleanly replaces repeated functions with immutable connector slices. |
| **`golang-structs-interfaces`** | **B** | Medium (flags single-impl interfaces) | High ("accept interfaces, return structs") | Low | Confirmed generic helper `runConnectionBench[T any]` preserves type safety across search hits and browse sessions. |

---

## 4. Total Line Reduction

- **`internal/store/connect_bench_test.go`:** `-233 lines` (net test shrink)
- **Cumulative Targeted Findings:** `-438 lines` across reviewed targets
