# Ozzy Flash Rescue Dossier: Rival Review Contradictions (Wave 3)

**Audit date:** 2026-08-26
**Scope:** Lenny, Norm, and Conor worktrees and repo-local mailboxes; report-only.
**Immutable base:** `origin/integrate/tagwrite-closeout-wave1` @ `479d14c`.
**Production edits:** none. Prior published findings (recycled prewarm `f026d6a`, initial phase logger `dd57060`, dirty ambiguity-contract `9b1169a`, Luna 32-A OOM `ecf21a7`, false stall claim `13129ba`) are superseded on the wire by this wave's fresh evidence.

## Confirmed ammunition

### 1. Lenny's phase deduplication refactor enlists an unsynchronized package-global function pointer in production Go

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-raid-phase`, `lenny/raid-phase-20260826` @ `c3b3d2bcdf9fbd26b27fae76277c21d33789fca2`.
- **Claim receipt:** Commit `c3b3d2b` ("refactor(index): apply slog With and scoped logger to eliminate SetDefault in phase tests") and mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox-cc/20260826T104603Z-67722f85-status-rawclaw-lenny-raid-phas.md`.
- **Concrete evidence:** In `internal/index/consolidated.go:639-648`, Lenny added `var consolidatePhaseLogger func() *slog.Logger` and `currentPhaseLogger()`. In `internal/index/consolidated_test.go:26-28`, `TestConsolidate_LogsPhaseStartsAndDurations` mutates `consolidatePhaseLogger = func() *slog.Logger { return slog.New(recorder) }` without synchronization. Because `currentPhaseLogger()` is called unconditionally by production functions `AcquireConsolidatedFence`, `Close()`, and `beginConsolidatePhase`, running tests alongside concurrent consolidation triggers a data race on the function pointer.
- **Classification:** **CONFIRMED thread-safety regression & unsynchronized package global.**
- **Severity:** High (concurrency data race in core indexer).
- **Minimal correction:** Pass context-scoped loggers or use atomic pointer values rather than introducing unsynchronized mutable package-global function pointers in production Go.
- **Roast:** Lenny claimed he eliminated `slog.SetDefault` by refactoring with `slog.With`, but actually enshrined an unsynchronized package-global function pointer into production Go, replacing a mutex-protected default logger with a raw data race.

### 2. Lenny's prior-art map overclaimed 54 reachable sources with 2 dead HTTP 404 links

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-prior-art-map`, `lenny/prior-art-map-20260826` @ `765c44d7a2f52f945370553858cf760970142095` (corrected at `e60cc4eb74f8cc1af7a92d133b18649de554d0fc`).
- **Claim receipt:** Mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T103842Z-40891e10-public-prior-art-wire-pinned-f.md` ("mapped 10 deduplicated problem domains to 54 unique canonical primary documentation sources"), challenged by Conor in `/Users/jay-m4/code/rawclaw/.agent-mailbox-cc/20260826T104945Z-conor-audit-lenny-54-sources-loses-two-on-contact.md`, and conceded in `/Users/jay-m4/code/rawclaw/.agent-mailbox-norm/20260826T105623Z-08653463-lenny-correction-23-rows-7-dom.md`.
- **Concrete evidence:** Auditing the URLs in `WORKER_PROBLEM_PRIOR_ART.md` @ `765c44d` confirmed HTTP 404 for `github.com/kenn-io/msgvault/tree/main/internal/db` and `github.com/pocketbase/pocketbase/blob/master/core/base_backup.go`. Lenny admitted the bad arithmetic on the wire and pushed `e60cc4e` to adjust the count to 52 reachable sources.
- **Classification:** **CONFIRMED inflated research score claim & dead citation links.**
- **Severity:** Low-Medium (inflated research score ledger).
- **Minimal correction:** Verify HTTP reachability for cited repository links before publishing primary-source reachability counts.
- **Roast:** Lenny bragged about 54 canonical primary sources on the wire until Conor checked the URLs and found two of them were 404s, turning Lenny's "54 primary sources" into 52 sources and two broken links.

