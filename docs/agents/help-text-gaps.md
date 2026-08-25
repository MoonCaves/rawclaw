# User-facing documentation coverage audit

Scope: `README.md` and the CLI help declarations under `internal/cli/`.

The highest-priority gaps are in help text because agents see command help at the point of use. The current `ingest` help already explains its automatic background trigger and safe repetition, so the corresponding README gap is lower priority.

## 1. `rawclaw ingest [session]` is missing from the README command surface

Evidence: `README.md:95-109` lists the public commands but omits `ingest`. The command exists at `internal/cli/cmd_ingest.go:39-46` and supports one session or all discoverable active sessions, automatic SessionStart triggering, watermark skips, and concurrent serialization.

Missing guidance: users and agents cannot discover the manual recovery/maintenance verb from the README, or learn that session-start hooks normally invoke it for them.

Copy-ready README wording:

```markdown
rawclaw ingest [session8]                    # refresh one session, or all discoverable active sessions
```

Add after the usage block:

```markdown
`rawclaw ingest [session8]` refreshes one session (full id or prefix) into the consolidated search store; without an argument it refreshes all discoverable active sessions. `rawclaw setup` starts the targeted form in the background at SessionStart, so normal reads and searches usually query an already-indexed store. Repeated or concurrent ingests are safe: unchanged sessions are skipped and the consolidated-store write is serialized with bounded retry.
```

## 2. Setup documentation omits the durable session catalog

Evidence: `README.md:34-45` says that setup installs a session-start note, but does not say that the hook records a durable catalog entry or starts background ingest. The installed hook behavior is documented in `internal/cli/setup.go:25-36`; the catalog location and entry purpose are implemented in `internal/paths/paths.go:66-89`.

Missing guidance: users may assume setup only changes the agent-visible banner. They are not told that session birth is recorded for later fast lookup, or that the catalog survives process restarts and is distinct from the rebuildable search indexes.

Copy-ready README wording:

```markdown
At SessionStart, the installed hook also records the session id, transcript path, working directory, and source in RawClaw's durable session catalog, then starts a detached targeted ingest. The catalog is a lightweight lookup aid; the SQLite indexes remain rebuildable caches. If a catalog entry is missing or unusable, RawClaw falls back to its normal transcript discovery path.
```

## 3. Root help does not explain the catalog-first lookup or its fallback

Evidence: the root help at `internal/cli/cli.go:188-199` describes search, read, outline, retention, and freshness verification, but says nothing about catalog-backed session lookup. The catalog-first exact lookup and fallback are implemented at `internal/paths/paths.go:294-303` and `internal/paths/paths.go:311-315`.

Missing guidance: an agent does not know why a session id can resolve quickly after setup, nor what happens when the catalog is absent or stale.

Copy-ready root help line:

```text
Session ids use the durable session catalog for fast exact/prefix lookup when available, then fall back to transcript discovery if the catalog misses; a catalog miss is not itself a failure.
```

## 4. `read` help does not state the freshness behavior of a returned excerpt

Evidence: `internal/cli/cli.go:579-583` documents refs, budgets, and window controls, but not that the result is indexed session history rather than guaranteed current transcript content. Human `read` output does print the standing freshness note at `internal/agentproto/agentproto.go:2319-2320`, while the root help only gives a general warning at `internal/cli/cli.go:192-194`.

Missing guidance: an agent reading `read --help` alone can mistake a successful excerpt for a current snapshot, especially while a session is still being written.

Copy-ready `read` help wording:

```text
The excerpt is indexed session history, not a guarantee of the live transcript's current tail; verify current state before acting. Human output adds a freshness note when applicable.
```

## 5. `outline` help does not carry the same freshness warning as `read`

Evidence: `internal/cli/cli.go:647-650` only describes the goal-to-resolution arc. Unlike `read`, the current human outline renderer ends after the resolution/subagent sections (`internal/agentproto/agentproto.go:2323-2345`) and does not emit the standing freshness footer.

Missing guidance: the two read-oriented verbs have different documented and rendered freshness posture, but `outline --help` does not disclose the limitation at all. If the intended merged behavior is to add an O(1) staleness note to outline, the help text should still explain what that note means.

Copy-ready `outline` help wording:

```text
The arc is read from the indexed session record. It may lag a transcript that is still being written; verify current state before acting. A human-rendered result may add a staleness note when the index is behind the source.
```

## 6. Bare browse documentation omits the consolidated-store path and freshness trade-off

Evidence: `README.md:100-101` only says that bare `rawclaw` browses recent sessions and that `--include-path` filters projects. The root flag help at `internal/cli/cli.go:228,243-244` describes scope, but not the store used. Cross-project browse now answers from one consolidated read connection and falls back to per-project databases only when that store is unavailable (`internal/cli/cli.go:1244-1248`, `internal/cli/cli.go:1260-1280`). `--reindex` bypasses that consolidated path for the scoped browse implementation.

Missing guidance: users cannot tell that bare cross-project browse is a fast consolidated-store query, or how to force a source reindex when they need newly written transcript content immediately.

Copy-ready README wording:

```markdown
Bare browse and cross-project browse normally read the consolidated store, which gives one newest-first view across projects and runtimes. If that store is unavailable, RawClaw falls back to the per-project indexes. Use `--reindex` when you need browse to refresh the source indexes before answering; otherwise a just-written session may not appear until background ingest catches up.
```

Copy-ready root help line:

```text
Bare browse normally answers from the consolidated store; use --reindex to bypass it and refresh the selected source indexes before browsing.
```

## Scope note: the named O(1) freshness implementation is not in this worktree HEAD

The brief names O(1) freshness checks and staleness notes on read verbs as recently merged behavior. The current `internal/cli/cli.go` read and outline handlers end after rendering and autosync (`internal/cli/cli.go:613-617` and `internal/cli/cli.go:665-670`); they contain no `CheckSessionFreshness` call, and `rg` finds no such symbol in the current tree. The historical change exists on a separate branch/commit, but is not an ancestor of this worktree's `HEAD`.

Therefore this report treats the freshness behavior as a documentation contract gap and does not recommend documenting details that the current tree cannot verify. Before shipping the proposed freshness wording, merge or otherwise verify the intended implementation on the documentation base branch.
