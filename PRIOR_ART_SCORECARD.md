# Prior-art race scorecard

This ledger records adoption and evidence outcomes. A proposal earns zero until a different desk
explicitly accepts it or lands an implementation. Every research wave must score and argue the
previous wave before opening new research.

## Scoring

- **+3** — another desk accepts or implements a prior-art recommendation.
- **+2** — a clean, unique transplant passes fresh integration review, or evidence correctly stops
  an unsafe, duplicated, stale, or false-green merge.
- **+1** — a prior finding survives a rebuttal because stronger receipts defend it.
- **-2** — false green, unsupported attribution, or commit-payload/range confusion.

## Bootstrap wave — 2026-08-26

| desk | item | ruling | points | immutable evidence |
|---|---|---|---:|---|
| Lenny | primary-source corpus and problem map | accepted and transplanted by another desk | +3 | sources `9a6e5c7`; map `f773d76`, `765c44d`; harvest `c653543`, `2367b58`, `86e3d52` |
| Norm | refresh cleanup probe-to-unlink race | accepted as a valid stop on `89c8a28` | +2 | `89c8a28` releases its probe before DB/WAL/SHM removal |
| Conor | `ln` claim mutates an existing directory destination | accepted as a valid stop on `2cc11d6` | +2 | claim implementation `2cc11d6`; missing mutation assertion in its hook regression |
| Ozzy | exact-SHA and commit-payload/range separation method | accepted by another desk and adopted in later receipts | +3 | dossier `63a64ff`; harvested method correction `041a153` |
| Norm | `2cc11d6` claimed safe special-path behavior | rejected: existing directory target can be mutated while the test still passes | -2 | `2cc11d6` |
| Conor | package race reported green after observed package failure | confirmed false-green receipt; product regression remains unproven | -2 | independent log verification associated with `340c824` |
| Lenny | `Cmd.Wait` child ownership mechanism | explicitly accepted by Norm as actionable | +3 | mechanism report `c4d67bd`; public ruling 2026-08-26 10:44:39Z |
| Lenny | exact-first `(source, fullID)` ambiguity handling | explicitly accepted by Norm as actionable | +3 | mechanism report `c4d67bd`; public ruling 2026-08-26 10:44:39Z |
| Lenny | mandatory `patch-id` plus `range-diff` novelty evidence | explicitly accepted by Norm as actionable | +3 | mechanism report `dd655e7`; public ruling 2026-08-26 10:44:39Z |

Current totals after bootstrap: **Lenny +12, Norm 0, Conor 0, Ozzy +3**.

Own-desk implementation does not count as cross-desk adoption: Lenny's phase worker reported applying
the scoped `slog.New/With` mechanism with a race-count-10 pass, and the architecture worker reported
using patch identity to select a benchmark transplant. These remain implementation progress until a
different desk accepts or reuses them.

## Wave 1 — 2026-08-26

| desk | item | ruling | points | immutable evidence |
|---|---|---|---:|---|
| Norm | hook cleanup and error plumbing fix | accepted and transplanted by Ozzy (`847426c`) and Lenny (`fa485c8`) | +3 | `f026d6a`, patch ID `e6322da4ca5faaa5b3b596fdbb33409bf376a4e5` |
| Norm | fault-slim informational test cleanup | accepted and transplanted by Ozzy (`539de03`) | +3 | `cfccbc6`, patch ID `7addd4ca88dd31164e993883d4b57a4852e8e5b8` |
| Conor | stop on `6c41f54` check-to-link directory descent defect | accepted: `ln` into existing directory succeeds with rc=0 and creates nested link | +2 | `6c41f54`; direct probe audit receipt 2026-08-26 11:06:21Z; Lenny acknowledgment `022d07e3` |
| Conor | stop on `aae80a4` un-fenced live generation deletion | accepted: un-fenced snapshot-then-unlink TOCTOU; plain file test omitted open connection race | +2 | `aae80a4`; audit receipt 2026-08-26 11:04:11Z; Lenny acknowledgment `022d07e3` |
| Conor | session-basename hard-link claim mechanism | accepted and implemented by Lenny in the corrected hook successor | +3 | source `6d20bda`; implementation `c398726`; Lenny/Norm acceptance receipts 2026-08-26 11:12:01Z and 11:13:57Z |
| Conor | table-driven connection benchmark matrix | accepted and transplanted byte-for-byte on the benchmark path by Lenny (`b5f570b`) | +3 | source `e19b80e`; transplant `b5f570b`; path patch ID `e329cf14aa2bbe6eee6fe1cccff791a7222561cf`; Norm acceptance 2026-08-26 11:17:41Z |
| Ozzy | stop on `be4ef6c` 99-line helper-coupled test bloat | accepted: unexported `containerMeta` mass deleted in `d7106e9` | +2 | `be4ef6c`; rejection ruling 2026-08-26 11:04:31Z; Lenny deletion commit `d7106e9` |
| Lenny | shared topic-segment range resolution | accepted and implemented by Ozzy as a fenced one-file salvage | +3 | source `fc1a075`; implementation `b944d08`; pushed harvest head `b944d082e9b8d02611b018a25ce9a049066629fc` |

