# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-04 — RawClaw v0.11.0 Released, Archive Bundle Seeding Shipped, PR #99 Merged, Fleet-Wide Deployment & Sync Complete Across All Nodes.**

### 📍 Now
- Release `v0.11.0` tagged, published, and curated on GitHub with release binaries and checksums.
- Commit `4abcc41` landed on `main` and pushed to `origin/main`:
  1. **Archive Bundle Seeding Seam (`export-bundle` & `--from-bundle`)**:
     - `rawclaw archive export-bundle <path>` packages local archive clone into a portable git bundle in ~45s.
     - `rawclaw archive init --from-bundle <path> <remote-url>` seeds new machines in ~15s, bypassing multi-GB WAN clone transfers.
     - Fail-closed safety: strictly verifies zero unpushed commits (`strandedCommits`) and acquires `clone.lock` with double-checked configuration verification.
     - 10-minute bounded watchdog floor (`exportBundleWatchdog = 10 * time.Minute`) in `internal/cli/timeout.go`.
  2. **PR #99 Merged (Closes #96)**:
     - Invalidates prewarm dump cache on `tag-write` and resolves project topics during delayed folds in `internal/cli/tagrefresh.go` and `internal/cli/cmd_prewarm.go`.
  3. **New Runtime Adapters & Hooks**:
     - Native Hermes Agent SQLite transcript reader (`~/.hermes/state.db`).
     - Native OpenCode & Crush transcript reader (`~/.local/share/opencode/`).
     - Native Pi coding agent transcript reader (`~/.pi/sessions/`).
     - Antigravity session-birth catalog registration and subagent lineage tracking.
     - Unified `rawclaw closeout <session_id>` guidance across all 7 runtimes.
  4. **Fleet-Wide Deployment & Bidirectional Sync**:
     - **jay-m4 (M4 Mac)**: Running `v0.11.0+`, all 7 runtime hooks wired via `rawclaw setup`.
     - **jay-m1 (M1 Mac)**: Woken up, updated to `v0.11.0+`, hooks wired, bidirectional sync executed (72 session files pushed & pulled).
     - **muppet-server (Linux amd64)**: Updated to `v0.11.0+`, hooks wired, archive pulled.
     - **ai-server (Linux amd64)**: Updated to `v0.11.0+`, hooks wired, seeded from bundle and fully synchronized.
     - **coolify-server (Gitea)**: Healthy, verified, protected by split packs + MIDX reachability bitmaps.
- Verified:
  - Full test suite passed with race detector: `CGO_ENABLED=0 go test -race -count=1 ./...` (100% green).
  - Formatting: `gofmt -l internal/` clean.
  - Multi-machine transcript search verified live on `jay-m4`.

### ✅ Decisions
- **Archive Bundle Seeding** (2026-09-02): Enables local multi-gigabyte seeding in 15 seconds from a portable git bundle instead of multi-hour network clones.
- **Fail-Closed Unpushed Protection** (2026-09-02): Disallows bundle exports and destructive clone wipes if unpushed commits cannot be verified zero.
- **Prewarm Dump Invalidation** (2026-09-02): Removes cached prewarm dumps immediately on `tag-write` to prevent stale topic window loops (PR #99).
- **Fleet-Wide Runtime Setup** (2026-09-04): Deploys discovery hooks and catalog extensions across Claude Code, Codex, Antigravity, Pi, OpenCode, Goose, and Hermes.

### 🧵 Open threads (with status)
- **Cluster Git Governance Critique**: Dispatched open-ended critique request to OLUMBRA, Marrowlight, and cluster peers on Agent Mail thread `cluster-git-governance`.

### ⏭️ Next
- Review peer feedback from Agent Mail thread `cluster-git-governance`.
- Monitor continuous background ingestion across active coding agent workflows.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
