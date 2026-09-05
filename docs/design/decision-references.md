# Decision references (2026-09-05)

Every fork we will hit while lifting search from prior art, and the exact places to open before choosing.
`✔` = file and lines read by BoldIsland this session. `scout` = reported by a Sourcegraph scout, lines as given, not re-read.
Licenses are recorded, never a filter. Companion: `steal-code.md` (verbatim blocks) and `prior-art-steal-list.md` (ranked units).

## D1. Shape of the second FTS5 table: external-content over `messages`, or standalone copy?

- ✔ neilberkman/ccrider `internal/core/db/schema.go` 86–122 — external-content pair, `'delete'` triggers. MIT.
- ✔ kovidgoyal/calibre `resources/fts_sqlite.sql` 22–43 — external-content pair over `books_text`, years in production. GPL-3.
- ✔ Dicklesworthstone/coding_agent_session_search `tests/pages_fts.rs` 167–175 — standalone pair (own content copy). Local checkout.
- ✔ RawClaw `internal/store/store.go` 67 and 100 — our current standalone `messages_fts` and trigram table.
- scout kenn-io/agentsview `internal/db/db.go` 497–510 — external-content `content='messages'`, plus a second FTS for recall entries.
- scout fastclaw-ai/fastclaw `internal/store/fts.go` ~50 — external-content with extra filter columns (`agent_id`, `chat_id`).
- scout sympozium-ai/sympozium `cmd/memory-server/main.go` 819 — same pattern, memories table.
- scout safedep/gryph `storage/fts.go` 16 — standalone with `UNINDEXED` id columns for joins.
- scout debian-calibre/calibre `resources/metadata_sqlite.sql` 176–190 — the pair with stock `unicode61`, no custom tokenizer.
- scout MemTensor/MemOS `001-initial.sql` 351 — trigram table as a separate sidecar.

## D2. Tokenizer for the exact table

- ✔ CASS `tests/pages_fts.rs` 174 — `unicode61 tokenchars '-_./:@#$%\\'`.
- ✔ ccrider `schema.go` 96–101 — plain `unicode61`.
- ✔ calibre `fts_sqlite.sql` 22 — `unicode61 remove_diacritics 2` (via their wrapper).
- ✔ navidrome `persistence/sql_search_fts.go` 138–143 — why `remove_diacritics 2` is not enough, transliteration at query time.
- ✔ navidrome 17–30 — CJK detection, tokenizer cannot split CJK.
- scout continuedev/continue `core/indexing/FullTextSearchCodebaseIndex.ts` 29–35 — trigram for code paths.
- ✔ RawClaw `store.go` 100 — our trigram table for substring probes.

## D3. Query sanitizing and operator handling

- ✔ zk-org/zk `internal/util/fts5/fts5.go` 6–104 — quotes, parens, AND/OR/NOT passthrough, `^`/`*`, `col:`, `-` as NOT, `|` as OR. GPL-3.
- ✔ ccrider `internal/core/search/search.go` 356–425 (+ `search_test.go`) — quote every token, keep phrases and trailing `*`. MIT.
- ✔ navidrome `sql_search_fts.go` 32–43, 107–188 — strip specials, lowercase operators, `(t OR t*)`, degraded detection. GPL-3.
- ✔ CASS `src/pages/fts.rs` 30–34 — `escape_fts5_query`, quote and double inner quotes.
- scout openclaw/wacli `internal/store/search.go` 72–107 — `sanitizeFTSQuery`, implicit AND.
- scout dbinky/Pommel `internal/db/fts.go` 154–230 — keeps AND/OR/NOT and phrases, strips unsafe chars.
- scout nyxCore-Systems/ckb `internal/storage/fts.go` 566–600 — minimal escape for phrase and prefix.
- scout zhimaAi/ChatClaw `internal/fts/tokenizer/tokenizer.go` 190–320 — per-token prefix builder with dedup.
- scout mvanhorn/printing-press-library `hackernews/internal/cli/search_local.go` 116–125 — smallest correct version.
- scout ddxfish/sapphire `plugins/memory/tools/knowledge_tools.py` 1163–1182 — exact query first, then rebuild with OR and prefix.

## D4. Which table answers: route by query shape, user flag, exact-first fallback, or merge?

