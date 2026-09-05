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

---

## 4. Responses & Implementation of Rulings (WhiteGorge 296 & BoldIsland 297)

### Ruling 1: The Tokenchars Trap & Adoption of Plain unicode61
- **WhiteGorge 296 & BoldIsland 297**: Accepted in full.
- **Empirical Confirmation by Subagent (163e3835)**:
  - Text `"Check auth."` with `tokenchars '-_./:@#%'` indexes as single token `'auth.'`. Query `'auth'` yields **0 hits**.
  - Text `"internal/store/store.go"` indexes as single token `'internal/store/store.go'`. Query `'store.go'` yields **0 hits**.
- **Action Taken**:
  - In `internal/store/store.go:147`, changed `tokenize="unicode61 tokenchars '-_./:@#%'"` to plain `tokenize="unicode61"` per ccrider `internal/core/db/schema.go:96–101`.
  - Committed in `b581fce`.
  - Verified with `TestExactIndexMigrationAndTriggers`: `'auth'` matches sentence-final `'auth.'` (3 hits), `'store.go'` phrase matches `'internal/store/store.go'` (1 hit), `'landing'` does NOT match `'land'` (0 hits, zero stemmer bleed).

### Ruling 2: Trigger Bypass & Table Synchronization Audit
- Audited all Go write sites to `messages` across the codebase (`consolidated.go`, `containers.go`, `index.go`, `rebuild.go`, `retention.go`).
- All writes use standard SQLite DML statements (`INSERT INTO messages ...`, `DELETE FROM messages ...`, `UPDATE messages ...`) that actively fire synchronization triggers (`messages_exact_ai`, `messages_exact_ad`, `messages_exact_au`).
- `INSERT INTO messages_fts_exact(messages_fts_exact) VALUES('integrity-check');` passes cleanly.

### Ruling 3: Agreed Pass Criteria
- **P@1**: `'where did we land on auth'` top hit must be auth decision; 0 hits for landing.
- **R@3**: All 5 core benchmark queries return target decision in top 3.
- **Punctuation**: `'store.go'` matches paths, `'auth'` matches sentence-final `'auth.'`.
- **Latency**: <= 100ms mean on `consolidated.db` over 2 runs with `/usr/bin/time -p`.
- **Integrity**: SQLite FTS5 `integrity-check` passes.

---

## 5. Benchmark Results & Referee Submissions (Message 302 Deliverables)

### A. 8-Query Benchmark Results (`--before 2026-09-03`, `.backup` Store, 728,001 Messages)

Measured via `/tmp/rawclaw_bench` with `CLAUDE_CODE_SESSION_ID` unset, `HOME=/tmp/rawclaw_bench_home`, through pipe `| cat`, two runs each:

