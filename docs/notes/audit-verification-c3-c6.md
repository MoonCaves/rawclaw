# Verification Report: Code-Reduction Candidates 3–6

**Date:** 2026-08-26  
**Session Tag:** `20260826-audit-verification`  
**Target Document:** `docs/notes/ponytail-audit-20260826.md` (Branch: `audit/ponytail-day1`)  
**Audit Target Codebase:** `MoonCaves/rawclaw` @ `43cb92f` / `audit/verify-c3-c6`  
**Scope:** Verification of Duplication & Code Reduction Claims for Candidates 3, 4, 5, and 6 against actual code.

---

## 1. Executive Summary & Verdict Scorecard

The code-reduction audit in `docs/notes/ponytail-audit-20260826.md` proposed eliminating **~455 LOC** across Candidates 3, 4, 5, and 6. Each claim was evaluated against the actual codebase using regional code extraction, unified diffs, structural dependency analysis, and backward-compatibility / invariant reviews.

### Scorecard

| Candidate | Target File(s) | Claimed Removable LOC | Verified Removable LOC | Verdict | Primary Rationale |
|---|---|---|---|---|---|
| **Candidate 3** | `internal/index/consolidated.go` | **~210 LOC** | **~40–70 LOC** | **OVERSTATED** | All candidate functions in the target scope total only 157 LOC. Legacy watermark migration (`rewriteLegacyWatermark`) is active compatibility code; PRAGMA deduplication yields ~29 LOC net savings. |
| **Candidate 4** | `internal/cli/cmd_ingest.go`, `internal/cli/tagrefresh.go` | **~120 LOC** | **~25–35 LOC** | **OVERSTATED** | `cmd_ingest.go` did **not** duplicate the lookup pipeline—it already calls `stemTagSources`, `discoverTagSources`, and `locatedTagSource` directly. Only `backingPath` is a verbatim clone (6 LOC). Shared resolution glue saves at most ~25 LOC. |
| **Candidate 5** | `internal/view/view.go`, `internal/agentproto/agentproto.go`, `internal/store/verdict.go` | **~75 LOC** | **~60–75 LOC** | **CONFIRMED** | `sortCandidates` is 100% byte-for-byte identical (38 LOC) across `view.go` and `agentproto.go`. `RoutineVerdictSet` (18 LOC) is redundant and violates the *"real tag beats routine"* invariant. |
| **Candidate 6** | `internal/paths/paths.go` | **~50 LOC** | **~10–14 LOC** | **OVERSTATED / PARTIALLY WRONG** | Catalog resolution (`resolveSessionCatalog`) queries flat catalog files, whereas stem resolution (`resolveSessionStem`) globs project trees; they are not duplicates. Only the 7-line CWD fallback in `sessionHitFromCatalog` is reducible (~10–14 LOC). |
| **TOTAL** | | **~455 LOC** | **~135–194 LOC** | **OVERSTATED overall** | Only Candidate 5 is fully confirmed. Candidates 3, 4, and 6 inflated removable line counts by 3x–5x. |

---

## 2. Candidate 3: Consolidated Store Migration Scaffolding & PRAGMA Introspection Bloat

- **Target File:** `internal/index/consolidated.go`
- **Claimed Removable Lines:** **~180–250 LOC** (Scorecard: **~210 LOC**)
- **Verified Removable Lines:** **~40–70 LOC**
- **Verdict:** **OVERSTATED**

### 2.1 Audit Claims
`ponytail-audit-20260826.md` asserted three sub-claims:
1. **Legacy Basename Watermark Scaffolding (`consolidated.go:108-148`):** `rewriteLegacyWatermark` and the single-candidate basename fallback loop exist purely to migrate pre-`ebc086a` builds and can be deleted.
2. **Duplicate SQLite PRAGMA Introspection (`consolidated.go:841-913`):** `srcHasTable` (lines 847–858), `srcHasColumn` (lines 864–888), and `hasScopeColumns` (lines 890–913) execute manual `PRAGMA table_info` scans that duplicate helpers in `internal/source/goose/` and `internal/store/`. A single generic helper was claimed to replace ~75 lines.
3. **Redundant Backfill Queries in `migrateSessionSources` & `healUpgradedConsolidatedStore` (`consolidated.go:976-1100`):** Redundant table checks and sequential `SELECT COUNT(*)` statements across `sessions` and `session_sources` can be unified.

### 2.2 Verification Against Actual Code

