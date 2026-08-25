# Go standards audit wave 2

Date: 2026-08-25

Target: `main` at `789ab2e`, covering the commits returned by
`git log --since="10 hours ago" --oneline main`. I read the routed Go skills from
an external Go standards skill set, starting with `golang-how-to`, and
read `AGENTS.md`, `docs/notes/adversarial-review-20260825.md`, and
`docs/notes/golang-skills-audit.md` first.

This wave reports only findings in the later merged code that are not already in
the two baseline notes. The RawClaw project rule wins where it is stricter than
the generic skill: in particular, `AGENTS.md` requires no silent truncation or
failure and requires the static, dependency-free core to remain honest about
partial state.

## Findings

### 1. High — rebuild can destroy the existing consolidated store before replacement is viable

- **File:line:** `internal/index/consolidated.go:354-362`
- **Merged code:** `ConsolidateFrom(..., rebuild=true)`
- **Skill rule:** `golang-error-handling` — “Returned errors MUST always be checked — NEVER discard with `_`.”
- **Why it matters here:** The rebuild path removes `consolidated.db`, its WAL, and its SHM before `store.ConnectRW`, schema setup, migrations, or source folding have succeeded. The remove errors are discarded. If the open, schema migration, or any later rebuild step fails, the previously usable consolidated copy is already gone. That is a data-availability loss in the one-store read path, and it conflicts directly with RawClaw's no-silent-failure invariant. A rebuild is recoverable from project stores only if every source remains readable and the rebuild completes.
- **Smallest fix:** Build the replacement in a separate temporary database directory/path, validate and close it successfully, then swap it into place atomically. Preserve the old database until the replacement is complete; handle the WAL/SHM sidecars as part of the swap and report cleanup failures.

### 2. High — consolidated source folding is not atomic despite claiming transaction rollback

- **File:line:** `internal/index/consolidated.go:487-515`, `589-641`
- **Merged code:** `consolidateOne`
- **Skill rule:** `golang-database` — “Use transactions for multi-statement operations — wrap related writes in `BeginTxx`/`Commit`.”
- **Why it matters here:** The comment at lines 487-490 says the merge is atomic and rolls back, but the function never calls `Begin`, `Commit`, or `Rollback`. It performs session-source upsert/prune, session merge, message merge, topic/verdict merge, recount, file-index merge, and watermark stamping as independent autocommit statements. A failure after the message merge but before the watermark can leave a partially folded source while the next retry reuses partially committed rows. A failure in a later source also leaves earlier sources committed even though `ConsolidateFrom` returns an error. This can produce mismatched sessions, messages, provenance, file watermarks, and sync marks.
- **Smallest fix:** After the read-only `ATTACH` and watermark checks, begin one transaction per source, run every merge/prune/recount/file-index/watermark write through that transaction, roll back on every error, commit before `DETACH`, and test failure after each write stage.

### 3. High — freshness checks fail open on database and directory errors

- **File:line:** `internal/index/consolidated.go:911-918`, `1014-1023`
- **Merged code:** `CheckIndexFreshness` and `CheckProjectFreshness`
- **Skill rule:** `golang-error-handling` — “Returned errors MUST always be checked — NEVER discard with `_`.”
- **Why it matters here:** `CheckIndexFreshness` discards both `QueryRow(...).Scan` errors. If one metadata query fails while the other returns an old value, the function can compare that partial state and return `Fresh: true`; a database read failure is treated as a current index. In `CheckProjectFreshness`, when no file-index rows are found, `os.ReadDir(tdir)` is also discarded. An unreadable directory therefore skips the only unindexed-transcript check and returns `Fresh: true`. The new O(1) freshness gate then suppresses the refresh and makes later reads/searches answer from an incomplete store, contrary to the project's explicit no-silent-failure rule.
- **Smallest fix:** Check and classify both metadata scans, returning an error or `Fresh:false` on any unexpected database error. Check `os.ReadDir` and return `Fresh:false` with the reason when enumeration fails. Add tests for each failed query and an unreadable project directory.

### 4. Medium — source-filtered search can skip refreshes using freshness from another source

- **File:line:** `internal/cli/cli.go:1513-1529`; `internal/index/consolidated.go:960-984`
- **Merged code:** default search freshness gate and `CheckProjectFreshness`
- **Skill rule:** `golang-error-handling` — “Returned errors MUST always be checked — NEVER discard with `_`.” The same fail-open result-handling principle applies to the freshness verdict: a successful `Fresh:true` must represent the requested source, not merely some other source's rows.
- **Why it matters here:** `runSearch` calls `CheckProjectFreshness(con, projLabel, td)` without passing `o.Source`. `CheckProjectFreshness` considers every `file_index` row for the project and every path below `tdir`, regardless of the requested source. Therefore a current Claude row can make the project appear fresh during `--source codex` (or another source) search even when that source has a new or never-ingested transcript. The refresh is then skipped, and the source-scoped query answers from stale consolidated rows. The added freshness tests exercise Claude-only fixtures and do not constrain this source axis.
- **Smallest fix:** Make freshness source-aware: pass the selected source ID into `CheckProjectFreshness`, filter the session/file-index query by `source_tool` (or maintain per-source ingest watermarks), and add an end-to-end test with one fresh source and one changed source in the same project.

### 5. Medium — ingest silently discards adapter discovery failures

- **File:line:** `internal/cli/cmd_ingest.go:64-75`, `159-169`, `199-223`
- **Merged code:** `runIngest`, `resolveIngestMatches`, and `discoverAllIngestSources`
- **Skill rule:** `golang-error-handling` — “Returned errors MUST always be checked — NEVER discard with `_`.” Also: “Errors MUST be either logged OR returned, NEVER both.”
- **Why it matters here:** The all-sessions path continues on every `adapter.Discover()` error and returns `nil` error. The targeted path gets an error from `discoverTagSources`, but `resolveIngestMatches` falls through to consolidated lookup and finally returns `(nil, nil)` when discovery found no match. `runIngest` also ignores a non-nil resolver error whenever partial matches exist. A broken source adapter can therefore make `rawclaw ingest` report success or “no sessions” while that source was never searched. This violates RawClaw's explicit requirement that an agent must not mistake a partial answer for a complete one.
- **Smallest fix:** Collect discovery errors with source IDs and return them (using `errors.Join` where several sources fail). Preserve partial matches only with an explicit partial-result error/status in both human and JSON output; never convert a discovery failure into “no sessions.”

## Reviewed without a new finding

- The prior Goose `rows.Err()` issue, dynamic-identifier issue, semantic worker cancellation issue, atomic `restoreSession` issue, routine-query error issue, archive goleak gap, and embedding HTTP context issue were checked against the baseline notes and are not repeated here.
- The final `IndexStatusUnknown` merge correction was checked; the enum now has an explicit unknown zero value and error paths no longer report `IndexFresh`.
- The changed Go files were reviewed as the `main` tree, not the older checked-out worktree commit.
