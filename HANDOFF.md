# HANDOFF — rawclaw

> **HANDOFF = where I am right now** — a *rebuilt summary*, not a scratchpad. On close, REBUILD this
> in the structure below (condense — don't append-pad); **absolute dates only** (never "today"/"recently");
> keep it lean (raw detail → mnemon/log, not here). You can't fill these sections honestly without
> reading the current state first — that's the point.

<!-- ───── header above is managed · write/edit your current state below ───── -->

**2026-09-04 — Search Latency Sub-0.5s Certified, Vector Tier Made Opt-In, Cobra Flag Bug Fixed, Outline Ref Pipe Repaired.**

### 📍 Now
- Commits landed to address interactive search latency and CLI friction:
  1. **Vector Tier Opt-In & MeasureCoverage Stripped (`cli_options.go`, `cli_search.go`, `search.go`)**:
     - Made vector/semantic tier strictly opt-in per query via `--vector`, ensuring keyword search runs as pure zero-overhead default.
     - Stripped `MeasureCoverage` from the interactive search path, eliminating 100MB+ transcript scanning and SHA hashing during queries.
     - Interactive search on installed binary measures **0.39s–0.46s** (down from 5.2s).
  2. **Cobra Flag Space Parsing Repaired (`cli_read.go`)**:
     - Deleted `NoOptDefVal` from `--budget` and `--more` flags in `cli_read.go`. Space-separated arguments (e.g. `rawclaw read <ref> --budget 20000 --more 2`) now parse cleanly without triggering Cobra argument count errors.
  3. **Outline Ref Pipe Repaired (`store/messages.go`, `view/view.go`, `agentproto/render.go`)**:
     - `ViewMsg` and `store.Msg` now propagate message `UUID`.
     - `rawclaw outline` prints actionable `[role <sess8>:<uuid8>]` refs directly readable via `rawclaw read <ref>`.
  4. **Phrase Search Help Guidance (`cli_options.go`)**:
     - Added explicit documentation in search `--help` steering agents to quote multi-word exact phrases.
- Verified:
  - All 6 deterministic harness gates passed (`sh scripts/harness-gate.sh`) with 0 race conditions, 0 deadlocks, and 100% `gofmt` compliance.
  - Linter: `golangci-lint run ./...` reports **0 issues**.
  - Graphify AST knowledge graph refreshed (4,140 nodes, 11,863 edges, 254 communities).
  - Deployed binary `~/.local/bin/rawclaw` (MD5 `8734c9f904906f1d051a3eab36e63aeb`).
  - Benchmarks:
    - `time rawclaw "sit and wait for that" --this-project`: **0.390s – 0.465s** hot path.
    - `rawclaw read 197cdecc:aec45473 --budget 20000`: **PASS** (instant excerpt).
    - `rawclaw outline 197cdecc`: **PASS** (prints `[user 197cdecc:c1ef7bfe]`, directly accepted by `rawclaw read`).

### ✅ Decisions
- **Vector Tier Opt-In (`--vector`)** (2026-09-04): Pure keyword search is the sovereign default invariant. Brute-force KNN and coverage scans over machine-wide vector tables must never run implicitly on rare phrases.
- **NoOptDefVal Deprecation on Value Flags** (2026-09-04): Never attach `NoOptDefVal` to integer value flags in Cobra/pflag, as it forces `=` syntax and causes space-separated values to be misparsed as positional args.
- **Actionable Outline Refs** (2026-09-04): Outlines render `<sess8>:<uuid8>` tokens so agents can copy-paste refs directly into `read` without conversion.

### 🧵 Open threads (with status)
- **Fleet Sync**: Commit clean changes and push to `origin/main`.

### ⏭️ Next
- Commit and push changes.

### ⛔ Blockers
- None.

### ⚠️ Contested
- None.
