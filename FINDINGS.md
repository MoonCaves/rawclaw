# Issue #45 findings

- `internal/cli/cli.go:1246` calls `allScope` before `runBrowseScoped` can apply
  `scopes.FilterByPath`; `runSearch` likewise gives its fallback an unfiltered
  `allScope` at `internal/cli/cli.go:1560`.
- `internal/scopes/container.go:21` discovers and groups every container, then
  calls `index.EnsureIndexedContainers` for every CWD before any path predicate
  can run. The same helper is shared by Codex, Antigravity, and Goose.
- The smallest root-cause seam is to pass one optional path predicate into
  `scopes.All` and `containerScopes`, filter `byCWD` before sorting/indexing,
  and preserve `FilterByPath` for the already-built browse fallback.
- Current-main measurement: the scoped Codex search command timed out after
  RawClaw's 30s watchdog (42.25s wall), and stderr showed folds for unrelated
  Codex CWD databases. This is direct evidence that post-enumeration filtering
  is a placebo.

Ponytail review: no separate abstraction or dependency is needed; reuse the
existing `query.PathPredicate` and `scopes.CWD` logic. `golang-modernize`
produced no shrinking change relevant to this path.
