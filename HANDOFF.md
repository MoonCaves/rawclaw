# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-04 — Real-Time Active Session Freshness Landed (Commit 2fdc193), Fleet Updated (M4, M1, muppet-server).**

### 📍 Now
Target commit state is `2fdc193`. RawClaw now provides real-time freshness for active coding sessions without running a daemon or external services:
1. **Delta Tail Ingestion on Read Verbs**: Synchronously ingests any unindexed turns in the caller's active session (`refreshActiveSessions(o.currentSession())`) using sub-20ms `appendTailIfPossible` before searching or browsing.
2. **Authoritative Per-Transcript Watermark Verification**: `CheckIndexFreshness` inspects all catalog entries against stored `file_index` watermarks (`mtime`, `size`, `fp`) with `paths.Realpath` canonicalization. Unrelated ingests advancing global timestamps can no longer mask stale active transcripts.
3. **Turn-Level Background Hooks**: Registered `UserPromptSubmit` in Claude Code setup to trigger background ingestion on prompt submission in addition to session start/stop.
4. **Fleet Binary Deployment**: Static binaries cross-compiled (`CGO_ENABLED=0`) and deployed across all three target environments:
   - **M4** (local): `/Users/jay-m4/.local/bin/rawclaw` (Darwin arm64)
   - **M1** (`jay-m1`): `/opt/homebrew/bin/rawclaw` (Darwin arm64)
   - **muppet-server** (`muppet-server`): `/usr/local/bin/rawclaw` (Linux x86_64)

### 👁️ Seen, not touched
- **FoggySnow Worktree**: `worktree-foggysnow-readonly-scopes` left parked.
- **Concurrent Test Receipt Isolation**: Added `HOME` isolation in `vectortopup_test.go` to prevent race conditions on shared log file during concurrent test execution.

### ✅ Decisions
- **Active-Session Delta Tail vs. Full Crawl** (2026-09-04): Search only refreshes the caller's active session (`currentSession`) synchronously, keeping search latency sub-0.3s while guaranteeing 100% freshness for the active agent's own turns. Arbitrary uncataloged sessions trigger an immediate answer with a staleness notice and background ingest.
- **Per-Transcript Watermark Checking** (2026-09-04): Addressed Codex review findings: replaced global `lastIngestTime` comparison and 10-entry cap with full catalog scan against `file_index` watermarks, with symlink-safe `paths.Realpath` matching and non-silent error propagation.

### 🧵 Open threads (with status)
- None.

### ⏭️ Next
- Monitor live session freshness in multi-agent workflows.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
