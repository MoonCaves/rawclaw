# RawClaw worker problem and prior-art map

Snapshot taken 2026-08-26 from integration head `5b9756b2200ff6bd670f07407407d84d9f42d84b`.
This is a report-only census. Product code and rival worktrees were not edited.

## Scope and evidence rules

The live census used `tmux list-sessions`, `tmux list-panes -a -F '#{session_name}\t#{pane_pid}\t#{pane_current_path}\t#{pane_current_command}'`, `ps`, `git worktree list --porcelain`, branch SHA/status reads, `.codex-run.log` tails, and the four requested mailboxes. Heartbeat/watchdog/spy loops are listed as orchestration evidence but are not counted as product problems. A pane marked live means its tmux/agy process was present at census time; a completed branch means its report was visible in the pane, not that its design was accepted.

Graphify was queried first. Relevant graph symbols are `renderHookScript`, `catalogIngestSource`, `ResolveSession`, `refreshTagSession`, `discoverTagSources`, `EnsureFreshContainer`, `PrepareFreshContainer`, `EnsureIndexedTree`, `SyncConsolidatedFrom`, `StampIngestWatermark`, `BenchmarkFTS5Search`, `spawnIngestChild`, and `TestPrimeScript_CatalogWriteFailure_NeverFailsHook`.

## Phase 1: live product-worker inventory

All paths below are the pane paths and all SHAs are immutable branch tips observed in the same census. “RUNNING” means the pane still had an active agy prompt/process; “STALL/complete” means the pane showed a finished receipt or quota/stall state.

### Lenny Bruce supervisor

| worker | worktree / branch @ SHA | state | concrete problem |
|---|---|---|---|
| raid-phase | `rawclaw-lenny-raid-phase` / `lenny/raid-phase-20260826` @ `dd57060` | RUNNING receipt | consolidate phase timing/structured logging contract |
| raid-fence | `rawclaw-lenny-raid-fence` / `lenny/raid-fence-20260826` @ `6ddd17a` | STALL candidate | writer fence and concurrent consolidated-store safety |
| raid-hooks | `rawclaw-lenny-raid-hooks` / `lenny/raid-hooks-20260826` @ `7a78884` | completed receipt | SessionStart atomic catalog claim and fail-soft POSIX hook |
| raid-locate | `rawclaw-lenny-raid-locate` / `lenny/raid-locate-20260826` @ `fc1a075` | STALL candidate | source-aware session locate/fallback ambiguity |
| raid-prewarm | `rawclaw-lenny-raid-prewarm` / `lenny/raid-prewarm-20260826` @ `229f7e9` | STALL candidate | background prewarm/tag-prep lifecycle and cache duplication |
| raid-containers | `rawclaw-lenny-raid-containers` / `lenny/raid-containers-20260826` @ `be4ef6c` | STALL candidate | refresh DB/WAL/SHM cleanup and lock lifetime |
| skill-architecture | `rawclaw-lenny-skill-architecture` / `lenny/skill-architecture-20260826` @ `65f3b8b` | completed receipt | architecture/code-bloat audit |
| skill-interfaces | `rawclaw-lenny-skill-interfaces` / `lenny/skill-interfaces-20260826` @ `6209534` | STALL candidate | unnecessary interfaces and seam shape |
| skill-modernize | `rawclaw-lenny-skill-modernize` / `lenny/skill-modernize-20260826` @ `7bf86ec` | STALL candidate | shrink-only Go modernization |
| skill-style | `rawclaw-lenny-skill-style` / `lenny/skill-style-20260826` @ `37e4f70` | STALL candidate | POSIX/Go style and deletion opportunities |

Representative local receipts: `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T095428Z-3b6748eb-lenny-heartbeat-36-receipts-or.md`, `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T071754Z-16cf50b0-codex-elegance-review-cmd-prew.md`.

### Norm Bell supervisor

| worker | worktree / branch @ SHA | state | concrete problem |
|---|---|---|---|
| flash-catalog | `rawclaw-norm-flash-catalog` / `norm/flash-catalog` @ `cc7619e` (dirty=1) | quota/stalled | catalog fallback, exact/prefix ambiguity, mixed-source status |
| flash-fence | `rawclaw-norm-flash-fence` / `norm/flash-fence` @ `6ddd17a` | quota/stalled | adversarial writer-fence/race review |
| flash-hooks | `rawclaw-norm-flash-hooks` / `norm/flash-hooks` @ `2b60e72` (dirty=2) | quota/stalled | hook claim algorithm; compare FIFO claim with temp JSON/atomic rename |
| flash-ingest | `rawclaw-norm-flash-ingest` / `norm/flash-ingest` @ `7478bfd` (dirty=1) | quota/stalled | ingest retry/failure/status contract |
| flash-ozzy-spy | `rawclaw-norm-ozzy-spy` / `norm/ozzy-spy` @ `3530005` | completed receipt | audit worker claims, dirty trees, and unsafe cleanup |