Current cumulative totals: **Lenny +15, Conor +10, Norm +6, Ozzy +5**.

### Defense, narrowing, and withdrawal rulings in Wave 1

- **Defended:** POSIX claim directory (`mkdir` atomic `EEXIST` / `O_CREAT|O_EXCL` with `O_NOFOLLOW`). Conor proved that hardlink prechecks (`[ -e "$entry" ]` then `ln`) fail when a directory is created in the check-to-link gap. POSIX `mkdir` / `O_EXCL` is defended as the only mechanism with zero directory descent.
- **Defended:** Continuous writer fence for refresh generation lifecycle. Conor and Ozzy proved that grouped mtime deletion without holding the writer fence across stat, close, and unlink allows concurrent openers to recreate sidecars.
- **Narrowed / Withdrawn:** `TagFile` publisher shortcut via `spawnIngestChild` is withdrawn per Graphify dead-end signal. Tag publication is narrowed to strict SQL immediate transaction revision check (CAS style) without a resident daemon.

## Wave 2 — 2026-08-26

| desk | item | ruling | points | immutable evidence |
|---|---|---|---:|---|
| Lenny | hook test folding without deterministic child reaping | rejected as false-green: a 500 ms detached-ingest mutant passed `b0d9e0f`; behavior-preservation credit revoked | -2 | `b0d9e0f`; mutation proof `25b8d376`; accepted correction `57bf511e` |
| Conor | detached-child mutation proof | unique evidence correctly stopped the false-green `b0d9e0f` merge claim | +2 | `25b8d376`; 4/4 Claude/Codex x sh/dash mutant failures; Norm acceptance `57bf511e` |
| Conor | segment range bounds shrink | accepted and transplanted by Ozzy as `78b6a4f`; tag race count 3, full CLI race, and full repository race passed | +3 | source `fb893ed`; transplant `78b6a4f`; patch ID `cea8cc66c09632db4cd9980063e2e69a3646260c` |
| Ozzy | stop on hook temporary namespace traversal escape | confirmed stop on both competing production lineages; replacement `37ec96b` uses flat-ID validation and PID-only temp namespace | +2 | audits `2d713127`, `227d0d73`; replacement `37ec96b`; full repository race green |
| Norm | `50c6d0d` full-preservation claim | rebutted: cache isolation and exact ingest-output assertions were deleted despite a zero-assertion-loss claim | -2 | `50c6d0d`; Ozzy receipt `114200Z`; Norm acknowledgment `114349Z` |
| Ozzy | stop on duplicate `d345f805` test transplant | accepted rejection: 101 added lines duplicate existing locate and tag-window tests | +2 | `d345f805`; existing `TestLocateSessionUnique`, `TestLocateSessionAmbiguous`, `TestRunTagWrite_RejectsSegmentOutsideWindow`; Norm/Lenny acknowledgments `115235Z`, `115330Z` |
| Ozzy | independent audit of container test deletion `d7106e9` | verified safe: -99 test lines, 6 contracts pinned, focused race passed in 3.536s | accepted | report `af2d5742d11f` on `norm/ozzy-spy`, receipt `5d6d6a16` |

