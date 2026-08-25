# Adversarial Review of Fixes Merged on 2026-08-25

**Date:** 2026-08-25  
**Reviewer:** Antigravity (Adversarial Audit Team)  
**Session Tag:** `20260825-adversarial`  
**Scope:** Commits merged to `main` within the 2026-08-25 sprint cycle.

---

## 1. Executive Summary

During today's sprint, multiple agent-written fixes landed rapidly to address security vulnerabilities, concurrency hazards, database error handling, and ranking/consolidation edge cases. All commits passed baseline CI checks (`go test -race ./...` across 27 packages).

However, an adversarial review reveals that while several fixes (such as the source registry mutex) successfully eliminate races, **multiple "fixes" introduced secondary regressions, left critical edge cases unhandled, or masked underlying failures with fallback behaviors**:

1. **Goose Adapter `rows.Err()` Check (`56d3dec`):** In `discoverDatabaseContainers`, checking `err := rows.Err(); err == nil && len(res) > 0` causes any iteration failure (`rows.Err() != nil`) or empty `sessions` table (`len(res) == 0`) on a multi-session database to fall through to the standalone single-session fallback. This creates a bogus ghost session with ID `"sessions"` and reports `ok = true`, masking database corruption and bypassing skipped-container counting (`bad++`).
2. **Consolidated Store Session Purge & Deletion Propagation (`bd6cda9`):** `mergeSessionsSQL` explicitly filters with `WHERE session_id IN (SELECT id FROM src.sessions)`. If a session is deleted or purged from a source database, the source contribution is removed from `session_sources`, but `mergeSessionsSQL` completely ignores the deleted session. As a result, deleted sessions are never re-evaluated or deleted from `main.sessions` and remain orphaned forever. Furthermore, if all surviving copies are purged, `main.sessions` never updates `missing_since` and falsely reports the session as live.
3. **Topics Search Violates "Real Tag Beats Routine" Invariant (`3bea613` / `a9b2e71`):** `topicsFromStore` and `topicsByFanOut` invoke `store.RoutineVerdictSet(con)` instead of `store.RoutineSet(con)`. `RoutineVerdictSet` does not inspect `topic_segment`. Because every hit in topic search has a real topic tag, the core system invariant (*"A real tag beats routine"*) requires that tagged sessions are not routine. Instead, `topics` demotes tagged sessions if they possess an overridden routine verdict, directly contradicting keyword FTS search on the exact same session.
4. **`SessionHasRealSegments` vs `RoutineSet` Divergence (`2e20a52`):** `SessionHasRealSegments` was fixed to propagate non-missing-table errors, but companion batch function `RoutineSet(con)` (`internal/store/verdict.go:145`) continues to swallow all query errors into `nil`. If `session_verdict` exists but `topic_segment` does not yet exist, `IsEffectivelyRoutine` returns `true` while `RoutineSet` returns an empty map (`false` for all sessions).
5. **Goose Dynamic Identifier Sanitizer Edge Cases (`0501803`):** `isSafeIdent` permits purely numeric column identifiers (e.g. `"123"`) and SQL reserved words (e.g. `group`, `order`). In unquoted SQL query generation (`SELECT 1 FROM sessions`), SQLite treats `"1"` as literal integer `1` instead of column `1`, producing corrupted constant session IDs. Furthermore, `file:%s` URI string formatting in `sql.Open` leaves database filepaths containing `?` or `#` vulnerable to URI query injection.
6. **Session Catalog Write Path Path Traversal (`13ddc1a`):** `WriteCatalogEntry` checks for path separators (`/`), but does not reject `"."` or `".."`, which contain no separators but cause `filepath.Join(catalogDir, "..")` to point to the parent directory.
7. **Semantic Worker Cancellation Limits (`5a72e87`):** Wrapping `ctx` with `context.WithCancel` safely unblocks worker channel sends on DB upsert failures, but because `embed.Embedder` lacks `context.Context` propagation, in-flight HTTP requests continue running against the remote API for up to 60 seconds.

---

## 2. Detailed Adversarial Breakdowns

---

### Finding 1: Goose Adapter `rows.Err()` Fallthrough Creates Ghost Sessions and Masks Corruption

- **Target File & Lines:** `internal/source/goose/goose.go:244-301`
- **Commits Reviewed:** `56d3dec`, `51acdf7`
- **Severity:** High (Data Integrity & Error Suppression)

#### Mechanism & Intent
Commit `56d3dec` aimed to prevent silent truncation by checking `rows.Err()` after scanning the `sessions` table in `discoverDatabaseContainers`:
```go
if err := rows.Err(); err == nil && len(res) > 0 {
    return res, true
}
```

