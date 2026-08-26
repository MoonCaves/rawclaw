# RawClaw worker problem and prior-art map

Snapshot taken 2026-08-26 Wave 8 from integration head `bd8346c` / `1b22703` / `f5dbe89` / `8dfa677` / `f15d1af` / `6330cc5` / `508f3544` / `3f4969e5`.
This is a report-only census. Product code and rival worktrees were not edited.

## Scope and evidence rules

The live census used `tmux list-sessions`, `tmux list-panes -a -F '#{session_name}\t#{pane_pid}\t#{pane_current_path}\t#{pane_current_command}'`, `ps`, `git worktree list --porcelain`, branch SHA/status reads, `.codex-run.log` tails, and repo-local mailbox receipts. Heartbeat/watchdog/spy loops are listed as orchestration evidence but are not counted as product problems. A pane marked live means its tmux/agy process was present at census time; a completed branch means its report was visible in the pane, not that its design was accepted.

Graphify was queried first. Relevant graph symbols are `renderHookScript`, `catalogIngestSource`, `ResolveSession`, `refreshTagSession`, `discoverTagSources`, `EnsureFreshContainer`, `PrepareFreshContainer`, `EnsureIndexedTree`, `SyncConsolidatedFrom`, `StampIngestWatermark`, `BenchmarkFTS5Search`, `spawnIngestChild`, and `TestPrimeScript_CatalogWriteFailure_NeverFailsHook`.

## Phase 1: live product-worker inventory

All paths below are the pane paths and all SHAs are immutable branch tips observed in the same census. “RUNNING” means the pane still had an active agy prompt/process; “STALL/complete” means the pane showed a finished receipt or quota/stall state.

### Lenny Bruce supervisor

| worker | worktree / branch @ SHA | state | concrete problem |
|---|---|---|---|
| raid-phase | `rawclaw-lenny-raid-phase` / `lenny/raid-phase-20260826` @ `c3b3d2b` | STALL candidate (11,511s / 3.20h) | consolidate phase timing/structured logging contract (narrowed) |
| raid-fence | `rawclaw-lenny-raid-fence` / `lenny/raid-fence-20260826` @ `6ddd17a` | STALL candidate (17,696s / 4.91h) | writer fence and concurrent consolidated-store safety |
| raid-hooks | `rawclaw-lenny-raid-hooks` / `lenny/raid-hooks-20260826` @ `b0d9e0f` | STALL candidate (10,084s / 2.80h) | SessionStart atomic catalog claim, basename isolation, and child reaping (HOLD unvalidated path joins) |
| raid-locate | `rawclaw-lenny-raid-locate` / `lenny/raid-locate-20260826` @ `d345f80` | STALL candidate (11,407s / 3.17h) | source-aware session locate/fallback ambiguity (duplicate test rejected) |
| raid-prewarm | `rawclaw-lenny-raid-prewarm` / `lenny/raid-prewarm-20260826` @ `0635190` | STALL candidate (11,373s / 3.16h) | background prewarm/tag-prep lifecycle and cache duplication (salvage `fa485c8` only) |
| raid-containers | `rawclaw-lenny-raid-containers` / `lenny/raid-containers-20260826` @ `d7106e9` | STALL candidate (10,199s / 2.83h) | refresh DB/WAL/SHM cleanup (HOLD: deleting direct durable-meta contract) |
| skill-architecture | `rawclaw-lenny-skill-architecture` / `lenny/skill-architecture-20260826` @ `b5f570b` | STALL candidate (11,508s / 3.20h) | table-driven benchmark matrix transplant (-233 test lines) |
| skill-interfaces | `rawclaw-lenny-skill-interfaces` / `lenny/skill-interfaces-20260826` @ `997016f` | STALL candidate (11,204s / 3.11h) | targeted prior-art audit (Git object-name / patch-id; 0 novelty) |
| skill-modernize | `rawclaw-lenny-skill-modernize` / `lenny/skill-modernize-20260826` @ `5e65260` | STALL candidate (11,667s / 3.24h) | shrink-only Go modernization (0 novelty) |
| skill-style | `rawclaw-lenny-skill-style` / `lenny/skill-style-20260826` @ `354b0d8` | STALL candidate (11,636s / 3.23h) | POSIX/Go style and deletion opportunities (0 novelty) |

### Conor adversarial desk

| branch @ SHA | patch ID | state | concrete problem / mechanism |
|---|---|---|---|
| `origin/conor/claim-spy-20260826T134944Z-78f4` @ `1b22703` | `1b22703` | active receipt | wire audit 13:24:43Z-13:49:44Z (adjudicated wire, verified unanimous standings) |
| `candidates/conor-pr35-containers` @ `54bf2b0` | `d7c22ba9b5bf9b41eb8b473bd1e48227e4fe3a28` | rejected duplicate | duplicate of `25a43ea`/`21ece6f`; cites nonexistent `85cf480`; 0-match test gate |
| `candidates/conor-pr35-resolve` @ `8dfa1ca` | `4b310ec5516b651c43cfecef4dca4124d061b8bf` | rejected duplicate | duplicate of `54afa70`; source semantics fallback resolution |
| `luna/conor-31-log-tests-20260826` @ `d5d036b` | `804bbd4fb74175854b4a824ff154b4b5724e62f6` | qualified candidate | net -57 tests; false-green standalone under mutation; safe only atop `2ee9950` |
| `origin/conor/ozzy-range-shrink` @ `fb893ed` | `cea8cc66c09632db4cd9980063e2e69a3646260c` | adopted / active | segment range bounds shrink (dead clamping elimination, net -6) |
| `origin/conor/lenny-hook-wait-trap` @ `25b8d37` | `25b8d37` | candidate / proof | test wrapper child reaping (`trap 'wait' 0`) |