#### A. Total Line Count Contradiction
First, examining the entire functions identified in Candidate 3:
- `rewriteLegacyWatermark` (`consolidated.go:130-148`): **19 lines**
- `unconsolidatedDBs` single-candidate fallback loop (`consolidated.go:108-125`): **18 lines**
- `srcHasTable` (`consolidated.go:847-858`): **12 lines**
- `srcHasColumn` (`consolidated.go:864-888`): **25 lines**
- `hasScopeColumns` (`consolidated.go:890-913`): **24 lines**
- `migrateSessionSources` (`consolidated.go:975-1030`): **56 lines**
- `healUpgradedConsolidatedStore` (`consolidated.go:1080-1100`): **21 lines**

The entire combined line count of all candidate functions is **175 lines** (of which only a fraction is removable). Claiming **~210 LOC removable** is mathematically impossible without deleting required business logic.

#### B. PRAGMA Introspection Diff Evidence
`srcHasColumn` and `hasScopeColumns` both query the attached database schema (`PRAGMA src.table_info`):

```diff
--- srcHasColumn (consolidated.go:864-888)
+++ hasScopeColumns (consolidated.go:890-913)
@@ -1,9 +1,10 @@
-func srcHasColumn(con *sql.DB, table, col string) (bool, error) {
-	rows, err := con.Query("PRAGMA src.table_info(" + table + ")")
+func hasScopeColumns(con *sql.DB) (bool, error) {
+	rows, err := con.Query("PRAGMA src.table_info(sessions)")
 	if err != nil {
-		return false, fmt.Errorf("read source %s columns: %w", table, err)
+		return false, fmt.Errorf("read source columns: %w", err)
 	}
 	defer rows.Close()
+	have := map[string]bool{}
 	for rows.Next() {
 		var (
 			cid         int
@@ -12,14 +13,12 @@
 			dflt        sql.NullString
 		)
 		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
-			return false, fmt.Errorf("scan source %s column: %w", table, err)
+			return false, fmt.Errorf("scan source column: %w", err)
 		}
-		if name == col {
-			return true, nil
-		}
+		have[name] = true
 	}
 	if err := rows.Err(); err != nil {
-		return false, fmt.Errorf("iterate source %s columns: %w", table, err)
+		return false, fmt.Errorf("iterate source columns: %w", err)
 	}
-	return false, nil
+	return have["project"] && have["cwd"], nil
 }
```

**Analysis:**
- SQLite requires `PRAGMA <schema>.table_info(<table>)` syntax for ATTACHed databases; standard helpers like `store.columnSet` (`internal/store/store.go:241-265`) or `tableColumns` (`internal/index/index.go:540-562`) operate only on the `main` schema.
- Creating a shared `srcTableColumns(con *sql.DB, table string) (map[string]struct{}, error)` (~20 LOC) allows replacing `srcHasColumn` (25 LOC) and `hasScopeColumns` (24 LOC).
- **Net PRAGMA reduction:** $25 + 24 - 20 = \mathbf{29\text{ LOC}}$.

#### C. Legacy Watermark Migration Invariant
`rewriteLegacyWatermark` (lines 130–148, 19 LOC) was added in commit `791c2b2` (*"fix(index): rewrite legacy basename watermark under full-path identity"*). It allows existing pre-v0.4.0 repositories to upgrade seamlessly on their next consolidation pass without re-indexing from scratch. Removing this code breaks the zero-downtime store upgrade contract. If migrated to a one-time schema version check, net code reduction is at most **~15–20 LOC**.

#### D. Migration & Healing Query Unification
`migrateSessionSources` (56 LOC) runs during store connection setup to ensure schema DDL and initial backfill. `healUpgradedConsolidatedStore` (21 LOC) runs during consolidation to detect and invalidate stale sync marks when `sessions` is populated but `session_sources` is missing. Consolidating the `SELECT COUNT(*)` checks saves at most **~10–15 LOC**.

### 2.3 Candidate 3 Summary
- Real removable lines: **~40–70 LOC**.
- Audit claim of ~210 LOC was overstated by **3x–5x**.

---

## 3. Candidate 4: Ingest vs Tag-Prep Source Resolution Duplication

- **Target Files:** `internal/cli/cmd_ingest.go:153-240`, `internal/cli/tagrefresh.go:44-150`
- **Claimed Removable Lines:** **~100–140 LOC** (Scorecard: **~120 LOC**)
- **Verified Removable Lines:** **~25–35 LOC**
- **Verdict:** **OVERSTATED / MISCHARACTERIZED**

