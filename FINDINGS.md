# Issue #50 findings

- `runResume` eagerly builds Codex, Antigravity, and Goose scopes before it
  knows whether the requested prefix is already indexed.
- `agentproto.LocateConsolidatedSession` exposes only one ID and cannot carry
  the source/CWD/project fields needed by resume output. The existing store
  APIs provide the same bounded prefix lookup plus `SessionBackingFor`, so a
  CLI-local fast path is sufficient and avoids an `agentproto` API change.
- The fast path will query the consolidated store for up to three top-level
  rows, preserving the existing ambiguity behavior. It will use the recorded
  source, CWD, and project to build resume candidates, and fall back to the
  current container-scope discovery only when Claude catalog and consolidated
  lookups both miss.
