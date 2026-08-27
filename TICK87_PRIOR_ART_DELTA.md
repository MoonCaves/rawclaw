# Tick 87 prior-art delta: PR #43 refresh-cache eviction

## Run and predecessor verification

- `run_timestamp_utc`: `2026-08-27T16:35:21Z`, captured with `date -u`.
- `report_base`: `029f60d77e7e03192bc966de3a835a4a32a00fe2`.
- `current_public_main_sha`: `029f60d77e7e03192bc966de3a835a4a32a00fe2`; worker `origin/main` matches.
- `canonical_ledger_path`: `/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/PRIOR_ART_LOG.md`.
- `canonical_ledger_sha256`: `848279d55b70579e6c1b029655d2adc475bc6ca24a103295f0d4972073ac7d72`.
- `previous_report_path_handoff`: `/Users/jay-m4/code/rawclaw-furiosa-t57-prior-art/TICK57_PRIOR_ART_DELTA.md` was absent.
- `previous_report_path_verified`: `/Users/jay-m4/code/rawclaw-furiosa-t57-prior-art-delta/TICK57_PRIOR_ART_DELTA.md` exists and has SHA-256 `dd1f0d4aa2a10bd22123fb6e847b59c887247b73710f02701f3352726847b974`; its worktree HEAD is the supplied `98ae85385a328f58640ec262e7c189c1d821083d`.
- `predecessor_verdict`: the handoff path contains a directory-name typo, but the matching report hash and commit verify the intended predecessor. No ledger bytes were changed.
- `prior_watermark`: `20260827T032519Z`; later mailbox control messages were not score-eligible receipt evidence, so the watermark does not advance.

## Live problem census

- PR #43 is open, unmerged, and current-base-equivalent: head `8d2cb52047ea00d4b123ea747fa5d035d3deff4c`, base `main`, changed files `internal/index/containers.go` and `internal/index/containers_test.go`.
- The PR probes a stale database with `BEGIN IMMEDIATE`, rolls back and closes, then unlinks the database and `-wal`/`-shm` sidecars. A writer can enter between close and unlink and commit data that eviction immediately discards.
- The PR also deletes on every non-busy `BEGIN IMMEDIATE` error. `SQLITE_NOTADB`, I/O, and other probe failures are uncertainty, not proof that deletion is safe.
- Sidecars are one SQLite generation and must be evaluated and removed together. Grouping alone does not close the probe-release window.
- Current source call evidence: `RefreshDBPath()` calls `CacheDir()` and `realpath()`; `EnsureFreshContainer()` calls `PrepareFreshContainer()` and `SyncConsolidatedFrom()`; `EnsureIndexedContainers()` calls `updateContainers()`; `updateContainers()` calls `backingFileState()`, `reindexContainer()`, and retention. These paths are adjacent but not a substitute for eviction proof.
- Graphify MCP impact for PR #43: 37 nodes across communities 4, 21, 34, and 89. The call traversal connected `RefreshDBPath()`, `CacheDir()`, `EnsureFreshContainer()`, `updateContainers()`, `ReconcileRetention()`, and `Remove()`; `IsBusy()` resolves through `isBusy()` and `runTagPrepCmdWithSources()`.

## Exact mechanisms inspected: maximum three

### 1. Hold the SQLite write transaction through eviction

- `source`: `https://github.com/MoonCaves/rawclaw/commit/3de6a4acee90bda84e3a978e13929f1c32f6f60a` — `fix(index): close PR43 refresh-cache TOCTOU`, committed `2026-08-27T16:24:58Z`, inspected `2026-08-27T16:35:21Z`.
- `official corroboration`: `https://www.sqlite.org/lang_transaction.html` — SQLite Transactions, Last-Modified `2026-08-27T09:51:16Z`; `BEGIN IMMEDIATE` starts a write transaction and SQLite permits only one simultaneous writer.
- `mechanism`: open the candidate SQLite database with a zero busy timeout; on successful `BEGIN IMMEDIATE`, unlink the grouped database sidecars while that transaction remains open, then roll back and close. A busy/locked or any other probe error retains the cache.
- `relevance`: directly closes PR #43's release-to-unlink interleaving and makes probe uncertainty fail closed. The current descendant adds a deterministic WAL-write regression and an unreadable-cache regression.
- `stable_recommendation_id`: `PA-SQLITE-EVICTION-HOLD-LOCK-THROUGH-UNLINK-001`.
- `normalized_text_sha256`: `6cf5068c8089145a6dfabc2e6399425910b137a469ba655402c19d52df8356c7`.
- `adoption_status`: `candidate-unadopted`; `3de6a4a` is not on public `main`, and PR #43 remains open. Score `0`.

### 2. Fail closed on every inconclusive probe

- `source`: same immutable commit `3de6a4acee90bda84e3a978e13929f1c32f6f60a`, inspected `2026-08-27T16:35:21Z`; the exact behavior is pinned by `TestEvictStaleRefreshDB_RetainsUnreadableCache`.
- `mechanism`: do not classify only `SQLITE_BUSY` as unsafe. `BEGIN IMMEDIATE` failure of any kind leaves the cache in place because malformed content, permissions, I/O, and driver errors do not establish eviction safety.
- `relevance`: fixes the second independent PR #43 deletion-eligibility defect with the smallest possible policy rule. It is complementary to, not a replacement for, the held transaction.
- `stable_recommendation_id`: `PA-SQLITE-EVICTION-FAIL-CLOSED-001`.
- `normalized_text_sha256`: `4bf91e16989440a508f0344c241d95e474b420489a53120845a81f32daee20af`.
- `adoption_status`: `candidate-unadopted`; branch/report evidence only, score `0`.