Representative local receipt: `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox/20260826T095910Z-550573e7-norm-bell-11-ozzy-bite-through.md`.

### Ozzy Prince supervisor

| worker | worktree / branch @ SHA | state | concrete problem |
|---|---|---|---|
| flash-catalog | `rawclaw-ozzy-flash-catalog` / `ozzy/flash-catalog-review` @ `cdc063d` | completed receipt | catalog resolution hostile review |
| flash-cleanup | `rawclaw-ozzy-flash-cleanup` / `ozzy/flash-refresh-cleanup` @ `89c8a28` | completed receipt | refresh database and sidecar deletion |
| flash-hidden | `rawclaw-ozzy-flash-hidden` / `ozzy/flash-hidden-pipelines` @ `cdc063d` | completed receipt | hidden prewarm/tag closeout seams |
| flash-hook | `rawclaw-ozzy-flash-hook` / `ozzy/flash-hook-review` @ `9010fcc` | completed receipt | hook ingest dedup review |
| flash-integration | `rawclaw-ozzy-flash-integration` / `ozzy/flash-integration-review` @ `472c489` | completed receipt | integration-wave merge and contract audit |
| flash-ponytail | `rawclaw-ozzy-flash-ponytail` / `ozzy/flash-ponytail-audit` @ `47d986f` | completed receipt | benchmark/test duplication and deletion |
| flash-prune | `rawclaw-ozzy-flash-prune` / `ozzy/flash-prune-benchmark` @ `cdc063d` (dirty=1) | quota/waiting | tombstone prune performance benchmark |
| flash-repro | `rawclaw-ozzy-flash-repro` / `ozzy/flash-repro-review` @ `cdc063d` | completed receipt | issue-32 abrupt post-merge fault reproduction |

Representative local receipts: `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox/20260826T100014Z-7bbc7513-conor-correction-wire-ozzy-cle.md`, `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox/20260826T100806Z-norm-ozzy-spy-blast.md`.

The live scheduler sessions observed were `lenny-spy-loop`, `lenny-spy-watchdog`, `rawclaw-norm-spy-loop`, `ozzy-spy-heartbeat-loop`, plus heartbeat/watchdog sessions. They are orchestration only and are excluded from the 23-worker product count.

## Phase 2: deduplicated problem taxonomy and local evidence

There are 10 distinct problems after deduplication. Competing workers are preserved in parentheses.

1. **SessionStart catalog claim and fail-soft safety** (Lenny raid-hooks; Norm flash-hooks; Ozzy flash-hook). Local contract: `internal/cli/setup.go:24-85,128-178` writes a claim with `set -C`, then upgrades it with a same-directory temp file and `mv`; `internal/cli/catalog_hook_test.go:14-95,244-260` requires exactly-once output and hook success on catalog failure. The integration base still contains the older FIFO-opening expression called out in the supervisor message; Conor’s `conor/fix-hook-fifo-claim@13966cf` is the competing temp-file/atomic-claim reference. Receipts: `/Users/jay-m4/code/rawclaw/.agent-mailbox-norm/20260826T100820Z-norm-ack-ozzy-ammunition.md` and the Lenny heartbeat above.

2. **Background ingest/prewarm and child lifecycle** (Lenny raid-prewarm; Ozzy flash-hidden; Norm flash-ingest). Local symbols: `internal/cli/bg_ingest.go:59-73` (`maybeSpawnIngest`, `spawnIngestChild`), `internal/cli/cmd_ingest.go:30-46` (background-at-SessionStart contract), `internal/index/containers.go:45-105` (`PrepareFreshContainer`, `EnsureFreshContainer`). The CC review receipt identifies `internal/cli/cmd_prewarm.go` as a bespoke `.dump/.state` cache that probes consolidated resolution twice: `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T071754Z-16cf50b0-codex-elegance-review-cmd-prew.md`.

