Issue #41 is caused by removing refresh-cache eviction while retaining refresh DBs for the prewarm/closeout cache. Eviction must therefore not run from PrepareFreshContainer, whose contract is to preserve fresh entries, and must allow sessions to outlive the old 24-hour window. The smallest shared fix is a 30-day inactivity TTL at RefreshDBPath, which is used whenever a refresh cache entry is acquired; protect the acquired path during that pass and remove SQLite sidecars with the database so reused/active entries survive while abandoned entries are eventually reclaimed.

## Issue #57 findings

- `internal/cli/cli.go` wires `--include-path` as a regex over project working
  directories, and passes it into `agentproto.SearchOpts.IncludePath`.
- `internal/query.PathPredicate` applies the include/exclude pattern to the
  supplied `cwd` string. `internal/scopes.FilterByPath` applies the same
  predicate to `scopes.CWD(sc)`, never to a session ID.
- The consolidated search path resolves the pattern against stored `(project,
  cwd)` pairs in `agentproto.resolveStoreProjects`; a nonmatching path can
  therefore produce an empty result without identifying that the filter value
  was likely a session ID.
- `rawclaw read <session8>:<uuid8>`, `rawclaw outline <session8>`, and
  `rawclaw --resume <session8>` resolve session IDs/prefixes. There is no
  search or browse flag that scopes a result set to one session ID.
- The minimal fix is an additive hint on an empty path scope when
  `--include-path` looks like a bare UUID/session ID. It should point to
  `rawclaw outline <id>` / `rawclaw --resume <id>` and preserve the existing
  path-filter and JSON-output semantics. `read` is not valid recovery because
  it requires a `<session8>:<uuid8>` message reference, not a session ID.

# Ponytail rulings: store stats lane

## Finding #7 — ACCEPT

`internal/store/stats.go:GetCorpusStats` has six independent aggregate queries.
Replace them with exactly two conditional-aggregation queries: one over
`sessions`, one over `messages`. Preserve zero-row counts, `MIN`/`MAX` NULL
handling, the existing zero-stats-on-scan-error behavior, and the public
`CorpusStats` result. Reuse `database/sql`; add no abstraction or dependency.

## Finding #19 — ACCEPT

`first10` only receives documented ASCII ISO timestamps. Replace the rune-slice
allocation with `s[:min(len(s), 10)]`; preserve empty and short-string behavior.

## Scope fence

Only `internal/store/stats.go` and its directly corresponding store stats test
are in scope. No GitHub, graphify, or mailbox state is modified.

# Accepted Ponytail Finding 13

Ruling: replace the duplicate hand-rolled `strings.IndexByte` fragment stripping
in `internal/cli/cmd_ingest.go:backingPath` and
`internal/index/containers.go:backingFilePath` with exact-equivalent
`strings.Cut` calls. Keep the two local functions; add no shared helper or
unrelated cleanup.

Estimated net reduction: 4 lines.

# Accepted ponytail findings

- Finding 10 — ACCEPT: `internal/durable/durable.go:sanitize` can use `strings.Map` with the same portable-rune allowlist and `'-'` replacement; preserve dot-only prefixing and path containment.
- Finding 18 — ACCEPT: `internal/durable/durable.go:recordType` can use `slices.Contains(parse.IndexableTypes, role)`; preserve the `"system"` fallback.

Both changes are localized to `internal/durable/durable.go`; existing durable path, rendering, and rebuild-contract tests are the guard.

# Independent verification: store stats candidate

The stale candidate is already present on current `origin/main` in
`internal/store/stats.go`: `GetCorpusStats` uses one conditional aggregate over
`sessions` and one over `messages`, with no additional implementation needed.
The rewrite is behaviorally identical for SQLite: `COUNT(CASE WHEN ... THEN
1 END)` matches the prior filtered counts, `COUNT(*)` preserves empty-store
zeros, and conditional `MIN`/`MAX` preserve NULL results for empty timestamps.
The existing zero-value/nil-error behavior for scan failures is unchanged, as
is the open-error wrapping. The result is measurably simpler at the SQL-call
level (2 queries instead of 6) and reduces production code by 3 lines in the
candidate diff. Verdict: ACCEPT, already landed; no new production edit.
