# Adversarial review: `12b9c98..HEAD`

Reviewed commits:

- `1b9f1b7` — surface freshness check errors
- `ebc086a` — distinguish same-named source databases
- `5864be4` — merge of `1b9f1b7`
- `59a2064` — merge of `ebc086a`

The merge commits add no independent tree changes. I inspected the complete
range with `git log`, `git show` for each commit, the resulting source and
tests, the freshness callers, and the SQLite connection policy. I also ran the
targeted and full race test commands; their final results are recorded below.

## Findings

### HIGH — the path-identity migration leaves old basename provenance rows immortal

`internal/index/consolidated.go:611-635` now records each new contribution under
`sourceIdentity(src)`, an absolute path. Before `ebc086a`, the same rows were
recorded under `filepath.Base(src)`. `migrateSessionSources` only creates the
table and backfills rows with `source_db = ''` (`internal/index/consolidated.go:861-903`);
it never rewrites existing non-empty basename rows.

Concrete failure:

1. An existing consolidated store contains `session_sources(session_id='s',
   source_db='sessions.db', ...)`, produced by the pre-`ebc086a` code.
2. The upgraded binary folds `/cache/project/sessions.db`. It creates a second
   row for `/cache/project/sessions.db`; the old `sessions.db` row remains.
3. The source is purged and folded again. The new absolute-path row is removed
   at lines 634-635, but the old basename row is not. `mergeSessionsSQL` still
   sees the basename contribution, so the supposedly deleted session remains
   live in the consolidated store.

The same issue duplicates contribution metadata during the first post-upgrade
fold and can preserve stale messages through the merge. The added same-basename
test (`internal/index/consolidated_test.go:1129-1181`) starts with a new store,
so it cannot exercise this upgrade state. The fix needs an explicit, unambiguous
migration of old provenance rows, or a compatibility rule that removes/aliases
them safely before the first path-identity fold.

The existing test `TestConsolidateFrom_PrunesLegacySourceAfterFullPass`
(`internal/index/consolidated_test.go:1525-1530`) also fails on this HEAD:
`real row for session with real source = 0, want 1`. Its expectation still uses
the old basename identity, which is direct evidence that the transition was not
updated end-to-end.

### HIGH — legacy basename watermark compatibility can permanently hide an unprocessed database

`internal/index/consolidated.go:100-105` accepts a legacy `sync:<basename>` key
for every source whose basename matches. This preserves compatibility for one
old source, but it cannot distinguish same-named files.

Concrete failure:

1. An old store contains `meta(key='sync:sessions.db', ...)` because
   `/one/sessions.db` was folded by the old implementation.
2. `/two/sessions.db` exists but was never folded.
3. The new `UnconsolidatedDBs` call checks `/two/sessions.db`: it has no
   `sync:<absolute-path>` key, then accepts the basename key and omits it from
   the missing list.

The caller therefore reports the corpus as consolidated while `/two` is absent
from the one-store answer. The new test only proves two fresh absolute paths
remain independent; it does not pin an old basename watermark plus an
unprocessed same-named source. A compatibility migration must distinguish the
old source or conservatively report the ambiguous source(s) as needing a fold.

### MEDIUM — freshness errors are still silently discarded by browse

The helper now returns errors for unreadable watermarks and filesystem checks
(`internal/index/consolidated.go:989-1008` and `1051-1093`), but browse callers
only act when `fErr == nil`:

- `internal/cli/cli.go:1313-1329` continues using the consolidated result and
  emits no stale note when `CheckIndexFreshness` fails.
- `internal/cli/cli.go:1383-1401` and `1410-1417`/`1427-1434` likewise leave
  freshness unset and still accept consolidated rows.

Concrete failure: corrupt `meta.value` for `last_ingest_time` (the new helper
test creates exactly this shape at `internal/index/freshness_test.go:103-115`),
then run default browse. The helper returns an error, but browse treats that as
“no freshness result”, prints no warning, and can present stale consolidated
data as a normal answer. The helper-level tests do not cover the CLI contract.
The search path at `internal/cli/cli.go:1651-1656` does react to an error and
shows the inconsistency is in the browse paths.

### MEDIUM — two writers can interleave one source fold despite the atomicity claim

`consolidateOne` says it is “atomic per source” (`internal/index/consolidated.go:509-512`),
but there is no `BEGIN`/`COMMIT` around the sequence at lines 615-703. With
`ConnectRW` capped at one connection per process and SQLite's busy timeout,
different processes serialize individual statements, not the whole fold.

Concrete concurrent failure:

1. Process A attaches an older snapshot of source X and records the sessions
   missing from that snapshot in `temp.consolidation_affected_sessions`.
2. Process B attaches a newer snapshot of X, records a newly present session
   into `main.session_sources`, and commits its statement.
3. Process A executes its source-specific delete at lines 634-635 using its
   older attached snapshot and can delete B's newly recorded row, then merges
   and stamps its own watermark.

The result can lose the newer contribution until another source change causes a
fold. A process death between the separate statements also exposes partial
state: session_sources, sessions, messages, file_index, and the watermark can be
observed at different stages. A retry usually replays the source because the
watermark is written last, so the crash path is not a demonstrated permanent
corruption by itself; the cross-process interleaving is the durable defect. No
test starts two independent consolidators against one store.

## Checks that held up

- I found no context/channel wait in the changed code. The changed paths use
  SQLite operations with configured 10-second write and 5-second read busy
  timeouts; the specific darwin-only unbounded channel shape was not present.
- The new freshness helper tests do prove that a NULL watermark and a
  non-directory project path return errors rather than being silently treated as
  fresh. The missing coverage is the callers' handling of those errors.
- The new same-basename test proves fresh path identities do not overwrite each
  other's contribution. It does not prove upgrade compatibility.

## Verification

- `git diff --check 12b9c98..HEAD`: passed.
- `go test ./internal/index ./internal/cli`: failed in
  `TestConsolidateFrom_PrunesLegacySourceAfterFullPass` (expected
  `source_db='real.db'`, found no such row after the new absolute-path write).
- `CGO_ENABLED=0 go test -race -count=1 ./...`: did not complete within the
  review window. It produced no package result after several minutes while the
  machine had many concurrent `cli.test ingest` processes and an index test in
  an uninterruptible state; I terminated the waiting test session rather than
  claim a green gate. This is consistent with the concurrency/deadlock risk
  called out above, but is not by itself proof that this exact range caused the
  other processes.

## Verdict

Request changes. The path-identity fix is unsafe for upgraded stores and the
legacy fallback can hide an entire same-named source. Freshness error surfacing
is incomplete at browse call sites, and the fold's claimed per-source
atomicity is not true under concurrent RawClaw processes.
