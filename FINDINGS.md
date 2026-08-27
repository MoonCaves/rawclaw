# Issue #24 findings

- `tag-prep` already provides the bounded, incremental untagged-window dump and
  prints the manual `rawclaw tag-prep <session>` recovery instruction when a
  window remains; reuse its testable core rather than duplicating window logic.
- `tag-write` already owns authoritative writes and best-effort detached
  publication; closeout must invoke its existing command/core and preserve that
  contract.
- `bg_ingest.go` provides detached self-invocation, logging, and a per-session
  spawn-token pattern. Closeout needs a session/revision marker using that same
  pattern, without changing ingest behavior.
- The current tree has no configured headless tagger integration. The closeout
  seam must therefore be optional and fail soft to the existing manual recovery
  command when no configured tagger is present.
- Scope is limited to `internal/cli/`: add the command, child orchestration,
  focused tests, and root-command wiring. No core storage, parser, daemon, or
  dependency redesign is warranted.
