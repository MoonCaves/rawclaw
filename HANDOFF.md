# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-02 — Multi-Column Role-Weighted BM25F & Code Tokenizer Landed Clean.**

### 📍 Now
- Landed commit `2301dd1`: Multi-column role-weighted BM25F SQLite FTS5 index (`user_prompt`, `assistant_text`, `tool_output`) with 5.0x / 2.0x / 0.1x weight vectors and standard code-aware identifier tokenizer in `internal/text/tokenize.go`.
- All 39 internal packages pass race tests 100% green (`CGO_ENABLED=0 go test -race -count=1 ./...`).
- Single static binary rebuilt and active at `~/.local/bin/rawclaw`.

### ✅ Decisions
- **Multi-Column Role-Weighted BM25F** (2026-09-02): Standard SQLite FTS5 table `messages_fts` splits content into `user_prompt`, `assistant_text`, `tool_output` via triggers. Ranked queries apply `bm25(messages_fts, 5.0, 2.0, 0.1)` so human prompts and assistant reasoning outrank raw compiler dumps without external models.
- **Standard Code-Aware Tokenizer** (2026-09-02): `internal/text/tokenize.go` exports `SplitCodeIdentifier` using standard regex/path splitting for camelCase, snake_case, PascalCase, and file paths.
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
