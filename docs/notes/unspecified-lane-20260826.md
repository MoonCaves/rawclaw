# Unspecified Lane Audit Report: Blitz `ada503f..HEAD`

**Date:** 2026-08-26  
**Auditor:** Antigravity (Unspecified Lane — Independent Roam)  
**Branch:** `audit/unspecified-lane`  
**Diff Range:** `ada503f..HEAD` (231 commits: one-store consolidation, chunked tagging, tail ingest, freshness paths, answer-first reads, only_copy_since rename, Goose adapter)  

---

## 1. Executive Summary & Prioritized Findings Table

| # | Severity | Category | Finding | Target `file:line` |
|---|---|---|---|---|
| 1 | **HIGH** | Data Destruction / Invariant | `tag-write --routine` unconditionally nukes existing topic segments; floor verdict variable shadowing | `internal/cli/cmd_tag.go:524-552` |
| 2 | **HIGH** | Data Leak / Orphan State | Consolidated store purge deletes `messages` and `sessions` but leaves `topic_segment`, `session_verdict`, `file_index`, and `chunk_vec` orphaned | `internal/index/consolidated.go:741-764` |
| 3 | **MEDIUM** | False Staleness / Process Storm | `CheckIndexFreshness` on per-project database always returns not fresh, triggering background ingest on every fallback browse | `internal/cli/cli.go:1416`, `internal/index/consolidated.go:1111-1125` |
| 4 | **MEDIUM** | Contract Contradiction | `vaultContainer` writes lossy synthetic records on full ingest while `vaultContainerAll` writes verbatim byte copies on append | `internal/index/containers.go:525`, `internal/index/containers.go:546`, `internal/durable/durable.go:102-111` |
| 5 | **MEDIUM** | Inconsistent State / Rebuild | `RebuildFromTranscripts` on consolidated store leaves `session_sources` completely empty | `internal/index/rebuild.go:210-280`, `internal/cli/cmd_consolidate.go:92` |
| 6 | **MEDIUM** | Spurious Invalidation | `CheckProjectFreshness` matches projects on bare directory basename, falsely checking and invalidating unrelated projects with identical names | `internal/index/consolidated.go:1187-1196` |
| 7 | **MEDIUM** | Parser Role Mismatch | Goose adapter normalizes tool execution output (`"tool"`, `"tool_result"`) as role `"assistant"` instead of `"tool"` | `internal/source/goose/goose.go:542-543` |
| 8 | **LOW** | Malformed JSON / Fallback | `setup.go` prime hook script escapes backslashes but not double quotes in `cwd` and `transcript_path` when creating catalog entries | `internal/cli/setup.go:59-63`, `internal/cli/setup.go:137-141` |
| 9 | **LOW** | Input Validation Gap | `writeSegments` in chunked tagging lacks chronological order validation, generating overlapping segments on out-of-order input | `internal/cli/cmd_tag.go:687-714` |
| 10 | **LOW** | Dead Code / Duplicate Logic | `view.sortCandidates` in `internal/view/view.go` is unexported dead code superseded by `agentproto.sortCandidates` | `internal/view/view.go:318-356`, `internal/agentproto/agentproto.go:1423` |
| 11 | **LOW** | Dead Code / Redundant Resolve | `runBrowseScoped` fallback loop calls `scopes.Resolve` twice for the first scope and contains an unreachable `if !checkedFreshness` block | `internal/cli/cli.go:1411-1439` |
| 12 | **LOW** | Connection Discipline Drift | Goose SQLite connections omit `con.SetMaxOpenConns(1)` and use 1000ms busy timeout rather than `store.ConnectRO` standard | `internal/source/goose/goose.go:178, 341` |

---

## 2. Detailed Findings & Evidence

---

### Finding 1: `tag-write --routine` Destroys Existing Topic Segments & Shadows Floor Verdict Source

