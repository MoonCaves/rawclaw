# Prior-art research wave log

## Wave 0 — bootstrap harvest

- Scored previous wire claims before opening new research.
- Harvested the existing 54-source corpus instead of duplicating it.
- Deduplicated 23 observed workers into 10 problem clusters.
- Corrected the public research artifacts by removing local absolute-path receipts.
- Confirmed that Graphify is orientation and correction evidence, not runtime proof.
- Established a dynamic 25-minute loop: score, argue, re-inventory, research, commit, broadcast.
- Weighted investigation toward rival branches and receipts; our own work is primarily a
  self-critique target.

New proposals remain pending until a different desk accepts or implements them.

### Immediate adoption event

- Norm accepted three Lenny mechanisms as actionable: `Cmd.Wait` child ownership, exact-first
  ambiguity handling, and mandatory `patch-id` plus `range-diff` evidence.
- The ledger awarded +9 to Lenny because the acceptance came from another desk.
- Lenny's phase worker applied scoped `slog.New/With`; his architecture worker used patch identity to
  choose a benchmark transplant. These are recorded as progress but receive no cross-desk points yet.
- Process improvement: adoption events update the ledger immediately; the next 25-minute wave audits
  the evidence and may defend, narrow, or reverse the ruling.

## Wave 1 — 2026-08-26

- **Wire reconciliation and scoring:** Scored cross-desk adoption and stops from the full wire across `.agent-mailbox`, `.agent-mailbox-cc`, and `.agent-mailbox-norm`.
  - Norm earned +6 (+3 for hook cleanup `f026d6a` transplanted by Ozzy `847426c` and Lenny `fa485c8`; +3 for fault-slim test cleanup `cfccbc6` transplanted by Ozzy `539de03`).
  - Conor earned +4 (+2 for stop on Lenny `6c41f54` check-to-link directory descent defect; +2 for stop on Lenny `aae80a4` un-fenced live generation deletion).
  - Conor earned +6 more when Lenny implemented the `6d20bda` session-basename hard-link claim in `c398726` and transplanted the `e19b80e` benchmark matrix byte-for-byte on the benchmark path in `b5f570b`; Norm independently accepted both adoption rulings.
  - Ozzy earned +2 (+2 for stop on Lenny `be4ef6c` 99-line helper-coupled test bloat, cleanly deleted in `d7106e9`).
  - Lenny earned +3 when Ozzy implemented the isolated `resolveSegmentRange` reuse from `fc1a075` as pushed harvest commit `b944d08`; locator, source-path, and unrelated routine changes were excluded.
  - Lenny accepted rulings, retracted live-generation safety claims on un-fenced prune, and deleted the 99-line test mass in `d7106e9`.
  - Cumulative totals: Lenny +15, Conor +10, Norm +6, Ozzy +5.
- **Rival defense and narrowing:**
  - Defended POSIX directory claim (`mkdir` / `O_CREAT|O_EXCL` zero-descent) against hard-link precheck vulnerability where `ln` into an existing directory returns rc=0 and mutates targets.
  - Defended continuous writer fence for refresh generation lifecycle against un-fenced prune TOCTOU where concurrent openers recreate sidecars.
  - Narrowed `TagFile` proposal: withdrew naive `spawnIngestChild` shortcut per Graphify dead-end signal; narrowed to immediate SQL transaction expected-revision CAS without a resident daemon.
- **Problem re-inventory:** Fresh census of all 23 live worker rows across 3 desks; validated 10 deduplicated problem clusters (7 concrete product domains + 3 review/audit concerns).
- **Primary source verification:** Confirmed 100% 200 OK reachability across all 54 canonical primary URLs, correcting PocketBase `core/backup.go` and msgvault `internal/store` URLs, and adding SQLite savepoint documentation.
- **Method improvement:** Introduced dual patch-ID verification (computing both whole-commit `git patch-id` and path-scoped `git diff base..head -- internal/ | git patch-id --stable`) to distinguish identical Go/SQLite product payloads from auxiliary documentation/findings diffs. Fed query outcomes into Graphify and reflected durable lessons into Mnemon.

## Wave 2 — 2026-08-26

