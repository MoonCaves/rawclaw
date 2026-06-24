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
- **`--resume <session8>`** prints the paste-ready `claude --resume` command (with `cd` to the cwd).
- **`--stats`** corpus overview (per-project, or `--all` for everything).
- **Optional RRF-fused semantic tier** — bring-your-own-embedder, pure-Go cosine over BLOB vectors
  (no numpy, no GPU), `--no-vector` for byte-identical keyword-only. See below for the tuning still ahead.

---

## Near term

### A pluggable Source port

Today the index is fed by one reader: Claude Code's JSONL transcripts under `~/.claude/projects`.
That reader already lives behind a clean seam — parse a record, flatten it to searchable text, order
it within a session. The next step is to lift that seam into an explicit **`Source` port**: a small
interface (enumerate sessions → stream ordered messages with role, timestamp, and raw text) that the
index consumes without knowing the file format underneath.

Once the port exists, other agent CLIs become adapters, not rewrites — these are just on-disk logs.
A survey of 11 coding-agent runtimes found their session formats fall into **four families**, so the
port needs four adapter shapes, not one:

- **JSONL** — Claude Code (`~/.claude/projects/<cwd>/*.jsonl`), Codex (`~/.codex/sessions/**/rollout-*.jsonl`),
  Gemini CLI (`~/.gemini/tmp/<hash>/chats/session-*.jsonl`), Qwen Code (`~/.qwen/projects/<cwd>/chats/*.jsonl`).
- **SQLite** — Goose (`~/.local/share/goose/sessions/sessions.db`), Crush (`<repo>/.crush/crush.db`),
  opencode (`~/.local/share/opencode/opencode.db`).
- **JSON-array** — Cline / Roo Code (VS Code globalStorage `tasks/<id>/api_conversation_history.json`),
  Continue (`~/.continue/sessions/<id>.json`).
- **Markdown** — Aider (`.aider.chat.history.md`, role inferred from line prefix).

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

- **`archive` / `delete`** — **built; cli wiring landing.** User-driven, never an auto-heuristic. Archive is a
  filesystem move (default `~/.claude/archive/`); delete is filter-gated and dry-run-first (refuses to delete
  everything) with a tombstone file so re-index won't resurrect a deleted session.
- **`fork` (`--fork-session`) + `here` (`--here`)** — pass-throughs to `claude` (fork a session; or copy its
  JSONL into the CWD's project and resume there). **(planned — WANTED:** powers the session-start
  resume/fork *offer* — "continue from here, or fork from the decision / *before* it for clean,
  un-tainted context." Thin wrappers over `claude --fork-session`/`--resume`. Note: lineage-dedup in
  search already collapses a session + its forks to one hit, so forking won't flood recall.)
- **`list` / `show` / `usage` subcommands** — composable, exit-code-clean siblings for `jq`/`fzf` pipelines.
  **(planned — deferred:** `--list` (projects), `--stats` (corpus), and the `read` / `outline` verbs
  already cover this ground; subcommands would add public surface for marginal gain.)

### Session recap — auto, background, out-of-band  **(planned — WANTED)**

A cheap per-session recap so the *next* session (and the org) starts informed. Three nesting layers:
**title** (1 line) ⊂ **recap** (begin → middle transitions → final/current state + sidequests, via
topic-section markers) ⊂ **central digest** (aggregator aggregates many recaps org-wide). rawclaw owns the
per-session recap (its domain = transcripts); the recap **feeds** aggregator's central digest — rawclaw
does not build the bulletin itself (single responsibility).

- **We generate it ourselves.** Claude Code does NOT store a reusable session summary (compaction
  summaries are baked into history, not a clean field). So a background Haiku reads the transcript (via
  rawclaw) and writes a sidecar recap. The transcript IS the context, so the background agent needs no
  live main-agent context — it can poke around fully without touching the user's convo.
- **Trigger = out-of-band only (zero UX latency).** Never run the recap synchronously in the user's
  flow (`Stop` fires every turn AND blocks — disqualified for the heavy work). Use:
  (1) **SessionStart-lazy** — on start/resume, background-recap the *previous* session (non-blocking;
  catches the common abandoned-session case on next return); plus
  (2) **cron mtime-scan** — periodically recap sessions idle > N min (the only reliable catch for
  abandoned/window-closed sessions — Claude fires NO event on abandonment; `SessionEnd` only fires on
  clean `/exit`). `Stop` is reserved at most for ultra-light incremental *titling* (background-spawn +
  exit, never synchronous work).
- Likely shape: a `rawclaw recap <sess>` / `rawclaw title <sess>` verb so the logic lives in the tool,
  and hooks/cron just invoke it in the background.

### Progressive read — shipped

The read protocol (source-stable uuid refs, git-style ambiguity guards, whole-by-default reads with an opt-in
`--budget` ceiling, `--more` / `--around` expand-in-place, never-silent trims, and incompleteness-as-data)
**shipped** — see *Already shipped*. Likewise the LLM-free **titles + low-signal filtering**. Remaining
refinements, **(planned)**: the orthogonal **`--with tools|thinking|subagents`** richness axis (layer detail
onto the *same* window, distinct from `--more` widening it), and **content-hash refs** as a second-phase
hardening for very large corpora (the `uuid8` prefix is already collision-guarded). The **`--debug-search`**
scoring explainer — honest about RawClaw's actual bm25 / bm25+coverage / sort-overlay regimes (there is no
composite score to fake) — is **built; cli wiring landing.**

