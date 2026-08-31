# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-02 — Synchronous Active-Project Search Freshness, Mac-wide Discovery Parity, and Fleet Reconnaissance.**

### 📍 Now
- RawClaw core engine is 100% green across all 32 internal packages with race detector (`CGO_ENABLED=0 go test -race -count=1 ./...`).
- Pushed commits through `ef510b5` to `origin/main` on GitHub (`MoonCaves/rawclaw`).

### ✅ Decisions
- **Synchronous Active-Project Freshness** (2026-09-01): `internal/cli/cli.go:1786-1815` invokes `refreshThisProject(o)` synchronously when active project is stale before returning query results.
- **Mac-Wide Global Rules Parity** (2026-09-01): Symlinked `/Users/jay-m4/AGENTS.md` across all 7 coding agent harness global configuration paths (`~/.gemini/config/AGENTS.md`, `~/.codex/instructions.md`, `~/.pi/agent/SYSTEM.md`, `~/.hermes/instructions.md`, `~/.config/opencode/instructions.md`, `~/.config/goose/custom_instructions.txt`).
- **Identity Standard** (2026-09-01): Standardized mandatory `[<AgentName> @ <CWD>]` opening and closing tags in top-level `AGENTS.md`.

### 🧵 Open threads (with status)
- **Standalone Headless Tagger Script** (`OPEN`): Dedicated zero-token JSON segment parser at `~/.cache/session-search/tagger-config.json` for headless tag writes.
- **Pi/OpenCode Live Context Injections** (`VERIFIED`): Lifecycle hooks in `setup.go` verified compatible with native session startup events.

### ⏭️ Next
- Monitor CASS / Luna PR merge progress from Supervisor B.
- Maintain RawClaw zero-runtime dependency and pure Go invariants.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
