# Furiosa nil-scope attack

## Verdict

FIX APPLIED. Default nil-scope tag-write no longer waits on the consolidated
fence, so retained-history authoring returns promptly while derived publication
is busy.

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
The focused regression now asserts the requested liveness behavior. Note that
this changes the prior fence-safety contract for direct consolidated writes;
rebuild snapshot/rename semantics should be re-audited before release.