### 3. Atomic quarantine rename before asynchronous unlink

- `source`: `https://man7.org/linux/man-pages/man2/rename.2.html` — `rename(2)`, Last-Modified `2026-08-25T07:55:25Z`, inspected `2026-08-27T16:34:39Z`.
- `mechanism`: POSIX `rename()` changes a directory entry atomically, and existing open file descriptors to the old path remain valid. A cache generation can be renamed into a private quarantine name before later unlink, so a new opener cannot reacquire the old pathname during deletion.
- `relevance`: useful as a future design comparator if the refresh cache becomes a directory-per-generation format. It is not a direct safe transplant today: renaming `.db`, `-wal`, and `-shm` separately is not one atomic multi-file operation, and the current flat naming scheme has no generation directory.
- `stable_recommendation_id`: `PA-POSIX-RENAME-CACHE-QUARANTINE-001`.
- `normalized_text_sha256`: `75d54fe80fd08ad13b0d12fceb2ee7b1306b1aa014ed41fe6b0c6f2d3ea60869`.
- `adoption_status`: `comparator-only`; no score and no merge recommendation for the current PR #43 shape.

## Candidate deduplication and scoring

- `764eb0d80db23e3c32e346d79184b42d306c27eb` and public PR head `8d2cb52047ea00d4b123ea747fa5d035d3deff4c` are the same stale-pruning mechanism; the latter is the current PR head.
- `e2f1d3e4599d1dd6b92d6caffae79ad1648e49c4` is the one-line implementation of mechanism 1 and is subsumed by `3de6a4a`; do not score or mint a second recommendation ID.
- `89c8a284d20e4f6adba72accb3c0b34831a3b422` groups DB/WAL/SHM and probes activity, but retains the release-to-remove gap and is superseded by the held-lock candidate.
- `aae80a41882610ae47bcbdb6bc7c720ecc32c718` groups sidecars without an active SQLite fence; duplicate/incomplete for this defect.
- `21ece6f`, `25a43ea`, and `54bf2b0` remove unsafe pruning rather than establish deletion eligibility; they are safety withdrawals, not adopted mechanisms.
- No candidate is on `origin/main`; no external or public-main adoption event was found. Score delta `0`; prior totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`.
- `direction_lock`: `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically `LOCKED`; this report grants `NO MERGE AUTHORIZATION`.

## Proposed supervisor delta

- `new_watermark`: `20260827T032519Z`.
- `recommendations`: hold `BEGIN IMMEDIATE` through grouped sidecar unlink as `PA-SQLITE-EVICTION-HOLD-LOCK-THROUGH-UNLINK-001`; fail closed on every probe error as `PA-SQLITE-EVICTION-FAIL-CLOSED-001`; retain rename quarantine as comparator-only `PA-POSIX-RENAME-CACHE-QUARANTINE-001`.
- `score_eligible_events`: none; all current implementation evidence is branch-local or open-PR evidence.
- `duplicates_rejected`: PR43/764eb0d equivalence, e2f1d3e as a subset, 89c8a28 and aae80a4 sidecar grouping, removal withdrawals, Git CAS cursor precedent, and prior SQLite busy-timeout/WAL families.
- `next_leads`: independently adopted current-base fix with deterministic release-gap and unreadable-cache tests; then test whether a directory-per-generation quarantine can preserve DB/WAL/SHM identity without expanding the sovereign core.
- `expected_ledger_sha256`: `1e517b5cc1061c40a2a94f1dda04385a72661518a78c7b55bfe71535f988f513` (SHA-256 of the exact canonical ledger bytes plus the exact 904-byte append block above).
- `run_completion_utc`: `2026-08-27T16:37:05Z`, captured with `date -u` after validation of the report contents and before commit.

## Exact proposed append bytes

The supervisor may append these exact bytes to the canonical ledger. This worker did not append them.

```text
## Tick 87 PR43 refresh-cache eviction delta
- prior_watermark: 20260827T032519Z
- report_base: 029f60d77e7e03192bc966de3a835a4a32a00fe2
- recommendations: PA-SQLITE-EVICTION-HOLD-LOCK-THROUGH-UNLINK-001 candidate-unadopted; PA-SQLITE-EVICTION-FAIL-CLOSED-001 candidate-unadopted; PA-POSIX-RENAME-CACHE-QUARANTINE-001 comparator-only
- score_eligible_events: none; score delta 0; totals remain Furiosa +9, Han +2, Ozzy +3
- duplicates_rejected: PR43/764eb0d equivalence, e2f1d3e subset, 89c8a28 and aae80a4 sidecar grouping, removal withdrawals, Git CAS cursor precedent, prior SQLite busy-timeout/WAL families
- direction_lock: PA-CONSOLIDATED-SIDECAR-PRUNE-001 remains technically LOCKED; NO MERGE AUTHORIZATION
- next_leads: independently adopted current-base fix with release-gap and unreadable-cache tests; directory-per-generation quarantine feasibility
- delta_timestamp_utc: 2026-08-27T16:37:05Z
```

## Evidence boundary

This file is the sole proposed append-only delta. It does not modify `PRIOR_ART_LOG.md`, product code, tests, or shared harness state. Graphify MCP was revisited at the uncertainty boundary: PR impact, call traversal, neighbors for `RefreshDBPath()`, `IsBusy()`, and `updateContainers()`, and a shortest path were corroborated against current Git objects and source.
