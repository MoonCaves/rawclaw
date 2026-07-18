# Prior art: multi-machine SQLite (one logical store fed by N machines)

> **CORRECTION (2026-07-17, verified against sources):**
> This doc repeatedly frames `dogsheep-beta` as a live `ATTACH`+`UNION` query-time
> federation exemplar. That is **wrong**: dogsheep-beta consolidates at *index
> time* into one central `search_index` table (verified at
> github.com/dogsheep/dogsheep-beta) — it is not live cross-DB federation. So there
> are **two** distinct topologies with **distinct** prior art:
> (1) query-time federation → prior art is **Datasette cross-database queries**
> (ATTACH-based), NOT dogsheep-beta; (2) index-time consolidation → prior art
> **is** dogsheep-beta. `sqlite3_rsync` as transport is verified and correct.
> The reconciled version lives in `../durable-retention.md` (Deferred topology).
> Read that section as authoritative where it conflicts with the dogsheep framing below.

Research for rawclaw's north-star: "multiple machines feeding into ONE central store — all agent
sessions queryable from anywhere," without breaking the non-negotiables in `ROADMAP.md`:

> "a single static binary, keyword search by default, zero runtime dependencies, no LLM, no API key
> required" — and the explicit non-goals: **no cgo in the default build**, **no daemon requirement**,
> keyword search must always work **offline, with no network and no key**.

Workload shape: append-mostly (transcripts don't change once written), single-writer-per-source (each
machine owns its own sessions — nobody else writes them), must tolerate machines being offline for
arbitrary periods.

All claims below are cited to a fetched source (repo README/docs/go.mod, or a named blog post). Where
a claim could not be directly verified it is marked **NOT VERIFIED**.

---

## Comparison table

| Option | Replication model | Needs a daemon/server? | cgo or pure-Go? | Offline tolerance | Writer model |
|---|---|---|---|---|---|
| **Litestream** | Streaming WAL/LTX shipping to object storage (S3-compatible); optional live "read replica" pull | Yes — runs as a background process alongside the app | Pure Go (`modernc.org/sqlite`) for the main binary; cgo (`CGO_ENABLED=1`) only for the optional loadable-VFS build target and Windows builds | Primary buffers locally and streams when reachable; replicas simply lag when disconnected | Single-writer (one primary); replicas are read-only |
| **rqlite** | Raft consensus over an HTTP API | Yes — clustered server process | cgo (`mattn/go-sqlite3`, replaced by `rqlite/go-sqlite3`, still cgo-based) | No — a partition without majority quorum stops accepting writes entirely | Single-writer (Raft leader; all writes proxied to it) |
| **dqlite** | Raft consensus, embedded C library | Embedded-in-process, but still requires a live clustered quorum | cgo (C library + libuv + raft; Go bindings via `go-dqlite` use cgo); Linux-only | No — same Raft quorum requirement as rqlite | Single-writer (leader) |
| **LiteFS** | FUSE filesystem intercepting SQLite writes, ships per-transaction LTX files; leader election via Consul lease or a "static" lease | Yes — FUSE mount must run continuously; requires the FUSE kernel module | Requires `CGO_ENABLED=1` in its own release build | Not designed for disconnected operation — built for ephemeral cloud clusters; explicitly warns against autostop/autostart because a stale machine can win the lease and roll back data | Single-writer (primary); replicas read-only |
| **Turso / libSQL embedded replicas** | Client syncs from a remote Turso Cloud primary (`syncUrl` + periodic/manual `sync()`) | Requires a remote hosted service (Turso Cloud) — not self-hostable as a single binary | Split: `libsql-client-go` is pure Go but network-only (no local file, so no offline reads); `go-libsql` (embedded replicas) requires cgo; newer `turso-go` avoids cgo via `purego` FFI to a prebuilt native library (not a pure-Go build) | Local reads can work offline from the last-synced snapshot; writes require reaching the cloud primary in the classic embedded-replica model | Effectively single-writer at the cloud primary (replicas are read-only in the embedded-replica model); a newer "Turso Sync" claims local-first writes — recent, **NOT VERIFIED** as mature |
| **marmot** | v1: NATS JetStream CDC + Raft. v2 (current): gossip-based, leaderless, eventual consistency, 2PC distributed transactions | Yes — runs as a clustered sidecar/server (`-daemon` flag, MySQL-wire-compatible server) | cgo (`mattn/go-sqlite3` in `go.mod`) | v2's gossip protocol tolerates partitions better than Raft (anti-entropy recovers split-brain), but it's built for online node clusters, not machines offline for long stretches | Multi-writer / leaderless in v2 ("any node accepts writes") |
| **cr-sqlite** | Loadable SQLite extension adding CRDTs (LWW, fractional-index, observe-remove sets) to merge independently-written DBs | No daemon — it's a library/extension invoked at write and merge time | cgo — it's a native (C/Rust) run-time-loadable extension; loading it requires an SQLite driver built with `CGO_ENABLED=1` and extension-loading enabled (e.g. `mattn/go-sqlite3`); a pure-Go driver like `modernc.org/sqlite` cannot load it | Excellent — purpose-built for offline-first divergent writes merged later | True multi-writer (CRDT merge); ~2.5x slower inserts into CRDT-enabled tables |

