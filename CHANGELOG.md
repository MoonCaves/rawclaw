# Changelog

All notable changes to RawClaw are documented in this file.

## [Unreleased]

### Fixed

- **The tagging queue only accepts sessions that produced a transcript.** Claude Code fires
  SessionEnd for ephemeral sessions too — opened and closed without a message ever landing —
  which flooded the queue with hundreds of ids per day that nothing could resolve. The
  SessionEnd hook now checks the `transcript_path` Claude Code hands it and queues only when
  that file exists on disk. Pure mechanics, no judgment: a real session has a transcript, an
  ephemeral one doesn't. Re-run `rawclaw setup` to refresh the installed hook script.

## [0.5.0] — 2026-07-19

### Added

- **Automatic topic tagging: a SessionEnd queue behind `rawclaw setup`.** `setup` now wires a
  second Claude Code hook: on SessionEnd the finished session is queued for topic tagging
  (`rawclaw tag-queue add`, called by the installed `tagqueue.sh`), and the SessionStart banner
  lists the queue so the next session's agent tags what's pending (`tag-prep` → `tag-write`; a
  successful `tag-write` dequeues its session). Tagging happens on its own, agent to agent —
  no human closeout discipline needed, and rawclaw itself still calls no model. New verb:
  `rawclaw tag-queue` (bare = list pending as 8-char ids, silent when empty; `add` is the hook's
  entry point, idempotent; `remove` skips a session that won't resolve or isn't worth tagging;
  `--json` on the listing). The queue is a plain-text file in the state dir, one session id per
  line. Codex keeps SessionStart only — its SessionEnd event surface is unverified, so nothing
  is registered there that might never fire. `setup --eject` removes both scripts and both
  entries, same symmetry as before.

- **Positional session delete: `rawclaw delete <session8>` (or the full id).** Deletes exactly
  that one session — same dry-run-first plan, same y/N prompt (`--yes` works), same tombstone and
  receipts as a filter delete. Sub-8-char prefixes must match a full id exactly; a prefix
  matching two sessions deletes neither (refused, exit 1); an unknown id is a clear error, exit
  1; a session that lives only in another machine's archive is refused with the origin-machine
  pointer. Retained sessions (source file already purged) are deletable by id too. This makes the
  README's `rawclaw delete --yes <session8>` example true.
- **`--all` in the folder-scoped shapes.** Bare browse and `--stats` now honor `--all`: browse
  merges the most recent sessions across every project (newest first, per-row project label,
  `--json` scope-tagged); the corpus stats aggregate no longer needs a history-bearing `--dir`.
  The "No transcript history for --dir …" hint now names both escapes: `--list`, or `--all` for
  every project. `--this-project` wins over `--all`, as it already did for stats.
- **Delete provenance note.** After a real delete rawclaw states what was removed: the session's
  transcript file plus rawclaw's copy (index + archive) for live sessions; for retained-only
  deletes, rawclaw's copy alone — Claude Code / Codex transcript files untouched. The same
  sentence lives in `delete --help` and the README.

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
- **Live peek: `rawclaw live <machine> [session-prefix]`.** "What is the agent on that machine
  doing right now" — one direct SSH hop that lists the machine's recent sessions (newest first,
  Claude + Codex, ages computed on the far end so clock skew never lies) or renders one
  in-progress session's current transcript, messages written seconds ago included. The archive
  is never touched. The machine name is the ssh destination by default (an `~/.ssh/config` alias
  just works); the archive config's optional `"ssh"` map overrides it per machine. Degrades with
  distinct, actionable errors: unresolvable machine name, unreachable ssh, no rawclaw on the
  remote's non-interactive PATH (with the install one-liner), or a remote rawclaw too old to
  serve. `--json` for agents (raw message text, tool calls and all); `--tail N` to widen the
  transcript window. The human render follows the same display posture as `read`/`outline` —
  tool calls stripped by default, `--include-tools` to render them. No summarization.

- **Cross-machine search: `rawclaw archive pull` + transparent foreign scopes.**
  Pull refreshes the local clone (a deleted/corrupt clone is simply re-cloned; `--throttle`
  honors a 5-minute stamp-file window for background callers, an explicit pull always runs).
  From then on a plain `rawclaw "query"` covers every other machine's pushed sessions like
  local hits: each foreign machine dir is enumerated as extra search scopes (own dir excluded,
  so local sessions never double-count), indexed through the existing ingest paths into
  archive-namespaced cache dbs, and provenance-stamped with the owning machine's id from the
  dir's committed manifest. Hits are labeled `<machine>/<project>`; `read`/`outline` resolve
  foreign refs like any local ref; `--resume` on a foreign session degrades to a clear
  run-it-there hint naming the machine. A machine silent for over a day is reported through the
  existing stale-scope posture ("N stale — results may be incomplete") with its results still
  served.

- **`rawclaw archive status`** — an offline freshness report: remote, local clone (and whether it
  is usable), this machine's last successful push and pull, and one line per machine dir with the
  time new content last arrived. Wording is honest about what the clone can know: an idle machine
  with nothing new to push and a dead one look identical from here, so no machine is ever labeled
  stale/dead — the only warning raised is for what THIS machine knows first-hand, its own sync
  stamps going overdue (no successful push/pull sync in over a day; a verified-nothing-to-push
  run counts as a successful sync). Search's possibly-out-of-date posture for silent machines is
  unchanged.
