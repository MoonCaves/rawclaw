# Findings: scoped browse AND regression

- `runBrowse` now sends `thisScope` directly to `runBrowseConsolidated` before the existing `runBrowseScoped` path filter runs.
- `runBrowseConsolidated` converts that unfiltered scope into consolidated project labels, so `--this-project --include-path` can return the project even when the path predicate rejects it.
- The shared correction is to apply `scopes.FilterByPath` inside `runBrowseConsolidated` before deriving project labels. An empty filtered scope must fall through so `runBrowseScoped` emits the established no-match message.
- This preserves issue #45: matching scoped reads still use the consolidated store and avoid machine-wide discovery.
