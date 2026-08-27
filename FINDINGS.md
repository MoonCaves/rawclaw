# Issue #50 findings

- `runResume` resolves Claude through the durable catalog, but eagerly builds
  Codex, Antigravity, and Goose scopes before it knows whether the requested
  prefix is already present in the consolidated store.
- The consolidated store has the bounded prefix rows and source/backing data
  needed by resume output. A CLI-local fast path is sufficient; no
  `agentproto` interface change is required.
- The fast path must preserve global ambiguity, deduplicate one session found
  by both Claude catalog and consolidated lookup, ignore foreign archive rows,
  and fall back to discovery when the store is unavailable, stale, or cannot
  safely map a row to a local source.
- Baseline: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run
  'TestRunResume|TestResume_'` passed, wall `27.60s`.
