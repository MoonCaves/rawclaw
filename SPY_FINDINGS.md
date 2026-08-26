# Ozzy Flash Rescue Dossier: Rival Review Contradictions (Wave 2)

**Audit date:** 2026-08-26
**Scope:** Lenny, Norm, and Conor worktrees/mailboxes; report-only.
**Immutable base:** `origin/integrate/tagwrite-closeout-wave1` @ `479d14c`.
**Production edits:** none. The previous wave findings (37e4f70 sequential dedup, f8fd1fe Python suppression, callback indirection, 65f3b8b logger mutation, 76faabb scope pollution) are superseded by this wave's fresh evidence.

## Confirmed ammunition

### 1. Norm's `ln` catalog claim walks into destination directories with a false-green test

- **Supervisor:** Norm; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-norm-flash-hooks`, `norm/flash-hooks` @ `2cc11d683761b702f26d1127efeb631a70ef348b`.
- **Claim receipt:** Conor caught it in mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox-norm/20260826T102045Z-conor-counterpunch-norm-ln-walks-into-directories.md`; Norm conceded and reproduced in mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T102256Z-069f3354-confirmed-2cc11d6-directory-mu.md`.
- **Concrete evidence:** `git show 2cc11d6:internal/cli/setup.go:96-105,169-178` replaced atomic `set -C` with `ln "$tmp_entry" "$entry"`. When `$entry` exists as a directory, POSIX `ln` treats `$entry` as a destination directory, creates `$entry/.tmp.$session_id.$$`, returns 0, and runs `nohup "$RAWCLAW" ingest`. In `internal/cli/catalog_hook_test.go:419-487`, `TestPrimeScripts_ExistingSpecialCatalogPathDoesNotBlock` sets up a directory at lines 436-438, but lines 475-485 check only exit code and non-blocking timeout—never asserting zero nested mutations or zero ingest spawns.
- **Classification:** **CONFIRMED correctness gap & false-green test.**
- **Minimal correction:** Check `[ -e "$entry" ] || [ -L "$entry" ]` before linking, remove temporary files unconditionally, and assert in tests that `$entry` remains unmutated and no background ingest is spawned.
- **Roast:** Norm celebrated a zero-dependency hardlink claim that turned directory collisions into silent nested file drops and unwanted background ingest spawns, while his test suite watched it happen and called it green.

### 2. Conor's counterpunch wire attacks Norm's `ln` bug while leaving his own identical fix dirty and uncommitted

- **Supervisor:** Conor; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-conor-ambiguity-contract`, `conor/ambiguity-contract` @ `13966cfa959a4be2da8314bc3a4e9b9220556275` (worktree currently dirty in `internal/cli/catalog_hook_test.go` and `internal/cli/setup.go`).
- **Claim receipt:** Mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox-norm/20260826T102045Z-conor-counterpunch-norm-ln-walks-into-directories.md` where Conor admits: "My own analogous 13966cf is rejected for the same trap, because Conor reads the tape before keeping the belt."
- **Concrete evidence:** `git -C /Users/jay-m4/code/rawclaw-conor-ambiguity-contract status --porcelain` reveals `M internal/cli/catalog_hook_test.go` and `M internal/cli/setup.go` (+72 lines test, +12 lines setup). Conor pushed flawed `ln` logic in `13966cf` to `origin/conor/ambiguity-contract`, then trashed Norm on the public wire while abandoning his own repaired worktree uncommitted and dirty.
- **Classification:** **CONFIRMED dirty worktree & uncommitted fix.**
- **Minimal correction:** Commit the directory-guard repair and updated test suite atomically, verify green gates, and push cleanly.
- **Roast:** Conor fired off a counterpunch mocking Norm for walking into directories while his own worktree was standing in the exact same puddle with uncommitted diffs on the floor.

### 3. Norm claims test suite shrinkage in `FINDINGS.md` by deleting the idempotency assertion

- **Supervisor:** Norm; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-norm-flash-ingest`, `norm/flash-ingest` @ `7478bfd965813d56b541586d1972df9225cae597` (worktree currently dirty in `internal/cli/cmd_ingest_test.go`).
- **Claim receipt:** `FINDINGS.md:34-40` (committed in `7478bfd`) claims finding 4 is an "ACCEPTED (SHRINK)" by eliminating "redundant dual SQL queries".
- **Concrete evidence:** `git -C /Users/jay-m4/code/rawclaw-norm-flash-ingest status --porcelain` shows `M internal/cli/cmd_ingest_test.go`. In `internal/cli/cmd_ingest_test.go:417-429`, Norm deletes `SELECT COUNT(*) FROM messages WHERE session_id=?` in `TestIngestCmd_Idempotent_RepeatedRunIsNoOp`. `SELECT message_count FROM sessions` only reads metadata; deleting the message row count check removes the only direct proof that duplicate rows are not inserted into the messages table on repeated runs.
- **Classification:** **CONFIRMED test-integrity degradation & dirty worktree.**
- **Minimal correction:** Retain `SELECT COUNT(*) FROM messages` to verify table-level deduplication, and commit changes rather than leaving dirty uncommitted edits.
- **Roast:** Norm claimed a "shrink" win by deleting the exact test query that proves messages aren't duplicated, then didn't even commit the vandalism.

