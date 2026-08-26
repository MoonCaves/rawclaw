# External nil-scope referee receipt

- Worktree: `/Users/jay-m4/code/rawclaw-furiosa-external-nilscope-referee`
- Branch@HEAD before receipt commit: `worker/furiosa-external-nilscope-referee-20260827@8e52d8d6cea1d6f0c1831b3d28ad3b0d59155ea4`
- Base: `2789d6f7d6ccc8fd5ce90b8a03eb3ce5364c03ce`
- Exact substantive commit: `8e52d8d6cea1d6f0c1831b3d28ad3b0d59155ea4`
- Stable patch-id: `3c24d3ff98ae50d513f776e8a6d4cdea83e8cba4`
- Files changed: `internal/cli/hostile_default_scope_test.go`,
  `FURIOSA_EXTERNAL_NILSCOPE_REFEREE.md`, and this receipt.
- Production implementation: unchanged. No mailbox or cursor touched.

## Graphify

- `graphify reflect --if-stale`: 0 useful, 0 dead-end, 0 corrected lessons;
  wrote the local reflection under `graphify-out/`.
- Local checkout had no `graphify-out/graph.json`; used the current-tree graph
  at `/Users/jay-m4/code/rawclaw/graphify-out/graph.json` for orientation.
- `graphify query 'tagWrite scope catalog publish' --graph ... --budget 4000`:
  identified `runTagWriteCmd`, `SyncConsolidatedFrom`, `AcquireConsolidatedFence`,
  and `LocateSessionGuarded`.
- `graphify explain 'runTagWriteCmd' --graph ...`: showed calls to
  `SyncConsolidatedFrom` and `AcquireConsolidatedFence`.
- `graphify explain 'spawnTagPublish' --graph ...`: no node found.
- `graphify path 'runTagWriteCmd' 'SyncConsolidatedFrom' --graph ...
  --context call --budget 4000`: one-hop call path.
- `graphify update .` was not run because the isolated checkout has no local
  graph and the task fence permits only the implementation, corresponding
  tests, report, and scratchpad files.

## Mutation evidence

The base test expected completion within 300 ms while `consolidated.lock` was
held. It blocked, reproducing the reported observation. The test was changed
to require blocking, then completion after unlock. A hostile mutation deleting
only the consolidated-fence block made the corrected test fail in 0.11 s:
`tag-write returned before fence release (err=<nil>)`. This distinguishes the
safe implementation from a superficial fence bypass.

## Tests and wall times

- `gofmt -w internal/cli/hostile_default_scope_test.go`: passed.
- `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run
  '^TestTagWriteDefaultScopeConsolidatedOnlyWaitsForFence$'`: passed,
  wall `11.286s`.
- Hostile unfenced mutation of the same focused race test: expected failure,
  wall `4.738s`.
- `CGO_ENABLED=0 go test -race -count=1 ./...`: passed, wall `2:49.55`.

## Final disposition

**REJECT / NO BUG.** The observed wait is the required serialization invariant
for direct writes to source-less retained history in `consolidated.db`.
Removing it reopens snapshot-then-rename lost-write risk. No production fix is
warranted; the committed change only corrects and strengthens the regression
test.

## Push receipt

- `git push -u origin worker/furiosa-external-nilscope-referee-20260827`:
  succeeded; remote branch created and tracking configured.
- Receipt update commit: `a0496fa3ec174cf98604764236e02250b7774d45`.
