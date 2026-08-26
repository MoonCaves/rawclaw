# Design: Instant, Crash-Durable Tag Closeout

Status: implementation-ready design, 2026-08-26.

## Decision

`tag-prep` and `tag-write` must not wait for the consolidated store.

The foreground closeout path will refresh only the requested live session, validate against an
explicit prep snapshot, atomically persist one authoritative tag file, print a durability receipt,
start a deduplicated detached ingest child, and exit. The ingest child will publish the tag file to
the refresh database and consolidated store in the background.

This reuses RawClaw's existing seams, with the narrow corrections called out below:

- `index.PrepareFreshContainer` for a private, source-specific refresh database, after moving its
  global stale-refresh cleanup out of the foreground call;
- `archive.TagFile` for the one-file-per-session authored unit;
- `spawnIngestChild`, detachment, spawn throttling, retry, and the bounded ingest log;
- the transcript vault's fsync and same-directory rename pattern, strengthened so directory-sync
  failures are returned rather than ignored;
- the current incremental earliest-untagged-window and insert-only topic behavior.

It does not add a daemon, service, model dependency, queue database, or second background-worker
subsystem. RawClaw remains a static, local, no-LLM binary.

## Product contract

The primary contract is structural, because a millisecond promise without a bound on the work is
not trustworthy:

1. Closeout never performs work proportional to the number of projects, per-project databases,
   consolidated rows, tombstones, or archive remotes.
2. No hook waits for refresh, tagging, folding, archive work, model work, or a detached child.
3. A successful `tag-write` means the authored tag file is crash-durable. It does not mean the
   rebuildable consolidated cache has already caught up.
4. Consolidated-store contention cannot turn a successful durable tag write into a failed or slow
   closeout.
5. Background publication is idempotent, observable, and retried by ordinary ingest triggers.
6. Without hooks installed, the prep/write pair remains correct. Hooks only accelerate discovery
   and publication.
7. Foreground work does not enumerate or prune other refresh databases. Refresh-cache housekeeping
   is background maintenance.

The release benchmarks use a checked-in fixture and record machine details. On a warm local SSD,
the targets are:

| Operation | Target | Required scaling property |
| --- | ---: | --- |
| Hook catalog write plus detached spawn | p95 <= 25 ms | No child wait |
| `tag-write` durable receipt, payload <= 60 KB | p95 <= 25 ms; p99 <= 100 ms | Independent of consolidated-store size and lock state |
| Unchanged-session `tag-prep` | p95 <= 100 ms | Proportional only to one session |
| Changed-tail `tag-prep`, 5 MB / 2,500 messages | p95 <= 250 ms | Proportional only to that transcript and a 60 KB dump |
| Background publish when the store is idle | p95 <= 5 s | No unrelated session is refreshed or folded |

The no-global-work invariants are hard release gates. The numeric targets are benchmark gates on
the reference fixture, not claims about every disk and machine.

## Evidence and uncertainty

### Confirmed failure classes

The incident reports describe two independent failures.

First, a missing consolidated row can invoke `LocateSession(..., more)`. Building that fallback
scope eagerly indexes Codex and Antigravity containers. A lookup for one session can therefore fold
many unrelated databases before the target has even been resolved. The current entry point remains
visible in `internal/cli/cmd_tag.go:475-479`; the fallback and eager resolution path are in
`internal/agentproto/agentproto.go:1759-1775` and `:1863-1903`.

Second, one solo closeout completed its logged merge in milliseconds and then stalled in the
uninstrumented post-merge tail. `consolidateOne` starts its merge timer with a defer after an earlier
`DETACH` defer, so Go's last-in-first-out defer order prints the merge duration before `DETACH`
runs. `SyncConsolidatedFrom` then has pruning, watermark stamping, connection close, and fence
release work that is not attributed separately. See `internal/index/consolidated.go:612-744` and
`internal/index/consolidated.go:534-572`.

These failures stack, but neither depends on the other. The fast path must remove both from the
foreground rather than betting the user experience on choosing the right one.

### Reconciled disputed evidence

- Investigations 1 and 2 rank tombstone pruning first. Investigation 3 ranks `DETACH` first.
- Investigation 3's APFS clone completed an unchanged fold in about 0.84 seconds and a changed
  fold in about 1.53 seconds, with about 31 milliseconds attributed to merge work. That does not
  reproduce the multi-minute solo stall.
- Investigation 1 used live `EXPLAIN QUERY PLAN` evidence across all six pruned tables and found
  full scans. Investigations 2 and 3 inspected or timed narrower messages-table cases.
- The reported 45-second prune and 6.67-millisecond prototype numbers were not rerun in the
  investigation that cited them, and their exact originating benchmark is unclear. They must not
  be treated as a release benchmark.
- Store totals, message counts, and cache-file counts differ because the investigations sampled a
  growing store at different times and sometimes counted different directory depths.
- The design claim that tombstone deletes are indexed and sub-millisecond is contradicted by the
  observed query plans. `docs/design/tombstone-consolidation-contract.md:33-37` must not be used as
  performance evidence until a reproducible benchmark and plans confirm it.

Therefore the exact solo-stall root cause remains unproven. The implementation must instrument
`DETACH`, pruning, watermark stamping, connection close, checkpoint work, and fence release as
separate phases.

### Other findings that affect the design

- One investigated Claude root had five background-agent JSONLs on disk but only one in the
  indexes. Root-only publication without descendant discovery would preserve that gap.
