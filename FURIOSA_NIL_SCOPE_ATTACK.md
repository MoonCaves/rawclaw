# Furiosa nil-scope attack

## Verdict

NO BUG. No production correction is warranted.

## Scope

The generated `rawclaw tag-write <session-prefix>` shape passes `scope == nil`.
For retained history with no surviving source or catalog row,
`LocateSessionGuarded` resolves `consolidated.db`. `runTagWriteCmd` then acquires
the consolidated fence before opening that database for write.

## Evidence

- Graphify orientation: `graphify reflect --if-stale` (0 stored lessons in this
  checkout); the checkout had no graph, so the current-tree graph at
  `/Users/jay-m4/code/rawclaw/graphify-out/graph.json` was used.
- `graphify explain "runTagPrepCmdWithSources"`, `graphify explain
  "refreshTagSession"`, and `graphify explain "runTagPrepWithTopics"` linked the
  tag-refresh source path, while `graphify path
  "runTagPrepCmdWithSources" "applyResolvedTags"` found no path.
- `graphify query "tagrefresh scope publish" --budget 4000` identified
  `runTagWriteCmd`, `LocateSessionGuarded`, `AcquireConsolidatedFence`, and
  `SyncConsolidatedFrom` as the relevant nodes.
- The hostile mutation seeded a consolidated-only retained session, held
  `consolidated.lock`, and invoked `runTagWriteCmd(..., nil, nil, ...)`.
  It remained blocked for 300 ms and completed after lock release.
- Removing the fence would make the test return promptly but would reintroduce
  the proven snapshot-then-rename lost-write race documented by commit
  `73443049bf0565417e931ddda36d022ebfe843c6`.

The corrected regression test preserves the safety contract: direct
consolidated writes wait for the rebuild fence and complete after release.