#### Adversarial Failure Scenario
When `sessionsTableExists == 1`, the database is identified as a multi-session database. However, if:
1. `rows.Err() != nil` (e.g. SQLite database corruption, I/O read failure during row scan), OR
2. `len(res) == 0` (a valid multi-session database initialized with 0 sessions, or where all sessions have been cleared),

the `if err == nil && len(res) > 0` condition evaluates to `false`.

Instead of aborting with `return nil, false` (which would trigger `bad++` in `Discover()` to warn the user about an unreadable database), execution **falls through** to line 252:
```go
// Standalone single-session database fallback
defaultID := strings.TrimSuffix(filepath.Base(dbPath), ".db") // e.g. "sessions"
...
return []source.Container{{
    ID:         defaultID,
    Path:       dbPath,
    CWD:        cwd,
    IsSubagent: isSub,
    ParentID:   parentID,
    ResumeArgv: []string{"goose", "session", "--resume", "--session-id", defaultID},
}}, true
```

#### Concrete Impact
1. **Empty Multi-Session DB:** If a Goose database `sessions.db` has 0 rows in `sessions`, RawClaw returns a container with session ID `"sessions"`. During indexing, `Messages()` queries `WHERE session_id = 'sessions'`, finding 0 messages and creating an empty ghost session in the index.
2. **Corrupted Multi-Session DB:** If `sessions` table iteration fails midway due to corruption, the error is swallowed, `Discover()` receives `ok = true`, `bad` is not incremented, and RawClaw returns `ID: "sessions"`, completely discarding the partial sessions already read and hiding the database corruption.

#### What the Test Missed
`TestGooseMessages_RowsErr_TruncationError` in `goose_test.go` only tested `Adapter.Messages()`. It did not test `discoverDatabaseContainers` with an empty or corrupt `sessions` table.

#### Remediation
If `sessionsTableExists == 1`:
- If `err := rows.Err(); err != nil`: return `nil, false` (incrementing `bad++`).
- If `len(res) == 0`: return `nil, true` (valid multi-session database with 0 sessions).
- Do not fall through to the single-session fallback when a `sessions` table exists.

---

### Finding 2: Consolidated Store Session Purge/Deletion Leaves Orphaned Ghost Sessions

- **Target File & Lines:** `internal/index/consolidated.go:90-137, 567-575`
- **Commits Reviewed:** `bd6cda9`, `5b4ecfc`
- **Severity:** High (Data Integrity & Stale Search Results)

#### Mechanism & Intent
Commit `bd6cda9` introduced `session_sources` to track per-source contributions and accurately propagate `missing_since` timestamps when contributing sources are purged.

#### Adversarial Failure Scenario
In `consolidateOne`:
```go
srcBase := filepath.Base(src)
_ = con.Exec(recordSessionSourcesSQL, srcBase)
_ = con.Exec("DELETE FROM main.session_sources WHERE source_db = ? AND session_id NOT IN (SELECT id FROM src.sessions)", srcBase)
_ = con.Exec(mergeSessionsSQL)
```
And in `mergeSessionsSQL`:
```sql
WITH ranked AS (
  ...
  FROM main.session_sources
  WHERE session_id IN (SELECT id FROM src.sessions)
),
agg AS (
  ...
  FROM main.session_sources
  WHERE session_id IN (SELECT id FROM src.sessions)
  GROUP BY session_id
)
INSERT INTO main.sessions (...)
SELECT ... FROM agg a JOIN ranked r ...
ON CONFLICT(id) DO UPDATE SET ...
```

Notice the predicate: `WHERE session_id IN (SELECT id FROM src.sessions)`.

**Scenario A: Session Deleted From Its Only Source DB**
1. Session `S1` is indexed from `project-a.db`. `session_sources` records `(S1, project-a.db)`. `main.sessions` records `S1` with `missing_since = NULL`.
2. Session `S1` is deleted from disk and pruned from `project-a.db` (e.g. via `rawclaw delete S1 --files` or retention pruning).
3. `SyncConsolidatedFrom("project-a.db")` runs:
   - `DELETE FROM main.session_sources WHERE source_db = 'project-a.db' AND session_id NOT IN (SELECT id FROM src.sessions)` executes and deletes `(S1, project-a.db)` from `session_sources`.
   - `session_sources` now has **zero** records for `S1`.
   - `mergeSessionsSQL` runs ONLY for `WHERE session_id IN (SELECT id FROM src.sessions)`. Because `S1` is not in `src.sessions`, `mergeSessionsSQL` **does not execute for `S1`**.
   - `main.sessions` is never updated or pruned. `S1` remains in `main.sessions` indefinitely as an active, live session.

