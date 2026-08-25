# Go Standards Audit Findings

**Session Tag:** `20260825-golang-audit`  
**Standard Library:** `samber/cc-skills-golang` (external Go standards skill set)  
**Target Codebase:** RawClaw — Go 1.24 CLI, pure-Go SQLite (`modernc.org/sqlite`), `CGO_ENABLED=0`, FTS5 search.

---

## Executive Summary

This audit evaluated the RawClaw codebase against the external Go standards library routed via `golang-how-to`:
- `golang-database`
- `golang-concurrency`
- `golang-safety`
- `golang-context`
- `golang-error-handling`
- `golang-testing`
- `golang-naming`
- `golang-code-style`

Where generic Go rules conflict with RawClaw's North Star invariants (`AGENTS.md`: pure Go static binary, zero runtime dependencies, no silent truncation/failure, rebuildable store from transcripts), the project invariants prevail.

Findings are ranked below in order of real-world operational impact:
1. **Correctness & Reliability** (Data truncation & silent failures)
2. **Security & Data Safety** (Unsanitized SQL interpolation)
3. **Concurrency & Resource Leaks** (Goroutine leaks & missing watchdog cancellation)
4. **Data Integrity & Atomicity** (Non-transactional multi-statement mutations)
5. **Testing & QA Gates** (Missing goroutine leak assertions)
6. **API Design & Context Flow** (Missing context propagation in adapter ports)
7. **Code Style & Naming** (Cosmetic improvements)

---

## Prioritized Findings

---

### Finding 1: Silent Message Truncation via Unchecked `rows.Err()` in Goose Adapter
- **Severity:** Correctness / Data Loss (Critical)
- **File & Line:** `internal/source/goose/goose.go:406` (also `internal/source/goose/goose.go:244`, `442`)
- **Skill & Rule:** `golang-database` (Rule 5)
  > *"Rows MUST be closed after iteration — `defer rows.Close()` immediately after `QueryContext` calls... `if err := rows.Err(); err != nil { return fmt.Errorf("iterating users: %w", err) }` (always check after iteration)"*  
  Also `golang-error-handling` (Rule 1):
  > *"Returned errors MUST always be checked — NEVER discard with `_`"*
- **Concrete Impact:**  
  In `(*Adapter).Messages(c source.Container)`, `rows, err := db.Query(query, args...)` scans messages in a `for rows.Next() { ... }` loop. When `rows.Next()` returns `false`, line 406 immediately returns `return messages, nil` without calling `rows.Err()`. If iteration halted early due to an I/O read failure, disk error, or SQLite database corruption mid-scan, RawClaw returns a truncated message slice with `nil` error. During index ingestion, this truncated session is written to the index and marked clean. This directly breaks RawClaw's invariant in `AGENTS.md`: *"No silent truncation, no silent failure... An agent must never mistake partial for complete."*
- **Smallest Fix:**
  ```go
  if err := rows.Err(); err != nil {
      return nil, fmt.Errorf("goose: iterate messages %s: %w", backingPath, err)
  }
  return messages, nil
  ```

---

### Finding 2: Goroutine Leak on DB Upsert Error During Concurrent Embedding
- **Severity:** Concurrency / Resource Leak (High)
- **File & Line:** `internal/semantic/semantic.go:273-277`, `303`
- **Skill & Rule:** `golang-concurrency` (Core Principle 1 & Mistake Table)
  > *"Every goroutine must have a clear exit — without a shutdown mechanism (context, done channel, WaitGroup), they leak and accumulate until the process crashes"*  
  > *"Fire-and-forget goroutine | Provide stop mechanism (context, done channel)"*  
  Also `golang-context` (Rule 5):
  > *"`cancel()` MUST be called on all control-flow paths for `WithCancel`/`WithTimeout`/`WithDeadline`"*
