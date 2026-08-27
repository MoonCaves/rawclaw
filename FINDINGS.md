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
- Same-source/full-ID matches from the Claude catalog and consolidated store
  must be deduplicated; distinct IDs and distinct sources remain ambiguous.
- Product probe `go run ./cmd/rawclaw --resume 87783881` timed out at the
  built-in 30-second limit on current main (`30.96s`) and on this branch
  (`32.07s`); both ran amid concurrent consolidated writers and produced no
  useful stdout, so product-path speedup is inconclusive. The focused race
  gate is the isolated behavior evidence.