- **File & Lines:** `internal/cli/cmd_tag.go:524-552`
- **Why it matters:** Marking a session routine via `tag-write --routine` permanently wipes all authored topic segments, violating the invariant that real topic tags demote routine verdicts at read time.

#### Evidence & Analysis
In `runTagWriteRoutine`:
```go
func runTagWriteRoutine(con *sql.DB, fullSID, source string, taggedAt float64) error {
	if err := store.EnsureTopicSchema(con); err != nil {
		return fmt.Errorf("ensure topic schema: %w", err)
	}
	if source == "" {
		source = store.VerdictSourceAgent
	}
	if source == store.VerdictSourceFloor {
		source, ok, err := verdictSource(con, fullSID)
		if err != nil {
			return fmt.Errorf("read existing verdict: %w", err)
		}
		if ok && source == store.VerdictSourceAgent {
			return nil
		}
	}
	if err := store.UpsertVerdict(con, store.Verdict{
		SessionID: fullSID,
		Verdict:   store.VerdictRoutine,
		Source:    source,
		TaggedAt:  taggedAt,
	}); err != nil {
		return err
	}
	if source == store.VerdictSourceFloor {
		return nil
	}
	return store.ReplaceSessionSegments(con, fullSID, nil)
}
```
1. **Destructive wipe on agent verdict:** When a user or agent marks a session with `--routine` (where `source` defaults to `"agent"`), line 551 executes `store.ReplaceSessionSegments(con, fullSID, nil)` which completely purges all topic segments from `topic_segment`. The system design doctrine explicitly states: *"A real tag beats routine"* at query time (e.g. `store.RoutineSet` and `SessionHasRealSegments`). Deleting existing topic segments destroys user data.
2. **Variable shadowing:** On line 532, `source, ok, err := verdictSource(con, fullSID)` re-declares `source` within the `if source == store.VerdictSourceFloor` block. If `ok == false` (no prior verdict), the inner `source` is `""`, but the outer `source` remains `"floor"`. While the outer condition on line 548 happens to match `"floor"`, the shadowing is brittle and masks intent.

---

### Finding 2: Consolidated Store Deletion Pruning Leaves Orphaned Sidecar Rows

- **File & Lines:** `internal/index/consolidated.go:741-764`, `internal/index/consolidated.go:941-965`
- **Why it matters:** Sessions purged from upstream source databases leave permanent orphaned rows in `topic_segment`, `session_verdict`, `file_index`, and `chunk_vec`, poisoning topic queries, routine sets, and vector storage.

#### Evidence & Analysis
In `consolidateOne`:
```go
	if _, err := tx.Exec(`
		DELETE FROM main.messages
		WHERE session_id IN (
			SELECT a.session_id
			FROM temp.consolidation_affected_sessions a
			WHERE NOT EXISTS (
				SELECT 1 FROM main.session_sources s WHERE s.session_id = a.session_id
			)
		)
	`); err != nil {
		return 0, false, true, fmt.Errorf("prune deleted session messages: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM main.sessions
		WHERE id IN (
			SELECT a.session_id
			FROM temp.consolidation_affected_sessions a
			WHERE NOT EXISTS (
				SELECT 1 FROM main.session_sources s WHERE s.session_id = a.session_id
			)
		)
	`); err != nil {
		return 0, false, true, fmt.Errorf("prune deleted sessions: %w", err)
	}