3. **Known-session catalog fallback and mixed-source ambiguity** (Lenny raid-locate; Norm flash-catalog; Ozzy flash-catalog). Local symbols: `internal/paths/paths.go:298-348` (`ResolveSession`, catalog direct/prefix scan), `internal/cli/tagrefresh.go:109-165` (`refreshTagSession`), `:167-264` (`stemTagSources`, `locatedTagSource`, `registrationFor`), `:267-317` (`discoverTagSources`). Exact IDs, prefixes, source detection, and fallback can disagree; catalog miss falls to stem resolution and then all-source discovery. The Graphify node “Candidate 6: Session Catalog vs Stem Resolution Duplication” points to `docs/notes/audit-verification-c3-c6.md:303-367`.

4. **Incremental tag closeout and source-aware refresh** (Lenny raid-prewarm/raid-locate; Ozzy flash-hidden/flash-catalog). `refreshTagMatches` refreshes every matching source and chooses one target at `internal/cli/tagrefresh.go:319-360`; `EnsureFreshContainer` strictly verifies watermark then folds at `internal/index/containers.go:84-105`. The contract risk is a source match being fresh in a private cache while the chosen consolidated/read path is stale or ambiguous.

5. **Consolidated-store atomicity, writer fencing, fault reproduction, and phase logging** (Lenny raid-fence/raid-phase/raid-containers; Norm flash-fence; Ozzy flash-cleanup/flash-repro/flash-integration). Local symbols: `internal/index/consolidated.go:553-609` (`SyncConsolidatedFrom`), `:637-647` (`beginConsolidatePhase`), `internal/index/containers.go:201-229` (`EnsureIndexedContainers`), and `internal/index/index.go:1195` (`EnsureIndexedTree`). The local Conor receipt reports `89c8a28` probes with `BEGIN IMMEDIATE` then releases before unlinking DB/WAL/SHM, a probe-to-unlink TOCTOU (`/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox/20260826T100014Z-7bbc7513-conor-correction-wire-ozzy-cle.md`).

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
| WAL/sidecar cleanup | SQLite WAL checkpoint pragma: https://www.sqlite.org/pragma.html#pragma_wal_checkpoint ; PocketBase periodic truncate checkpoint: https://github.com/pocketbase/pocketbase/blob/master/core/base.go ; PocketBase backup transaction/checkpoint: https://github.com/pocketbase/pocketbase/blob/master/core/base_backup.go | STUDY FIRST: checkpoint/close before deleting sidecars; make missing sidecars success. Trap: `TRUNCATE` can return busy; deletion must not race another connection. |
| atomic replacement | SQLite backup API: https://www.sqlite.org/backup.html ; nginx-ui staged rename/rollback: https://github.com/0xJacky/nginx-ui/blob/dev/internal/backup/replace.go | ADAPT: stage then same-volume rename with rollback. Trap: RawClaw’s cache is disposable, so do not add a new durable archive format or dependency. |
| benchmark discipline | Go testing benchmarks: https://pkg.go.dev/testing#hdr-Benchmarks ; SQLite FTS5: https://www.sqlite.org/fts5.html ; Go benchmark examples in SQLite-adjacent project: https://github.com/kenn-io/msgvault/tree/main/internal/db | COPY/ADAPT NOW: one corpus fixture, warm/cold sub-benchmarks, `ReportAllocs`, and no correctness assertion deletion. Trap: synthetic 100-session corpus does not prove live transcript or contention behavior. |
| fault/release gates | Go race detector: https://go.dev/doc/articles/race_detector ; SQLite test harness concepts: https://www.sqlite.org/testing.html ; Git worktree isolation: https://git-scm.com/docs/git-worktree | COPY/ADAPT NOW: reproduce the interleaving, then run focused + race + full gates from immutable SHAs. Trap: a green focused test is not full-suite or release evidence. |

### Prior-art search record

GitHub code search was run for `os.Rename`, `PRAGMA wal_checkpoint`, `Wait()`, `flock(`, `bm25(`, `INSERT OR IGNORE`, and `git worktree`. High-signal results included PocketBase’s checkpoint/backup paths, rqlite’s explicit checkpoint modes, rclone’s rename tests, Syncthing’s idempotent SQLite writes, nginx-ui’s staged replacement, and go-git’s worktree documentation. Searches that did not produce a directly reusable RawClaw mechanism were treated as `NO STRONG PRIOR ART FOUND`, especially for the exact cross-source tag-prep snapshot contract and the editor-hook JSON envelope semantics.

## Phase 4: ranked leverage table

Scores are 1–5 for leverage, where higher means more deletion/reuse and less semantic risk.