- A truncated `head` pipeline probably caused EPIPE or SIGPIPE, but the exact signal was not
  captured. It is not evidence that RawClaw itself deadlocked.
- Duplicate writer processes were observed. Publication must deduplicate spawns and remain safe
  when duplicate children still occur.
- One long session spent roughly 3 minutes 39 seconds composing model output. That is a separate
  model-latency class; RawClaw cannot make model generation instantaneous.

## End-to-end pipeline

```text
background tagging agent
        |
        | rawclaw tag-prep <full-session-id>
        v
exact live source -> private refresh DB -> prep manifest -> bounded dump
        |                                      |
        | model chooses segments               | token + exact snapshot
        v                                      v
rawclaw tag-write <full-session-id> --prep <token>
        |
        | validate small DB + compare-and-swap local tag file
        v
fsync temp -> atomic rename -> fsync directory -> durability receipt -> detached ingest -> exit
                                                                      |
                                                                      v
                                                           background publisher
                                                                      |
                                 authoritative tag file -> refresh root + descendants
                                                                      |
                                             apply tags to refresh DB -> fold
                                                                      |
                                      consolidated search cache + later archive push
```

The durable tag file is the commit point. Everything below it is rebuildable publication.

## Foreground behavior

### `tag-prep`

`tag-prep` performs the following steps in order:

1. Resolve one requested live session. The default path must never call a fallback that constructs
   all project scopes.
2. Call `index.PrepareFreshContainer` for that exact container. Remove its current
   `pruneStaleRefreshDBs` call from this path and run that cleanup from background ingest. The
   exact-container helper already calls the private `ensureIndexedContainers` implementation rather
   than the publishing `EnsureIndexedContainers` wrapper; preserve that distinction. Do not call
   `index.SyncConsolidatedFrom`.
3. Read the latest locally-authored tag file, if one exists, and combine it with effective existing
   topics for window calculation. The durable file wins over a stale local database copy. If no
   durable file exists yet, capture the exact locally-authored legacy state into the manifest as
   described under migration; `tag-write` must not rediscover it.
4. Compute the earliest contiguous untagged displayable window using the existing 60 KB budget.
5. Write a short-lived prep manifest before printing a dump. If the manifest cannot be written,
   fail without printing a dump that cannot be safely committed.
6. Print the bounded dump, the full session ID, the opaque prep token, and the exact write command:

   ```text
   rawclaw tag-write <full-session-id> --prep <opaque-token>
   ```

7. Exit. Do not fold, archive, checkpoint, or wait on a child.

The existing `tagrefresh.go` behavior prints before folding but still folds synchronously afterward.
That loop at `internal/cli/tagrefresh.go:35-68` is removed from the foreground path.

If freshness cannot be proven for a live transcript, `tag-prep` fails closed. It does not print a
known-stale dump. A retained-only session already present in the consolidated store may still be
prepared from that retained record. A default miss must return a recovery message rather than
starting an all-project sweep. An explicit `--allow-scan` escape hatch may retain the old exhaustive
behavior for manual recovery, but hooks and generated instructions must never use it.

### `tag-write`

Topic writes require the prep token printed by `tag-prep`. `tag-write` performs the following work:

1. Load `tag-prep/<token>.json` directly. Do not call `agentproto.LocateSession` and do not construct
   project scopes.
2. Require the command's full session ID to match the manifest exactly.
3. For a live-source manifest, re-stat and fingerprint the transcript. Reject a changed path, size,
   mtime, or fingerprint. A retained-only manifest has no live path and is validated entirely from
   its prepared message snapshot.
4. For a live-source manifest, open the private refresh database read-only and revalidate its
   watermark. Do not open the consolidated store from `tag-write`.
5. Recompute the effective-topic hash from the current authoritative file, or from the immutable
   manifest seed when no file existed at prep, and recheck the local-file hash. Reject either
   mismatch without querying legacy databases.
6. Parse the submitted segments and resolve their starts against the prepared messages.
7. Reject starts outside the prepared window, overlapping existing segments, ambiguous prefixes,
   duplicate or reordered starts, and any segment set that does not cover the prepared window.
8. Under a short per-session file lock, recheck the local tag-file hash, merge the new insert-only
   segments into the whole locally-authored set, and atomically replace the authoritative tag file.
9. Print a receipt containing the session ID, tag-file revision, segment count, and `publication:
   pending`.
10. Start a revision-aware, deduplicated detached `rawclaw ingest <full-session-id>` and exit without
    waiting.

The normal path never opens the consolidated store read-write, never acquires
`consolidated.lock`, and never performs `ATTACH`, `DETACH`, pruning, checkpointing, or archive work.

`--retag-all` remains explicit. It replaces this machine's complete authored segment set in the
tag file; it does not delete another machine's archived file. `--routine` updates the verdict while
preserving real topic segments. A routine verdict must never erase real tags.

For compatibility, `--routine` may accept an exact full session ID without a prep token only when the
authoritative local file already exists, because that path is a single canonical file lookup and
does not resolve message boundaries. It is an atomic verdict-only patch: acquire the same per-session
writer lock before reading the current file, preserve the exact segment set read under that lock,
bump the revision, and atomically replace the file. It must never write a whole-file snapshot captured
before the lock. If the file is absent, `--routine` requires a prep token so the manifest carries any
legacy local seed; it must not rediscover databases. Short IDs without a manifest are rejected with a
command to run `tag-prep`. Topic JSON without `--prep` is rejected rather than silently using a
mutable snapshot.