- **Concrete Impact:**  
  `TopUp` spins up `numWorkers` worker goroutines to compute batch embeddings. Workers send results to `resultsChan` (capacity `numWorkers`):
  ```go
  select {
  case resultsChan <- resBatch:
  case <-ctx.Done():
      return
  }
  ```
  And a tracking goroutine waits on `wg.Wait()` before closing `resultsChan`.  
  `TopUp` uses the caller's `ctx` directly without creating a local cancellable context. If database insertion fails at line 303 (`store.VecUpsert`), `TopUp` returns early with `return 0, fmt.Errorf("insert vector: %w", err)`. Because the parent `ctx` remains active/uncancelled, all worker goroutines that attempt to send subsequent batches to `resultsChan` block forever once `resultsChan` is full. The `wg.Wait()` goroutine also blocks forever.
- **Smallest Fix:**  
  Wrap `ctx` with a local `context.WithCancel` at the start of `TopUp`:
  ```go
  ctx, cancel := context.WithCancel(ctx)
  defer cancel()
  ```
  When `TopUp` returns on error or completion, `cancel()` executes, immediately unblocking all workers via `case <-ctx.Done():`, allowing `wg.Done()` and `wg.Wait()` to complete cleanly.

---

### Finding 3: SQL Injection Vulnerability via Unsanitized Column/Table String Formatting in Goose Adapter
- **Severity:** Security / Correctness (High)
- **File & Line:** `internal/source/goose/goose.go:218`, `268`, `351`, `364`, `422`
- **Skill & Rule:** `golang-database` (Rule 2 & Section "Parameterized Queries")
  > *"Queries MUST use parameterized placeholders — NEVER concatenate user input into SQL strings"*  
  > *"Never interpolate column names from user input. Use an allowlist"*  
  Also `golang-security` (SQL injection prevention)
- **Concrete Impact:**  
  In `internal/source/goose/goose.go`, table and column names detected from external third-party SQLite databases are interpolated directly into SQL queries:
  - Line 218: `query := fmt.Sprintf("SELECT %s FROM sessions", strings.Join(selectCols, ", "))`
  - Line 268: `db.Query("SELECT " + idCol + ", " + valCol + " FROM session_meta LIMIT 10")`
  - Line 351: `query += fmt.Sprintf(" WHERE %s = ?", sessionCol)`
  - Line 364: `query += fmt.Sprintf(" ORDER BY %s ASC", orderCol)`
  - Line 422: `db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))`  
  If a scanned Goose SQLite database contains crafted column or table names (e.g. containing quotes or SQL statements), RawClaw executes un-sanitized SQL during discovery or message extraction.
- **Smallest Fix:**  
  Validate all dynamically resolved table and column identifiers with an alphanumeric/identifier check before interpolation, or escape identifiers with SQLite bracket/double-quote delimiters:
  ```go
  func isSafeIdent(s string) bool {
      for _, r := range s {
          if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
              return false
          }
      }
      return len(s) > 0
  }
  ```

---

### Finding 4: Non-Atomic Multi-Statement Vault Rebuild
- **Severity:** Data Loss / Integrity (High)
- **File & Line:** `internal/index/rebuild.go:158-213`
- **Skill & Rule:** `golang-database` (Rule 7 & Summary)
  > *"Use transactions for multi-statement operations — wrap related writes in `BeginTxx`/`Commit`"*
- **Concrete Impact:**  
  In `restoreSession(con *sql.DB, v durable.Session, rows []reindexRow, ...)`, five distinct write operations are executed sequentially in auto-commit mode:
  1. `DELETE FROM messages WHERE session_id=?`
  2. `DELETE FROM sessions WHERE id=?`
  3. Iterative `INSERT INTO messages(...)`
  4. `INSERT OR REPLACE INTO sessions(...)`
  5. `INSERT OR REPLACE INTO file_index(...)`  
  If process termination, a kill signal, or an I/O error occurs after step 1/2 or mid-way through step 3, the database is left in a corrupted state: existing messages are permanently deleted while new messages or session metadata are only partially written. Furthermore, executing un-batched `con.Exec` calls in SQLite triggers an individual disk sync per message, severely impacting rebuild performance.
