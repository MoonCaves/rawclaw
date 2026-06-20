# RawClaw 🐾

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
- **Built for agents too.** `--json` on every shape, clean exit codes, and an `agent` protocol that returns *budgeted excerpts* instead of whole transcripts.

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

Or grab a release binary — a single static file with no dependencies — from the [releases page](https://github.com/MoonCaves/rawclaw/releases) and put it on your `PATH`.

RawClaw is a single static binary: pure Go, no cgo, cross-compiles to Linux / macOS / ARM. The keyword core has **zero runtime dependencies, no LLM, and needs no API keys**.

---

## Usage

```bash
rawclaw "where did we set up auth"         # discovery: goal → match → resolution (all projects)
rawclaw --this-project "auth"              # narrow to the current project
rawclaw                                     # browse: your most recent sessions
rawclaw --scroll <session8> --around <#>   # keep reading around a hit
rawclaw --resume <session8>                # paste-ready `claude --resume` for that session
rawclaw --stats                             # corpus overview (this project; --all for everything)
rawclaw "query" --json                     # machine-readable output for scripts/agents
rawclaw "query" --since 2026-01-01 --before 2026-02-01   # date-scoped
rawclaw --list                              # list searchable projects
```

**Query tips:** a single distinctive word is sharpest · `"exact phrase"` for adjacency · `term*` for a prefix · `a NOT b` to exclude · `--include-path` / `--exclude-path` to scope by project · `--json` works on every shape.

### Agent protocol

For an LLM agent recalling its own history without pasting whole transcripts:

```bash
rawclaw agent search "query"           # ranked, copyable read-refs
rawclaw agent read <session8>:<msg>    # a budgeted excerpt (--budget N / --no-budget)
rawclaw agent outline <session8>       # the goal → resolution arc
```

`--json` throughout — so an agent searches, picks a ref, and reads a bounded slice instead of blowing its context on a whole transcript.

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