### 3.1 Audit Claims
`ponytail-audit-20260826.md` claimed:
1. `cmd_ingest.go` and `tagrefresh.go` reimplement nearly identical 4-stage resolution pipelines for a session argument.
2. `cmd_ingest.go:311-316` defines private `backingPath(p string)` which is byte-for-byte identical to `internal/index/containers.go:87-92` `backingFilePath(p string)`.
3. Unifying session resolution would eliminate ~120 LOC.

### 3.2 Verification Against Actual Code

#### A. Re-use vs Duplication Analysis
Inspecting `internal/cli/cmd_ingest.go:153-184`:
```go
func resolveIngestMatches(sessionArg string, regs []source.Registration) ([]tagSourceMatch, error) {
	if sessionArg == "" {
		return discoverAllIngestSources(regs)
	}
	prefix := agentproto.NormalizeSessionArg(sessionArg)

	// 1. Check the durable session catalog first.
	if catalogMatch, ok := catalogIngestSource(prefix, regs); ok {
		return []tagSourceMatch{catalogMatch}, nil
	}
	// 2. Check stem lookup (e.g. paths.ResolveSession for Claude).
	if stems := stemTagSources(prefix, regs); len(stems) > 0 {
		return stems, nil
	}
	// 3. Check registered source adapters discovery.
	if discovered, err := discoverTagSources(prefix, regs); len(discovered) > 0 {
		return discovered, err
	} else if err != nil {
		return nil, err
	}
	// 4. Check already located backing in the consolidated store.
	if located, ok := locatedTagSource(index.ConsolidatedPath(), prefix, regs); ok {
		return []tagSourceMatch{located}, nil
	}
	return nil, nil
}
```

**Key Finding:** `cmd_ingest.go` **does not duplicate** `stemTagSources`, `discoverTagSources`, or `locatedTagSource`! Because both files belong to package `cli`, `cmd_ingest.go` **directly calls** the helper functions defined in `tagrefresh.go:97-177`.

The audit's claim that `cmd_ingest.go` reimplemented the 4 stages is incorrect:
- Stage 2 calls `stemTagSources` (`tagrefresh.go:97`).
- Stage 3 calls `discoverTagSources` (`tagrefresh.go:197`).
- Stage 4 calls `locatedTagSource` (`tagrefresh.go:144`).

#### B. Pipeline Semantic Divergence
`refreshTagSession` in `tagrefresh.go:50-95` and `resolveIngestMatches` in `cmd_ingest.go:153-184` serve fundamentally different command lifecycles:
- `refreshTagSession` (`rawclaw tag --refresh`): Probes the consolidated store *first* via `agentproto.LocateSession` to avoid expensive multi-adapter scans if the transcript is already indexed and unchanged.
- `resolveIngestMatches` (`rawclaw ingest`): Checks the filesystem catalog *first* to locate newly created live transcripts before consulting the consolidated store.

#### C. Verbatim Clone: `backingPath`
`backingPath` in `internal/cli/cmd_ingest.go:311-316` vs `backingFilePath` in `internal/index/containers.go:87-92`:

```diff
--- internal/index/containers.go:87-92
+++ internal/cli/cmd_ingest.go:311-316
@@ -1,4 +1,4 @@
-func backingFilePath(p string) string {
+func backingPath(p string) string {
 	if idx := strings.IndexByte(p, '#'); idx >= 0 {
 		return p[:idx]
 	}
```

This 6-line helper is indeed a clone and can be exported as `index.BackingFilePath(p string)`.

#### D. Realistic Reduction
- Replace `cmd_ingest.go:backingPath` with `index.BackingFilePath`: **6 LOC**.
- Unify `catalogIngestSource` (26 LOC) and shared match mapping: **~20–25 LOC**.
- Total net reduction: **~25–35 LOC**.

### 3.3 Candidate 4 Summary
- Real removable lines: **~25–35 LOC**.
- Audit claim of ~120 LOC was overstated by **~4x** due to mistaking function calls for duplicate code.

---

## 4. Candidate 5: Shared Candidate Sort & Routine Verdict Duplication

- **Target Files:** `internal/view/view.go:318-355`, `internal/agentproto/agentproto.go:1420-1457`, `internal/store/verdict.go:167-184`
- **Claimed Removable Lines:** **~75 LOC**
- **Verified Removable Lines:** **~60–75 LOC**
- **Verdict:** **CONFIRMED**

