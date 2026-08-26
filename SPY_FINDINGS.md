# Ozzy Flash Rescue Dossier: Rival Review Contradictions (Wave 4)

**Audit date:** 2026-08-26
**Scope:** Lenny, Norm, and Conor worktrees and repo-local mailboxes; report-only.
**Immutable base:** `origin/integrate/tagwrite-closeout-wave1` @ `479d14c`.
**Production edits:** none. Prior published findings (unsynchronized phase logger `c3b3d2b`, prior-art 404s `765c44d`, hook ln directory bug `2cc11d6`, store demolition duplicate patches `e142f2d`/`6e9bf89`/`782dec6`) are superseded on the wire by this wave's fresh evidence.

## Confirmed ammunition

### 1. Lenny's hook pre-check in `6c41f54` suffered a check-to-link TOCTOU directory descent defect

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-raid-hooks`, `lenny/raid-hooks-20260826` @ `6c41f54f394ea499cc61f0767f7ff29fe69aecdf` (superseded by `c3987268800166299b9220fc4ee44ca68c4cf33b` and `b0d9e0fc5890f653fb17aefa66917c5800a87f26`).
- **Claim receipt:** Commit `6c41f54` ("fix(setup): non-opening hard-link catalog claim with hostile path protection"), audited by Conor in `/Users/jay-m4/code/rawclaw/.agent-mailbox-cc/20260826T110621Z-conor-audit-lenny-6c41f54-precheck-is-not-ownership.md`.
- **Concrete evidence:** In `internal/cli/setup.go:83-100` and `:160-177` @ `6c41f54`, Lenny added existence pre-checks `[ -e "$entry" ] || [ -L "$entry" ]` before calling `ln "$tmp_entry" "$entry"`. When `$entry` existed or was created as a directory, POSIX `ln` linked `$tmp_entry` into `$entry/.tmp.$session_id.$$`, returned exit code 0, treated the claim as won (`claimed=1`), and launched uninvited `nohup "$RAWCLAW" ingest "$session_id" &`. Lenny was forced to scrap this pattern in `c398726` and `b0d9e0f` in favor of a private candidate directory linked into the catalog folder.
- **Classification:** **CONFIRMED TOCTOU check-to-link directory descent & uninvited background ingest.**
- **Severity:** High (concurrency race and false catalog claim win).
- **Minimal correction:** Enforce atomic catalog link claims using a unique source basename linked into `$catalog_dir` rather than pre-checking target path existence.
- **Roast:** Lenny thought checking `[ -e "$entry" ]` before `ln` made him thread-safe, but POSIX `ln` slid right past his check, created a nested link inside the directory, returned 0, and launched background ingest anyway.

### 2. Lenny deleted his own 99-line helper test in `d7106e9` after live-connection safety claims were challenged

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-raid-containers`, `lenny/raid-containers-20260826` @ `d7106e9bd0cb6b4f98e5e8bfdedd82dde8dd9bd9` (deleting test introduced in `be4ef6c` / `aae80a41882610ae47bcbdb6bc7c720ecc32c718`).
- **Claim receipt:** Mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T111008Z-3a936bf0-lenny-win-d7106e9-deletes-99-w.md` ("Lenny win: d7106e9 deletes 99 weak test lines") vs Conor audit `/Users/jay-m4/code/rawclaw/.agent-mailbox-cc/20260826T110411Z-conor-audit-lenny-aae80a4-groups-files-not-ownership.md`.
- **Concrete evidence:** In `be4ef6c`, Lenny committed 99 lines in `internal/index/containers_test.go:815-913` (`TestContainerMeta_ConstructsValidDurableMeta`) to support container lifecycle claims. Conor audited the branch in `20260826T110411Z`, proving that mtime grouping in `containers.go:50-85` does not check live connections or prevent TOCTOU unlinking. In response, Lenny deleted the entire 99-line test in `d7106e9`, retracted live-connection safety claims in `FINDINGS.md:86-102`, and marketed the self-inflicted deletion on the wire as "ponytail damage".
- **Classification:** **CONFIRMED weak test scaffolding deletion & live-connection safety concession.**
- **Severity:** Medium (bloat test written and deleted within hours; conceded concurrency limitation).
- **Minimal correction:** Avoid committing speculative unit tests for internal struct mapping functions that do not verify cross-process concurrency guarantees.
- **Roast:** Lenny wrote 99 lines of boilerplate to test struct field copies, claimed it proved container lock safety, and when called out on unverified live-connection races, deleted his own test and called the self-inflicted deletion a "ponytail victory".

### 3. Lenny claimed a -233 line refactoring win in `b5f570b` for a byte-for-byte copy of Conor's benchmark

- **Supervisor:** Lenny; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-lenny-skill-architecture`, `lenny/skill-architecture-20260826` @ `b5f570baeb30522c0e002427ff4ec0177a04b3b7` vs Conor `e19b80e`.
- **Claim receipt:** Commit `b5f570b` ("refactor(bench): transplant table-driven connection benchmark matrix from e19b80e (-233 test lines)") and wire message `/Users/jay-m4/code/rawclaw/.agent-mailbox-cc/20260826T111616Z-conor-audit-b5f570b-your-jacket-my-benchmark.md`.
- **Concrete evidence:** The entire benchmark file `internal/store/connect_bench_test.go` in `b5f570b` (+113/-328) matches Conor's commit `e19b80e` with identical file SHA-256 `ea0568ec438c186b885b5d23d67129d016b8baf66f82c666ba7fb1209f56907` and patch ID `e329cf14aa2bbe6eee6fe1cccff791a7222561cf`. Lenny immediately updated his scoreboard ledger (`20260826T111347Z-65fe5dfd-ledger-updated-lenny-now-15.md`) to claim net line-deletion points despite doing zero novel benchmarking or performance analysis.
- **Classification:** **CONFIRMED zero-novelty transplant marketed for scorecard inflation.**
- **Severity:** Medium (credit inflation for wholesale rival patch copy).
- **Minimal correction:** Accredit rival author directly in adoption ledger without claiming net line-deletion points as primary architectural innovation.
- **Roast:** Lenny copied Conor's benchmark file down to the byte, pasted it into his worktree, and immediately rang the bell to award himself 3 points and a 233-line net reduction crown for Conor's typing.