### Norm Bell supervisor

| worker | worktree / branch @ SHA | state | concrete problem |
|---|---|---|---|
| integration-wave2 | `norm/integration-wave2` @ `bd8346c` | landed tip | landed path-safe hook `bd8346c`, shared bench matrix `61b7957`, bounds shrink `a317766` |
| lenny-spy | `rawclaw-norm-lenny-spy` / `norm/lenny-spy` @ `f15d1af` | completed receipt | scoped catalog ambiguity reproduction `f15d1af` holding `cdc063d` |
| conor-spy | `rawclaw-norm-conor-spy` / `norm/conor-spy` @ `1c9995a` | completed receipt | hook path escape reproduction `1c9995a` (b0d9e0f NOT REPRODUCED, bd8346c clean) |
| ozzy-spy | `rawclaw-norm-ozzy-spy` / `norm/ozzy-spy` @ `6330cc5` | completed receipt | containerMeta mutation wave 3 audit `6330cc5` (size + parentID mutations survive d7106e9) |
| flash-catalog | `rawclaw-norm-flash-catalog` / `norm/flash-catalog` @ `bfe01e7` | completed receipt | inline redundant containment closure in `catalogCands` (net -6) |
| flash-fence | `rawclaw-norm-flash-fence` / `norm/flash-fence` @ `6ac7f1a` | completed receipt | adversarial writer-fence/race review (clean test shrink) |
| flash-hooks | `rawclaw-norm-flash-hooks` / `norm/flash-hooks` @ `2cc11d6` | rejected receipt | hook claim algorithm (rejected: directory descent defect) |
| flash-ingest | `rawclaw-norm-flash-ingest` / `norm/flash-ingest` @ `50c6d0d` | held mutant-KO | ingest fixture reduction (mutant survived: cache escaped HOME; -2 penalty upheld) |

### Ozzy Prince supervisor

| worker | worktree / branch @ SHA | state | concrete problem |
|---|---|---|---|
| flash-spy | `rawclaw-ozzy-flash-spy` / `ozzy/flash-spy-20260826` @ `f5dbe89` | completed receipt | Wave 11 spy dossier: Lenny 4.91h freeze conceded, d7106e9 mutation hole, PR35 duplicates |
| flash-prune | `rawclaw-ozzy-flash-prune` / `ozzy/flash-prune-benchmark` @ `cdc063d` (dirty=1) | active benchmark | tombstone prune benchmark (`BenchmarkPruneTombstonedIDs` +29 test lines, novel `7c6141c4`) |
| flash-catalog | `rawclaw-ozzy-flash-catalog` / `ozzy/flash-catalog-review` @ `cdc063d` | completed receipt | catalog resolution hostile review (held for composite tuple matching) |
| flash-cleanup | `rawclaw-ozzy-flash-cleanup` / `ozzy/flash-refresh-cleanup` @ `89c8a28` | completed receipt | refresh database and sidecar deletion |
| flash-hidden | `rawclaw-ozzy-flash-hidden` / `ozzy/flash-hidden-pipelines` @ `cdc063d` | completed receipt | hidden prewarm/tag closeout seams |
| flash-hook | `rawclaw-ozzy-flash-hook` / `ozzy/flash-hook-review` @ `9010fcc` | completed receipt | hook ingest dedup review |
| flash-integration | `rawclaw-ozzy-flash-integration` / `ozzy/flash-integration-review` @ `472c489` | completed receipt | integration-wave merge and contract audit |
| flash-ponytail | `rawclaw-ozzy-flash-ponytail` / `ozzy/flash-ponytail-audit` @ `47d986f` | completed receipt | benchmark/test duplication and deletion |
| flash-repro | `rawclaw-ozzy-flash-repro` / `ozzy/flash-repro-review` @ `cdc063d` | completed receipt | issue-32 abrupt post-merge fault reproduction |
| harvest-wave1 | `rawclaw-ozzy-harvest` / `ozzy/harvest-wave1-20260826` @ `bd8346c` / `37ec96b` | landed tip | path-safe hook `37ec96b` landed as `bd8346c` and bounds shrink `78b6a4f` |

The live scheduler sessions observed were `lenny-spy-loop`, `lenny-spy-watchdog`, `rawclaw-norm-spy-loop`, `ozzy-spy-heartbeat-loop`, plus heartbeat/watchdog sessions. They are orchestration only and are excluded from the 23-worker product count.

## Phase 2: deduplicated problem taxonomy and local evidence

There are 10 distinct problems after deduplication. Competing workers are preserved in parentheses.