#### Normative `tag-write` input

Topic writes read exactly one JSON value from standard input: a top-level array of objects with this
schema. Unknown fields and trailing JSON values are rejected.

```json
[
  {
    "start_uuid": "a1b2c3d4",
    "topic": "closeout latency investigation",
    "summary": "Compared the foreground and deferred publication paths."
  }
]
```

- `start_uuid` is required. It may be the exact message UUID or a case-sensitive prefix of at least
  eight characters, but it must resolve uniquely inside the prepared displayable-message set.
- `topic` is required and must be non-empty after trimming. `summary` is optional.
- `end_uuid`, `tagged_at`, `origin_machine`, revision fields, and verdict fields are output state and
  are not accepted from stdin. The writer derives each `end_uuid` from the next segment start and
  the manifest window.
- Starts are strictly increasing. The first start must equal the manifest window start, and the
  derived final end must equal the window end; this is the exact meaning of complete coverage.
- An empty array is rejected. `--routine` reads no topic JSON.

`--retag-all` requires a token produced by `tag-prep <full-session-id> --retag-all`. That manifest
has `mode: "retag-all"`, covers the complete displayable session, and is accepted only when the
bounded dump contains every displayable message. If the condensed session exceeds the 60 KB dump
budget, prep fails with an explicit message instead of replacing the old set from a partial view.
The submitted array must cover the complete manifest window and replaces only this machine's
authored set.

`--routine` reads no stdin. With an exact full session ID and an existing authoritative file, it may
omit the prep token and performs the locked verdict-only patch above, preserving the latest real
segments even when a topic writer races it. If the file does not exist, the command requires
`--prep <token>` from `tag-prep` and uses the manifest's exact-session local-only seed. A short or
ambiguous ID without a manifest is rejected.

`payload_sha256` is not a hash of raw stdin bytes. After validation and prefix resolution, marshal a
dedicated, map-free replay struct with `encoding/json` and hash those bytes with SHA-256. Its fields,
in order, are `version`, `mode`, `session_id`, and `segments`; every segment contains the full resolved
`start_uuid`, derived full `end_uuid`, exact decoded `topic`, and exact decoded `summary`. Generated
`tagged_at`, revision, prep token, origin, the legacy seed, and the resulting merged whole-file state
are excluded. Therefore JSON whitespace, object-key order, and short-versus-full UUID spelling that
resolve to the same boundaries replay identically, while any semantic tag change conflicts.

### Replay behavior

The same prep token and identical payload are idempotent. If the durable file already records that
token and payload hash, `tag-write` prints the existing revision receipt and starts publication if
needed. Reusing the token with different content is rejected.

This makes a caller retry safe across a lost stdout response or process termination after the
rename.

## Resolution order

The closeout resolver is shared by prep and targeted ingest, but the writer normally bypasses it by
using its manifest. Extract one targeted resolver from the overlapping logic in
`resolveIngestMatches` and `refreshTagSession`; do not maintain two lookup ladders.

For `tag-prep` and `rawclaw ingest <session>`, resolve in this order:

1. Exact full-ID session catalog entry.
2. Consolidated backing metadata probe with a nil fallback. This is a read-only hint, never an
   invitation to build all scopes.
3. Source-specific stem or other bounded direct-path resolution.
4. Retained consolidated history for prep and retained-only background publication.
5. Existing unbounded `Discover()` calls only behind explicit `--allow-scan`.

The default resolver must never call `discoverTagSources`, `discoverAllIngestSources`, or a scope
builder that enumerates all containers and filters afterward. Prep may return either a live target or
an exact retained-only target. Targeted ingest uses the same result type and sends retained-only
targets through the background direct-apply branch below instead of pretending they have a live
backing path.

An exact match wins over prefix matches. Ambiguous prefixes fail with candidate IDs. The generated
closeout instructions always pass the full session ID.

The current session catalog is already the strongest O(1) ingest path in
`internal/cli/cmd_ingest.go:152-210`. The Claude and Codex SessionStart scripts currently launch
ingest before the catalog entry is placed (`internal/cli/setup.go:49-74` and `:128-151`). Reverse
that order so the child can use the catalog on its first lookup.

## On-disk records

### Authoritative local tag file

Location for all newly authored local files:

```text
$XDG_DATA_HOME/rawclaw/tags/v2-<lowercase-hex-sha256-of-full-session-id-utf8>.json
```

The key is exactly `"v2-" + hex(sha256([]byte(fullSessionID)))`. The full ID remains embedded in the
JSON and is the authority; loaders recompute the filename and reject a mismatch as corruption. If a
file already exists at that hash with a different embedded ID, the operation fails closed and never
overwrites it. This is fixed-width, portable, and immune to separators, dot components, Unicode
normalization collisions, and the current lossy `durable.sanitize` mapping. The same key is used for
new own-machine archive tag files. Archive readers continue to accept legacy v1 nested filenames.

For each machine and requested session, archive ingest probes the canonical v2 path first. A valid
v2 file wins and any legacy file for the same machine/session is ignored. If v2 is absent, ingest may
use the legacy path only after `filepath.Rel` proves it remains inside that machine's tag directory.
If v2 exists but is corrupt, its filename/content hash mismatches, or
its embedded `session_id` differs, fail closed for that machine and do not fall back to possibly stale
v1 state. When both valid forms exist with different content, use v2 and emit a migration warning;
never feed both into cross-machine conflict resolution. After a successful own-machine v2 export,
remove that session's legacy own-machine file so the migration converges.

