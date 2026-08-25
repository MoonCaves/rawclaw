# Ponytail Review Findings: internal/cli/cli.go & internal/cli/timeout.go

Fenced scope: `internal/cli/cli.go` and `internal/cli/timeout.go` ONLY.
Root command wiring refactor pass.

## Findings

- `internal/cli/cli.go:L1965-1967`: stdlib: 3-line baseName wrapper around filepath.Base with single caller. Inline `filepath.Base(sc.TDir)` directly and delete `baseName`.
- `internal/cli/cli.go:L160-175`, `L1940-1945`: stdlib: manual empty-string defaults in `BuildInfo.versionString` and `orQ`. Replace with `cmp.Or`.
- `internal/cli/cli.go:L402-409`: shrink: `isReindexVectorsInvocation` creates a second `pflag.FlagSet` and parses argv twice. Bind `--reindex-vectors` to primary probe flagset in `resolveTimeoutFromArgs` and delete `isReindexVectorsInvocation`.
- `internal/cli/cli.go:L1099-1159`: shrink: 3 identical 20-line resume hit helpers (`codexResumeHits`, `antigravityResumeHits`, `gooseResumeHits`). Consolidate into single `scopeResumeHits(scopes, prefix)` helper.
- `internal/cli/cli.go:L1412-1439`: delete: duplicate `!checkedFreshness` block and redundant `scopes.Resolve` before/after in `runBrowseScoped` loop. Keep single freshness check after successful resolve.
- `internal/cli/cli.go:L627-634`, `L719-726`, `L1320-1326`, `L1457-1463`, `L1658-1671`: shrink: repeated 8-line stale note generation and `maybeSpawnIngest` handling across read, outline, browse, and search. Extract small `staleIngestNote` and `sessionStaleNote` helpers.
- `internal/cli/cli.go:L467-480`: shrink: `isArchiveSyncInvocation` branch cascade with redundant length checks. Simplify to direct match on leading tokens.
- `internal/cli/cli.go:L307-320`, `L782-785`: shrink: duplicate loops collecting source IDs from `sources.Registered()` in flag completion and `runRoot`. Extract local `validSourceIDs` helper.

## Cross-Package Dedup Opportunities (Reported, Unmodified per Fence)

- `lastSlice8` / `trunc8`: Duplicated across `internal/cli/cli.go`, `internal/cli/cmd_tag.go`, and `internal/cli/tagrefresh.go`. Sibling files are owned by other agents per fence rules; cross-package helper can be unified in a future pass in a shared package (e.g. `internal/paths` or `internal/agentproto`).
- Clone sites in `*_test.go`: Journey and fixture helpers across `cmd_*_test.go` contain test setup duplication (e.g., temporary directory environment overrides and mock CLI execution). Reported per fence guidelines for test-file owner review.
- `internal/cli/timeout.go`: Lean already. Self-bounding watchdog and deadline resolution are minimal and adhere to zero-allocation where possible.

## Scoring

net: -95 lines possible.