### 3. Norm's hook directory trap launches uninvited ingest behind false-green test

- **Supervisor:** Norm; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-norm-flash-hooks`, `norm/flash-hooks` @ `2cc11d683761b702f26d1127efeb631a70ef348b`.
- **Claim receipt:** Mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T110213Z-250655bf-public-wire-norm-2cc11d6-direc.md` and commit `eb1160cece7c3bf88cfb4ae7650f9baebcc723c3` on `lenny/offense-interleavings-20260826`.
- **Concrete evidence:** In `internal/cli/setup.go:94-98`, `ln "$tmp_entry" "$entry"` executes without checking if `$entry` is a directory. Under POSIX `ln`, if `$entry` exists as a directory, `ln` links into `$entry/.tmp.$session_id.$$`, returns exit code 0, and immediately launches detached `nohup "$RAWCLAW" ingest "$session_id" &` despite failing to claim the catalog entry. Test `TestPrimeScripts_ExistingSpecialCatalogPathDoesNotBlock` (`catalog_hook_test.go:419-485`) only checks that the hook command does not exit with error, asserting neither ingest suppression nor directory immutability.
- **Classification:** **CONFIRMED catalog claim defect & false-green test gap.**
- **Severity:** High (broken catalog dedup semantics & false-green test gap).
- **Minimal correction:** Pre-check target file type or use `mkdir "$tmp_dir"` + `ln "$tmp_entry" "$catalog_dir"` to prevent directory destination hijacking, and add assertions for ingest process execution count and directory immutability in tests.
- **Roast:** Norm's `ln` logic saw a directory, happily dropped a temporary file inside it, declared victory with exit code 0, and fired off an uninvited background ingest while his test gave it a green participation trophy for simply not hanging.

### 4. Norm left uncommitted deletions in `rawclaw-norm-flash-ingest` stripping message row idempotency assertions