The file extends the existing backward-compatible `archive.TagFile` JSON shape:

```json
{
  "version": 2,
  "session_id": "full-session-id",
  "origin_machine": "",
  "revision": "opaque-random-128-bit-value",
  "prepared_from": "opaque-prep-token",
  "payload_sha256": "hex",
  "written_at": "2026-08-26T12:34:56.123456789Z",
  "segments": [
    {
      "start_uuid": "full-message-uuid",
      "end_uuid": "full-message-uuid",
      "topic": "topic",
      "summary": "inconclusive summary",
      "tagged_at": 1787747696
    }
  ],
  "verdict": {
    "verdict": "routine",
    "source": "agent",
    "tagged_at": 1787747696
  }
}
```

`origin_machine` is empty in the local authoritative copy. Archive export stamps the configured
machine ID exactly as it does today. Unknown JSON fields are ignored by v1 readers, and v2 readers
accept v1 files with absent metadata.

The file holds this machine's complete authored set for the session. Incremental writes therefore
merge into and atomically replace the file; they do not append independent fragments.

The commit sequence is:

1. create a temporary sibling;
2. write the complete JSON plus trailing newline;
3. `fsync` the temporary file;
4. rename over the destination;
5. `fsync` the destination directory.

Factor the transcript-vault sequence in `internal/durable/durable.go:209-272` into a small shared
internal package rather than cloning it, but first strengthen `fsyncDir`: opening the directory,
syncing it, and closing it all return errors. A success receipt is printed only after directory sync
succeeds. On a filesystem that does not support directory sync, return an explicit non-durable error;
the rename may already have landed. An identical retry that finds matching content must reopen and
`fsync` the destination file, close it successfully, then reopen, `fsync`, and close the parent
directory before issuing a success receipt. If any proof step still fails, retain the non-durable
error and leave publication pending; merely recognizing matching bytes is never enough to
acknowledge durability.

### Prep manifest

Location:

```text
$XDG_CACHE_HOME/session-search/tag-prep/<opaque-token>.json
```

The manifest is cache state, not the authored record. It expires after 15 minutes by default and
may be removed after successful use. A missing manifest means "rerun tag-prep," never "guess the
snapshot."

Required fields:

```json
{
  "version": 1,
  "token": "opaque-random-128-bit-value",
  "session_id": "full-session-id",
  "mode": "incremental",
  "snapshot_kind": "live",
  "source": "claude",
  "transcript": {
    "path": "source path",
    "mtime": 1787747600.123,
    "size": 1234567,
    "fingerprint": "existing-file-fingerprint"
  },
  "refresh_db_path": "private-refresh-db",
  "refresh_db_identity": "source/session/path identity hash",
  "prepared_message_uuids": ["full-message-uuid"],
  "prepared_messages_sha256": "hex",
  "effective_topics_sha256": "hex",
  "local_tag_file_sha256": "hex-or-empty",
  "local_seed": {
    "sha256": "hex-or-empty",
    "segments": [],
    "verdict": null
  },
  "window": {
    "start_uuid": "full-message-uuid",
    "end_uuid": "full-message-uuid",
    "displayable_messages": 321,
    "dump_byte_cap": 60000,
    "formatter_version": 1
  },
  "created_at": "2026-08-26T12:34:00Z",
  "expires_at": "2026-08-26T12:49:00Z"
}
```

Hashes use canonical field ordering and SHA-256 from the standard library. Topic-generation hashes
cover boundaries, topic, summary, and origin, but exclude cosmetic database row IDs.
`local_seed` is empty when an authoritative file exists. Otherwise it contains the exact canonical
local-only legacy set selected during prep, so the foreground writer does not reopen or scan legacy
databases. Its hash participates in replay and snapshot validation.

`snapshot_kind` is `live` or `retained`. A live manifest requires the transcript and refresh fields.
A retained manifest omits them, records the exact ordered prepared UUIDs plus a canonical hash of
the prepared displayable messages, and is produced from one exact retained-session query. The UUID
list is sufficient for prefix resolution and derived ends, so `tag-write` never reopens the
consolidated store. Publication of a retained-only file is background work.

### Publication receipts

The existing bounded `ingest.log` remains the process receipt stream. Publication lines are
structured, one line per phase:

```text
tag-publish session=<id> revision=<rev> phase=<phase> status=<status> duration_ms=<n> error=<text>
```

Phases are `resolve`, `refresh`, `apply-tag`, `attach`, `merge`, `detach`, `prune`, `stamp`,
`close`, and `archive-trigger`. Errors are retained in the rotating log. The log is the observability
surface; do not add a second last-applied-revision cache until a concrete status query requires it.

## Background publication

The publisher is the existing `rawclaw ingest <session>` child with one extended responsibility,
not a new command or daemon.

For a live targeted session it performs:

1. Resolve the exact root through the catalog-first resolver.
2. Discover the root and its descendants.
3. Call `PrepareFreshContainer` for each selected container. Never call a refresh function for an
   unrelated container.
4. Open the root refresh database read-write and apply the latest authoritative tag file as one
   session unit.
5. Close the refresh database before folding.
6. Call `SyncConsolidatedFrom` for the prepared databases.
7. Reread the tag file revision. If it changed during publication, repeat the root apply/fold for
   the new revision, bounded to four immediate loops. A still-changing file remains pending for
   the next trigger.
8. Emit phase receipts and exit honestly. A busy fence or post-merge error is a failed publication,
   not a failed authored tag.

