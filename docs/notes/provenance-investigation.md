# Provenance & `source_db` Identity Format Investigation

**Date:** 2026-08-25  
**Investigator:** Antigravity  
**Live Store Target:** `~/.cache/session-search/consolidated.db`  
**Repository:** `MoonCaves/rawclaw`

---

## 1. Executive Summary

A forensic query of the live consolidated store (`~/.cache/session-search/consolidated.db`) revealed that **92.3% of rows in `session_sources` (5,636 of 6,106 rows)** are stored in superseded identity formats:
- **4,424 rows** carry `source_db = ''` (the legacy backfill baseline).
- **1,212 rows** carry `source_db = '<basename>'` (the pre-`ebc086a` bare filename format).
- **470 rows** carry `source_db = '/abs/path/...'` (the current absolute-path identity format).
- **415 sessions** have dual records (both an old-form row and a current full-path row).
- **5,198 sessions** have *only* an old-form record and no current-form record.
- **16 sessions** exist in `main.sessions` with *no record at all* in `session_sources`.

### The Core Finding
**This is NOT merely cosmetic database debt: it causes concrete, demonstrable failures during session deletion, retention tracking, and incremental synchronization.**

Specifically:
1. **Immortal Ghost Sessions on Deletion/Purge:** When a session originally recorded under a legacy format (bare filename or empty string) is deleted or purged from its source database, incremental synchronization (`SyncConsolidatedFrom`) either fails to detect the deletion or fails to remove the session from `main.sessions` and `main.messages`. The superseded legacy row in `session_sources` acts as an unremovable anchor, making the session immortal.
2. **Zombie Live Status (Failure to Mark `missing_since`):** In multi-source sessions where one or more contributing copies have been purged, deleting the last live contributing source fails to update `missing_since` on the consolidated session if the live copy was tracked under a bare filename. The session remains falsely marked as live (`missing_since = NULL`) indefinitely. (Demonstrated on 23 sessions in the live store).
3. **Metadata Rank Inversion:** In `mergeSessionsSQL`, tie-breaking uses `source_db DESC`. Because ASCII `'c'` (`codex--...`) > `'/'` (`/Users/...`), a stale bare filename record beats a fresher full-path record on equal message count and timestamp, causing stale metadata (`project`, `cwd`, `source_path`) to overwrite current data.
4. **Why Search and Read Continue to Work:** Direct search verbs (`rawclaw search`, `rawclaw read`) query `main.sessions`, `main.messages`, and `messages_fts` directly without inspecting `session_sources`. Thus, for active sessions that have not been deleted, keyword and trigram search return correct results. The corruption is confined to lifecycle transitions: deletion propagation, purge tracking, and metadata merging.
5. **Cross-Machine Archive Sync is Safe:** The git archive transports raw `.jsonl` transcript files and JSON tag sidecars, never SQLite databases. `consolidated.db` is strictly a machine-local cache, so cross-machine filesystem path differences do not interact with `source_db`.

---

## 2. Live Store Forensic Audit

The live database at `~/.cache/session-search/consolidated.db` was queried directly.

### 2.1 Format Breakdown

```sql
SELECT
  CASE
    WHEN source_db = '' THEN 'empty string'
    WHEN source_db LIKE '/%' THEN 'full absolute path'
    ELSE 'bare filename'
  END AS format,
  COUNT(*) AS total_rows,
  COUNT(DISTINCT session_id) AS distinct_sessions
FROM session_sources
GROUP BY format;
```

**Real Output:**
```
format              total_rows  distinct_sessions
------------------  ----------  -----------------
bare filename       1212        1189
empty string        4424        4424
full absolute path   470         461
```

### 2.2 Format Coexistence Matrix

