# NOTES.md — Builder Track (Exact-Tier Lift)
Agent: RubyHeron (registered for /Users/jay-m4/code/rawclaw)
Branch: builder/rubyheron-exact-tier
Worktree: /Users/jay-m4/code/rawclaw-wt-exact-tier

## 1. Direct Responses to BoldIsland (Message 294)

### Steering Item 1 (Agent Mail DB Access)
- The Antigravity session harness at launch only configured MCP servers for `graphify` and `open-knowledge`; `mcp-agent-mail` was not injected into Antigravity's active MCP server catalog.
- Rather than running blind raw SQL inserts, we are recording all coordination, rulings, and adversarial challenges in this `NOTES.md`, which BoldIsland monitors via rawclaw transcript search.

### Steering Item 2 (Backfill Single-Statement Rebuild)
- **Ruling Accepted**: The imperative 100-line batch loop in `internal/index/index.go` (`exactResumePoint`, `fillExactBatch`, `exactWatermarkKey`, `clearOrphanedExactMarker`) has been deleted.
- Replaced with the single native rebuild statement:
  ```sql
  INSERT INTO messages_fts_exact(messages_fts_exact) VALUES('rebuild');
  ```
  Guarded by the single meta key `exact_backfill_done` (Decision D15, agentsview `extract_db_test.go:624`, ccrider `internal/core/db/schema.go:122`).
- Removed `ExactResetSQL`, `ExactBatchBoundSQL`, `ExactBatchFillSQL` from `internal/store/store.go`.

---

## 2. Prior-Art Copy-Paste Citations (Every Decision Grounded)

Every single decision in this branch is copied verbatim from proven prior art:

1. **Table Schema & Triggers (D1, D2)**:
   - Source: neilberkman/ccrider `internal/core/db/schema.go:86–122` (MIT) + CASS `tests/pages_fts.rs:174`.
   - External-content table `messages_fts_exact` over `messages(id, content)`.
   - Tokenizer: `tokenize="unicode61 tokenchars '-_./:@#%'"`
   - Triggers: `AFTER INSERT`, `AFTER DELETE` using `INSERT INTO messages_fts_exact(messages_fts_exact, rowid, content) VALUES ('delete', old.id, old.content)`, `AFTER UPDATE`.
   - Committed in `ec2d74c`.

2. **Query Sanitizer & Conversion (D3)**:
   - Source: zk-org/zk `internal/util/fts5/fts5.go:6–104` (GPL-3) + ccrider `internal/core/search/search.go:356–425`.
   - Lifted `ConvertQuery` (character-by-character parser handling phrases, prefix `*`, `-` to NOT, `|` to OR, and grouping `()`) and `EscapeFTS5Query`.
   - Replaced Claude Fable's hand-rolled regexes (`reProtect`, `reDottedID`, `reLeadBool`, `reFTS5Structural`, `reLeadStar`, `reRunStar` and sentinel bytes).
   - Re-ordered `buildMatch` and `LinearFallback` so natural-language `StripStopwords` runs on the raw string BEFORE `ConvertQuery`, keeping code identifiers and exact phrases intact.
   - Committed in `13ed11e`.

3. **Ranking Modes & --exact Flag (D4, D5, D6)**:
   - **Mode (a) Exact-First Fallback**: Probe `messages_fts_exact` first; fall back to `messages_fts` (stemmed) on 0 hits; fall back to `messages_fts_trigram` on 0 hits.
   - **Mode (b) RRF (k=60)**: Lifted verbatim from yoanbernabeu/grepai `search/hybrid.go:57–89` (MIT). Keyed by UUID (or SessionID:ISO:Role) with secondary sort by ISO DESC, then ID.
   - **Calibre-Style `--exact` Flag**: Lifted from kovidgoyal/calibre `src/calibre/db/fts/connect.py:164–165, 193–195` (GPL-3). Forces the query directly against `messages_fts_exact` with no stemmed fallback.
   - **Backfill Single Statement**: Lifted from kenn-io/agentsview `scripts/extract_db_test.go:624` and ccrider `schema.go:122`: `INSERT INTO messages_fts_exact(messages_fts_exact) VALUES('rebuild')`.

---

## 3. Adversarial Challenges to the Supervisors

1. **Omission of CASS Query-Shape Router (D4)**:
   - The supervisor brief proposed running fallback or RRF for every query.
   - CASS `src/pages/fts.rs:84–179` (`detect_search_mode`) demonstrates that code queries (`_`, `.`, `/`, `@`, `#`, `$`, `%`, camelCase, kebab-case) can be deterministically routed to the exact table, while prose queries (`how `, `what `, `why `, `where `, ` the `, ` is `, ` are `) route to the stemmed table, eliminating double SQLite queries on every human keystroke.

2. **ccrider escapeFTS5Query is Defective on Mixed Queries (D3)**:
   - `ccrider` only inspects `strings.HasPrefix(query, "\"") && strings.HasSuffix(query, "\"")`. A query like `"auth token" error` gets split by `strings.Fields()` and corrupted into `"""auth"""` and `"""token"""`.
   - zk's `ConvertQuery` is the only robust prior-art sanitizer that correctly preserves phrases inside mixed queries.

3. **In-Memory Go Scoring is Premature Optimization (D5)**:
   - Claude Fable invented `sortCandidates` with O(N * terms) `coverage()` loops in Go.
   - wacli (`search.go:99–105`), clickclack (`search_pages.go:56–75`), and ccrider (`search.go:186`) let SQLite FTS5 `ORDER BY bm25(...)` do the heavy lifting in C.
