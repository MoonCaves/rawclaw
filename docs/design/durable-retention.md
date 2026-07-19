# Design: durable retention (central-store feature #1)

Status: DRAFT (evidence, not ratified). Branch `feat/durable-retention`.
Every decision below cites verified prior art. Citations were independently
re-verified against the source (repo+file+line / doc), not
taken from a secondhand summary. See `prior-art/` for the full research.

## Problem

`rawclaw`'s indexer prunes any session from its SQLite DB when the backing
transcript file is absent from the local disk walk
(`internal/index/index.go:640-652`, `internal/index/containers.go:116-130`). It
runs on **every** search (not just `--reindex`). So:

- Claude Code purges local transcripts at `cleanupPeriodDays` (default 30) → the
  next search deletes those sessions from the DB. rawclaw is a **mirror, not an
  archive**.
- A central store holding **other machines'** sessions can't work: those files
  aren't in this machine's walk, so every local search deletes them.

But a real user delete (`rawclaw delete`) must still take effect. So the core
job is to **tell "file intentionally deleted" apart from "file merely absent,"**
and only prune the former.

Grounded enablers (verified this session):
- `read`/`outline` read content from the SQLite `messages` table
  (`agentproto.go:1102`, `:215`), **not** the JSONL — so a retained row stays
  fully searchable and readable with the source file gone.
- `sessions` schema today is `(id TEXT PRIMARY KEY, started_at, last_ts,
  message_count, is_subagent, parent_id)` — no provenance columns
  (`index.go:29-33`).

## Decisions

### D1 — Never infer a delete from a missing file
On scan, a session whose backing file is absent is **not** pruned. Mark it
`missing_since = <now>` and keep the row (searchable, readable, flagged).
- **Prior art (verified):** Zoekt moves repos absent from the authoritative
  listing to `.trash` (soft delete), restoring them if they reappear — it never
  hard-deletes on absence during a scan (`cmd/zoekt-sourcegraph-indexserver/cleanup.go:134-146`).
  notmuch's filename-removal is scoped to the `notmuch new` directory it just
  walked and, per `lib/message.cc:1005`, "does not remove a document from the
  database" — deletion is a separate explicit call. recoll gates its purge on a
  *complete* scan and ships `--nopurge` for "temporarily not accessible" sets.
- **Tradeoff:** DB retains missing-but-not-deleted rows → unbounded-looking
  growth. Accepted: it's the cost every surveyed indexer pays to avoid silent,
  unrecoverable loss — which is today's bug. Content is compact vs raw JSONL.

### D2 — Own-source prune key
The prune is scoped to what **this machine's live tree actually sources**. A row
whose `(origin_machine, source_path)` is not part of this run's scope is never a
prune candidate — foreign-machine and other-root rows are structurally
out of scope, not "missing."
- **Prior art (verified):** Zoekt's `Repository.Source` field exists explicitly
  "to detect orphaned index shards" (`api.go:593-597`) — the same field-drives-
  prune-safety pattern.
- **Tradeoff:** prune becomes scope-relative, not global. Correct for a
  multi-source store; requires provenance (D3).

### D3 — Per-session provenance columns
Add to `sessions` (denormalized onto `file_index` where the watermark needs it):
- `origin_machine TEXT` — a stable, self-minted machine id (persisted once), not
  the hostname. **Prior art:** Syncthing self-mints a `DeviceID` rather than
  trusting a mutable name (`lib/protocol/deviceid.go`). Derivation is rawclaw's
  choice (persisted random id is enough); the point is *stable + self-minted*.
- `source_tool TEXT` — persist the existing `source.Registration.ID`
  ("claude"/"codex"), which is minted today but never stored on the row
  (`internal/source/source.go:43`).
- `source_path TEXT` — the backing root, mirroring Zoekt `Repository.Source`
  (D2).
- `missing_since REAL` — NULL when present; set when the source vanished (D1).
- **Tradeoff:** schema change → migration (see Migration). Small, additive.

### D4 — Identity: tool-minted id verbatim, compound uniqueness
Keep the session id exactly as the tool minted it. Uniqueness becomes the
compound `(origin_machine, source_tool, session_id)`, replacing the bare
`id TEXT PRIMARY KEY`. Never hash or re-derive the id.
- **Prior art (verified/corroborated):** Syncthing keys on `(folder, device)`;
  Automerge's `OpId` is `(actor, counter)`; Sourcegraph stacks `TenantID` above
  Zoekt's repo `ID` (`api.go:578-582`). Counter-example: git content-addressing
  collapses identical content across origins — the wrong property here.
- **Fixes the demonstrated bug:** a sibling runtime's `state.db` put `UNIQUE` on session
  *title*, conflating a display label with storage identity → foreign-session
  collisions. Identity and dedup are kept as **separate** mechanisms.
- **Tradeoff:** Claude/Codex UUIDs are already independently unique (collision
  math per RFC 9562 is not the real risk); the compound key is belt-and-braces
  and, more importantly, the hook the store needs to scope prune by origin.

### D5 — Tombstone = explicit delete only, propagates, bounded GC
`rawclaw delete` writes a tombstone (this exists today) meaning "deleted
everywhere"; an upstream purge writes **no** tombstone (D1). Tombstones are
GC'd after a bounded grace window.
- **Prior art (verified):** deletion is a *positive signal* in every sync
  system — Syncthing `SetDeleted()` sets an explicit `Deleted` bit + bumps the
  version (`lib/protocol/bep_fileinfo.go:588`); IMAP has `\Deleted` + a separate
  `EXPUNGE` (RFC 3501). Cassandra bounds tombstone life via `gc_grace_seconds`
  (default 10 days), sized to convergence time — the model for the GC window.
