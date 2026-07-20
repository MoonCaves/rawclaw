# RawClaw

## Own your AI history.

**One sovereign, portable, searchable corpus for every supported conversation, agent, and machine.**

Stop renting access to your own past. Bring your conversations home from cloud products and local
agents, preserve them beyond vendor purges, and use them through infrastructure you control.

### Bring everything

- Import conversations from Claude.ai, Claude Desktop, and Cowork.
- Collect local sessions from Claude Code, Codex, and new agents through source adapters.
- Search across every project and every machine from one place.
- Keep the original source, machine, project, and session provenance attached.

### Keep everything

- Keep sessions after Claude Code's default 30-day cleanup removes the original files.
- Preserve decisions, reversed decisions, mistakes, pivots, tool calls, and dead ends.
- Treat a missing file as missing — not as permission to delete your history.
- Propagate explicit deletions so another machine cannot silently restore them.

### Run it anywhere

- One static Go binary with a local SQLite FTS5 index.
- No account. No internet. No API key. No model.
- No daemon. No GPU. No external runtime. No hosted service.
- Linux, macOS, and ARM — including small self-hosted machines and Raspberry Pis.

### Use it for anything

- Search years of work in seconds.
- Feed another memory provider or build a personal knowledge base.
- Create CRM records, project timelines, audits, datasets, and new applications.
- Move between AI products without leaving your history behind.

**Own the corpus once. Build on it forever.**