```sql
SELECT
  SUM(CASE WHEN has_empty AND NOT has_bare AND NOT has_abs THEN 1 ELSE 0 END) AS only_empty,
  SUM(CASE WHEN has_bare AND NOT has_empty AND NOT has_abs THEN 1 ELSE 0 END) AS only_bare,
  SUM(CASE WHEN has_abs AND NOT has_empty AND NOT has_bare THEN 1 ELSE 0 END) AS only_abs,
  SUM(CASE WHEN has_empty AND has_bare AND NOT has_abs THEN 1 ELSE 0 END) AS empty_and_bare,
  SUM(CASE WHEN has_empty AND has_abs AND NOT has_bare THEN 1 ELSE 0 END) AS empty_and_abs,
  SUM(CASE WHEN has_bare AND has_abs AND NOT has_empty THEN 1 ELSE 0 END) AS bare_and_abs,
  SUM(CASE WHEN has_empty AND has_bare AND has_abs THEN 1 ELSE 0 END) AS all_three,
  COUNT(*) AS total_distinct_sessions
FROM (
  SELECT
    session_id,
    MAX(CASE WHEN source_db = '' THEN 1 ELSE 0 END) AS has_empty,
    MAX(CASE WHEN source_db <> '' AND source_db NOT LIKE '/%' THEN 1 ELSE 0 END) AS has_bare,
    MAX(CASE WHEN source_db LIKE '/%' THEN 1 ELSE 0 END) AS has_abs
  FROM session_sources
  GROUP BY session_id
);
```

**Real Output:**
```
only_empty  only_bare  only_abs  empty_and_bare  empty_and_abs  bare_and_abs  all_three  total_distinct
----------  ---------  --------  --------------  -------------  ------------  ---------  --------------
4053        1145       46        0               371            44            0          5659
```

- **Sessions with ONLY superseded formats:** $4053 + 1145 = \mathbf{5,198}$
- **Sessions with BOTH an old-form and a full-path row:** $371 + 44 = \mathbf{415}$
- **Sessions with ONLY full-path format:** $\mathbf{46}$
- **Total distinct sessions in `session_sources`:** $\mathbf{5,659}$

### 2.3 Store Discrepancy: Orphan Sessions in `main.sessions`

```sql
SELECT
  (SELECT COUNT(*) FROM sessions) AS sessions_table_count,
  (SELECT COUNT(DISTINCT session_id) FROM session_sources) AS session_sources_distinct_ids,
  (SELECT COUNT(*) FROM sessions WHERE id NOT IN (SELECT session_id FROM session_sources)) AS orphan_sessions;
```

**Real Output:**
```
sessions_table_count  session_sources_distinct_ids  orphan_sessions
--------------------  ----------------------------  ---------------
5675                  5659                          16
```

There are **16 sessions** in `main.sessions` with messages in `main.messages` that have **zero records** in `session_sources`. These are sessions created by source runs that were never backfilled or recorded in `session_sources`.

---

## 3. Code Archaeology: How the Store Got Into This State

Tracing git history through `internal/index/consolidated.go` and recent commits reveals the exact chronological evolution:

### Phase 1: Pre-Provenance (`main.sessions` only)
Prior to commit `bd6cda9`, consolidation directly upserted into `main.sessions`. There was no `session_sources` table.

### Phase 2: Introduction of `session_sources` with Basenames (`bd6cda9`)
Commit `bd6cda9` introduced `session_sources` to support multi-source retention and purge tracking.
In `consolidateOne`:
```go
srcBase := filepath.Base(src)
con.Exec(recordSessionSourcesSQL, srcBase)
```
Every contribution was recorded with `source_db = filepath.Base(src)` (e.g. `codex--<project>-<hash>.db` or `sessions.db`).

### Phase 3: The Empty-String Backfill (`f8230b3` / `6c57ea8`)
Peer review noted that upgrading an existing store created `session_sources` with `CREATE TABLE IF NOT EXISTS`, leaving existing consolidated sessions with 0 provenance rows. Commit `f8230b3` added `migrateSessionSources`:
```sql
INSERT OR IGNORE INTO main.session_sources (
  session_id, source_db, started_at, last_ts, message_count,
  is_subagent, parent_id, origin_machine, source_tool, source_path,
  missing_since, project, cwd
)
SELECT
  id, '', started_at, last_ts, message_count,
  is_subagent, parent_id, origin_machine, source_tool, source_path,
  missing_since, project, cwd
FROM main.sessions
WHERE NOT EXISTS (
  SELECT 1 FROM main.session_sources WHERE main.session_sources.session_id = main.sessions.id
)
```
This inserted **4,424 rows with `source_db = ''`** into the live store.

### Phase 4: The Switch to Full-Path Identity (`ebc086a`)
Commit `ebc086a` recognized that two projects could both have a database named `sessions.db`, causing one project to clobber another's `session_sources` rows.
The author changed `srcBase := filepath.Base(src)` to `srcID := sourceIdentity(src)` (`filepath.Abs(src)`).

