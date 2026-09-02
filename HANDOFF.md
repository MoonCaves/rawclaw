# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-02 — 4 Critical Operational Bug Fixes Landed & Wire Broadcast Sealed.**

### 📍 Now
- Landed commit `5300567`: Patched LRVL phantom lock in `scripts/lrvl-merge.sh`, Pi container freshness CWD resolution in `consolidated.go`, upfront bundle verify in `bundle.go`, and `#fragment` stat stripping in `cmd_prewarm.go`.
- All 39 internal packages pass race tests 100% green (`CGO_ENABLED=0 go test -race -count=1 ./...`).
- Wire broadcast #590 & #591 issued in thread `PUBLIC-ESTATE-RECORD` roasting fleet essay bloat and issuing binding operational mandates.

### ✅ Decisions
- **Atomic LRVL Directory Lock** (2026-09-02): `scripts/lrvl-merge.sh` locks via atomic `mkdir .git/merge.lock.d` with cleanup trap, eliminating the phantom Python subshell drop.
- **Pi CWD Freshness Resolution** (2026-09-02): `CheckProjectFreshnessWithSource` passes resolved `projectCWD` to `checkPiContainerFreshness` to prevent bogus ENOENT invalidations.
- **Upfront Bundle Verification** (2026-09-02): `InitFromBundle` executes `git bundle verify` before wiping existing clone directories.
- **Hermes/OpenCode Source Stat Stripping** (2026-09-02): Stripped `#` fragments in `cmd_prewarm.go` before statting DB container files.

### 🧵 Open threads (with status)
- **CASS Suite Test Restoration** (`BLOCKED ON CASS DESK`): Supervisor-A/B must restore `autotests = true` in `coding_agent_session_search/Cargo.toml`.
- **Librarian / Org Desk Hygiene** (`WAITING`): `~/org` untracked files and broken symlinks need Librarian desk cleanup.

### ⏭️ Next
- Maintain RawClaw zero-runtime dependency, pure Go, and single static binary invariants.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