- **Smallest Fix:**  
  Wrap all operations of `restoreSession` in a single SQLite transaction:
  ```go
  tx, err := con.Begin()
  if err != nil {
      return fmt.Errorf("begin restore session %s: %w", v.ID, err)
  }
  defer tx.Rollback()
  // execute DELETEs and INSERTs using tx.Exec(...)
  if err := tx.Commit(); err != nil {
      return fmt.Errorf("commit restore session %s: %w", v.ID, err)
  }
  return nil
  ```

---

### Finding 5: Database Failure Swallowed as False-Negative in `SessionHasRealSegments`
- **Severity:** Correctness / Data Integrity (Medium)
- **File & Line:** `internal/store/topics.go:123-125`
- **Skill & Rule:** `golang-database` (Rule 4) & `golang-error-handling` (Rule 1)
  > *"`sql.ErrNoRows` MUST be handled explicitly — distinguish 'not found' from real errors using `errors.Is`"*  
  > *"Returned errors MUST always be checked — NEVER discard with `_`"*
- **Concrete Impact:**  
  In `SessionHasRealSegments(con *sql.DB, sessionID string) (bool, error)`:
  ```go
  err := con.QueryRow(
      "SELECT COUNT(*) FROM topic_segment WHERE session_id=? AND topic IS NOT NULL AND topic<>''",
      sessionID,
  ).Scan(&n)
  if err != nil {
      return false, nil // missing table / read error reads as "no real segments"
  }
  return n > 0, nil
  ```
  `SELECT COUNT(*)` always produces exactly 1 row (it never returns `sql.ErrNoRows`). Any non-nil `err` is a genuine SQLite failure (disk error, locked database, corrupt file, closed connection). Swallowing the error and returning `false, nil` misleads callers into concluding that the session has no real topic tags. Upstream logic that relies on the invariant *"a real tag beats routine"* will erroneously classify the session as routine and overwrite or discard topic verdicts.
- **Smallest Fix:**  
  Check whether `err` is a schema/table missing error specifically; otherwise return the real error:
  ```go
  if err != nil {
      if strings.Contains(err.Error(), "no such table") {
          return false, nil
      }
      return false, fmt.Errorf("check real segments for %s: %w", sessionID, err)
  }
  ```

---

### Finding 6: Uninitialized `IndexStatus` Enum Silently Masquerades as Fresh
- **Severity:** Correctness / API Clarity (Medium)
- **File & Line:** `internal/index/index.go:1071-1076`
- **Skill & Rule:** `golang-naming` (Section "Enum zero values")
  > *"Enum (iota) — type name prefix, zero-value = unknown: `StatusUnknown` at 0, `StatusReady`"*  
  > *"Enum zero values: Always place an explicit `Unknown`/`Invalid` sentinel at iota position 0. A `var s Status` silently becomes 0 — if that maps to a real state like `StatusReady`, code can behave as if a status was deliberately chosen when it wasn't."*
- **Concrete Impact:**  
  `IndexStatus` is defined as:
  ```go
  const (
      IndexFresh IndexStatus = iota // 0
      IndexStale                    // 1
  )
  ```
  In Go, uninitialized struct fields or zero-value variables of type `IndexStatus` default to `0` (`IndexFresh`). If an indexing routine fails to set the status or an uninitialized result is read, it falsely asserts that the index was freshly built and verified current. This undermines RawClaw's reporting requirement to distinguish between complete and cached/fallback states.
- **Smallest Fix:**
  ```go
  const (
      IndexStatusUnknown IndexStatus = iota
      IndexFresh
      IndexStale
  )
  ```

---

