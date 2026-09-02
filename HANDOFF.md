# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-03 — Incremental Byte-Offset Streaming Ingestion Shipped (Copied Across 20 Reference Projects), 6/6 Deterministic Harness Gates Passed, Fleet Currency Deployed.**

### 📍 Now
- Commit `38cac82` landed on `main` and pushed to `origin/main`:
  1. **Incremental Byte-Offset Transcript Ingestion** (`internal/index`, `internal/parse`):
     - Added `byte_offset INTEGER DEFAULT 0` column to `file_index` via additive PRAGMA migration.
     - Implemented `parse.StreamJSONLLines` using `bufio.Reader` over `io.LimitReader` — streams only unread tail bytes without allocating full files in RAM.
     - Replaced destructive `DELETE FROM messages` full reloads with non-destructive `INSERT OR IGNORE` appending.
     - Integrated Filebeat/Litestream-style truncation/rotation recovery (`if size < offset { offset = 0 }`).
     - Added boundary validation: uncommitted partial lines keep the existing watermark until flushed.
     - Added `BenchmarkIncrementalAppend` pinning end-to-end tail ingestion vs full reload.
  2. **Prior Initiatives Active & Preserved**:
     - Git worktree root resolution and multi-shard project query widening.
     - Durable tombstone storage in `$XDG_DATA_HOME/rawclaw/.deleted` with unconditional legacy union.
     - Modular 5-file CLI split.
     - Dynamic source registry iteration.
     - Zero-ldflags toolchain buildinfo stamping.
- Verified:
  - Full test suite passed with race detector: `CGO_ENABLED=0 go test -race -count=1 ./...` (39/39 packages).
  - Linter: `golangci-lint run ./...` reports **0 issues**.
  - All 6 deterministic harness gates passed (`sh scripts/harness-gate.sh`).
  - Graphify AST knowledge graph refreshed (4,117 nodes, 11,791 edges, 246 communities).
  - Fleet binary `rawclaw v0.8.0 (commit 38cac82)` deployed live across **Mac HQ**, **jay-m1**, **muppet-server**, and **ai-server**.

### ✅ Decisions
- **Streaming Byte-Offset Watermarking** (2026-09-03): Tracks `byte_offset` on `file_index` to stream-decode only appended tail bytes on live sessions instead of full-file re-parsing.
- **Non-Destructive Tail Inserts** (2026-09-03): Replaces whole-session DELETE/INSERT cycles with atomic incremental appends and session metadata updates.
- **Truncation/Rotation Fallback** (2026-09-03): Reverts to byte 0 full parse only if a file is rewritten or truncated (`size < byte_offset`).
- **Query-Side Worktree Widening** (2026-09-03): Widens `--this-project` via `GitCommonRoot` and `Projects []string` filter at query time rather than resharding database tables.
- **Durable Flat Append-Only Tombstones** (2026-09-03): Preserves flat text append-only atomicity in permanent XDG data storage instead of introducing JSON lock contention.

### 🧵 Open threads (with status)
- None. Incremental byte-offset streaming ingestion is completed, verified, audited, and deployed.

### ⏭️ Next
- Monitor live subagent concurrent search performance and tail ingestion latency in active coding workflows.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
