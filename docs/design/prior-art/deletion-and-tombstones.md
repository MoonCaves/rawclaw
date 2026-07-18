# Prior art: missing files vs. real deletes, and tombstone patterns

Context: rawclaw's indexer currently prunes a session from its SQLite FTS5 DB
whenever the backing Claude Code / Codex transcript file is gone from disk.
That conflates two very different situations: (a) Claude Code purges local
transcripts after `cleanupPeriodDays` (~30 days) — file gone, but nothing was
"deleted" by the user; (b) a central store holding sessions synced from other
machines will never see those files locally at all. We still need to honor a
*real* user delete. This doc collects prior art from mature indexers,
file-sync systems, and replicated-delete patterns to inform the design.

All claims below are cited to a fetched repo/file/line or a fetched doc
URL. Anything I could not verify is marked NOT VERIFIED.

---

## 1. How mature search indexers handle a vanished indexed file

### notmuch — explicit, scoped reconciliation; never prunes on "can't find it"

notmuch never prunes the database as a side effect of failing to open a
message. Removal only happens inside `notmuch new`, and only for a directory
that was actually scanned:

- `notmuch-new.c` defines `remove_filename()` — "Remove one message filename
  from the database" — called only from `_remove_directory()`, which is
  itself scoped to one directory that the scanner just walked:
  `github.com/notmuch/notmuch`, `notmuch-new.c:943-973` (`remove_filename`)
  and `notmuch-new.c:975-1019` (`_remove_directory`, "Recursively remove all
  filenames from the database referring to 'path' (or to any of its
  children)").
  https://github.com/notmuch/notmuch/blob/master/notmuch-new.c#L943-L1019
- The undocumented-but-real mechanism: notmuch diffs the DB's own recorded
  child files for a directory (`notmuch_directory_get_child_files`) against
  what's still there on disk for *that directory*, and only removes entries
  for files missing from a directory it positively scanned — it does not
  reach into directories it never visited.
- `notmuch new` reports the result explicitly: "Added N new messages... /
  Removed N messages... / Detected N renames" — `notmuch-new.c:1021-1055`
  (`print_results`). https://github.com/notmuch/notmuch/blob/master/notmuch-new.c#L1021-L1055
- Man page confirms scanning is mtime-optimized per-directory (`--full-scan`
  disables that optimization), and a directory that can't be read produces a
  *temporary-failure* exit status inviting retry, not a silent delete:
  https://notmuchmail.org/doc/latest/man1/notmuch-new.html
- Real-world failure mode reported on the mailing list: when a sync tool
  (mbsync) manipulates directory mtimes, `notmuch new` can fail to notice
  files have vanished at all — i.e., its missing-file detection is a
  best-effort *scan* artifact, not a robust ground truth:
  https://notmuchmail.org/pipermail/notmuch/2014/019773.html

**Relevance to rawclaw:** notmuch's model is "only remove what you can prove
is gone from a place you actually looked," scoped per-directory, during an
explicit reconciliation pass — never opportunistically during a read/query.
That directly supports treating "session file not present on *this* machine"
as inconclusive rather than a delete signal, especially once other machines'
sessions are in play.

### recoll — index retains missing files unless an explicit purge pass runs, and purge is deliberately *not* run on partial scans

- `recollindex(1)`: "the --nopurge option will disable the normal erasure of
  deleted documents from the index, useful in special cases when it is known
  that part of the document set is temporarily not accessible" — i.e. Recoll
  itself anticipates "file absent because temporarily inaccessible" as a
  first-class case distinct from "file deleted."
  https://www.recoll.org/manpages/recollindex.1.html
- The same man page: "the -P option will force the purge pass, which is
  useful only if the idxnoautopurge parameter is set" and, for a
  subtree-scoped run (`-i`/`-e`), "the indexer normally does not perform the
  deleted-files purge pass because it cannot be sure to have seen all the
  existing files" — purge is gated on having done a *complete* scan.
  https://www.recoll.org/manpages/recollindex.1.html
- Config-level equivalent: `idxnoautopurge` = "Do not purge data for deleted
  or inaccessible files. This can be overridden by recollindex."
  https://www.recoll.org//usermanual/webhelp/docs/RCL.INSTALL.CONFIG.RECOLLCONF.MISC.html

**Relevance to rawclaw:** Recoll's rule — purge only after a scan you're
confident was *exhaustive* over the whole set, and make purge an explicit,
separately-invoked pass, off by default for partial runs — maps directly onto
"don't purge a session just because this run's directory listing doesn't
include it," which is exactly the central-store scenario.

### Zoekt / Sourcegraph indexserver — soft-delete to trash with a grace period, only hard-deletes after the grace period, and a real repo-level tombstone bit

This is the closest prior art to rawclaw's actual shape (a background indexer
reconciling against a possibly-incomplete listing of "what should exist").