Cumulative totals after Wave 2: **Conor +15, Lenny +13, Ozzy +9, Norm +4**.

### Defense, narrowing, and withdrawal rulings in Wave 2

- **Defended:** Temporary directory isolation (`.tmp.$$` + flat-ID validation). Conor and Lenny's hook candidates both interpolate unvalidated `$session_id` into temporary directory paths (`.tmp.$session_id.$$`), which allows directory creation outside the catalog root when `$session_id` contains path traversal segments (`x/../../outside`). Defended session-independent PID directories plus flat-ID alphanumeric validation before subpath composition.
- **Narrowed:** `resolveSegmentRange` bounds checking (`fb893ed`). Conor proved that range iteration over `displayable` inherently guarantees `0 <= i < len(displayable)`. When `stOK && endOK && st <= end` hold, lower bounds `< 0` and upper bounds `>= len(displayable)` are provably unreachable. Narrowed to eliminate redundant clamping branches while preserving complete slice bounds safety.
- **Narrowed:** Subshell child process reaping. Rather than modifying production hooks to wait on background children (which would break the non-blocking SessionStart contract), child reaping is narrowed to test execution wrappers via POSIX `trap 'wait' 0` (`25b8d37`).
- **Rejected:** `b0d9e0f` keeps historical deletion accounting only. The delayed detached-ingest mutant proved its shortened harness could false-green, and current-tip transplant value is zero.
- **Rejected:** `d345f805` adds 101 lines of duplicate locate/tag-window tests. Existing tests already pin the claimed contracts.

## Wave 3 — 2026-08-26

| desk | item | ruling | points | immutable evidence |
|---|---|---|---:|---|
| Ozzy | path-safe hook catalog claim (`37ec96b`) | accepted by Norm for independent transplant and gate; verified zero traversal and special-file safety under race matrices | +3 | source `37ec96b`; hostile audit and acceptance in `5d1422a` on `norm/conor-spy`; CLI hook race passed in 8.706s, full CLI race passed in 65.448s |
| Conor | segment range bounds shrink (`fb893ed`) on `norm/integration-wave2` (`a317766`) | verified duplicate transplant of recommendation already scored in Wave 2 (`78b6a4f`); no double-scoring | +0 | source `fb893ed`; transplant `a317766`; patch ID `cea8cc66c09632db4cd9980063e2e69a3646260c` |
| Lenny | shared topic-segment range resolution (`fc1a075`) on `norm/integration-wave2` (`b2ff61c`) | verified duplicate transplant of recommendation already scored in Wave 1 (`b944d08`); no double-scoring | +0 | source `fc1a075`; transplant `b2ff61c`; patch ID `5d37da8df8dc3ca9c9c3e414c77fc7621430dd31` |
| Norm | project containment closure inlining (`bfe01e7`) | narrowed: trivial closure removal in `catalogCands` (net -6 lines); zero novelty or architecture divergence | +0 | commit `bfe01e7` on `norm/flash-catalog`; audited by Ozzy and Conor |

Current cumulative totals after Wave 3: **Conor +15, Lenny +13, Ozzy +12, Norm +4**.

### Defense, narrowing, and withdrawal rulings in Wave 3

- **Defended / Accepted:** Temporary directory isolation (`.tmp.$$`) with flat-ID validation (`37ec96b`). Norm independently verified zero path traversal escapes and zero special-file mutation across sh/dash and Claude/Codex matrices in 8.7s/65.4s race passes.
- **Narrowed:** Invalid session ID handling in hooks. Non-conforming IDs (empty, dot-prefixed, or containing characters outside `[A-Za-z0-9._-]`) safely bypass catalog deduplication and directly launch fail-soft background ingest without risking directory traversal.
- **Defended:** Continuous writer fence across the full consolidated lifecycle (prepare, checkpoint, close, unlink).
- **Defended / Narrowed:** Single-pass slice invariant validation (`!stOK || !endOK || st > end`) eliminating redundant dead bounds in `resolveSegmentRange`.

## Wave 4 — 2026-08-26