For a retained-only target, skip discovery and refresh. Acquire the consolidated fence in the
background child, verify that the exact session still exists, apply the authoritative file as one
session unit, reread the revision with the same bounded retry rule, emit receipts, and exit. If the
session is absent or the fence is busy, leave the file pending. This compatibility branch never runs
inside `tag-write` and never turns the consolidated store back into authored state.

Every targeted ingest checks the durable tag file even when the transcript watermark is unchanged.
The tag file remains authoritative and is never "drained" or deleted after publication. A deleted
or rebuilt database can therefore be repopulated from it.

`tag-write` extends the existing spawn token with a trigger revision. The same session and revision
is suppressed inside the current spawn window; a newly written revision is allowed immediately
even if SessionStart spawned an earlier ingest. Keep one marker per session rather than leaking one
marker per revision.

The current `Stat` then `WriteFile` throttle is neither atomic nor revision-aware. Replace it with a
non-blocking per-session `TryLock` claim using the existing `gofrs/flock` dependency and the same
hashed session key as tag files. While holding the claim, read a marker containing
`{session_id, revision, spawned_at}`.
Suppress only an identical revision inside the window. Otherwise start the child while the claim is
held, atomically store the new marker only after `cmd.Start` succeeds, release the claim, and never
wait for the child. A lost claim or spawn failure leaves the durable file pending and returns from
the CLI without converting an acknowledged tag into a failure.

Ordinary SessionStart ingest, stale-read self-healing, explicit `rawclaw ingest`, archive activity,
and future scheduled maintenance all retry a pending durable file. A successful detached spawn is
an acceleration, not the only retry path.

## Descendant completeness

A targeted root publication includes descendants because the incident corpus proved that the root
and background-agent transcripts can diverge in index coverage.

Add an optional source capability such as:

```go
type RelatedSource interface {
    DiscoverRelated(root Container) ([]Container, error)
}
```

The returned set is the complete authoritative lineage snapshot for that root at discovery time. It
includes the root exactly once and only its descendants. Implementations deduplicate by exact full
session ID plus canonical backing path, reject conflicting duplicate IDs, and return a stable
parent-before-child order with ID as the tie-break. A disappearing root or partial discovery is an
error; the publisher leaves the file pending rather than publishing an asserted-complete subset.

- Claude derives its `subagents/` tree directly from the root path, so work is proportional to that
  lineage.
- Antigravity reuses the existing parent header and invoked-conversation extraction.
- Codex uses known catalog/backing entries first. If it must enumerate rollout headers, it filters
  to the requested lineage before any refresh or fold.
- Sources without the optional capability publish the exact root only and log
  `descendants=unsupported`; they must never fall back to refreshing every discovered container.

Each returned container gets its own
`RefreshDBPath(sourceID, container.ID, container.Path)` and is passed as a singleton to
`PrepareFreshContainer`. Do not place a root-plus-descendant subset into a shared scope database:
`EnsureIndexedContainers` requires a complete set for its database and may treat omissions as
absence. Per-container refresh databases make the singleton the complete set and prevent an omitted
descendant from being pruned or retention-stamped accidentally.

A later adapter can implement this seam without changing index or publication logic.

## Concurrency and crash semantics

### Concurrent writers

Use one short per-session file lock for every read-modify-replace sequence, including tokenless
`--routine`. The lock wait is bounded to 100 milliseconds. Reuse the repository's existing
`github.com/gofrs/flock` dependency; do not add another locking library. A manifest-backed write
compares the manifest's local-file hash again inside the lock. A tokenless routine write reads the
current file only after acquiring the lock and changes only its verdict. This prevents prepared
writers from silently overwriting each other and prevents a routine update from restoring stale
segments over a concurrent topic write.

A hash mismatch returns a conflict and tells the tagging agent to rerun `tag-prep`. It never merges
two independently chosen segmentations into a franken-set.

### Process termination

- Before rename: the old tag file remains authoritative; an orphan temporary file is ignored.
- After rename but before stdout: retrying the same token and payload returns the existing receipt.
- After receipt but before child spawn: the durable file remains and the next ingest retries it.
- During database apply: SQLite transaction rollback leaves the file pending.
- During fold, `DETACH`, prune, stamp, close, or checkpoint: the refresh database and tag file remain
  available for retry.
- During archive push: the local authoritative file remains; git transport failure does not affect
  local durability.

### Delete semantics

An explicit user delete first materializes one root-plus-descendant deletion set. Union exact/prefix
matches from live related-source discovery, indexed session IDs, durable/local tag enumeration, and
the own-machine archive clone; record a tombstone for every discovered ID. The shared delete matcher
also treats every root tombstone as matching `root` and `root/…`, so a descendant missed during the
initial expansion is still removed when a later store or archive exposes it.

Use that same set and matcher for durable transcript removal, database-row removal, local
authoritative tag removal, archive-clone removal, exporter filtering, and tombstone persistence. Tag
removal enumerates embedded `session_id` values; hashed filenames are never prefix-matched.
`Archive.removeTombstoned` removes both v2 hashed files and legacy v1 paths from this machine's clone
before `git add` and push, never from foreign-machine directories. Delete is complete only after a
later archive push can commit those removals; pull/rebuild must not resurrect root or descendant tag
state.

## Hook strategy

Hooks may run only a millisecond-scale catalog write and detached spawn. They must not perform
refresh, dump generation, tagging, folding, archive work, or model work themselves.