- `cleanup()` in `cmd/zoekt-sourcegraph-indexserver/cleanup.go` is invoked
  with the current authoritative list of repo IDs (`repos []uint32`) that
  *should* be indexed, e.g. from Sourcegraph's own catalog — this is the
  external source of truth, analogous to "the transcript exists on disk right
  now."
- Repos found on disk but **not** in that list are never deleted directly —
  they are moved into a `.trash` directory first:
  `github.com/sourcegraph/zoekt`, `cmd/zoekt-sourcegraph-indexserver/cleanup.go:134-146`
  (`// index: Move non-existent repos into trash` → `moveAll(trashDir, shards)`).
  https://sourcegraph.com/github.com/sourcegraph/zoekt/-/blob/cmd/zoekt-sourcegraph-indexserver/cleanup.go?L134-146
- Trashed shards are only permanently removed once they've sat in trash for
  24 hours (`minAge := now.Add(-24 * time.Hour)`), and if the repo
  *reappears* in the authoritative list before then, it's restored from trash
  instead of re-indexed from scratch:
  `cleanup.go:26-29` (doc comment) and `cleanup.go:41-61` (trash-aging loop)
  and `cleanup.go:112-123` (`// index: Restore deleted or tombstoned repos`).
  https://sourcegraph.com/github.com/sourcegraph/zoekt/-/blob/cmd/zoekt-sourcegraph-indexserver/cleanup.go?L26-61
