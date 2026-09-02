# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-03 — 4 Architectural Cleanups Shipped, Audited by Codex Luna Medium & FoggySnow, 6/6 Deterministic Harness Gates Passed, Fleet Currency Deployed.**

### 📍 Now
- Commit `bf8d2ad` landed on `main` and pushed to `origin/main`:
  1. **Git Worktree Scoping & Multi-Shard Resolution** (`internal/paths`, `internal/agentproto`, `internal/scopes`, `internal/cli`):
     - `paths.GitCommonRoot` follows `.git` pointer files and `commondir` back to the parent repository root.
     - `Projects []string` query filter transparently searches sessions across all sibling worktrees without database resharding.
  2. **Durable Tombstone Vault Relocation & Legacy Union** (`internal/lifecycle`, `internal/paths`):
     - Relocated default `.deleted` file from wipeable cache to `$XDG_DATA_HOME/rawclaw/.deleted` (`~/.local/share/rawclaw/.deleted`).
     - Kept flat atomic append-only format (`O_APPEND | O_CREATE | O_WRONLY`).
     - `LoadTombstones` unconditionally merges legacy `~/.cache/session-search/.deleted` entries so existing deletions are never lost.
  3. **Modular CLI Split** (`internal/cli`):
     - Decomposed 2,284-line monolithic `cli.go` into 5 single-responsibility files: `cli_options.go`, `cli_search.go`, `cli_read.go`, `cli_lifecycle.go`, and root `cli.go`.
     - 100% backwards-compatible flag, UX, and exit code contracts preserved.
  4. **Unified Source Registration & Dynamic Discovery** (`internal/sources`, `internal/scopes`, `internal/source`):
     - Dynamic loop over `sources.Registered()` across `scopes.All` and `refreshThisProject` eliminates hardcoded adapter switch statements.
     - Added `Label` and `OptedIn` capability hooks on `source.Registration`.
  5. **Automatic Toolchain Buildinfo Stamping** (`cmd/rawclaw/version.go`):
     - Integrated `runtime/debug.ReadBuildInfo()` so unstamped `go build` / `go install` binaries automatically display exact Git commit and build timestamp.
- Verified:
  - Full test suite passed with race detector: `CGO_ENABLED=0 go test -race -count=1 ./...` (39/39 packages).
  - Linter: `golangci-lint run ./...` reports **0 issues**.
  - All 6 deterministic harness gates passed (`sh scripts/harness-gate.sh`).
  - Graphify AST knowledge graph refreshed (4,117 nodes, 11,791 edges, 246 communities).
  - Fleet binary `rawclaw v0.8.0 (commit bf8d2ad)` deployed live across **Mac HQ**, **jay-m1**, **muppet-server**, and **ai-server**.

### ✅ Decisions
- **Query-Side Worktree Widening** (2026-09-03): Widens `--this-project` via `GitCommonRoot` and `Projects []string` filter at query time rather than resharding database tables.
- **Durable Flat Append-Only Tombstones** (2026-09-03): Preserves flat text append-only atomicity in permanent XDG data storage instead of introducing JSON lock contention.
- **Unconditional Legacy Tombstone Union** (2026-09-03): Merges legacy cache entries alongside durable XDG storage so prior deletions remain active across mixed environments.
- **Zero-Dep Source Registry** (2026-09-03): Wires discovery and project refresh through `sources.Registered()` while keeping parsers pure Go adapters.

### 🧵 Open threads (with status)
- None. All 4 targeted improvements are completed, verified, audited, and deployed.

### ⏭️ Next
- Monitor cross-worktree search latency and fleet sync during daily coding workflows.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