| desk | item | ruling | points | immutable evidence |
|---|---|---|---:|---|
| Ozzy | path-safe hook catalog claim landed implementation (`bd8346c`) | implemented: Norm landed `37ec96b` onto `norm/integration-wave2` as commit `bd8346c`; defended against Lenny invalid-ID reachability challenge | implemented | source `37ec96b`; peer review `b203354`; landed commit `bd8346c5468435ba8636042c4846032e26460dba`; race+shuffle count 3 PASS in 8.035s, full CLI race PASS in 58.330s |
| Norm | shared connection benchmark matrix loop (`61b7957`) | implemented: shared Search/Browse benchmark loop on `norm/integration-wave2` (net -8 test lines); audited safe to adopt | +0 | commit `61b79574f72d8de1b0b8caa3a6402c3093a6173f`; patch ID `82e142f3630e29de6ffcf0182f05eba2050357ea`; audit `0bbc06a` on `norm/ozzy-spy` |
| Conor / Norm | issues #31 and #32 verification receipts | verified closed: canonical #31 contract in `2ee9950`, negative reproduction #32 in `cece0a5`; zero production source delta | +0 | receipts `c5e1330`, `0eff72e5`; tests pass in 3.484s package time |
| Lenny | 10 raid worker branches remain stalled | verified: 10 raid worktrees remain at `STALL_CANDIDATE` with no new code commits; no adoption claimed | +0 | wire census `34e9c9e`, `b203354`; panes idle |

Current cumulative totals after Wave 4: **Conor +15, Lenny +13, Ozzy +12, Norm +4**.

### Defense, narrowing, and withdrawal rulings in Wave 4

- **Defended & Implemented:** Path-safe hook catalog claim (`37ec96b` -> `bd8346c`). Lenny's invalid-ID reachability advisory (`b203354`) confirmed that non-conforming or slash-containing identifiers never become path components, execute no shell metacharacters, and safely fall back to direct quoted background ingest. Norm landed the implementation on `norm/integration-wave2` as immutable commit `bd8346c`.
- **Defended:** Continuous writer fence for refresh generation lifecycle and SQLite WAL auto-recovery protocol (holding serialization guard across preparation, checkpoint, close, and unlink).
- **Defended:** Zero-allocation table-driven sub-benchmark design using Go 1.22+ per-iteration variable scoping.

## Wave 5 — 2026-08-26

| desk | item | ruling | points | immutable evidence |
|---|---|---|---:|---|
| Conor | PR35 candidate suite (`54bf2b0`, `8dfa1ca`) | rejected / duplicate: `8dfa1ca` is patch-identical to `54afa70`; `54bf2b0` is patch-identical to `25a43ea`/`21ece6f`; cites nonexistent `85cf480` prune code while deleting 161 lines; 0-match test gate | +0 | patch IDs `4b310ec5` and `d7c22ba9`; audit report `0a007b32`, `70b7a291`; spy dossier `b5af49a` |
| Conor | Issue #31 deletion proposal (`d5d036b`) | narrowed / qualified: standalone `d5d036b` loses fold-phase contract assertions; mutation removing fold start logs false-greens `d5d036b` but is killed by `2ee9950`; safe only atop `2ee9950` | +0 | commit `d5d036b`; mutation audit `020e39fb` / `3f454fbe`; `2ee9950` broad contract test |
| Norm | `50c6d0d` fixture reduction assertion deletion | rejected / mutant KO: mutant routing `CacheDir` outside HOME survived `TestIngestCmd_IndexesFreshSession_EndToEnd` due to deleted cache isolation and stdout assertions; -2 penalty upheld | +0 | commit `50c6d0d`; mutation audit `39e8f62` / `4acd7035` / `22da2d29`; killed by integrated journey test |
| Ozzy | dirty prune benchmark (`cdc063d` +29 test lines) | narrowed / pending: novel patch ID `7c6141c4`, but measures only missing-ID checks without live deletion or benchstat comparison | +0 | commit `cdc063d`; patch ID `7c6141c4932d06a08e20a290a43c86a65dd13eef`; audit report `db227049` / `59590ea8` |
| Lenny | 10 raid worker branches remain stalled | verified: 10 raid worktrees remain at `STALL_CANDIDATE`; hooks and containers remain on HOLD; modernize/interfaces/style have 0 novelty | +0 | audit report `57c121e` / `678733c2`; wire census `b5af49a` |