### 4.1 Audit Claims
`ponytail-audit-20260826.md` asserted:
1. `internal/view/view.go:318-355` and `internal/agentproto/agentproto.go:1423-1460` contain an identical 38-line implementation of `sortCandidates`.
2. `store.RoutineVerdictSet` (`internal/store/verdict.go:167-184`, 18 LOC) is a harmful / redundant helper that ignores `topic_segment`, violating the invariant *"A real tag beats routine"*.

### 4.2 Verification Against Actual Code

#### A. Verbatim Duplicate: `sortCandidates`
Diffing `internal/view/view.go:318-355` against `internal/agentproto/agentproto.go:1423-1460`:

```diff
--- internal/view/view.go:318-355
+++ internal/agentproto/agentproto.go:1423-1460
@@ -0,0 +0,0 @@
```
*(Diff is empty: the two implementations are 100% byte-for-byte identical across all 38 lines).*

```go
func sortCandidates(cands []retrieve.Anchor, mode string) {
	switch mode {
	case "newest":
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].ISO != cands[j].ISO {
				return cands[i].ISO > cands[j].ISO
			}
			if cands[i].Routine != cands[j].Routine {
				return !cands[i].Routine && cands[j].Routine
			}
			return false
		})
	case "oldest":
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].ISO != cands[j].ISO {
				return cands[i].ISO < cands[j].ISO
			}
			if cands[i].Routine != cands[j].Routine {
				return !cands[i].Routine && cands[j].Routine
			}
			return false
		})
	default:
		sort.SliceStable(cands, func(i, j int) bool {
			a, b := cands[i], cands[j]
			if a.Fused != b.Fused {
				return a.Fused > b.Fused
			}
			if a.Cov != b.Cov {
				return a.Cov > b.Cov
			}
			if a.Routine != b.Routine {
				return !a.Routine && b.Routine
			}
			return a.Rank < b.Rank
		})
	}
}
```

Exporting `view.SortCandidates(cands []retrieve.Anchor, mode string)` (or placing it in `internal/retrieve`) eliminates **38 LOC** from `agentproto.go`.

#### B. Harmful Helper: `RoutineVerdictSet`
In `internal/store/verdict.go:167-184` (18 LOC):
```go
// RoutineVerdictSet returns the set of all session IDs in con that carry a
// "routine" verdict in session_verdict.
func RoutineVerdictSet(con *sql.DB) (map[string]bool, error) {
	rows, err := con.Query("SELECT session_id FROM session_verdict WHERE verdict = ?", VerdictRoutine)
	if err != nil {
		return map[string]bool{}, nil
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			return out, err
		}
		out[sid] = true
	}
	return out, rows.Err()
}
```

As documented in `docs/notes/adversarial-review-20260825.md` (§ Finding 3), `RoutineVerdictSet` only checks `session_verdict` without querying `topic_segment`. When invoked in `agentproto.go:2461` (`topicsFromStore`) and `agentproto.go:2552` (`topicsByFanOut`), it inappropriately demotes sessions that carry valid topic tags.

Replacing these two call sites with `store.RoutineSet(con)` (which respects the invariant) and deleting `RoutineVerdictSet` removes **18 LOC** from `verdict.go` and cleans up call sites.

#### C. Candidate 5 Reduction Breakdown
- `sortCandidates` dedup: **38 LOC**
- `RoutineVerdictSet` removal: **18 LOC**
- Call site & comment cleanups: **~8 LOC**
- Total removable lines: **~64–75 LOC**.

### 4.3 Candidate 5 Summary
- Real removable lines: **~64–75 LOC**.
- Verdict: **CONFIRMED**.

---

## 5. Candidate 6: Session Catalog vs Stem Resolution Duplication

- **Target File:** `internal/paths/paths.go:298-396`
- **Claimed Removable Lines:** **~50 LOC**
- **Verified Removable Lines:** **~10–14 LOC**
- **Verdict:** **OVERSTATED / PARTIALLY WRONG**

### 5.1 Audit Claims
`ponytail-audit-20260826.md` claimed:
- `resolveSessionCatalog`, `validateCatalogHit`, and `sessionHitFromCatalog` (`paths.go:309-396`) duplicate directory extraction and project label computation already implemented in `ProjectLabel`, `ProjectDirOf`, and `resolveSessionStem`.
- `sessionHitFromCatalog` contains an 18-line fallback ladder for project resolution that mirrors `ProjectLabel(pdir)`.

### 5.2 Verification Against Actual Code