- **Deletes propagate to the archive — own sessions, explicit deletes only.** A session removed
  with `rawclaw delete` (file gone + tombstone recorded) is also removed from the archive on this
  machine's next push, so an explicit delete is never resurrected by a later pull. On the OTHER
  machines, the next pull + search reindex drops the session from their archive-replica dbs too:
  for archive scopes the synced clone is the source of truth and absence is authoritative, so an
  owner's delete can never be resurrected by a replica's index. Absence alone never deletes
  LOCALLY: upstream purges and `RAWCLAW_RETENTION=mirror` prunes keep their archive copies — the
  archive is the durable mirror. Foreign machine dirs are read-only from every box: a delete
  filter reaching another machine's archived sessions is refused with a pointer at the origin
  machine, and foreign replica dbs can never enter the tombstone path.

### Changed

- **The delete confirmation says which copy dies — and `--yes` alone no longer removes
  original files.** A delete reaching live sessions (original transcript files still on disk)
  prompts with "This removes rawclaw's copy (index and archive) and the original session
  transcript files. Confirm with your user." and the receipt matches; retained-only deletes
  keep the rawclaw's-copy-only prompt and receipt. Non-interactively, `--yes` alone now covers
  retained-only deletes only — a delete that removes original files errors (exit 2) unless
  `--yes --files` authorizes it. `--dry-run` is unchanged.
- **Archive transfers are stall-bounded, not wall-clock-capped.** `archive init/push/pull`
  (and the background autosync) now run with the wall-clock watchdog off by default and catch
  hangs the way rsync/curl do — stall detection: `http.lowSpeedLimit/lowSpeedTime` for HTTP(S)
  remotes and ssh keepalives (`ServerAliveInterval`/`CountMax`, layered onto any existing
  `GIT_SSH_COMMAND`) for SSH remotes. A hung transfer dies in ~30–60s; a slow-but-moving
  multi-GB first push runs to completion. An explicit `--timeout` / `RAWCLAW_TIMEOUT` still
  applies a hard cap.
- **One seam for user-facing time.** All user-facing timestamps now render through a single
  policy package: agent-parsed surfaces (search results, the outline header, the live list and
  stream, `--json` fields) are marked UTC — RFC3339 `Z`, or `HH:MM:SSZ` for bare clocks — and
  human surfaces (`archive status`) are local time with the zone abbreviation. The outline
  header previously rendered unmarked local time and the live stream an unmarked UTC clock;
  both now carry explicit markers.
- **Empty query coaching.** `rawclaw ""` (or an all-whitespace query) prints a distinct
  "Empty query…" line pointing at bare browse and `--all`, instead of the no-matches coaching.
- **Delete abort exit code.** Answering `n` (or EOF) at the delete prompt still prints
  "Aborted; nothing deleted." but now exits 1 so scripts can distinguish an abort from a
  completed delete; `--dry-run` stays exit 0.

### Fixed

- **Timer install/eject hardening.** A launchd reinstall whose registration fails now restores
  the previous working timer instead of leaving none; eject checks by CONTENT that the
  plist/units were written by rawclaw before disabling or deleting a same-named file; on Linux,
  install records where the systemd units landed so a later `XDG_CONFIG_HOME` change can't
  strand them at eject time.
- **Folder guard: a directory merely holding loose `.jsonl` files is no longer treated as a
  transcripts dir by implicit discovery.** A bare `rawclaw` run from a folder like `/tmp` used
  to index that folder itself into the cache. Location-based discovery (the known Claude Code /
  Codex dirs and recorded cwds) is untouched; indexing an arbitrary jsonl-bearing folder now
  requires the explicit `--dir` opt-in.
- **Kill-safety across the whole push sequence.** A push killed at any point — mid-copy, before
  the commit, before the push, during the rebase, even mid-`git clone` — now leaves a clone the
  next push fully recovers on its own: a clone interrupted mid-creation is detected
  (completed-clone sentinel) and rebuilt; leftover mid-rebase/mid-merge state is aborted, and a
  clone whose HEAD stays unresolvable after recovery is rebuilt outright; a stale
  `.git/index.lock` left by git itself dying (power loss, process-group kill) is aged out after
  15 minutes while fresh locks stay honored. The remote never sees partial state beyond git's
  own commit atomicity.
- **Rebuild only with proof, recover under a dying watchdog.** The wipe-and-reclone recovery
  path now refuses to destroy anything it cannot prove disposable: before any rebuild the clone
  is checked for commits the remote lacks (a stranded, unpushed sync) and the rebuild is refused
  with instructions instead of eating them; a sentinel-less but structurally complete clone
  (created before the sentinel existed) is adopted in place — the marker is stamped — rather
  than wiped, where "complete" demands a finished checkout (resolvable branch HEAD AND a clean
  status — a clone killed mid-checkout has the former but not the latter, and adopting it would
  let the next push commit a tree missing every other machine's dir); `archive init` refuses to
  wipe a leftover clone that still holds unpushed commits (config lost after a sync committed
  but before it pushed); and the mid-rebase/mid-merge abort runs on its own short-deadline
  context, so the watchdog cancelling a verb mid-recovery can no longer make a recoverable
  wedge look like corruption and trigger a destructive re-clone.
- **Replica reconciliation only from a verified, quiescent clone.** Because absence from the
  clone is now authoritative for archive scopes, search-time replica ingest is gated twice: a
  clone without the completed-clone sentinel (torn, or not yet adopted) is not enumerated at
  all, and while a sibling sync holds the machine-wide lock (a pull may be mid-rebase, with the
  worktree half-rewritten) enumeration serves the previously built scope dbs without
  reconciling — foreign sessions can no longer transiently vanish from search mid-sync.

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