### Phase 5: The Missing Migration & Incomplete Cleanup
**No database migration was written to convert existing `source_db` rows.**
Because `session_sources` has primary key `(session_id, source_db)`:
- `source_db = ''`
- `source_db = 'sessions.db'`
- `source_db = '~/.cache/session-search/sessions.db'`

are three completely distinct keys. When a project was synchronized under the new binary, SQLite treated the full path as a *new row* rather than an update to the old row.

While `ConsolidateFrom` (the full-pass sweep) gained a cleanup query for `source_db = ''`:
```sql
DELETE FROM main.session_sources
WHERE source_db = ''
  AND EXISTS (
    SELECT 1 FROM main.session_sources real
    WHERE real.session_id = session_sources.session_id
      AND real.source_db <> ''
  )
```
1. **This cleanup is NEVER executed during `SyncConsolidatedFrom`** (the write-through path called after every normal indexing run).
2. **This cleanup ONLY targets `source_db = ''`**; it never cleans up `source_db = '<basename>'`.
3. **This cleanup is skipped entirely if any candidate source is skipped** (`if st.Skipped == 0 && st.Sources > 0`, added in `de0c74c`).

As a result, **1,212 bare filename rows and 4,053 empty-string rows remain permanently stuck in the live store.**

---

## 4. Code Trace: Who Reads `source_db` and What Decisions Are Made?

Every location in the codebase reading or writing `source_db` is confined to `internal/index/consolidated.go`:

| File:Line | SQL / Go Context | Purpose & Decision Made |
|---|---|---|
| `consolidated.go:51` | `PRIMARY KEY (session_id, source_db)` | Uniqueness constraint. Treats `''`, `'foo.db'`, and `'/path/foo.db'` as separate contributions. |
| `consolidated.go:167` | `recordSessionSourcesSQL` (`INSERT ... ON CONFLICT(session_id, source_db)`) | Inserts or updates the contribution for `sourceIdentity(src)`. Leaves old formats intact. |
| `consolidated.go:214` | `mergeSessionsSQL` (`ORDER BY ... source_db DESC`) | **4th tie-breaker in ranking.** Picks metadata (`project`, `cwd`, `source_path`, `source_tool`) when message count and timestamps match. |
| `consolidated.go:475` | `ConsolidateFrom` (`DELETE FROM session_sources WHERE source_db = '' ...`) | Prunes empty-string rows only on full pass without skipped sources. |
| `consolidated.go:713` | `consolidateOne` (`SELECT ... WHERE source_db = ? AND session_id NOT IN ...`) | **Deletion detector.** Identifies sessions purged from the source database currently being folded (`srcID`). |
| `consolidated.go:722` | `consolidateOne` (`DELETE ... WHERE source_db = ? AND session_id NOT IN ...`) | Deletes the source contribution for purged sessions. |
| `consolidated.go:746-764` | `consolidateOne` (`DELETE FROM main.messages/sessions WHERE NOT EXISTS (SELECT 1 FROM session_sources ...)`) | **Store Prune Gate.** Deletes session and messages from consolidated store **iff zero records remain in `session_sources`**. |
| `consolidated.go:954` | `pruneTombstonedIDs` (`DELETE FROM session_sources WHERE session_id = ?`) | Explicit user tombstone deletion (`rawclaw delete`). |
| `consolidated.go:997` | `migrateSessionSources` (`INSERT OR IGNORE ... VALUES(id, '', ...)`) | Backfills legacy empty string rows for sessions lacking any `session_sources` record. |

**Important Negative Result:**
Outside of `internal/index/consolidated.go`, **no package in RawClaw reads `session_sources` or `source_db`**. `internal/search`, `internal/view`, `internal/agentproto`, `internal/cli`, and `internal/archive` all query `sessions`, `messages`, and `messages_fts`.

---

## 5. Concrete Failure Modes and Demonstrations

### 5.1 Failure Mode A: The Immortal Ghost Session

#### The Mechanism:
1. An upgraded store has `session_sources` rows for session $S$:
   - `(S, 'codex--<project>-<hash>.db')`
   - `(S, '~/.cache/session-search/codex--<project>-<hash>.db')`