- [Your data lives with you](#1-your-data-lives-with-you)
- [Take your history with you](#2-take-your-history-with-you)
- [Choose where it lives and how it moves](#3-choose-where-it-lives-and-how-it-moves)
- [One corpus can power countless systems](#4-one-corpus-can-power-countless-systems)
- [Every new agent is an adapter](#5-every-new-agent-is-an-adapter)
- [Your history should outlive its source](#6-your-history-should-outlive-its-source)
- [The record is permanent; interpretations are replaceable](#7-the-record-is-permanent-interpretations-are-replaceable)
- [Agents should retrieve progressively and admit what they missed](#8-agents-should-retrieve-progressively-and-admit-what-they-missed)

## Eight opinions behind the architecture

### 1. Your data lives with you

Many "local" tools still require an account, API key, model, cloud service, or background server.
RawClaw's core search works fully on your machine.

- Your history and search index stay local by default.
- Keyword search needs no network connection or third-party service.
- There is no required account, API key, model, daemon, GPU, or external runtime.
- The index is ordinary SQLite, not a proprietary database server.
- The binary is static, portable, and built without cgo.

**Local sovereignty is the architecture, not a privacy setting.**

### 2. Take your history with you

Your AI history should not remain trapped inside a website, desktop app, hidden folder, or machine
you may stop using.

- Bring conversations home from Claude.ai, Claude Desktop, and Cowork.
- Combine cloud conversations with Claude Code and Codex sessions.
- Keep a local copy you can search, move, back up, process, and use with other tools.
- Change AI products without starting your history from zero.
- Keep access even when the original interface, account, or product changes.

**Your conversations become data you own — not pages you borrow.**

### 3. Choose where it lives and how it moves

Sovereignty does not mean every user must make the same choice. It means the owner chooses.

- Keep everything on one machine with no network access.
- Synchronize through GitHub, GitLab, Gitea, a self-hosted Git server, or bare SSH.
- Use automatic background synchronization or an optional timer.
- Stay local with Ollama embeddings.
- Opt into OpenAI, Voyage, or another compatible endpoint when you want hosted embeddings.
- Disable the archive or vector layer without weakening local keyword search.

**Your machines. Your storage. Your models. Your choice.**

### 4. One corpus can power countless systems

Search is the first application. The owned corpus is the durable asset.

- Search every past project and recover abandoned work.
- Feed memory providers and personal knowledge bases.
- Extract people, companies, and relationships into a CRM.
- Build project histories, decision timelines, and agent audits.
- Analyze tool usage or create datasets.
- Build applications we have not imagined yet.

RawClaw is not meant to be the only tool allowed to read your data. It creates a common substrate
that other tools and agents can use.

**One sovereign corpus. Countless downstream uses. No lock-in.**

### 5. Every new agent is an adapter

AI tools store history in different files and databases. That should not require a new search engine
for every tool.

- Claude Code and Codex proved the shared Source adapter port.
- Claude cloud imports feed the same storage and retrieval pipeline.
- Hermes, OpenClaw, NanoClaw, and other agents can connect through adapters.
- Search, provenance, progressive reads, and synchronization stay source-independent.
- New embedding providers connect through a separate optional seam.
- Embeddings live as BLOBs in SQLite and fuse with the keyword results.

Need another source? [Submit an adapter request](https://github.com/MoonCaves/rawclaw/issues/new) or
open a pull request. The seam is already there.

**Sovereign by default. Upgradeable by design.**

### 6. Your history should outlive its source

Claude Code automatically removes session files after about 30 days by default. Files also vanish
when machines go offline, directories move, or products change.

- RawClaw keeps sessions searchable after Claude Code removes the original files.
- `RAWCLAW_RETENTION=mirror` remains available if you prefer to follow upstream cleanup.
- A missing source is reported as missing instead of silently treated as a deletion.
- Deletion is a separate, explicit action with a clear confirmation.
- Intentional deletions propagate through the archive instead of being resurrected later.

**Claude's cleanup policy does not have to become your deletion policy.**

### 7. The record is permanent; interpretations are replaceable

Memory systems summarize, merge, rank, and decide what mattered. Those interpretations can be
useful. They can also be wrong.

- Preserve decisions and the evidence around them.
- Preserve reversals, doubts, mistakes, pivots, and rejected approaches too.
- Keep the original transcript separate from tags, summaries, and memory layers.
- Correct or replace an interpretation without rewriting the underlying record.
- Treat the archive as the durable source and the search index as rebuildable machinery.

**Memory tells you what it thinks mattered. Raw history lets you check.**

### 8. Agents should retrieve progressively and admit what they missed

Dumping a whole transcript into an agent's context window is expensive and noisy. Returning a
partial search without saying what was unavailable is worse.

One real recall hunt took 76 grep calls. RawClaw reduced it to one:

```bash
rawclaw "where did we land on auth"
```

- Search returns ranked matches with the goal, matching point, and resolution.
- Stable references let an agent read a small excerpt and expand around the same point.
- Every trim is visible and includes the command needed to retrieve more.
- Results retain the source tool, machine, project, session, and availability state.
- The completeness envelope reports what was searched, empty, skipped, stale, or unavailable.

**Fewer tool calls. Fewer tokens. Faster answers — without hiding the gaps.**

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
rawclaw delete --yes --files <session8>     # delete a session non-interactively (--files: originals still on disk)
rawclaw version                             # print the version + build stamp
rawclaw --timeout 2m "query"                # raise the self-terminating deadline (0 disables it)
```

**Query tips:** a single distinctive word is sharpest · `"exact phrase"` for adjacency · `term*` for a prefix · `a NOT b` to exclude · `--include-path` / `--exclude-path` to scope by project · `--json` works on every shape.

**Built to never hang.** Every run is bounded by a self-terminating watchdog so an agent never needs an external `timeout(1)`: `--timeout 2m` raises the deadline, `--timeout 0` disables it, and `RAWCLAW_TIMEOUT` (a Go duration like `45s` or `2m`) overrides the default. The default is `30s`; exceeding the deadline exits `124` (the `timeout(1)` convention). The syncing archive verbs (`archive init/push/pull`) are the exception: their transfers are bounded by *stall* detection instead — a hung transfer dies in under a minute, a slow-but-moving push runs as long as it keeps moving (an explicit `--timeout` still applies a hard cap). `delete --yes` (alias `-y`) skips the confirmation prompt for non-interactive use; when the delete would remove original transcript files still on disk, `--yes` alone refuses — add `--files` to authorize that too.

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

### Topic tags — where the conversation pivoted

Every session gets tagged at its topic-change points with concept keywords, so an agent can always find the moment a conversation pivoted — `rawclaw topics "<concept>"` drops it right there.

### Optional transcript archive — a git remote as durable, multi-machine storage

Raw transcripts die upstream (Claude Code's ~30-day purge) and live on only one machine until they don't. The archive gives them one durable home: any private git remote (GitHub, Gitea, a bare repo over SSH — no rawclaw-specific server).

```bash
rawclaw archive init git@github.com:you/rawclaw-transcripts.git   # clone + register this machine (--name to pick the dir name)
rawclaw archive push                                              # upload this machine's Claude + Codex transcripts
rawclaw archive pull                                              # fetch the other machines' transcripts
rawclaw archive status                                            # remote, clone, last push/pull, per-machine freshness
```

The remote argument accepts a shorthand: `you/rawclaw-transcripts` (or a bare `you`, which assumes a repo named `rawclaw-transcripts`) expands to `git@github.com:you/rawclaw-transcripts.git`; `gitlab.com/you/repo` and `sr.ht/~you/repo` work too. A full URL is used as-is.

Each machine gets a top-level directory in the repo (`<machine>/<source>/...`), so pushes from different machines never conflict on content; concurrent pushes are resolved with a bounded pull-rebase-and-retry. A second push moves only changed files. The local clone lives in the state dir and is a rebuildable cache — deleting it just forces a re-clone (`archive pull` rebuilds everything); a clone left broken by a kill at any point (mid-clone, mid-rebase, even a stale git lock after a power loss) is detected and rebuilt automatically on the next push or pull. Rebuilds are evidence-gated: if the broken clone still holds commits that never reached the remote, rawclaw refuses to wipe it and tells you how to recover them instead.

**Deletes propagate — but only your own, and only explicit ones.** `rawclaw delete <session8>` (or a `--before`/`--project`/`--max-messages` filter) removes the session's transcript file when it is still on disk plus rawclaw's copy (index + archive); deleting a *retained* session — one whose transcript the source tool already purged — removes only rawclaw's copy, leaving Claude Code / Codex transcript files untouched. The confirmation says which of the two you're doing; non-interactively, `--yes` alone covers retained-only deletes and a delete that removes original files requires `--yes --files`. A session removed with `rawclaw delete` is also removed from the archive on this machine's next push, so an explicit delete is never resurrected by a later pull — and once other machines pull, their search indexes drop it too (for archive scopes, gone from the archive means gone). Nothing else ever deletes from the archive: transcripts purged upstream (or pruned locally by `RAWCLAW_RETENTION=mirror`) keep their archive copies — the archive is the durable mirror. Other machines' directories are read-only from here; a delete filter that matches another machine's sessions is refused with a pointer at the origin machine.

**Cross-machine search is automatic.** After a pull, a plain `rawclaw "query"` covers every other machine's pushed sessions like local hits — labeled `<machine>/<project>`, provenance-stamped with the owning machine's id, readable and outlinable by ref. Your own machine's directory in the clone is ignored (the live local tree is fresher), so nothing ever double-counts. `--resume` on a foreign session tells you which machine it lives on and the command to run there. A machine that hasn't pushed in over a day shows up in search output as possibly stale — its results are still served.

**Staying current is automatic.** Once the archive is configured, ordinary usage keeps it fresh: after a search/read/outline prints its results, rawclaw fires a *detached* background sync (push + throttled pull) — your command never waits on the network, a hung push can never hold a tool call open, and a burst of searches costs one sync per five-minute window. Receipts land in `archive/autosync.log` under the state dir; set `RAWCLAW_ARCHIVE_AUTOSYNC=off` to disable the background sync, or `RAWCLAW_ARCHIVE=off` to switch the whole archive feature off (searches go local-only, nothing syncs) without touching its config. For machines that mostly run agents rather than searches, `rawclaw archive enable-timer` installs an hourly `archive push` under your user account (a launchd agent on macOS, a systemd user timer on Linux) — never wired silently, and `--eject` removes exactly what was added. Two rawclaw processes syncing at once (timer + search, two shells) is safe: a machine-wide lock serializes them, and the loser skips cleanly.

> **The remote MUST be a private repository.** Transcripts contain whatever you and your agents pasted into sessions — API keys, tokens, private code. RawClaw prints this warning at `archive init` and cannot verify a host's visibility settings for you.

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
