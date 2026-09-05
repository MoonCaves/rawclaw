# Prior-art steal list (2026-09-05)

Five Sourcegraph scouts, one per problem area, asked for units we can lift whole.
Licenses below were read from each repo's LICENSE file unless marked "unread".
Verdicts: **copy** = paste with attribution · **port** = rewrite in Go / trim · **idea** = pattern only.

## Copy verbatim (MIT, Go, self-contained)

| # | Unit | Repo · path · lines | Size | Replaces in RawClaw |
|---|------|---------------------|------|---------------------|
| 1 | Twin FTS5 tables (porter + unstemmed) over one content table, external-content triggers using the `'delete'` command | neilberkman/ccrider · `internal/core/db/schema.go` 86–122 | ~40 | second table beside `messages_fts` in `internal/store/store.go` |
| 2 | `escapeFTS5Query`: per-token quoting, keeps user phrases and trailing `*`, doubles inner quotes; has `search_test.go` | neilberkman/ccrider · `internal/core/search/search.go` 356–425 | ~70 | `query.SanitizeFTS5Query` |
| 3 | `sanitizeFTSQuery` + `snippet()` + `ORDER BY bm25(), rowid DESC` tie-break | openclaw/wacli · `internal/store/search.go` 72–107 | ~50 | cross-check for #2; tie-break for `store.SearchHits` |
| 4 | Rotation / truncation / growth detector (`os.SameFile`, size vs prev, mtime) | hpcloud/tail · `watch/polling.go` | ~120 | watermark drift logic in `internal/index` |
| 5 | Coalescing notify (`sendOnlyIfEmpty` on buffered chan) | hpcloud/tail · `watch/filechanges.go` | 37 | settle-window wake-up glue |
| 6 | Generic RRF over N ranked lists, `k` param | yoanbernabeu/grepai · `search/hybrid.go` | ~35 | `semantic.Fuse` merge step; also exact+stemmed merge |
| 7 | Retry policy: backoff + jitter, `IsRetryable(429/5xx)` | yoanbernabeu/grepai · `embedder/retry.go` | 99 | embed client retries |
| 8 | 4×-unrolled float64-accumulating cosine over `[]float32` | offgrid-llm · `internal/rag/sqlite_store.go` ~430–480 (license unread) | ~30 | current cosine in `internal/semantic` |
| 9 | `AddJSONFlags(cmd, &exporter, fields)` → `--json/--jq/--template`; `go-gh/pkg/jq` and `pkg/template` importable | cli/cli · `pkg/cmdutil/json_flags.go` 1–90 (MIT) | ~250 | hand-rolled `--json`/`--format` handling |
| 10 | `IsStdoutTTY()` override-then-isatty, cached | cli/cli · `pkg/iostreams/iostreams.go` 180–190 | ~10 | `machineStream()` |

## Port (right idea, trim or translate)

| # | Unit | Repo · path | Note |
|---|------|-------------|------|
| 11 | Code-aware tokenizer `unicode61 tokenchars '-_./:@#$%\'` + `detect_search_mode` (route code-shaped queries to exact table) | Dicklesworthstone/coding_agent_session_search · `tests/pages_fts.rs` 172, `src/pages/fts.rs` 84–128 | Rust; ~40 lines to port; local checkout exists |
| 12 | Watermark record shape: id, path, offset, mtime, OS identity (+ our fingerprint) | elastic/beats · `filebeat/input/file/state.go` (Apache-2.0) | shape only; drop TTL/Meta |
| 13 | Lease-locked `embedding_jobs` backfill queue (`locked_at`, `attempts`, `last_error`) with skip-fresh-locks test | openclaw/discrawl · `internal/store/store.go`, `store_write_test.go` | vector coverage bookkeeping |
| 14 | Ollama / OpenAI-compatible embed clients with batching, reorder by index, adaptive 429 handling | grepai · `embedder/ollama.go`, `embedder/openai.go` | take batching + reorder, trim the rest |
| 15 | Diagnostic schema: one JSON object per line, `code`, `severity`, `location`, `message`; text trailer `✖ N problems` | dominikh/go-tools · formatters doc + `lintcmd/lint.go` 369–398 | add a `fix` field with the exact command |
| 16 | Frecency: per-access `exp(-k·days)` summed | zzet/gortex · `internal/mcp/frecency.go` 90–140 (license unread) | browse ordering, later |

## Idea only

- calibre `resources/fts_sqlite.sql` — exact + stemmed pair in production for years; **GPL-3, do not copy text**.
- kubectl printers — "wide" columns pattern; header once per stream.
- promtail changelog — write-temp-then-rename for the watermark file.
- ripgrep `--json` — begin/match/context/end/summary record types.
- fsnotify kqueue — no distinct truncate event on macOS; detect by size, not events.
- comet `fusion.go` — explicit ascending/descending per list; skip its O(n²) rank step.

## Recommended first lift, in order

1. #1 + #11 tokenchars: second FTS5 table, unstemmed, code-aware. Additive migration, same triggers.
2. #2: replace the sanitizer, bring its tests.
3. #6: fuse exact-table and stemmed-table hit lists with RRF (k=60). Exact hits get credit from both lists and float up. Fall back to route-by-shape (#11) if fusion measures worse.
4. #9 + #10: `--jq`/`--template` via `go-gh`, one exporter helper.
5. #4: wrap watermark drift in tail's detector; #13 for vector backfill only if the vector tier is kept.

## Caveats

- Embedding path reality (fleet memory, verified 2026-06-21): `RAWCLAW_EMBED_ENDPOINT` "nomic-embed-text" routes through LiteLLM to Voyage-4-large at 1024 dims. There is no local model in the path; that is the 2.2 s per query measured on 2026-09-04. A truly local embedder means a separate vector space from the fleet's.
- The five scouts read code via Sourcegraph, not by running it. Every "copy" above still needs a build and the existing gate before it lands.
- Scout claims not verified: Loki `positions.go` no longer resolvable; Continue's FTS file not found this pass; xoai/sage-wiki and chromem-go unread.