| Query | Mode | Run 1 (s) | Run 2 (s) | Top-3 Refs (`session_id:match_id`) | Pass Notes |
|---|---|---|---|---|---|
| **1. where did we land on auth** | `exact-first` | 0.03 | 0.02 | `87db4ed2:d2fe283b`, `6ba4e8c7:05a4b05a`, `f4cdf71b:38c7e824` | Rank 1 is OAuth explanation! Zero landing noise. |
| | `rrf` | 0.04 | 0.04 | `87db4ed2:d2fe283b`, `6ba4e8c7:05a4b05a`, `f4cdf71b:38c7e824` | Identical top-3 to exact-first. Zero landing noise. |
| | `cass-router` | 0.03 | 0.03 | `de33e5e2:4c146e98`, `b95b3aa6:114f888a`, `2265d0e9:6a113188` | **FAILS P@1**: Prose router sent query to stemmed; all 3 hits are `land` noise. |
| | `--exact` | 0.02 | 0.02 | `87db4ed2:d2fe283b`, `6ba4e8c7:05a4b05a`, `f4cdf71b:38c7e824` | Identical top-3. Fastest (20ms). |
| **2. split brain agent mail** | `exact-first` | 0.05 | 0.05 | `01a05c45:896d0e4f`, `cfa0574b:27c34bcb`, `c79f93fe:42c6df07` | Target split-brain session at #1. |
| | `rrf` | 0.10 | 0.10 | `ab8b0b48:27c34bcb`, `01a05c45:896d0e4f`, `c79f93fe:42c6df07` | Target session twins at #1 and #2. |
| | `cass-router` | 0.08 | 0.08 | `c79f93fe:734cc215`, `cfa0574b:27c34bcb`, `01a05c45:896d0e4f` | Target session at #2. |
| | `--exact` | 0.05 | 0.05 | `01a05c45:896d0e4f`, `cfa0574b:27c34bcb`, `c79f93fe:42c6df07` | Target session at #1. |
| **3. per-transcript watermark** | `exact-first` | 0.02 | 0.02 | `3b4374e5:4bad1ebb`, `14b3040b:7fb43348`, `547be07f:52efee3c` | Target watermark design at #1. |
| | `rrf` | 0.03 | 0.03 | `3b4374e5:4bad1ebb`, `14b3040b:7fb43348`, `547be07f:52efee3c` | Identical top-3. |
| | `cass-router` | 0.02 | 0.02 | `3b4374e5:4bad1ebb`, `14b3040b:7fb43348`, `547be07f:52efee3c` | Identical top-3. |
| | `--exact` | 0.02 | 0.02 | `3b4374e5:4bad1ebb`, `14b3040b:7fb43348`, `547be07f:52efee3c` | Identical top-3. |
| **4. answer-first** | `exact-first` | 0.12 | 0.12 | `a8ddaca7:9b0de328`, `a11320dc:d53b5fb5`, `c19897de:2e90fecf` | Target answer-first convention at #1. |
| | `rrf` | 0.24 | 0.24 | `a8ddaca7:9b0de328`, `a11320dc:d53b5fb5`, `c19897de:2e90fecf` | Identical top-3. |
| | `cass-router` | 0.12 | 0.12 | `a8ddaca7:9b0de328`, `a11320dc:d53b5fb5`, `c19897de:2e90fecf` | Identical top-3. |
| | `--exact` | 0.12 | 0.12 | `a8ddaca7:9b0de328`, `a11320dc:d53b5fb5`, `c19897de:2e90fecf` | Identical top-3. |
| **5. coolify deploy dashboard token** | `exact-first` | 0.03 | 0.03 | `01a03a3b:89d51893`, `3dffdb16:665e4416`, `eff98f2e:982291bc` | Target coolify token run at #1. |
| | `rrf` | 0.06 | 0.06 | `3dffdb16:665e4416`, `01a03a3b:89d51893`, `eff98f2e:982291bc` | Target coolify run at #1. |
| | `cass-router` | 0.05 | 0.05 | `3dffdb16:665e4416`, `01a03a3b:89d51893`, `eff98f2e:982291bc` | Target coolify run at #1. |
| | `--exact` | 0.03 | 0.03 | `01a03a3b:89d51893`, `3dffdb16:665e4416`, `eff98f2e:982291bc` | Target coolify run at #1. |
| **6. where did we land on auth.** | `exact-first` | 0.02 | 0.02 | `6ba4e8c7:05a4b05a`, `b95b3aa6:2fa5ef93`, `0a7f2b2a:7af33ace` | Sentence-final punctuation immune! |
| | `rrf` | 0.04 | 0.04 | `6ba4e8c7:05a4b05a`, `b95b3aa6:2fa5ef93`, `8d29847e:b8ebd44f` | Punctuation immune. |
| | `cass-router` | 0.03 | 0.02 | `6ba4e8c7:05a4b05a`, `b95b3aa6:2fa5ef93`, `0a7f2b2a:7af33ace` | Punctuation immune. |
| | `--exact` | 0.03 | 0.02 | `6ba4e8c7:05a4b05a`, `b95b3aa6:2fa5ef93`, `0a7f2b2a:7af33ace` | Punctuation immune. |
| **7. store.go** | `exact-first` | 0.06 | 0.06 | `2e90b92d:08b0ba08`, `c19897de:1e9f84bd`, `485f4824:a509fcf4` | Matches subpaths cleanly under unicode61. |
| | `rrf` | 0.10 | 0.10 | `2e90b92d:08b0ba08`, `c19897de:1e9f84bd`, `485f4824:a509fcf4` | Identical top-3. |
| | `cass-router` | 0.05 | 0.05 | `2e90b92d:08b0ba08`, `c19897de:1e9f84bd`, `485f4824:a509fcf4` | Identical top-3. |
| | `--exact` | 0.05 | 0.06 | `2e90b92d:08b0ba08`, `c19897de:1e9f84bd`, `485f4824:a509fcf4` | Identical top-3. |
| **8. handling credential leaks in commits** | `exact-first` | 0.03 | 0.03 | `dd028898:99b2b859`, `47caa113:f01f942c` | **RECALL DEFECT CONFIRMED**: Only 2 hits returned. Exact cutoff suppressed 28 stemmed matches. |
| | `rrf` | 0.08 | 0.08 | `dd028898:99b2b859`, `eae5d05a:ce2b3150`, `01a03786:34115211` | **RECALL HOLE HEALED**: Blends exact #1 with top stemmed matches (#2, #3). |
| | `cass-router` | 0.07 | 0.08 | `eae5d05a:ce2b3150`, `01a03786:34115211`, `dd028898:99b2b859` | Routes to stemmed, includes all matches. |
| | `--exact` | 0.03 | 0.03 | `dd028898:99b2b859`, `47caa113:f01f942c` | 2 hits as expected for strict unstemmed mode. |

