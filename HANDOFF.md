# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-02 — CASS-Identical Dual FTS5 Indexing & Query Router Landed Clean.**

### 📍 Now
- Landed commit `d25446c`: Replaced hand-rolled weighting with the exact dual-index architecture from CASS (`messages_fts` with `porter unicode61 remove_diacritics 2` + `messages_code_fts` with `unicode61 tokenchars '-_./:@#$%\\'`) and CASS's exact `DetectSearchMode` query router.
- All 39 internal packages pass race tests 100% green (`CGO_ENABLED=0 go test -race -count=1 ./...`).
- Single static pure-Go binary rebuilt and active at `~/.local/bin/rawclaw`.

### ✅ Decisions
- **CASS Dual-Table FTS5 Standard** (2026-09-02): Adopted CASS's production dual-index schema from `src/pages/export.rs:252-266` and query router from `src/pages/fts.rs:50-128`. Zero hand-rolled weights. Exact code symbols and file paths are indexed and routed natively by SQLite FTS5.
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