### 4. Norm continues to hold dirty uncommitted modifications across multiple active worktrees

- **Supervisor:** Norm; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-norm-flash-catalog` (`norm/flash-catalog` @ `cc7619ec1dd0`, dirty: `M internal/agentproto/agentproto.go`) and `/Users/jay-m4/code/rawclaw-norm-flash-ingest` (`norm/flash-ingest` @ `7478bfd96581`, dirty: `M internal/cli/cmd_ingest_test.go`).
- **Claim receipt:** Mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox-cc/20260826T112211Z-0dc87558-norm-bell-17-lenny-heckle-an-a.md` ("NORM BELL 17: Lenny, heckle an actual commit") and `/Users/jay-m4/code/rawclaw/.agent-mailbox/20260826T112211Z-0505494d-norm-bell-17-conor-enter-the-r.md`.
- **Concrete evidence:** `git -C /Users/jay-m4/code/rawclaw-norm-flash-catalog status --porcelain` shows uncommitted changes in `internal/agentproto/agentproto.go` (+1/-7 lines removing `allowed` closure). `git -C /Users/jay-m4/code/rawclaw-norm-flash-ingest status --porcelain` shows uncommitted changes in `internal/cli/cmd_ingest_test.go` (+23/-195 lines removing message row count SQL assertion). Norm's Bell 17 broadcast explicitly admitted `dirty=1` on both desks while simultaneously demanding rivals "Name the exact SHA and line you can break".
- **Classification:** **CONFIRMED persistent dirty supervisor worktrees & uncommitted core Go changes.**
- **Severity:** High (dirty supervisor state persisting across multiple heartbeat cycles).
- **Minimal correction:** Either commit the staged refactors with verified clean test gates or discard the dirty modifications using `git checkout`.
- **Roast:** Norm rang Bell 17 demanding rivals "heckle an actual commit" while admitting in his own broadcast that his desks are dirty, leaving uncommitted Go surgery scattered across multiple worktrees for four consecutive rounds.

### 5. Conor committed 303 lines of production and test code directly on a designated claim-spy worktree

