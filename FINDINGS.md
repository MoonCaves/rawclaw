# Hostile ponytail findings

- `internal/cli/setup.go:addRawclawAntigravityHooks`: `yagni`/`shrink` — the helper always returns `nil`; delete the error result and the caller's unreachable error branch. RULING: restore exactly the current hook JSON shape and idempotent remove-then-add behavior; change only the impossible error plumbing. Net target: -9 lines including its white-box callers.
- `internal/cli/setup.go:antigravityHooksPath`: `shrink` candidate — this returns the same `hooks.json` path as `codexHooksPath`. REJECTED: the names document distinct target seams, and renaming or cross-target reuse would make the schema distinction less clear for a one-line saving.
- `internal/cli/cmd_prewarm.go:prewarmSourcePath`: `delete` candidate — direct SQL lookup resembles `store.SessionBackingFor`. REJECTED: the fallback to legacy `file_index` is intentionally broader than that session-row helper, so replacement is not behavior-preserving.

Net confirmed target: -9 lines.