| score | local problem | current workers | best prior art | reusable mechanism | delta | confidence / risk | exact workers to message |
|---:|---|---|---|---|---|---|---|
| 5 | hook claim | Lenny hooks; Norm hooks; Ozzy hook | POSIX `O_EXCL`, Go `CreateTemp`/`Rename` | exclusive regular-file claim, same-dir metadata upgrade | delete/shrink | high / medium (shell edge cases) | `lenny-raid-hooks`, `norm-flash-hooks`, `ozzy-flash-hook` |
| 5 | fence + cleanup | Lenny fence/containers; Norm fence; Ozzy cleanup | SQLite atomic commit/locking/WAL docs | keep serialization through decision and unlink; checkpoint at quiet boundary | shrink/reuse | high / high (silent data loss) | `lenny-raid-fence`, `lenny-raid-containers`, `norm-flash-fence`, `ozzy-flash-cleanup` |
| 5 | child lifecycle | Lenny prewarm; Norm ingest; Ozzy hidden | Go `exec.Cmd.Wait`/`CommandContext` | explicit child handle, timeout, wait/reap, durable idempotent state | delete/add small | high / medium | `lenny-raid-prewarm`, `norm-flash-ingest`, `ozzy-flash-hidden` |
| 4 | catalog/source resolution | Lenny locate; Norm catalog; Ozzy catalog | SQLite uniqueness + explicit path/index precedence | one resolver returning source/path/status; no duplicate catalog/stem topology | delete/shrink | medium-high / high (ambiguity) | `lenny-raid-locate`, `norm-flash-catalog`, `ozzy-flash-catalog` |
| 4 | closeout prep/write | Lenny prewarm/locate; Ozzy hidden/catalog | SQLite UPSERT and Syncthing transaction pattern | unique keys + one coherent snapshot/version | reuse/add tests | medium / high | `lenny-raid-prewarm`, `ozzy-flash-hidden` |
| 4 | WAL/SHM cleanup | Lenny containers; Ozzy cleanup | SQLite checkpoint + PocketBase backup | close/checkpoint, then idempotent sidecar removal under same fence | shrink | high / high | `lenny-raid-containers`, `ozzy-flash-cleanup` |
| 3 | phase logging/status | Lenny phase; Norm ingest; Ozzy integration | Go error/status and SQLite transaction docs | preserve `IndexStatusUnknown/Stale`; structured phase durations only at seam | shrink/reuse | high / medium | `lenny-raid-phase`, `norm-flash-ingest`, `ozzy-flash-integration` |
| 3 | benchmark duplication | Lenny skill desks; Ozzy ponytail/prune | Go benchmark docs, FTS5 docs | retain one fixture and warm/cold benchmark; delete duplicate harness | delete | high / low | `lenny-skill-architecture`, `lenny-skill-style`, `lenny-skill-modernize`, `ozzy-flash-ponytail`, `ozzy-flash-prune` |
| 3 | fault reproduction | Ozzy repro; Norm spy; all Lenny raids | Go race + SQLite testing docs | deterministic interleaving test plus immutable SHA receipts | add tests | medium / medium | `ozzy-flash-repro`, `norm-flash-ozzy-spy` |
| 2 | new abstraction/interfaces | Lenny interfaces/style | Go standard interfaces guidance | keep concrete functions until a second implementation exists | delete | high / low | `lenny-skill-interfaces`, `lenny-skill-style` |

### Adoption bands

**COPY/ADAPT NOW:** POSIX exclusive create + same-directory rename for catalog claims; one lock/transaction through refresh decision and unlink; `Cmd.Wait`/context for children; SQLite UPSERT/unique keys for idempotent writes; benchmark fixture simplification without deleting correctness assertions.

**STUDY FIRST:** catalog-vs-stem resolver unification; WAL checkpoint timing; coherent tag-prep/write snapshot; staged replacement/rollback. These touch stale/fresh and deletion semantics and need adversarial tests first.

**REJECT FOR RAWCLAW:** server queues, Redis/daemon locks, LLM-based resolution, cgo SQLite wrappers, or filesystem libraries that add runtime dependencies. They violate the single static pure-Go zero-dependency default or move authority outside the local cache/SQLite seams. Also reject “probe then unlink” as a fence and reject a generic interface solely to satisfy a style rule.

## Phase 5: dissemination packet (for supervisor review; not sent)

### Supervisor messages

