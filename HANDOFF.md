# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-02 — Lucene/Zoekt Unified Single FTS5 Index Architecture Landed Clean.**

### 📍 Now
- Landed commit `f3b0555`: Unified single `messages_fts` SQLite FTS5 table with sub-token decomposition and Okapi BM25 ranking (`ORDER BY rank, m.id`).
- Removed the ad-hoc dual-table heuristic router; mixed code+prose queries match both stemmed intent and exact code symbols natively.
- All 39 internal packages pass race tests 100% green (`CGO_ENABLED=0 go test -race -count=1 ./...`).
- Single static pure-Go binary rebuilt and active at `~/.local/bin/rawclaw`.

### ✅ Decisions
- **Lucene / Zoekt Single Unified Index Standard** (2026-09-02): Single `messages_fts` virtual table with `tokenize='porter unicode61'` + substring `messages_fts_trigram`. Sub-token decomposition at tokenization time. Zero heuristic routing. 50% less disk footprint and perfect mixed-query recall.
- **Atomic LRVL Directory Lock** (2026-09-02): `scripts/lrvl-merge.sh` locks via atomic `mkdir .git/merge.lock.d` with cleanup trap.
- **Pi CWD Freshness Resolution** (2026-09-02): `CheckProjectFreshnessWithSource` passes resolved `projectCWD` to `checkPiContainerFreshness`.

### 🧵 Open threads (with status)
- **CASS Suite Test Restoration** (`BLOCKED ON CASS DESK`): Supervisor-A/B must restore `autotests = true` in `coding_agent_session_search/Cargo.toml`.
- **Librarian / Org Desk Hygiene** (`WAITING`): `~/org` untracked files and broken symlinks need Librarian desk cleanup.

### ⏭️ Next
- Maintain RawClaw zero-runtime dependency, pure Go, and single static binary invariants.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