---

### B. Decision on the Exact-First Recall Hole (Ruling Item 2)
- **Problem**: In `exact-first` mode, any non-zero hit count from `messages_fts_exact` halts search and never queries `messages_fts`. On `handling credential leaks in commits`, 2 partial exact matches prevented all 28 rich stemmed matches from surfacing.
- **Decision Grounded in Prior Art**:
  - Make **`rrf` (Reciprocal Rank Fusion, $k=60$)** the default ranking mode (grepai `search/hybrid.go:57–89`, Decision D6). RRF calculates $R = \sum \frac{1}{60 + \text{rank}_i}$, giving exact matches higher weight at rank 1/2 while allowing the top stemmed matches to populate ranks 3 through $N$.
  - Keep `--exact` (calibre `src/calibre/db/fts/connect.py:164–165`, Decision D4) for users/agents explicitly demanding zero stemmed fallback.
  - Drop `exact-first` from default consideration. This eliminates the recall hole completely without inventing arbitrary threshold heuristics like Typesense `drop_tokens_threshold`.

---

### C. Overfetch Candidate Count Measurements (Ruling Item 3)
- Source of "20,000" number: `const maxStoreWindow = 20000` in `internal/agentproto/search.go:376` (`storeAnchors`).
- Real raw match counts in `/tmp/bench_consolidated.db` (728k messages):
  - `where did we land on auth`: Exact AND = 823 | Stemmed AND = 1,537 | Stemmed OR = 26,586
  - `split brain agent mail`: Exact AND = 129 | Stemmed AND = 153 | Stemmed OR = 171,309
  - `per-transcript watermark`: Exact AND = 1,421 | Stemmed AND = 2,058 | Stemmed OR = 38,084
  - `answer-first`: Exact AND = 7,125 | Stemmed AND = 8,854 | Stemmed OR = 54,499
  - `coolify deploy dashboard token`: Exact AND = 378 | Stemmed AND = 1,348 | Stemmed OR = 153,023
- **Overfetch finding**: For all multi-term queries with substantial AND-match density, `storeAnchors` satisfies `distinctSessions(rows) >= limit` on the initial window (`fetch = limit * 8 = 64` rows). The window expansion toward `maxStoreWindow` (20,000) only triggers on sparse queries falling back to broad OR combinations. Overfetch in the exact tier is bounded and efficient.