---

## Per-option notes

### Litestream
Standalone disaster-recovery / streaming-replication tool that watches the WAL, converts changes to
immutable LTX files, and ships them to cloud storage or another file; it talks to SQLite only through
the SQLite API so it can't corrupt the DB. [github.com/benbjohnson/litestream](https://github.com/benbjohnson/litestream), [litestream.io](https://litestream.io/)

Its own `AGENTS.md` states the current implementation directly: "It uses `modernc.org/sqlite` (pure Go,
no CGO required)." `go.mod` still lists `mattn/go-sqlite3` as a dependency, but the `Makefile`/CI show
`CGO_ENABLED=1` is only used for the optional loadable-VFS build target (`-buildmode=c-archive`) and
Windows cross-builds; the default Linux/macOS binary build path in `.goreleaser.yml` uses
`CGO_ENABLED=0`. [litestream go.mod](https://sourcegraph.com/github.com/benbjohnson/litestream/-/blob/go.mod), [litestream AGENTS.md via Sourcegraph search], [litestream Makefile/.goreleaser.yml via Sourcegraph search]

Multi-node topology: the `litestream-read-replica-example` repo demonstrates a 3-node deployment (one
fly.io region as primary, two as read-only replicas), using Litestream's "live read replication"
feature — each node mounts its own volume and keeps a local copy that continuously applies changes
streamed from the primary; replicas are strictly read-only. [github.com/benbjohnson/litestream-read-replica-example](https://github.com/benbjohnson/litestream-read-replica-example), guide linked from its README: [litestream.io/guides/read-replica/](https://litestream.io/guides/read-replica/)

### rqlite
"A lightweight, fault-tolerant database built on SQLite… uses Raft to achieve consensus across all
instances," positioned like Consul/etcd but relational. [github.com/rqlite/rqlite](https://github.com/rqlite/rqlite)

`go.mod` requires `github.com/hashicorp/raft` and `github.com/mattn/go-sqlite3` (replaced with the
`rqlite/go-sqlite3` fork, still cgo). [rqlite go.mod](https://github.com/rqlite/rqlite/blob/master/go.mod)

Quorum behavior is explicit in rqlite's own docs/blog: a cluster needs a majority ("(N/2)+1") online to
process writes; on a network partition, "an rqlite cluster will remain available only on the side of
the partition that contains a majority of nodes; the other side, by default, will stop accepting
requests." A single surviving node in a 3-node cluster is "effectively offline until at least one more
node comes back online." [rqlite.io/docs/faq](https://rqlite.io/docs/faq/), [Consistency Over Availability — rqlite and CAP](https://philipotoole.com/consistency-over-availability-how-rqlite-handles-the-cap-theorem/)

### dqlite
"Embeddable, replicated and fault-tolerant SQL engine" — a C library, using libuv for its event loop and
Raft for replication; build requires libuv and SQLite headers (`autoreconf`/`./configure`/`make`); the
Go bindings (`go-dqlite`) use cgo. Runs on Linux only (requires native async I/O). [github.com/canonical/dqlite](https://github.com/canonical/dqlite), [dqlite README](https://github.com/canonical/dqlite/blob/master/README.md), [dqlite architecture docs](https://canonical.com/dqlite/docs/explanation/architecture)

### LiteFS
"FUSE-based file system for replicating SQLite databases across a cluster of machines" — intercepts
writes via FUSE, detects transaction boundaries, ships per-transaction LTX files. Extends Litestream's
idea with finer-grained transactional shipping. [github.com/superfly/litefs](https://github.com/superfly/litefs), [Introducing LiteFS](https://fly.io/blog/introducing-litefs/)

Its release workflow builds with `CGO_ENABLED=1`. [litefs .github/workflows/release.yml via Sourcegraph search]

Leader election supports two modes: `consul` (dynamic, requires a Consul cluster) or `static` (a fixed
primary, no failover) — confirmed directly in `cmd/litefs/etc/litefs.yml` and `mount_linux.go`. Even in
`static` mode the FUSE mount and the LiteFS process must be running continuously; Fly's docs explicitly
warn against combining LiteFS with autostop/autostart because the proxy can restart machines with no
awareness of lease ownership, risking rollback/data loss on a stale node. [litefs config via Sourcegraph search], [fly.io/docs/litefs/](https://fly.io/docs/litefs/)

### Turso / libSQL embedded replicas
Embedded Replicas keep a local read-replica file of a **remote Turso Cloud database**: reads are local,
writes go to the cloud primary and are reflected back via `sync()`/`syncInterval`. [docs.turso.tech/features/embedded-replicas/introduction](https://docs.turso.tech/features/embedded-replicas/introduction)

Go driver landscape: `libsql-client-go` is pure Go but purely network (wire protocol to the remote DB,
no local file — so no offline reads); `go-libsql` (needed for actual embedded replicas) "uses CGO to
make calls to LibSQL" and ships precompiled native libraries for a handful of platforms; a newer
`turso-go` avoids cgo specifically by using `purego` to call the same native (Rust, C-ABI) library
without the cgo toolchain — still a prebuilt native library, not a pure-Go build. [tursodatabase/libsql-client-go README](https://github.com/tursodatabase/libsql-client-go), [gorm-libsql README](https://github.com/ytsruh/gorm-libsql), [Turso docs on go-libsql / turso-go]

Turso itself is a **hosted service** — the "primary" embedded replicas sync from is Turso Cloud, not a
self-hosted peer. A newer feature, "Turso Sync," claims local-first writes with less bandwidth than the
older page-level embedded-replica sync, but is a recent addition — **NOT VERIFIED** as mature/stable
beyond the vendor's own blog post. [turso.tech/blog/sync-benchmark](https://turso.tech/blog/sync-benchmark)

### marmot
v1: "distributed SQLite replicator built on top of NATS" — required `nats-server` with JetStream,
Raft-based consensus under the hood. v2 is a ground-up rewrite: "leaderless, distributed SQLite
replication system built on a gossip-based protocol with distributed transactions and eventual
consistency," replacing the NATS/Raft architecture; NATS becomes optional (for CDC/event streaming
only). It now exposes a MySQL-wire-compatible server and supports running `-daemon` in the background.
[github.com/maxpert/marmot](https://github.com/maxpert/marmot), [maxpert.github.io/marmot](https://maxpert.github.io/marmot/), [HN thread with author's own architecture explanation](https://news.ycombinator.com/item?id=46460676)

`go.mod` confirms `mattn/go-sqlite3` (cgo) plus `hashicorp`-adjacent and gRPC/gossip deps for the
clustering layer. [marmot go.mod via Sourcegraph]

### cr-sqlite
"Convergent, Replicated SQLite. Multi-writer and CRDT support for SQLite" — a run-time-loadable
extension. Tables become "Conflict-free Replicated Relations" (CRRs); columns are typed as LWW,
fractional-index, or observe-remove-set CRDTs; two independently-written DBs can be merged offline with
no manual conflict resolution. Inserts into CRR tables are ~2.5x slower than plain SQLite inserts; reads
are the same speed. [github.com/vlcn-io/cr-sqlite](https://github.com/vlcn-io/cr-sqlite)

Loading any run-time extension (cr-sqlite included) requires a driver built with `CGO_ENABLED=1` and
`SQLITE_DBCONFIG_ENABLE_LOAD_EXTENSION` — `mattn/go-sqlite3`'s extension-loading code path
(`sqlite3_load_extension.go`) only exists in cgo builds; the pure-Go `modernc.org/sqlite` driver has no
equivalent loadable-extension mechanism. [sqlite.org/loadext.html](https://sqlite.org/loadext.html), [mattn/go-sqlite3 extension-loading source via search]

---

## The two simpler patterns the maintainer's question anticipated

### Pattern A — query-time federation: each machine keeps its own DB, `ATTACH` + `UNION` at query time
This is core SQLite (`ATTACH DATABASE`), not a project — but it's a well-established, widely-used
pattern, not a novel idea:

- Simon Willison, on adding cross-database query support to Datasette/`sqlite-utils`: "SQLite databases
  are individual files on disk… you can run queries, including joins, across tables from more than one
  database, using the `ATTACH DATABASE` command… combine them with `UNION`." [Cross-database queries in SQLite](https://simonwillison.net/2021/Feb/21/cross-database-queries/)
- `sqlite-utils` ships this as a first-class `--attach` CLI flag and a Python `db.attach()` API.
  [sqlite-utils CLI docs](https://sqlite-utils.datasette.io/en/3.15/cli.html)
- **`dogsheep-beta`** (part of Simon Willison's Dogsheep project) is the closest real analog to rawclaw's
  exact shape: it builds "a single, full text searchable database from the content of one or more
  database tables in **one or more [separate] databases**," using `ATTACH`, specifically to give a
  combined full-text-search index over independently-populated per-source SQLite DBs (Twitter, GitHub,
  HealthKit, Swarm, etc.) — i.e. N independently-written SQLite files, federated at query time into one
  FTS surface. [dogsheep.github.io](https://dogsheep.github.io/), [github.com/dogsheep](https://github.com/dogsheep)

No daemon, no cgo (`ATTACH`/`UNION` are core SQLite, work fine through `modernc.org/sqlite`), and it's
inherently single-writer-per-file since each attached DB is simply read.

### Pattern B — push/pull file sync, no daemon
- **`sqlite3_rsync`** is now an *official* SQLite tool (merged into the SQLite project) purpose-built
  for this: it hashes pages on the replica side and transfers only pages that differ over SSH, is
  WAL-aware ("gives an exact snapshot of the database state as it existed when the copy was initiated,
  even if the source database continues to apply changes"), and both sides can stay "live" (written-to /
  read-from) while it runs. No daemon required — it's a one-shot CLI invocation, cron-friendly.
  [sqlite.org/rsync.html](https://sqlite.org/rsync.html), [source: sqlite/sqlite tool/sqlite3-rsync.c](https://github.com/sqlite/sqlite/blob/sqlite3-rsync/tool/sqlite3-rsync.c)
- Third-party prior art in the same spirit: `moisseev/sqlite3-sync` does cron-friendly "live SQLite3
  database master-slave replication with sqlite3-rdiff using rsync over SSH." [github.com/moisseev/sqlite3-sync](https://github.com/moisseev/sqlite3-sync)

---

## RECOMMENDATION FOR RAWCLAW

Ranked for rawclaw's actual constraints (append-mostly, single-writer-per-source, must tolerate offline
machines, single static pure-Go binary, no daemon in the default path):

### 1st — Push-file-to-shared-location + query-time `ATTACH`/`UNION` federation (RECOMMENDED default)
Each machine keeps exactly what it has today: its own local SQLite FTS5 index of its own transcripts —
nothing about the single-machine index format needs to change. Each machine independently pushes its DB
(or a compacted snapshot of it) to a shared location — could be `sqlite3_rsync` over SSH to a central
box/NAS, or a periodic upload to S3/object storage — on its own schedule, no coordination needed since
each machine only ever writes its own file. A `rawclaw` search command run anywhere with access to that
shared location (or a local mirror of it) opens the N per-machine DBs with `ATTACH DATABASE` and runs
the FTS5 query as a `UNION ALL` across them, exactly the way `dogsheep-beta` federates N independently-
populated Dogsheep databases into one full-text search surface.

- **Modeled on:** `sqlite3_rsync` (official SQLite, [sqlite.org/rsync.html](https://sqlite.org/rsync.html)) for the transport + Datasette/`sqlite-utils`/`dogsheep-beta`'s `ATTACH`+`UNION` federation pattern ([simonwillison.net/2021/Feb/21/cross-database-queries](https://simonwillison.net/2021/Feb/21/cross-database-queries/), [github.com/dogsheep](https://github.com/dogsheep)) for the query side.
- **Tradeoff:** freshness is only as good as the last sync — a machine that's been offline for a week is
  just queried against its last pushed snapshot. This is not a gap to hide: rawclaw's search output
  already has a vocabulary for exactly this ("searched / empty / skipped / **stale**" per `ROADMAP.md`'s
  *Already shipped* section) — a source DB that hasn't synced recently is reported `stale`, not silently
  dropped. SQLite's default `ATTACH` limit (`SQLITE_MAX_ATTACHED`, default ~10, compile-time-raisable to
  125) caps how many machines can be federated in one query — plenty for an org's current handful of
  boxes, worth flagging if the machine count grows much past that.
- **Fit:** exact fit for every non-negotiable — no daemon (the push and the query are both one-shot CLI
  invocations, cron-friendly), no cgo (`ATTACH`+`UNION`+FTS5 are core SQLite, work through
  `modernc.org/sqlite`), works fully offline (a query against locally-mirrored snapshots needs no
  network at query time), single static binary unchanged.

### 2nd — Continuous streaming via Litestream, fanned in with the same `ATTACH`/`UNION` query layer
Each machine runs its own Litestream instance (as a sidecar process, not embedded in the `rawclaw`
binary) continuously streaming its local DB's LTX generations to a shared object-storage bucket, one
independent single-writer stream per machine — mirroring the topology in `litestream-read-replica-example`,
except instead of one shared DB with a primary+replicas, it's N independent streams. A query node either
live-restores each stream to a local read-only mirror (Litestream's own "read replica" mode) or just
downloads the latest snapshot per machine, then federates with the same `ATTACH`/`UNION` layer as option 1.

- **Modeled on:** `benbjohnson/litestream` + `litestream-read-replica-example` ([github.com/benbjohnson/litestream-read-replica-example](https://github.com/benbjohnson/litestream-read-replica-example)).
- **Tradeoff:** materially fresher (near-real-time instead of cron-interval), but reintroduces a
  continuously-running process per machine — technically an opt-in sidecar rather than something baked
  into rawclaw's own binary (same relationship rawclaw's own speculative `--watch` mode would have), so
  it doesn't violate "no daemon requirement" for the `rawclaw` binary itself, but it is a daemon in the
  full system. Worth deferring until sync-cadence (option 1) proves too coarse in practice.
- **Fit:** rawclaw's own binary stays pure-Go/no-cgo/no-daemon; the freshness cost is paid by an
  optional external sidecar, matching how Litestream is deployed everywhere it's used today (alongside
  an app, never embedded in it).

### 3rd — cr-sqlite CRDT merge (explicitly NOT recommended for the default path; flag for future multi-writer needs)
If rawclaw ever needs genuine multi-writer merge (e.g. two machines editing the *same* session's
metadata, not just appending disjoint sessions), `vlcn-io/cr-sqlite`'s CRDT tables are the closest real
prior art for "offline, independently-writable, merge-without-a-coordinator." Today's workload doesn't
need this — transcripts are append-mostly and already single-writer-per-source — so it earns no place in
the default path.

- **Modeled on:** `vlcn-io/cr-sqlite` ([github.com/vlcn-io/cr-sqlite](https://github.com/vlcn-io/cr-sqlite)).
- **Tradeoff:** it's the only option here with real multi-writer conflict resolution, but loading it
  requires a cgo-enabled SQLite driver — a hard violation of the no-cgo default. If it's ever justified,
  it should land the same way rawclaw's `ROADMAP.md` already frames the `sqlite-vec` ANN tier: "a build
  tag / opt-in tier… only users who explicitly want [it] opt into a build that carries it" — never the
  default binary.

### Explicitly ruled out for rawclaw
- **rqlite, dqlite** — both require an always-on clustered daemon and Raft quorum; a machine that's
  offline doesn't just serve stale local data, the *whole cluster* can stop accepting writes if quorum is
  lost. Directly contradicts "must tolerate machines being offline." dqlite additionally requires cgo and
  is Linux-only.
- **LiteFS** — requires an always-mounted FUSE daemon and lease coordination (Consul, or a fixed static
  primary with no failover); explicitly designed for ephemeral, always-connected cloud clusters, and its
  own docs warn that a node coming back stale after being offline can roll back data. Also builds with
  `CGO_ENABLED=1`.
- **Turso/libSQL embedded replicas** — the "primary" is a hosted cloud service, not a peer machine;
  introduces a mandatory network dependency for writes in the classic embedded-replica model, and the
  cgo-free client (`libsql-client-go`) has no offline/local-file mode at all. Wrong shape entirely for a
  "no required network, no API key" tool.
- **marmot** — v2's leaderless gossip protocol is the most partition-tolerant of the clustered options,
  but it's still a permanently-running clustered daemon (MySQL-wire server + gossip mesh) built for
  online node clusters, and it depends on cgo (`mattn/go-sqlite3`).

---

## Summary

Ranked topologies, most to least recommended:

1. **Push-file/delta-to-shared-location + query-time `ATTACH`+`UNION` federation** — modeled on the
   official `sqlite3_rsync` tool ([sqlite.org/rsync.html](https://sqlite.org/rsync.html)) for transport
   and Simon Willison's Datasette/`sqlite-utils`/`dogsheep-beta` `ATTACH DATABASE` federation pattern
   ([simonwillison.net/2021/Feb/21/cross-database-queries](https://simonwillison.net/2021/Feb/21/cross-database-queries/),
   [github.com/dogsheep](https://github.com/dogsheep)) for querying. No daemon, no cgo, fully offline-capable,
   each machine stays single-writer of its own file — this is the one that actually fits every
   non-negotiable in `ROADMAP.md` without modification.
2. **Continuous Litestream streaming per machine to shared object storage, fanned in with the same
   `ATTACH`/`UNION` layer** — modeled on `benbjohnson/litestream`'s own multi-node read-replica example
   ([github.com/benbjohnson/litestream-read-replica-example](https://github.com/benbjohnson/litestream-read-replica-example)).
   Fresher than cron-based rsync, at the cost of an optional always-running sidecar process (not baked
   into the `rawclaw` binary itself) — worth it only once sync-interval staleness actually bites.
3. **cr-sqlite CRDT merge** ([github.com/vlcn-io/cr-sqlite](https://github.com/vlcn-io/cr-sqlite)) — the
   right tool if rawclaw ever needs true multi-writer conflict resolution, but it requires a cgo-loaded
   native extension, so it can only ever be an opt-in build tier, never the default — same treatment
   `ROADMAP.md` already gives the speculative `sqlite-vec` ANN tier.

Ruled out outright: rqlite and dqlite (Raft quorum — a cluster can go write-unavailable when machines are
offline, plus dqlite needs cgo and is Linux-only), LiteFS (always-mounted FUSE daemon + lease
coordination, cgo build, warns against exactly the stale-offline-node scenario rawclaw needs to
tolerate), Turso/libSQL embedded replicas (the "primary" is a hosted cloud service — a mandatory network
dependency, wrong shape for a no-network-required tool), and marmot (always-on clustered daemon +
gossip mesh, cgo-based).

One-paragraph recommendation: build the multi-machine store as query-time federation over independently
synced per-machine SQLite files — each machine keeps writing its own local FTS5 index exactly as it does
today, pushes it (or deltas) to a shared location on its own schedule using something in the spirit of
SQLite's own official `sqlite3_rsync` tool, and a `rawclaw` query run anywhere `ATTACH`es the available
per-machine files and `UNION ALL`s the FTS5 query across them — the same pattern Simon Willison's
Dogsheep project (`dogsheep-beta`) already uses in production to federate full-text search across
independently-populated per-source SQLite databases. It needs no daemon, no cgo, no consensus protocol,
and degrades gracefully (a stale/offline machine's snapshot is just queried as-is and reported `stale`,
never silently dropped) — a genuinely closer fit to rawclaw's constraints than any of the seven
purpose-built multi-machine-SQLite projects surveyed, all of which trade away either the no-daemon, the
no-cgo, or the offline-tolerance requirement to get continuous multi-node consistency rawclaw doesn't
actually need for an append-mostly, single-writer-per-source workload.