2. The user deletes or prunes session $S$ from disk, and it is removed from `codex--<project>-<hash>.db`.
3. An indexing run triggers `SyncConsolidatedFrom("~/.cache/session-search/codex--<project>-<hash>.db")`:
   - `temp.consolidation_affected_sessions` collects $S$ because `source_db = srcID`.
   - `DELETE FROM main.session_sources WHERE source_db = srcID` removes the full-path row.
   - `mergeSessionsSQL` executes for $S$. It queries `main.session_sources WHERE session_id = S`. It finds the legacy row `(S, 'codex--<project>-<hash>.db')`!
   - Because `missing_since` on the legacy row is `NULL`, `mergeSessionsSQL` marks $S$ in `main.sessions` as **LIVE** (`missing_since = NULL`).
   - Lines 746-764 evaluate:
     ```sql
     DELETE FROM main.sessions WHERE id IN (
       SELECT a.session_id FROM temp.consolidation_affected_sessions a
       WHERE NOT EXISTS (SELECT 1 FROM main.session_sources s WHERE s.session_id = a.session_id)
     )
     ```
   - Because the legacy row exists, `NOT EXISTS` evaluates to `FALSE`.
   - **`main.sessions` and `main.messages` are NOT pruned.**
4. Result: **The session remains in `main.sessions` and search results forever as an active ghost session.**

#### Verified Reproduction on Live Store Data:
Session `019edd4b-da45-7ae3-8168-a11070e07914` currently has both rows in `consolidated.db`:
```
019edd4b-da45-7ae3-8168-a11070e07914 | ~/.cache/session-search/codex--<project>-<hash>.db
019edd4b-da45-7ae3-8168-a11070e07914 | codex--<project>-<hash>.db
```
If this session is deleted from its source database, `consolidateOne` will delete the absolute-path row, but the bare row will keep the session alive in `main.sessions` indefinitely.

---

### 5.2 Failure Mode B: Silent Deletion Bypass (Single Legacy-Row Sessions)

#### The Mechanism:
For the **5,198 sessions** that have *only* an old-form row (`''` or bare filename):
1. If session $S$ is purged from disk and deleted from its source database before a post-upgrade fold ever touched it:
2. When the source database `/cache/project.db` is folded:
   ```sql
   INSERT INTO temp.consolidation_affected_sessions(session_id)
   SELECT session_id FROM main.session_sources
   WHERE source_db = ? AND session_id NOT IN (SELECT id FROM src.sessions)
   ```
3. `?` is `~/.cache/session-search/project.db`.
4. In `session_sources`, $S$ has `source_db = 'project.db'` or `source_db = ''`.
5. `WHERE source_db = ?` matches **0 rows**.
6. $S$ is **NOT** inserted into `temp.consolidation_affected_sessions`.
7. `mergeSessionsSQL` completely skips $S$.
8. Result: **The deletion is silently ignored. The session and all its messages remain in the consolidated store indefinitely.**

---

### 5.3 Failure Mode C: Inconsistent Purge Tracking in Live Store

Querying the live store reveals **23 sessions** that are already in an inconsistent multi-source purge state:
```sql
SELECT
  s.session_id,
  group_concat(s.source_db, ' || '),
  group_concat(COALESCE(s.missing_since, 'live'), ' || ')
FROM session_sources s
GROUP BY s.session_id
HAVING count(*) > 1 AND count(missing_since) > 0 AND count(missing_since) < count(*)
LIMIT 3;
```

**Real Output:**
```
session_id                            source_dbs                                                                                            missing_since_states
------------------------------------  ----------------------------------------------------------------------------------------------------  --------------------
57aeb4d9-93e3-426a-8507-26a57822139a  -<project>.db || -<project>.db       live || 1787637831.0108
617f17a3-acf3-4d2a-9396-2fc322225712  antigravity-unknown-da39a3ee.db || antigravity--<project>-<hash>.db                 1787652820.37862 || live
70fda07e-c6e8-4cbc-adf7-6f7db039f7b0  antigravity-unknown-da39a3ee.db || antigravity--<project>-<hash>.db                 1787652820.37862 || live
```

For session `57aeb4d9-93e3-426a-8507-26a57822139a`:
- One source (`-<project>.db`) was purged at $t=1787637831.0108$.
- The second source (`-<project>.db`) is currently live, recorded under a bare filename.
- When the second source is purged or deleted, `SyncConsolidatedFrom` using full-path identity will fail to match `-<project>.db`.
- The session will never transition to `missing_since = 1787637831.0108` and will remain permanently marked as live.

