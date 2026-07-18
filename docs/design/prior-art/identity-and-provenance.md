# Prior art: session identity and provenance across machines

Context: rawclaw is about to hold sessions from multiple machines in one
store. Two needs follow: (a) every indexed session must be tagged with its
ORIGIN (which machine, which source tool, which on-disk path) so the prune
step only ever deletes sessions *this* machine actually sourced from its own
live tree — never another machine's; and (b) the same logical session must be
recognizable if it gets ingested from two places, without a bare-id collision.
A sibling agent runtime has a `state.db` with a `UNIQUE` constraint on
session *title* that would already collide if a foreign session were
inserted — so this is a real, previously-hit failure mode, not a
hypothetical.

Current rawclaw state (read directly, cited by path):
- `internal/index/index.go:31-34` — the `sessions` table today is
  `id TEXT PRIMARY KEY, started_at, last_ts, message_count, is_subagent,
  parent_id`. No origin/machine/source-tool column exists. `id` alone is the
  primary key.
- `internal/source/source.go:18-25` — `Container.ID` is documented as
  "already lineage-namespaced by the source," but per-adapter, `ID` is set to
  the bare tool-minted id: `internal/source/claude/claude.go:62` (`ID: sid`,
  the Claude Code transcript UUID from the filename) and
  `internal/source/codex/meta.go:64` (`m.id, _ = p["id"].(string)`, the
  Codex CLI's own `session_meta.id`). Neither is actually machine- or
  source-namespaced yet — both are the raw ids the originating CLI minted.
- `internal/index/index.go:41` — `file_index(path TEXT PRIMARY KEY, mtime,
  size, fp, session_id)` is the only place a filesystem path is tracked, and
  it is local-path-keyed with no machine dimension either.

All claims below are cited to a fetched repo/file/line or a fetched doc URL.
Anything I could not verify is marked NOT VERIFIED.

---

## 1. How distributed/replicated systems assign identity that encodes origin

### Syncthing — device identity is a content hash of the device's own cert; record identity is a (folder, device) tuple, never a bare id

- `DeviceID` is a 32-byte value computed as `sha256.Sum256(rawCert)` — a
  self-certifying identity the device itself mints from its own TLS
  certificate, not assigned by any coordinator:
  `github.com/syncthing/syncthing`, `lib/protocol/deviceid.go:44-48`
  (`NewDeviceID`).
- Folder identity is a separate, independent string field:
  `FolderConfiguration.ID string` — `lib/config/folderconfiguration.go:50`.
- Syncthing never stores a file record keyed by device or folder alone. Its
  on-disk key scheme is explicit about compounding both: `KeyTypeDevice
  <int32 folder ID> <int32 device ID> <file name> = FileInfo` and
  `KeyTypeIndexID <int32 device ID> <int32 folder ID> = protocol.IndexID` —
  `internal/db/olddb/keyer.go:24-49`. Every record is addressed by
  **(which folder, which device produced this view of it)**, not by file
  name alone.

**Relevance to rawclaw:** this is the direct precedent for a compound key.
Syncthing does not try to make a single id collision-proof across every
device; it scopes every record by an explicit origin dimension (device) laid
over the content dimension (folder/file). The device id itself is
self-minted and content-derived (hash of the device's own cert) — nothing
external needs to hand out machine ids.

### Automerge (CRDT) — every op is (origin replica id, local counter), never a bare id

- `ActorId` is a small (≤16-byte) value, one per replica/process, declared
  explicitly to be ordered by its raw bytes because change encoding depends
  on it: `github.com/automerge/automerge`, `rust/automerge/src/types.rs:40-45`.
- The legacy `OpId` is *literally* the tuple `(seq: u64, actor: ActorId)` —
  "or, a Lamport timestamp" in spirit: `rust/automerge/src/legacy/mod.rs:26-32`
  (`pub struct OpId(pub u64, pub ActorId)`).
- The current internal representation keeps the same shape as a packed
  `(u32, u32)` (counter, actor-index into a per-document actor table):
  `rust/automerge/src/types.rs:422-423`.
- JS test tooling for the legacy wire format documents the sort order in
  English: ids are compared "first by counter, then by actorId," i.e.
  Lamport-clock order broken by origin — `javascript/test/legacy/columnar.js:149-158`.

**Relevance to rawclaw:** a CRDT never tries to make a flat id globally
unique by making the id "big enough." It always pairs a **local, monotonic
counter** with a **replica identity**, and origin is structurally part of the
id, not an afterthought. This is the strongest precedent for "encode which
replica produced this in the identity itself," which is literally research
question 1's phrasing.

### Event sourcing — stream/event identity is (category/aggregate type, aggregate id[, version]), never the aggregate id alone

- `github.com/looplab/eventhorizon` (Go event-sourcing library) types every
  event by the triple `(AggregateType, AggregateID uuid.UUID, Version int)`:
  `aggregate.go:44-50` (`AggregateType` type), `eventstore.go:88-95`
  (`EventStoreEvent{Op, AggregateType, AggregateID, AggregateVersion}`), and
  the wire codec explicitly serializes all three together:
  `codec/json/event.go:106-115` (`AggregateType`, `AggregateID string`,
  `Version int` on the same `evt` struct).
- EventStoreDB's documented convention is the same shape spelled as a string:
  `<category>-<id>` stream names, with a system projection deriving the
  category by splitting on `-`. I could not re-fetch the current Kurrent
  docs page for this (404/redirect during this session) — **NOT VERIFIED
  against a live doc URL this session**, flagging it as a well-known but
  unconfirmed-today claim; the `eventhorizon` code above is the verified
  stand-in for the same pattern.

**Relevance to rawclaw:** identical shape to Syncthing/Automerge — a coarse
"what kind / which replica" dimension is carried as its own field(s)
alongside the fine-grained instance id, not folded into it.

### Git — object ids are pure content hashes and deliberately encode NO origin

- A loose object's id is "the SHA-1 or SHA-256 (as appropriate) hash of the
  uncompressed data" — the prefix is only `<type> <size>\0` followed by the
  object bytes: `github.com/git/git`,
  `Documentation/gitformat-loose.adoc:28-35`.
- `hash_object_file()` is the function that computes this:
  `object-file.c:474-476`.

**Relevance to rawclaw:** this is a *counter-example*, and worth stating
plainly because it's tempting to reach for "just hash the content." Git's
object id is intentionally origin-blind: byte-identical content from two
different machines collapses to the *same* id — that's the point (dedup by
content). rawclaw's problem is the opposite: two *different* logical sessions
that happen to share a source-minted id (or the same session legitimately
seen from two machines) must NOT collapse into one row structurally, while
still being recognizable as "the same conversation" for display/dedup
purposes. Content-addressing solves a different problem than origin-scoped
identity; don't reach for a content hash as the primary key here.

---

## 2. How code-search indexers tag document provenance

### Zoekt — a `Source` field exists *specifically* to detect orphaned shards, which is rawclaw's exact prune-safety problem

- `zoekt.Repository` (shard-level metadata every document in that shard
  belongs to) carries, verbatim:
  ```
  // The physical source where this repo came from, eg. full
  // path to the zip filename or git repository directory. This
  // will not be exposed in the UI, but can be used to detect
  // orphaned index shards.
  Source string
  ```
  `github.com/sourcegraph/zoekt`, `api.go:576-598` (`Repository` struct,
  `Source` field at line 597, comment at 593-596).
- Every individual search hit is tagged with which repo it came from via a
  small integer, not the repo name string: `RepositoryID uint32` on
  `FileMatch` — `api.go:83-87` ("RepositoryID is a Sourcegraph extension.
  This is the ID of Repository in Sourcegraph."). The same field is threaded
  through the gRPC wire format: `grpc/protos/zoekt/webserver/v1/webserver.pb.go:1640-1641`
  (`RepositoryId` on `FileMatch`), and merge/sort logic groups results by it:
  `search/shards.go:875-888` (`curRepoID := result.Files[0].RepositoryID`).
- Sourcegraph's own multi-tenant extension adds a *second*, coarser origin
  dimension on top of `RepositoryID`: `TenantID int` — "Sourcegraph's tenant
  ID" — `api.go:578-579`, the very first field on `Repository`, ahead of the
  repo id itself.

**Relevance to rawclaw:** this is close to a direct blueprint. Zoekt already
solved "tag every document with where it physically came from, specifically
so a prune/orphan-detection pass can act only on what it can prove came from
that source" — `Source string` on the shard, `RepositoryID` on every result
row. Sourcegraph then found `RepositoryID` alone insufficient once multiple
tenants shared infrastructure and added `TenantID` as a still-coarser id
*above* it — exactly the shape rawclaw needs: `session_id` (fine),
`source_tool` (medium — which adapter/registry `ID`, already exists as
`source.Registration.ID` per `internal/source/source.go:43`), `origin_machine`
(coarse — the new dimension), stacked as separate typed fields, not
concatenated into one string.

### Document-level provenance for symbols

- `index.Document` (the per-file unit Zoekt indexes) carries `Branches
  []string` and `SubRepositoryPath string` directly on the document, not
  just at the shard level — provenance travels with the row that will be
  returned, not only with the container it lives in:
  `github.com/sourcegraph/zoekt`, `index/document.go:5-19`.

**Relevance to rawclaw:** supports denormalizing `origin_machine` /
`source_tool` onto the `sessions` row itself (as Zoekt does with
`RepositoryID` on `FileMatch`) rather than only recording it in a separate
ingest-time table — the prune step needs to filter *sessions* directly by
origin, not join out to something else.

---

## 3. UUID collision reality

- RFC 9562 (obsoletes RFC 4122) is the current UUID spec. Per a fetch of
  `https://www.rfc-editor.org/rfc/rfc9562.html`: Section 6.4 "Distributed
  UUID Generation" discusses embedding "a pseudorandom Node ID value...
  within the UUID layout" for schemes that want one (recommending UUIDv8 for
  that, and that "the node id SHOULD NOT be an IEEE 802 MAC address").
  Section 6.7 "Collision Resistance" and Section 6.8 "Global and Local
  Uniqueness" are the relevant discussion for whether a bare UUIDv4 is safe
  to treat as globally unique: **flagging this as fetched-but-paraphrased,
  not a verbatim quote** — the fetch tool summarized rather than quoted
  full sections (RFC prose exceeded what could be reproduced verbatim in one
  pass). The gist reported: UUIDs "MAY be used to provide local uniqueness
  guarantees" and true global uniqueness "can't be guaranteed without a
  shared knowledge scheme," though such a scheme is "not required" for most
  practical purposes. Treat the exact wording as **NOT independently
  verbatim-verified this session** — re-fetch
  `https://www.rfc-editor.org/rfc/rfc9562.html#section-6.8` directly before
  quoting it in anything load-bearing; the section numbers and general
  position (RFC declines to promise global uniqueness without coordination,
  but treats collision probability as acceptably low for v4's ~122 bits of
  entropy) are consistent with the well-known public understanding of this
  RFC.
- Concretely: both rawclaw source tools already mint their own random ids
  independently — Claude Code's session filename UUID
  (`internal/source/claude/claude.go:62`, `ID: sid`) and Codex's own
  `session_meta.id` (`internal/source/codex/meta.go:64`). Neither is
  rawclaw's to control. The real risk in the prompt's scenario is not
  "two independently-generated UUIDv4s happen to collide" (per RFC 9562's
  entropy budget this is negligible) — it is a **different session
  minting a *non-random*, low-entropy identifier** (the sibling runtime's session
  *title*, which is exactly the verified collision it already hit), or the
  *same* session being visible from two machines (e.g. a synced project
  directory) and needing a schema that doesn't choke on seeing the same
  tool-minted id twice from two origins.

### Systems that add a namespace/origin prefix anyway, despite a "big enough" random id space

- **MongoDB `OID`** — even the modern, all-random-tail `OID` format is not
  trusted to survive a `fork()` unassisted: "Warning: You MUST call
  `OID::justForked()` after a fork(). This ensures that each process will
  generate unique OIDs." — `github.com/mongodb/mongo`, `src/mongo/bson/oid.h:53-58`,
  with the current layout `kOIDSize = 12, kTimestampSize = 4,
  kInstanceUniqueSize = 5, kIncrementSize = 3` at line 63. Even a
  mostly-random 12-byte id still needs an explicit re-randomization hook at
  a known origin-boundary event (process fork) to stay collision-safe —
  "big enough" alone wasn't treated as sufficient by the implementers.
- **Discord Snowflake** — every id explicitly embeds *which shard/process*
  minted it, not just a timestamp+random tail: per
  `https://docs.discord.com/developers/reference` (fetched this session,
  redirected from `discord.com/developers/docs/reference`), the 64-bit
  layout is 42 bits timestamp, **5 bits internal worker ID**, **5 bits
  internal process ID**, 12 bits increment. Origin (worker/process) is a
  first-class, always-present part of the id, not an add-on.
- **Zoekt/Sourcegraph** (§2 above) — `TenantID` stacked above `RepositoryID`
  is the same move: once multiple untrusted-relative-to-each-other origins
  share one store, a coarser origin field gets added even though
  `RepositoryID` was already a perfectly good, collision-free integer within
  a single tenant.

**Why they do it anyway:** in every case above, the id was either (a) never
actually guaranteed unique in the first place without a boundary-aware
generation step (Mongo), or (b) needed to support *routing/filtering by
origin* as a first-class query need, not just uniqueness (Discord's shard
introspection, Sourcegraph's per-tenant isolation, Zoekt's orphan detection).
rawclaw is squarely in bucket (b): the ask isn't "make ids unique enough" —
Claude/Codex ids already are, practically — it's "let the prune step ask
'is this row mine?' without a join or a guess."

---

## RECOMMENDATION FOR RAWCLAW

**Provenance fields to add to `sessions` (and denormalize onto `file_index`
so both tables can be filtered by origin without a join):**

| field | type | modeled on | tradeoff |
|---|---|---|---|
| `origin_machine` | TEXT | Syncthing `DeviceID` (self-minted, stable identity of the producing replica) — `lib/protocol/deviceid.go:44-48` | Use a stable machine id (macOS hardware UUID / `/etc/machine-id`), not `hostname` — Syncthing derives `DeviceID` from a stable cert precisely because a mutable label (hostname) is the wrong thing to key identity on. A hostname rename must not silently orphan every prior row. |
| `source_tool` | TEXT | rawclaw's own existing `source.Registration.ID` (`internal/source/source.go:43`, already `"claude"`/`"codex"`) + Zoekt's `RepositoryID`-as-medium-grain-dimension (`api.go:83-87`) | This already exists as a registry concept but is **not currently persisted onto the `sessions` row** (`internal/index/index.go:31-34` has no such column) — single-source-of-truth fix, not a new concept. |
| `source_path` | TEXT | Zoekt `Repository.Source` — "physical source... can be used to detect orphaned index shards" (`api.go:593-597`) | Denormalize the already-tracked `file_index.path` onto the session row too, scoped by `origin_machine`, so a prune query is `WHERE origin_machine = <this machine> AND source_path NOT IN (<live tree glob>)` with zero risk of touching another machine's rows even if `session_id`s happen to be visible in the same table. |

**Identity/dedup scheme:**

- **Primary key: compound `(origin_machine, source_tool, session_id)`**, not
  bare `session_id`. Modeled on Syncthing's `(folder ID, device ID)` compound
  keys (`internal/db/olddb/keyer.go:24-49`), Automerge's `(actor, counter)`
  `OpId` (`rust/automerge/src/legacy/mod.rs:26-32`), and Sourcegraph's
  `(TenantID, RepositoryID)` stacked-origin pattern (`api.go:578-582`). This
  is the direct fix for the sibling runtime's failure mode: its bug was using a
  single low-entropy, un-scoped field (session *title*) as a `UNIQUE`
  constraint, conflating "human label" with "storage identity." A compound
  key with an explicit origin component structurally cannot collide across
  machines — two machines independently minting the same `session_id` (or
  the same tool re-using an id) still land on different primary keys because
  `origin_machine` differs.
- **`session_id` stays the tool-minted id verbatim — do not hash or
  re-derive it.** Git's content-addressing (`gitformat-loose.adoc:28-35`) is
  the useful counter-example here: hashing content collapses *different*
  origins that happen to produce identical bytes into *one* id, which is
  backwards for rawclaw — a session_id collision across machines should stay
  visible as distinct rows (different `origin_machine`), not silently merge.
- **Dedup is a separate, explicit reconciliation step layered on top of the
  unique storage key — never baked into the uniqueness constraint itself.**
  This is the Automerge lesson: `OpId` (actor+counter) is what makes every
  op *storage-unique*; recognizing that two ops are causally the "same
  change" is a *merge* operation the CRDT runs afterward, not something the
  id format tries to guarantee by construction. Concretely for rawclaw: when
  the same `(source_tool, session_id)` is seen from two `origin_machine`s
  (e.g. a synced project directory visible on two machines), that is a valid
  *display-time* dedup candidate — group by `(source_tool, session_id)` when
  presenting "distinct sessions," or run an explicit reconciliation pass
  that picks one row as canonical — but it must never be a DB-level `UNIQUE`
  constraint on `(source_tool, session_id)` alone, because that is exactly
  the sibling-runtime crash shape: the moment two machines legitimately both hold a
  live row for the same source-minted id, the insert fails instead of
  coexisting.
- **Don't machine-prefix the id string itself** (e.g. `"mba:claude:<uuid>"`
  as one opaque TEXT column). Keep `origin_machine` / `source_tool` /
  `session_id` as separate typed columns, per Zoekt/Sourcegraph's
  `RepositoryID` + `TenantID` as distinct fields rather than a mangled
  composite string (`api.go:578-582`) — it's what lets the prune query
  filter on `origin_machine` alone without string-parsing every row, and
  what lets a later dedup pass group by `(source_tool, session_id)` without
  stripping a prefix first.
- **UUID collision math is not the load-bearing risk here** — both source
  tools already mint sufficiently-random ids independently
  (`internal/source/claude/claude.go:62`, `internal/source/codex/meta.go:64`),
  and RFC 9562's entropy budget for v4 makes an accidental cross-machine
  collision negligible (fetched-but-paraphrased this session — re-verify
  exact wording before quoting §6.7/6.8 verbatim elsewhere). The real,
  already-demonstrated risk is a schema that uses a single unscoped field as
  both identity and uniqueness constraint (the sibling runtime's title `UNIQUE`) — which
  the compound key above fixes structurally, independent of how "random"
  any individual id is.

**Open / not resolved by this pass:**
- The exact reconciliation UX for the display-dedup case (pick-canonical vs.
  merge-messages vs. show-both-with-a-badge) is a product decision, not a
  prior-art question — OPEN.
- EventStoreDB's `<category>-<id>` stream-naming doc could not be re-fetched
  this session (404/redirect) to confirm current wording — the `eventhorizon`
  Go library citation stands in as the verified instance of the same pattern,
  but the EventStoreDB-specific doc claim is NOT VERIFIED against a live URL
  this session.