Vendor-documented lifecycle limits make hooks unsuitable as the correctness foundation:

- Codex `SessionEnd` is synchronous even when `async: true`; its default timeout is one second and
  its supported maximum is three seconds.
- Codex cancels unfinished background hooks and discards undelivered output when the session ends.
- Claude Code `SessionEnd` has a synchronous exit budget, 1.5 seconds by default.
- Claude Code and Codex `Stop` are turn-level hooks, not a durable completion boundary.
- Antigravity command hooks are synchronous with a 30-second default timeout; its `Stop` payload
  provides `fullyIdle`, but the command still cannot own durable background completion.

References:

- <https://learn.chatgpt.com/docs/hooks#limitations>
- <https://learn.chatgpt.com/docs/hooks#config-shape>
- <https://code.claude.com/docs/en/hooks>
- <https://antigravity.google/docs/hooks/>

Do not install a per-turn pre-warm hook in the first release. The private refresh database already
provides the reusable session-local cache, and parsing on every turn creates repeated work plus a
teardown race. Keep the existing SessionStart discovery/ingest hook, but write its catalog entry
before launching ingest. The generated POSIX script creates a sibling claim directory atomically;
only the winner writes the complete catalog entry via temp-plus-rename and then spawns ingest.
Existing entries and losing concurrent invocations do not spawn. A stale claim is background
cleanup, never hook work. Any future Stop/SessionEnd integration may only launch the same detached,
revision-aware ingest child and exit.

Antigravity's existing discovery hook does not provide the same catalog/ingest state as Claude and
Codex. Add equivalent catalog acceleration only when its payload supplies a stable session and
transcript identity; otherwise keep it as a banner and rely on the resolver.

## Search, outline, and archive visibility

The durability receipt is immediate; search visibility becomes current after background
publication. Until then, normal search may show the prior topic state. This lag is explicit and
observable, not silently called fresh.

The current range-based `TopicForMessage` behavior must be preserved: a message outside every
stored segment returns no topic. Tests must assert the effective reader result for omitted middle
and tail messages, not only stored `end_uuid` values.

Archive export changes from database-first to file-first:

1. Enumerate the shared local tag root, validate each filename against its embedded full session ID,
   and build the complete authoritative-file session set.
2. Export those `archive.TagFile` payloads to canonical v2 filenames, stamping the configured
   machine ID.
3. Query legacy local databases only for sessions absent from that file set. Per-source local
   databases precede the consolidated fallback; every query requires
   `COALESCE(origin_machine,'')=''`. Conflicting locally-authored whole sets fail export visibly
   instead of using traversal-order "first DB wins."
4. Keep the existing deterministic cross-machine resolver and conflict reporting.

This makes authored files survive cache deletion while retaining all pre-migration tags.

## Migration and compatibility

1. Existing archive tag files without `version`, `revision`, or prep metadata remain valid v1
   files.
2. On the first durable write for a session, `tag-prep` captures one exact-session local-only seed in
   its manifest. Prefer the live source database identified by the resolved backing/session-source
   metadata when it has local rows; otherwise use an exact-session consolidated query with
   `COALESCE(origin_machine,'')=''`. Never enumerate archive/foreign scopes, and never copy a
   non-empty foreign origin. If multiple local databases disagree on the whole authored set, fail
   closed and require an explicit `--retag-all` rather than unioning them. `tag-write` merges the new
   write into the manifest seed only when the authoritative file hash was empty.
3. During migration, archive export is file-first and then falls back to database-only sessions.
4. Background publication writes the authoritative file back into the refresh database, so old
   readers continue to work without an overlay layer.
5. The CLI help and generated closeout banner switch to full session IDs plus `--prep <token>` in
   the same release as the writer change.
6. Remove the synchronous post-prep fold and post-write fold only after the durability, replay,
   locked-store, and eventual-publication tests pass.
7. Keep `--allow-scan` as an explicit recovery escape hatch, not a default lookup tier.

No migration deletes old database rows. They become rebuildable copies of the durable file.

## Consolidation instrumentation and tombstone work

Decoupling is required even if every current slow SQL path is later optimized. Separately:

1. Time and log `ATTACH`, merge, `DETACH`, tombstone load/prune, watermark stamp, connection close,
   checkpoint, and fence release independently.
2. Make the merge timer include only the merge by calling its completion function explicitly,
   rather than relying on defer order.
3. Add a reproducible tombstone benchmark with realistic table sizes and descendants.
4. Capture `EXPLAIN QUERY PLAN` for all six delete tables. The release gate is no full-table scan
   per tombstone on corpus-sized tables.
5. Batch pruning in one transaction and use indexable exact/range predicates. Preserve escaped
   literal semantics for `_`, `%`, and `\\` and preserve descendant deletion.
6. Do not publish the disputed 45-second or 6.67-millisecond numbers as current performance unless
   the checked-in benchmark reproduces them.

Tombstone optimization is required for the background-publication freshness target, but it is not
allowed to recouple `tag-write` to the consolidated store.

## Implementation file map

