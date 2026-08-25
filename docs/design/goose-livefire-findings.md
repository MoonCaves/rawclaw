# Goose Adapter Live-Fire Findings & Stranded-Sidecar Analysis

**Trackers:** #205 (Goose adapter verification against real schema), #206 (Stranded-sidecar edge)  
**Date:** 2026-08-25  
**Author:** Antigravity / Gemini 3.7 Flash (High)  

---

## 1. Background & Purpose

RawClaw shipped a Goose source adapter in `v0.9.0` (`internal/source/goose/`). Because it had not been exercised against a live Goose installation, its README entry was marked experimental.

The goal of this investigation is:
1. **Determine the true schema** of Goose session databases from primary sources (installed binary or upstream repository documentation/code) rather than assuming our adapter's internal heuristics are correct.
2. **Exercise the adapter hermetically** against a realistic database fixture matching the upstream schema.
3. **Analyze the stranded-sidecar edge case (Tracker #206)** when a SQLite database in Write-Ahead Logging (WAL) mode is moved without its `-wal` and `-shm` sidecars.

---

## 2. Step 1 — Real Goose Shape & Schema

### Route Taken
- **Local environment check:** `which goose` returned `not found` (Goose is not installed in the local environment).
- **Upstream research:** Investigated the official upstream repository (`block/goose`, now maintained under the Agentic AI Foundation / AAIF) and official documentation (`goose-docs.ai`, `agent-safehouse.dev`).

### Verified Upstream Storage Architecture
- **Version 1.10.0+ Migration:** Starting in v1.10.0, Goose migrated from per-session `.jsonl` files in `~/.local/share/goose/sessions/` to a centralized SQLite database (`sessions.db`).
- **Database Locations:**
  - Linux / macOS default: `~/.local/share/goose/sessions/sessions.db` (also checked: `~/.config/goose/sessions/sessions.db`, `~/Library/Application Support/goose/sessions/sessions.db`, `$GOOSE_PATH_ROOT`, `$GOOSE_HOME`).
  - Windows: `%APPDATA%\Block\goose\data\sessions\sessions.db`.
- **Database Journaling:** SQLite WAL mode (`PRAGMA journal_mode=WAL;`), generating `sessions.db-wal` and `sessions.db-shm` alongside `sessions.db`.

### Database Schema (Goose v1.10.0+)

#### `sessions` Table
```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    working_dir TEXT NOT NULL,
    name TEXT NOT NULL,
    user_set_name INTEGER DEFAULT 0,
    session_type TEXT DEFAULT 'user',
    created_at TIMESTAMP
);
```

#### `messages` Table
```sql
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,       -- 'user' | 'assistant'
    content TEXT NOT NULL,    -- JSON formatted array of content blocks or text
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### Message Content Format
Goose stores messages conforming to Model Context Protocol (MCP) content structures:
- Text content: `[{"type": "text", "text": "Message content here"}]` or `[{"text": "Message content here"}]`
- Multi-block text: `[{"type": "text", "text": "Part 1"}, {"type": "text", "text": "Part 2"}]`
- Tool result: `[{"type": "tool_result", "content": "Command output", ...}]`
- Plain string fallback: `"Plain text message"`

#### CLI Session Resumption
Goose CLI supports resuming sessions via:
```bash
goose session --resume --session-id <session_id>
goose session -r --name <session_name>
```

---

## 3. Step 2 — Adapter Compatibility & Verification

We evaluated `internal/source/goose/goose.go` against the verified upstream schema:

1. **Table & Column Resolution:**
   - **`sessions` table:** `findMatchingCol` successfully detects `id` and `working_dir` (`cwd`). `parent_id` and `is_subagent` columns are absent in standard Goose, gracefully defaulting to empty string and `false` without errors.
   - **`messages` table:** `findMatchingCol` successfully detects `id`, `role`, `content`, and `timestamp` (`tsCol`). Filtering by `session_id = ?` correctly isolates messages per session within the shared `sessions.db`.
2. **Container Construction:**
   - `c.Path` is formatted as `<path>/sessions.db#<session_id>`, allowing multiple sessions to share the single physical file.
   - `c.ResumeArgv` produces `["goose", "session", "--resume", "--session-id", sID]`, matching Goose CLI syntax.
3. **Content Extraction:**
   - `extractContent` unpacks `[{"type":"text", "text":"..."}]` and `[{"content":"..."}]` JSON blocks into human-readable text for FTS5 indexing.
4. **Timestamp Parsing:**
   - `parseTimestamp` parses SQLite `CURRENT_TIMESTAMP` (`2006-01-02 15:04:05`) as well as ISO8601/RFC3339 timestamps.

---

## 4. Step 3 — Stranded-Sidecar Edge (Tracker #206)

### The Edge Case
In SQLite WAL mode:
- Active writes are committed to `<name>.db-wal` with an index in `<name>.db-shm`.
- Changes are merged back into `<name>.db` only during a checkpoint (explicit `PRAGMA wal_checkpoint` or auto-checkpoint).
- If a component moves or archives only the primary `<name>.db` file (e.g. `lifecycle.Archive` using `moveFile(src, dst)` on a verbatim path), the `-wal` and `-shm` files remain at the source path.

### Experimental Findings
We constructed an explicit test simulating uncheckpointed WAL writes followed by moving only the `.db` file:
1. **SQLite Behavior on Read:**
   - When SQLite opens the moved `.db` file without a `-wal` file present, **it does not crash, return an error, or fail with database corruption**.
   - SQLite reads the database cleanly as of the **last checkpoint**.
   - Any transactions that resided solely in the stranded `-wal` file are omitted from queries on the moved database.
2. **Adapter & Indexer Degradation:**
   - The Goose adapter executes queries against the moved `.db` successfully.
   - The read **degrades cleanly**: all checkpointed sessions and messages are returned intact. Uncheckpointed messages are not visible until/unless the WAL is restored or checkpointed prior to move.
3. **RawClaw Indexing Fingerprint Protection:**
   - `internal/index/containers.go:105-113` already checks `rawPath + "-wal"` when generating file fingerprints (`backingFileState`). For live Goose databases, WAL modifications alter the combined fingerprint, prompting reindexing.

### Conclusion & Recommendation
- Moving only a `.db` file degrades cleanly to the last-checkpointed state rather than causing runtime crashes or database read errors.
- For maximum fidelity when archiving standalone `.db` files, callers should ideally copy or checkpoint sidecars, but the adapter's read behavior degrades safely under current conditions.

---

## 5. Step 4 — Hermetic Upstream Schema Test Suite & E2E Verification (#205 Resolved)

To definitively complete Tracker #205, we constructed a hermetic test suite under `internal/source/goose/` that models the verified upstream Block/AAIF Goose v1.10.0+ schema directly:

### 1. Hermetic Fixture Builder (`fixture_test.go`)
`setupUpstreamGooseFixture(t, baseDir)` sets up a WAL-journaled `sessions.db` with:
- Exact upstream DDL for `sessions` and `messages` tables.
- Diverse realistic test sessions:
  - **Session 1:** Multi-turn coding session with MCP JSON arrays (`[{"type":"text","text":"..."}]`), multi-block assistant responses, and MCP tool results (`[{"type":"tool_result","content":"..."}]`).
  - **Session 2:** Web UI session with `user_set_name = 1` and isolated message history.
  - **Session 3:** System role session with plain text fallback (non-JSON strings).
  - **Session 4:** Empty session with zero message rows.

### 2. Verification Test Suite (`e2e_test.go`)
The adapter was exercised against this fixture across all key subsystems:
1. **Discovery & Container Metadata (`TestGooseUpstreamSchema_DiscoveryAndContainerMetadata`):**
   - Discovers all 4 sessions with `#<session_id>` container paths.
   - Accurately maps `working_dir` -> `CWD`.
   - Verifies that absent `parent_id` and `is_subagent` columns in upstream schema gracefully default to `ParentID: ""` and `IsSubagent: false`.
   - Confirms `ResumeArgv` produces `["goose", "session", "--resume", "--session-id", <id>]`.
2. **Message Extraction & Content Parsing (`TestGooseUpstreamSchema_MessageExtractionAndContentParsing`):**
   - Verifies strict session isolation (`session_id = ?`) with zero message bleed.
   - Verifies chronological ordering by timestamp.
   - Confirms `extractContent` unpacks MCP JSON blocks, joins multi-block text with newlines, extracts tool results, and preserves plain strings.
   - Confirms `parseTimestamp` parses SQLite `CURRENT_TIMESTAMP` format (`YYYY-MM-DD HH:MM:SS`) into epoch floats and RFC3339 strings.
   - Confirms empty sessions return zero messages without error.
3. **Full RawClaw Indexing & FTS5 Recall (`TestGooseUpstreamSchema_FullRawClawIndexingAndSearch`):**
   - Feeds discovered containers through `index.EnsureIndexedContainers` into an isolated RawClaw SQLite store.
   - Verifies that all sessions and 9 messages are indexed cleanly (`IndexFresh`).
   - Executes FTS5 queries against `messages_fts` (e.g. searching "checkout", "navbar", "checkpoint"), verifying accurate hit counts and session mapping.

### 3. Upstream Divergence Summary
- **No breaking divergences found.**
- Upstream Goose does not store subagent lineage or parent IDs in `sessions.db`; the adapter's column discovery handles this without errors or schema modifications.
- Upstream message content formats (MCP JSON arrays and plain strings) are fully handled by `extractContent`.

---

## 6. Final Status

- **Tracker #205 (Goose adapter verification against real schema):** **RESOLVED.** Verified hermetically against primary-source schema and MCP content block shapes with full indexing and FTS5 recall tests.
- **Tracker #206 (Stranded-sidecar WAL behavior):** **RESOLVED.** Verified that moving a `.db` without `-wal`/`-shm` degrades cleanly to the last checkpoint without crashes or corruption.