- **Tradeoff:** a bounded window means a delete must propagate before GC; fine
  for our mesh cadence.

### D6 — Migration is additive `ALTER TABLE`, NOT a version-bump rebuild
`SchemaVersion` mismatch triggers a **full rebuild from source**
(`index.go:26-27`). A rebuild re-walks the live tree — which would re-prune every
already-purged session and defeat this feature on the very first upgrade. So the
durability columns are added by an **in-place `ALTER TABLE ADD COLUMN`** guarded
migration, NOT by bumping `SchemaVersion`. Backfill existing rows: `origin_machine`
= this machine, `source_tool` = the scope's source, `source_path` = the existing
`file_index.path`, `missing_since` = NULL.
- **Basis:** verified `index.go` rebuild-on-mismatch semantics + the index's own
  precedent of lossless additive cache migrations (`index.go:417`).
- **Tradeoff:** a hand-written migration path instead of a free rebuild; the
  rebuild is exactly what we must avoid.

### D7 — Recency / staleness signal on retained rows
Retained missing/old rows must not read as current. Surface `missing_since` +
age in the envelope, reusing rawclaw's existing recency hint and its "raw
history — verify against current state" vocabulary (`--help`, envelope).
- **Prior art:** rawclaw already ships a recency hint that surfaces a buried
  newer match; extend it to flag retained-but-missing rows.
- **Tradeoff:** none beyond a column read; strictly additive signal.

### D8 — Discovery is store-driven, not only disk-driven  (added 2026-07-17 after live-verify)
Retaining a row is useless if the search can't reach it. `scopes.Claude()`
enumerates scopes from `paths.AllProjectDirs()` — the live disk only — so a
session whose project dir emptied or vanished (the exact 30-day-purge case) has a
retained DB that is **never opened**. Fix: scope discovery must enumerate the
**union** of (live source dirs) and (existing per-machine index DBs that still
hold ≥1 non-tombstoned session). An orphaned-source DB is surfaced as an eager
read-only scope (DBP set, like the Codex scopes) so search reaches it without
re-walking a source that is gone.
- **Prior art (verified):** Zoekt's cleanup enumerates the **index shards**
  themselves (`for repo, shards := range indexShards`, `cleanup.go:134-146`) and
  reconciles them against the authoritative source listing — the index is a
  first-class discovery surface, not a mirror of the source. We were discovering
  source→index only; Zoekt discovers index, reconciled against source.
- **How it was caught:** unit tests call the indexer with a live DB handle and
  passed while the feature was broken end-to-end; driving the real binary through
  a purge (index → search hit → delete the JSONL → search) returned "No matches"
  because `--list` dropped the emptied project. Live verification is mandatory
  here for exactly this reason.
- **Tradeoff:** discovery now stats the DB cache dir too (cheap); an orphaned DB
  with only tombstoned/empty sessions must be excluded so deletes still read as
  deleted.

## Deferred — central-store topology (forward hook, NOT built here)
The provenance from D3 serves either of two verified topologies; picking one is a
later feature. Both consume `(origin_machine, source_tool, source_path)`
unchanged, so building provenance now commits us to neither.
1. **Query-time federation** — per-machine DBs synced to a shared location,
   `ATTACH`+`UNION` at query. Transport: `sqlite3_rsync` (official SQLite,
   transaction-aware DB replication over SSH — `sqlite.org/rsync.html`, verified).
   Query pattern: Datasette cross-database queries (ATTACH-based).
   *(Correction: dogsheep-beta was mis-cited by research as this — it actually
   does #2.)*
2. **Index-time consolidation** — a central store ingests every machine's
   sessions into one index table. Prior art: Simon Willison's **dogsheep-beta**,
   which builds one `search_index` table from many source DBs at index time
   (verified — it is NOT live cross-DB federation).
- **Ruled out for the store** (research, cited): rqlite/dqlite (Raft quorum
  stops writes when partitioned; cgo), LiteFS (FUSE daemon + cgo), Turso/libSQL
  (hosted primary = mandatory network), marmot (always-on daemon + cgo). All
  violate the static-binary / no-daemon / offline-tolerant north-star.
- **OPEN, verify at federation-build time:** `modernc.org/sqlite` (rawclaw's
  pure-Go driver) `ATTACH` + cross-DB FTS5 behavior.

## Build scope (this branch)
D1–D7 only. The topology is out of scope. Contained edits:
- Schema: add the four columns + the additive migration (D6).
- Ingest: stamp provenance on write (`reindexContainer` / `file_index` insert).
- Prune: replace the "absent from disk walk → delete" loops with
  own-source-scoped + explicit-tombstone-only deletion; set `missing_since`
  instead of deleting on absence.
- Envelope: surface `missing_since` (D7).

## Test plan (build must prove all)
1. **Purge survives:** index a session, move its JSONL out of the walk, search →
   session still returns; `read <ref>` still works.
2. **User-delete still prunes:** `rawclaw delete` → row gone + tombstone written;
   reindex does not resurrect it.
3. **Foreign-source survives local walk:** a row with a different `origin_machine`
   is never pruned by this machine's scan.
4. **Unchanged-file skip preserved:** the mtime/size/fingerprint fast-path still
   skips genuinely-unchanged files.
5. **Migration is in-place:** upgrading an existing DB adds columns + backfills
   WITHOUT a full rebuild (assert row count preserved across the version step).
6. `go build ./... && go test ./...` green; no scope creep.