- ✔ CASS `src/pages/fts.rs` 84–179 — `detect_search_mode`, route code-shaped to exact, prose to stemmed.
- ✔ calibre `src/calibre/db/fts/connect.py` 164–165, 193–195 — user flag flips the table name, everything else shared.
- ✔ navidrome 156–177 — exact-word boost inside one bm25 expression, no second table needed for that effect.
- ✔ yoanbernabeu/grepai `search/hybrid.go` 57–89 — RRF over N lists, k=60. MIT.
- ✔ RawClaw `internal/retrieve/retrieve.go` `buildMatch` (commit 0b06796) — AND first, OR on zero hits; same control flow.
- scout tridz-dev/huf `huf/ai/knowledge/backends/sqlite_hybrid.py` 340–347 — RRF written in SQL with `COALESCE(1.0/(60+rnk))`.
- scout paradedb/paradedb `pg_search/tests/pg_regress/sql/reciprocal_rank_fusion.sql` 49–60 — RRF as a `UNION ALL` in SQL.
- scout dvlin-dev/moryflow `apps/moryflow/pc/src/main/search-index/store.ts` 33–68 — mode→table lookup map.
- concept Typesense `drop_tokens_threshold` — AND, then drop lowest-value token, retry. Docs, no file.
- concept Elasticsearch multi-fields `text` + `keyword` with boost — the industry shape.

## D5. Ranking order and tie-breaks

- scout openclaw/wacli `search.go` 99–105 — `ORDER BY bm25(messages_fts), m.rowid DESC`.
- scout openclaw/clickclack `apps/api/internal/store/sqlite/search_pages.go` 56–75 — bm25, then created_at, then id; cursor pagination on all three.
- scout EKKOLearnAI/hermes-studio `packages/server/src/db/hermes/sessions-db.ts` 1624–1635 — `ORDER BY rank, base.last_active DESC`.
- scout taracodlabs/aiden `core/v4/sessionStore.ts` 557–568 — role filter in the MATCH query, configurable orderBy.
- ✔ ccrider `search.go` 236–262 — session score: 10 per matching message, +50 if query in summary, log recency decay.
- ✔ navidrome 196–232 — per-column bm25 weights table, precomputed at init.
- scout souvikinator/lsx `utils/rank.go` 1–25 — Mozilla frecency `1000*hits*e^(-λ·age)`.
- scout zzet/gortex `internal/mcp/frecency.go` 90–140 — per-access decay summed.
- ✔ RawClaw `internal/agentproto/search.go` 651–690 `sortCandidates` — fused, coverage, routine-last.

## D6. Snippet and highlight parameters

- scout hermes-studio `sessions-db.ts` 1624 — `snippet(messages_fts, 0, '>>>', '<<<', '...', 40)`, same markers as ours.
- ✔ calibre `connect.py` 169–176 — `snippet()` when a size is given, `highlight()` otherwise, size clamped 1–64.
- ✔ ccrider `search.go` 186 — `snippet(%s, -1, '', '', '...', 64)`, column -1 = best column.
- scout kunwar-shah/claudex `server/src/services/searchDatabase.js` 185 — `<mark>` tags, 64 tokens.
- scout wacli 99 — `'[' ']' '…' 12`, very short.
- ✔ CASS `fts.rs` 199 — `<mark>`, 64.
- scout gupsammy/Claudest `plugins/claude-memory/hooks/import_conversations.py` 716–730 — two snippet forms depending on column.

## D7. Context around a hit (before N, after M)

- ✔ RawClaw `internal/view` `AnchoredView`, `rawclaw read --around N` — already built, bookends included.
- concept VictoriaMetrics/VictoriaLogs `lib/logstorage/pipe_stream_context.go` — reported by an agy agent, path not verified.
- concept Zulip narrow `before`/`after` anchors — idea only.

## D8. One hit per conversation, dedup across forks

- ✔ RawClaw `internal/agentproto/search.go` 93–115 — uuid dedup plus project+root key.
- ✔ ccrider `search.go` 120–160 — `sessionMap`, best message per session, then sort sessions by score.
- scout Claudest 716–730 — group by conversation in SQL.
- concept git object model — content-addressed dedup, no file.

## D9. Pagination for agents