**Scenario B: Multi-Source Session Where Live Source is Deleted While Purged Source Remains**
1. Session `S2` exists in `p1.db` (purged at $t=1000$) and `p2.db` (live, `missing_since = NULL`).
2. `main.sessions` holds `missing_since = NULL` (since `p2.db` is live).
3. `p2.db` is deleted/pruned from disk so `S2` is completely absent from `p2.db`.
4. `SyncConsolidatedFrom("p2.db")` runs:
   - `session_sources` deletes `(S2, p2.db)`.
   - `session_sources` now contains only `(S2, p1.db, missing_since=1000)`.
   - Because `S2` is not in `p2.db.sessions`, `mergeSessionsSQL` skips `S2`.
   - `main.sessions` **never updates `missing_since`** to `1000`. It remains `missing_since = NULL`, falsely reporting the session as live.

**Scenario C: Source DB Basename Collisions**
`srcBase := filepath.Base(src)` uses only the filename. If two cache databases share a filename across directories (e.g. `/cache/a/sessions.db` and `/cache/b/sessions.db`), the second synchronization wipes out the `session_sources` rows of the first.

#### What the Test Missed
`TestConsolidate_MergedSessionHonestPerContributionSemantics` tested updating `missing_since` in place in `src.sessions`, but never tested removing or deleting a session from `src.sessions`.

#### Remediation
1. Recompute `mergeSessionsSQL` for all affected session IDs (both present in `src.sessions` and deleted from `session_sources` during this pass).
2. Delete from `main.sessions` any session whose `session_sources` count has dropped to zero:
   ```sql
   DELETE FROM main.sessions WHERE id NOT IN (SELECT session_id FROM main.session_sources);
   ```

---

### Finding 3: Topic Search Violates "A Real Tag Beats Routine" Invariant

- **Target File & Lines:** `internal/agentproto/agentproto.go:2461, 2552`
- **Commits Reviewed:** `3bea613`, `856561b`, `a9b2e71`
- **Severity:** Medium (Contract Inconsistency & Ranking Inversion)

#### Mechanism & Intent
PR #122 established the routine sort tier: routine sessions sort below equal-relevance normal hits across all search surfaces. The system invariant documented in `internal/store/verdict.go:117` specifies:
> *"IsEffectivelyRoutine resolves the cross-kind rule at read time: a session is effectively routine iff it carries a `routine` verdict AND has no real topic segment. 'A real tag beats routine' — someone bothered to tag it, so it is not noise."*

#### Adversarial Failure Scenario
In `agentproto.go`:
- `searchByStore` and `searchByFanOut` (keyword FTS) call `store.RoutineSet(con)`. `RoutineSet` excludes sessions that have real topic segments in `topic_segment`.
- `topicsFromStore` (line 2461) and `topicsByFanOut` (line 2552) call `store.RoutineVerdictSet(con)`. `RoutineVerdictSet` reads raw `session_verdict` without checking `topic_segment`.

Every result returned by `Topics()` matched a row in `topic_segment`, meaning **every hit has a real topic tag**. Under the system contract, none of these sessions are effectively routine.

However, because `Topics()` queries `RoutineVerdictSet`:
1. If a session has a topic tag AND a routine verdict (e.g. tagged as routine first, then tagged with a topic segment), `Topics()` marks `Routine: true` and demotes it below other topic hits.
2. In keyword FTS search (`searchScored`), that exact same session is evaluated using `RoutineSet()` and marked `Routine: false`.
3. In `IsEffectivelyRoutine()`, that session returns `false`.

#### Concrete Impact
Search surfaces disagree on whether a session is routine:
- `rawclaw search "tagging"` -> `routine: false`
- `rawclaw topics "tagging"` -> `routine: true` (demoted)

#### What the Test Missed
`TestRoutine_Topics_SortsDownAtEqualRelevance` in `agentproto_routine_test.go` specifically tagged sessions with topics and explicitly marked one with `VerdictRoutine`. The test passed only because `topicsFromStore` used `RoutineVerdictSet` to bypass the "real tag beats routine" check, enshrining the contradiction in the test suite.

#### Remediation
Replace `store.RoutineVerdictSet(con)` in `topicsFromStore` and `topicsByFanOut` with `store.RoutineSet(con)` (or remove routine demotion within `Topics()`, where all results are by definition real topic tags).

---

### Finding 4: Error Handling Divergence Between `SessionHasRealSegments` and `RoutineSet`