| File | Change |
| --- | --- |
| `internal/cli/tagrefresh.go` | Return an exact prepared-session object; stop folding after the dump; create and print the prep manifest/token; share the targeted resolver with ingest instead of maintaining a second ladder. |
| `internal/cli/cmd_tag.go` | Add `--prep`; validate the manifest and prepared window; write the durable file instead of a database; preserve `--retag-all` and verdict semantics; spawn publication. |
| `internal/cli/tagmanifest.go` | New small manifest codec, expiry cleanup, canonical topic hashing, and transcript/watermark validation. |
| `internal/cli/tagfile.go` | New shared local tag-root enumerator plus authoritative-file load, compare-and-swap merge, replay, receipt, delete, and database-apply helpers using `archive.TagFile`. |
| `internal/cli/cmd_lifecycle.go` | Invoke authoritative local tag-file removal for exact and descendant session IDs during explicit delete. |
| `internal/lifecycle/lifecycle.go` | Materialize the root-plus-descendant deletion set and expose one exact-or-descendant matcher shared by database, local-file, exporter, and archive cleanup. |
| `internal/archive/tags.go` | Add backward-compatible v2 metadata and exported codec helpers; use canonical hashed filenames and the shared crash-durable atomic writer. |
| `internal/archive/tagapply.go` | Probe canonical v2 then contained legacy paths per machine; validate embedded IDs, suppress duplicate v1/v2 inputs, and fail closed on a corrupt present v2 file. |
| `internal/archive/delete.go` | Remove tombstoned exact/descendant own-machine tag files, including legacy paths, before archive commit. |
| `internal/cli/tagexport.go` | Export durable files first, then exact-session local-origin legacy database sets; replace traversal-order first-wins behavior with deterministic conflict detection. |
| `internal/atomicfile/atomicfile.go` | Extract the vault's fsync + same-directory rename + directory-fsync primitive for reuse. |
| `internal/durable/durable.go` | Use the extracted atomic-file primitive and propagate directory open/sync/close errors. |
| `internal/paths/paths.go` | Add the canonical fixed-width v2 tag key and local tag-root helpers; do not reuse the lossy transcript sanitizer. |
| `internal/source/source.go` | Add the optional related-container capability. |
| `internal/source/claude/claude.go` | Return the complete root-plus-`subagents/` lineage with stable ordering. |
| `internal/source/codex/codex.go` and `meta.go` | Resolve known backing/catalog entries first; add bounded lineage-header lookup without refreshing unrelated rollouts. |
| `internal/source/antigravity/antigravity.go` | Return the complete parent-header/invoked-conversation lineage. |
| `internal/source/goose/goose.go` | Declare exact-root-only support unless a bounded lineage relation is proven; honor Goose opt-in. |
| `internal/cli/cmd_ingest.go` | Own the shared targeted resolver; apply the latest tag file between refresh and fold; ingest only root plus descendants; retry a changed revision. |
| `internal/cli/bg_ingest.go` | Make spawn tokens trigger/revision-aware while retaining one marker per session and the existing detached child/logging. |
| `internal/cli/setup.go` | Atomically claim and write SessionStart catalog entries before spawning ingest; update closeout instructions to use the prep token; do not add a heavy Stop hook. |
| `internal/index/containers.go` | Keep `PrepareFreshContainer` non-publishing and exact-only; move `pruneStaleRefreshDBs` to background ingest/maintenance. |
| `internal/index/consolidated.go` | Add explicit post-merge phase timings; make `DETACH` attribution honest; optimize pruning only with benchmark/query-plan proof. |
| `internal/store/topics.go` | Preserve exact range semantics and the insert-only/replace primitives; no foreground consolidated write. |
| `README.md` | Document the durability receipt, eventual publication, full-ID/token flow, status/log recovery, and `--allow-scan`. |
| `ROADMAP.md` | Correct hook-lifecycle claims and point future recap/closeout work at the detached-publication contract. |

Names may be adjusted during implementation, but ownership and boundaries above are fixed.

## Required tests

### Foreground correctness

- `tag-prep` writes a manifest before any dump bytes.
- A live transcript mutation between prep and write is rejected.
- A topic-generation or local-file-hash mutation between prep and write is rejected.
- A token/session mismatch, expired token, missing token, and replay with different payload are
  rejected.
- Identical replay returns the same revision receipt.
- Starts outside the window, ambiguous prefixes, reordered starts, overlap, and incomplete window
  coverage are rejected.
- Incremental writes cover only the earliest contiguous untagged window and preserve earlier local
  segments.
- `--retag-all` replaces only the local authored set.
- `--routine` preserves real segments.
- `TopicForMessage` returns no topic for omitted middle and tail messages.
- Tool-only, envelope-only, and bare-thinking messages remain non-displayable as intended.
- The stdin decoder rejects unknown fields, trailing JSON, submitted `end_uuid`, short ambiguous
  prefixes, and a first segment that starts after the prepared window start.
- `tag-prep --retag-all` produces a complete-session manifest or fails before printing a partial
  retag dump.
- A retained-only prep stores the exact ordered UUID snapshot; `tag-write` completes without opening
  consolidated storage, and only the detached publisher acquires the consolidated fence.

### Latency isolation

- Hold `consolidated.lock` longer than the old 30-second wait. `tag-write` must still produce a
  durable receipt inside the benchmark bound.
- Seed hundreds of unrelated scope databases. Neither prep nor write may open or modify them.
- With catalog, backing metadata, and direct-path lookup absent, the default resolver returns the
  recovery message without calling adapter-wide `Discover()`; only `--allow-scan` enables it.
- Seed thousands of stale refresh DB files. Foreground prep does not enumerate or prune the refresh
  directory; background ingest does.
- Seed a multi-gigabyte-equivalent synthetic consolidated schema and many tombstones. Foreground
  syscall/SQL traces must be unchanged.
- Fail child spawn after the durable rename. The command succeeds with publication pending, and a
  later ingest publishes the file.

