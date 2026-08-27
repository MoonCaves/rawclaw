# Tick 29 SQLite prior-art findings

run_timestamp: 2026-08-27T00:02:57Z
prior_watermark: 20260826T232815Z
scope: bounded Who-not-How research for RawClaw foreground closeout latency, writer admission, tombstone cleanup, and busy handling; no implementation.

## Re-grade of prior recommendations

All three recommendations below were re-graded against the supplied watermark. No immutable external adoption receipt was found after `20260826T232815Z`; each remains `pending`, score `0`.

| Stable ID | Current status | Re-grade / duplicate ruling |
|---|---|---|
| `PA-SQLITE-BEGIN-IMMEDIATE-001` | pending, score 0 | Still a distinct writer-admission mechanism. No post-watermark external adopter or immutable green receipt. Not credited by this run. |
| `PA-FTS5-DELETEMERGE-001` | pending, score 0 | Still a distinct FTS5 segment/tombstone-compaction mechanism. No post-watermark external adopter or immutable green receipt. Not repeated under another label. |
| `PA-GO-SINGLEFLIGHT-FALLBACK-001` | pending, score 0 | Still an in-process fallback-trigger coalescing mechanism, not durable ownership/fencing. No post-watermark external adopter or immutable green receipt. Not credited by this run. |

## New mechanism: decouple WAL checkpoint work from foreground closeout

- stable ID: `PA-SQLITE-WAL-PASSIVE-CHECKPOINT-001`
- source 1: https://www.sqlite.org/pragma.html#pragma_wal_autocheckpoint — SQLite `PRAGMA wal_autocheckpoint`, Last-Modified 2026-08-24, accessed 2026-08-27. Exact rule: a checkpoint runs automatically whenever the WAL reaches `N` pages; setting `N` to zero or negative disables automatic checkpointing; automatic checkpoints are PASSIVE and the default interval is 1000 pages.
- source 2: https://www.sqlite.org/pragma.html#pragma_wal_checkpoint — SQLite `PRAGMA wal_checkpoint`, Last-Modified 2026-08-24, accessed 2026-08-27. Exact rule: `PRAGMA wal_checkpoint(PASSIVE)` checkpoints as many frames as possible without waiting for database readers or writers; its busy handler is never invoked. FULL instead blocks and invokes the busy handler.
- mature source: https://gitlab.com/cznic/sqlite/-/blob/v1.45.0/func_test.go — modernc/sqlite v1.45.0 test at commit `b8975b7dcf269f7c09929073c1feb64701066f41`, published 2026-02-07, accessed 2026-08-27. Exact invariant in `ExecContext` cancellation test: after a cancelled query, `PRAGMA wal_checkpoint` must succeed, proving rows/statements were not leaked in an unclosed state.
- inspected-source SHA-256: SQLite pragma page `b9bcb335ae818497f3fa05114a10492f64f35503f275da2264f2d5d436db3f5d`; modernc/sqlite `func_test.go` `ae1067fdde425026139572ce0e05d2014b3ff3b61cf87442f0edeef71e64ba38`.
- normalized text: `Use PRAGMA wal_autocheckpoint=0 to disable commit-triggered automatic PASSIVE checkpoints, then run bounded PRAGMA wal_checkpoint(PASSIVE) outside foreground closeout; PASSIVE never waits for readers or writers and reports incomplete progress for later retry.`
- normalized-text SHA-256: `86a2faf69f9e11c899eaa9e1c13672f8edb997900905416de6cb483e4b3fd2e8`
- exact mechanism/invariant to copy: disable implicit threshold-triggered checkpoints for the closeout connection; perform explicit bounded PASSIVE checkpoints in a separately budgeted maintenance phase; treat unfinished frames as deferred work, not success; require checkpoint success after cancellation before declaring a connection reusable.
- relevance: RawClaw already uses WAL and has observed closeout time spent after the logged merge. SQLite’s default automatic checkpoint can run during a committing write and consume foreground latency. PASSIVE avoids waiting on readers/writers and gives a bounded, retryable maintenance operation. This does not replace writer fencing, transaction admission, or FTS5 `deletemerge`; it addresses checkpoint scheduling and cancellation cleanup only.
- status: `pending`; no external adoption evidence found in the post-watermark corpus; score `0`.
- duplicate analysis: not `PA-SQLITE-BEGIN-IMMEDIATE-001` (no transaction admission); not `PA-FTS5-DELETEMERGE-001` (no FTS5 segment merge or tombstone eligibility); not `PA-GO-SINGLEFLIGHT-FALLBACK-001` (no request coalescing). The existing RawClaw `wal_autocheckpoint`/`wal_checkpoint` test references are local evidence, not external-adoption credit.
- adoption evidence: none. The modernc test is mature-source precedent, not an independent RawClaw adopter or immutable product receipt.
- score event: none; no score eligible without an immutable external adoption receipt and green gate.
- valid candidate watermark: `20260827T000257Z` (run completion UTC; candidate only, not advanced because no conforming receipt was processed).
- next lead: obtain an immutable, independently adopted green receipt showing explicit PASSIVE checkpoint scheduling reduces foreground closeout latency while reporting incomplete frames and preserving cancellation cleanup.

## Research boundary

One additional exact mechanism was sufficient and source-verified before the evidence freeze. No second mechanism was added. This report is research-only; no product files were changed.