- scout clickclack `search_pages.go` 56–75 — keyset cursor over (rank, created_at, id) in both sort modes.
- ✔ RawClaw envelope `has_more`, `next_command`, `--offset` — offset-based today.

## D10. Freshness, watermarks, rotation, settle

- ✔ hpcloud/tail `watch/polling.go` 83–109 — `os.SameFile`, size shrink = truncate, size grow = append, mtime-only = rewrite. MIT.
- scout hpcloud/tail `watch/filechanges.go` 1–37 — coalescing notify.
- scout nxadm/tail `tail.go` 211–236 — reopen with retry on ENOENT during rotation.
- scout elastic/beats `filebeat/input/file/state.go` 1–86 — watermark record: id, source, offset, timestamp, OS identity.
- scout elastic/beats `filebeat/input/file/identifier.go` — pluggable identity strategies.
- ✔ RawClaw `internal/index/consolidated.go` 1467–1575 — directory mtime gate, settle window, purged = continue.
- concept git `core.untrackedCache`, Watchman `settle_period`, promtail write-temp-then-rename.
- scout fsnotify `backend_kqueue.go` — macOS has no truncate event; detect by size.

## D11. Vector coverage and backfill

- scout openclaw/discrawl `internal/store/store.go`, `store_write_test.go` — `embedding_jobs` lease-locked queue, `TestPendingEmbeddingJobsSkipsFreshLocks`. MIT.
- scout l33tdawg/sage `internal/store/sqlite.go` — `embedding_hash` to skip unchanged rows.
- ✔ RawClaw `internal/agentproto/search.go` ~330 — vector tier skipped when keyword fills the limit (bug, unfixed).
- ✔ RawClaw `chunk_vec` 124,788 rows of 725,226 messages — 17 percent coverage as of 2026-09-04.

## D12. Embedding client and fusion math

- scout grepai `embedder/retry.go` (99 lines), `embedder/ollama.go` (179), `embedder/openai.go` (537) — retry, batching, reorder by index. MIT.
- scout offgrid-llm `internal/rag/sqlite_store.go` ~430–480 — unrolled cosine.
- scout josephgoksu/TaskWing `internal/memory/sqlite.go` — float32 BLOB codec.
- scout KybernesisAI/kyberbot `packages/cli/src/brain/hybrid-search.ts` — 5-line FTS pool + RRF + LLM rerank (reported by agy, not re-read).
- fact: `RAWCLAW_EMBED_ENDPOINT` routes via LiteLLM to Voyage-4-large 1024-dim; no local model in the path.

## D13. Machine-mode output

- scout cli/cli `pkg/cmdutil/json_flags.go` 1–90 — `--json/--jq/--template`; `go-gh/v2/pkg/jq`, `pkg/template` importable. MIT.
- scout cli/cli `pkg/iostreams/iostreams.go` 180–190 — `IsStdoutTTY()`.
- scout cli/cli `internal/ghcmd/cmd.go` 210–220 — no-results message only on a TTY, exit 0 either way.
- scout kubernetes/cli-runtime `pkg/printers/tableprinter.go` 88–235 — header once per stream, wide columns.
- ✔ RawClaw `internal/cli/cli_options.go` 92–101 `machineStream`, `internal/agentproto/render.go` 72–135.
- concept ripgrep `--json` begin/match/context/end/summary.

## D14. Diagnostics and warnings

- scout dominikh/go-tools `lintcmd/lint.go` 369–398 — severity enum, merge strategy for duplicate findings.
- scout dominikh/go-tools formatters doc 46–79 — one JSON object per line, `code`, `severity`, `location`, `message`; text trailer with counts.
- ✔ RawClaw `render.go` `renderWarnings` (7119b7d) — single collapsed note line.
- concept rustc/clippy one-line coded diagnostics; Sentry fingerprint grouping; Postgres `client_min_messages`.

## D15. Migration of FTS schema on an existing store

- scout dvlin-dev/moryflow `store.ts` 33–68 — `SEARCH_SCHEMA_VERSION`, drop and recreate both tables on bump.
- scout kenn-io/agentsview `scripts/extract_db_test.go` 624 — `INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`.
- ✔ RawClaw rule (AGENTS.md, index.go) — additive ALTER, never version-bump rebuild; the DB is the only copy of purged sessions.

