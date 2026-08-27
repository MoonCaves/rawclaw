# Issue 45 scoped browse audit

- `runBrowse` called `allScope` for every non-reindex scoped browse before the
  consolidated query could run.
- `runBrowseScoped` already uses `view.BrowseScoped`, which delegates to
  `store.BrowseScopedSessions` for source/project/date/limit filtering.
- The fix is a direct consolidated attempt for normal scoped reads, with the
  existing discovery path retained for unavailable or empty stores and for
  explicit `--reindex`.