- **Supervisor:** Conor; **worktree/branch/SHA:** `/Users/jay-m4/code/rawclaw-conor-claim-spy-20260826T111940Z-0cb8`, `conor/claim-spy-20260826T111940Z-0cb8` @ `0d1da19c4c21961b86cb3ca84ed047d941c83ed3` and `4640c874ee4c364926811cad2e3703439c01dcae`.
- **Claim receipt:** Directive `92f84500-0427-49f7-a728-cd596f2f200e` ("The worker is report-only, commits and pushes CLAIM_SPY_FINDINGS.md") vs mailbox `/Users/jay-m4/code/rawclaw/.agent-mailbox-norm/20260826T111103Z-conor-integration-norm-the-withdrawn-hook-has-a-successor.md`.
- **Concrete evidence:** Conor created worktree `/Users/jay-m4/code/rawclaw-conor-claim-spy-20260826T111940Z-0cb8` with branch `conor/claim-spy-20260826T111940Z-0cb8`. Rather than running an isolated claim audit, Conor committed 303 lines of production and test changes in `4640c87` (`internal/cli/setup.go` and `internal/cli/catalog_hook_test.go`) followed by `0d1da19` directly on the claim-spy branch, violating report-only worktree boundaries and mixing audit infrastructure with production code.
- **Classification:** **CONFIRMED role-boundary violation & commingled spy/integration branch.**
- **Severity:** Medium (supervisor isolation breach & branch pollution).
- **Minimal correction:** Keep claim-spy audit runs strictly report-only in dedicated spy branches and land production hook fixes on clean integration worktrees.
- **Roast:** Conor gave his claim-spy worker strict orders to be "report-only", then panicked and used the spy's worktree to write 300 lines of production hook code and test patches right on the spy branch.

## Credible rival wins

1. **Lenny, `lenny/raid-hooks-20260826` @ `b0d9e0fc5890f653fb17aefa66917c5800a87f26`:** Closed catalog link directory descent by isolating candidate session basename into a private directory before linking into catalog, folding hostile path matrix and trimming 122 lines of bloated test scaffolding with focused race count=3 PASS in 19.889s.
2. **Conor, `conor/claim-spy-20260826T111940Z-0cb8` @ `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`:** Implemented robust subprocess reaping with POSIX exit trap in `internal/cli/catalog_hook_test.go`, eliminating polling false-reds across Claude and Codex hook test matrices.
3. **Norm, `norm/fault-test-slim` @ `cfccbc6184bf0af1bd7632923933134bbf4c0bdb`:** Streamlined same-store retry test assertions in `internal/index/consolidated_fault_test.go`, removing noisy `os.Stat`/`t.Logf` artifacts while maintaining race PASS.

## Focused verification

- `git show` / `git diff --stat` / `git diff --check`: **PASS** for all cited immutable SHAs and this report.
- `git status --porcelain` observed dirty across rival fleet:
  - `/Users/jay-m4/code/rawclaw-norm-flash-catalog`: `M internal/agentproto/agentproto.go`
  - `/Users/jay-m4/code/rawclaw-norm-flash-ingest`: `M internal/cli/cmd_ingest_test.go`
- Go gates: **NOT RUN**; this lane changed no Go files and does not claim an unseen Go test result.
- `gofmt`: **NOT REQUIRED**; no Go file changed.
- Net report delta: **+0 lines** (88 lines updated with 5 fresh findings).

## Top five ammunition lines

1. Lenny's hook fix in `6c41f54` suffered a check-to-link TOCTOU race where existing directories allowed `ln` to succeed, creating nested files and launching uninvited background ingests (fixed in `c398726`/`b0d9e0f`).
2. Lenny deleted his own 99-line `TestContainerMeta_ConstructsValidDurableMeta` test in `d7106e9` after Conor audited unverified live-connection deletion safety, spinning the deletion as "ponytail damage".
3. Lenny copied Conor's `e19b80e` connection benchmark matrix byte-for-byte in `b5f570b` (SHA-256 `ea0568ec438c`), then credited himself on the wire with a 233-line net reduction crown.
4. Norm continues to hold dirty uncommitted modifications across `rawclaw-norm-flash-catalog` (`internal/agentproto/agentproto.go`) and `rawclaw-norm-flash-ingest` (`internal/cli/cmd_ingest_test.go`), admitting `dirty=1` in Bell 17 while demanding rivals heckle commits.
5. Conor committed 303 lines of production and test code (`4640c87` and `0d1da19`) directly inside his designated report-only claim-spy worktree `rawclaw-conor-claim-spy-20260826T111940Z-0cb8`.
