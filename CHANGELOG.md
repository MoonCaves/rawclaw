# Changelog

All notable changes to RawClaw are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-06-20

Initial release.

### Added

- **Keyword search** over Claude Code transcripts (`~/.claude/projects/*.jsonl`) with a
  goal → match → resolution view — SQLite FTS5, a single static Go binary, no LLM, no API key.
- **All projects by default** (`--this-project` to narrow, `--list` to enumerate); boolean
  operators (`a NOT b`, `x OR y`), `"exact phrase"`, `term*` prefix, and date / role / path /
  `--min-messages` scoping.
- **Agent protocol** — `--json` on every shape plus `agent search|read|outline` with
  source-stable `<session8>:<uuid8>` refs (survive reindex and transcript appends), git-style
  ambiguity rejection, whole-by-default reads with an opt-in `--budget` ceiling, `--more` /
  `--around` expand-in-place, never-silent trims, and incompleteness-as-data.
- **Session lifecycle** — `archive` and `delete` (filter-gated, dry-run-first, confirm-before-delete)
  with a tombstone so a delete survives reindex.
- **`--debug-search`** — an honest, LLM-free scoring explainer (the real bm25 / coverage / sort ranking).
- **LLM-free titles + noise filtering** — a session's title comes from its first substantive message,
  and low-signal messages are filtered from previews without ever dropping the session.
- **Optional semantic tier** — bring-your-own-embedder, reciprocal-rank-fused with the keyword hits,
  pure-Go cosine, off by default (keyword search needs no model or network).

### Fixed

- **`browse` no longer deadlocks** on the single-connection read pool: session rows are now
  drained and the cursor closed before per-session preview queries run, instead of issuing a
  second query while the first cursor still holds the only connection.
- **`--scroll` / `agent` session resolution** now resolves a bare session UUID to its parent
  session rather than false-tripping the ambiguity guard against that session's own subagent
  transcript (which shares the UUID prefix).

[0.1.0]: https://github.com/MoonCaves/rawclaw/releases/tag/v0.1.0
