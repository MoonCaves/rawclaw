# Roadmap

RawClaw's forward plan. The north star doesn't change: **a single static binary, keyword search by
default, zero runtime dependencies, no LLM, no API key required.** Every item below is weighed against that —
anything that would drag a service, a model, or cgo into the default path stays optional and opt-in.

Items are marked **(planned)**, **(exploring)**, or **(speculative)**. Speculative means we like the
idea but haven't committed to the design or proven it pays for its complexity. Nothing here is a promise
of timing; it's the direction, in roughly the order we expect to tackle it.

---

## Already shipped

So the forward plan below isn't mistaken for the whole product — what already works today, in the
single keyword binary:

- **Keyword FTS5 search as the default verb** — `rawclaw "query"` returns ranked hits with copyable
  read-refs and a never-silent completeness envelope; browse (no query → recent sessions).
- **All projects by default**, `--this-project` to narrow, `--list` to enumerate them.
- **Query hygiene built in:** boolean operators (`a NOT b`, `x OR y`), `"exact phrase"` adjacency,
  `term*` prefix — raw agent queries can't break FTS5 syntax.
- **Scoping flags:** `--role`, `--sort newest|oldest`, `--since`/`--before`, `--include-path` /
  `--exclude-path` (regex over the project's working dir), `--min-messages N` (drop thin/bootstrap
  threads), `--include-tools` / `--include-subagents` to widen past clean human text.
- **`--json` on every command** (search, browse, read, outline, stats, resume) + real exit codes.
- **Top-level `read` / `outline` verbs** — the agent read-protocol. Refs are **source-stable**
  (`<session8>:<uuid8>`, anchored on Claude Code's own message uuid, so a citation survives re-index and
  transcript appends), and **ambiguity is rejected git-style** (a colliding prefix returns candidates, never
  the wrong session/message). A `read` returns the message **whole by default**; `--budget N` is an opt-in
  ceiling for multi-message ranges; `--more` / `--around` **expand in place on the same ref** (no re-search);
  any trim emits the literal recovery command (`[+N chars · … --more]`) — never silent.
- **Incompleteness-as-data** — search reports which scopes were `searched` / `empty` / `skipped` / `stale`,
  so an agent never reads a partial result as complete.
- **LLM-free titles + noise filtering** — a session's "about" line comes from its first *substantive* user
  message (a "hi"-opener still gets a real title), preferring Claude Code's own `ai_title` / `summary` /
  `custom_title`; low-signal messages (warmup, `/clear`, command-envelope markup) are filtered from previews
  **without dropping the session**.
- **`--resume <session8>`** prints the paste-ready resume command (`claude --resume`, `codex resume`, `agy --conversation`) with `cd` to the recorded working directory.
- **`--stats`** corpus overview (per-project, or `--all` for everything).
- **Pluggable Source port & Multi-Agent Ingestion** — unified, source-agnostic indexing across **Claude Code**, **Codex**, **Google Antigravity (`agy`)**, and **Goose** transcripts with deterministic message UUIDs, subagent lineage tracking, and live write-through consolidation.
- **Optional RRF-fused semantic tier** — bring-your-own-embedder, pure-Go cosine over BLOB vectors
  (no numpy, no GPU), `--no-vector` for byte-identical keyword-only. See below for the tuning still ahead.

---

## Near term

### Additional Source Adapters

The `Source` port has shipped, with adapters covering the JSONL family (Claude Code, Codex,
Antigravity) and the first SQLite reader (Goose). The coding-agent runtimes still unread fall
into three format families:

- **SQLite** — Crush (`<repo>/.crush/crush.db`), opencode (`~/.local/share/opencode/opencode.db`).
- **JSON-array** — Cline / Roo Code (VS Code globalStorage `tasks/<id>/api_conversation_history.json`),
  Continue (`~/.continue/sessions/<id>.json`).
- **Markdown / flat files** — Aider (`.aider.chat.history.md`, role inferred from line prefix),
  Cursor (composer transcripts).

Each is one adapter implementing `Source`; the FTS5 index, fusion, rendering, and the `agent` protocol
stay identical. A `--source` flag (and auto-detection by path) lets one RawClaw search across mixed
histories, or scope to one tool. **(planned)**

> Why this is the opening: of those 11 runtimes, only 3 search their own session *content* (Codex, Gemini,
> Goose — all by substring or a TUI filter) and 4 ship a *title-only* filter dressed up as search. None rank,
> recap, or offer an agent read-protocol. RawClaw searches the conversation, not just its title.

> Design note: keep the port narrow. An adapter's only job is *records in canonical order*; everything
> downstream (goal → match → resolution shaping, tool/subagent filtering, budgeting) is format-agnostic
> and must stay that way. If an adapter needs a downstream change, that's a smell.

