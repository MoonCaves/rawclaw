# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-02 — Release v0.12.0 Cut & Pushed to GitHub.**

### 📍 Now
- Release `v0.12.0` tagged and pushed cleanly to `MoonCaves/rawclaw`.
- `rawclaw setup` executed: discovery and session-lifecycle hooks installed across Claude Code, Codex, and Antigravity pointing to the static binary at `~/.local/bin/rawclaw`.
- All 39 internal packages pass race tests 100% green (`CGO_ENABLED=0 go test -race -count=1 ./...`).

### ✅ Decisions
- **Release v0.12.0** (2026-09-02):
  - Lucene / Zoekt unified single FTS5 index (`messages_fts`) with sub-token decomposition and Okapi BM25 ranking.
  - Read-only connection discipline (`_pragma=query_only(1)`).
  - Silent hot-path logging (`slog.Debug` on fence acquire/release).
  - Estate defect hardening (POSIX atomic directory lock in `lrvl-merge.sh`, Pi CWD resolution, `git bundle verify` upfront, and Hermes DB fragment stripping).

### 🧵 Open threads (with status)
- **CASS Suite Test Restoration** (`BLOCKED ON CASS DESK`): Supervisor-A/B must restore `autotests = true` in `coding_agent_session_search/Cargo.toml`.
- **Librarian / Org Desk Hygiene** (`WAITING`): `~/org` untracked files and broken symlinks need Librarian desk cleanup.

### ⏭️ Next
- Maintain RawClaw zero-runtime dependency, pure Go, and single static binary invariants.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
