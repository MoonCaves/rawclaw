# Ticket #121 (Rewrite): Triage 01 — Math Floor (Auto-Mark Trivial Sessions Routine)

**Status:** Ready for build  
**Area:** `internal/index`, `internal/store`, `internal/parse`, `internal/cli`  
**Tracking:** Rewrite of stale Ticket #121 (queue-based triage specification)  
**Dependencies:** Landed — TopicSchemaVersion 2 (`session_verdict`, triage #128), Routine Verdict & Sort-Tier Surfacing (#122)

---

## 1. Problem & Context

### 1.1 Stale Specification Elimination
The original Ticket #121 ("triage 01: math floor — auto-mark trivial sessions routine") was designed around an asynchronous, file-based tag queue (`tag-queue`, SessionEnd enqueue, SessionStart dequeue/drain). In commit `32eb5ea`, the tag queue was completely removed from `rawclaw` because the queue drifted in practice (accumulating hundreds of ephemeral/unindexed session IDs that never wrote transcripts) and was never part of the sovereign core architecture.

Consequently, any specification relying on "tag-queue drain", "dequeue", or `SessionEnd` queue hooks is obsolete and cannot be built.

### 1.2 Surviving Intent & Already-Landed Foundations
The core intent survives intact: **cheaply, deterministically, and conservatively mark obviously-trivial sessions as `routine`, using math only (zero LLM, zero external API calls, pure Go, zero runtime dependencies).**

The downstream storage and retrieval mechanisms are already fully implemented:
1. **Storage Layer (`internal/store/verdict.go`):** The `session_verdict` table (`TopicSchemaVersion = 2`) stores per-session verdicts (`verdict TEXT`, `source TEXT`, `origin_machine TEXT`, `tagged_at REAL`). `VerdictSourceFloor = "floor"` and `VerdictSourceAgent = "agent"` are established constants.
2. **Read-Time Demotion Rule (`internal/store/verdict.go:122-132`):** `IsEffectivelyRoutine` enforces the rule that "a real topic tag beats routine" — if a session has any real topic segment in `topic_segment` (`SessionHasRealSegments`), the routine verdict is rendered inert at read time non-destructively.
3. **Search Surfacing (`internal/agentproto/agentproto.go:1423-1460`):** Routine hits partition *after* normal hits at equal relevance across all search regimes (FTS, semantic RRF, and topic search). Routine hits are **never excluded by default** (a sole-match routine session still returns).
4. **Rendering & Explanation (`internal/render/render.go:141-145`, `internal/retrieve/retrieve.go`):** Results visibly display `· routine` in header formatting and report `tier="routine"` in `ScoreExplain` text and JSON envelopes.
5. **Multi-Machine Sync (`internal/archive/tags.go`, `internal/archive/tagapply.go`):** `TagVerdict` serializes `verdict="routine"` and `source="floor"` across git archive remotes with deterministic LWW tie-breaks.

---

## 2. Where the Math Floor Attaches (Trigger Mechanism)

With no tag queue, the math floor attaches to the **live indexing lifecycle** and an **explicit CLI backfill/sweep path**:

```
[Transcript File Written / Appended]
               │
               ▼
[Ingest / Indexing Path]
  ├─ internal/index/containers.go: reindexContainer()
  └─ internal/index/index.go: reindexFileWithOrigin()
               │
               ▼
[EvaluateMathFloor(messages)] (Pure Go arithmetic)
               │
      ┌────────┴────────┐
      ▼                 ▼
[Passes Floor]    [Exceeds Floor]
      │                 │
      ▼                 ▼
UpsertVerdict(floor)  RetractFloorVerdict() (Anti-stickiness)
      │
      ▼
[Consolidated Store Write-Through] (index.SyncConsolidatedFrom)
```

### 2.1 Trigger 1: Ingest & Reindexing Time (Live Ingestion)
- **Attachment point:** Synchronously inside `index.reindexContainer` (`internal/index/containers.go:299`) and `index.reindexFileWithOrigin` (`internal/index/index.go:580`), as well as targeted live refreshes via `rawclaw ingest` / `index.EnsureFreshContainer` (`internal/index/containers.go:40`).
- **Execution:** At the moment rawclaw parses the transcript records (`[]model.Message` or `[]parseRow`), it evaluates the math-floor predicate in memory.
- **Persistence:** Inside the same SQLite transaction (or immediately following session row upsert), if the floor condition is met and no `agent` verdict or real topic segment exists, it calls `store.UpsertVerdict` with `Source = store.VerdictSourceFloor`.
- **Dynamic Retraction (Anti-Stickiness):** If a previously evaluated session grew (e.g. user resumed a 1-turn session into a full work session) and now exceeds the floor thresholds, rawclaw retracts any existing `floor` verdict (`DELETE FROM session_verdict WHERE session_id=? AND source='floor'`).

### 2.2 Trigger 2: Explicit Corpus Sweep / Backfill Verb
- **Attachment point:** `rawclaw triage --floor` (or flag on `rawclaw reindex` / `rawclaw consolidate`).
- **Execution:** Scans indexed sessions in the target database, reads message stats, and writes `source = "floor"` verdicts for all qualifying sessions lacking topic segments or agent verdicts.
- **Output:** Emits a deterministic count receipt (e.g., `Evaluated 412 sessions: marked 89 as routine (floor), 323 normal`).

---

## 3. Math Floor Specification & Input Metrics

### 3.1 The Inflation Trap & The Single-Prompt Research Session
- **Inflation Trap:** `sessions.message_count` is inflated by ~10x due to runtime tool results (`[TOOL_RESULT]`), hook-injected envelope banners (`<command-message>`, `<task-notification>`, `<system-reminder>`, `<local-command-*>`), and status lines. A session with 30 database rows might only contain 1 human turn and 29 tool/banner rows.
- **Single-Prompt Research Session Trap:** A user issues a single prompt (`claude -p "deep research ..."` or an autonomous subagent task), prompting the agent to execute 50 tool calls and generate 200 assistant messages. Although `human_turns == 1`, this session is **substantive and high-value**.
- **The Strict AND Rule:** The math floor **MUST BE A CONJUNCTION (AND)** across all metrics, **NEVER A DISJUNCTION (OR)**:
  $$\text{IsRoutineFloor} \iff (\text{SubstantiveHumanTurns} \le T_{\text{human}}) \land (\text{TotalMessages} \le T_{\text{total}}) \land (\text{AssistantProseBytes} \le T_{\text{prose}})$$

### 3.2 Metric Definitions (Inputs)
All inputs are computed from parsed transcript records using existing `internal/parse` primitives:

1. **`SubstantiveHumanTurns` (`int`):**
   Count of messages where `MsgRole(m) == "user"` AND `parse.IsSubstantive(m.Content)` is `true`.
   - `parse.IsSubstantive` (`internal/parse/parse.go:403`) already rejects low-signal content: empty text, bare greetings (`hi`, `hey`, `hello`, `ok`, `thanks`, etc.), bare slash commands (`/clear`, `/exit`, `/quit`, `/help`), command envelope tags (`<command-name>`, `<command-message>`, `<local-command-*>`), and JSON/XML openers (`{`, `[`, `<`).
2. **`TotalMessages` (`int`):**
   Total count of raw records/messages in the session (`len(messages)` or `sessions.message_count`).
3. **`AssistantProseBytes` (`int`):**
   Total byte length of assistant messages with tool calls stripped via `parse.StripTools` (`internal/parse/parse.go:228`) and thinking blocks stripped via `parse.StripThinking` (`internal/parse/parse.go:651`).

### 3.3 Concrete Thresholds
A session is marked routine by the math floor if and only if it matches **Condition A (Aborted/Bounce)** OR **Condition B (Trivial Single-Exchange)**:

- **Condition A (Bounce / Aborted Launch):**
  - `SubstantiveHumanTurns == 0`
  - `TotalMessages <= 2`
  *(Examples: accidental CLI launch followed by immediate `/exit`, or a single bare "hi" with no follow-up).*

- **Condition B (Trivial Single-Turn Exchange):**
  - `SubstantiveHumanTurns <= 1`
  - `TotalMessages <= 4`
  - `AssistantProseBytes <= 400`
  *(Examples: "what is the date?" → "It's Tuesday, August 25, 2026.", with 0 tool calls).*

**Any session with `SubstantiveHumanTurns > 1`, OR `TotalMessages > 4`, OR `AssistantProseBytes > 400` FAILS the math floor and remains normal.**

---

## 4. Fail-Safes and Invariants

1. **Agent / Human Verdict Supremacy:**
   - The math floor only writes rows with `source = 'floor'`.
   - If a session already carries an `agent` verdict (`source = 'agent'`), the math floor **never overwrites or clears it**.
2. **Read-Time Real-Tag Precedence ("Real Tag Beats Routine"):**
   - `store.IsEffectivelyRoutine` (`internal/store/verdict.go:122`) evaluates `session_verdict.verdict == "routine" AND NOT SessionHasRealSegments(con, sid)`.
   - If topic segments exist or are added later via `tag-write <sid>`, the session is immediately treated as normal/non-routine at search time without mutating the verdict row.
3. **Anti-Stickiness Retraction on Session Growth:**
   - Active sessions grow over time. When an active session is re-indexed upon receiving new messages, `EvaluateMathFloor` runs on the updated transcript.
   - If the session previously had a `source = 'floor'` verdict but now exceeds the thresholds, rawclaw executes:
     `DELETE FROM session_verdict WHERE session_id = ? AND source = 'floor'`.
4. **Non-Destructive Surfacing Only:**
   - A `routine` verdict is strictly a search sorting hint (partitions below equal-relevance normal hits in `sortCandidates`).
   - It never deletes records, never prunes transcripts, and never hides hits from search results unless explicit filtering is requested.
5. **Deterministic & Zero-Dependency:**
   - No network calls, no model calls, no non-deterministic heuristic clocks.
   - The same transcript evaluated on any machine yields the exact same floor verdict.
6. **Graceful Degrade on Partial / Unparseable Transcripts:**
   - If a transcript fails parsing or is corrupted, no verdict is written. Uncertainty leaves the session un-verdicted (normal tier).

---

## 5. Implementation Plan

### Step 1: Floor Evaluation Primitive (`internal/parse` or `internal/triage`)
Implement a pure evaluation helper:
```go
type FloorStats struct {
    SubstantiveHumanTurns int
    TotalMessages         int
    AssistantProseBytes   int
}

func EvaluateMathFloor(messages []model.Message) (isRoutine bool, stats FloorStats) {
    var humanTurns, totalMsgs, proseBytes int
    totalMsgs = len(messages)
    for _, m := range messages {
        switch m.Role {
        case "user":
            if IsSubstantive(m.Text) {
                humanTurns++
            }
        case "assistant":
            prose := StripThinking(StripTools(m.Text))
            proseBytes += len(prose)
        }
    }
    stats = FloorStats{
        SubstantiveHumanTurns: humanTurns,
        TotalMessages:         totalMsgs,
        AssistantProseBytes:   proseBytes,
    }
    // Condition A: Bounce
    if humanTurns == 0 && totalMsgs <= 2 {
        return true, stats
    }
    // Condition B: Trivial single-exchange
    if humanTurns <= 1 && totalMsgs <= 4 && proseBytes <= 400 {
        return true, stats
    }
    return false, stats
}
```

### Step 2: Store Ingest & Retraction Primitives (`internal/store/verdict.go`)
1. Add `RetractFloorVerdict(con *sql.DB, sessionID string) error`:
   ```sql
   DELETE FROM session_verdict WHERE session_id = ? AND source = 'floor'
   ```
2. Add `ApplyFloorVerdict(con *sql.DB, sessionID string, isRoutine bool, taggedAt float64)`:
   - If `isRoutine`: check existing verdict via `VerdictFor`. If none exists or existing `source == "floor"`, upsert `VerdictRoutine` with `Source = VerdictSourceFloor`. If existing `source == "agent"`, preserve the agent verdict.
   - If `!isRoutine`: call `RetractFloorVerdict`.

### Step 3: Indexer Integration (`internal/index/containers.go` & `internal/index/index.go`)
- In `reindexContainer` (`internal/index/containers.go:299`) and `reindexFileWithOrigin` (`internal/index/index.go:580`), invoke `ApplyFloorVerdict` inside the reindexing transaction.
- When `SyncConsolidatedFrom` runs, the updated verdict row (or deletion) automatically propagates to `consolidated.db`.

### Step 4: CLI Sweep Verb (`internal/cli/cmd_tag.go` or `internal/cli/cmd_triage.go`)
- Add `rawclaw tag-floor [--this-project] [--dir <path>]` or `rawclaw triage --floor` to sweep existing local databases, evaluate sessions, and report summary statistics.

---

## 6. Test Plan & Acceptance Matrix

| Test Case | Scenario | Expected Outcome |
| :--- | :--- | :--- |
| **`TestMathFloor_BounceSession`** | 1 user turn with `/exit` or `hi`, 0 tool calls, 1 assistant ack (`TotalMessages = 2`). | Marked `routine` (`source = "floor"`). |
| **`TestMathFloor_TrivialExchange`** | 1 human question ("date?"), 1 short assistant response (50 bytes), 0 tool calls. | Marked `routine` (`source = "floor"`). |
| **`TestMathFloor_SinglePromptResearch`** | 1 human prompt, 20 tool calls (`[TOOL:...]`), 20 tool results (`[TOOL_RESULT]`), 5000 assistant prose bytes (`TotalMessages = 42`). | **NOT routine** (Normal tier; floor fails on message count & prose bytes). |
| **`TestMathFloor_MultiTurnConversation`** | 3 substantive human turns ("let's debug", "try this", "fixed"), 6 total messages. | **NOT routine** (Normal tier; floor fails on human turns > 1). |
| **`TestMathFloor_RetractionOnGrowth`** | Session starts with 1 trivial turn (marked routine by floor). Second turn adds substantive prompt and tool work. Reindexed. | Floor verdict **retracted** (`session_verdict` row deleted). Session returns to normal tier. |
| **`TestMathFloor_AgentVerdictSupremacy`** | Session has 1 message marked normal/custom by agent (`source = "agent"`). Math floor runs. | Agent verdict **preserved**; floor does not overwrite. |
| **`TestMathFloor_RealTagBeatsRoutine`** | Session marked routine by floor. Topic segment added via `tag-write`. | `IsEffectivelyRoutine` returns `false`. Search treats session as normal. |
| **`TestMathFloor_ArchiveSync`** | Floor verdict exported to archive `TagFile` and pulled on second machine. | Replicated with `source = "floor"`, participating in LWW verdict tie-break. |