#### A. Catalog Resolution vs Stem Resolution Topology
- `resolveSessionCatalog` (`paths.go:310-350`, 41 LOC): Reads JSON files in the flat directory `~/.cache/session-search/catalog/<session_id>`. This is an $O(1)$ direct lookup or a single directory scan across all registered source adapters (Claude, Goose, Antigravity, Codex).
- `resolveSessionStem` (`paths.go:392-410`, 18 LOC): Iterates through Claude-specific project directories `~/.claude/projects/*/*.jsonl`.

These two functions query completely different on-disk directory layouts and data formats. The claim that `resolveSessionCatalog` duplicates `resolveSessionStem` is **WRONG**.

#### B. Project Label Fallback in `sessionHitFromCatalog`
In `internal/paths/paths.go:366-390`:
```go
func sessionHitFromCatalog(entry CatalogEntry) SessionHit {
	cwd := entry.CWD
	if cwd == "" {
		cwd = firstCWD(entry.TranscriptPath)
	}
	var proj string
	if pdir := ProjectDirOf(entry.TranscriptPath); pdir != "" {
		proj = ProjectLabel(pdir)
	} else if cwd != "" {
		if base := baseName(strings.TrimRight(cwd, "/")); base != "" {
			proj = base
		} else {
			proj = cwd
		}
	}
	if proj == "" {
		proj = ProjectLabel(filepath.Dir(entry.TranscriptPath))
	}
	return SessionHit{
		SessionID: entry.SessionID,
		Path:      entry.TranscriptPath,
		CWD:       cwd,
		Project:   proj,
	}
}
```

The fallback ladder (lines 374–380, 7 lines):
```go
	} else if cwd != "" {
		if base := baseName(strings.TrimRight(cwd, "/")); base != "" {
			proj = base
		} else {
			proj = cwd
		}
	}
```
handles non-Claude sessions (Goose, Antigravity, Codex) whose transcripts reside outside `~/.claude/projects/` (`ProjectDirOf` returns `""`).

`ProjectLabel(tdir string)` (`paths.go:217-226`, 10 LOC) expects a Claude project directory and samples `DirCWD(tdir)`. If a helper `ProjectLabelFromCWD(cwd, fallbackDir string)` is extracted, the net reduction is only **~10–14 LOC**.

### 5.3 Candidate 6 Summary
- Real removable lines: **~10–14 LOC**.
- Audit claim of ~50 LOC was overstated by **~4x–5x**.

---

## 6. Synthesis & Recommended Action Plan

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│ CANDIDATES 3–6 RECONCILIATION SUMMARY                                                  │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ Candidate 3 (consolidated.go):   Claimed ~210 LOC  ──►  Verified ~40–70 LOC  (OVERSTATED)│
│ Candidate 4 (cmd_ingest/tag):    Claimed ~120 LOC  ──►  Verified ~25–35 LOC  (OVERSTATED)│
│ Candidate 5 (sort / verdict):    Claimed  ~75 LOC  ──►  Verified ~60–75 LOC  (CONFIRMED) │
│ Candidate 6 (paths.go):          Claimed  ~50 LOC  ──►  Verified ~10–14 LOC  (OVERSTATED)│
├────────────────────────────────────────────────────────────────────────────────────────┤
│ TOTAL (Candidates 3–6):          Claimed ~455 LOC  ──►  Verified ~135–194 LOC (~35% real)│
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### Prioritized Implementation Recommendations

1. **High Priority (Immediate Clean Landing): Candidate 5 (~64–75 LOC)**
   - Export `view.SortCandidates` (or place in `internal/retrieve`) and remove the duplicate function in `internal/agentproto/agentproto.go`.
   - Remove `store.RoutineVerdictSet` from `internal/store/verdict.go` and replace call sites in `agentproto.go:2461, 2552` with `store.RoutineSet`, simultaneously fixing the *"real tag beats routine"* invariant violation.

2. **Medium Priority (Safe Modular Refactor): Candidate 3 PRAGMA Dedup (~29 LOC) & Candidate 4 `backingPath` (6 LOC)**
   - Add `srcTableColumns(con *sql.DB, table string) (map[string]struct{}, error)` in `internal/index/consolidated.go` and collapse `srcHasColumn` and `hasScopeColumns`.
   - Export `index.BackingFilePath(p string)` from `internal/index/containers.go` and delete private `backingPath` in `internal/cli/cmd_ingest.go`.

3. **Low Priority / Defer: Candidate 3 Legacy Watermarks & Candidate 6 Catalog Helpers (~20–30 LOC)**
   - Leave `rewriteLegacyWatermark` and `sessionHitFromCatalog` intact. The LOC savings are negligible (<25 LOC combined) and touch critical backward-compatibility and multi-adapter project resolution paths.
