---
name: rawclaw-search
description: Search the raw Claude Code transcript record (all projects or one) with RawClaw. Use when recalling a prior decision, finding where or when something was discussed, recovering a past session, or auditing what was built and why. Triggers on "where did we", "find the session where", "what did we decide about", "recall a prior conversation", "search past sessions", "search transcripts", "look up an old session", "what was said about", "find prior art in past chats".
---

# rawclaw-search

RawClaw is full-text search over your Claude Code session transcripts (the JSONL files under `~/.claude/projects`). Reach for it when the answer is "we talked about this before" — a past decision, the session where something was built, the moment a path or value was chosen.

## Raw record, not current truth

Transcripts are an immutable log. A match shows what was **said** in that session at that time — not what is true now. A value may have changed, a plan may have been dropped, a "let's do X" may have been reversed two messages later. **Treat every hit as evidence, then verify** against the current code, config, or a fresher source before acting on it. Use rawclaw to find *where and when* something was discussed; confirm the *what's-true-now* separately.

## The three shapes

### 1. Discovery (default)
Org-wide keyword search. Each hit comes back as **goal → match → resolution**: what the session set out to do, the matched message inside a small window (anchor marked `▶`, with `#id`s), and what got decided.

```bash
rawclaw "<query>"                       # all projects
rawclaw "<query>" --this-project        # narrow to the current working dir
rawclaw "<query>" --json                # machine-readable — agents use this
rawclaw "<query>" --since 2026-05-01    # date window (YYYY-MM-DD)
rawclaw "<query>" --before 2026-06-01
rawclaw "<query>" --min-messages 10     # skip tiny/throwaway sessions
rawclaw "<query>" --include-path <glob> # restrict by project path
rawclaw "<query>" --exclude-path <glob>
```

### 2. Scroll (keep reading around a hit)
Widen around one hit without a new search. Use the 8-char session id and a `#id` from the discovery output.

```bash
rawclaw --scroll <session8> --around <#id>
rawclaw --scroll <session8> --around <#id> --json
```

### 3. Browse (no query)
Run rawclaw with no query to list recent sessions — for "what was I working on".

```bash
rawclaw                                 # browse recent sessions
rawclaw --list                          # session counts per project
rawclaw --stats                         # corpus stats
```

## Agent protocol (preferred for agents)

`rawclaw agent <verb>` is the budgeted, agent-facing surface: it returns bounded excerpts instead of whole transcripts, so you can search → read without blowing your context. Always pass `--json`.

```bash
# 1. search — find candidate sessions and message refs
rawclaw agent search "<query>" --json

# 2. read — pull a bounded excerpt around one ref (session8:msg_id from the search hits)
rawclaw agent read <session8>:<msg_id> --json
rawclaw agent read <session8>:<msg_id> --focus "<term>" --json   # center on a term
rawclaw agent read <session8>:<msg_id> --budget 4000 --json      # cap excerpt size
rawclaw agent read <session8>:<msg_id> --no-budget --json        # full excerpt, no cap

# 3. outline — table-of-contents view of one session, to pick where to read next
rawclaw agent outline <session8> --json
```

Pattern: **`agent search` to locate, then `agent read <ref>` bounded.** Read one or two refs, not the whole session — the budget exists so you stay cheap. Reach for `--no-budget` only when a capped excerpt clearly cut off the thing you need.

## Querying well

- **One distinctive term is sharpest.** A unique identifier (`traefik-vhost`, an app name, an error string) beats a full sentence. FTS5 ORs the terms and ranks by how many match, so natural phrasing works but dilutes precision.
- **Quote for adjacency:** `"embed endpoint"` requires those words together.
- **Prefix with `*`:** `autoscal*` matches `autoscaling`, `autoscaler`.
- **Exclude with `NOT`:** `bravo NOT traefik`.
- Common stop-words are dropped automatically.

## When "no matches" is surprising

Default scope is top-level human conversation — tool calls and delegated subagent threads are excluded. If you expected a hit and got nothing, widen:

```bash
rawclaw "<query>" --include-tools         # search tool-call messages
rawclaw "<query>" --include-subagents      # search delegated subagent threads
```

## Resume a past session

Get a paste-ready command to reopen a session in the right directory:

```bash
rawclaw --resume <session8> --json
# → {"session_id": "...", "cwd": "...", "command": "cd <dir> && claude --resume ..."}
```

## Semantic search (optional)

Keyword is the default and needs no network or key. If an embedding endpoint is configured via `RAWCLAW_EMBED_ENDPOINT` (plus `RAWCLAW_EMBED_MODEL`, `RAWCLAW_EMBED_WIRE`, `RAWCLAW_EMBED_KEY` as needed), results are fused with vector similarity. Build the index first:

```bash
export RAWCLAW_EMBED_ENDPOINT=...
export RAWCLAW_EMBED_MODEL=...
rawclaw --reindex-vectors                  # add --this-project to narrow
```

Without an embedder set, every command above works unchanged in keyword-only mode.
