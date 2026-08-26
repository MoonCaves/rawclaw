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
