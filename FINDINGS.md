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

# Issue #50 findings

- `runResume` resolves Claude's durable catalog and then eagerly discovers
  Codex, Antigravity, and Goose scopes even when a full session ID is already
  known. The existing consolidated store has the bounded session row and
  `SessionBackingFor` metadata needed to answer that case without discovery.
- `paths.SessionHit` currently drops `CatalogEntry.Source`, and `runResume`
  hardcodes every path hit as Claude. A catalog entry for Codex, Antigravity,
  or Goose therefore emits the wrong resume command. Preserve the source on
  the hit, defaulting legacy stem resolution to Claude.
- Fast-store hits must only short-circuit for an exact full ID. Prefix lookups
  still run the existing scope discovery so cross-runtime ambiguity remains
  visible. Merge fast and fallback candidates by source plus full ID so the
  same candidate is not reported twice while distinct IDs remain ambiguous.

Both changes are localized to `internal/durable/durable.go`; existing durable path, rendering, and rebuild-contract tests are the guard.
## Ponytail finding #14 audit

Verdict: PARTIAL ACCEPT.

- `internal/retention/retention.go:94,186-192`: `isMember` has one caller. Replace it with the native comma-ok lookup at that caller. A map read from a nil map remains safe and reports absent, so behavior is unchanged. Removing the wrapper and its comment while adding one local lookup is a net -7 lines.
- `internal/index/index.go:925,1082,1275-1280`: REJECT in this fence. The wrapper has callers in `internal/index/rebuild.go:158` and `internal/index/containers.go:341`, in addition to the two callers in `index.go`. Removing it would require touching files outside the allowed fence; changing only the two local callers leaves the wrapper and does not shrink the code. Nil-map behavior is already safe.

No test changes are needed: existing retention/index behavior tests cover the call paths, and the replacement is a direct map lookup with identical nil-map semantics.

Net change after implementation: -7 lines in `internal/retention/retention.go`; no index change.

# Ponytail findings: setup hook templates

## Ruling

`internal/cli/setup.go` contains two near-identical Claude/Codex SessionStart
catalog lifecycle blocks and two copies of the same `rawclawBanner`. Replace
the shared catalog lifecycle with one POSIX fragment carrying a source
placeholder, and compose both templates from that fragment. Compose both
banner outputs from the existing `rawclawBanner`; retain Claude's plain
heredoc and Codex's Python JSON envelope. Do not alter Antigravity, binary
resolution, validation, hard-link claim, fail-soft ingest, Stop prewarm, or
any user-visible strings.

Expected production result: approximately -70 lines in `setup.go`, zero new
dependencies, and byte-equivalent generated hook behavior.
