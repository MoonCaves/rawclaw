# Changelog

All notable changes to RawClaw are documented in this file.

## [Unreleased]

### Added

- **Transcript archive (tracer): `rawclaw archive init <remote-url>` + `rawclaw archive push`.**
  Any private git remote becomes durable, multi-machine storage for raw transcripts: init clones
  the repo (an empty remote works — it is born on the first push), registers the machine under a
  human-readable top-level dir (`--name`, default the sanitized hostname; a committed manifest
  maps the dir to a stable machine id, and init refuses a dir already claimed by a different
  machine), and prints a hard the-remote-must-be-PRIVATE warning. Push mirrors the Claude + Codex
  transcript trees into `<machine>/<source>/...`, copying only changed files (size + mtime +
  content-fingerprint quick check); nothing changed = no commit. Concurrent pushes from other
  machines are absorbed by a bounded pull-rebase-and-retry loop (never a force-push, never a
  clone left mid-rebase). Unconfigured, the feature is off: `archive push` is a clean no-op
  pointing at init.

## [0.4.0] — 2026-07-18

### Added

- **`rawclaw setup`** — one command to wire the discovery hook into your agent runtimes: a short
  session-start note teaching agents rawclaw exists and how to call it. Targets Claude Code
  (always) and Codex (when its config dir exists — never created for you). Global by default
  (honors `$CLAUDE_CONFIG_DIR` / `$CODEX_HOME`); `--project` writes to the current project's own
  config instead (Codex project installs print a trust-gating note). `--yes` for non-interactive
  use. Sibling hooks in the same config files are never touched; re-running replaces rawclaw's
  own entry, never duplicates it.
- **`rawclaw setup --eject`** — symmetric removal: the hook script, rawclaw's config entries, and
  any directories left truly empty; a config file that still holds anything else survives intact.
  Known limitation: Codex may keep a stale per-hook trust-state row in its own config (its format
  is undocumented and deliberately left alone).

## [0.3.0] — 2026-07-18

### Added

- **Durable retention — your history outlives Claude Code's 30-day cleanup.** When a source tool
  purges a transcript, RawClaw now keeps its indexed copy: still searchable and readable, labeled
  `source file gone — retained history` in results (keyword and semantic paths both). Controlled by
  `RAWCLAW_RETENTION` (`keep`, the default, or `mirror` for the old drop-on-purge behavior; mirror
  governs live scans only — retained history is removed by `rawclaw delete` alone). Sessions carry
  origin provenance (machine / source tool / source path), the groundwork for cross-machine search.
- **`rawclaw delete` reaches retained sessions** — the plan lists them as
  `(retained, source file already gone)` and a real delete tombstones them permanently.

### Fixed

- Search is read-only: retention bookkeeping happens at indexing, so repeated searches never write
  to (or slow down on) archived project databases.
- The one-time in-place database upgrade is kill-safe: interrupted at any instant, the next run
  completes the provenance backfill with no rebuild and no lost rows.
- Purged-source discovery now covers Codex session groups too, not only Claude projects.

## [0.2.0] — 2026-07-17

### Added

- **Multi-source recall via a pluggable `Source` port** — the index no longer knows a transcript's
  on-disk format. A small port (enumerate sessions → stream ordered messages) feeds the FTS5 index,
  and each agent CLI is an adapter behind it. Ships with two: **Claude Code** (`~/.claude/projects`)
  and **Codex** (`~/.codex/sessions/**/rollout-*.jsonl`). `--source claude|codex` scopes a search to
  one tool, or search across both at once; refs, `read`/`outline`, ranking, and the completeness
  envelope are identical regardless of source.
- **Codex-aware `--resume`** — prints the paste-ready `codex resume <id>` for a Codex hit (and the
  `claude --resume` form for a Claude hit).