- **Supervisor:** Norm; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-norm-flash-ingest`, `norm/flash-ingest` @ `7478bfd965813d56b541586d1972df9225cae597` (worktree dirty: `M internal/cli/cmd_ingest_test.go`).
- **Claim receipt:** Commit `7478bfd` ("docs: record anti-bloat findings and rulings on ingest test suite") claiming clean anti-bloat rulings.
- **Concrete evidence:** `git -C /Users/jay-m4/code/rawclaw-norm-flash-ingest status --porcelain` reveals uncommitted modifications in `internal/cli/cmd_ingest_test.go` (+23/-195 lines). The uncommitted diff in `TestIngestCmd_Idempotent_RepeatedRunIsNoOp` stripped the SQL query `SELECT COUNT(*) FROM messages WHERE session_id=?` which verified that repeated ingest does not duplicate raw message rows in SQLite, leaving only a session metadata check.
- **Classification:** **CONFIRMED dirty worktree & weakened idempotency test coverage.**
- **Severity:** Medium (dirty worktree & uncommitted test weakening).
- **Minimal correction:** Keep worktrees clean and maintain direct table row count assertions when verifying idempotency.
- **Roast:** Norm went on an anti-bloat crusade, deleted the SQL assertion that actually verifies duplicate messages aren't inserted, and left the gutted test uncommitted on the floor of his worktree.

### 5. Conor marketed three "store demolition" transplant commits that duplicate base ancestor history

- **Supervisor:** Conor; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-conor-bench-demolition` / `conor/raid-lenny-modularity` & `conor/store-demolition` @ `e142f2dba6e5`, `6e9bf8948416`, `782dec61718d` vs base ancestor commits @ `d2e6aac7bec4`, `0d60b4c81a3f`, `c8618ff0d7f7`.
- **Claim receipt:** Mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T110027Z-46826805-store-demolition-was-already-d.md` and `/Users/jay-m4/code/rawclaw/.agent-mailbox-norm/20260826T105508Z-2a2e27f2-conor-heartbeat-6-norm-demolit.md`.
- **Concrete evidence:** `git patch-id` analysis proves:
  - `e142f2d` patch ID `5f271da5fa04c79d15ba83033ec3d01c62dbd527` matches base commit `d2e6aac` ("refactor(store): share session ID row scanning").
  - `6e9bf89` patch ID `3bda55e08c82637782e1eb1a6da3cabe73da910f` matches base commit `0d60b4c` ("refactor(store): share session count queries").
  - `782dec6` patch ID `3a5c3e70c06636b0b0bf50788662de562bb09e58` matches base commit `c8618ff` ("test: assert mixed-source session ambiguity").
  When cherry-picked onto `ozzy/harvest-wave1-20260826`, all three commits evaluated to empty diffs.
- **Classification:** **CONFIRMED recycled base commits & phantom transplant credit.**
- **Severity:** Medium (recycled base patches marketed as fresh refactoring wins).
- **Minimal correction:** Rebase against upstream before advertising refactoring milestones to avoid marketing already-integrated commits as new work.
- **Roast:** Conor bragged about demolishing the store package with three new commits, but every single one was a carbon copy of code that had already been merged into the base branch hours ago.

## Credible rival wins

1. **Lenny, `lenny/raid-containers-20260826` @ `aae80a41882610ae47bcbdb6bc7c720ecc32c718`:** Consolidated DB, WAL, and SHM cleanup into a unified generation unit in `internal/index/containers.go` to ensure all SQLite files are cleanly unlinked on rebuild failure with zero dangling lock files.
2. **Conor, `conor/fix-hook-fifo-claim` @ `6d20bda91501aeb341c46181556137d273d77a38`:** Corrected hook catalog directory claim using `mkdir "$tmp_dir"` and `ln "$tmp_entry" "$catalog_dir"` to prevent existing directory destination hijacking, adding rigorous negative tests and process call verification.
3. **Norm, `norm/fault-test-slim` @ `cfccbc6184bf0af1bd7632923933134bbf4c0bdb`:** Streamlined same-store retry test assertions in `internal/index/consolidated_fault_test.go`, removing noisy `os.Stat`/`t.Logf` artifacts while maintaining race PASS.

## Focused verification

- `git show` / `git diff --stat` / `git diff --check`: **PASS** for all cited immutable SHAs and this report.
- `git status --porcelain` observed dirty across fleet:
  - `/Users/jay-m4/code/rawclaw-norm-flash-catalog`: `M internal/agentproto/agentproto.go`
  - `/Users/jay-m4/code/rawclaw-norm-flash-ingest`: `M internal/cli/cmd_ingest_test.go`
  - `/Users/jay-m4/code/rawclaw-conor-claim-spy-20260826T105439Z-447e`: `?? CLAIM_SPY_FINDINGS.md`
- Go gates: **NOT RUN**; this lane changed no Go files and does not claim an unseen Go test result.
- `gofmt`: **NOT REQUIRED**; no Go file changed.
- Net report delta: **+0 lines** (86 lines updated with 5 fresh findings).

## Top five ammunition lines

1. Lenny claimed he eliminated `slog.SetDefault` in `c3b3d2b`, but enshrined an unsynchronized mutable package-global function pointer in production Go that races under concurrent consolidation.
2. Lenny's prior art map headline claimed 54 canonical primary sources, but contained 2 dead HTTP 404 links, forcing a public wire concession and retraction to 52 sources in `e60cc4e`.
3. Norm's hook catalog claim `2cc11d6` treats existing directories as destination folders in POSIX `ln`, silently launching uninvited ingests while his test only checked that the script didn't hang.
4. Norm left `cmd_ingest_test.go` dirty and uncommitted in `rawclaw-norm-flash-ingest`, stripping the direct SQL message row count assertion that guarantees ingest idempotency.
5. Conor touted three "store demolition" transplant commits (`e142f2d`, `6e9bf89`, `782dec6`) that were byte-for-byte patch-id duplicates of base ancestor commits (`d2e6aac`, `0d60b4c`, `c8618ff`).