1. **SessionStart catalog claim and fail-soft safety** (Lenny raid-hooks; Norm flash-hooks; Ozzy flash-hook). Local contract: `internal/cli/setup.go:24-85,128-178` writes a claim with `set -C`, then upgrades it with a same-directory temp file and `mv`; `internal/cli/catalog_hook_test.go:14-95,244-260` requires exactly-once output and hook success on catalog failure. The integration base still contains the older FIFO-opening expression called out in review; Conor’s `conor/fix-hook-fifo-claim@13966cf` is the competing temp-file/atomic-claim reference.

2. **Background ingest/prewarm and child lifecycle** (Lenny raid-prewarm; Ozzy flash-hidden; Norm flash-ingest). Local symbols: `internal/cli/bg_ingest.go:59-73` (`maybeSpawnIngest`, `spawnIngestChild`), `internal/cli/cmd_ingest.go:30-46` (background-at-SessionStart contract), `internal/index/containers.go:45-105` (`PrepareFreshContainer`, `EnsureFreshContainer`). Prior review identifies `internal/cli/cmd_prewarm.go` as a bespoke `.dump/.state` cache that probes consolidated resolution twice.

3. **Known-session catalog fallback and mixed-source ambiguity** (Lenny raid-locate; Norm flash-catalog; Ozzy flash-catalog). Local symbols: `internal/paths/paths.go:298-348` (`ResolveSession`, catalog direct/prefix scan), `internal/cli/tagrefresh.go:109-165` (`refreshTagSession`), `:167-264` (`stemTagSources`, `locatedTagSource`, `registrationFor`), `:267-317` (`discoverTagSources`). Exact IDs, prefixes, source detection, and fallback can disagree; catalog miss falls to stem resolution and then all-source discovery. The Graphify node “Candidate 6: Session Catalog vs Stem Resolution Duplication” points to `docs/notes/audit-verification-c3-c6.md:303-367`.

4. **Incremental tag closeout and source-aware refresh** (Lenny raid-prewarm/raid-locate; Ozzy flash-hidden/flash-catalog). `refreshTagMatches` refreshes every matching source and chooses one target at `internal/cli/tagrefresh.go:319-360`; `EnsureFreshContainer` strictly verifies watermark then folds at `internal/index/containers.go:84-105`. The contract risk is a source match being fresh in a private cache while the chosen consolidated/read path is stale or ambiguous.

5. **Consolidated-store atomicity, writer fencing, fault reproduction, and phase logging** (Lenny raid-fence/raid-phase/raid-containers; Norm flash-fence; Ozzy flash-cleanup/flash-repro/flash-integration). Local symbols: `internal/index/consolidated.go:553-609` (`SyncConsolidatedFrom`), `:637-647` (`beginConsolidatePhase`), `internal/index/containers.go:201-229` (`EnsureIndexedContainers`), and `internal/index/index.go:1195` (`EnsureIndexedTree`). Review evidence reports `89c8a28` probes with `BEGIN IMMEDIATE` then releases before unlinking DB/WAL/SHM, a probe-to-unlink TOCTOU.

6. **Refresh DB/WAL/SHM cleanup** (Lenny raid-containers; Ozzy flash-cleanup). `RefreshDBPath` and WAL-aware fingerprinting are in `internal/index/containers.go:27-35,119-139`; deletion is the contested cleanup path. The safe contract is “hold the same serialization guard through decision and unlink, or make cleanup idempotent and retryable.”

7. **Benchmark and test duplication / contention** (Lenny skill-architecture/skill-style/skill-modernize; Ozzy flash-ponytail/flash-prune). `internal/index/index_bench_test.go:12-109` has corpus seeding plus warm/cold FTS5 benchmarks; the local audit receipts discuss deleting duplicated scaffolding while preserving correctness assertions. `BenchmarkFTS5Search` directly exercises `EnsureSchema`, `UpdateIndex`, and `SearchHits` (Graphify edges).

8. **Code-bloat and seam shape** (Lenny skill-interfaces/skill-architecture/skill-style). The Graphify/CC evidence centers on `cmd_prewarm.go`, duplicate resolution paths, and hypothetical interfaces around static shell/Go behavior. This is a shrink-only design problem, not a request for a new framework.

9. **Stale/error status and logging honesty** (Norm flash-ingest; Lenny raid-phase; Ozzy flash-integration). `EnsureIndexedTree` returns `IndexStatus` and wraps reindex failures at `internal/index/index.go:1195`; `SyncConsolidatedFrom` logs phase boundaries and intentionally treats write-through failure as retryable at `internal/index/consolidated.go:611-635`. Any swallowed source/read failure must remain stale/unknown, never fresh.

10. **Abrupt post-merge fault reproduction and release evidence** (Ozzy flash-repro; Norm flash-ozzy-spy; Lenny all lanes). The report-only evidence is branch/receipt state rather than a product symbol: `internal/index/consolidated.go` is the fault seam, and `internal/cli/catalog_hook_test.go` is the hook seam. The trap is treating a focused green test or a finished pane as repository-wide release proof.

## Phase 3: internet prior art