- **Target File & Lines:** `internal/store/topics.go:121-128`, `internal/store/verdict.go:137-158`
- **Commits Reviewed:** `2e20a52`, `ff4f488`
- **Severity:** Medium (Error Suppression & Logic Inconsistency)

#### Mechanism & Intent
Commit `2e20a52` updated `SessionHasRealSegments` to differentiate `"no such table"` from real SQLite query errors so database failures are propagated rather than swallowed as false negatives.

#### Adversarial Failure Scenario
While `SessionHasRealSegments` was fixed, its companion function `RoutineSet(con)` in `internal/store/verdict.go:137-158` was left untouched:
```go
func RoutineSet(con *sql.DB) (map[string]bool, error) {
    rows, err := con.Query(`
SELECT session_id FROM session_verdict
WHERE verdict = ?
  AND session_id NOT IN (
    SELECT DISTINCT session_id FROM topic_segment
    WHERE topic IS NOT NULL AND topic <> ''
  )`, VerdictRoutine)
    if err != nil {
        return map[string]bool{}, nil
    }
    ...
```

1. **Swallowed Database Errors:** If `con.Query` fails due to disk corruption, lock timeout, or I/O failure, `RoutineSet` swallows the error at line 145 and returns `map[string]bool{}, nil`.
2. **Behavioral Inconsistency on Missing `topic_segment` Table:**
   - On a database where `session_verdict` exists with routine entries but `topic_segment` has not yet been created:
     - `IsEffectivelyRoutine(con, sid)` calls `VerdictFor` (finds routine verdict), then calls `SessionHasRealSegments(con, sid)` (catches `"no such table"`, returns `false, nil`), and concludes `!real == true`. Result: **Routine (`true`)**.
     - `RoutineSet(con)` executes the single SQL statement with `FROM topic_segment`. SQLite returns error: `no such table: topic_segment`. Line 145 catches `err != nil` and returns `map[string]bool{}, nil`. Result: **Not Routine (`false`)**.

#### Remediation
Update `RoutineSet` to inspect error strings or verify table existence, and propagate unexpected database errors rather than returning an empty map with `nil` error.

---

### Finding 5: Goose Dynamic Identifier Sanitizer Incomplete on Numbers, Keywords, and URIs

- **Target File & Lines:** `internal/source/goose/goose.go:184, 219, 310, 349, 448-466`
- **Commits Reviewed:** `0501803`, `8f0c40b`
- **Severity:** Medium (Query Failure & Silent Misattribution)

#### Mechanism & Intent
Commit `0501803` added `isSafeIdent(s)` (`[a-zA-Z0-9_]+`) to sanitize dynamic table and column identifiers interpolated into SQL queries in the Goose adapter.

#### Adversarial Failure Scenario
1. **Numeric Column Identifiers:** `isSafeIdent("123")` and `isSafeIdent("0")` return `true`. When a column is named `1`, `SELECT 1 FROM sessions` does not select column `1`; SQLite evaluates literal integer `1`. Every row scanned assigns `sID = "1"`, collapsing all sessions into session ID `"1"`.
2. **Unquoted SQL Keywords:** Standard SQL keywords (`order`, `group`, `where`, `index`, `limit`, `case`) return `true` from `isSafeIdent`. When interpolated unquoted into `PRAGMA table_info(order)` or `SELECT order FROM messages`, SQLite returns syntax errors.
3. **SQLite URI Metacharacter Injection in `sql.Open`:**
   Lines 184 and 310 open connections via:
   ```go
   sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)&_pragma=mmap_size(%d)", dbPath, store.ROMmapSize))
   ```
   `file:%s` activates SQLite URI parsing. If `dbPath` contains `?` (e.g. `/home/user/project?1/sessions.db`) or `#`, SQLite splits the path at `?` and fails to open the database file.

#### Remediation
- Quote column and table identifiers with double quotes in SQL statements (`SELECT "%s" FROM "%s"`).
- URL-encode `dbPath` when constructing `file:` URIs for SQLite connections.

---

### Finding 6: Session Catalog `WriteCatalogEntry` Incomplete Path Traversal Check

- **Target File & Lines:** `internal/paths/paths.go:99-125`
- **Commits Reviewed:** `13ddc1a`
- **Severity:** Low (Filesystem Traversal Edge Case)

#### Mechanism & Intent
Commit `13ddc1a` added validation to `WriteCatalogEntry` to prevent directory traversal:
```go
if entry.SessionID == "" || strings.ContainsRune(entry.SessionID, os.PathSeparator) || strings.ContainsRune(entry.SessionID, '/') {
    return fmt.Errorf("invalid session id: %q", entry.SessionID)
}
```