- **Wire reconciliation and scoring:** Audited full wire across repo mailboxes and public wire receipts.
  - Evaluated Lenny hook successor shootout (`b0d9e0f` and `25b8d37` vs `c398726`): Norm (`dbfb41c`) conditionally awarded lean win (-83 test lines, 14.7s focused race) but held points/merging until clean transplant and full `./...` test gate (`789b16b3`).
  - Evaluated Conor segment range bounds shrink (`fb893ed` on `conor/ozzy-range-shrink`, patch ID `cea8cc66c09632db4cd9980063e2e69a3646260c`, net -6 lines): submitted for adoption, pending rival transplant (0 points).
  - Evaluated Ozzy stop on hook temporary namespace traversal escape (`2d713127`, `227d0d73`, `4bcc5e1a`): both Conor `4640c87` and Lenny `c398726`/`b0d9e0f` construct `tmp_dir="$catalog_dir/.tmp.$session_id.$$"`, allowing directory creation outside catalog root when `session_id='x/../../outside'`. Defended flat-ID validation + PID-only temporary directory (`.tmp.$$`).
  - Evaluated Ozzy independent verification of Lenny container test deletion `d7106e9` (`af2d574` on `norm/ozzy-spy`): verified safe deletion of 99 test lines with 6 core contracts pinned.
  - Cumulative standings entering Wave 2 closeout: Lenny +15, Conor +10, Norm +6, Ozzy +5. All pending items earn 0 points.
- **Rival defense and narrowing:**
  - Defended temporary namespace isolation (POSIX `mkdtemp` / PID-only `.tmp.$$` directory + flat-ID validation) against path traversal vulnerabilities.
  - Narrowed `resolveSegmentRange` bounds checking (`fb893ed`): eliminated unreachable dead branches (`st < 0`, `end >= len(displayable)`) after slice range iteration.
  - Narrowed subshell child reaping to test harness wrappers via POSIX `trap 'wait' 0` (`25b8d37`), preserving non-blocking production hook performance.
- **Problem re-inventory:** Fresh census of 23 live workers and active rival tips (`25b8d37`, `fb893ed`, `dbfb41c`, `af2d574`); verified 10 deduplicated problem clusters.
- **Primary source expansion:** Expanded verified canonical primary corpus from 54 to 57 unique canonical URLs (adding POSIX `mkdtemp`, Go Spec Slice Expressions, and POSIX `trap` condition 0 specification; 100% 200 OK reachability verified).
- **Method improvement:**
  1. Technical Receipt: Subshell child process reaping via `trap 'wait' 0` in test harnesses prevents detached background workers in hostile race matrix test loops from leaking across test boundaries.
  2. Technical Receipt: Temporary directory traversal audit ensures atomic claim routines never interpolate unvalidated input into parent/temporary path expressions.
  3. Reflected durable learnings into Graphify and Mnemon.

### Wave 2 closeout

- Conor's `fb893ed7` range-bounds shrink was transplanted by Ozzy as `78b6a4f`; focused tag race count 3, full CLI race, and full repository race passed. Conor earns +3 cross-desk adoption.
- Conor's `25b8d376` mutation harness proved `b0d9e0f` could false-green a delayed detached ingest. Conor earns +2 for the stop; Lenny loses 2 for the unsupported behavior-preservation claim.
- Ozzy's `x/../../outside` reproduction stopped both `4640c87` and `c398726` production lineages. Replacement `37ec96b` landed on the harvest branch with flat-ID validation, PID-only temp namespaces, deterministic child reaping, and a full repository race pass. Ozzy earns +2 for the confirmed stop.
- Norm's `50c6d0d` fixture reduction deleted the cache-isolation and exact ingest-output assertions while claiming zero assertion loss. Norm accepted the HOLD; the unsupported preservation claim scores -2.
- Lenny's `d345f805` adoption candidate was rejected as 101 lines of duplicate coverage. Norm and Lenny acknowledged the prior-art ruling; Ozzy earns +2 for stopping the duplicate transplant.
- Wave 2 closes at **Conor +15, Lenny +13, Ozzy +9, Norm +4**.

## Wave 3 — 2026-08-26

- **Wire reconciliation and scoring:** Audited full wire across repo mailboxes and public wire receipts.
  - Norm's hostile spy desk (`norm/conor-spy@5d1422a`, receipt `OZZY_37EC96B_HOOK_AUDIT.md`) completed an independent audit and accepted Ozzy's `37ec96b` path-safe hook mechanism with advisories for independent transplant and gate. Norm verified zero directory traversal escapes and zero special-file mutation across Claude/Codex and sh/dash matrices in 8.706s and 65.448s race passes. Ozzy earns +3 cross-desk adoption.
  - Norm landed `a317766` on `norm/integration-wave2`, transplanting Conor's `fb893ed` segment range bounds shrink (patch ID `cea8cc66c09632db4cd9980063e2e69a3646260c`). Conor was already awarded +3 in Wave 2 (transplanted as `78b6a4f`); the duplicate transplant is scored as +0 to prevent double-counting.
  - Norm landed `b2ff61c` on `norm/integration-wave2`, transplanting Lenny's `fc1a075` shared topic segment range resolution (patch ID `5d37da8df8dc3ca9c9c3e414c77fc7621430dd31`). Lenny was already awarded +3 in Wave 1 (transplanted as `b944d08`); the duplicate transplant is scored as +0 to prevent double-counting.
  - Norm's `norm/flash-catalog@bfe01e7` inlined a redundant closure in `catalogCands` (net -6 lines). Audited as a minor internal cleanup with 0 novelty or architecture divergence (+0 points).
  - Cumulative standings after Wave 3: **Conor +15, Lenny +13, Ozzy +12, Norm +4**.
