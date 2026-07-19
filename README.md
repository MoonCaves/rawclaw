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

## Claude Code deletes your history after 30 days

Claude Code removes conversation files older than ~30 days (`cleanupPeriodDays`). RawClaw keeps its own copy: sessions stay searchable and readable after the original file is gone. Results mark them `source file gone — retained history`, and `rawclaw delete` removes them for good.

Want RawClaw to track your Claude Code retention setting instead? Set `RAWCLAW_RETENTION=mirror` and sessions drop when their source file does.

## Setup: let your agents discover it

Installing the binary gives *you* the tool; `rawclaw setup` tells your **agents** about it. One
command wires a short session-start note into Claude Code (and Codex, when installed) so every
new session knows rawclaw exists and how to search with it — no hand-editing config files.

```bash
rawclaw setup        # shows the plan, asks y/N
rawclaw setup --yes  # non-interactive
```

Global by default (rawclaw searches every project); `--project` narrows the note to the current
project only. Everything else already hooked into your configs is left untouched, and re-running
never duplicates. `rawclaw setup --eject` removes exactly what setup installed — nothing more.

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

### Optional transcript archive — a git remote as durable, multi-machine storage

Raw transcripts die upstream (Claude Code's ~30-day purge) and live on only one machine until they don't. The archive gives them one durable home: any private git remote (GitHub, Gitea, a bare repo over SSH — no rawclaw-specific server).

```bash
rawclaw archive init git@github.com:you/my-transcripts.git   # clone + register this machine (--name to pick the dir name)
rawclaw archive push                                          # upload this machine's Claude + Codex transcripts
rawclaw archive pull                                          # fetch the other machines' transcripts
rawclaw archive status                                        # remote, clone, last push/pull, per-machine freshness
```

Each machine gets a top-level directory in the repo (`<machine>/<source>/...`), so pushes from different machines never conflict on content; concurrent pushes are resolved with a bounded pull-rebase-and-retry. A second push moves only changed files. The local clone lives in the state dir and is a rebuildable cache — deleting it just forces a re-clone (`archive pull` rebuilds everything); a clone left broken by a kill at any point (mid-clone, mid-rebase, even a stale git lock after a power loss) is detected and rebuilt automatically on the next push or pull.

**Deletes propagate — but only your own, and only explicit ones.** A session removed with `rawclaw delete` is also removed from the archive on this machine's next push, so an explicit delete is never resurrected by a later pull. Nothing else ever deletes from the archive: transcripts purged upstream (or pruned locally by `RAWCLAW_RETENTION=mirror`) keep their archive copies — the archive is the durable mirror. Other machines' directories are read-only from here; a delete filter that matches another machine's sessions is refused with a pointer at the origin machine.

**Cross-machine search is automatic.** After a pull, a plain `rawclaw "query"` covers every other machine's pushed sessions like local hits — labeled `<machine>/<project>`, provenance-stamped with the owning machine's id, readable and outlinable by ref. Your own machine's directory in the clone is ignored (the live local tree is fresher), so nothing ever double-counts. `--resume` on a foreign session tells you which machine it lives on and the command to run there. A machine that hasn't pushed in over a day shows up in search output as possibly stale — its results are still served.

**Staying current is automatic.** Once the archive is configured, ordinary usage keeps it fresh: after a search/read/outline prints its results, rawclaw fires a *detached* background sync (push + throttled pull) — your command never waits on the network, a hung push can never hold a tool call open, and a burst of searches costs one sync per five-minute window. Receipts land in `archive/autosync.log` under the state dir; set `RAWCLAW_ARCHIVE_AUTOSYNC=off` to disable the background sync, or `RAWCLAW_ARCHIVE=off` to switch the whole archive feature off (searches go local-only, nothing syncs) without touching its config. For machines that mostly run agents rather than searches, `rawclaw archive enable-timer` installs an hourly `archive push` under your user account (a launchd agent on macOS, a systemd user timer on Linux) — never wired silently, and `--eject` removes exactly what was added. Two rawclaw processes syncing at once (timer + search, two shells) is safe: a machine-wide lock serializes them, and the loser skips cleanly.

> **The remote MUST be a private repository.** Transcripts contain whatever you and your agents pasted into sessions — API keys, tokens, private code. RawClaw prints this warning at `archive init` and cannot verify a host's visibility settings for you.

GitHub rejects files over 100 MB; transcripts that large need a remote without that limit (a self-hosted Gitea, or a bare repo over SSH).

### Live peek — what is that machine's agent doing *right now*?

The archive is durable, not instant. For seconds-fresh visibility, `rawclaw live` skips it entirely and reads the other machine's in-progress session over one SSH hop:

```bash
rawclaw live box-a               # list box-a's recent sessions, newest first
rawclaw live box-a 3f2a91c0      # render that session's current transcript (messages written seconds ago included)
```

The machine name doubles as the ssh destination — an `~/.ssh/config` Host alias just works — or map it explicitly in the archive config (`"ssh": {"box-a": "user@10.0.0.5"}`). The far end needs sshd plus a rawclaw on its non-interactive PATH; without that, `live` fails with a pointed error and your freshness is whatever the last `archive push` uploaded. `--json` gives agents the structured form (raw message text, tool calls and all); `--tail N` widens the transcript window. The rendered transcript follows the same display posture as `read` and `outline` — tool calls stripped unless `--include-tools` asks for them. No summarizing, no interpretation.

---

## Roadmap

Where RawClaw is headed — new source formats (Codex/Gemini/opencode), semantic tuning, and the trade-offs we deliberately *won't* make — is in **[ROADMAP.md](ROADMAP.md)**.

---

## License

MIT