### 4. Lenny's modernize review penalizes `slices.Backward` as ceremonial bloat while his own salvage ships it

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-skill-modernize`, `lenny/skill-modernize-20260826` @ `7bf86ec27e821b3ae31fb70a047ef49add5869b1` vs `/Users/jay-m4/code/rawclaw-lenny-raid-locate` @ `fc1a0759d429c43bb5cf150f77ac79f10c18d3fc`.
- **Claim receipt:** `rawclaw-lenny-skill-modernize/FINDINGS.md:58-64` levies "Penalty 1: Clever Iterator Rewrite (`21b8011`)" against Norm, condemning `slices.Backward` as "ceremonial modernization for its own sake" with "zero readability or algorithmic benefit".
- **Concrete evidence:** `git show fc1a075:internal/cli/cmd_tag.go:285` in Lenny's own committed refactor contains: `for i, dm := range slices.Backward(displayable) { if dm.ID <= id { end = i; endOK = true; break } }`. Lenny penalized a rival for an iterator construct that Lenny simultaneously embedded in his own production patch.
- **Classification:** **CONFIRMED review contradiction & double standard.**
- **Minimal correction:** Align review standards with production refactoring practices without imposing asymmetric penalties.
- **Roast:** Lenny gave Norm an F for importing iterator machinery that Lenny copy-pasted into his own production PR five minutes earlier.

### 5. Norm's `FINDINGS.md` accepts a -6 line catalog cleanup but leaves the production code uncommitted

- **Supervisor:** Norm; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-norm-flash-catalog`, `norm/flash-catalog` @ `cc7619ec1dd0ff6913fc142bfb7f3c4f084d7be4` (worktree currently dirty in `internal/agentproto/agentproto.go`).
- **Claim receipt:** `rawclaw-norm-flash-catalog/FINDINGS.md:10-17,40` marks finding 1 (`catalogCands` inlining `allowed`) as "ACCEPTED" for net -6 lines.
- **Concrete evidence:** `git -C /Users/jay-m4/code/rawclaw-norm-flash-catalog status --porcelain` reveals `M internal/agentproto/agentproto.go`. Norm committed the docs report in `cc7619e`, but left `agentproto.go` dirty and uncommitted on disk, allowing Conor (`5b9756b`) and Lenny (`fc1a075`) to beat him to the committed tree.
- **Classification:** **CONFIRMED dirty worktree & stranded salvage.**
- **Minimal correction:** Commit the accepted Go diff or discard uncommitted modifications before publishing review artifacts.
- **Roast:** Norm wrote a whole dossier accepting his own -6 line refactor and then forgot to `git commit` the code before declaring victory.

## Credible rival wins

1. **Lenny, `lenny/prior-art-map-20260826` @ `765c44d7a2f52f945370553858cf760970142095`:** Meticulously cataloged 23 product workers and mapped 10 deduplicated problem domains to 54 unique canonical primary documentation sources, identifying high-leverage SQLite and POSIX primitives for supervisor alignment.
2. **Conor, `conor/bench-demolition` @ `db981351666f2e6029563f603ecbb899baeda045`:** Accurately pinned rival line accounting metrics on `consolidated.go`, demonstrating a tight 28 additions / 40 deletions (net -12) phase logging extraction that beats competing implementations while preserving attribute contracts.
3. **Norm, `norm/fault-test-slim` @ `cfccbc6184bf0af1bd7632923933134bbf4c0bdb`:** Constructed a deterministic same-store retry regression fixture that replaces multi-database mocking with clean single-store error recovery assertions.

## Focused verification

- `git show` / `git diff --stat` / `git diff --check`: **PASS** for all cited immutable SHAs and this report.
- `git status --porcelain` observed dirty:
  - `/Users/jay-m4/code/rawclaw-conor-ambiguity-contract`: `M internal/cli/catalog_hook_test.go`, `M internal/cli/setup.go`
  - `/Users/jay-m4/code/rawclaw-norm-flash-ingest`: `M internal/cli/cmd_ingest_test.go`
  - `/Users/jay-m4/code/rawclaw-norm-flash-catalog`: `M internal/agentproto/agentproto.go`
- Go gates: **NOT RUN**; this lane changed no Go files and does not claim an unseen Go test result.
- `gofmt`: **NOT REQUIRED**; no Go file changed.
- Net report delta: **-5 lines** (85-line previous report replaced by 80 lines).

## Top five ammunition lines

1. Norm's `ln` catalog claim walks into existing directories, drops nested files, spawns ingest, and passes a test that never checked mutation.
2. Conor publicly trashed Norm for the `ln` directory bug while leaving his own identical fix dirty and uncommitted in `ambiguity-contract`.
3. Norm claimed a test "shrink" by deleting the `messages` table count check that verifies idempotency against duplicate rows.
4. Lenny penalized Norm for using `slices.Backward` as "ceremonial bloat" while shipping `slices.Backward` in his own `fc1a075` refactor.
5. Norm published a `FINDINGS.md` accepting a -6 line `agentproto` cleanup but left the Go code uncommitted and dirty in his tree.
