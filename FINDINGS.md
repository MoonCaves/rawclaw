# Existing audit records

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

# Ponytail finding #15: `cacheHome` versus `os.UserCacheDir`

## Verdict: REJECT

Replacing `internal/store/cacheHome` with `os.UserCacheDir` is not behaviorally
compatible and is not a net-negative change.

## Evidence

- Current source (`internal/store/store.go:269-286`) resolves the store root to
  `$HOME/.cache`, with a relative `.cache` fallback when `os.UserHomeDir` cannot
  resolve a home directory. `CacheDir` therefore remains
  `$HOME/.cache/session-search`.
- Go 1.26.3's `os.UserCacheDir` documentation and implementation return
  `$HOME/Library/Caches` on Darwin, regardless of `XDG_CACHE_HOME`. On this
  Darwin/arm64 host, the proposed replacement would move RawClaw's state to
  `~/Library/Caches/session-search`, orphaning the existing
  `~/.cache/session-search` corpus and sidecars.
- On non-Darwin Unix, `os.UserCacheDir` honors `XDG_CACHE_HOME` and errors for a
  relative value. That would change RawClaw's current deliberate behavior,
  including test isolation assumptions and the fallback behavior when the
  environment is incomplete.
- The standard-library targeted tests passed with
  `HOME=/tmp/rawclaw-pony15-home XDG_CACHE_HOME=/tmp/rawclaw-pony15-xdg`:
  `go test "$(go env GOROOT)/src/os" -run
  'TestUserCacheDir(XDGConfigDirEnvVar)?$' -count=1`.
  The Go source explicitly skips the XDG test on Darwin and implements the
  Darwin `HOME/Library/Caches` branch.
- Existing repository tests and comments consistently refer to and isolate
  `$HOME/.cache/session-search`; there is no migration or dual-location lookup
  that could preserve existing indexes, tombstones, machine identity, refresh
  databases, or consolidated state.

## Recommendation

Keep `cacheHome` unchanged. A migration would need an explicit product decision,
compatibility plan, and broad changes outside this finding's file fence; it is
not a one-line standard-library cleanup.

## Scope

No production or test code was changed. There is no `internal/store/store_test.go`
in this checkout, so no new test is warranted for a rejected, incompatible
replacement.
