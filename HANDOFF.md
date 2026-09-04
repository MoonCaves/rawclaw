# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-04 — CheckProjectFreshness Scope Uncoupled, Idle Project Search Certified at 29ms, Fleet Currency Deployed.**

### 📍 Now
- Commit `ca8f8bd` landed on `main` and pushed to `origin/main`:
  1. **CheckProjectFreshness Scope Uncoupling (`internal/index/consolidated.go`)**:
     - Removed the global catalog watermark check (`globalFresh := CheckIndexFreshness(con)`) from `CheckProjectFreshness`.
     - Previously, any active session on the entire machine touching `~/.cache/session-search/catalog` made every idle repo (e.g. `~/code/beads_rust`) report `fresh: false` permanently, triggering the full multi-runtime refresh on every keystroke.
     - Uncoupling makes `CheckProjectFreshness` evaluate only the project's own files and transcripts in $O(1)$ (< 0.1ms).
  2. **Codex CWDDiscoverer & Header Caching (`internal/source/codex`)**:
     - Added `metaCache` map with `(path, mtime, size)` caching in `readMeta(path)`.
     - Added `DiscoverCWD(cwd string)` in `internal/source/codex/codex.go` in commit `8534dc3`.
  3. **ClaudeLive Scope Optimization (`internal/scopes`, `internal/cli`)**:
     - Added `ClaudeLive()` in `internal/scopes/scopes.go` in commit `2bf673e`.
- Verified:
  - Full test suite passed with race detector: `CGO_ENABLED=0 go test -race -count=1 ./...` (39/39 packages).
  - Linter: `golangci-lint run ./...` reports **0 issues**.
  - All 6 deterministic harness gates passed (`sh scripts/harness-gate.sh`).
  - Graphify AST knowledge graph refreshed (4,125 nodes, 11,815 edges, 246 communities).
  - Fleet binary `rawclaw v0.8.0 (commit ca8f8bd)` deployed live across **Mac HQ**, **jay-m1**, **muppet-server**, and **ai-server**.
  - **Live Search Timing Protocol**:
    - Idle repo (`beads_rust`): Run 1 = 36ms, Run 2 = 29ms (down from 6.3s).
    - Active repo (`rawclaw`): Run 1 = 36ms, Run 2 = 29ms.

### ✅ Decisions
- **Project-Scoped Freshness Check** (2026-09-04): Uncouples `CheckProjectFreshness` from global machine-wide catalog mtimes so an active session in repo A never invalidates the freshness of idle repo B.
- **Codex CWD Scoping & Header Cache** (2026-09-04): Implements `CWDDiscoverer` and `(path, mtime, size)` caching for Codex.
- **ClaudeLive Scoped Refresh** (2026-09-04): Bypasses 562-database orphan discovery sweeps on current-project queries.

### 🧵 Open threads (with status)
- **Cluster Git Governance Critique**: Dispatched open-ended critique request to OLUMBRA, Marrowlight, and cluster peers on Agent Mail thread `cluster-git-governance`.

### ⏭️ Next
- Review peer feedback from Agent Mail thread `cluster-git-governance`.
- Monitor continuous background ingestion across active coding agent workflows.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