- There's a second, explicit "Tombstone" bit at the repo/shard level
  (`Repository.Tombstone`, `FileTombstones`) used for compound shards where a
  single physical shard holds many repos and one repo can't just be
  file-deleted without affecting siblings:
  `github.com/sourcegraph/zoekt`, `api.go:636-648` (`Tombstone bool` /
  `FileTombstones map[string]struct{}`), and the setter/unsetter in
  `index/tombstones.go:14-22` (`SetTombstone` / `UnsetTombstone`, "idempotently
  sets/removes a tombstone").
  https://sourcegraph.com/github.com/sourcegraph/zoekt/-/blob/api.go?L636-648
  https://sourcegraph.com/github.com/sourcegraph/zoekt/-/blob/index/tombstones.go?L14-22
- A separate, unconditional hard-delete path (`purgeTenantShards` in
  `purge.go`) exists for actual tenant/account removal — i.e. Zoekt
  deliberately has *two* delete paths: a soft, reversible, grace-period one
  for "not currently in the listing" and a hard, immediate one for "we were
  explicitly told this is gone":
  `github.com/sourcegraph/zoekt`, `cmd/zoekt-sourcegraph-indexserver/purge.go:15-73`.
  https://sourcegraph.com/github.com/sourcegraph/zoekt/-/blob/cmd/zoekt-sourcegraph-indexserver/purge.go?L15-73

**Relevance to rawclaw:** this is the single closest analog. Zoekt already
solved "an indexer's periodic listing doesn't include everything it should"
by (1) never trusting one negative listing as proof of deletion, (2) staging
absence as a reversible soft-delete with a grace window, and (3) reserving
immediate hard-delete for an explicit, out-of-band deletion signal rather
than absence-from-scan.

### mu (mu4e indexer) and Sourcegraph's general repo search — NOT VERIFIED

I could not get direct code-level confirmation of mu's (djcb/mu) stale/orphan
handling logic via code search (queries against `lib/index` and NEWS.org
returned no on-point hits beyond changelog mentions of "don't warn about
missing files with `--quiet`", `NEWS.org:1694`, which only confirms mu *has*
a missing-file code path, not its removal semantics). Treat mu's specific
behavior as NOT VERIFIED rather than assumed-similar to notmuch.

---

## 2. How file-sync systems distinguish "intentional delete" from "file merely absent locally"

### Syncthing — deletion is a normal file-info record with a `Deleted` bit, versioned like any edit; not a separate tombstone table

- The wire/DB model: `FileInfo.Deleted bool`, exposed via `IsDeleted()`:
  `github.com/syncthing/syncthing`, `lib/protocol/bep_fileinfo.go:346-349`.
  https://sourcegraph.com/github.com/syncthing/syncthing/-/blob/lib/protocol/bep_fileinfo.go?L346-349
- A deletion is synced through the exact same index-exchange path as a
  content edit — `IsDeleted()` is checked throughout the puller/scanner
  (`lib/model/folder_sendrecv.go:329`, `:341`, `:369-371`;
  `lib/model/folder_sendonly.go:67-69`; `lib/scanner/blockqueue.go:106-108`
  guards against hashing a deleted file) — i.e. "deleted" is a *state of the
  versioned record itself*, not the record's absence.
  https://sourcegraph.com/github.com/syncthing/syncthing/-/blob/lib/model/folder_sendrecv.go?L329-343
- This means a device that never had the file, and a device where the file
  was scanned-then-deleted, are distinguishable in Syncthing precisely
  *because* the delete is a positive, versioned record ("this path is now
  deleted as of version V") rather than an inferred absence.
- Documented conflict-resolution behavior when a delete races an edit: prior
  to 2.0 the edit always won and resurrected the file; 2.0+ allows delete to
  win, in which case the deleted-elsewhere file is preserved as a
  `.sync-conflict` copy rather than silently vanishing — confirmed via
  Syncthing's own docs/forum: https://docs.syncthing.net/users/syncing.html
  and https://forum.syncthing.net/t/2-0-conflict-changes/24786
- Known failure mode (useful as a cautionary prior-art data point, not a
  pattern to copy): because Syncthing folds deletion into per-device version
  vectors rather than a garbage-collected tombstone, removing a device from a
  cluster leaves that device's vector-clock entries stuck forever ("ghost
  counters"), which can cause deleted files to resurrect years later:
  https://github.com/syncthing/syncthing/issues/10590

**Relevance to rawclaw:** confirms the core mechanism rawclaw needs — a
*positive, versioned* delete record, not "absence from a listing" — but also
flags a real cautionary case (unbounded per-actor state that can never be
retired) worth avoiding when designing the tombstone's own lifecycle.

### Dropbox delta/list_folder API — deletion is an explicit tagged entry in the change feed, not a missing entry

- Delta API (v1): each entry is `[path, metadata]`; a `null` metadata value
  is the explicit deletion signal ("if the metadata is null, it means the
  path was deleted") — confirmed via Dropbox's own developer blog post
  introducing the call: https://dropbox.tech/developers/the-new-delta-api-call-beta
- Current API (v2) `list_folder` / `list_folder/continue`: deleted entries
  are returned with an explicit `DeletedMetadata` tag (`.tag = "deleted"`)
  rather than by omission — https://developers.dropbox.com/dbx-file-access-guide
- In both versions, a client is *only* supposed to delete local state on
  receipt of the explicit deletion entry for a path; a path that simply
  never appears in a delta/list response is not treated as deleted.

**Relevance to rawclaw:** same shape as Syncthing — deletion must be a
positive, explicit signal traveling through the same change/sync channel,
never inferred from "didn't show up this time."

### rsync — three different flags exist because "missing from source" and "explicit delete" are genuinely different operations

- `--delete`: "removes extraneous files from the destination so it matches
  the source's file list" — deletes destination files not present in a
  *complete* source file list (i.e., only meaningful when the source listing
  is trusted to be exhaustive).
- `--ignore-missing-args`: suppresses the error when an explicitly-named
  source argument doesn't exist at transfer start; does *not* delete
  anything and does *not* affect files that vanish mid-transfer after being
  found.
- `--delete-missing-args`: the bridging option — explicitly turns a named,
  missing source argument into a deletion request on the destination.
- Source: rsync mailing list thread on the exact ambiguity between these
  three, and the man-page-derived summary of each flag's contract:
  https://lists.samba.org/archive/rsync/2017-April/031167.html

**Relevance to rawclaw:** rsync's design encodes the same distinction rawclaw
needs — "not currently visible" (ignorable) vs. "was named and confirmed
gone" (a real delete) vs. "sync destination to match a complete listing"
(only safe when the listing is known-complete) — as three different, opt-in
operations rather than one implicit behavior.

### IMAP — two-phase delete: mark, then a separate explicit expunge

- RFC 3501 defines the `\Deleted` flag as marking a message "for removal by
  later EXPUNGE" — setting the flag (`STORE +FLAGS (\Deleted)`) does not
  remove anything by itself; the message remains fully present and fetchable
  until a client issues `EXPUNGE`.
  https://www.rfc-editor.org/rfc/rfc3501
- RFC 4315 (UIDPLUS) adds `UID EXPUNGE`, which permanently removes only
  messages that are *both* flagged `\Deleted` *and* named by UID in the
  command — explicitly to prevent one client's expunge from removing
  messages a different client marked deleted concurrently (avoids
  cross-client false deletes).
  https://datatracker.ietf.org/doc/html/rfc4315
- Cyrus IMAP's own FAQ distinguishes "Deleted" (flag set, still present),
  "Expunged" (removed from the mailbox, may still be recoverable
  server-side), and "Purged" (actually gone) as three separate stages:
  https://www.cyrusimap.org/imap/reference/faqs/o-deleted-expired-expunged-purged.html

**Relevance to rawclaw:** the mark/confirm split is the cleanest prior art
for "distinguish intent from absence" — a client (or rawclaw's own delete
path) makes an *explicit, separate* deletion call; nothing is inferred from
a message/file simply not being observed during a routine sync.

---

## 3. Tombstone pattern for propagating deletes without resurrection, and tombstone GC

### The core pattern (as implemented by Zoekt, Cassandra, and implied by Dropbox/Syncthing)

A tombstone is a **positive record that a thing was deleted**, written with
a timestamp/version, that:
1. Propagates through the same replication/sync channel as ordinary writes
   (so every replica eventually sees "deleted," not just "absent"), and
2. Is retained for a bounded grace period so that late-arriving replicas
   (or a delayed local scan) don't resurrect the item by re-adding it after
   only seeing the old, non-deleted state, and
3. Is garbage-collected after that grace period, once the operator is
   confident all replicas have converged.

### Concrete GC mechanics — Cassandra

- Tombstones are written into the normal write path/SSTables just like an
  insert. https://cassandra.apache.org/doc/latest/cassandra/managing/operating/compaction/tombstones.html
- `gc_grace_seconds` (default **864000s / 10 days**) is the retention window;
  a tombstone is only eligible for removal once it is older than
  `gc_grace_seconds`, *and* actual removal only happens opportunistically
  during a compaction that covers both the tombstone's SSTable and every
  SSTable containing older data for the same partition (an SSTable with only
  newer data can be safely excluded from that requirement).
  https://cassandra.apache.org/doc/latest/cassandra/managing/operating/compaction/tombstones.html
- The explicit rationale documented by Cassandra: the grace period exists
  "to give unresponsive nodes time to recover and process tombstones
  normally, using hinted handoff or repair" — i.e. the GC delay is sized to
  the expected worst-case replica-convergence time, not an arbitrary number.

### Concrete GC mechanics — Zoekt (as detailed in §1)

Zoekt's `.trash` + 24h age-out is the same shape at a much smaller scale:
soft-delete now, hard-delete only after a bounded window, restore-on-reappear
before that window elapses. See citations under §1 above
(`cleanup.go:26-61`, `:112-146`).

### Why this avoids resurrection

In all of the above, "absence" is never itself write-worthy — only an
explicit delete/tombstone event is. That's what prevents the classic bug:
replica A deletes item X; replica B, which hasn't heard about the delete yet,
re-syncs X back to A because B still has it. A tombstone (rather than a mere
gap) is what lets A tell B "no, X was deleted *after* the version you have,"
which is exactly the ordering guarantee a plain "row not present" can't
provide.

---

## RECOMMENDATION FOR RAWCLAW

**Adopt a Zoekt-style soft-delete-with-grace-period, modeled primarily on
Zoekt's `cleanup.go` trash mechanism, with the mark/confirm split from IMAP
and notmuch's "only trust what you positively scanned" discipline layered on
top:**

1. **Never infer a delete from absence-during-scan.** A session whose
   transcript file is missing during a routine reconciliation pass is left
   alone in the DB (notmuch's `_remove_directory` / recoll's
   `idxnoautopurge` model) — it's marked `missing_since = now()` (first
   observation), not deleted. This alone fixes both problems from the context above:
   the 30-day local purge and cross-machine sessions never look like deletes.
2. **Require an explicit delete signal, not a listing gap, to write a
   tombstone** — analogous to IMAP's `\Deleted` + `EXPUNGE` split and
   Dropbox's explicit `DeletedMetadata` entries: a real user delete (e.g. a
   rawclaw CLI `delete`/`forget` command, or an explicit "this session was
   removed" event if/when synced from another peer) is the only thing that
   ever writes a tombstone row. A file simply not being found is not that
   signal.
3. **Tombstones propagate and are retained for a bounded grace window**,
   Cassandra/Zoekt-style — pick a window sized to rawclaw's actual
   convergence risk (e.g. long enough to cover the slowest peer's sync
   interval, the way Cassandra sizes `gc_grace_seconds` to repair time and
   Zoekt sizes its trash window to one cleanup cycle's worth of listing
   flakiness). Tombstones (and the trash/soft-delete records) are only
   purged from the DB after that window, at which point they're truly gone
   and won't resurrect.
4. **If a "missing" session's file reappears** (re-synced from another
   machine, or the local transcript comes back) before any tombstone exists
   for it, treat it exactly like Zoekt's "restore from trash" path — no
   special-casing needed, since nothing was ever removed from the index in
   the first place.

**Tradeoff:** this permanently grows the DB with `missing_since`-marked rows
that are never actually gone (bounded only by the eventual, explicit
tombstone-and-GC cycle) — the same tradeoff every system surveyed here
accepts (Cassandra's SSTables carry tombstones for 10 days by default;
Zoekt's trash holds shards for 24h; Syncthing keeps deleted-file records
indefinitely as ordinary versioned entries). The alternative — pruning
promptly on absence, which is rawclaw's current behavior — is exactly the
behavior every one of these systems deliberately avoids, because it's the
one that causes silent, unrecoverable data loss on nothing more than a
transient or partial view of the filesystem.
