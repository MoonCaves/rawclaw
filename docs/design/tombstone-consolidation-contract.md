# Design: Tombstone Consolidation Contract (Tracker #217)

Status: RATIFIED DESIGN.
Issue: A deleted session can stay searchable in consolidated.db when source databases have not changed.

## 1. Problem Statement

In internal/index/consolidated.go, ConsolidateFrom coordinates folding per-project SQLite databases into the single consolidated search database (consolidated.db). To avoid expensive re-scans, consolidateOne compares each source database against its last-folded-in watermark (sync:<dbname>), setting changed = true only if session counts, message counts, max IDs, or topic/verdict watermarks differ.

At the end of ConsolidateFrom, tombstone pruning was gated behind:
`if rebuild || anyChanged`

The current foreground tag flow is also intentionally asynchronous: on its live-source
path, `tag-prep` refreshes the requested session, while `tag-write` authors its tags;
each then attempts to queue detached publication/fold work. The foreground command
does not wait for the full consolidated-store fold. A successful detached child start
is best-effort and is not a durable completion or retry guarantee.

### The Defect
1. A session is indexed into a per-project db (p.db) and consolidated into consolidated.db.
2. The user deletes the session (rawclaw delete <session>). The CLI removes the transcript file (if live) and appends the session ID to the tombstone sidecar (~/.cache/session-search/.deleted). Crucially, p.db is NOT touched by the delete command (by design, per-project dbs are reconciled during their next scan).
3. The user or background process runs consolidation (rawclaw consolidate without --rebuild).
4. ConsolidateFrom checks p.db, finds its watermark matches the recorded sync:p.db mark, and reports changed = false.
5. Because no source reported changes, anyChanged remains false.
6. Pruning is skipped completely.
7. The deleted session remains searchable in consolidated.db.

When a source database itself no longer offers a session, consolidation removes
that source contribution during the merge. For each relevant sidecar table present
in the source, the affected rows are pruned after the merge only when no
`session_sources` contributor remains. This prevents orphaned sidecar rows while
preserving tags belonging to another surviving source. A completely empty topic set
does not itself carry a durable deletion revision; the current publisher therefore
cannot use emptiness alone to prove that it supersedes an existing consolidated topic
set.

## 2. Evaluation of Candidate Contracts

We evaluate two design shapes for resolving this defect:

### Candidate (a): Tombstone application is INDEPENDENT of source-change detection — pruning always runs

- **Contract**:
  ConsolidateFrom splits the consolidation pass into two orthogonal responsibilities:
  1. Source fold-in phase: Ingests new or modified rows from per-project source databases using source watermarks to skip unchanged sources.
  2. Destination invariant phase: Enforces destination store hygiene by executing pruneTombstoned(con) unconditionally on every pass.

- **Cost Analysis**:
  - In pruneTombstoned(con), lifecycle.LoadTombstones("") opens and reads the .deleted sidecar file.
  - If .deleted does not exist or is empty (the standard state when no sessions have been deleted), LoadTombstones returns an empty map (len(tombstoned) == 0) and returns nil immediately. Exactly zero SQL statements are issued. The overhead is a single file-system stat/open (microseconds).
  - If tombstones exist, pruneTombstoned first performs an indexed primary-key existence check (`SELECT 1 FROM sessions WHERE id=?`) for each ID. A missing ID skips the six-table delete entirely. For an ID that exists, six `DELETE ... LIKE` statements still run, now inside one transaction instead of six separate autocommits.
  - Consolidation is invoked during explicit CLI commands (rawclaw consolidate) or post-indexing syncs. The unconditional tombstone check therefore runs on each pass, while the existence check avoids unnecessary delete work for IDs absent from the destination.

- **State & Invariants**:
  - Requires no additional state or schema changes.
  - Eliminates state drift: regardless of how or when tombstones or source databases were modified, running ConsolidateFrom guarantees that consolidated.db contains no tombstoned sessions.

### Candidate (b): The change signal must INCLUDE "there are unapplied tombstones" (anyChanged includes pending tombstones)

- **Contract**:
  Consolidation defines a composite change detector: anyChanged = sourcesChanged || tombstonesChanged. consolidated.db records a tombstone watermark (e.g. hash, timestamp, or line count of .deleted) in the meta table. If the sidecar watermark differs from the stored mark, tombstonesChanged becomes true, pruneTombstoned runs, and the new watermark is stamped into meta.

- **Cost Analysis**:
  - To check if tombstones changed, (b) must read/stat .deleted AND execute a SELECT value FROM meta WHERE key = 'sync:tombstones' query against SQLite.
  - On a no-op consolidation where no tombstones exist, (b) is actually more expensive than (a) because it executes a SQLite query on every run rather than immediately returning on len == 0.
  - When tombstones change, (b) must additionally execute an INSERT OR REPLACE INTO meta query.

- **State & Invariants**:
  - Introduces a new metadata key and synchronization state.
  - Subtle failure mode with additive merges: Merges from source databases are additive (INSERT INTO ... ON CONFLICT DO UPDATE). If source DB B.db is updated and folded into consolidated.db at a later time, and B.db still contains an old session that was already tombstoned and marked "applied", the additive merge will re-insert the tombstoned session into consolidated.db. To prevent resurrection, pruneTombstoned would STILL have to run whenever sourcesChanged is true, regardless of whether tombstones changed.

## 3. Decision

We choose Candidate (a).

Rationale:
1. Decoupled Responsibilities: Source-change detection determines whether source databases need to be re-read; tombstone pruning determines whether the consolidated store satisfies the deletion invariant ("A user delete is the one absence that must propagate"). Gating destination hygiene on source file modification was a category error.
2. Minimal Complexity & Zero State: Candidate (a) introduces no watermark tracking for tombstones in SQLite meta, avoiding state drift and synchronization bugs.
3. Bounded Cost: With no tombstones present, (a) issues 0 SQL queries. Missing tombstoned IDs avoid the six-table delete; existing IDs use one transaction for the six targeted deletes.

## 4. Implementation Plan

1. In internal/index/consolidated.go (ConsolidateFrom), remove the `rebuild || anyChanged` gate around pruneTombstoned(con):
   unconditionally execute `if err := pruneTombstoned(con); err != nil { return st, err }`.
2. Add a CLI journey test in internal/cli/ verifying the contract:
   - Index a project containing a session -> fold it into consolidated.db via rawclaw consolidate.
   - Verify session is searchable in consolidated.db.
   - Add a tombstone for the session without touching the source project or its db.
   - Run rawclaw consolidate without --rebuild.
   - Assert the session is no longer searchable in consolidated.db.
3. Verify test failure when the fix is reverted (red), and passing when restored (green).