### Finding 7: Missing Goroutine Leak Detection in `internal/archive` Test Suite
- **Severity:** Testing / CI Reliability (Medium)
- **File & Line:** `internal/archive/` (missing `leak_test.go`)
- **Skill & Rule:** `golang-testing` (Rule 6) & `golang-concurrency` (Rule 9)
  > *"Packages with goroutines SHOULD use `goleak.VerifyTestMain` in `TestMain` to detect goroutine leaks"*
- **Concrete Impact:**  
  `internal/archive` executes multi-process file locking and background sync loops, spawning goroutines in tests (`lock_test.go:82, 139, 204`, `killsafe_test.go:414`). Every other concurrent package in RawClaw (`adapters`, `agentproto`, `cli`, `durable`, `index`, `live`, `semantic`) provides a `leak_test.go` with `goleak.VerifyTestMain(m)`. `internal/archive` lacks `leak_test.go`, allowing goroutine leaks in archive sync/lock routines to evade CI.
- **Smallest Fix:**  
  Add `internal/archive/leak_test.go`:
  ```go
  package archive

  import (
      "testing"
      "go.uber.org/goleak"
  )

  func TestMain(m *testing.M) {
      goleak.VerifyTestMain(m)
  }
  ```

---

### Finding 8: Loss of Context Cancellation in Embedding HTTP Adapters
- **Severity:** API Design / Resource Consumption (Medium)
- **File & Line:** `internal/adapters/adapters.go:148`, `234`
- **Skill & Rule:** `golang-context` (Rules 1, 2, 8)
  > *"The same context MUST be propagated through the entire request lifecycle: HTTP handler → service → DB → external APIs"*  
  > *"`ctx` MUST be the first parameter, named `ctx context.Context`"*  
  > *"NEVER create a new `context.Background()` in the middle of a request path"*
- **Concrete Impact:**  
  `EmbedBatch` and `post` construct standalone contexts with `context.WithTimeout(context.Background(), timeout)` because the `embed.Embedder` interface does not accept a `context.Context`. When the parent CLI watchdog timeout fires or a user issues Ctrl+C, in-flight HTTP embedding requests continue executing on the network for up to 60 seconds, wasting network sockets and upstream API quotas.
- **Smallest Fix:**  
  Extend `embed.Embedder` and `embed.BatchEmbedder` to accept `ctx context.Context` (or provide `EmbedWithContext` / `EmbedBatchWithContext`), and propagate the caller's context through to `http.NewRequestWithContext`.

---

### Finding 9: Long Parameter Lists Exceeding 4 Parameters
- **Severity:** Style / API Clarity (Cosmetic)
- **File & Line:**
  - `internal/agentproto/agentproto.go:1091` (`OutlineAndRender`: 6 parameters)
  - `internal/agentproto/agentproto.go:1128` (`searchByFanOut`: 6 parameters)
  - `internal/index/rebuild.go:158` (`restoreSession`: 6 parameters)
  - `internal/retention/retention.go:138` (`DecideRetention`: 6 consecutive boolean flags)
- **Skill & Rule:** `golang-code-style` (Section "Function Design")
  > *"Functions SHOULD have ≤4 parameters. Beyond that, use an options struct"*
- **Concrete Impact:**  
  Functions with 6 parameters, especially consecutive booleans like `DecideRetention(present, tombstoned, own, missingSet, mirror, replica bool)`, are prone to accidental transposition at call sites.
- **Smallest Fix:**  
  Group arguments into options structs (e.g. `type RetentionInputs struct { Present, Tombstoned, Own, MissingSet, Mirror, Replica bool }`).

---

### Finding 10: Unexported Boolean Struct Fields Missing `is`/`has`/`can` Prefix
- **Severity:** Style / Naming (Cosmetic)
- **File & Line:** `internal/agentproto/agentproto.go:537-539`
- **Skill & Rule:** `golang-naming` (Section "Boolean struct fields")
  > *"Boolean struct fields: Unexported boolean fields MUST use `is`/`has`/`can` prefix — `isConnected`, `hasPermission`, not bare `connected` or `permission`."*
