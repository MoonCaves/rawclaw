# T85 issue #26 index audit

## Verdict

`PARTIAL / ONE INDEX MISSING`: current base `029f60d77e7e03192bc966de3a835a4a32a00fe2` already contains the issue's tombstone existence guard and one transaction. The only uncovered indexed workload found is `file_index(session_id)`. The minimal fix is `CREATE INDEX IF NOT EXISTS idx_file_index_session ON file_index(session_id)`, but its schema owner is `internal/store/store.go`, outside this worker's authorized fence; no production change was made pending supervisor authorization.

## Issue and current-code coverage

Issue #26 names six deletes, each with `session_id = ? OR session_id LIKE ? ESCAPE '\\'` (the sessions table uses `id`): `messages`, `sessions`, `session_sources`, `file_index`, `topic_segment`, and `session_verdict`. Current `pruneTombstonedIDs` first executes `SELECT 1 FROM sessions WHERE id=?`; `sessions.id` is the primary key. Missing IDs skip all six deletes. Existing IDs are pruned in one transaction, preserving the escaped anchored subagent pattern.

Other current `file_index` predicates are:

| Predicate | Existing coverage | Current plan | With candidate index |
| --- | --- | --- | --- |
| `SELECT path ... WHERE session_id=? LIMIT 1` (prewarm) | none | `SCAN file_index` | `SEARCH file_index USING INDEX idx_file_index_session (session_id=?)` |
| `DELETE ... WHERE session_id=? AND path<>?` (reindex/append) | none | `SCAN file_index` | `SEARCH file_index USING INDEX idx_file_index_session (session_id=?)` |
| provenance backfill correlated `WHERE file_index.session_id=sessions.id` | none | correlated `SCAN file_index` | correlated `SEARCH ... USING INDEX idx_file_index_session` |
| `WHERE path=?` reads/deletes | `file_index.path` primary key | covering/index search | unchanged |
| full `SELECT ... FROM file_index` watermark/retention scans | no predicate needed | table scan | unchanged |

The six tombstone delete plans on the current base are indexed only for `messages.session_id`, `sessions.id`, `session_sources(session_id,source_db)`, and `topic_segment.session_id`; `file_index` and `session_verdict` are scans. This does not defeat issue #26's absent-ID optimization because the indexed `sessions.id` guard skips those deletes entirely for absent sessions. For an existing tombstone, the LIKE-or-equality shape remains a scan in SQLite; adding a speculative index for that shape is not justified by this issue.

## Peer and identity evidence

- Rabbit `ff1fedacc128df312b491356291243a1c0c5c338` is directly based on current base `029f60d`; it adds one schema line and two schema assertions.
- Khan `8f89adacf1b0278cffcfa44f945e43e68587d36f` is based on `3b1e899`; it has the same two-file, 12-line patch.
- Stable patch-id for both is `5844b92d9c7f763dae2656b25be0f588ba77d524`.

## Graphify corroboration

Canonical Graphify MCP call: `mcp__graphify__query_graph({question: "index file_index session_id EnsureSchema", depth: 2, token_budget: 4000, project_path: "/Users/jay-m4/code/rawclaw"})`.

Result: the graph returned `EnsureSchema()` at `internal/index/index.go:65`, `Rebuild()` at `internal/store/store.go:287`, `loadFileIndex()` at `internal/index/index.go:1105`, and `reindexFileWithOrigin()` at `internal/index/index.go:607`; it did not surface a schema-owned `file_index` index node. This corroborates the source inspection: schema ownership and the missing index belong in `internal/store/store.go`, while the current worker fence permits only the index package and report.

## Deterministic mutation/plan check

SQLite `EXPLAIN QUERY PLAN` on equivalent current schema produced `SCAN file_index` for each `session_id` predicate above. Adding only `idx_file_index_session` changed each targeted equality lookup to `SEARCH ... USING INDEX idx_file_index_session`; dropping it restored `SCAN file_index`. Primary-key `path=?` remained an existing `sqlite_autoindex_file_index_1` search. This is direct plan evidence for the one missing index; no speculative index was added.

## Gates and tree

- `git diff --check`: PASS before this report.
- Focused `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...`: launched, but did not return within the bounded observation window; result is `UNCERTAIN`, not green.
- No Go files changed, so no `gofmt` action was required.
- Report-only commit is required because the proper one-line schema fix is outside the authorized file fence.