```
When a session is completely removed from all source databases:
1. Its rows in `main.messages` and `main.sessions` are deleted.
2. Its rows in `main.topic_segment`, `main.session_verdict`, `main.file_index`, and `main.chunk_vec` are **never deleted**.
3. Consequently:
   - `TaggedSessionIDs(con)` (`topics.go:233`) scans `topic_segment UNION session_verdict` and continues returning deleted session IDs.
   - `RoutineSet(con)` (`verdict.go:138`) continues evaluating deleted session verdicts.
   - `CheckProjectFreshness` / `CheckSessionFreshness` continue scanning orphaned entries in `file_index`.
   - `chunk_vec` retains orphaned embedding BLOBs.

---

### Finding 3: `CheckIndexFreshness` on Per-Project Stores Always Reports Stale

- **File & Lines:** `internal/cli/cli.go:1416`, `internal/index/consolidated.go:1111-1125`
- **Why it matters:** Every fallback browse query over per-project databases falsely marks the result as stale and triggers a redundant background `rawclaw ingest` process.

#### Evidence & Analysis
In `runBrowseScoped` (`cli.go:1416`):
```go
if db, dbErr := store.ConnectRO(dbp); dbErr == nil {
	if f, fErr := index.CheckIndexFreshness(db); fErr == nil {
		freshness = &f
	}
	_ = db.Close()
}
```
In `CheckIndexFreshness` (`consolidated.go:1116-1125`):
```go
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", MetaLastIngestTime).Scan(&ingestTimeStr); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IndexFreshness{Fresh: false, Reason: "read_ingest_watermark_failed"}, ...
	}
	if err := con.QueryRow("SELECT value FROM meta WHERE key=?", MetaLastIngestCatalogMTime).Scan(&catMTimeStr); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IndexFreshness{Fresh: false, Reason: "read_catalog_watermark_failed"}, ...
	}

	if ingestTimeStr == "" && catMTimeStr == "" {
		return IndexFreshness{Fresh: false, Reason: "no_ingest_watermark"}, nil
	}
```
`StampIngestWatermark` is only ever called on `consolidated.db` (in `consolidated.go:501, 543`). Per-project databases created during container indexing never stamp `last_ingest_time` or `last_ingest_catalog_mtime`. Therefore, `CheckIndexFreshness` on any per-project database unconditionally returns `IndexFreshness{Fresh: false, Reason: "no_ingest_watermark"}`. Line 1450 in `cli.go` sets `indexStale = true` and fires `maybeSpawnIngest("")` on every fallback browse.

---

### Finding 4: Vault Indexing Contradiction Between Full Index and Append

- **File & Lines:** `internal/index/containers.go:509-526`, `internal/index/containers.go:545-548`, `internal/durable/durable.go:102-111`
- **Why it matters:** Full container indexing of Claude transcripts creates lossy synthetic records in the durable vault, while append indexing copies verbatim bytes, creating divergent formats and violating byte-identical rebuild guarantees.

#### Evidence & Analysis
Package `internal/durable` documentation states:
> *"Content blocks are stored VERBATIM wherever the source is already in that shape (a byte copy), so a rebuilt index is byte-identical to the live one."*

However, in `internal/index/containers.go`:
- During full indexing (`vaultContainer`, line 525):
  ```go
  func vaultContainer(c source.Container, ms []model.Message, sourceID string, projectArg, cwdArg any) error {
      ...
      return durable.StoreMessages(m, ms)
  }
  ```
  It unconditionally invokes `durable.StoreMessages(m, ms)`, which serializes normalized text into synthetic Claude JSONL records (`{"type":"text","text":"..."}`), discarding original block structures.
- During append indexing (`vaultContainerAll`, lines 545-547):
  ```go
  if sourceID == sourceClaude || sourceID == "" {
      return durable.StoreFile(m, rawPath)
  }
  ```
  It checks `sourceID` and calls `durable.StoreFile(m, rawPath)` to perform a verbatim byte copy.
As a result, a session vaulted on initial indexing has synthetic JSONL, but switching to append overwrites it with verbatim JSONL.

---

### Finding 5: `RebuildFromTranscripts` on Consolidated Store Leaves `session_sources` Empty

- **File & Lines:** `internal/index/rebuild.go:210-280`, `internal/cli/cmd_consolidate.go:92`
- **Why it matters:** Rebuilding the consolidated store from the transcript vault leaves the `session_sources` table completely empty, breaking multi-source contribution tracking until watermarks are manually invalidated.

#### Evidence & Analysis
`rawclaw consolidate --from-transcripts` executes `runRebuildFromTranscripts` (`cmd_consolidate.go:92`), which calls `index.RebuildFromTranscripts(index.ConsolidatedPath())`.
In `rebuild.go:210-280`, `restoreSession` repopulates `messages`, `sessions`, and `file_index`, but does NOT insert any rows into `session_sources`.
When subsequent incremental write-through syncs (`SyncConsolidatedFrom`) execute:
1. `session_sources` contains no rows for existing sessions.
2. `healUpgradedConsolidatedStore` (`consolidated.go:1080`) detects `nSessions > 0 && nSources == 0` and deletes all `sync:*` watermarks.
3. The store is forced into an unexpected full re-fold on its next operation.

---

### Finding 6: `CheckProjectFreshness` Collides on Directory Basenames

- **File & Lines:** `internal/index/consolidated.go:1187-1196`
- **Why it matters:** Freshness checks for a project are corrupted by unrelated projects on different paths that share the same directory basename.

#### Evidence & Analysis
In `CheckProjectFreshness`:
```sql
SELECT path, mtime, size, fp
FROM file_index
WHERE (? = '' AND path LIKE ?)
   OR session_id IN (
        SELECT id FROM sessions
        WHERE project = ? AND (? = '' OR source_tool = ?)
    )