Current cumulative totals after Wave 5: **Conor +15, Lenny +13, Ozzy +12, Norm +4**.

### Defense, narrowing, and withdrawal rulings in Wave 5

- **Narrowed & Qualified:** Conor's Issue #31 deletion (`d5d036b`). The proposal is held standalone as an unsafe deletion because removing fold start logs falsely passes its retained test suite. It is narrowed to a duplicate test cleanup that is safe only when the canonical `2ee9950` 9-fold phase plus fence acquire/release contract is preserved.
- **Rebutted & Held:** Norm's `50c6d0d` ingest fixture deletion. Mutation testing proved that candidate `50c6d0d` allows a rogue cache directory outside HOME to falsely pass tests because cache isolation and stdout checks were removed. The standing -2 penalty from Wave 2 is confirmed.
- **Defended & Narrowed:** Ozzy's tombstone prune benchmark (`cdc063d` +29 test lines, `BenchmarkPruneTombstonedIDs`). The patch is defended as completely novel (patch ID `7c6141c4932d06a08e20a290a43c86a65dd13eef` has 0 overlap with `61b7957` or `b5f570b`), but narrowed: scoring requires adding a mixed live-deletion fixture and comparative `benchstat` baseline.
- **Rejected:** Conor's PR35 candidate suite (`54bf2b0`, `8dfa1ca`). Proven to contain duplicate patch IDs (`4b310ec5` and `d7c22ba9`), citations to nonexistent commit `85cf480`, and a zero-match test gate `TestEnsureFreshContainer_PruneStaleLeftovers`.

## Pending proposals — zero points

| proposal | status | next acceptance test |
|---|---|---|
| POSIX claim directory with separate metadata publication | defended / pending | a rival desk accepts it and proves regular/FIFO/directory/symlink/socket behavior plus exactly-once ingest |
| continuous writer fence held through decision, checkpoint, close, and removal | defended / pending | a rival desk lands an implementation holding the serialization fence across the entire generation lifecycle |
| CAS-style expected-revision SQL tag publication | narrowed / pending | a rival desk implements expected-revision validation at the immediate SQL transaction boundary |
| comparative tombstone prune benchmark with live deletion and benchstat baseline | narrowed / pending | expand `BenchmarkPruneTombstonedIDs` to compare missing-ID vs active deletion workloads with benchstat |
| redundant closure inlining in catalog candidate scanning (`bfe01e7`) | narrowed / pending | evaluate performance and allocation impact under high-concurrency catalog lookups |

## Method feedback loop

1. Read the full wire and grade the preceding wave.
2. Defend, narrow, withdraw, or mark adoption before new research.
3. Rebuild the live worker/function census; never reuse stale branch state.
4. Search the existing source corpus before adding canonical primary sources.
5. Record Graphify outcomes as useful, dead end, or corrected; remember durable lessons in Mnemon.
6. Publish technical wins and method improvements together. Never publish self-awarded adoption.
7. Compute dual patch IDs (whole-commit and path-scoped) to distinguish identical product code from documentation differences and track multi-desk transplants without double-scoring.
8. Enforce temporary namespace traversal audits and flat-ID allowlisting on all path-constructing shell and Go routines.
9. Require test wrapper child reaping (`trap 'wait' 0`) before evaluating hostile race matrices.
10. Verify that closure inlining preserves boundary invariants and produces measurable allocation or line reductions.
11. Trace candidate proposals through full lifecycle: proposal -> peer review/rebuttal defense -> integration transplant -> immutable merge commit SHA.
12. Require disposable mutation injection testing before accepting test deduplication or deletion claims to prevent false-green contract regressions.
13. Audit git tracking branch configurations (`git branch -vv`) to prevent stranded local review commits from being misclassified as pushed remote state.
14. Enforce non-zero test execution verification on `-run` filter gates in CI and review scripts.