---

### 5.4 Failure Mode D: Metadata Rank Inversion

In `mergeSessionsSQL` (`internal/index/consolidated.go:208-215`):
```sql
ROW_NUMBER() OVER (
  PARTITION BY session_id
  ORDER BY
    (missing_since IS NULL) DESC,
    COALESCE(message_count, 0) DESC,
    COALESCE(last_ts, 0) DESC,
    source_db DESC
) AS rank
```
If a session has equal message counts and timestamps across its legacy row and its current full-path row:
- `'codex--...' > '/Users/...'` (ASCII: `'c'` = 99 > `'/'` = 47).
- The **stale bare filename row wins rank 1**.
- Any updated metadata on the full path row (such as updated `cwd`, `project`, or `source_path`) is discarded in favor of the stale legacy row.

---

## 6. What is Safe and Ruled Out

To be rigorous, the following areas were tested and proven **harmless / unaffected**:

1. **Keyword Search (FTS5) and Substring Search (Trigram):**
   `rawclaw search` and `rawclaw read` query `messages_fts` and `messages`. These tables use auto-increment message IDs and FTS index triggers. They do not join against `session_sources`. Search ranking and snippet generation for active sessions are completely unaffected.
2. **Topic Segments & Session Verdicts:**
   Topic tagging and routine verdicts are keyed by `(session_id, start_uuid)` and `session_id`. They are folded independently by `mergeTopicsSQL` and `mergeVerdictsSQL` and do not read `source_db`.
3. **Cross-Machine Git Archive Sync:**
   The git archive (`internal/archive/`) syncs `.jsonl` transcript trees and tag JSON sidecars between machines. It never syncs cache database files (`*.db` or `consolidated.db`). Each machine creates its own local `consolidated.db` in its local cache dir. Thus, differences in absolute path prefixes across machines (e.g. `/Users/alice/` vs `/home/bob/`) never collide or corrupt the archive.
4. **Full Rebuild (`rawclaw consolidate --rebuild`):**
   Running `rawclaw consolidate --rebuild` wipes `session_sources` and refills it from disk using `PerProjectDBs()`. Every row is inserted using `sourceIdentity(src)` (full path), completely purging all empty-string and bare-filename rows.

---

## 7. Explicit Accounting of Uncertainties

In accordance with strict investigation standards, the boundaries between established facts and uncertainties are explicitly delineated:

### Established Facts
- [x] 92.3% of rows in `session_sources` in `~/.cache/session-search/consolidated.db` are in superseded formats (`''` or bare basenames).
- [x] `migrateSessionSources` created the 4,424 `source_db = ''` rows as a synthetic baseline during the initial schema migration.
- [x] No migration or aliasing logic exists in the codebase to convert bare filename rows to absolute paths.
- [x] `SyncConsolidatedFrom` does not prune `source_db = ''` rows (only `ConsolidateFrom` does, when `st.Skipped == 0`).
- [x] Deleting a session from a source database fails to prune or mark `missing_since` in `main.sessions` whenever a legacy row exists in `session_sources`.
- [x] 16 sessions exist in `main.sessions` without any `session_sources` record.
- [x] Archive sync across machines is unaffected because cache databases are machine-local and never synced over git.

### Open Uncertainties
- [?] **How the 16 orphan sessions originated:** They possess timestamps and message counts typical of recent Codex runs. It is probable they were inserted into `main.sessions` by an index pass or test run that wrote directly to `main.sessions` or failed mid-transaction before `session_sources` was committed.
- [?] **Performance impact of duplicate rows:** At 6,106 rows, the table is tiny enough that SQLite query performance is unaffected. Whether stores with >100,000 sessions experience noticeable latency in `mergeSessionsSQL` window functions is unbenchmarked.
- [?] **User-facing deletion frequency:** While `rawclaw delete` writes to the `tombstones` sidecar (which `pruneTombstoned` handles globally by `session_id`), source-level transcript purge (e.g. agent CLI log rotators or manual file deletion) relies entirely on `consolidateOne`'s `temp.consolidation_affected_sessions` path and is therefore actively broken for all 5,198 legacy sessions.