```
`project` in `sessions` is populated with `paths.ProjectLabel(tdir)` (the directory basename). If two distinct projects have the same folder name (e.g. `/home/user/work/service` and `/home/user/personal/service`), both share `project = "service"`.
When checking `/home/user/work/service`, the subquery selects sessions from both paths. When stat-ing the resulting `file_index` paths, if files in `/home/user/personal/service` were touched or purged, `CheckProjectFreshness` reports `transcript_content_changed` or `transcript_stat_changed` for the work repository.

---

### Finding 7: Goose Adapter Maps Tool Results to Role `"assistant"`

- **File & Lines:** `internal/source/goose/goose.go:542-543`
- **Why it matters:** Goose tool output is indexed as assistant conversation rather than tool results, breaking role filters and outline displays.

#### Evidence & Analysis
In `internal/source/goose/goose.go`:
```go
func normalizeRole(r string) string {
	r = strings.ToLower(strings.TrimSpace(r))
	switch r {
	case "user", "human":
		return "user"
	case "assistant", "model", "ai", "bot", "goose":
		return "assistant"
	case "system":
		return "system"
	case "tool", "tool_call", "tool_result":
		return "assistant"
	default:
		return "assistant"
	}
}
```
All other adapters (Claude, Codex, Antigravity) normalize tool outputs to role `"tool"`. Goose maps `"tool"` and `"tool_result"` to `"assistant"`. Consequently, searches filtering on `--role tool` find zero Goose tool results, and `--role assistant` returns raw tool output strings as model dialogue.

---

### Finding 8: Unescaped Double Quotes in `setup.go` Prime Hook Catalog JSON

- **File & Lines:** `internal/cli/setup.go:59-63`, `internal/cli/setup.go:137-141`
- **Why it matters:** Catalog entries for working directories or transcript paths containing double quotes emit invalid JSON, breaking O(1) catalog-first resolution.

#### Evidence & Analysis
In `setup.go`:
```sh
esc_session_id=$(printf '%s' "$session_id" | sed 's/\\/\\\\/g' || true)
transcript_path=$(printf '%s' "$input" | sed -n 's/.*"transcript_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
esc_transcript_path=$(printf '%s' "$transcript_path" | sed 's/\\/\\\\/g' || true)
cwd=$(printf '%s' "$input" | sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
esc_cwd=$(printf '%s' "$cwd" | sed 's/\\/\\\\/g' || true)
```
The substitution `sed 's/\\/\\\\/g'` escapes backslashes but ignores double quotes (`"`). If a path contains quotes (e.g. `/code/"test"/session`), the resulting catalog JSON is syntactically invalid. `paths.ReadCatalogEntry` fails during unmarshaling, forcing `ResolveSession` to abandon O(1) resolution and fall back to filesystem scans.

---

### Finding 9: `writeSegments` Missing Chronological Order Validation

- **File & Lines:** `internal/cli/cmd_tag.go:687-714`
- **Why it matters:** Out-of-order segment inputs in chunked tagging produce corrupt overlapping topic segments spanning the entire session tail.

#### Evidence & Analysis
In `cmd_tag.go`:
```go
		endDisp := rEnd
		if i+1 < len(segs) {
			nextSt := startDispIdx[i+1]
			if nextSt > st && nextSt <= rEnd {
				endDisp = nextSt - 1
			}
		}
```
If an agent submits segments out of chronological order (e.g. segment 0 starts at index 20 and segment 1 starts at index 5), `nextSt > st` (5 > 20) evaluates to `false`. Segment 0 is assigned `endDisp = rEnd` (the end of the range). Segment 1 also ends at `rEnd`. Both segments are written as overlapping ranges in `topic_segment` instead of being rejected with a validation error.

---

### Finding 10: Dead Unexported Function `view.sortCandidates`

- **File & Lines:** `internal/view/view.go:318-356`, `internal/agentproto/agentproto.go:1423-1460`
- **Why it matters:** Duplicate unexported candidate sorting logic creates dead code and maintenance confusion.

#### Evidence & Analysis
`sortCandidates` in `internal/view/view.go` is an unexported package-private function that is never called anywhere in `internal/view` (except in unit tests). The actual search sorting implementation was migrated to `agentproto.sortCandidates` in `internal/agentproto/agentproto.go:1423`. Both files received parallel updates for routine tier sorting, leaving `view.sortCandidates` as dead code.

---

### Finding 11: Dead Code and Redundant Resolution in `runBrowseScoped` Fallback Loop

- **File & Lines:** `internal/cli/cli.go:1411-1439`
- **Why it matters:** Redundant resolution of the first scope and duplicate unreachable freshness checks waste cycles during browse fallback.

#### Evidence & Analysis
In `cli.go`:
```go
		for _, sc := range scope {
			if !checkedFreshness {
				if dbp, _, err := scopes.Resolve(sc, o.Reindex); err == nil {
					...
				}
				checkedFreshness = true
			}
			if o.Source != "" && sc.Source != "" && sc.Source != o.Source {
				continue
			}
			dbp, _, err := scopes.Resolve(sc, o.Reindex)
			if err != nil {
				continue
			}
			if !checkedFreshness {
				if db, dbErr := store.ConnectRO(dbp); dbErr == nil {
					...
				}
				checkedFreshness = true
			}
			for _, r := range view.BrowseDB(dbp, o.Limit, o.Since, o.Before) { ... }
		}
```
1. `scopes.Resolve(sc, o.Reindex)` is called on line 1414, and then immediately called again on line 1427 for the first scope.
2. Lines 1431-1439 repeat the `if !checkedFreshness` block, which can never be entered because `checkedFreshness` was already set to `true` on line 1422.

---

### Finding 12: Goose SQLite Connection Lacks Single-Connection Discipline

- **File & Lines:** `internal/source/goose/goose.go:178, 341`, `internal/store/store.go:321-328`
- **Why it matters:** Goose adapter bypasses the single-connection pool discipline (`con.SetMaxOpenConns(1)`) and 5000ms busy timeout used across all other SQLite connections, risking connection pool deadlocks and busy timeouts.

#### Evidence & Analysis
`store.ConnectRO` (`internal/store/store.go:321`) sets `busy_timeout(5000)` and `con.SetMaxOpenConns(1)` to satisfy modernc.org/sqlite concurrency requirements. `goose.go` creates custom DSN strings with `busy_timeout(1000)` and does not configure `SetMaxOpenConns(1)`. Under concurrent discovery and message reading, multiple open connections against the same SQLite database can trigger lock contention and fail after 1000ms.