### Concurrency and crash safety

- Race two writers from the same manifest; one durable set wins and the other reports a snapshot
  conflict or idempotent replay.
- Race a tokenless `--routine` verdict patch with a topic write; the final file contains the newest
  topic segments and the verdict, never a stale segment snapshot.
- Race a publisher with a second tag revision; the final refresh and consolidated stores contain
  the newest whole set, never a mixed set.
- Kill subprocesses before fsync, after fsync, after rename, after receipt, during database apply,
  and during fold. On restart, the result is either the old whole file or the new whole file, and
  pending publication is recoverable.
- Run duplicate detached children and the race detector; publication remains idempotent and no
  child or goroutine is leaked.
- Race same-revision and different-revision spawn claims. Exactly one same-revision child starts,
  while a new revision is eligible immediately; marker writes occur only after successful start.
- Inject directory open, sync, and close failures. No durability receipt is printed, and an
  identical retry recognizes a rename that already landed.

### Descendants, archive, and hooks

- A root plus four background-agent transcripts publishes all five and no unrelated container.
- A related-source result is root-inclusive, duplicate-free, stable, and complete; a simulated
  partial discovery publishes nothing. Each result uses its own refresh DB, so omitted descendants
  are never treated as absent from a shared database.
- Archive export prefers the durable file, falls back to legacy DB rows, and never claims foreign
  rows as local.
- Distinct hostile or Unicode session IDs cannot collide or escape the tag root; a filename/content
  hash mismatch fails closed; legacy v1 archive paths still read.
- Mixed local/foreign legacy rows seed only empty-origin state, and conflicting local whole sets
  require explicit retagging.
- V1 and V2 archive files resolve identically when their tag content is identical.
- Delete removes transcript, local tag file, index rows, own-machine archived tag files, and
  archived-export eligibility; delete -> push -> pull -> rebuild cannot resurrect tag state.
- Deleting a root with four descendants materializes and tombstones the full set; the same matcher
  removes every descendant v1/v2 tag through push, pull, and rebuild even if one descendant appears
  only in the archive clone.
- Concurrent Claude and Codex SessionStart scripts atomically create one complete catalog entry
  before launching exactly one ingest child.
- Hook install/eject preserves foreign entries. Generated hooks remain POSIX `sh` and resolve the
  binary by the baked absolute path with a `command -v` fallback.

### Consolidation evidence

- Phase logs prove merge, `DETACH`, prune, stamp, close, and fence release have separate timings.
- Query-plan tests cover all six tombstone tables.
- Tombstone pruning handles literal `_`, `%`, and `\\`, root-plus-descendant deletion, and runs in
  one transaction.

All implementation commits run `CGO_ENABLED=0 go test -race -count=1 ./...` and the repository lint
gate. Performance claims require repeated benchmark samples, corpus shape, machine details, and
before/after commits.

## Rollout

### Phase 1: Instrument and pin the incident

- Add phase receipts and the unrelated-scope regression.
- Add the locked-consolidated-store latency test.
- Add the realistic tombstone benchmark and query-plan coverage.

### Phase 2: Durable foreground commit

- Add manifests, authoritative tag files, replay safety, and the detached publication trigger.
- Switch `tag-prep` and `tag-write` off synchronous folding.
- Update help and generated closeout instructions in the same commit series.

### Phase 3: Complete publication

- Apply files through targeted ingest.
- Add root-plus-descendant discovery.
- Make archive export file-first and deletion complete.
- Correct the SessionStart catalog/spawn ordering.

### Phase 4: Background freshness performance

- Land tombstone/index changes that the checked-in benchmark proves necessary.
- Verify no post-merge phase can create a growing publication backlog.

### Release gates

- The full race suite and lint pass.
- Foreground commands perform no consolidated write and no all-scope construction.
- Foreground commands do not enumerate the refresh-cache directory.
- A held consolidated fence does not change durable-write latency materially.
- Crash/replay tests prove no acknowledged tag can be lost.
- Root-plus-descendant coverage is complete on Claude and on every adapter that claims related
  discovery support.
- Background publication catches up after a forced lock failure without manual repair.
- Retained-only publication remains background-only and eventually applies or reports pending
  without weakening the foreground latency gates.
- Benchmarks meet the product targets and include truthful uncertainty for unreplicated incident
  measurements.

## Rejected alternatives

- **Increase the CLI timeout.** This makes interruption longer and does not bound the work.
- **Optimize only session resolution.** It fixes the all-project sweep but leaves closeout coupled
  to the consolidated fence and post-merge tail.
- **Optimize only tombstone SQL or `DETACH`.** It improves a suspected phase but does not make the
  latency invariant structural.
- **Pre-warm on every Stop event.** It repeats parsing each turn, depends on runtime-specific hook
  behavior, and can be cancelled at teardown.
- **Append-only pending-tags JSONL.** It introduces multi-writer locking, drain, compaction, replay,
  and corruption semantics while `archive.TagFile` already provides a whole-session authored unit.
- **A resident daemon or external queue.** It violates the single-binary, zero-runtime-dependency
  default.
- **Write directly to `consolidated.db`.** It reacquires the exact large-store fence the foreground
  path must avoid and makes a rebuildable cache the durability authority.
- **Trust a cached dump without a manifest.** It permits transcript or topic drift between prep and
  write.

The durable file plus detached targeted ingest is the smallest existing-path design that makes
closeout latency independent of the machine's accumulated RawClaw history.
