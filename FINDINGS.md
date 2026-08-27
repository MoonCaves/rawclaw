Issue #41 is caused by removing refresh-cache eviction while retaining refresh DBs for the prewarm/closeout cache. Eviction must therefore not run from PrepareFreshContainer, whose contract is to preserve fresh entries, and must allow sessions to outlive the old 24-hour window. The smallest shared fix is a 30-day inactivity TTL at RefreshDBPath, which is used whenever a refresh cache entry is acquired; protect the acquired path during that pass and remove SQLite sidecars with the database so reused/active entries survive while abandoned entries are eventually reclaimed.

## Issue #58 transplant-first gate

Outcome: REJECTED_AFTER_EXECUTABLE_TRIAL.

The requested session listing already exists. Bare browse (`rawclaw --this-project`, or simply
`rawclaw` from the target project) routes through `runBrowse`/`runBrowseScoped` in
`internal/cli/cli.go`, and `internal/view.Browse`/`BrowseScoped` reuse the typed
`store.BrowseSessions` query. `internal/store/sessions.go` orders by `last_ts DESC, id` and applies
the requested limit, so newest sessions are first. The CLI help explicitly says “Bare browse ...”
and README documents `rawclaw # browse: your most recent sessions`.

Executable trial against a populated project (`--dir .../rawclaw --this-project --json --limit 5`)
returned five sessions in descending `last_ts`; the first row was the newest. `--list` is a
different command: it lists searchable projects and counts, not sessions, and
`--list --this-project` still printed the project table. The empty result from this worker worktree
is honest because that directory has no transcript history.

Prior art/pattern checked: `git log -n`/`ls -t` style newest-first bounded listing. No new query
path is warranted; the existing `store.BrowseSessions` helper is the reusable implementation.
No source files edited.
