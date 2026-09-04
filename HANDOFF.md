# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-04 — Codex & Antigravity CWDDiscoverer Shipped, Scoped In-Repo Search Certified at 30–37ms, Fleet Currency Deployed.**

### 📍 Now
- Commit `8534dc3` landed on `main` and pushed to `origin/main`:
  1. **Codex CWDDiscoverer & Header Caching (`internal/source/codex`)**:
     - Added `metaCache` map with `(path, mtime, size)` caching in `readMeta(path)` — eliminates 16MB buffer allocations and repeated JSON unmarshals across 1,236 Codex rollouts.
     - Added `DiscoverCWD(cwd string)` in `internal/source/codex/codex.go` — filters rollouts directly down to matching CWD, dropping Codex scoped refresh from 4.1s–9.7s to **43ms**.
     - Pinned with `TestDiscoverCWD`.
  2. **ClaudeLive Scope Optimization (`internal/scopes`, `internal/cli`)**:
     - Added `ClaudeLive()` in `internal/scopes/scopes.go` and wired into `refreshThisProject` in commit `2bf673e` — bypasses opening 562 orphan shard databases on in-repo search.
  3. **Antigravity CWDDiscoverer & Full-File Subagent Lineage Cache (`internal/source/antigravity`)**:
     - Added `DiscoverCWD(cwd)` and `(path, mtime, size)` cache storing header and subagent lineage in commit `3ff274d`.
- Verified:
  - Full test suite passed with race detector: `CGO_ENABLED=0 go test -race -count=1 ./...` (39/39 packages).
  - Linter: `golangci-lint run ./...` reports **0 issues**.
  - All 6 deterministic harness gates passed (`sh scripts/harness-gate.sh`).
  - Graphify AST knowledge graph refreshed (4,125 nodes, 11,815 edges, 246 communities).
  - Fleet binary `rawclaw v0.8.0 (commit 8534dc3)` deployed live across **Mac HQ**, **jay-m1**, **muppet-server**, and **ai-server**.
  - **Live Search Timing Protocol**:
    - Codex-only (`--this-project --source codex`): Run 1 = 46ms, Run 2 = 43ms.
    - Full in-repo search (`--this-project`): Run 1 = 37ms, Run 2 = 30ms.

### ✅ Decisions
- **Codex CWD Scoping & Header Cache** (2026-09-04): Implements `CWDDiscoverer` and `(path, mtime, size)` caching for Codex so 1,236 session files don't block in-repo searches.
- **ClaudeLive Scoped Refresh** (2026-09-04): Bypasses 562-database orphan discovery sweeps on current-project queries.
- **Full-File Subagent Scan with Mtime Cache** (2026-09-04): Caches `(hdr, children)` by `(path, mtime, size)` so unchanged files cost 0 file reads.

### 🧵 Open threads (with status)
- **Cluster Git Governance Critique**: Dispatched open-ended critique request to OLUMBRA, Marrowlight, and cluster peers on Agent Mail thread `cluster-git-governance`.

### ⏭️ Next
- Review peer feedback from Agent Mail thread `cluster-git-governance`.
- Monitor continuous background ingestion across active coding agent workflows.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
