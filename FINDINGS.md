Issue #41 is caused by removing refresh-cache eviction while retaining refresh DBs for the prewarm/closeout cache. Eviction must therefore not run from PrepareFreshContainer, whose contract is to preserve fresh entries, and must allow sessions to outlive the old 24-hour window. The smallest shared fix is a 30-day inactivity TTL at RefreshDBPath, which is used whenever a refresh cache entry is acquired; protect the acquired path during that pass and remove SQLite sidecars with the database so reused/active entries survive while abandoned entries are eventually reclaimed.

# Issue 59 findings

- Existing signal: `internal/agentproto.outline` already reads an over-fetched
  tail with `store.BookendMessages`, applies the shared
  `view.FilterDisplayableWith` filter, and renders the resulting `view.ViewMsg`
  values. `ViewMsg.Role` therefore already identifies the last relevant event.
- Existing related signal: `view.SessionLastActivity` finds the latest
  displayable message, but returns only preview text for search/browse and is
  not suitable for outline's resolution state.
- Added signal: `OutlineResult.UnansweredAssistant` is set when the last
  displayable outline event has role `assistant`; text rendering appends a
  plain note for that case. No new transcript parsing or resolution heuristic
  is added, and the normal outline output remains unchanged.