## D16. Search-engine-as-a-service seam (if local ranking is still short)

- fact: ai-server runs Meilisearch v1.52 bundled with LibreChat (indexes `convos`, `messages`, empty); content-server runs v1.6 bundled with Hoarder. Neither is ours.
- concept Meilisearch MCP, Typesense, dedicated Coolify one-click instance.

## Not found or not verified this week

- `mattn/go-searchquery` — does not exist on Sourcegraph.
- Loki promtail `positions.go` — no longer at the reported path.
- Continue `FullTextSearchCodebaseIndex.ts` — one scout found it, another could not; line numbers above from the first.
- xoai/sage-wiki, philippgille/chromem-go, coder/hnsw — unread.

## Additions 2026-09-05, scout RubyHeron (message 323 in thread prior-art-lines)

Verified by RubyHeron on Sourcegraph; line numbers not supplied, paths and defaults as reported.

- **D3 stopwords** — apache/lucene `lucene/analysis/common/src/resources/org/apache/lucene/analysis/snowball/english_stop.txt` (Apache-2.0; list itself Snowball BSD-3): 174 words, one per line, `|` comments. postgres/postgres `src/backend/snowball/stopwords/english.stop` (PostgreSQL licence): identical 174 words, read by `readstoplist()` in `src/backend/tsearch/ts_utils.c`. Either replaces `query.Stopwords`.
- **D1/D15 FTS5 external-content contract** — sqlite/sqlite `ext/fts5/test/fts5content.test`: the canonical trigger triple (`'delete'` form on delete and update) and `INSERT INTO t(t) VALUES('rebuild')`. `ext/fts5/test/fts5integrity.test`: `('integrity-check', 0)` checks B-trees only; `('integrity-check', 1)` also checks 1:1 against the content table. Our exact table should be checked with rank=1.
- **D10 settle window** — elastic/beats `filebeat/input/log/config.go`: `CloseInactive: 5 * time.Minute`, timer starts at EOF. facebook/watchman `watchman/cmds/trigger.cpp` and `.watchmanconfig`: `settle` default 20 ms. Our 60 s sits between a build tool and a log shipper; it is policy, and these are the two reference points.

## Addition 2026-09-05, scout RubyHeron (message 379), verified by BoldIsland by fetch

- **D4/D5 sort overrides fusion** — meilisearch/meilisearch `crates/milli/src/search/hybrid.rs` L51–57 and L163–176 @1d8619825c3b1871398c539cb566f2de38564493 (MIT): in hybrid (keyword+vector) search, `compare_scores` returns the `ScoreValue::Sort` ordering before any relevance comparison, so an explicit sort pre-empts fusion. Source for RawClaw's rule "newest/oldest skip RRF and keep the SQL ORDER BY". Corroborated by clickclack `search_pages.go` L56–75 (rank expression only under SortRelevance).

## Addition 2026-09-05, N4 freshness RFC (thread fleet-doctrine 400/402), verified by BoldIsland by fetch

- **Correction to WhiteGorge 400 #5.** `elastic/beats filebeat/input/log/harvester.go` @3b91674eedf9ecb1cb3bfece3fb65c4857f8a955: L120–175 contain no offset logic (only `h.state.TTL = h.config.CleanInactive`). The byte-offset watermark is L352–354 (`state := h.getState(); startingOffset := state.Offset; state.Offset += int64(message.Bytes)`) and L210/L365 (`readOffset.Set(state.Offset)`). Cite those, not 120–175.
- **Stale-serve + background refresh, one file (RFC #11 + #20).** `navidrome/navidrome core/external/provider.go` @a7365e119b4b2fdc3debbff7d8d801c3417f2824: L138–142 serve the cached row and, if expired, `e.albumQueue.enqueue(&album)` instead of blocking; L306–309 `RefreshInfo ... is synchronous: callers that must not block are responsible for detaching it` with `context.WithTimeout(ctx, refreshTimeout)`; L702–720 `newRefreshQueue` = buffered channel + one goroutine + per-item `WithTimeout`. Maps onto `refreshActiveSessions`: answer from the cold index, enqueue the active session for ingest, bound the ingest by a deadline. Constants there are `refreshTimeout = 15s` (L29); ours would be a `// policy:` label with a measurement.