### Smaller polish

- **`CLAUDE_CLI_NAME`** honored alongside `CLAUDE_CONFIG_DIR`, for custom installs. **(planned)**

### Output ergonomics — grep-composability + mode discoverability

Live signal (2026-06-20): an agent piped the default discovery output through `grep`, requiring the date and a
content keyword on the *same* line — but the default view puts the date on the `━━` header and the content on
separate `▶`/role lines, so they never co-occur, and the filter silently dropped real hits. The multi-line view
is right for *reading* a result; it's hostile to *line-filtering across* results — and agents reach for `grep`
by default. `--json` already composes with `jq`; a self-contained one-line hit mode
(`iso · session8 · role · snippet`) for `grep` is still wanted. The gap is **discoverability** (agents
don't reach for it) as much as the mode itself.

**Governing tenet — self-evident like Google, no guide needed.** Order of preference for closing *any* agent-usage
gap, worst → best: (1) ship a **skill** (knowledge in the agent's head — it must read a guide first); (2) compress
that into the **tool's own menu / `--help`** (knowledge on the surface, shown on use, not a separate file);
(3) **make the tool absorb the native behavior itself** so nothing has to be read at all — like Google handling
any query. Skills and menu-hints are scaffolding for where the tool isn't self-evident *yet*; treat their contents
as a punch-list of self-evidence gaps to retire.

Applied here, best → worst: **auto-detect non-tty (piped) output and emit grep-friendly lines** (the agent greps,
it just works — zero knowledge required); failing that, a one-line stderr hint at the moment of grepping; failing
that, the README documents `--json` (and a one-line hit mode, once it lands). Pair with **forgiving input parsing**
(single-dash long flags, case-folding, typo-correction, `find`/`grep`→`search` aliases) so an agent's
natural attempt succeeds without knowing the exact syntax. **(planned)**

### Shell completion

`spf13/cobra` ships completion generation; we just haven't wired the command yet. Plan: a
`rawclaw completion bash|zsh|fish|powershell` subcommand, plus dynamic completion for the arguments
that benefit most — session-id prefixes (for `read` / `outline` / `--resume`), known project
paths (for `--include-path` / `--exclude-path`), and tool names (for `--include-tools`). The session-id
and project completers read straight from the existing index, so they cost nothing extra to maintain. **(planned)**

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

- **Watch mode** (`rawclaw --watch`): keep the index warm as transcripts grow, instead of the
  incremental refresh-on-run we do today. Useful for long-lived agent processes. **(exploring)**
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