### Session lifecycle — archive, delete, fork, resume-here

Read-only recall is the core, but lightweight scriptable session management belongs here too, no TUI required:

- **`archive` / `delete`** — **shipped.** User-driven, never an auto-heuristic. Archive is a
  git-remote-backed multi-machine sync and backup mechanism; delete is filter-gated and dry-run-first
  (refuses to delete everything) with a tombstone file and vault removal so re-index won't resurrect a
  deleted session.
- **`fork` (`--fork-session`) + `here` (`--here`)** — pass-throughs to `claude` (fork a session; or copy its
  JSONL into the CWD's project and resume there). **(planned — WANTED:** powers the session-start
  resume/fork *offer* — "continue from here, or fork from the decision / *before* it for clean,
  un-tainted context." Thin wrappers over `claude --fork-session`/`--resume`. Note: lineage-dedup in
  search already collapses a session + its forks to one hit, so forking won't flood recall.)
- **`list` / `show` / `usage` subcommands** — composable, exit-code-clean siblings for `jq`/`fzf` pipelines.
  **(planned — deferred:** `--list` (projects), `--stats` (corpus), and the `read` / `outline` verbs
  already cover this ground; subcommands would add public surface for marginal gain.)

### Session recap — auto, background, out-of-band  **(planned — WANTED)**

A cheap per-session recap so the *next* session starts informed. Two nesting layers rawclaw owns:
**title** (1 line) ⊂ **recap** (begin → middle transitions → final/current state + sidequests, via
topic-section markers). rawclaw owns the per-session recap (its domain = transcripts); the recap can
**feed** an optional downstream aggregator, but rawclaw does not build that aggregator itself
(single responsibility).

- **We generate it ourselves.** Claude Code does NOT store a reusable session summary (compaction
  summaries are baked into history, not a clean field). So a background Haiku reads the transcript (via
  rawclaw) and writes a sidecar recap. The transcript IS the context, so the background agent needs no
  live main-agent context — it can poke around fully without touching the user's convo.
- **Trigger = out-of-band only (zero UX latency).** Never run recap work inside a hook. Hook
  contracts differ by runtime and are all too weak to own completion: Codex `SessionEnd` is
  synchronous even when async is requested; Claude Code `SessionEnd` has a short synchronous exit
  budget; Claude Code and Codex `Stop` fire each turn and unfinished async work may be cancelled at
  teardown; Antigravity command hooks are synchronous. A hook may only write a tiny catalog record,
  launch a detached RawClaw child, and exit. Correctness comes from durable local intent plus normal
  ingest/mtime retries, not from the hook firing. Use SessionStart as an acceleration path and a
  periodic idle-session scan as the reliable abandoned/window-closed catch. See
  [docs/design/tag-closeout-fast-path.md](docs/design/tag-closeout-fast-path.md) for the shared
  detached-publication contract.
- Likely shape: a `rawclaw recap <sess>` / `rawclaw title <sess>` verb so the logic lives in the tool,
  and hooks/cron just invoke it in the background.

### Progressive read — shipped

The read protocol (source-stable uuid refs, git-style ambiguity guards, whole-by-default reads with an opt-in
`--budget` ceiling, `--more` / `--around` expand-in-place, never-silent trims, and incompleteness-as-data)
**shipped** — see *Already shipped*. Likewise the LLM-free **titles + low-signal filtering**, and the
**`--debug-search`** scoring explainer (honest about bm25 / coverage / sort-overlay and routine sort-tier
regimes). Remaining refinements, **(planned)**: the orthogonal **`--with tools|thinking|subagents`** richness axis (layer detail
onto the *same* window, distinct from `--more` widening it), and **content-hash refs** as a second-phase
hardening for very large corpora (the `uuid8` prefix is already collision-guarded).

### Smaller polish

- **`CLAUDE_CLI_NAME`** honored alongside `CLAUDE_CONFIG_DIR`, for custom installs. **(planned)**

### Output ergonomics — grep-composability + mode discoverability

Line-oriented grep output is **shipped** via `--oneline` / `--format text|oneline|line|json`
(`<read_ref>\t<started_iso>\t<project>\t<snippet>`), alongside `--json` for `jq` composition.

Remaining discoverability refinements, **(planned)**: **auto-detect non-tty (piped) output and emit grep-friendly lines**
(the agent greps, it just works — zero knowledge required); failing that, a one-line stderr hint at the moment of grepping;
pair with **forgiving input parsing** (single-dash long flags, case-folding, typo-correction, `find`/`grep`→`search` aliases)
so an agent's natural attempt succeeds without knowing the exact syntax.

### Shell completion — shipped

A `rawclaw completion bash|zsh|fish|powershell` subcommand is **shipped** via `spf13/cobra`.
Dynamic completion for session-id prefixes and project paths remains an area for future exploration.

### Semantic scoring tuning

The optional vector channel works (brute-force cosine, reciprocal-rank fused with FTS5), but the
ranking is deliberately plain. Two tunings, both pure-Go and dependency-free:

- **Field-weighted scoring.** A hit in a session's *goal* or a human question should outweigh the same
  term buried in a long tool result. Weight by message role and position before fusion.
- **Recency bias.** An optional, tunable decay so a decision from last week can edge out an identical
  phrase from a year ago — off by default, surfaced as a flag/env knob, never silently reordering
  results. **(exploring)**

---

## Mid term

### Richer read-protocol verbs

The default search plus the `read` / `outline` verbs (all `--json`, all budgeted) are the surface an
LLM uses to recall its own history without burning context on whole transcripts. Candidate additions,
each keeping the budget discipline:

- **`timeline <session8>`** — a compact, ordered spine of a session (goals, decisions, hand-offs)
  cheaper than a full outline, for "what happened, in order."
- **`related <ref>`** — given one hit, surface adjacent sessions that share entities or pick up
  the same thread, so an agent can widen without re-querying blind.
- **`context <ref> --budget N`** — a single call that returns a hit *plus* its goal/resolution
  bookends pre-fit to a token budget, collapsing the common search→read round-trip.

All additive; the existing verbs and their JSON shapes don't change. **(exploring)**

### Indexing & freshness

- **Watch mode** (`rawclaw --watch`): continuous active transcript watcher and live store
  updater for long-lived agent processes, complementing hook-driven ingest and answer-first queries. **(exploring)**
- **Export / import the index**: move a built index between machines without re-reading every
  transcript — handy for CI caches and constrained hosts. **(speculative)**

---

## Longer term

### Optional ANN index for very large corpora — an explicit trade-off

The vector channel is brute-force cosine KNN: every query scans every stored vector. That's a feature,
not a gap — it's exact, it's pure Go, and it keeps the single-static-binary promise with no extra
service and no native code. For most histories it's plenty fast.

At a *very* large corpus (hundreds of thousands of messages with vectors enabled), brute force starts
to cost. The longer-term option is an **approximate nearest-neighbor (ANN) index** — most naturally via
an embeddable SQLite extension such as **sqlite-vec**, since vectors already live in the same on-disk db.

This is called out as a **deliberate trade-off, not a default**:

- A native extension means **cgo or a loadable module** — which dents the "one pure-Go static binary,
  no dependencies" guarantee that is the whole point of the keyword core.
- So if it lands, it lands as a **build tag / opt-in tier**, exactly like the embedder: the default
  binary stays brute-force-or-keyword, and only users who explicitly want ANN over a huge corpus opt
  into a build that carries it.
- It only earns its place if real corpora prove brute force is the bottleneck. Until then, profiling
  and tightening the pure-Go path comes first. **(speculative)**

### Beyond a single machine

- **Read-only index sharing**: point RawClaw at a prebuilt index (e.g. a teammate's, or a CI artifact)
  for query-only recall, without owning the source transcripts. **(speculative)**
- **Pluggable index backend**: the storage layer is already behind its own package; in principle the
  same FTS-and-fusion logic could sit on a different store. Far out, and only if a concrete need
  appears — we won't abstract for its own sake. **(speculative)**

---

## Non-goals

To keep the roadmap honest, things we are intentionally **not** planning:

- **No LLM in the loop.** RawClaw *retrieves*; it never calls a model to do its job. It finds the session;
  your agent (which already *is* an LLM) does any reasoning over what it finds. The optional embedder for
  semantic search is the one exception — opt-in, bring-your-own-endpoint, never required.
- **No bundled model or required API key in the default path.** Keyword search must always work offline,
  with no network and no key. Embeddings stay opt-in, bring-your-own-endpoint.
- **No cgo in the default build.** Pure Go, cross-compiles cleanly, single static file. Any native
  dependency (e.g. an ANN extension) is an opt-in build tier, never the default.
- **No daemon requirement.** RawClaw is a CLI you run; watch mode, if it ships, is a convenience, not a
  prerequisite.
- **No lock-in to one agent CLI.** The Source port exists precisely so RawClaw isn't married to a single
  transcript format.

---

*Have a source format you want read, an agent verb you'd use, or a corpus big enough to stress brute-force
search? Open an issue at* `github.com/MoonCaves/rawclaw` *— concrete use cases move items up this list.*