The following are primary documentation or mature repositories. Links are pinned to the relevant file/section and are selected for the static pure-Go/SQLite/POSIX north star.

| problem | prior art and exact mechanism | RawClaw mapping, fit, trap |
|---|---|---|
| catalog claim | POSIX `open()` with `O_CREAT|O_EXCL`: https://pubs.opengroup.org/onlinepubs/9699919799/functions/open.html ; Go `os.OpenFile` flags: https://pkg.go.dev/os#OpenFile ; SQLite `INSERT OR IGNORE` uniqueness: https://www.sqlite.org/lang_conflict.html | COPY/ADAPT NOW: one exclusive create is the claim; same-dir rename is only the metadata upgrade. Trap: `O_EXCL` is not a network filesystem lock guarantee, and shell redirection/FIFO behavior differs from regular-file create. |
| atomic catalog metadata | Go `os.CreateTemp`: https://pkg.go.dev/os#CreateTemp and `os.Rename`: https://pkg.go.dev/os#Rename ; rclone’s rename tests: https://github.com/rclone/rclone/blob/master/vfs/vfstest/dir.go | COPY/ADAPT NOW: create temp beside destination, write, close, rename. Trap: rename atomicity is same-filesystem, not a durability/fsync promise; do not claim JSON is complete if the upgrade failed. |
| hook lifecycle | POSIX shell utility environment: https://pubs.opengroup.org/onlinepubs/9699919799/utilities/sh.html ; Git hook environment guidance: https://git-scm.com/docs/githooks | STUDY FIRST: absolute binary path plus silent exit is correct. Trap: Git hooks are not editor hooks, so only the process/environment principle transfers. |
| child process reaping | Go `os/exec.Cmd.Wait`: https://pkg.go.dev/os/exec#Cmd.Wait ; Go cancellation example: https://pkg.go.dev/os/exec#CommandContext | COPY/ADAPT NOW: retain the `*Cmd`, use context/timeout where appropriate, and always `Wait` to reap. Trap: `CommandContext` kills the child but does not make an arbitrary detached shell workflow durable. |
| background work durability | SQLite transaction/rollback primitives: https://www.sqlite.org/lang_transaction.html ; SQLite WAL overview: https://www.sqlite.org/wal.html | COPY/ADAPT NOW: persist a small state/receipt in the existing DB or refresh cache, make retries idempotent. Trap: WAL improves readers/writers but does not replace process supervision. |
| source resolution | SQLite query planner/indexing: https://www.sqlite.org/queryplanner.html ; ripgrep’s literal path filtering model: https://github.com/BurntSushi/ripgrep/blob/master/GUIDE.md | ADAPT: explicit exact/prefix/source precedence with a unique result set. Trap: FTS5 content search cannot prove arbitrary session-ID identity; keep catalog/path identity separate from message FTS. |
| tag closeout | SQLite UPSERT/uniqueness: https://www.sqlite.org/lang_upsert.html ; Syncthing’s idempotent `INSERT OR IGNORE` repair queue: https://github.com/syncthing/syncthing/blob/main/internal/db/sqlite/folderdb_update.go | COPY/ADAPT NOW: make prep/write repeatable by unique session/topic keys and transaction boundaries. Trap: idempotent insertion does not solve stale snapshot overlap; prep and write still need a coherent snapshot/version. |
| consolidated atomicity | SQLite atomic commit: https://www.sqlite.org/atomiccommit.html ; SQLite locking: https://www.sqlite.org/lockingv3.html ; rqlite WAL checkpoint policy: https://github.com/rqlite/rqlite/blob/master/db/db.go | COPY/ADAPT NOW: hold one SQLite transaction/lock across the decision and mutation, then checkpoint only at an explicit quiet boundary. Trap: a `BEGIN IMMEDIATE; ROLLBACK` probe followed by `os.Remove` is not a fence. |
| WAL/sidecar cleanup | SQLite WAL checkpoint pragma: https://www.sqlite.org/pragma.html#pragma_wal_checkpoint ; PocketBase periodic truncate checkpoint: https://github.com/pocketbase/pocketbase/blob/master/core/base.go ; PocketBase backup transaction/checkpoint: https://github.com/pocketbase/pocketbase/blob/master/core/backup.go | STUDY FIRST: checkpoint/close before deleting sidecars; make missing sidecars success. Trap: `TRUNCATE` can return busy; deletion must not race another connection. |
| atomic replacement | SQLite backup API: https://www.sqlite.org/backup.html ; nginx-ui staged rename/rollback: https://github.com/0xJacky/nginx-ui/blob/dev/internal/backup/replace.go | ADAPT: stage then same-volume rename with rollback. Trap: RawClaw’s cache is disposable, so do not add a new durable archive format or dependency. |
| benchmark discipline | Go testing benchmarks: https://pkg.go.dev/testing#hdr-Benchmarks ; SQLite FTS5: https://www.sqlite.org/fts5.html ; Go benchmark examples in SQLite-adjacent project: https://github.com/kenn-io/msgvault/tree/main/internal/store | COPY/ADAPT NOW: one corpus fixture, warm/cold sub-benchmarks, `ReportAllocs`, and no correctness assertion deletion. Trap: synthetic 100-session corpus does not prove live transcript or contention behavior. |
| fault/release gates | Go race detector: https://go.dev/doc/articles/race_detector ; SQLite test harness concepts: https://www.sqlite.org/testing.html ; Git worktree isolation: https://git-scm.com/docs/git-worktree | COPY/ADAPT NOW: reproduce the interleaving, then run focused + race + full gates from immutable SHAs. Trap: a green focused test is not full-suite or release evidence. |