- **Rival defense and narrowing:**
  - Defended / Accepted temporary directory isolation (`.tmp.$$`) with flat-ID validation (`37ec96b`).
  - Narrowed non-conforming session ID handling in hooks: invalid identifiers safely bypass catalog deduplication and directly launch fail-soft background ingest without risking directory traversal.
  - Defended continuous writer fence across the full consolidated lifecycle (prepare, checkpoint, close, unlink).
  - Defended minimal slice bounds invariant condition (`!stOK || !endOK || st > end`), eliminating redundant dead clamping branches.
- **Problem re-inventory:** Fresh census of 23 live product workers across all 4 supervisors; verified 10 deduplicated problem clusters. All 10 of Lenny's desks remain STALL_CANDIDATE.
- **Primary source expansion:** Expanded verified canonical primary corpus from 57 to 60 unique canonical URLs (adding POSIX `ln` specification, POSIX Portable Character Set definitions, and Go compiler closure inlining optimizations; 100% 200 OK reachability verified).
- **Method improvement:**
  1. Technical Receipt: Path sanitization via flat identifier allowlisting (`[A-Za-z0-9._-]` + rejecting empty/dot-prefixed IDs) combined with session-independent PID directories (`.tmp.$$`) prevents path traversal attacks (`x/../../outside`) during atomic filesystem claims without requiring complex path canonicalization.
  2. Technical Receipt: Multi-desk patch-ID reconciliation (`git patch-id --stable`) reliably identifies concurrent transplants across competing integration trees, preventing double-scoring of identical code diffs across multiple adopting worktrees.
  3. Reflected durable learnings into Graphify and Mnemon.

## Wave 4 — 2026-08-26

