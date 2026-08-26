# Tag overlay findings

- Authoritative seam: `runTagWriteCmd` writes the current tag rows to the resolved refresh/per-project DB, then best-effort calls `index.SyncConsolidatedFrom`; `runTagPrepCmdWithSources` reads only `consolidated.db` via `readConsolidatedTopics` before computing its window.
- Contract: a committed tag must be visible to the next tag-prep/read even when consolidated publication is delayed or absent; unreadable authoritative data must remain an error, while derived-store staleness must not erase current topics.
- Smallest failing test: write a topic to a session's refresh DB, leave consolidated without that row, call the tag-prep topic-read path, and assert the current topic is returned exactly once in deterministic order.
- Smallest fix: read the authoritative resolved DB and overlay it over consolidated rows for the session; deduplicate by stable segment identity and keep authoritative replacements instead of stale derived copies. No cache, new seam, persistence format, or detached publication.
- Authorized files: `internal/cli/cmd_tag.go`, `internal/cli/tagrefresh.go`, `internal/cli/tagrefresh_test.go` (new `tagoverlay.go` only if inlining is larger).
- Estimate: +35 test lines, +25 production lines, 0 new dependencies.

## Han rival census checkpoint

This report-only lane audits the named Ozzy and newer rival claims against immutable Git objects,
patch identity, ancestry, current-base readiness, and personally observed gates. No production-code
changes are authorized. Detailed parity results belong in `RIVAL_CENSUS.md` after this checkpoint.