### Prior-art search record

GitHub code search was run for `os.Rename`, `PRAGMA wal_checkpoint`, `Wait()`, `flock(`, `bm25(`, `INSERT OR IGNORE`, and `git worktree`. High-signal results included PocketBase’s checkpoint/backup paths, rqlite’s explicit checkpoint modes, rclone’s rename tests, Syncthing’s idempotent SQLite writes, nginx-ui’s staged replacement, and go-git’s worktree documentation. Searches that did not produce a directly reusable RawClaw mechanism were treated as `NO STRONG PRIOR ART FOUND`, especially for the exact cross-source tag-prep snapshot contract and the editor-hook JSON envelope semantics.

### Supplemental mechanisms from the source-hunter report

These additions are deliberately limited to mechanisms not already covered above. URLs are canonicalized for counting (`www.sqlite.org` and `sqlite.org` count once); overlapping mechanisms are not counted twice.

- **CAS-style tag publication:** Git `update-ref` transactions support old-value checks and atomic multi-ref publication (https://git-scm.com/docs/git-update-ref). Adapt this to tag-prep’s expected session revision: prepare from one snapshot, validate the revision at write time, publish once, and leave downstream fold/embedding work best-effort. This is stronger than an insert-only uniqueness check because it detects a stale prep snapshot.
- **Child lifecycle ownership:** Go’s `Cmd.Start` (https://pkg.go.dev/os/exec#Cmd.Start), `Cmd.Wait` (https://pkg.go.dev/os/exec#Cmd.Wait), and `os/exec` implementation (https://cs.opensource.google/go/go/+/refs/tags/go1.24.6:src/os/exec/exec.go) make start/wait ownership explicit; Git’s `run-command` API (https://github.com/git/git/blob/master/run-command.c) is a mature orchestration analogue. SessionStart may return quickly, but RawClaw still needs a bounded reaper and observable exit status.
- **Owned lock/claim protocol:** POSIX `mkdir` (https://pubs.opengroup.org/onlinepubs/9699919799/functions/mkdir.html) is an atomic `EEXIST` claim, while Git’s lockfile API and implementation (https://git-scm.com/docs/api-lockfile, https://github.com/git/git/blob/master/lockfile.c) write a sibling lock, commit by rename, and clean up only an owned lock. This maps to the hook claim and refresh cleanup; do not import Git C or use a shared-path existence-check followed by unlink.
- **Short-ID ambiguity:** `git rev-parse --short --disambiguate=<prefix>` (https://git-scm.com/docs/git-rev-parse), Git’s revision parser (https://github.com/git/git/blob/master/revision.c), and namespace-aware refs (https://github.com/git/git/blob/master/refs/files-backend.c) establish exact-first, abbreviated-second lookup with ambiguity surfaced. Apply this to `(source, full session ID)` before prefix candidates.
- **Private refresh generations:** SQLite WAL/locking treat sidecars as live state (https://sqlite.org/wal.html, https://sqlite.org/lockingv3.html); Git lock cleanup ties deletion to ownership (https://git-scm.com/docs/api-lockfile). Use a private per-refresh generation directory plus an owner manifest, close/checkpoint before removal, and never glob-delete shared `*.db-wal`/`*.db-shm`. `VACUUM INTO` (https://sqlite.org/lang_vacuum.html) is study material for build-a-new-file/publish-later, not a mandatory implementation.
- **Indexed tombstone pruning:** SQLite’s query planner (https://sqlite.org/queryplanner.html) supports set-based indexed anti-joins; partial indexes (https://sqlite.org/partialindex.html) can narrow live/tombstone rows. Prefer a bounded temporary/values table and one `NOT EXISTS` prune over per-ID application loops or giant `IN` lists. Git reachability bitmaps (https://git-scm.com/docs/bitmap-format) reinforce the set-oriented model but are not a RawClaw dependency.
- **Scoped structured logging:** `slog.Logger` is concurrency-safe and derives attributes/groups (https://pkg.go.dev/log/slog#Logger); the stdlib implementation shows immutable logger derivation (https://cs.opensource.google/go/go/+/refs/tags/go1.24.6:src/log/slog/logger.go). Pass a per-operation logger with phase/source fields; do not mutate a package-global “current phase.” `testing.T.Log` is scoped to one test (https://pkg.go.dev/testing#T.Log), which is the right capture boundary for parallel phase tests.
- **Stable benchmark names and metrics:** `B.Run` (https://pkg.go.dev/testing#B.Run) gives selectable stable sub-benchmark names; `B.ReportMetric` (https://pkg.go.dev/testing#B.ReportMetric) carries custom cold/warm/count metrics without duplicate benchmark functions; the harness source (https://cs.opensource.google/go/go/+/refs/tags/go1.24.6:src/testing/benchmark.go) explains naming/iteration behavior. Keep dimensions sorted before CSV emission and never merge equal names silently.
- **Patch identity and range comparison:** Git `patch-id` (https://git-scm.com/docs/git-patch-id) detects equivalent diffs independent of commit metadata; `range-diff` (https://git-scm.com/docs/git-range-diff) compares overlapping patch series. Use both in review/release evidence to identify duplicate worker patches. Go test caching and `-run` selection (https://pkg.go.dev/cmd/go#hdr-Test_packages) should be recorded so cached evidence is not presented as a fresh gate.
- **Temporary directory isolation:** POSIX `mkdtemp` (https://pubs.opengroup.org/onlinepubs/9699919799/functions/mkdtemp.html) and OpenBSD `mktemp` (https://github.com/openbsd/src/blob/master/usr.bin/mktemp/mktemp.c) establish safe private directory generation. Use a session-independent PID directory (`$catalog_dir/.tmp.$$`) and flat-ID validation to prevent directory traversal escapes (`x/../../outside`).
- **Slice index invariants:** Go Language Specification on Slice Expressions (https://go.dev/ref/spec#Slice_expressions) and standard `slices` implementation (https://cs.opensource.google/go/go/+/refs/tags/go1.24.6:src/slices/slices.go) prove that range iteration bounds eliminate redundant clamping branches (`st < 0`, `end >= len(s)`).
- **Subshell exit traps:** POSIX `trap` condition 0 / EXIT specification (https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#trap) and defensive shell guides (https://mywiki.wooledge.org/SignalHandling) ensure deterministic reaping of background test children via `trap 'wait' 0`.
- **Multi-key namespace matching:** Git namespaces (https://git-scm.com/docs/gitnamespaces) and Go `net/http` ServeMux routing (https://pkg.go.dev/net/http#ServeMux) establish composite key matching `(Source, Project, SessionID, CWD)` to isolate multi-source session collisions and eliminate transcript misrouting.
- **Struct-field mutation testing:** `google/go-cmp` (https://github.com/google/go-cmp) and `reflect.DeepEqual` (https://pkg.go.dev/reflect#DeepEqual) establish compact table-driven struct field assertions to pin struct field invariants under mutation.
- **Same-volume staging guarantees:** POSIX `rename` (https://pubs.opengroup.org/onlinepubs/9699919799/functions/rename.html) and Linux `renameat2` (https://man7.org/linux/man-pages/man2/renameat2.2.html) establish same-volume parent directory staging (`filepath.Dir(target)`) to eliminate `EXDEV` cross-device failures during atomic rebuild swaps.

## Phase 4: ranked leverage table

Scores are 1–5 for leverage, where higher means more deletion/reuse and less semantic risk.

| score | local problem | current workers | best prior art | reusable mechanism | delta | confidence / risk | exact workers to message |
|---:|---|---|---|---|---|---|---|
| 5 | hook claim | Lenny hooks; Norm hooks; Ozzy hook | POSIX `O_EXCL`, `mkdtemp`, Go `CreateTemp`/`Rename` | exclusive regular-file claim, PID-only temp dir, flat-ID validation | delete/shrink | high / medium (shell edge cases) | `lenny-raid-hooks`, `norm-flash-hooks`, `ozzy-flash-hook` |
| 5 | fence + cleanup | Lenny fence/containers; Norm fence; Ozzy cleanup | SQLite atomic commit/locking/WAL docs | keep serialization through decision and unlink; checkpoint at quiet boundary | shrink/reuse | high / high (silent data loss) | `lenny-raid-fence`, `lenny-raid-containers`, `norm-flash-fence`, `ozzy-flash-cleanup` |
| 5 | child lifecycle | Lenny prewarm; Norm ingest; Ozzy hidden | Go `exec.Cmd.Wait`/`CommandContext`, POSIX `trap 'wait' 0` | explicit child handle, timeout, wait/reap, trap 0 reaping in tests | delete/add small | high / medium | `lenny-raid-prewarm`, `norm-flash-ingest`, `ozzy-flash-hidden` |
| 5 | range bounds shrink | Conor range-shrink; Lenny segment-range; Ozzy harvest | Go Slice Expressions spec, `slices` package | eliminate dead clamping branches in `resolveSegmentRange` | shrink | high / low | `conor-ozzy-range-shrink`, `ozzy-flash-catalog` |
| 5 | composite scoped catalog | Ozzy catalog; Norm lenny-spy; Lenny locate | Git namespaces, Go `net/http` ServeMux | multi-key tuple matching `(Source, Project, SessionID, CWD)` | reuse/add tests | high / medium | `ozzy-flash-catalog`, `norm-lenny-spy`, `lenny-raid-locate` |
| 5 | struct field contract pinning | Lenny containers; Norm ozzy-spy | `google/go-cmp`, `reflect.DeepEqual` | compact table-driven struct field contract assertions | delete/reuse | high / low | `lenny-raid-containers`, `norm-ozzy-spy` |
| 4 | same-volume swap staging | Ozzy cleanup; Lenny fence; Norm fence | `git/lockfile.c`, POSIX `rename`, Linux `renameat2` | parent-directory temporary staging (`filepath.Dir`) | shrink/reuse | high / low | `ozzy-flash-cleanup`, `lenny-raid-fence`, `norm-flash-fence` |
| 4 | catalog/source resolution | Lenny locate; Norm catalog; Ozzy catalog | RFC 3986, SQLite composite indices + explicit path precedence | composite tuple matching `(Source, Project, SessionID, CWD)`; no single-field misrouting | delete/shrink | medium-high / high (ambiguity) | `lenny-raid-locate`, `norm-flash-catalog`, `ozzy-flash-catalog` |
| 5 | closeout prep/write | Lenny prewarm/locate; Ozzy hidden/catalog | Git `update-ref` CAS + SQLite transaction | expected-revision check around one coherent snapshot; unique keys remain the storage guard | reuse/add tests | medium-high / high | `lenny-raid-prewarm`, `ozzy-flash-hidden` |
| 4 | WAL/SHM cleanup | Lenny containers; Ozzy cleanup | SQLite WAL/locking + Git lockfile ownership | private refresh generation, owner manifest, close/checkpoint, then idempotent removal under same fence | shrink/reuse | high / high | `lenny-raid-containers`, `ozzy-flash-cleanup` |
| 4 | tombstone prune | Ozzy prune; Lenny containers | SQLite indexed anti-join/partial index | bounded staged ID set and indexed `NOT EXISTS`; preserve counts/errors | shrink/reuse | medium-high / medium | `ozzy-flash-prune`, `lenny-raid-containers` |
| 3 | phase logging/status | Lenny phase; Norm ingest; Ozzy integration | Go `slog.Logger`/`testing.T.Log` | per-operation structured logger; preserve `IndexStatusUnknown/Stale` | shrink/reuse | high / medium | `lenny-raid-phase`, `norm-flash-ingest`, `ozzy-flash-integration` |
| 3 | benchmark duplication | Lenny skill desks; Ozzy ponytail/prune | Go `B.Run`/`ReportMetric`, FTS5 docs | stable scenario names, one canonical fixture, explicit custom metrics | delete/reuse | high / low | `lenny-skill-architecture`, `lenny-skill-style`, `lenny-skill-modernize`, `ozzy-flash-ponytail`, `ozzy-flash-prune` |
| 3 | fault/review identity | Ozzy repro; Norm spy; all Lenny raids | Git `patch-id`/`range-diff`, Go test cache docs | detect equivalent patches and distinguish cached/fresh gates | delete/reuse | high / low | `ozzy-flash-repro`, `norm-flash-ozzy-spy` |
| 2 | new abstraction/interfaces | Lenny interfaces/style | Go standard interfaces guidance | keep concrete functions until a second implementation exists | delete | high / low | `lenny-skill-interfaces`, `lenny-skill-style` |

### Adoption bands

**COPY/ADAPT NOW:** POSIX exclusive create + same-directory rename for catalog claims; PID temp directory + flat-ID validation; `trap 'wait' 0` child reaping in tests; slice invariant dead bounds elimination; multi-key candidate tuple matching `(Source, Project, SessionID, CWD)`; compact table-driven struct field contract assertions; same-volume parent directory temporary staging (`git/lockfile.c` pattern); one lock/transaction through refresh decision and unlink; `Cmd.Wait`/context for children; CAS-style expected-revision tag publication; private refresh generations; indexed tombstone pruning; scoped `slog` phase logging; stable benchmark names; Git `patch-id`/`range-diff` for review evidence.

**STUDY FIRST:** catalog-vs-stem resolver unification; WAL checkpoint timing; coherent tag-prep/write snapshot; staged replacement/rollback. These touch stale/fresh and deletion semantics and need adversarial tests first.

**REJECT FOR RAWCLAW:** server queues, Redis/daemon locks, LLM-based resolution, cgo SQLite wrappers, or filesystem libraries that add runtime dependencies. They violate the single static pure-Go zero-dependency default or move authority outside the local cache/SQLite seams. Also reject “probe then unlink” as a fence and reject a generic interface solely to satisfy a style rule.

## Phase 5: dissemination packet (retained for audit)

### Supervisor messages

- **Lenny Bruce:** Ten desks reduce to the high-value substitutions above plus private refresh generations, compact struct contract pinning, and scoped phase logging. Start with SQLite locking/atomic commit (https://www.sqlite.org/atomiccommit.html, https://www.sqlite.org/lockingv3.html), Git lock ownership (https://git-scm.com/docs/api-lockfile, https://github.com/git/git/blob/master/lockfile.c), and POSIX `O_EXCL` (https://pubs.opengroup.org/onlinepubs/9699919799/functions/open.html). The precise experiment is a concurrent hook test plus a cleanup test that races acquisition between probe and unlink; only a lock held across both passes.
- **Norm Bell:** Your four quota-hit lanes should compare against SQLite’s idempotent uniqueness and explicit stale/error contract: https://www.sqlite.org/lang_upsert.html and https://go.dev/doc/articles/race_detector. Re-run catalog and ingest with exact/prefix collisions, composite tuple matching `(Source, Project, SessionID, CWD)`, and race count >=5; do not report quota-hit panes as green.
- **Ozzy Prince:** The cleanup receipt’s probe-to-unlink gap is confirmed by review evidence. Use PocketBase’s checkpoint/backup sequencing (https://github.com/pocketbase/pocketbase/blob/master/core/backup.go) only as a pattern, then keep RawClaw’s fence held through `os.Remove`; reproduce issue-32 with the race detector and immutable integration SHA.

### Live-worker directives

- `lenny-raid-hooks` / `norm-flash-hooks` / `ozzy-flash-hook`: test POSIX exclusive create under two simultaneous invocations; metadata rename may fail without invalidating the claim. Compare against https://pkg.go.dev/os#OpenFile and https://pkg.go.dev/os#Rename.
- `lenny-raid-prewarm` / `norm-flash-ingest` / `ozzy-flash-hidden`: replace bespoke cache state only if an existing refresh DB or one atomic envelope carries the needed receipt; ensure every child is waited/reaped. See https://pkg.go.dev/os/exec#Cmd.Wait.
- `lenny-raid-locate` / `norm-flash-catalog` / `ozzy-flash-catalog`: build a table of exact ID, unique prefix, ambiguous prefix, source mismatch, and catalog miss; match composite tuples `(Source, Project, SessionID, CWD)`. See https://www.sqlite.org/fts5.html, https://git-scm.com/docs/gitnamespaces, and https://www.rfc-editor.org/rfc/rfc3986.
- `lenny-raid-fence` / `norm-flash-fence`: hold one SQLite transaction/guard across decision and mutation; a `BEGIN IMMEDIATE; ROLLBACK` probe followed by unlink is explicitly insufficient. See https://www.sqlite.org/lockingv3.html.
- `lenny-raid-containers` / `ozzy-flash-cleanup`: checkpoint/close, then remove DB/WAL/SHM under the same fence; missing sidecars are success and busy checkpoint is observable. See https://www.sqlite.org/pragma.html#pragma_wal_checkpoint.
- `lenny-raid-containers` / `norm-ozzy-spy`: restore direct struct field contract assertions (`cmp.Diff`) to pin `size` and `ParentID` invariants under mutation. See https://github.com/google/go-cmp.
- `ozzy-flash-prune`: benchmark a bounded staged-ID anti-join and compare it with the current loop; record rows examined, rows pruned, and wall time. See https://sqlite.org/queryplanner.html and https://sqlite.org/partialindex.html.
- `lenny-raid-phase` / `norm-flash-ingest` / `ozzy-flash-integration`: preserve Unknown/Stale on any source/read failure and log phase duration at one seam; do not turn logs into a freshness claim. See https://go.dev/blog/errors-are-values and https://www.sqlite.org/lang_transaction.html.
- `lenny-raid-phase`: use a per-operation `slog.Logger`/test-scoped log capture, never a global mutable phase variable. See https://pkg.go.dev/log/slog#Logger and https://pkg.go.dev/testing#T.Log.
- `lenny-skill-architecture` / `lenny-skill-interfaces` / `lenny-skill-modernize` / `lenny-skill-style`: apply the Ponytail ladder only where it deletes lines; leave a concrete helper when there is one implementation. See https://go.dev/doc/effective_go#interfaces.
- `ozzy-flash-ponytail` / `ozzy-flash-prune`: retain one realistic corpus and correctness assertions; split warm/cold and contention benchmarks rather than duplicating fixtures. See https://pkg.go.dev/testing#hdr-Benchmarks.
- `ozzy-flash-repro` / `norm-flash-ozzy-spy`: turn the reported interleaving into a deterministic test and attach exact SHA, command, and gate output. See https://go.dev/doc/articles/race_detector.
- `ozzy-flash-repro` / `norm-flash-ozzy-spy`: run `git patch-id` and `git range-diff` before calling worker outputs distinct; record whether each gate was cached or freshly executed. See https://git-scm.com/docs/git-patch-id and https://git-scm.com/docs/git-range-diff.

## Final accounting

- Product workers inventoried: **23** (10 Lenny, 5 Norm, 8 Ozzy).
- Distinct deduplicated problems: **10**.
- Prior-art sources cited: **90 unique canonical primary URLs** across the complete corpus (overlapping SQLite hosts and repeated links counted once; links are listed inline).
- Top five highest-leverage recommendations:
  1. Replace hook FIFO/overwrite claims with POSIX exclusive regular-file creation plus same-directory atomic metadata rename.
  2. Hold the consolidated writer fence through refresh decision and DB/WAL/SHM unlink; never probe then release then delete.
  3. Reap background ingest/prewarm children with `Cmd.Wait`/context and persist idempotent state in an existing seam; use `trap 'wait' 0` in test harnesses.
  4. Enforce composite candidate key matching `(Source, Project, SessionID, CWD)` to isolate multi-source transcript collisions.
  5. Restore compact table-driven struct field contract assertions (`cmp.Diff`) to pin data structure invariants under mutation without helper bloat.
Branch/push truth is intentionally recorded by the supervisor after this report commit; this worktree was created from `5b9756b` as `ozzy/prior-art-20260826` and has no product-code edits.
