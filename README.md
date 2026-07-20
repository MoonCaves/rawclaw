# RawClaw

## Own your AI history.

**One sovereign, private, searchable corpus for every conversation, agent, and machine.**

Bring your conversations home from the cloud. Keep a copy you control. Search years of AI work in
seconds. Back it up and synchronize it through your own infrastructure.

No account. No internet. No API keys. No models. No daemon. No GPU. The core works locally, on your
machine.

- [Bring cloud conversations home](#bring-cloud-conversations-home)
- [Keep history after Claude deletes it](#keep-history-after-claude-deletes-it)
- [Preserve the full record](#preserve-the-full-record)
- [Search every supported AI agent together](#search-every-supported-ai-agent-together)
- [Sync privately through any Git remote](#sync-privately-through-any-git-remote)
- [Turn 76 searches into one](#turn-76-searches-into-one)
- [Know where every conversation came from](#know-where-every-conversation-came-from)
- [One owned corpus, countless uses](#one-owned-corpus-countless-uses)

## Bring cloud conversations home

Your Claude conversations live inside Claude's website and desktop apps. You can read them there,
but you do not control how they are stored or how long that access will remain available.

RawClaw imports conversations from Claude.ai, Claude Desktop, and Cowork. It combines them with local
sessions from Claude Code and Codex.

Now you own a local copy. You can search it, move it, back it up, process it, or use it with another
tool. You do not need to open Claude's website every time you want to access your own history.

**Stop renting access to your own conversations.**

## Keep history after Claude deletes it

Claude Code automatically deletes session files after about 30 days by default.

RawClaw lets you keep your own copy. When Claude Code removes the original file, your conversation
remains available and searchable.

You can also mirror Claude Code's behavior if that is what you prefer. The choice belongs to you.

**Claude's 30-day cleanup policy does not have to become your deletion policy.**

## Preserve the full record

Memory systems do more than store information. They decide what matters. They summarize, merge,
rank, and sometimes turn an agent's old statement into a supposed fact.

Those interpretations can be wrong.

RawClaw preserves the full historical record:

- Decisions and reversed decisions
- Questions, doubts, mistakes, and dead ends
- Pivots, discoveries, and tool calls
- The path from the first question to the final answer

Tags, summaries, and memory systems can still be added on top. RawClaw keeps those interpretations
separate from the original record, so they can be corrected without rewriting history.

**Memory tells you what it thinks mattered. Raw history lets you check.**

## Search every supported AI agent together

Claude Code saves history in one place. Codex saves it somewhere else. Claude's cloud products keep
conversations inside their own apps. Other AI agents use still more formats.

RawClaw brings supported sources into one fast search. Search across Claude.ai, Claude Desktop,
Cowork, Claude Code, Codex, and every machine you use. Search everything together or narrow the
results to one agent, source, project, date, or machine.

New AI tools connect through source adapters. If your agent is not supported yet, request it, move
it up the roadmap, or contribute an adapter.

**One search across your entire AI history — no matter which agent created it.**

## Sync privately through any Git remote

Most products put your data on their servers to make it available across devices.

RawClaw lets you synchronize through any private Git remote you choose: GitHub, GitLab, Gitea, a
self-hosted Git server, or a bare repository over SSH.

Your machines can push and pull automatically. Optional timers keep machines current even when you
are not actively searching. The core does not require an account. Optional synchronization uses the
Git provider and credentials you choose.

**Your machines. Your storage. Your private synchronization path.**

## Turn 76 searches into one

Recovering one past answer can take dozens of searches, file reads, and tool calls. In real recall
hunts, one past decision took 76 grep calls to recover.

RawClaw reduced it to one search:

```bash
rawclaw "where did we land on auth"
```

Every result includes the goal, the matching point, and the resolution. Agents receive stable
references and read only the relevant section first.

This is progressive disclosure for transcripts: search the full corpus, find the strongest match,
read a small excerpt, and expand around that exact point only when needed.

No giant transcript dumps. No wasted context window. No endless grep storm.

**Fewer tool calls. Fewer tokens. Faster answers.**

## Know where every conversation came from

Combining everything into one corpus should not erase its origin.

RawClaw keeps the source attached to each conversation: which AI tool created it, which machine it
came from, which project it belonged to, which original session it represents, and whether that
source is current, missing, remote, or stale.

You get one unified search without turning your history into an anonymous pile of text.

**Bring everything together. Keep every receipt attached.**

## One owned corpus, countless uses

Once your conversations are local, organized, searchable, and under your control, they become
infrastructure.

Use the corpus to:

- Search every past project
- Feed a memory provider or build a personal knowledge base
- Extract people and companies into a CRM
- Create project and decision timelines
- Audit agent actions and analyze tool usage
- Recover abandoned work or move between AI products
- Build datasets and new applications on top of your history

RawClaw is not meant to be the only tool allowed to read your data. It creates a common substrate
that other tools and agents can use.

**Own the corpus once. Build on it forever.**

## The core works fully locally

Keyword search runs locally in a single static Go binary with SQLite FTS5.

- No account
- No internet connection
- No API key
- No AI model
- No daemon
- No GPU
- No external runtime
- No hosted service

Then you can add capabilities when you want them: private Git synchronization, local Ollama or
hosted embeddings, semantic and keyword search together, remote live-session access, and more AI
agent source adapters.

**Local sovereignty is the foundation. Network features are optional upgrades.**

## Missing does not mean deleted

Files disappear for reasons that have nothing to do with user intent. Claude may clean them up. A
machine may be offline. A directory may move. A synchronization may be late.

RawClaw does not treat a missing file as permission to erase your history.

Deletion is a separate, explicit action. RawClaw shows what will be removed and asks for
confirmation. When you intentionally delete a conversation, that deletion follows the archive so
another machine does not restore it later.

**Missing stays recoverable. Deleted stays deleted. You control both.**

## Every search tells you how complete it was

Imagine searching three machines when one has not synchronized since yesterday. Most tools simply
return the available results. Neither you nor the agent knows that part of the history was missing.

RawClaw reports what it searched, what contained no matches, what it skipped, what was unavailable,
and what may be out of date. "I found no result" and "I could not search one of your machines" are
not the same answer.

**Never mistake a partial search for your full history.**

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