- **Concrete Impact:**  
  `type sweepResult struct { hitCeiling bool; answered bool }` uses bare past participles instead of question predicates (`hasHitCeiling`, `isAnswered`).
- **Smallest Fix:**  
  Rename fields to `hasHitCeiling` and `isAnswered`.

---

### Finding 11: Formatting Independent Error with `%v` Instead of `errors.Join`
- **Severity:** Style / Error Handling (Cosmetic)
- **File & Line:** `internal/cli/cmd_upgrade.go:308`
- **Skill & Rule:** `golang-error-handling` (Rule 6)
  > *"SHOULD use `errors.Join` (Go 1.20+) to combine independent errors"*
- **Concrete Impact:**  
  `fmt.Errorf("github api failed (%v) and redirect fallback failed: %w", apiErr, ferr)` flattens `apiErr` to a formatted string, preventing callers from inspecting `apiErr` via `errors.Is` / `errors.As`.
- **Smallest Fix:**  
  Combine errors using `errors.Join`: `fmt.Errorf("upgrade failed: %w", errors.Join(apiErr, ferr))`.

---

### Finding 12: Uppercase Acronym at the Beginning of Error String
- **Severity:** Style / Naming (Cosmetic)
- **File & Line:** `internal/cli/cmd_upgrade.go:501`
- **Skill & Rule:** `golang-naming` (Section "Error strings are fully lowercase — including acronyms") & `golang-error-handling` (Rule 3)
  > *"Error strings are fully lowercase — including acronyms. Write `"invalid message id"` not `"invalid message ID"`, because error strings are often concatenated with other context (`fmt.Errorf("parsing token: %w", err)`) and mixed case looks wrong mid-sentence."*
- **Concrete Impact:**  
  `fmt.Errorf("GET %s: status %d", url, resp.StatusCode)` starts with uppercase `"GET"`, producing capitalized fragments when wrapped mid-chain downstream.
- **Smallest Fix:**  
  Change `"GET %s: status %d"` to `"get %s: status %d"`.

---

## Summary of Findings by Category

| # | File:Line | Category | Severity | Summary |
|---|---|---|---|---|
| 1 | `internal/source/goose/goose.go:406` | Database / Error Handling | Correctness | Missing `rows.Err()` check leads to silent transcript truncation |
| 2 | `internal/semantic/semantic.go:273` | Concurrency / Context | Resource Leak | Uncancelled context leaks worker goroutines on DB upsert failure |
| 3 | `internal/source/goose/goose.go:218` | Database / Security | Security | Unsanitized table/column name string interpolation into SQL |
| 4 | `internal/index/rebuild.go:158` | Database / Integrity | Data Loss | Non-transactional multi-statement restore leaves half-deleted DB on failure |
| 5 | `internal/store/topics.go:123` | Database / Error Handling | Correctness | Swallowing `QueryRow` error as `false, nil` discards real DB failures |
| 6 | `internal/index/index.go:1071` | Naming / Enums | Correctness | `IndexFresh` at `iota 0` causes uninitialized values to claim freshness |
| 7 | `internal/archive/leak_test.go` | Testing / Concurrency | QA Gate | Missing `goleak.VerifyTestMain` in `archive` package |
| 8 | `internal/adapters/adapters.go:148` | Context / API | Reliability | Missing `context.Context` propagation in HTTP embedder ports |
| 9 | `internal/agentproto/agentproto.go:1091` | Code Style | Cosmetic | Functions with >4 parameters (including 6 consecutive booleans) |
| 10 | `internal/agentproto/agentproto.go:537` | Naming | Cosmetic | Boolean struct fields missing `is`/`has`/`can` prefix |
| 11 | `internal/cli/cmd_upgrade.go:308` | Error Handling | Cosmetic | Flattening independent error with `%v` instead of `errors.Join` |
| 12 | `internal/cli/cmd_upgrade.go:501` | Naming / Error Handling | Cosmetic | Capitalized acronym `"GET"` at beginning of error string |