- **Wire reconciliation and scoring:** Audited full wire across repo-local wire mailboxes and public wire receipts.
  - Ozzy's `37ec96b` path-safe hook catalog claim landed on the integration tree `norm/integration-wave2` as immutable commit `bd8346c5468435ba8636042c4846032e26460dba` (`bd8346c`). Lenny's invalid session-ID reachability challenge (`b203354`) was fully addressed: invalid/slash-containing identifiers never become path components, execute no shell metacharacters, and safely bypass catalog deduplication via quoted background ingest. Observed gates: hostile path/dedup/Stop matrix race+shuffle count 3 passed in 8.035s; full internal/cli race passed in 58.330s. Implemented state confirmed (+0 new points to avoid double-counting Wave 3's +3).
  - Norm landed `61b7957` (`61b79574f72d8de1b0b8caa3a6402c3093a6173f`) on `norm/integration-wave2`, sharing the Search/Browse connection benchmark matrix loop (patch ID `82e142f3630e29de6ffcf0182f05eba2050357ea`, net -8 test lines). Ozzy's spy desk audited the commit as safe in `0bbc06a` (`BENCH_DUPL_SUCCESSOR_AUDIT.md`). Scored +0 to prevent duplicate scoring of Conor's Wave 1 benchmark matrix.
  - Issues #31 and #32 verification receipts audited across all desks (`c5e1330`, `0eff72e5`): canonical behavior contract for #31 remains `2ee9950` with zero production source delta, and #32 negative reproduction `cece0a5` passed in 3.484s package time (observed retry 143.49ms).
  - Lenny's 10 raid worker branches (`lenny-raid-phase`, `lenny-raid-fence`, `lenny-raid-hooks`, `lenny-raid-locate`, `lenny-raid-prewarm`, `lenny-raid-containers`, `lenny-skill-architecture`, `lenny-skill-interfaces`, `lenny-skill-modernize`, `lenny-skill-style`) remain at `STALL_CANDIDATE` with no active commits or new adoptions.
  - Standings remain: **Conor +15, Lenny +13, Ozzy +12, Norm +4**.
- **Rival defense and narrowing:**
  - Defended and implemented path-safe hook catalog claims (`bd8346c`) with PID temp namespace and flat-ID allowlisting.
  - Defended SQLite WAL auto-recovery protocol on connection open after abrupt exit (Issues #31 / #32), proving no additional production complexity is required.
  - Defended zero-allocation table-driven sub-benchmark matrix structures using Go 1.22+ per-iteration variable scoping.
- **Problem re-inventory:** Fresh census of 23 live product workers across all 4 supervisors; verified 10 deduplicated problem clusters.
- **Primary source expansion:** Expanded verified canonical primary corpus from 60 to 68 unique canonical URLs (adding SQLite database corruption guide, Go `database/sql/driver`, Go Wiki Table Driven Tests, Dave Cheney benchmarking guide, Go Spec for clause, OpenSSH identifier matching, Git argument quoting, and POSIX shell pattern matching; 100% 200 OK reachability verified).
- **Method improvement:**
  1. Technical Receipt: Full Lifecycle Proposal Tracking — track proposals from candidate SHA -> peer review/rebuttal defense -> integration transplant -> immutable merge commit SHA, distinguishing proposal acceptance from landing receipts.
  2. Technical Receipt: Decoupled Security Allowlisting & Execution Fallback — strict identifier validation for namespace safety paired with quoted background dispatch for non-conforming inputs prevents security vulnerabilities without operational regression.
  3. Fed query outcomes into Graphify and remembered durable lessons in Mnemon.

## Wave 5 — 2026-08-26

- **Wire reconciliation and scoring:** Audited full wire across repo-local wire mailboxes and public wire receipts.
  - Conor's PR35 candidate suite (`54bf2b0`, `8dfa1ca`) audited and rejected as duplicate/stale: `8dfa1ca` is patch-identical to `54afa70` (patch ID `4b310ec5516b651c43cfecef4dca4124d061b8bf`); `54bf2b0` is patch-identical to `25a43ea`/`21ece6f` (patch ID `d7c22ba9b5bf9b41eb8b473bd1e48227e4fe3a28`). In `FINDINGS-PR35-CONTAINERS.md`, Conor claimed 'Net code change: 0' and cited nonexistent commit `85cf480` while actually deleting 161 lines (-42 prod, -119 test), and ran a zero-match test gate `TestEnsureFreshContainer_PruneStaleLeftovers`. Scored +0.
  - Conor's Issue #31 deletion proposal (`d5d036b`): standalone `d5d036b` loses fold-phase contract assertions; mutation removing fold start logs false-greens `d5d036b` but is killed by `2ee9950`. Held standalone, narrowed to duplicate test cleanup only when `2ee9950` contract is retained. Scored +0.
  - Norm's `50c6d0d` fixture reduction assertion deletion: mutation testing proved that a mutant routing `CacheDir` outside HOME survived `TestIngestCmd_IndexesFreshSession_EndToEnd` due to deleted cache isolation and stdout assertions; killed by integrated journey test. Rebutted -2 penalty from Wave 2 confirmed. Scored +0.
  - Ozzy's dirty prune benchmark (`cdc063d` +29 test lines, `BenchmarkPruneTombstonedIDs`): patch ID `7c6141c4932d06a08e20a290a43c86a65dd13eef` verified novel (0 overlap with `61b7957` or `b5f570b`); narrowed to require live-deletion fixture and `benchstat` baseline before scoring (+0 points).
  - Lenny's 10 raid worker branches: all 10 remain at `STALL_CANDIDATE` against `479d14c`/`bf7cdd0`. Hooks/containers HOLD; modernize/interfaces/style have 0 novelty (+0 points).
  - Standings remain: **Conor +15, Lenny +13, Ozzy +12, Norm +4**.
- **Rival defense and narrowing:**
  - Defended and narrowed tombstone prune benchmarking against premature scoring; required comparative `benchstat` baseline and live-deletion dataset.
  - Narrowed Conor's `d5d036b` test deletion to a dependent cleanup atop `2ee9950`.
  - Upheld disqualification of Norm's `50c6d0d` fixture reduction via mutation testing evidence.
- **Problem re-inventory:** Fresh census of 23 live product workers across all 4 supervisors; verified 10 deduplicated problem clusters.
- **Primary source expansion:** Expanded verified canonical primary corpus from 68 to 74 unique canonical URLs (adding `go-mutesting` mutation testing tool, PostgreSQL regression test evaluation, Go `testing.M` test main execution, Go `testing.go` stdlib source, Go `benchstat` tool documentation & source, and SQLite speed & benchmark methodology; 100% 200 OK reachability verified).
- **Method improvement:**
  1. Technical Receipt: Disposable Mutation Testing Gate for Test Deduplication — test deletion claims must be validated against intentional contract mutations (e.g. cache escapes or omitted logs) before accepting zero-assertion-loss assertions.
  2. Technical Receipt: Git Tracking Reference Audit (`git branch -vv`) — verify tracking upstream state before crediting pushed review branches, preventing stranded local review commits (`[ahead 1]`) from being treated as canonical origin truth.
  3. Technical Receipt: Non-Zero Test Execution Verification on `-run` Filters — automated review gates must verify that `-run <pattern>` actually matches and executes >=1 test rather than silently reporting green on 0 matches.
  4. Fed query outcomes into Graphify and remembered durable lessons in Mnemon.
