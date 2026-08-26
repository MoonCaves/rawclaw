# FINDINGS: Session Catalog & Candidate Resolution Over-Engineering Audit

## Audit Method & Discipline
Scanned using `dupl` clone detection, `golangci-lint`, Graphify symbol analysis (`locateSession`, `searchOneStore`, `ResolveSession`, `Topics`), and Ponytail complexity reduction rules.

---

## Ranked Findings

### 1. `internal/agentproto/agentproto.go:1799-1804`
- **Tag:** `shrink` / `yagni`
- **What to cut:** Redundant single-use local closure `allowed := func(project string) bool` defined inside `catalogCands` to filter project scope.
- **Replacement:** Inline check `if projects != nil && !slices.Contains(projects, hit.Project) { continue }` directly in the loop.
- **Contract:** Identical filtering behavior: when `projects == nil`, all project hits pass; when non-nil, only hits whose project matches an allowed scope are retained.
- **Estimated Net Lines:** -6 lines.
- **Ruling:** **ACCEPTED**. Eliminates function allocation and boilerplate without touching semantics.

### 2. `internal/agentproto/agentproto.go:1428-1447`
- **Tag:** `stdlib`
- **What to cut:** Traditional `sort.SliceStable` index-based comparator closures.
- **Replacement:** `slices.SortStableFunc` with typed elements from Go stdlib `slices`.
- **Contract:** Stable sort ordering strictly preserved (`ISO` descending/ascending, routine partition, fused/cov ranking).
- **Estimated Net Lines:** -4 lines.
- **Ruling:** **DEFERRED / OUT OF IMMEDIATE ROOT CAUSE**. Keep candidate sort untouched to minimize blast radius on catalog flow.

### 3. `internal/cli/cmd_tag.go:171-210, 282-320`
- **Tag:** `shrink`
- **What to cut:** Redundant inline two-pass segment range resolution logic in `computeUntaggedWindow` and `findPrevSegment`.
- **Replacement:** Consolidated helper `resolveSegmentRange(seg, uuidToDispIdx, uuidToMsgID, displayable)`.
- **Contract:** Bounded index resolution against displayable message slice.
- **Estimated Net Lines:** -35 lines.
- **Ruling:** **OUT OF FENCE**. Fenced to `FINDINGS.md`, `internal/agentproto/agentproto.go`, `internal/cli/cmd_tag_onestore_test.go`.

---

## Summary of Rulings

| Rank | Path:Line | Tag | Est. Net | Ruling | Rationale |
|------|-----------|-----|----------|--------|-----------|
| 1 | `internal/agentproto/agentproto.go:1799` | `shrink` | -6 | **ACCEPTED** | Smallest evidence-backed root cause / YAGNI cleanup inside fence |
| 2 | `internal/agentproto/agentproto.go:1428` | `stdlib` | -4 | **DEFERRED** | Secondary candidate sort optimization; avoid collateral drift |
| 3 | `internal/cli/cmd_tag.go:171` | `shrink` | -35 | **REJECTED (FENCE)** | Outside mandatory file fence |

**Net Potential inside Fence:** -6 lines production code.