- **Topic layer** — `rawclaw tag` records agent-supplied topic segments for a session (RawClaw itself
  calls no model — the agent feeds the tags in), and `rawclaw topics` surfaces them on demand as a
  delayed-disclosure fallback when an ambiguous keyword search buries the right segment. Topic tags
  never pollute the primary search ranking.
- **Drowning-steer** — when a query's terms are corpus-common (so relevance ranking is near-useless),
  the agent envelope says so and steers toward a narrower query instead of returning a confident wrong
  top hit.

### Changed

- **Agent-first is now the only surface.** The default output is the machine-readable agent envelope;
  the separate human-formatted view and the bundled skill were removed — `read` / `outline` are
  top-level verbs and `--help` is the documentation. `--json` remains for scripted use.

## [0.1.0] — 2026-06-20

Initial release.

### Added

- **Agent-first search by default** over Claude Code transcripts (`~/.claude/projects/*.jsonl`) —
  `rawclaw "query"` returns ranked refs wrapped in a never-silent completeness envelope (which scopes
  searched / empty / skipped / stale, and whether more matches exist). SQLite FTS5, a single static Go
  binary, no LLM, no API key.
- **Freshness + project-spread signals** — a recency hint surfaces a buried "what just happened" match,
  and a spread line flags when hits cross multiple projects.
- **All projects by default** (`--this-project` to narrow, `--list` to enumerate); boolean
  operators (`a NOT b`, `x OR y`), `"exact phrase"`, `term*` prefix, and date / role / path /
  `--min-messages` scoping.
- **Top-level `read` / `outline` verbs** — `read <session8>:<uuid8>` returns a bounded excerpt around a
  ref, `outline <session8>` its goal → resolution arc. Refs are source-stable (survive reindex and
  transcript appends), ambiguity is rejected git-style, reads are whole by default with an opt-in
  `--budget` ceiling, `--more` / `--around` / `--focus` operate on the same ref, and any trim emits the
  literal recovery command — never silent.
- **`--json` on every command** for scripted and agent use.
- **Session lifecycle** — `archive` and `delete` (filter-gated, dry-run-first, confirm-before-delete)
  with a tombstone so a delete survives reindex.
- **`--debug-search`** — an honest, LLM-free scoring explainer (the real bm25 / coverage / sort ranking).
- **LLM-free titles + noise filtering** — a session's title comes from its first substantive message,
  and low-signal messages are filtered from previews without ever dropping the session.
- **Optional semantic tier** — RRF keyword + vector search: a bring-your-own embedder reciprocal-rank-fused
  with the keyword hits, pure-Go cosine, off by default (keyword search needs no model or network).
- **Self-update** — `rawclaw upgrade` (alias `update`) downloads the latest release for your
  OS/arch, sha256-verifies it against the release's published checksums (a mismatch aborts
  without touching the installed binary), and atomically replaces the running binary with
  rollback on failure; `rawclaw upgrade --check` reports whether a newer release exists
  (exit 10) without downloading anything.
- **`version` command + `--version` flag** — print the build stamp (version / commit / date)
  injected via ldflags at release time.
- **Self-terminating `--timeout` watchdog** — every run is bounded by a hard wall-clock deadline
  so an agent never needs an external `timeout(1)`; default 30s, `--timeout 0` disables it,
  `RAWCLAW_TIMEOUT` overrides the default, and exceeding the deadline exits 124.
- **`delete --yes`** (alias `-y`) — skip the y/N confirmation prompt for non-interactive use.

### Fixed

- **`browse` no longer deadlocks** on the single-connection read pool: session rows are now
  drained and the cursor closed before per-session preview queries run, instead of issuing a
  second query while the first cursor still holds the only connection.
- **`read` / `outline` session resolution** resolves a bare session UUID to its parent
  session rather than false-tripping the ambiguity guard against that session's own subagent
  transcript (which shares the UUID prefix).

[0.1.0]: https://github.com/MoonCaves/rawclaw/releases/tag/v0.1.0
