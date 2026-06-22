# RawClaw

**Your AI already knew. It just couldn't find it.**

Every decision your agent ever made is sitting in `~/.claude/projects`. The only way back to it is `grep` — which matches the string you typed, not the decision you meant: your past self phrased it differently, the answer got folded into a compaction summary, and grep walks right past it. So your agent re-derives what it already worked out.

Here's the cost — real recall hunts from one developer's history:

| recalling one past decision | grep calls |
|---|--:|
| a deployment/auth decision | **76** |
| how PR-review state got tracked | 68 |
| where the messaging stack landed | 51 |
| a git-worktree-per-desk workflow | 49 |
| model-routing between two services | 26 |
| handling externalized tool-results | 13 |

Dozens of greps to recover one fact — and plenty still came up empty. RawClaw replaces the "grep storm" with one search that hands you the whole arc: **goal → match → resolution**.

**76 → 1.**

```bash
rawclaw "where did we land on auth"
```

---

## What it does

Claude Code quietly saves every conversation you have with it as JSONL transcripts under `~/.claude/projects`. RawClaw indexes them with SQLite **FTS5** and searches them the way you actually type.

- **Goal → match → resolution.** Every hit returns the session's opening (what it set out to do), the matched message in context, and the closing (what was decided) — so one result usually answers the question without opening a file.
- **All your projects by default.** One query spans every Claude Code folder you've worked in (`--this-project` to narrow).
- **Natural phrasing works.** Multi-word queries OR their terms and rank by how many match — you don't need the exact wording.
- **Reads the full structure, shows the signal.** Subagent threads, tool calls, compaction summaries, and thinking blocks are all indexed, but search defaults to clean human conversation (`--include-tools` / `--include-subagents` to widen).
- **Built for agents too.** `rawclaw "query"` returns ranked refs with a never-silent completeness envelope; `read <ref>` returns a *bounded excerpt* instead of a whole transcript; `--json` on every command.

## Who it's for

- **Anyone who lives in Claude Code** and wants to find a past decision in one search instead of a dozen greps.
- **AI / agent builders** who need programmatic recall of prior sessions — JSON in, JSON out.
- **CI / automation / scripts** — non-interactive, composes with `jq`/`fzf`, real exit codes.
- **Resource-constrained / self-hosted / Raspberry Pi.** A single static binary + SQLite FTS5: low RAM, no GPU, no runtime to install. Keyword search needs no network and no API key.

---

## Install

```bash
go install github.com/MoonCaves/rawclaw/cmd/rawclaw@latest
```

Or grab a release binary — no runtime dependencies, a single static binary — from the [releases page](https://github.com/MoonCaves/rawclaw/releases) and put it on your `PATH`.

RawClaw is a single static binary: pure Go, no cgo, cross-compiles to Linux / macOS / ARM. The keyword core has **no runtime dependencies, no LLM, and needs no API keys**.

---

## Keeping it current

RawClaw updates itself — the binary you install by hand is the last one you install by hand:

```bash
rawclaw upgrade            # update in place to the latest release
rawclaw upgrade --check    # report whether a newer release exists (exit 10 if so)
```

`upgrade` downloads the release for your OS/arch, **sha256-verifies it against the release's published checksums** (a mismatch aborts without touching your installed binary), then **atomically replaces** the running binary with rollback on failure. `--check` only reports — it downloads nothing and exits `10` when an update is available (so scripts can gate on it). An unstamped `dev` build won't replace itself without `--force`.

---

## Usage

```bash
rawclaw "where did we set up auth"          # search (default): ranked hits + read-refs (all projects)
rawclaw --this-project "auth"               # narrow to the current project
rawclaw read <session8>:<uuid8>             # bounded excerpt around a ref (--more/--around/--budget/--focus)
rawclaw outline <session8>                  # the session's goal → resolution arc
rawclaw                                      # browse: your most recent sessions
rawclaw --resume <session8>                 # paste-ready `claude --resume` for that session
rawclaw --stats                             # corpus overview (this project; --all for everything)
rawclaw "query" --json                      # machine-readable output for scripts/agents
rawclaw "query" --since 2026-01-01 --before 2026-02-01   # date-scoped
rawclaw --list                              # list searchable projects
rawclaw delete --yes <session8>             # delete a session non-interactively (no y/N prompt)
rawclaw version                             # print the version + build stamp
rawclaw --timeout 2m "query"                # raise the self-terminating deadline (0 disables it)
```

**Query tips:** a single distinctive word is sharpest · `"exact phrase"` for adjacency · `term*` for a prefix · `a NOT b` to exclude · `--include-path` / `--exclude-path` to scope by project · `--json` works on every shape.

**Built to never hang.** Every run is bounded by a self-terminating watchdog so an agent never needs an external `timeout(1)`: `--timeout 2m` raises the deadline, `--timeout 0` disables it, and `RAWCLAW_TIMEOUT` (a Go duration like `45s` or `2m`) overrides the default. The default is `30s`; exceeding the deadline exits `124` (the `timeout(1)` convention). `delete --yes` (alias `-y`) skips the confirmation prompt for non-interactive use.

### For agents

The surface is agent-first by default — an LLM recalls its own history without pasting whole transcripts:

```bash
rawclaw "query"                        # ranked hits, each with a copyable read-ref + completeness envelope
rawclaw read <session8>:<uuid8>        # a bounded excerpt around a ref (--more/--around/--budget/--focus)
rawclaw outline <session8>             # the goal → resolution arc, to pick where to read next
```

`--json` throughout — so an agent searches, picks a ref, and reads a bounded slice instead of its whole context on one transcript. Search is the default verb; `read` and `outline` are top-level verbs.

---

## How it works

A small SQLite **FTS5** index over your transcripts, refreshed incrementally on each run — only changed sessions re-index, so it stays fast even on a large history. Within a session, messages are ordered by insertion id (not timestamp, which can be non-monotonic), and shaped into the goal → match → resolution view.

### Optional semantic search — bring your own embedder

RawClaw is **keyword-only out of the box** — no LLM, no model, no API key; that is the whole product. Semantic search is an opt-in upgrade: point one environment variable at any embeddings endpoint, and RawClaw reciprocal-rank-fuses vector hits with the keyword hits, so a paraphrase whose words never literally appear still finds the right conversation.

```bash
export RAWCLAW_EMBED_ENDPOINT=http://localhost:11434/api/embeddings   # any local Ollama
export RAWCLAW_EMBED_MODEL=nomic-embed-text
rawclaw --reindex-vectors          # embed your history once (incremental, resumable)
rawclaw "monthly billing for the paid tier"   # now lexical + semantic, fused
```

- **Local, fully private:** Ollama — no API key, nothing leaves your machine.
- **Hosted:** OpenAI, Voyage, or any OpenAI-compatible gateway (`RAWCLAW_EMBED_WIRE=openai` + `RAWCLAW_EMBED_KEY`).

Vectors live as BLOBs in the same on-disk index; cosine scoring and fusion run in pure Go — no LLM, no extra service, no numpy, no GPU. Unset the env (or pass `--no-vector`) and you're back to byte-identical keyword search.

---

## Roadmap

Where RawClaw is headed — new source formats (Codex/Gemini/opencode), semantic tuning, and the trade-offs we deliberately *won't* make — is in **[ROADMAP.md](ROADMAP.md)**.

---

## License

MIT
