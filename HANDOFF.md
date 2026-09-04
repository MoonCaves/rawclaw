# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-04 — Target Commit 954a45f Verified, Search Latency Sub-0.3s, Vector Opt-In Confirmed.**

### 📍 Now
Target commit state is `954a45f` / `fdf3418`. Measured search latency on the installed binary (`/Users/jay-m4/.local/bin/rawclaw`) is **0.20s–0.35s** for pure lexical searches (down from 5.2s). Verified `--vector` is fully wired: on queries without saturated lexical hits, `--vector` invokes KNN scan over 124,407 chunks in `~/.cache/session-search/consolidated.db` (measuring ~1.75s). The core architectural lesson holds: the fastest search is the one that does not run the synchronous multi-runtime crawl or the default vector tier.

### 👁️ Seen, not touched
- **Withheld Turn Note in Oneline / Piped Stream**: Running `rawclaw '"sit and wait for that"' --this-project 2>&1 | head -3` in an agent environment (or piped) routes through `machineStream(out)`, which sets `o.Oneline = true`. `RenderSearchOneline` emits tab-separated match rows only and intentionally omits conversational notes/warnings to preserve machine-stream purity (enforced by `cmd_root_oneline_test.go`). In `--json`, the turn exclusion fires as designed (`excluded_current_turn: 1`, warning `current_turn_excluded`). In human multi-line mode, warnings are rendered as footers below all match entries rather than in the top 3 lines. Left strictly untouched per instructions.
- **FoggySnow Worktree**: `worktree-foggysnow-readonly-scopes` left parked.

### ✅ Decisions
- **Vector Tier Opt-In (`--vector`)** (2026-09-04): Keyword search is the sovereign default invariant. Brute-force KNN and coverage scans over machine-wide vector tables must never run implicitly on rare phrases.
- **NoOptDefVal Deprecation on Value Flags** (2026-09-04): Removed `NoOptDefVal` from `--budget` and `--more` in `cli_read.go` to prevent Cobra from treating space-separated values as extra positional arguments.
- **Actionable Outline Refs** (2026-09-04): Outlines render `<sess8>:<uuid8>` tokens directly accepted by `rawclaw read`.

### 🧵 Open threads (with status)
- None.

### ⏭️ Next
- Keep monitoring agent usage of `rawclaw` keyword search and copy-paste outline refs.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