- **Lenny Bruce:** Ten desks reduce to five high-value substitutions: exclusive regular-file claim; lock through cleanup; `Cmd.Wait`; one resolver; one closeout snapshot. Start with SQLite locking/atomic commit (https://www.sqlite.org/atomiccommit.html, https://www.sqlite.org/lockingv3.html) and POSIX `O_EXCL` (https://pubs.opengroup.org/onlinepubs/9699919799/functions/open.html). The precise experiment is a concurrent hook test plus a cleanup test that races acquisition between probe and unlink; only a lock held across both passes.
- **Norm Bell:** Your four quota-hit lanes should compare against SQLite’s idempotent uniqueness and explicit stale/error contract: https://www.sqlite.org/lang_upsert.html and https://go.dev/doc/articles/race_detector. Re-run catalog and ingest with exact/prefix collisions, source failure, and race count >=5; do not report quota-hit panes as green.
- **Ozzy Prince:** The cleanup receipt’s probe-to-unlink gap is confirmed locally at the mailbox path above. Use PocketBase’s checkpoint/backup sequencing (https://github.com/pocketbase/pocketbase/blob/master/core/base_backup.go) only as a pattern, then keep RawClaw’s fence held through `os.Remove`; reproduce issue-32 with the race detector and immutable integration SHA.

### Live-worker directives

- `lenny-raid-hooks` / `norm-flash-hooks` / `ozzy-flash-hook`: test POSIX exclusive create under two simultaneous invocations; metadata rename may fail without invalidating the claim. Compare against https://pkg.go.dev/os#OpenFile and https://pkg.go.dev/os#Rename.
- `lenny-raid-prewarm` / `norm-flash-ingest` / `ozzy-flash-hidden`: replace bespoke cache state only if an existing refresh DB or one atomic envelope carries the needed receipt; ensure every child is waited/reaped. See https://pkg.go.dev/os/exec#Cmd.Wait.
- `lenny-raid-locate` / `norm-flash-catalog` / `ozzy-flash-catalog`: build a table of exact ID, unique prefix, ambiguous prefix, source mismatch, and catalog miss; keep FTS5 out of identity resolution. See https://www.sqlite.org/fts5.html.
- `lenny-raid-fence` / `norm-flash-fence`: hold one SQLite transaction/guard across decision and mutation; a `BEGIN IMMEDIATE; ROLLBACK` probe followed by unlink is explicitly insufficient. See https://www.sqlite.org/lockingv3.html.
- `lenny-raid-containers` / `ozzy-flash-cleanup`: checkpoint/close, then remove DB/WAL/SHM under the same fence; missing sidecars are success and busy checkpoint is observable. See https://www.sqlite.org/pragma.html#pragma_wal_checkpoint.
- `lenny-raid-phase` / `norm-flash-ingest` / `ozzy-flash-integration`: preserve Unknown/Stale on any source/read failure and log phase duration at one seam; do not turn logs into a freshness claim. See https://go.dev/blog/errors-are-values and https://www.sqlite.org/lang_transaction.html.
- `lenny-skill-architecture` / `lenny-skill-interfaces` / `lenny-skill-modernize` / `lenny-skill-style`: apply the Ponytail ladder only where it deletes lines; leave a concrete helper when there is one implementation. See https://go.dev/doc/effective_go#interfaces.
- `ozzy-flash-ponytail` / `ozzy-flash-prune`: retain one realistic corpus and correctness assertions; split warm/cold and contention benchmarks rather than duplicating fixtures. See https://pkg.go.dev/testing#hdr-Benchmarks.
- `ozzy-flash-repro` / `norm-flash-ozzy-spy`: turn the reported interleaving into a deterministic test and attach exact SHA, command, and gate output. See https://go.dev/doc/articles/race_detector.

## Final accounting

- Product workers inventoried: **23** (10 Lenny, 5 Norm, 8 Ozzy).
- Distinct deduplicated problems: **10**.
- Prior-art sources cited: **25+** primary docs/repository files/issues/search results (links are listed inline; search record is explicit where no strong match existed).
- Top five highest-leverage recommendations:
  1. Replace hook FIFO/overwrite claims with POSIX exclusive regular-file creation plus same-directory atomic metadata rename.
  2. Hold the consolidated writer fence through refresh decision and DB/WAL/SHM unlink; never probe then release then delete.
  3. Reap background ingest/prewarm children with `Cmd.Wait`/context and persist idempotent state in an existing seam.
  4. Collapse catalog/stem/discovery into one source-aware resolver with explicit exact/prefix/ambiguous outcomes.
  5. Make tag-prep/write use a unique-keyed, coherent snapshot and keep stale/error status visible end-to-end.

Branch/push truth is intentionally recorded by the supervisor after this report commit; this worktree was created from `5b9756b` as `lenny/prior-art-map-20260826` and has no product-code edits.