#### Adversarial Failure Scenario
The validation checks for path separators (`/`), but does not check for `"."` or `".."`:
- If `entry.SessionID == ".."`, it contains no slashes.
- `target := filepath.Join(catalogDir, "..")` resolves to `filepath.Dir(catalogDir)` (the parent directory).
- `tmp := filepath.Join(catalogDir, ".tmp.....")` is written, and `os.Rename(tmp, target)` attempts to overwrite the parent directory.

#### Remediation
Validate `entry.SessionID` against `filepath.Clean(entry.SessionID) != entry.SessionID` or enforce standard UUID/slug characters `[a-zA-Z0-9_-]+`.

---

### Finding 7: Semantic Index Goroutine Leak Fix Leaves In-Flight HTTP Uncancellable

- **Target File & Lines:** `internal/semantic/semantic.go:106-107, 276-281`
- **Commits Reviewed:** `5a72e87`, `35ae603`
- **Severity:** Low / Accepted Constraint (Resource Consumption on Error)

#### Mechanism & Intent
Commit `5a72e87` added `ctx, cancel := context.WithCancel(ctx); defer cancel()` in `VecIndex` so early returns unblock workers waiting on channel sends (`resultsChan <- resBatch`).

#### Adversarial Failure Scenario
The channel send `select { case resultsChan <- resBatch: case <-ctx.Done(): return }` properly frees workers that have already computed embeddings.

However, if a database failure triggers `cancel()` while workers are inside `bEmbedder.EmbedBatch(texts)`, the workers cannot be interrupted because `embed.Embedder` does not accept a `context.Context`. The goroutines remain blocked in network I/O until the HTTP timeout expires (default 60s).

#### Remediation
As noted in Go Standards Finding 8, extend the `embed.Embedder` interface to accept `context.Context` and pass it to `http.NewRequestWithContext`.

---

### Finding 8: Source Registry Mutex Synchronization

- **Target File & Lines:** `internal/source/source.go:55-103`
- **Commits Reviewed:** `b66fb17`, `471cff6`, `a41e285`, `f461adc`
- **Severity:** Verified Clean (Holds Up Under Scrutiny)

#### Adversarial Assessment
The synchronization design was tested against concurrent registrations, reads, and re-entrant callback invocations.
1. `registryMu sync.RWMutex` guards mutations (`Register`, `ResetForTesting`) and slice access (`Registered`).
2. `DetectID` takes an isolated shallow copy via `Registered()` before executing `r.Detect(path)`. Any re-entrant calls to `Register` or `ResetForTesting` from within caller callbacks acquire the lock without deadlocking.
3. Lock acquisition in `sources.Registered()` (`sources.mu` -> `source.registryMu`) maintains a strict unidirectional hierarchy.

**Verdict:** Holds up under adversarial review.

---

## 3. Summary Matrix of Reviewed Changes

| Area / Commit | Primary Risk | Status | Key Defect / Observation |
|---|---|---|---|
| **Goose `rows.Err()`** (`56d3dec`) | Data Corruption | **BROKEN** | `sessionsTableExists == 1` with 0 rows or read error falls through to fabricate a fake single-session container `"sessions"`. |
| **Purge Propagation** (`bd6cda9`) | Stale State | **BROKEN** | `mergeSessionsSQL` ignores deleted sessions (`WHERE session_id IN (SELECT id FROM src.sessions)`), leaving orphaned rows in `main.sessions`. |
| **Topics Sort Tier** (`3bea613`) | Contract Violation | **BROKEN** | `Topics()` queries `RoutineVerdictSet`, violating the *"real tag beats routine"* invariant by demoting tagged sessions. |
| **`SessionHasRealSegments`** (`2e20a52`) | Error Suppression | **PARTIAL** | Fixed single-session check, but `RoutineSet` still swallows errors as `nil` and contradicts `IsEffectivelyRoutine` when `topic_segment` is absent. |
| **Goose SQL Sanitizer** (`0501803`) | Query Failure | **PARTIAL** | `isSafeIdent` permits numeric column names and unquoted SQL keywords; `file:` URI opens lack path parameter escaping. |
| **Catalog Entry Path** (`13ddc1a`) | Directory Traversal | **PARTIAL** | Rejects `/` but permits `".."` and `"."`. |
| **Semantic Leak Fix** (`5a72e87`) | Resource Retention | **MITIGATED** | Channel deadlock resolved; in-flight HTTP requests remain bounded only by socket timeout (missing context in embedder interface). |
| **Source Registry Mutex** (`471cff6`) | Concurrency Race | **VERIFIED** | Clean snapshotting prevents re-entrancy deadlock; thread-safety verified under `-race`. |

---
